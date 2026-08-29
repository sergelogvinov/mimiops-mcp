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

package tools

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClustersDescribeResult represents the result of describing a cluster.
type ClustersDescribeResult struct {
	Name          string   `json:"name" jsonschema:"Name of the cluster"`
	APIVersions   []string `json:"api_versions" jsonschema:"API versions served by the cluster (group/version)"`
	Namespaces    int      `json:"namespaces" jsonschema:"Number of namespaces in the cluster"`
	NodeStatus    string   `json:"node_status" jsonschema:"Node counts in Ready/NotReady/Unknown"`
	NodeResources string   `json:"node_resources" jsonschema:"Total resources in the cluster (CPU, Memory, etc.)"`
	Regions       []string `json:"regions" jsonschema:"Regions where the cluster nodes run"`
	Zones         []string `json:"zones" jsonschema:"Zones where the cluster nodes run"`
}

// RegisterClustersDescribe adds the clusters_describe tool, which returns the
// API versions served by a cluster. In in-cluster mode the cluster parameter
// is hidden and the tool always describes the current cluster.
func RegisterClustersDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Cluster"),
		mcp.WithDescription("Return the API versions served by a cluster"),
		mcp.WithOutputSchema[ClustersDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("clusters_describe", opts...)
	s.AddTool(tool, handlerClustersDescribe(mc))
}

// handlerClustersDescribe returns a handler function for the clusters_describe tool.
func handlerClustersDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log := logger.FromContext(ctx)

		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		log.DebugContext(ctx, "clusters_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
		)

		apiVersions, err := apiVersionsForCluster(ctx, client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to fetch API versions for cluster %q: %v", client.ClusterName, err), nil
		}

		namespaceCount, nodeStatus, nodeResources, zones, regions, err := clusterStatus(ctx, client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to fetch cluster status for cluster %q: %v", client.ClusterName, err), nil
		}

		result := ClustersDescribeResult{
			Name:          client.ClusterName,
			APIVersions:   apiVersions,
			Namespaces:    namespaceCount,
			NodeStatus:    nodeStatus,
			NodeResources: nodeResources,
			Zones:         zones,
			Regions:       regions,
		}

		return mcp.NewToolResultStructured(result, formatter.ToText(result)), nil
	}
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=list;watch

// clusterStatus collects the namespace count, node status summary, total
// node resources, and the zones/regions where the cluster nodes run.
// NodeStatus is formatted as "Ready/NotReady/Unknown" counts, e.g. "4/0/0".
// NodeResources is the sum of node allocatable resources, rounded, e.g.
// "cpu=8 cores, memory=32GB, pods=110".
func clusterStatus(ctx context.Context, client *k8s.Client) (int, string, string, []string, []string, error) {
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, "", "", nil, nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, "", "", nil, nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var (
		ready, notReady, unknown int
		cpu, memory, pods        resource.Quantity
		zones, regions           []string
	)

	for i := range nodes.Items {
		node := &nodes.Items[i]

		switch nodeReadyStatus(node) {
		case "Ready":
			ready++
		case "NotReady":
			notReady++
		default:
			unknown++
		}

		if q := node.Status.Allocatable.Cpu(); q != nil {
			cpu.Add(*q)
		}
		if q := node.Status.Allocatable.Memory(); q != nil {
			memory.Add(*q)
		}
		if q := node.Status.Allocatable.Pods(); q != nil {
			pods.Add(*q)
		}

		if zone := node.Labels[corev1.LabelTopologyZone]; zone != "" && !slices.Contains(zones, zone) {
			zones = append(zones, zone)
		}
		if region := node.Labels[corev1.LabelTopologyRegion]; region != "" && !slices.Contains(regions, region) {
			regions = append(regions, region)
		}
	}

	sort.Strings(zones)
	sort.Strings(regions)

	nodeStatus := fmt.Sprintf("%d/%d/%d", ready, notReady, unknown)
	nodeResources := fmt.Sprintf("cpu=%d, memory=%dGiB, pods=%d",
		int(math.Round(float64(cpu.MilliValue())/1000)),
		int(math.Round(float64(memory.Value())/float64(int64(1<<30)))),
		pods.Value())

	return len(namespaces.Items), nodeStatus, nodeResources, zones, regions, nil
}

// nodeReadyStatus returns the node status based on the Ready condition only,
// ignoring the SchedulingDisabled suffix added by deriveNodeStatus.
func nodeReadyStatus(node *corev1.Node) string {
	for _, cond := range node.Status.Conditions {
		if cond.Type != corev1.NodeReady {
			continue
		}

		switch cond.Status {
		case corev1.ConditionTrue:
			return "Ready"
		case corev1.ConditionFalse:
			return "NotReady"
		case corev1.ConditionUnknown:
			return "Unknown"
		}

		break
	}

	return "Unknown"
}

// apiVersionsForCluster fetches the aggregated list of API versions
// (group/version, plus the legacy core "v1") served by a cluster.
func apiVersionsForCluster(_ context.Context, client *k8s.Client) ([]string, error) {
	groupList, err := client.Discovery().ServerGroups()
	if err != nil {
		return nil, fmt.Errorf("discovery failed: %w", err)
	}

	defaultPrefixAPIs := []string{
		"v1",
		"apps/",
		"autoscaling/",
		"batch/",
		"policy/",
		"metrics.k8s.io/",
		"external.metrics.k8s.io/",
	}

	versions := make([]string, 0, len(groupList.Groups)+1)
	for _, group := range groupList.Groups {
		for _, version := range group.Versions {
			if slices.ContainsFunc(defaultPrefixAPIs, func(s string) bool {
				return strings.HasPrefix(version.GroupVersion, s)
			}) {
				continue
			}

			switch {
			case version.GroupVersion == "":
				// Skip empty group/version entries.
				continue
			case strings.HasPrefix(version.GroupVersion, "v1"):
				// Skip the legacy core group, which is reported as "v1" without a group prefix.
				continue
			case strings.HasSuffix(version.GroupVersion, "k8s.io/v1"):
				// Skip the internal k8s.io group, which is not relevant to users.
				continue
			}

			versions = append(versions, version.GroupVersion)
		}
	}

	sort.Strings(versions)

	return versions, nil
}
