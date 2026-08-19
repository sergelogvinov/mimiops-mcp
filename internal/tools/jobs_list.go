package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		mcp.WithDescription("List Jobs in a namespace (or all namespaces)"),
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

		jobs, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Jobs found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list Jobs: %v", err), nil
		}

		result := JobsListResult{
			Jobs: make([]JobSummary, 0, len(jobs.Items)),
		}

		// Build result
		for _, job := range jobs.Items {
			result.Jobs = append(result.Jobs, toJobSummary(&job))
		}

		// Build fallback text
		fallbackText := "No Jobs found"
		if len(result.Jobs) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
