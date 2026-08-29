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
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/sergelogvinov/mimiops-mcp/internal/config"
	"github.com/sergelogvinov/mimiops-mcp/internal/oidc"
	"github.com/sergelogvinov/mimiops-mcp/internal/utils"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
)

// ClusterListEntry describes one cluster known to the kubeconfig.
type ClusterListEntry struct {
	// Name is the cluster name as it appears in the kubeconfig.
	Name string `json:"name"`
	// Server is the API server endpoint of the cluster.
	Server string `json:"server"`
	// Contexts lists the context names that reference this cluster.
	Contexts []string `json:"contexts"`
	// ContextCount is the number of contexts referencing this cluster.
	ContextCount int `json:"context_count"`
	// IsCurrent reports whether this is the active cluster of the current context.
	IsCurrent bool `json:"is_current"`
}

// MultiClusterClient owns the kubeconfig and builds per-cluster clients on
// demand. In in-cluster mode it wraps the single active client.
type MultiClusterClient struct {
	cfg *config.Config
	raw *api.Config // raw kubeconfig, nil in in-cluster mode

	mu        sync.Mutex
	inCluster *Client            // inCluster client configuration, nil in multi-cluster mode
	clients   map[string]*Client // per-cluster clients, built lazily
	contexts  map[string]string  // cluster name -> representative context name

	sanitizer *utils.Sanitizer
}

// NewMultiClusterClient builds the multi-cluster configuration from the kubeconfig.
// When no kubeconfig is available it falls back to the in-cluster configuration
// and returns a MultiClusterClient with a single inCluster client.
func NewMultiClusterClient(cfg *config.Config) (*MultiClusterClient, error) {
	sanitizer, err := utils.NewDefaultSanitizer()
	if err != nil {
		return nil, fmt.Errorf("failed to create log sanitizer: %v", err)
	}

	mc := &MultiClusterClient{
		cfg:       cfg,
		clients:   map[string]*Client{},
		sanitizer: sanitizer,
	}

	clientConfig := cfg.ConfigFlags.ToRawKubeConfigLoader()
	raw, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw kubeconfig: %w", err)
	}

	if len(raw.Clusters) > 0 {
		mc.raw = &raw
		mc.contexts = representativeContexts(mc.raw)

		return mc, nil
	}

	// No kubeconfig; fall back to the in-cluster configuration.
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := mc.newInClusterClient(restConfig)
	if err != nil {
		return nil, err
	}

	mc.inCluster = client

	return mc, nil
}

// IsMultiCluster reports whether the server was started with a kubeconfig
// (multi-cluster mode) as opposed to running in-cluster.
func (mc *MultiClusterClient) IsMultiCluster() bool {
	return mc.raw != nil
}

// ListClusters enumerates the clusters referenced by the kubeconfig. It
// returns nil in in-cluster mode.
func (mc *MultiClusterClient) ListClusters() []ClusterListEntry {
	if mc.raw == nil {
		return nil
	}

	contextsByCluster := map[string][]string{}
	for ctxName, kubeCtx := range mc.raw.Contexts {
		if kubeCtx == nil || kubeCtx.Cluster == "" {
			continue
		}
		contextsByCluster[kubeCtx.Cluster] = append(contextsByCluster[kubeCtx.Cluster], ctxName)
	}

	clusterNames := make([]string, 0, len(mc.raw.Clusters))
	for clusterName := range mc.raw.Clusters {
		clusterNames = append(clusterNames, clusterName)
	}
	sort.Strings(clusterNames)

	entries := make([]ClusterListEntry, 0, len(clusterNames))
	for _, clusterName := range clusterNames {
		contexts := contextsByCluster[clusterName]
		sort.Strings(contexts)

		server := ""
		if cluster := mc.raw.Clusters[clusterName]; cluster != nil {
			server = cluster.Server
		}

		entries = append(entries, ClusterListEntry{
			Name:         clusterName,
			Server:       server,
			Contexts:     contexts,
			ContextCount: len(contexts),
			IsCurrent:    slices.Contains(contexts, mc.raw.CurrentContext),
		})
	}

	return entries
}

// GetCluster returns the client for the named cluster. An empty name selects
// the active cluster: the inCluster client in in-cluster mode, or the client
// of the current context's cluster in multi-cluster mode.
func (mc *MultiClusterClient) GetCluster(clusterName string) (*Client, error) {
	if clusterName == "" && mc.inCluster != nil {
		return mc.inCluster, nil
	}

	if mc.raw == nil {
		return nil, fmt.Errorf("not running in multi-cluster mode: the server uses the in-cluster configuration")
	}

	if clusterName == "" && mc.raw.CurrentContext != "" {
		if c, ok := mc.raw.Contexts[mc.raw.CurrentContext]; ok && c != nil && c.Cluster != "" {
			clusterName = c.Cluster
		}
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	if client, ok := mc.clients[clusterName]; ok {
		return client, nil
	}

	c, ok := mc.contexts[clusterName]
	if !ok {
		return nil, fmt.Errorf("unknown cluster %q; see clusters_list", clusterName)
	}

	client, err := mc.newClientForCluster(c)
	if err != nil {
		return nil, err
	}

	mc.clients[clusterName] = client

	return client, nil
}

// GetClusterForRequest resolves the client for a tool call. When the request
// context carries a verified OIDC token (see internal/oidc), a fresh
// per-request client is built with that token forwarded as the bearer
// credential; per-request clients are not cached because every caller has a
// distinct token. Without OIDC auth in the context it delegates to
// GetCluster (the cached, identity-less path).
func (mc *MultiClusterClient) GetClusterForRequest(ctx context.Context, clusterName string) (*Client, error) {
	auth, ok := oidc.FromContext(ctx)
	if !ok || auth.Token == "" {
		return mc.GetCluster(clusterName)
	}

	if mc.inCluster != nil {
		if clusterName != "" {
			return nil, fmt.Errorf("not running in multi-cluster mode: the server uses the in-cluster configuration")
		}

		return mc.newInClusterClientWithToken(mc.inCluster.restConfig, auth)
	}

	if mc.raw == nil {
		return nil, fmt.Errorf("not running in multi-cluster mode: the server uses the in-cluster configuration")
	}

	if clusterName == "" && mc.raw.CurrentContext != "" {
		if c, ok := mc.raw.Contexts[mc.raw.CurrentContext]; ok && c != nil && c.Cluster != "" {
			clusterName = c.Cluster
		}
	}

	// mc.contexts is immutable after construction, safe to read unlocked.
	contextName, ok := mc.contexts[clusterName]
	if !ok {
		return nil, fmt.Errorf("unknown cluster %q; see clusters_list", clusterName)
	}

	return mc.newClientForClusterWithToken(contextName, auth)
}

func representativeContexts(raw *api.Config) map[string]string {
	contextNames := make([]string, 0, len(raw.Contexts))
	for ctxName := range raw.Contexts {
		contextNames = append(contextNames, ctxName)
	}
	sort.Strings(contextNames)

	activeCluster := ""
	if ctx, ok := raw.Contexts[raw.CurrentContext]; ok && ctx != nil {
		activeCluster = ctx.Cluster
	}

	contexts := map[string]string{}
	for _, ctxName := range contextNames {
		kubeCtx := raw.Contexts[ctxName]
		if kubeCtx == nil || kubeCtx.Cluster == "" {
			continue
		}
		if _, ok := contexts[kubeCtx.Cluster]; !ok || kubeCtx.Cluster == activeCluster {
			contexts[kubeCtx.Cluster] = ctxName
		}
	}

	return contexts
}
