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

// JobDeleteResult represents the result of deleting a Job.
type JobDeleteResult struct {
	Name      string `json:"name" jsonschema:"Name of the deleted Job"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the deleted Job"`
	Deleted   bool   `json:"deleted" jsonschema:"Whether the Job was successfully deleted"`
}

// RegisterJobsDelete adds the jobs_delete tool, which deletes a Job.
func RegisterJobsDelete(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_delete",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Delete Job"),
		mcp.WithDescription("Delete a Job (cascading — also deletes owned pods)"),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("propagation_policy", mcp.Description("propagation policy"), mcp.Enum("Background", "Foreground", "Orphan"), mcp.DefaultString("Background")),
		mcp.WithOutputSchema[JobDeleteResult](),
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

		propagationPolicyStr := req.GetString("propagation_policy", "Background")
		propagationPolicy := metav1.DeletionPropagation(propagationPolicyStr)

		log.DebugContext(ctx, "jobs_delete called",
			"namespace", namespace,
			"job", name,
			"propagation_policy", propagationPolicyStr,
		)

		// Delete the Job
		err := client.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to delete Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := JobDeleteResult{
			Name:      name,
			Namespace: namespace,
			Deleted:   true,
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}
