/*
Copyright 2025 The Kubernetes Authors.

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

// Package k8s implements the Kubernetes client for the mimiops-mcp server, including context resolution and impersonation.
package k8s

import (
	"fmt"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/utils"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is a Kubernetes clientset plus the resolved identity of the active
// context/cluster/namespace, so callers can report *what* they are talking to.
type Client struct {
	kubernetes.Interface

	configFlags *genericclioptions.ConfigFlags
	sanitizer   *utils.Sanitizer

	// ContextName is the resolved active context (from --context or current-context).
	ContextName string
	// ClusterName is the cluster the active context points to.
	ClusterName string
	// Namespace is the effective namespace: --namespace > kubeconfig context namespace > "default".
	Namespace string
	// User is the resolved AuthInfo (user) of the active context, including impersonation.
	User UserInfo
}

// UserInfo describes the authenticated identity used by the active context.
type UserInfo struct {
	// Name is the kubeconfig user name the active context references.
	Name string
	// Username is the basic-auth username, if any.
	Username string
	// Impersonate is the user to impersonate (kubeconfig AuthInfo.Impersonate), if any.
	Impersonate string
	// ImpersonateGroups are the groups to impersonate, if any.
	ImpersonateGroups []string
	// HasToken indicates whether the context authenticates via a token.
	HasToken bool
}

// NewClient builds a typed Kubernetes client from the supplied config, resolving
// the active context, cluster, and namespace via k8s.io/cli-runtime/pkg/genericclioptions.
func NewClient(cfg *config.Config) (*Client, error) {
	clientConfig := cfg.ConfigFlags.ToRawKubeConfigLoader()

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config: %w", err)
	}

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	resolved, err := resolveContext(cfg.ConfigFlags)
	if err != nil {
		return nil, err
	}

	sanitizer, err := utils.NewDefaultSanitizer()
	if err != nil {
		return nil, fmt.Errorf("failed to create log sanitizer: %v", err)
	}

	return &Client{
		configFlags: cfg.ConfigFlags,
		Interface:   clientSet,
		ContextName: resolved.context,
		ClusterName: resolved.cluster,
		Namespace:   resolved.namespace,
		User:        resolved.user,
		sanitizer:   sanitizer,
	}, nil
}

// ToRawKubeConfigLoader returns the underlying ConfigFlags for k8s client creation.
func (c *Client) ToRawKubeConfigLoader() *genericclioptions.ConfigFlags {
	return c.configFlags
}

// ToRESTConfig returns the REST config for the active context, cluster, and namespace.
func (c *Client) ToRESTConfig() (*rest.Config, error) {
	return c.configFlags.ToRESTConfig()
}

// Sanitizer returns the log sanitizer for masking sensitive values in logs.
func (c *Client) Sanitizer() *utils.Sanitizer {
	return c.sanitizer
}

type resolvedIdentity struct {
	context   string
	cluster   string
	namespace string
	user      UserInfo
}

// resolveContext loads the merged kubeconfig and extracts the active context's
// cluster and default namespace, honoring overrides from flags.
func resolveContext(configFlags *genericclioptions.ConfigFlags) (*resolvedIdentity, error) {
	// Load the raw config using clientcmd for detailed identity resolution
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if configFlags.KubeConfig != nil {
		loadingRules.ExplicitPath = *configFlags.KubeConfig
	}
	raw, err := loadingRules.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Determine the active context
	contextName := ""
	if configFlags.Context != nil {
		contextName = *configFlags.Context
	}
	if contextName == "" {
		contextName = raw.CurrentContext
	}

	// Determine the namespace
	namespace := ""
	if configFlags.Namespace != nil {
		namespace = *configFlags.Namespace
	}

	cluster := ""
	user := UserInfo{}

	if ctx, ok := raw.Contexts[contextName]; ok {
		cluster = ctx.Cluster
		if namespace == "" {
			namespace = ctx.Namespace
		}

		if auth, ok := raw.AuthInfos[ctx.AuthInfo]; ok {
			user = UserInfo{
				Name:              ctx.AuthInfo,
				Username:          auth.Username,
				Impersonate:       auth.Impersonate,
				ImpersonateGroups: auth.ImpersonateGroups,
				HasToken:          auth.Token != "" || auth.TokenFile != "",
			}
		}
	}

	if namespace == "" {
		namespace = "default"
	}

	// Resolve the effective impersonation: flag override wins, else the
	// kubeconfig AuthInfo.Impersonate.
	impersonate := ""
	if configFlags.Impersonate != nil {
		impersonate = *configFlags.Impersonate
	}
	if impersonate == "" {
		impersonate = user.Impersonate
	}
	user.Impersonate = impersonate

	return &resolvedIdentity{
		context:   contextName,
		cluster:   cluster,
		namespace: namespace,
		user:      user,
	}, nil
}
