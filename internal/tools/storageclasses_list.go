package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageClassesListResult represents the result of listing storage classes.
type StorageClassesListResult struct {
	StorageClasses []StorageClassSummary `json:"storageclasses" jsonschema:"List of storage classes"`
}

// RegisterStorageClassesList adds the storageclasses_list tool, which lists StorageClasses in the cluster.
func RegisterStorageClassesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("storageclasses_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List StorageClasses"),
		mcp.WithDescription("List StorageClasses in the cluster"),
		mcp.WithOutputSchema[StorageClassesListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.DebugContext(ctx, "storageclasses_list called")

		// List storage classes
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
				Age:                  formatAge(sc.CreationTimestamp),
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
		var fallbackText string
		switch len(result.StorageClasses) {
		case 0:
			fallbackText = "No storage classes found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 storage class: %s (provisioner: %s)", result.StorageClasses[0].Name, result.StorageClasses[0].Provisioner)
		default:
			fallbackText = fmt.Sprintf("Found %d storage classes", len(result.StorageClasses))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
