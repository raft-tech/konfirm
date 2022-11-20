package impersonate

import (
	"context"
	konfirm "github.com/raft-tech/konfirm/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
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
	Impersonate(ctx context.Context, namespace string, name string) (client.Client, error)
}

// NewImpersonatingClient creates a caching impersonatingClient with support for impersonation
func NewImpersonatingClient(cache cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {

	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	if dc, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader:     cache,
		Client:          c,
		UncachedObjects: uncachedObjects,
	}); err == nil {
		return &impersonatingClient{
			Client:          dc,
			cache:           cache,
			config:          config,
			options:         options,
			uncachedObjects: uncachedObjects,
		}, nil
	} else {
		return nil, err
	}
}

type impersonatingClient struct {
	client.Client
	cache           cache.Cache
	config          *rest.Config
	options         client.Options
	uncachedObjects []client.Object
}

func (ic impersonatingClient) Impersonate(ctx context.Context, namespace string, name string) (client.Client, error) {

	// Default to "default" userRefName if not explicitly set
	userRefName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if userRefName.Name == "" {
		userRefName.Name = DefaultUserRef.Name
	}

	// Find the UserRef
	var userRef konfirm.UserRef
	if err := ic.Get(ctx, userRefName, &userRef); err != nil {
		if apierrors.IsNotFound(err) && name == "" {
			if err = ic.Get(ctx, DefaultUserRef, &userRef); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// Copy the rest.Config and add impersonation
	config := *ic.config // Dereference original config to avoid reconfiguring other clients
	if user := userRef.Spec.UserName; user != "" {
		config.Impersonate.UserName = user
	}

	c, err := client.New(&config, ic.options)
	if err != nil {
		return nil, err
	}

	// Create a new impersonatingClient but maintain a reference to the original config
	if dc, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader:     ic.cache,
		Client:          c,
		UncachedObjects: ic.uncachedObjects,
	}); err == nil {
		return &impersonatingClient{
			Client:          dc,
			cache:           ic.cache,
			config:          ic.config,
			options:         ic.options,
			uncachedObjects: ic.uncachedObjects,
		}, nil
	} else {
		return nil, err
	}
}
