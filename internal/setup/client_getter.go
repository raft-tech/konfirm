/*
 Copyright 2023 Raft, LLC

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

package setup

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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
