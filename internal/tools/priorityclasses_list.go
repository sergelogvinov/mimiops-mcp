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
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterPriorityClassesList adds the priorityclasses_list tool, which lists PriorityClasses in the cluster.
func RegisterPriorityClassesList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("priorityclasses_list",
		mcp.WithDescription("List PriorityClasses in the cluster."),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "priorityclasses_list called")

		// List priority classes
		classes, err := client.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list priority classes: %v", err), nil
		}

		// Format output
		result, err := formatPriorityClassesList(classes.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatPriorityClassesList formats a list of priority classes for MCP tool output.
func formatPriorityClassesList(classes []schedulingv1.PriorityClass, format string) (string, error) {
	if format == "json" {
		return formatPriorityClassesListJSON(classes)
	}
	return formatPriorityClassesListText(classes), nil
}

// formatPriorityClassesListText formats a list of priority classes as a markdown table.
func formatPriorityClassesListText(classes []schedulingv1.PriorityClass) string {
	if len(classes) == 0 {
		return "No priority classes found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAME | VALUE | GLOBAL DEFAULT | DESCRIPTION | AGE |\n")
	buf.WriteString("|------|-------|----------------|-------------|-----|\n")

	for _, pc := range classes {
		name := pc.Name
		value := pc.Value
		globalDefault := "False"
		if pc.GlobalDefault {
			globalDefault = "True"
		}
		description := pc.Description
		if description == "" {
			description = "-"
		}
		age := formatAge(pc.CreationTimestamp)

		fmt.Fprintf(&buf, "| %s | %d | %s | %s | %s |\n",
			name, value, globalDefault, description, age)
	}

	return buf.String()
}

// formatPriorityClassesListJSON formats a list of priority classes as JSON.
func formatPriorityClassesListJSON(classes []schedulingv1.PriorityClass) (string, error) {
	summaries := make([]PriorityClassSummary, 0, len(classes))
	for _, pc := range classes {
		summary := PriorityClassSummary{
			Name:          pc.Name,
			Value:         pc.Value,
			GlobalDefault: pc.GlobalDefault,
			Description:   pc.Description,
			Age:           formatAge(pc.CreationTimestamp),
		}
		summaries = append(summaries, summary)
	}

	result := struct {
		PriorityClasses []PriorityClassSummary `json:"priorityclasses"`
	}{
		PriorityClasses: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PriorityClassSummary is the trimmed representation of a priority class used by priorityclasses_list.
type PriorityClassSummary struct {
	Name          string `json:"name"`
	Value         int32  `json:"value"`
	GlobalDefault bool   `json:"global_default"`
	Description   string `json:"description"`
	Age           string `json:"age"`
}
