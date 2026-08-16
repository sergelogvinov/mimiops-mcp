package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// RegisterCronJobsResume adds the cronjobs_resume tool, which resumes a suspended CronJob.
func RegisterCronJobsResume(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_resume",
		mcp.WithDescription("Resume a suspended CronJob (re-enables future scheduled runs)."),
		mcp.WithString("name", mcp.Description("CronJob name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
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

		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "cronjobs_resume called",
			"namespace", namespace,
			"cronjob", name,
			"format", format,
		)

		// Get the CronJob first
		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Check if already running (not suspended)
		if cronJob.Spec.Suspend == nil || !*cronJob.Spec.Suspend {
			return mcp.NewToolResultText(fmt.Sprintf("CronJob '%s' in namespace '%s' is already running (not suspended).", name, namespace)), nil
		}

		// Patch to resume
		patch := []byte(`{"spec":{"suspend":false}}`)
		_, err = client.BatchV1().CronJobs(namespace).Patch(ctx, name, k8stypes.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to resume CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := formatCronJobSuspendResume(cronJob, false, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}
