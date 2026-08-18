package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// RegisterCronJobsSuspend adds the cronjobs_suspend tool, which suspends a CronJob (disables future scheduled runs).
func RegisterCronJobsSuspend(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_suspend",
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Suspend CronJob"),
		mcp.WithDescription("Suspend a CronJob (stops future scheduled runs)."),
		mcp.WithString("name", mcp.Description("CronJob name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[CronJobSuspendResumeResult](),
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

		log.DebugContext(ctx, "cronjobs_suspend called",
			"namespace", namespace,
			"cronjob", name,
		)

		// Get the CronJob first
		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Check if already suspended
		if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
			result := CronJobSuspendResumeResult{
				CronJobSummary: toCronJobSummary(*cronJob),
			}
			return mcp.NewToolResultStructured(result, fmt.Sprintf("CronJob '%s' in namespace '%s' is already suspended.", name, namespace)), nil
		}

		// Patch to suspend
		patch := []byte(`{"spec":{"suspend":true}}`)
		_, err = client.BatchV1().CronJobs(namespace).Patch(ctx, name, k8stypes.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to suspend CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Get the updated CronJob after patch
		updatedCronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found after patch", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get updated CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := CronJobSuspendResumeResult{
			CronJobSummary: toCronJobSummary(*updatedCronJob),
		}
		fallbackText := fmt.Sprintf("Suspended CronJob '%s' in namespace '%s'", name, namespace)

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
