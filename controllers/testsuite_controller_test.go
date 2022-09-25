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
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	"github.com/raft-tech/konfirm/controllers"
	"github.com/robfig/cron/v3"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/clock"
	tclock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("TestSuite Controller", func() {

	const timeout = "100ms"

	var (
		ctx       context.Context
		namespace string
		testSuite *konfirm.TestSuite
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

		testSuite = &konfirm.TestSuite{
			TypeMeta: metav1.TypeMeta{
				APIVersion: konfirm.GroupVersion.String(),
				Kind:       "TestSuite",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "a-test-suite",
				Namespace: namespace,
			},
			Spec: konfirm.TestSuiteSpec{
				Template: konfirm.TestRunSpec{
					Tests: []konfirm.TestTemplate{
						{
							Description: "a-test",
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
					},
				},
			},
		}
	})

	When("a test suite is created", func() {

		JustBeforeEach(func() {
			Expect(k8sClient.Create(ctx, testSuite)).NotTo(HaveOccurred())
		})

		JustAfterEach(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, testSuite))).NotTo(HaveOccurred())
		})

		It("it reaches the Ready state", func() {
			Eventually(func() (konfirm.TestSuitePhase, error) {
				return testSuite.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
			}, timeout).Should(Equal(konfirm.TestSuiteReady))
		})

		When("a schedule is defined", func() {

			var next *time.Time

			BeforeEach(func() {
				// Ensure an hour between now and the next test run
				now := time.Now()
				min, hour := now.Minute(), now.Hour()+1
				if hour > 23 {
					hour = hour - 24
				}
				testSuite.Spec.When.Schedule = fmt.Sprintf("%d %d * * *", min, hour)
			})

			JustBeforeEach(func() {
				if s, err := cron.ParseStandard(testSuite.Spec.When.Schedule); err == nil {
					n := s.Next(time.Now())
					next = &n
				}
			})

			It("schedules the next test run", func() {
				Eventually(func() (*metav1.Time, error) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
					return testSuite.Status.NextRun, err
				}, timeout).Should(Satisfy(func(t interface{}) bool {
					if t, ok := t.(*metav1.Time); ok {
						return t != nil && next.Equal(t.Time)
					} else {
						return false
					}
				}))
				Expect(testSuite.Annotations[controllers.TestSuiteScheduleAnnotation]).To(Equal(testSuite.Spec.When.Schedule))
				Expect(testSuite.Status.Conditions).To(ContainElement(And(
					HaveField("Type", controllers.TestSuiteHasScheduleCondition),
					HaveField("Status", metav1.ConditionTrue),
					HaveField("Reason", "ScheduleSet"),
					HaveField("Message", "schedule is set"),
				)))
			})

			It("clears the schedule when it is removed", func() {

				// Wait for NextRun to be set
				Eventually(func() (*metav1.Time, error) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
					return testSuite.Status.NextRun, err
				}, timeout).Should(Satisfy(func(t interface{}) bool {
					if t, ok := t.(*metav1.Time); ok {
						return t != nil && next.Equal(t.Time)
					} else {
						return false
					}
				}))

				// Remove the schedule
				orig := testSuite.DeepCopy()
				testSuite.Spec.When.Schedule = ""
				Expect(k8sClient.Patch(ctx, testSuite, client.MergeFrom(orig))).NotTo(HaveOccurred())

				// Verify next run is removed
				Eventually(func() (*metav1.Time, error) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
					return testSuite.Status.NextRun, err
				}, timeout).Should(BeNil())
				Expect(testSuite.Annotations).NotTo(HaveKey(controllers.TestSuiteScheduleAnnotation))
				Expect(testSuite.Status.Conditions).To(ContainElement(And(
					HaveField("Type", controllers.TestSuiteHasScheduleCondition),
					HaveField("Status", metav1.ConditionFalse),
					HaveField("Reason", "ScheduleNotDefined"),
					HaveField("Message", "schedule is not defined"),
				)))
			})

			When("the schedule is not valid", func() {

				BeforeEach(func() {
					testSuite.Spec.When.Schedule = "not valid"
				})

				It("records the error", func() {
					Eventually(func() ([]metav1.Condition, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
						return testSuite.Status.Conditions, err
					}).Should(ContainElement(And(
						HaveField("Type", controllers.TestSuiteHasScheduleCondition),
						HaveField("Status", metav1.ConditionFalse),
						HaveField("Reason", "InvalidSchedule"),
						HaveField("Message", "schedule is not a valid cron value"),
					)))
					Expect(testSuite.Annotations[controllers.TestSuiteScheduleAnnotation]).To(Equal(testSuite.Spec.When.Schedule))
					Expect(testSuite.Status.NextRun).To(BeNil())
				})
			})

			When("the NextRun is reached", func() {

				var origClock clock.PassiveClock

				JustBeforeEach(func() {

					// Let the next run be set
					Eventually(func() (*metav1.Time, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
						return testSuite.Status.NextRun, err
					}, timeout).Should(Satisfy(func(t interface{}) bool {
						if t, ok := t.(*metav1.Time); ok {
							return t != nil && next.Equal(t.Time)
						} else {
							return false
						}
					}))

					// Time travel
					clk := tclock.NewFakePassiveClock(*next)
					origClock = setClock(clk)
					if s, err := cron.ParseStandard(testSuite.Spec.When.Schedule); err == nil {
						n := s.Next(clk.Now())
						next = &n
					} else {
						Expect(err).NotTo(HaveOccurred())
					}

					// Poke the TestSuite to trigger a reconciliation
					orig := testSuite.DeepCopy()
					patched := orig.DeepCopy()
					if patched.Annotations == nil {
						patched.Annotations = make(map[string]string)
					}
					patched.Annotations[konfirm.GroupName+"/touch"] = clk.Now().Format(time.RFC3339)
					Expect(k8sClient.Patch(ctx, patched, client.MergeFrom(orig)))
				})

				JustAfterEach(func() {
					// Revert to a real clock
					setClock(origClock)
				})

				It("Triggers a run", func() {
					Eventually(func() (konfirm.TestSuitePhase, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
						return testSuite.Status.Phase, err
					}, timeout).Should(Equal(konfirm.TestSuiteRunning))
				})
			})

			When("the NextRun is missed", func() {

				var origClock clock.PassiveClock

				BeforeEach(func() {
					// Schedule for 5 min from now
					now := time.Now()
					min, hour := now.Minute()+5, now.Hour()
					if min > 59 {
						min = min - 60
						hour++
					}
					if hour > 23 {
						hour = hour - 24
					}
					testSuite.Spec.When.Schedule = fmt.Sprintf("%d %d * * *", min, hour)
				})

				JustBeforeEach(func() {

					// Let the next run be set
					Eventually(func() (*metav1.Time, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
						return testSuite.Status.NextRun, err
					}, timeout).Should(Satisfy(func(t interface{}) bool {
						if t, ok := t.(*metav1.Time); ok {
							return t != nil && next.Equal(t.Time)
						} else {
							return false
						}
					}))

					// Time travel
					clk := tclock.NewFakePassiveClock(time.Now().Add(6 * time.Minute))
					origClock = setClock(clk)
					if s, err := cron.ParseStandard(testSuite.Spec.When.Schedule); err == nil {
						n := s.Next(clk.Now())
						next = &n
					} else {
						Expect(err).NotTo(HaveOccurred())
					}

					// Poke the TestSuite to trigger a reconciliation
					orig := testSuite.DeepCopy()
					patched := orig.DeepCopy()
					if patched.Annotations == nil {
						patched.Annotations = make(map[string]string)
					}
					patched.Annotations[konfirm.GroupName+"/touch"] = clk.Now().Format(time.RFC3339)
					Expect(k8sClient.Patch(ctx, patched, client.MergeFrom(orig)))
				})

				JustAfterEach(func() {
					// Revert to a real clock
					setClock(origClock)
				})

				It("Updates the NextRun", func() {
					Eventually(func() (*metav1.Time, error) {
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
						return testSuite.Status.NextRun, err
					}, timeout).Should(Satisfy(func(t interface{}) bool {
						if t, ok := t.(*metav1.Time); ok {
							return t != nil && next.Equal(t.Time)
						} else {
							return false
						}
					}))
					Expect(testSuite.Status.Conditions).To(ContainElement(And(
						HaveField("Type", controllers.TestSuiteHasScheduleCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "ScheduleSet"),
						HaveField("Message", "schedule is set"),
					)))
				})
			})
		})

		When("a Helm trigger is defined", func() {

			BeforeEach(func() {
				testSuite.Spec.When.HelmRelease = "a-release"
			})

			It("a label is added to track the Helm release", func() {
				Eventually(func() (labels map[string]string, err error) {
					err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
					labels = testSuite.Labels
					return
				}).Should(HaveKeyWithValue(controllers.TestSuiteHelmTriggerLabel, testSuite.Namespace+"."+testSuite.Spec.When.HelmRelease))
			})

			When("the Helm release exists", func() {

				var helmSecret *v1.Secret

				BeforeEach(func() {
					helmSecret = &v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: namespace,
							Name:      "sh.helm.release.v1." + testSuite.Spec.When.HelmRelease + ".v1",
							Labels: map[string]string{
								"modifiedAt": fmt.Sprintf("%d", time.Now().Unix()),
								"name":       testSuite.Spec.When.HelmRelease,
								"owner":      "Helm",
								"status":     "deployed",
								"version":    "1",
							},
						},
						Type: controllers.HelmSecretType,
					}
					Expect(k8sClient.Create(ctx, helmSecret)).NotTo(HaveOccurred())
				})

				It("it should progress to running", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
						g.Expect(testSuite.Status.Phase).To(Equal(konfirm.TestSuiteRunning))
						g.Expect(testSuite.Annotations[controllers.TestSuiteLastHelmReleaseAnnotation]).To(Equal(helmSecret.Labels["version"]))
						g.Expect(testSuite.Status.Conditions).To(HaveKey(And(
							HaveField("Type", controllers.TestSuiteRunStartedCondition),
							HaveField("Status", metav1.ConditionTrue),
							HaveField("Reason", "HelmRelease"),
							HaveField("Message", "TestSuite was manually triggered"),
						)))
					})
				})
			})
		})
	})

	Context("a test suite exists", func() {

		JustBeforeEach(func() {
			// Create a test suite and let it reach Ready
			Expect(k8sClient.Create(ctx, testSuite)).NotTo(HaveOccurred())
			Eventually(func() (konfirm.TestSuitePhase, error) {
				return testSuite.Status.Phase, k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
			}, timeout).Should(Equal(konfirm.TestSuiteReady))
		})

		When("and it is manually triggered", func() {

			JustBeforeEach(func() {
				orig := testSuite.DeepCopy()
				testSuite.Trigger = konfirm.TestSuiteTrigger{NeedsRun: true}
				Expect(k8sClient.Patch(ctx, testSuite, client.MergeFrom(orig))).NotTo(HaveOccurred())
			})

			It("it should progress to the Running phase", func() {
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
					g.Expect(testSuite.Status.Phase).To(Equal(konfirm.TestSuiteRunning))
					g.Expect(testSuite.Status.Conditions).To(HaveKey(And(
						HaveField("Type", controllers.TestSuiteRunStartedCondition),
						HaveField("Status", metav1.ConditionTrue),
						HaveField("Reason", "Manual"),
						HaveField("Message", "TestSuite was manually triggered"),
					)))
				})
			})
		})

		Context("with a Helm trigger", func() {

			var helmSecret *v1.Secret

			BeforeEach(func() {
				helmSecret = &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      "sh.helm.release.v1.my-chart.v1",
						Labels: map[string]string{
							"modifiedAt": fmt.Sprintf("%d", time.Now().Unix()),
							"name":       "my-chart",
							"owner":      "Helm",
							"status":     "deployed",
							"version":    "1",
						},
					},
					Type: controllers.HelmSecretType,
				}
				testSuite.Spec.When.HelmRelease = helmSecret.Labels["name"]
			})

			When("a Helm release occurs", func() {

				JustBeforeEach(func() {
					Eventually(func() (phase konfirm.TestSuitePhase, err error) {
						err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
						phase = testSuite.Status.Phase
						return
					}, timeout).Should(Equal(konfirm.TestSuiteReady))
					Expect(k8sClient.Create(ctx, helmSecret)).NotTo(HaveOccurred())
				})

				It("it should trigger a run", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
						g.Expect(testSuite.Status.Phase).To(Equal(konfirm.TestSuiteRunning))
						g.Expect(testSuite.Annotations[controllers.TestSuiteLastHelmReleaseAnnotation]).To(Equal(helmSecret.Labels["version"]))
						g.Expect(testSuite.Status.Conditions).To(HaveKey(And(
							HaveField("Type", controllers.TestSuiteRunStartedCondition),
							HaveField("Status", metav1.ConditionTrue),
							HaveField("Reason", "HelmRelease"),
							HaveField("Message", "TestSuite was manually triggered"),
						)))
					})
				})
			})

			Context("the Helm release is in a different namespace", func() {

				var helmReleaseNamespace *v1.Namespace

				BeforeEach(func() {
					helmReleaseNamespace = &v1.Namespace{}
					Expect(func() (err error) {
						helmReleaseNamespace.Name, err = generateNamespace()
						return
					}()).NotTo(HaveOccurred())
					Expect(k8sClient.Create(ctx, helmReleaseNamespace)).NotTo(HaveOccurred())
					helmSecret.Namespace = helmReleaseNamespace.Name
					testSuite.Spec.When.HelmRelease = helmReleaseNamespace.Name + "." + testSuite.Spec.When.HelmRelease
				})

				AfterEach(func() {
					Expect(k8sClient.Delete(ctx, helmSecret)).NotTo(HaveOccurred())
					Expect(k8sClient.Delete(ctx, helmReleaseNamespace)).NotTo(HaveOccurred())
				})

				When("a Helm release occurs", func() {

					JustBeforeEach(func() {
						Eventually(func() (phase konfirm.TestSuitePhase, err error) {
							err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
							phase = testSuite.Status.Phase
							return
						}, timeout).Should(Equal(konfirm.TestSuiteReady))
						Expect(k8sClient.Create(ctx, helmSecret)).NotTo(HaveOccurred())
					})

					It("it should NOT trigger a run", func() {
						Consistently(func(g Gomega) {
							g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
							g.Expect(testSuite.Status.Phase).NotTo(Equal(konfirm.TestSuiteRunning))
							g.Expect(testSuite.Annotations[controllers.TestSuiteLastHelmReleaseAnnotation]).NotTo(Equal(helmSecret.Labels["version"]))
							g.Expect(testSuite.Status.Conditions).NotTo(HaveKey(And(
								HaveField("Type", controllers.TestSuiteRunStartedCondition),
								HaveField("Status", metav1.ConditionTrue),
								HaveField("Reason", "HelmRelease"),
								HaveField("Message", "TestSuite was manually triggered"),
							)))
						})
					})
				})

				Context("a HelmPolicy exists", func() {

					var helmPolicy *konfirm.HelmPolicy

					BeforeEach(func() {
						helmPolicy = &konfirm.HelmPolicy{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: helmReleaseNamespace.Name,
								Name:      helmSecret.Labels["name"],
							},
							Spec: konfirm.HelmPolicySpec{
								ExportTo: []string{"some-namespace"},
							},
						}
					})

					JustBeforeEach(func() {
						Expect(k8sClient.Create(ctx, helmPolicy)).NotTo(HaveOccurred())
					})

					AfterEach(func() {
						Expect(k8sClient.Delete(ctx, helmPolicy)).NotTo(HaveOccurred())
					})

					When("a Helm release occurs", func() {

						JustBeforeEach(func() {
							Eventually(func() (phase konfirm.TestSuitePhase, err error) {
								err = k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)
								phase = testSuite.Status.Phase
								return
							}, timeout).Should(Equal(konfirm.TestSuiteReady))
							Expect(k8sClient.Create(ctx, helmSecret)).NotTo(HaveOccurred())
						})

						It("it should NOT trigger a run", func() {
							Consistently(func(g Gomega) {
								g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
								g.Expect(testSuite.Status.Phase).NotTo(Equal(konfirm.TestSuiteRunning))
								g.Expect(testSuite.Annotations[controllers.TestSuiteLastHelmReleaseAnnotation]).NotTo(Equal(helmSecret.Labels["version"]))
								g.Expect(testSuite.Status.Conditions).NotTo(HaveKey(And(
									HaveField("Type", controllers.TestSuiteRunStartedCondition),
									HaveField("Status", metav1.ConditionTrue),
									HaveField("Reason", "HelmRelease"),
									HaveField("Message", "TestSuite was manually triggered"),
								)))
							})
						})

						When("the HelmPolicy exports to all namespaces", func() {

							BeforeEach(func() {
								helmPolicy.Spec.ExportTo = []string{"*"}
							})

							It("it should trigger a run", func() {
								Eventually(func(g Gomega) {
									g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
									g.Expect(testSuite.Status.Phase).To(Equal(konfirm.TestSuiteRunning))
									g.Expect(testSuite.Annotations[controllers.TestSuiteLastHelmReleaseAnnotation]).To(Equal(helmSecret.Labels["version"]))
									g.Expect(testSuite.Status.Conditions).To(HaveKey(And(
										HaveField("Type", controllers.TestSuiteRunStartedCondition),
										HaveField("Status", metav1.ConditionTrue),
										HaveField("Reason", "HelmRelease"),
										HaveField("Message", "TestSuite was manually triggered"),
									)))
								})
							})
						})

						When("the HelmPolicy exports to TestSuite's namespace", func() {

							BeforeEach(func() {
								helmPolicy.Spec.ExportTo = append(helmPolicy.Spec.ExportTo, testSuite.Namespace)
							})

							It("it should trigger a run", func() {
								Eventually(func(g Gomega) {
									g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(testSuite), testSuite)).NotTo(HaveOccurred())
									g.Expect(testSuite.Status.Phase).To(Equal(konfirm.TestSuiteRunning))
									g.Expect(testSuite.Annotations[controllers.TestSuiteLastHelmReleaseAnnotation]).To(Equal(helmSecret.Labels["version"]))
									g.Expect(testSuite.Status.Conditions).To(HaveKey(And(
										HaveField("Type", controllers.TestSuiteRunStartedCondition),
										HaveField("Status", metav1.ConditionTrue),
										HaveField("Reason", "HelmRelease"),
										HaveField("Message", "TestSuite was manually triggered"),
									)))
								})
							})
						})
					})
				})
			})
		})

		Context("historical test runs exist", func() {

			var historicalRuns []konfirm.TestRun

			BeforeEach(func() {
				testSuite.Spec.HistoryLimit = 6
				testRun := konfirm.TestRun{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: testSuite.Name + "-",
						Namespace:    testSuite.Namespace,
						Annotations: map[string]string{
							konfirm.GroupName + "/template": "abcdefg",
						},
						Finalizers: []string{controllers.TestSuiteControllerFinalizer},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         testSuite.APIVersion,
								Kind:               testSuite.Kind,
								Name:               testSuite.Name,
								UID:                testSuite.UID,
								Controller:         &yes,
								BlockOwnerDeletion: &yes,
							},
						},
					},
					Spec: konfirm.TestRunSpec{
						RetentionPolicy: testSuite.Spec.Template.RetentionPolicy,
						Tests:           testSuite.Spec.Template.Tests,
					},
				}

				var results []konfirm.TestResult
				for i := range testRun.Spec.Tests {
					results = append(results, konfirm.TestResult{
						Description: testRun.Spec.Tests[i].Description,
						Passed:      true,
					})
				}

				for i := 0; i < 6; i++ {
					historicalRuns = append(historicalRuns, testRun)
				}
			})

			JustBeforeEach(func() {
				for i, j := 0, len(historicalRuns); i < j; i++ {

					// Create a test run
					testRun := &historicalRuns[i]
					testRun.OwnerReferences[0].UID = testSuite.UID
					Expect(k8sClient.Create(ctx, testRun)).NotTo(HaveOccurred())

					// Get the associated tests
					var tests []konfirm.Test
					Eventually(func() ([]konfirm.Test, error) {
						var err error
						tests, err = getTests(ctx, testRun)
						return tests, err
					}, timeout).ShouldNot(BeEmpty())

					// Get all the tests
					for i := range tests {

						test := &tests[i]

						// Get the associated pod
						var pod *v1.Pod
						Eventually(func() (*v1.Pod, error) {
							pods, err := getPods(ctx, test)
							if len(pods) > 0 {
								pod = &pods[0]
							}
							return pod, err
						}, timeout).ShouldNot(BeNil())

						// Pass the pod
						orig := pod.DeepCopy()
						pod.Status.Phase = v1.PodSucceeded
						pod.Status.Message = fmt.Sprintf("test-%s passed", test.Name)
						Expect(k8sClient.Status().Patch(ctx, pod, client.MergeFrom(orig))).NotTo(HaveOccurred())
					}

					// Make sure the test run passes
					Eventually(func() (konfirm.TestRunPhase, error) {
						if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(testRun), testRun); err != nil {
							return "", err
						}
						return testRun.Status.Phase, nil
					}, timeout).Should(Equal(konfirm.TestRunPassed))
				}
			})

			When("historical runs are deleted", func() {

				JustBeforeEach(func() {
					Expect(k8sClient.Delete(ctx, &historicalRuns[1])).NotTo(HaveOccurred())
					Expect(k8sClient.Delete(ctx, &historicalRuns[4])).NotTo(HaveOccurred())
					Expect(k8sClient.Delete(ctx, &historicalRuns[5])).NotTo(HaveOccurred())
				})

				It("removes the finalizer", func() {
					deleted := []int{1, 4, 5}
					for i := range deleted {
						n := deleted[i]
						Eventually(func() error {
							return k8sClient.Get(ctx, client.ObjectKeyFromObject(&historicalRuns[n]), &konfirm.TestRun{})
						}, timeout).Should(Satisfy(apierrors.IsNotFound), fmt.Sprintf("test run %d was not deleted", n))
					}
				})
			})
		})
	})
})
