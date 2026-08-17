package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobsListResult represents the result of listing Jobs.
type JobsListResult struct {
	Jobs []JobSummary `json:"jobs" jsonschema:"List of jobs"`
}

// RegisterJobsList adds the jobs_list tool, which lists Jobs in a namespace (or all namespaces).
func RegisterJobsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Jobs"),
		mcp.WithDescription("List Jobs in a namespace (or all namespaces)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithOutputSchema[JobsListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		labelSelector := req.GetString("label_selector", "")

		log.DebugContext(ctx, "jobs_list called",
			"namespace", namespace,
			"label_selector", labelSelector,
		)

		opts := metav1.ListOptions{}
		if labelSelector != "" {
			opts.LabelSelector = labelSelector
		}

		jobs, err := client.BatchV1().Jobs(namespace).List(ctx, opts)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list Jobs in namespace '%s': %v", namespace, err), nil
		}

		summaries := make([]JobSummary, 0, len(jobs.Items))
		for _, job := range jobs.Items {
			summaries = append(summaries, toJobSummary(job))
		}

		return mcp.NewToolResultStructuredOnly(JobsListResult{Jobs: summaries}), nil
	})
}

// toJobSummary converts a Job to a JobSummary.
func toJobSummary(job batchv1.Job) JobSummary {
	completions := fmt.Sprintf("%d/1", job.Status.Succeeded)
	if job.Spec.Completions != nil {
		completions = fmt.Sprintf("%d/%d", job.Status.Succeeded, *job.Spec.Completions)
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
	if job.Spec.Completions != nil && job.Status.Succeeded > 0 && job.Status.Succeeded == *job.Spec.Completions {
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
