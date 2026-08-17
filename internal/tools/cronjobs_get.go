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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobGetResult represents the result of getting a single CronJob.
type CronJobGetResult struct {
	CronJobSummary

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the CronJob"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the CronJob"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the CronJob"`
}

// RegisterCronJobsGet adds the cronjobs_get tool, which gets a single CronJob's full spec and status.
func RegisterCronJobsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get CronJob"),
		mcp.WithDescription("Get a single CronJob's spec and status"),
		mcp.WithString("name", mcp.Description("CronJob name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[CronJobGetResult](),
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

		log.DebugContext(ctx, "cronjobs_get called",
			"namespace", namespace,
			"cronjob", name,
		)

		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildCronJobGetResult(cronJob)
		fallbackText := fmt.Sprintf("CronJob '%s' in namespace '%s' has schedule '%s' with status '%s'. Age: %s.", result.Name, result.Namespace, result.Schedule, result.Status, result.Age)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildCronJobGetResult builds a CronJobGetResult from a CronJob.
func buildCronJobGetResult(cj *batchv1.CronJob) *CronJobGetResult {
	result := &CronJobGetResult{
		CronJobSummary: toCronJobSummary(*cj),
		Labels:         cj.Labels,
		Annotations:    cj.Annotations,
		Spec:           make(map[string]any),
	}

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
	result.Spec["concurrencyPolicy"] = cj.Spec.ConcurrencyPolicy
	result.Spec["startingDeadlineSeconds"] = cj.Spec.StartingDeadlineSeconds
	result.Spec["successfulJobsHistoryLimit"] = cj.Spec.SuccessfulJobsHistoryLimit
	result.Spec["failedJobsHistoryLimit"] = cj.Spec.FailedJobsHistoryLimit

	return result
}
