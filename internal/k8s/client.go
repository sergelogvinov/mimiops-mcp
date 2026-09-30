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

// Package k8s implements the Kubernetes client for the mimiops-mcp server, including context resolution and impersonation.
package k8s

import (
	"sync"
	"time"

	"github.com/sergelogvinov/mimiops-mcp/internal/utils"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
	"k8s.io/metrics/pkg/client/custom_metrics"
)

// customMetricsTimeout bounds every custom.metrics.k8s.io request. The
// custom-metrics client accepts no context, so the timeout is the only way
// to keep a slow or hung adapter from blocking the caller.
const customMetricsTimeout = 10 * time.Second

// Client is a Kubernetes clientset plus the resolved identity of the active
// context/cluster/namespace, so callers can report *what* they are talking to.
type Client struct {
	kubernetes.Interface

	configFlags *genericclioptions.ConfigFlags
	restConfig  *rest.Config
	sanitizer   *utils.Sanitizer

	metricsOnce sync.Once
	metrics     metricsclientset.Interface
	metricsErr  error

	customMetricsOnce sync.Once
	customMetrics     custom_metrics.CustomMetricsClient
	customMetricsErr  error

	usageWindow string

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

// ToRawKubeConfigLoader returns the underlying ConfigFlags for k8s client creation.
func (c *Client) ToRawKubeConfigLoader() *genericclioptions.ConfigFlags {
	return c.configFlags
}

// RESTConfig returns the underlying rest.Config for k8s client creation.
func (c *Client) RESTConfig() *rest.Config {
	return c.restConfig
}

// Metrics returns the metrics.k8s.io clientset for the cluster, created lazily.
// The metrics API is optional (it requires metrics-server); callers must handle
// request-time errors such as NotFound when it is not served.
func (c *Client) Metrics() (metricsclientset.Interface, error) {
	c.metricsOnce.Do(func() {
		client, err := metricsclientset.NewForConfig(c.restConfig)
		if err != nil {
			c.metricsErr = err
			return
		}

		c.metrics = client
	})

	return c.metrics, c.metricsErr
}

// CustomMetrics returns the custom.metrics.k8s.io client for the cluster, created lazily.
// The API version is negotiated through discovery on first use, so callers must
// handle request-time errors when no custom-metrics adapter is registered.
func (c *Client) CustomMetrics() (custom_metrics.CustomMetricsClient, error) {
	c.customMetricsOnce.Do(func() {
		config := rest.CopyConfig(c.restConfig)
		config.Timeout = customMetricsTimeout

		discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
		if err != nil {
			c.customMetricsErr = err
			return
		}

		mapper := restmapper.NewDeferredDiscoveryRESTMapper(cacheddiscovery.NewMemCacheClient(discoveryClient))
		availableAPIs := custom_metrics.NewAvailableAPIsGetter(discoveryClient)

		c.customMetrics = custom_metrics.NewForConfig(config, mapper, availableAPIs)
	})

	return c.customMetrics, c.customMetricsErr
}

// UsageWindow returns the averaging window of the custom.metrics.k8s.io usage
// metrics. Empty means custom metrics are disabled.
func (c *Client) UsageWindow() string {
	return c.usageWindow
}

// Sanitizer returns the log sanitizer for masking sensitive values in logs.
func (c *Client) Sanitizer() *utils.Sanitizer {
	return c.sanitizer
}
