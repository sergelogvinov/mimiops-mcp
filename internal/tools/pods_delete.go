package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterPodsDelete adds the pods_delete tool, which deletes a pod.
func RegisterPodsDelete(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("pods_delete",
		mcp.WithDescription("Delete a pod."),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithInteger("grace_period_seconds", mcp.Description("grace period in seconds"), mcp.DefaultNumber(30)),
		mcp.WithBoolean("confirm", mcp.Description("set to true to confirm deletion"), mcp.DefaultBool(false)),
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

		gracePeriodSeconds := req.GetInt("grace_period_seconds", 30)
		confirm := req.GetBool("confirm", false)

		// Check if destructive operations are allowed
		// This should be checked at registration time via allowDestructive flag
		// But we also check here as a safety measure
		if !confirm {
			return mcp.NewToolResultErrorf(
				"This will delete pod '%s' in namespace '%s'. Call again with confirm=true to proceed.",
				name, namespace,
			), nil
		}

		// Delete the pod - convert int to int64
		gracePeriodSecondsInt64 := int64(gracePeriodSeconds)
		err := client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
			GracePeriodSeconds: &gracePeriodSecondsInt64,
		})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to delete pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Pod '%s' in namespace '%s' deleted successfully.", name, namespace)), nil
	})
}
