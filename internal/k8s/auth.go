/*
Copyright 2026 Serge Logvinov.

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

package k8s

import (
	"fmt"
	"os"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// newClientForCluster builds a Client for the named kubeconfig context.
//
// It never mutates the shared global ConfigFlags: each client gets its own
// copy with the context pinned, so concurrently built clients and downstream
// consumers (e.g. helm, via ToRawKubeConfigLoader) resolve independently and
// always target the context they were built for.
func (mc *MultiClusterClient) newClientForCluster(contextName string) (*Client, error) {
	configFlags := &genericclioptions.ConfigFlags{
		KubeConfig: mc.cfg.ConfigFlags.KubeConfig,
		Namespace:  mc.cfg.ConfigFlags.Namespace,
		Context:    &contextName,
		// Global overrides that apply to every context.
		Impersonate:      mc.cfg.ConfigFlags.Impersonate,
		ImpersonateGroup: mc.cfg.ConfigFlags.ImpersonateGroup,
	}

	kubeCtx, ok := mc.raw.Contexts[contextName]
	if !ok || kubeCtx == nil {
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}

	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get REST config for context %q: %w", contextName, err)
	}

	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	namespace := kubeCtx.Namespace
	if configFlags.Namespace != nil && *configFlags.Namespace != "" {
		namespace = *configFlags.Namespace
	}
	if namespace == "" {
		namespace = "default"
	}

	return &Client{
		configFlags: configFlags,
		Interface:   clientSet,
		ContextName: contextName,
		ClusterName: kubeCtx.Cluster,
		Namespace:   namespace,
		User:        contextAuthInfo(restConfig),
		sanitizer:   mc.sanitizer,
	}, nil
}

// newInClusterClient builds a Client from an in-cluster rest.Config (as
// returned by rest.InClusterConfig). Identity fields reflect the pod's
// service account.
func (mc *MultiClusterClient) newInClusterClient(restConfig *rest.Config) (*Client, error) {
	clientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	namespace := "default"
	if ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil && len(ns) > 0 {
		namespace = string(ns)
	}

	// No kubeconfig context to pin: with no kubeconfig present, clientcmd
	// falls back to the in-cluster configuration.
	configFlags := &genericclioptions.ConfigFlags{
		KubeConfig: mc.cfg.ConfigFlags.KubeConfig,
	}

	return &Client{
		configFlags: configFlags,
		Interface:   clientSet,
		ClusterName: "in-cluster",
		Namespace:   namespace,
		User:        contextAuthInfo(restConfig),
		sanitizer:   mc.sanitizer,
	}, nil
}

// contextAuthInfo extracts identity information from a raw kubeconfig context.
func contextAuthInfo(restConfig *rest.Config) UserInfo {
	return UserInfo{
		Name:              restConfig.Username,
		Username:          restConfig.UserAgent,
		HasToken:          restConfig.BearerToken != "",
		Impersonate:       restConfig.Impersonate.UserName,
		ImpersonateGroups: restConfig.Impersonate.Groups,
	}
}
