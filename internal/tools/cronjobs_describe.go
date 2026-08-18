package tools

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobDescribeResult represents the result of describing a CronJob.
type CronJobDescribeResult struct {
	CronJobSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the CronJob"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the CronJob"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the CronJob"`
	ActiveJobs  []string          `json:"activeJobs" jsonschema:"List of active job names"`
}

// RegisterCronJobsDescribe adds the cronjobs_describe tool, which provides a structured CronJob summary.
func RegisterCronJobsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_describe",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe CronJob"),
		mcp.WithDescription("CronJob summary (schedule, suspend, concurrency policy, active jobs, last schedule, job template)"),
		mcp.WithString("name", mcp.Description("CronJob name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[CronJobDescribeResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		log.DebugContext(ctx, "cronjobs_describe called",
			"namespace", namespace,
			"cronjob", name,
		)

		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := buildCronJobDescribeResult(cronJob)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to build result: %v", err), nil
		}

		// Build fallback text with summary
		fallbackText := fmt.Sprintf("CronJob '%s' in namespace '%s' has schedule '%s' with status '%s'. Age: %s.",
			result.Name, result.Namespace, result.Schedule, result.Status, result.Age)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildCronJobDescribeResult builds a CronJobDescribeResult from a CronJob.
func buildCronJobDescribeResult(cj *batchv1.CronJob) (*CronJobDescribeResult, error) {
	result := &CronJobDescribeResult{
		CronJobSummary: toCronJobSummary(*cj),
	}

	result.Labels = cj.Labels
	result.Annotations = cj.Annotations

	if result.Labels == nil {
		result.Labels = make(map[string]string)
	}
	if result.Annotations == nil {
		result.Annotations = make(map[string]string)
	}

	// Remove internal annotations
	maps.DeleteFunc(result.Annotations, func(k, _ string) bool {
		return k == "kubectl.kubernetes.io/last-applied-configuration"
	})

	// Spec (simplified)
	result.Spec = make(map[string]any)
	result.Spec["schedule"] = cj.Spec.Schedule
	result.Spec["suspend"] = cj.Spec.Suspend
	result.Spec["concurrencyPolicy"] = cj.Spec.ConcurrencyPolicy
	result.Spec["startingDeadlineSeconds"] = cj.Spec.StartingDeadlineSeconds
	result.Spec["successfulJobsHistoryLimit"] = cj.Spec.SuccessfulJobsHistoryLimit
	result.Spec["failedJobsHistoryLimit"] = cj.Spec.FailedJobsHistoryLimit

	// Active Jobs
	result.ActiveJobs = make([]string, 0, len(cj.Status.Active))
	for _, job := range cj.Status.Active {
		result.ActiveJobs = append(result.ActiveJobs, job.Name)
	}

	return result, nil
}
