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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageClassesListResult represents the result of listing storage classes.
type StorageClassesListResult struct {
	StorageClasses []StorageClassSummary `json:"storageclasses" jsonschema:"List of storage classes"`
}

// RegisterStorageClassesList adds the storageclasses_list tool, which lists StorageClasses in the cluster.
func RegisterStorageClassesList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List StorageClasses"),
		mcp.WithDescription("List StorageClasses in the cluster"),
		mcp.WithOutputSchema[StorageClassesListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("storageclasses_list", opts...)
	s.AddTool(tool, handlerStorageClassesList(mc))
}

// +kubebuilder:rbac:groups="storage.k8s.io",resources=storageclasses,verbs=list;watch

// handlerStorageClassesList returns a handler function for the storageclasses_list tool.
func handlerStorageClassesList(mc *k8s.MultiClusterClient) func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "storageclasses_list called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
		)

		classes, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Storage Classes found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list storage classes: %v", err), nil
		}

		result := StorageClassesListResult{
			StorageClasses: make([]StorageClassSummary, 0, len(classes.Items)),
		}

		// Build result
		for _, sc := range classes.Items {
			allowExpansion := false
			if sc.AllowVolumeExpansion != nil {
				allowExpansion = *sc.AllowVolumeExpansion
			}
			summary := StorageClassSummary{
				Name:                 sc.Name,
				Provisioner:          sc.Provisioner,
				AllowVolumeExpansion: allowExpansion,
				Age:                  age.FormatAge(sc.CreationTimestamp),
			}
			if sc.ReclaimPolicy != nil {
				summary.ReclaimPolicy = string(*sc.ReclaimPolicy)
			}
			if sc.VolumeBindingMode != nil {
				summary.VolumeBindingMode = string(*sc.VolumeBindingMode)
			}

			result.StorageClasses = append(result.StorageClasses, summary)
		}

		// Build fallback text
		fallbackText := "No storage classes found"
		if len(result.StorageClasses) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
