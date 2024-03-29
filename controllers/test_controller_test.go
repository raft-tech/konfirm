/*
 Copyright 2022 Raft, LLC

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package controllers_test

import (
	"context"
	"errors"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strconv"
	"time"
)

var _ = Describe("On Test Controller reconciliation", func() {

	const (
		timeout = "500ms"
	)

	var (
		ctx       context.Context
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		if ns, err := generateNamespace(); err == nil {
			namespace = ns
			Expect(k8sClient.Create(ctx, &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			})).NotTo(HaveOccurred())
		} else {
			Expect(err).NotTo(HaveOccurred())
		}
	})

	When("a test is created", func() {

		var (
			test *konfirm.Test
		)

		BeforeEach(func() {
			test = &konfirm.Test{
				TypeMeta: metav1.TypeMeta{
					APIVersion: konfirm.GroupVersion.String(),
					Kind:       "Test",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a-test",
					Namespace: namespace,
				},
				Spec: konfirm.TestSpec{
					RetentionPolicy: konfirm.RetainOnFailure,
					Template: v1.PodTemplateSpec{
						Spec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Name:  "a-test-container",
									Image: "a-test-image",
								},
							},
						},
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, test)).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if test.DeletionTimestamp == nil {
				err := k8sClient.Delete(ctx, test)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred())
			}
		})

		It("an associated pod should be created", func() {
			Eventually(func() bool {
				pods := &v1.PodList{}
				if err := k8sClient.List(ctx, pods, &client.ListOptions{Namespace: test.Namespace}); err == nil {
					for _, p := range pods.Items {
						for _, o := range p.GetOwnerReferences() {
							if o.Kind == "Test" && o.Name == test.Name {
								return o.Controller != nil &&
									*o.Controller &&
									*o.BlockOwnerDeletion &&
									p.Spec.RestartPolicy == v1.RestartPolicyNever
							}
						}
					}
				}
				return false
			}, timeout).Should(BeTrue())
		})

		It("it should reach the Starting phase", func() {
			Eventually(func() (phase konfirm.TestPhase, err error) {
				if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
					phase = test.Status.Phase
				}
				return
			}, timeout).Should(Equal(konfirm.TestStarting))
			Expect(test.Status.Conditions).To(ContainElement(And(
				HaveField("Type", controllers.PodCreatedCondition),
				HaveField("Status", metav1.ConditionTrue),
				HaveField("Reason", "PodCreated"),
			)), "have the expected PodCreated condition")
			Expect(test.Status.Conditions).To(ContainElement(And(
				HaveField("Type", controllers.TestCompletedCondition),
				HaveField("Status", metav1.ConditionFalse),
				HaveField("Reason", "PodNotCompleted"),
			)), "have the expected TestCompleted condition")
		})

		Context("and the associated pod exists", func() {

			var pod *v1.Pod

			JustBeforeEach(func() {
				Eventually(func() (err error) {
					pods := v1.PodList{}
					if err = k8sClient.List(ctx, &pods, client.InNamespace(namespace)); err != nil {
						return
					}
					for i := range pods.Items {
						for _, o := range pods.Items[i].OwnerReferences {
							if o.UID == test.UID {
								pod = &pods.Items[i]
								return nil
							}
						}
					}
					return errors.New("pod not found")
				}, timeout).ShouldNot(HaveOccurred())
			})

			When("and the pod is running", func() {

				JustBeforeEach(func() {
					orig := pod.DeepCopy()
					pod.Status.Phase = v1.PodRunning
					err := k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))
					Expect(err).NotTo(HaveOccurred())
				})

				It("the test should be running", func() {
					Eventually(func() (phase konfirm.TestPhase, err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
							phase = test.Status.Phase
						}
						return
					}, timeout).Should(Equal(konfirm.TestRunning))
					Expect(test.Status.Conditions).To(ContainElement(And(
						HaveField("Type", controllers.PodCreatedCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "PodCreated"),
					)), "have the expected PodCreated condition")
				})
			})

			When("and the pod succeeds", func() {

				JustBeforeEach(func() {
					orig := pod.DeepCopy()
					pod.Status.Phase = v1.PodSucceeded
					pod.Status.ContainerStatuses = make([]v1.ContainerStatus, len(test.Spec.Template.Spec.Containers))
					for i := range test.Spec.Template.Spec.Containers {
						state := v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode:    0,
								Reason:      "Container exited",
								Message:     "Success!",
								StartedAt:   metav1.NewTime(test.CreationTimestamp.Add(time.Millisecond * 1)),
								FinishedAt:  metav1.NewTime(test.CreationTimestamp.Add(time.Millisecond * 10)),
								ContainerID: strconv.Itoa(i),
							}}
						pod.Status.ContainerStatuses[i] = v1.ContainerStatus{
							Name:                 test.Spec.Template.Spec.Containers[i].Name,
							State:                state,
							LastTerminationState: state,
							Ready:                false,
							RestartCount:         0,
							Image:                test.Spec.Template.Spec.Containers[i].Image,
							Started:              &no,
						}
					}
					err := k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))
					Expect(err).NotTo(HaveOccurred())
				})

				It("the test should pass", func() {
					Eventually(func() (phase konfirm.TestPhase, err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
							phase = test.Status.Phase
						}
						return
					}, timeout).Should(Equal(konfirm.TestPassed))
					Expect(test.Status.Conditions).To(ContainElement(And(
						HaveField("Type", controllers.PodCreatedCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "PodCreated"),
					)), "have the expected PodCreated condition")
					Expect(test.Status.Conditions).To(ContainElement(And(
						HaveField("Type", controllers.TestCompletedCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "PodSucceeded"),
					)), "have the expected TestCompleted condition")
				})

				It("the pod is deleted", func() {
					Eventually(func() bool {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
						return apierrors.IsNotFound(err)
					}, timeout).Should(BeTrue())
				})

				When("the retention policy is Always", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirm.RetainAlways
					})

					It("it retains the pod", func() {
						Consistently(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
							return apierrors.IsNotFound(err)
						}, timeout).Should(BeFalse())
					})

					When("the pod is manually deleted", func() {

						JustBeforeEach(func() {

							// Allow the test to complete
							Eventually(func() (phase konfirm.TestPhase, err error) {
								if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
									phase = test.Status.Phase
								}
								return
							}, timeout).Should(Equal(konfirm.TestPassed))

							// Delete the pod (i.e., manual deletion, eviction, etc.)
							Expect(k8sClient.Delete(ctx, pod)).NotTo(HaveOccurred())
						})

						It("it removes the finalizer", func() {
							Eventually(func() error {
								return k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)
							}, timeout).Should(Satisfy(apierrors.IsNotFound))
						})
					})
				})

				When("the retention policy is Never", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirm.RetainNever
					})

					It("it deletes the pod", func() {
						Eventually(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
							return apierrors.IsNotFound(err)
						}, timeout).Should(BeTrue())
					})
				})

				When("the retention policy is OnFailure", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirm.RetainOnFailure
					})

					It("it deletes the pod", func() {
						Eventually(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
							return apierrors.IsNotFound(err)
						}, timeout).Should(BeTrue())
					})
				})
			})

			When("the pod fails", func() {

				JustBeforeEach(func() {
					orig := pod.DeepCopy()
					pod.Status.Phase = v1.PodFailed
					pod.Status.ContainerStatuses = make([]v1.ContainerStatus, len(test.Spec.Template.Spec.Containers))
					for i := range test.Spec.Template.Spec.Containers {
						state := v1.ContainerState{
							Terminated: &v1.ContainerStateTerminated{
								ExitCode:    1,
								Reason:      "Container exited",
								Message:     "Failure!",
								StartedAt:   metav1.NewTime(test.CreationTimestamp.Add(time.Millisecond * 1)),
								FinishedAt:  metav1.NewTime(test.CreationTimestamp.Add(time.Millisecond * 10)),
								ContainerID: strconv.Itoa(i),
							}}
						pod.Status.ContainerStatuses[i] = v1.ContainerStatus{
							Name:                 test.Spec.Template.Spec.Containers[i].Name,
							State:                state,
							LastTerminationState: state,
							Ready:                false,
							RestartCount:         0,
							Image:                test.Spec.Template.Spec.Containers[i].Image,
							Started:              &no,
						}
					}
					err := k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))
					Expect(err).NotTo(HaveOccurred())
				})

				It("the test should fail", func() {
					Eventually(func() (phase konfirm.TestPhase, err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
							phase = test.Status.Phase
						}
						return
					}, timeout).Should(Equal(konfirm.TestFailed))
					Expect(test.Status.Conditions).To(ContainElement(And(
						HaveField("Type", controllers.PodCreatedCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "PodCreated"),
					)), "have the expected PodCreated condition")
					Expect(test.Status.Conditions).To(ContainElement(And(
						HaveField("Type", controllers.TestCompletedCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "PodFailed"),
					)), "have the expected TestCompleted condition")
				})

				It("the pod is retained", func() {
					Consistently(func() bool {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
						return apierrors.IsNotFound(err)
					}, timeout).Should(BeFalse())
				})

				When("the pod is manually deleted", func() {

					JustBeforeEach(func() {

						// Allow the test to complete
						Eventually(func() (phase konfirm.TestPhase, err error) {
							if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
								phase = test.Status.Phase
							}
							return
						}, timeout).Should(Equal(konfirm.TestFailed))

						// Delete the pod (i.e., manual deletion, eviction, etc.)
						Expect(k8sClient.Delete(ctx, pod)).NotTo(HaveOccurred())
					})

					It("it removes the finalizer", func() {
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)
						}, timeout).Should(Satisfy(apierrors.IsNotFound))
					})
				})

				When("the retention policy is Always", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirm.RetainAlways
					})

					It("it retains the pod", func() {
						Consistently(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
							return apierrors.IsNotFound(err)
						}, timeout).Should(BeFalse())
					})

					When("the pod is manually deleted", func() {

						JustBeforeEach(func() {

							// Allow the test to complete
							Eventually(func() (phase konfirm.TestPhase, err error) {
								if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
									phase = test.Status.Phase
								}
								return
							}, timeout).Should(Equal(konfirm.TestFailed))

							// Delete the pod (i.e., manual deletion, eviction, etc.)
							Expect(k8sClient.Delete(ctx, pod)).NotTo(HaveOccurred())
						})

						It("it removes the finalizer", func() {
							Eventually(func() error {
								return k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)
							}, timeout).Should(Satisfy(apierrors.IsNotFound))
						})
					})
				})

				When("the retention policy is Never", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirm.RetainNever
					})

					It("it deletes the pod", func() {
						Eventually(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
							return apierrors.IsNotFound(err)
						}, timeout).Should(BeTrue())
					})
				})

				When("the retention policy is OnFailure", func() {

					BeforeEach(func() {
						test.Spec.RetentionPolicy = konfirm.RetainOnFailure
					})

					It("it retains the pod", func() {
						Consistently(func() bool {
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
							return apierrors.IsNotFound(err)
						}, timeout).Should(BeFalse())
					})
				})
			})

			When("a pod is evicted after completing", func() {

				JustBeforeEach(func() {

					origPod := pod

					// Create the pod, but do not associate it with the Test yet
					pod = &v1.Pod{
						ObjectMeta: test.Spec.Template.ObjectMeta,
						Spec:       test.Spec.Template.Spec,
					}
					pod.ObjectMeta.Name = ""
					pod.ObjectMeta.Namespace = test.Namespace
					pod.ObjectMeta.GenerateName = test.Name + "-"
					pod.ObjectMeta.Finalizers = []string{
						controllers.TestControllerFinalizer,
						konfirm.GroupName + "/testing",
					}
					pod.Spec.RestartPolicy = v1.RestartPolicyNever
					Expect(k8sClient.Create(ctx, pod)).NotTo(HaveOccurred())

					// Progress the pod to Success and delete it
					pod.Status.Phase = v1.PodSucceeded
					Eventually(func() (err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod); err == nil {
							pod.Status.Phase = v1.PodSucceeded
							err = k8sClient.Status().Update(ctx, pod)
						}
						return
					}, timeout).ShouldNot(HaveOccurred())
					Eventually(func() (err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod); err == nil {
							err = k8sClient.Delete(ctx, pod)
						}
						return
					}, timeout).ShouldNot(HaveOccurred())

					// Associate the pod with the test
					Eventually(func() (err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod); err == nil {
							yes := true
							pod.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
								{
									APIVersion:         konfirm.GroupVersion.String(),
									Kind:               "Test",
									Name:               test.Name,
									UID:                test.UID,
									Controller:         &yes,
									BlockOwnerDeletion: &yes,
								},
							}
							err = k8sClient.Update(ctx, pod)
						}
						return
					}, timeout).ShouldNot(HaveOccurred())

					// Delete the original Pod
					Expect(k8sClient.Delete(ctx, origPod)).NotTo(HaveOccurred())
				})

				JustAfterEach(func() {
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).NotTo(HaveOccurred())
					pod.Finalizers = []string{}
					Expect(k8sClient.Update(ctx, pod)).NotTo(HaveOccurred())
				})

				It("the test should pass", func() {
					Eventually(func() (phase konfirm.TestPhase, err error) {
						if err = k8sClient.Get(ctx, client.ObjectKeyFromObject(test), test); err == nil {
							phase = test.Status.Phase
						}
						return
					}, timeout).Should(Equal(konfirm.TestPassed))
				})
			})

			When("when the pod is evicted", func() {

				JustBeforeEach(func() {

					// Ensure the finalizer is set
					Eventually(func() (*v1.Pod, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)
						return pod, err
					}, timeout).Should(HaveField("Finalizers", ContainElement(controllers.TestControllerFinalizer)))

					// Delete the pod
					Expect(k8sClient.Delete(ctx, pod)).NotTo(HaveOccurred())
				})

				It("it removes the finalizer", func() {
					Eventually(func() error {
						return k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)
					}, timeout).Should(Satisfy(apierrors.IsNotFound))
				})

				It("creates a new pod", func() {
					Eventually(func() ([]v1.Pod, error) {
						return getPods(ctx, test)
					}, timeout).Should(ContainElement(HaveField("UID", Not(Equal(pod.UID)))))
				})
			})

			When("the test is deleted", func() {

				JustBeforeEach(func() {
					err := k8sClient.Delete(ctx, test)
					Expect(err).NotTo(HaveOccurred())
				})

				It("the pod should be deleted", func() {
					Eventually(func() bool {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &v1.Pod{})
						return apierrors.IsNotFound(err)
					}, timeout).Should(BeTrue())
				})

				It("the test should be deleted", func() {
					Eventually(func() bool {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(test), &konfirm.Test{})
						return apierrors.IsNotFound(err)
					}, timeout).Should(BeTrue())
				})
			})
		})
	})
})
