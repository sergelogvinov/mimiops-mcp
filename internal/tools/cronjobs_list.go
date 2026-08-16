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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobSummary is the trimmed, agent-friendly representation of a CronJob
// used by cronjobs_list (and available in the JSON output of other cronjob tools).
type CronJobSummary struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	Schedule     string `json:"schedule"`
	Suspend      bool   `json:"suspend"`
	Status       string `json:"status"`
	LastSchedule string `json:"lastSchedule,omitempty"`
	Age          string `json:"age"`
}

// RegisterCronJobsList adds the cronjobs_list tool, which lists CronJobs in a namespace (or all namespaces).
func RegisterCronJobsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_list",
		mcp.WithDescription("List CronJobs in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; empty = all namespaces"), mcp.Required()),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "cronjobs_list called",
			"namespace", namespace,
		)

		// Use metav1.NamespaceAll for empty namespace (all namespaces)
		ns := namespace
		if ns == "" {
			ns = metav1.NamespaceAll
		}

		cronJobs, err := client.BatchV1().CronJobs(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list CronJobs in namespace '%s': %v", ns, err), nil
		}

		result, err := formatCronJobsList(cronJobs.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatCronJobsList formats a list of CronJobs for MCP tool output.
func formatCronJobsList(cronJobs []batchv1.CronJob, format string) (string, error) {
	if format == "json" {
		return formatCronJobsListJSON(cronJobs)
	}
	return formatCronJobsListText(cronJobs), nil
}

// formatCronJobsListText formats a list of CronJobs as a markdown table.
func formatCronJobsListText(cronJobs []batchv1.CronJob) string {
	if len(cronJobs) == 0 {
		return "No CronJobs found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAMESPACE | NAME | SCHEDULE | SUSPEND | STATUS | LAST_SCHEDULE | AGE |\n")
	buf.WriteString("|-----------|------|----------|---------|--------|---------------|-----|\n")

	for _, cj := range cronJobs {
		age := formatAge(cj.CreationTimestamp)
		suspend := "False"
		if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
			suspend = "True"
		}

		lastSchedule := "-"
		if cj.Status.LastScheduleTime != nil {
			lastSchedule = formatAge(*cj.Status.LastScheduleTime)
		}

		status := deriveCronJobStatus(cj)

		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s | %s |\n",
			cj.Namespace,
			cj.Name,
			cj.Spec.Schedule,
			suspend,
			status,
			lastSchedule,
			age,
		)
	}

	return buf.String()
}

// formatCronJobsListJSON formats a list of CronJobs as JSON.
func formatCronJobsListJSON(cronJobs []batchv1.CronJob) (string, error) {
	summaries := make([]CronJobSummary, 0, len(cronJobs))
	for _, cj := range cronJobs {
		summaries = append(summaries, toCronJobSummary(cj))
	}

	result := struct {
		CronJobs []CronJobSummary `json:"cronjobs"`
	}{
		CronJobs: summaries,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// toCronJobSummary converts a CronJob to a CronJobSummary.
func toCronJobSummary(cj batchv1.CronJob) CronJobSummary {
	suspend := false
	if cj.Spec.Suspend != nil {
		suspend = *cj.Spec.Suspend
	}

	lastSchedule := ""
	if cj.Status.LastScheduleTime != nil {
		lastSchedule = cj.Status.LastScheduleTime.String()
	}

	return CronJobSummary{
		Namespace:    cj.Namespace,
		Name:         cj.Name,
		Schedule:     cj.Spec.Schedule,
		Suspend:      suspend,
		Status:       deriveCronJobStatus(cj),
		LastSchedule: lastSchedule,
		Age:          formatAge(cj.CreationTimestamp),
	}
}

// deriveCronJobStatus derives the status string for a CronJob.
func deriveCronJobStatus(cj batchv1.CronJob) string {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		// Check for active jobs
		if len(cj.Status.Active) == 0 {
			return "Suspended"
		}
		return fmt.Sprintf("Suspended (%d/%d)", len(cj.Status.Active), len(cj.Status.Active))
	}
	return "Active"
}
