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

package toolshelm

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/helm"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"helm.sh/helm/v4/pkg/release/common"
)

// HelmStatusResult represents the result of getting Helm release status.
type HelmStatusResult struct {
	ReleaseSummary

	Resources       helm.ResourceList   `json:"resources,omitempty" jsonschema:"Kubernetes resources"`
	History         []helm.HistoryEntry `json:"history,omitempty" jsonschema:"Last 3 revisions of the releases"`
	Recommendations string              `json:"recommendations,omitempty" jsonschema:"Recommendations"`
}

// RegisterHelmStatus adds the helm_status tool, which gets the status of a Helm release.
func RegisterHelmStatus(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Helm Release Status"),
		mcp.WithDescription("Get the status of a Helm release, including the last 3 revisions of history"),
		mcp.WithString("name", mcp.Description("release name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[HelmStatusResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("helm_status", opts...)
	s.AddTool(tool, handlerHelmStatus(mc))
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=daemonsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;update;patch

// handlerHelmStatus returns a handler function for the helm_status tool.
func handlerHelmStatus(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "helm_status called",
			"cluster", client.ClusterName,
			"name", name,
			"namespace", namespace,
		)

		// Create Helm client
		helmClient, err := helm.NewHelmClient(client, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Helm client: %v", err), nil
		}

		// Get release status
		release, err := helmClient.GetRelease(name, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get status for release: %v", err), nil
		}

		// Get release history (last 3 revisions)
		history, err := helmClient.GetReleaseHistory(name, namespace, 3)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get history for release: %v", err), nil
		}

		resources, err := helmClient.GetReleaseResources(release)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get resources for release: %v", err), nil
		}

		result := HelmStatusResult{
			ReleaseSummary: toHelmSummary(release),
			History:        history,
			Resources:      resources,
		}

		now := time.Now()
		diff := now.Sub(release.Info.LastDeployed)

		// Move to skill-based recommendations
		switch result.Status {
		case string(common.StatusFailed):
			if diff > 15*time.Minute {
				result.Recommendations = "The release has failed recently. Please check the logs and consider rolling back or fixing the issue."
			}
		case string(common.StatusPendingUpgrade):
			if diff < 30*time.Minute {
				result.Recommendations = "The release is in a pending-upgrade state. It may be due to a long-running operation. Please wait and check again later."
			} else {
				result.Recommendations = "The release is in a pending-upgrade state for more than 30 minutes. It is safe to retry the upgrade again or rollback to the previous revision without hooks."
			}
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
