package tools

import (
	"context"
	"log/slog"
	"maps"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		mcp.WithToolTitle("Get Job"),
		mcp.WithDescription("Get a single Job's spec and status"),
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
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildJobGetResult(job)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildJobGetResult builds a JobGetResult from a Job.
func buildJobGetResult(job *batchv1.Job) *JobGetResult {
	result := &JobGetResult{
		JobSummary:  toJobSummary(job),
		Labels:      job.Labels,
		Annotations: job.Annotations,
		Spec:        make(map[string]any),
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
	result.Spec["completions"] = job.Spec.Completions
	result.Spec["parallelism"] = job.Spec.Parallelism
	result.Spec["backoffLimit"] = job.Spec.BackoffLimit
	result.Spec["activeDeadlineSeconds"] = job.Spec.ActiveDeadlineSeconds

	return result
}
