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

// JobGetResult represents the result of getting a single Job.
type JobGetResult struct {
	JobSummary

	Labels      map[string]string `json:"labels" jsonschema:"labels of the Job"`
	Annotations map[string]string `json:"annotations" jsonschema:"annotations of the Job"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the Job"`
}

// RegisterJobsGet adds the jobs_get tool, which gets a single Job's full spec and status.
func RegisterJobsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithDescription("Get a single Job's spec and status."),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[JobGetResult](),
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

		log.DebugContext(ctx, "jobs_get called",
			"namespace", namespace,
			"job", name,
		)

		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildJobGetResult(job)
		fallbackText := fmt.Sprintf("Job '%s' in namespace '%s' has status '%s' with completions %s. Age: %s.", result.Name, result.Namespace, result.Status, result.Completions, result.Age)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}

// buildJobGetResult builds a JobGetResult from a Job.
func buildJobGetResult(job *batchv1.Job) *JobGetResult {
	result := &JobGetResult{}

	result.Labels = job.Labels
	result.Annotations = job.Annotations

	// Spec (simplified)
	result.Spec = make(map[string]any)
	result.Spec["completions"] = job.Spec.Completions
	result.Spec["parallelism"] = job.Spec.Parallelism
	result.Spec["backoffLimit"] = job.Spec.BackoffLimit
	result.Spec["activeDeadlineSeconds"] = job.Spec.ActiveDeadlineSeconds

	// Summary
	result.JobSummary = toJobSummary(*job)

	return result
}
