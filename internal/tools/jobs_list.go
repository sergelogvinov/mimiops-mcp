package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobSummary is the trimmed, agent-friendly representation of a Job
// used by jobs_list (and available in the JSON output of other job tools).
type JobSummary struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Completions string `json:"completions"`
	Duration    string `json:"duration,omitempty"`
	Age         string `json:"age"`
	Status      string `json:"status"`
}

// RegisterJobsList adds the jobs_list tool, which lists Jobs in a namespace (or all namespaces).
func RegisterJobsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_list",
		mcp.WithDescription("List Jobs in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; empty = all namespaces"), mcp.Required()),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		labelSelector := req.GetString("label_selector", "")
		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "jobs_list called",
			"namespace", namespace,
			"label_selector", labelSelector,
		)

		// Use metav1.NamespaceAll for empty namespace (all namespaces)
		ns := namespace
		if ns == "" {
			ns = metav1.NamespaceAll
		}

		opts := metav1.ListOptions{}
		if labelSelector != "" {
			opts.LabelSelector = labelSelector
		}

		jobs, err := client.BatchV1().Jobs(ns).List(ctx, opts)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list Jobs in namespace '%s': %v", ns, err), nil
		}

		result, err := formatJobsList(jobs.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatJobsList formats a list of Jobs for MCP tool output.
func formatJobsList(jobs []batchv1.Job, format string) (string, error) {
	if format == "json" {
		return formatJobsListJSON(jobs)
	}
	return formatJobsListText(jobs), nil
}

// formatJobsListText formats a list of Jobs as a markdown table.
func formatJobsListText(jobs []batchv1.Job) string {
	if len(jobs) == 0 {
		return "No Jobs found."
	}

	var buf bytes.Buffer
	buf.WriteString("| NAMESPACE | NAME | COMPLETIONS | DURATION | AGE | STATUS |\n")
	buf.WriteString("|-----------|------|-------------|----------|-----|--------|\n")

	for _, job := range jobs {
		age := formatAge(job.CreationTimestamp)

		// Calculate completions
		completions := fmt.Sprintf("%d/%d", job.Status.Succeeded, *job.Spec.Completions)
		if job.Spec.Completions == nil {
			completions = fmt.Sprintf("%d/1", job.Status.Succeeded)
		}

		// Calculate duration
		duration := "-"
		if job.Status.StartTime != nil && job.Status.CompletionTime != nil {
			duration = formatDuration(*job.Status.CompletionTime, *job.Status.StartTime)
		}

		// Derive status
		status := deriveJobStatus(job)

		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s | %s |\n",
			job.Namespace,
			job.Name,
			completions,
			duration,
			age,
			status,
		)
	}

	return buf.String()
}

// formatJobsListJSON formats a list of Jobs as JSON.
func formatJobsListJSON(jobs []batchv1.Job) (string, error) {
	summaries := make([]JobSummary, 0, len(jobs))
	for _, job := range jobs {
		summaries = append(summaries, toJobSummary(job))
	}

	result := struct {
		Jobs []JobSummary `json:"jobs"`
	}{
		Jobs: summaries,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// toJobSummary converts a Job to a JobSummary.
func toJobSummary(job batchv1.Job) JobSummary {
	completions := fmt.Sprintf("%d/%d", job.Status.Succeeded, *job.Spec.Completions)
	if job.Spec.Completions == nil {
		completions = fmt.Sprintf("%d/1", job.Status.Succeeded)
	}

	duration := ""
	if job.Status.StartTime != nil && job.Status.CompletionTime != nil {
		duration = formatDuration(*job.Status.CompletionTime, *job.Status.StartTime)
	}

	return JobSummary{
		Namespace:   job.Namespace,
		Name:        job.Name,
		Completions: completions,
		Duration:    duration,
		Age:         formatAge(job.CreationTimestamp),
		Status:      deriveJobStatus(job),
	}
}

// deriveJobStatus derives the status string for a Job.
func deriveJobStatus(job batchv1.Job) string {
	if job.Status.Succeeded > 0 && job.Status.Succeeded == *job.Spec.Completions {
		return "Complete"
	}
	if job.Status.Failed > 0 {
		return "Failed"
	}
	if job.Status.Active > 0 {
		return "Running"
	}
	return "Pending"
}

// formatDuration calculates the duration between two times.
func formatDuration(end, start metav1.Time) string {
	endTime := end.Time
	startTime := start.Time
	diff := endTime.Sub(startTime)
	if diff < time.Second {
		return "0s"
	}
	if diff < time.Minute {
		return fmt.Sprintf("%ds", int(diff.Seconds()))
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd", int(diff.Hours()/24))
}
