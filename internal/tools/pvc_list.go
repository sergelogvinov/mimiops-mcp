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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PersistentVolumeClaimsListResult represents the result of listing PersistentVolumeClaims.
type PersistentVolumeClaimsListResult struct {
	PersistentVolumeClaims []PVCSummary `json:"persistentvolumeclaims" jsonschema:"List of PersistentVolumeClaims"`
}

// RegisterPersistentVolumeClaimsList adds the persistentvolumeclaims_list tool, which lists
// PersistentVolumeClaims in a namespace (or all namespaces).
func RegisterPersistentVolumeClaimsList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List PersistentVolumeClaims"),
		mcp.WithDescription("List PersistentVolumeClaims in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithOutputSchema[PersistentVolumeClaimsListResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("persistentvolumeclaims_list", opts...)
	s.AddTool(tool, handlerPersistentVolumeClaimsList(mc))
}

// handlerPersistentVolumeClaimsList returns a handler function for the persistentvolumeclaims_list tool.
func handlerPersistentVolumeClaimsList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		labelSelector := req.GetString("label_selector", "")
		fieldSelector := req.GetString("field_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "persistentvolumeclaims_list called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"label_selector", labelSelector,
			"field_selector", fieldSelector,
		)

		// List persistent volume claims
		pvcs, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
			FieldSelector: fieldSelector,
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no PersistentVolumeClaims found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list PersistentVolumeClaims: %v", err), nil
		}

		result := PersistentVolumeClaimsListResult{
			PersistentVolumeClaims: make([]PVCSummary, 0, len(pvcs.Items)),
		}

		// Build result
		for _, pvc := range pvcs.Items {
			result.PersistentVolumeClaims = append(result.PersistentVolumeClaims, toPVCSummary(&pvc))
		}

		// Build fallback text
		fallbackText := "No PersistentVolumeClaims found"
		if len(result.PersistentVolumeClaims) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}

// toPVCSummary converts a PersistentVolumeClaim to a PVCSummary.
func toPVCSummary(pvc *corev1.PersistentVolumeClaim) PVCSummary {
	// Prefer the bound capacity, fall back to the requested storage
	capacity := ""
	if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		capacity = q.String()
	} else if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		capacity = q.String()
	}

	storageClass := ""
	if pvc.Spec.StorageClassName != nil {
		storageClass = *pvc.Spec.StorageClassName
	}

	return PVCSummary{
		Name:         pvc.Name,
		Namespace:    pvc.Namespace,
		Status:       string(pvc.Status.Phase),
		Volume:       pvc.Spec.VolumeName,
		Capacity:     capacity,
		StorageClass: storageClass,
		Age:          age.FormatAge(pvc.CreationTimestamp),
	}
}
