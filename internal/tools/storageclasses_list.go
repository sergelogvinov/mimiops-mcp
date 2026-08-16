package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterStorageClassesList adds the storageclasses_list tool, which lists StorageClasses in the cluster.
func RegisterStorageClassesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("storageclasses_list",
		mcp.WithDescription("List StorageClasses in the cluster."),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "storageclasses_list called")

		// List storage classes
		classes, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list storage classes: %v", err), nil
		}

		// Format output
		result, err := formatStorageClassesList(classes.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatStorageClassesList formats a list of storage classes for MCP tool output.
func formatStorageClassesList(classes []storagev1.StorageClass, format string) (string, error) {
	if format == "json" {
		return formatStorageClassesListJSON(classes)
	}
	return formatStorageClassesListText(classes), nil
}

// formatStorageClassesListText formats a list of storage classes as a markdown table.
func formatStorageClassesListText(classes []storagev1.StorageClass) string {
	if len(classes) == 0 {
		return "No storage classes found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAME | PROVISIONER | RECLAIM POLICY | VOLUME BINDING MODE | ALLOW EXPANSION | AGE |\n")
	buf.WriteString("|------|-------------|----------------|---------------------|-----------------|-----|\n")

	for _, sc := range classes {
		name := sc.Name
		provisioner := sc.Provisioner
		reclaimPolicy := "-"
		if sc.ReclaimPolicy != nil {
			reclaimPolicy = string(*sc.ReclaimPolicy)
		}
		volumeBindingMode := "-"
		if sc.VolumeBindingMode != nil {
			volumeBindingMode = string(*sc.VolumeBindingMode)
		}
		allowExpansion := "False"
		if sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion {
			allowExpansion = "True"
		}
		age := formatAge(sc.CreationTimestamp)

		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s |\n",
			name, provisioner, reclaimPolicy, volumeBindingMode, allowExpansion, age)
	}

	return buf.String()
}

// formatStorageClassesListJSON formats a list of storage classes as JSON.
func formatStorageClassesListJSON(classes []storagev1.StorageClass) (string, error) {
	summaries := make([]StorageClassSummary, 0, len(classes))
	for _, sc := range classes {
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
		summaries = append(summaries, summary)
	}

	result := struct {
		StorageClasses []StorageClassSummary `json:"storageclasses"`
	}{
		StorageClasses: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// StorageClassSummary is the trimmed representation of a storage class used by storageclasses_list.
type StorageClassSummary struct {
	Name                 string `json:"name"`
	Provisioner          string `json:"provisioner"`
	ReclaimPolicy        string `json:"reclaim_policy"`
	VolumeBindingMode    string `json:"volume_binding_mode"`
	AllowVolumeExpansion bool   `json:"allow_volume_expansion"`
	Age                  string `json:"age"`
}
