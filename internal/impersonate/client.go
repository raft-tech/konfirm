package impersonate

import (
	"context"
	"errors"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	DefaultUserRef = types.NamespacedName{
		Namespace: "konfirm-system",
		Name:      "default",
	}
)

type Client interface {
	client.Client
	Impersonate(ctx context.Context, namespace string, name string) (Client, error)
}

// NewImpersonatingClient creates a caching impersonatingClient with support for impersonation
func NewImpersonatingClient(config *rest.Config, options client.Options) (client.Client, error) {
	if c, err := client.New(config, options); err == nil {
		return &impersonatingClient{
			Client:  c,
			config:  config,
			options: options,
		}, nil
	} else {
		return nil, err
	}
}

type impersonatingClient struct {
	client.Client
	config  *rest.Config
	options client.Options
}

// Impersonate returns a client.Client configured to impersonate the specified api.UserRef.
// namespace is required. If name is empty, the default UserRef name will be used
// (generally "default"). If name is empty and the default UserRef is not found in the
// specified namespace, the default UserRef will be used (generally, "konfirm-system/default".)
func (ic *impersonatingClient) Impersonate(ctx context.Context, namespace string, name string) (Client, error) {

	// Default to "default" userRefName if not explicitly set
	userRefName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if userRefName.Namespace == "" {
		return nil, errors.New("namespace must not be empty")
	}
	if userRefName.Name == "" {
		userRefName.Name = DefaultUserRef.Name
	}

	// Find the UserRef
	var userRef konfirm.UserRef
	if err := ic.Get(ctx, userRefName, &userRef); err != nil {
		if apierrors.IsNotFound(err) && name == "" {
			// Name was explicitly omitted, so the system default can be used
			if err = ic.Get(ctx, DefaultUserRef, &userRef); err != nil {
				return nil, errors.New("default UserRef not found")
			}
		} else {
			return nil, errors.Join(errors.New("error retrieving specified UserRef"), err)
		}
	}

	// Copy the rest.Config and add impersonation
	config := rest.CopyConfig(ic.config)
	config.Impersonate = rest.ImpersonationConfig{
		UserName: userRef.Spec.User,
		UID:      userRef.Spec.UID,
		Groups:   userRef.Spec.Groups,
		Extra:    userRef.Spec.Extra,
	}

	if _client, err := client.New(config, ic.options); err == nil {
		return &impersonatingClient{
			Client:  _client,
			config:  config,
			options: ic.options,
		}, nil
	} else {
		return nil, err
	}
}
