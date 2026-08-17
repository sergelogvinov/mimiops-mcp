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
			return mcp.NewToolResultErrorf("failed to list CronJobs in namespace '%s': %v", namespace, err), nil
		}

		result := CronJobsListResult{
			CronJobs: make([]CronJobSummary, 0, len(cronJobs.Items)),
		}

		// Build result
		for _, cj := range cronJobs.Items {
			result.CronJobs = append(result.CronJobs, toCronJobSummary(cj))
		}

		var fallbackText string
		switch len(result.CronJobs) {
		case 0:
			fallbackText = "No CronJobs found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 CronJob: %s in namespace %s (%s)", result.CronJobs[0].Name, result.CronJobs[0].Namespace, result.CronJobs[0].Status)
		default:
			fallbackText = fmt.Sprintf("Found %d CronJobs", len(result.CronJobs))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// toCronJobSummary converts a CronJob to a CronJobSummary.
func toCronJobSummary(cj batchv1.CronJob) CronJobSummary {
	suspend := false
	if cj.Spec.Suspend != nil {
		suspend = *cj.Spec.Suspend
	}

	lastSchedule := ""
	if cj.Status.LastScheduleTime != nil {
		lastSchedule = formatAgeMin(*cj.Status.LastScheduleTime)
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
