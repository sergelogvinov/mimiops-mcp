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

// CronJobsListResult represents the result of listing CronJobs.
type CronJobsListResult struct {
	CronJobs []CronJobSummary `json:"cronjobs" jsonschema:"List of CronJobs"`
}

// RegisterCronJobsList adds the cronjobs_list tool, which lists CronJobs in a namespace (or all namespaces).
func RegisterCronJobsList(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List CronJobs"),
		mcp.WithDescription("List CronJobs in a namespace (or all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithOutputSchema[CronJobsListResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		log.DebugContext(ctx, "cronjobs_list called", "namespace", namespace)

		// List CronJobs
		cronJobs, err := client.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no CronJobs found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list CronJobs: %v", err), nil
		}

		result := CronJobsListResult{
			CronJobs: make([]CronJobSummary, 0, len(cronJobs.Items)),
		}

		// Build result
		for _, cj := range cronJobs.Items {
			result.CronJobs = append(result.CronJobs, toCronJobSummary(&cj))
		}

		// Build fallback text
		fallbackText := "No CronJobs found"
		if len(result.CronJobs) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
