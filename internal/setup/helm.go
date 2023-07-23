package setup

import (
	"context"
	"fmt"
	"github.com/raft-tech/konfirm/logging"
	helm "helm.sh/helm/v3/pkg/action"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type HelmInstallInput struct {
	types.NamespacedName
	ChartUrl string
	Username string
	Password string
	Values   map[string]interface{}
}

type HelmRelease struct{}

type HelmClient interface {
	Install(ctx context.Context, config HelmInstallInput) (*HelmRelease, error)
	Uninstall(ctx context.Context, release *HelmRelease) error
}

func NewHelmClient(namespace string, cfg *rest.Config, mapper meta.RESTMapper, logger logging.Logger) (HelmClient, error) {
	debugLogger := logger.DebugLogger().WithName("helm").WithCallDepth(1)
	debugFunc := func(format string, v ...interface{}) {
		if debugLogger.Enabled() {
			debugLogger.Info(fmt.Sprintf(format, v...))
		}
	}
	config := &helm.Configuration{}
	if err := config.Init(&restClientGetter{config: cfg, restMapper: mapper}, namespace, "", debugFunc); err != nil {
		return nil, err
	}

	return &helmClient{config: config}, nil
}

type helmClient struct {
	config *helm.Configuration
}

func (h helmClient) Install(ctx context.Context, config HelmInstallInput) (*HelmRelease, error) {
	//TODO implement me
	panic("implement me")
}

func (h helmClient) Uninstall(ctx context.Context, release *HelmRelease) error {
	//TODO implement me
	panic("implement me")
}

type restClientGetter struct {
	config     *rest.Config
	restMapper meta.RESTMapper
}

func (r restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.config, nil
}

func (r restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	if dc, err := discovery.NewDiscoveryClientForConfig(r.config); err == nil {
		return memory.NewMemCacheClient(dc), nil
	} else {
		return nil, err
	}
}

func (r restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return r.restMapper, nil
}

func (r restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {

	// This is based on the behavior of genericclioptions.ConfigFlags.ToRawKubeConfigLoader

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}

	if v := r.config.CertFile; v != "" {
		overrides.AuthInfo.ClientCertificate = v
	}

	if v := r.config.KeyFile; v != "" {
		overrides.AuthInfo.ClientKey = v
	}

	if v := r.config.BearerTokenFile; v != "" {
		overrides.AuthInfo.TokenFile = v
	} else if v = r.config.BearerToken; v != "" {
		overrides.AuthInfo.Token = v
	}

	if v := r.config.Impersonate.UserName; v != "" {
		overrides.AuthInfo.Impersonate = v
	}

	if v := r.config.Impersonate.UID; v != "" {
		overrides.AuthInfo.ImpersonateUID = v
	}

	if v := r.config.Impersonate.Groups; v != nil {
		overrides.AuthInfo.ImpersonateGroups = make([]string, len(v))
		copy(overrides.AuthInfo.ImpersonateGroups, v)
	}

	if v := r.config.Username; v != "" {
		overrides.AuthInfo.Username = v
	}

	if v := r.config.Password; v != "" {
		overrides.AuthInfo.Password = v
	}

	if v := r.config.Host; v != "" {
		overrides.ClusterInfo.Server = v
		if v = r.config.APIPath; v != "" {
			overrides.ClusterInfo.Server += v
		}
	}

	if v := r.config.ServerName; v != "" {
		overrides.ClusterInfo.TLSServerName = v
	}

	if v := r.config.CAFile; v != "" {
		overrides.ClusterInfo.CertificateAuthority = v
	}

	overrides.ClusterInfo.InsecureSkipTLSVerify = r.config.Insecure
	overrides.Timeout = r.config.Timeout.String()

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}
