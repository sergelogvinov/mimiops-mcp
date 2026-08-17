package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodLogResult is the structured result of pods_log.
type PodLogResult struct {
	PodSummary

	Streams []LogStream `json:"streams" jsonschema:"Log streams from the pod's containers"`
}

// RegisterPodsLog adds the pods_log tool, which fetches pod logs.
func RegisterPodsLog(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("pods_log",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Get Pod Logs"),
		mcp.WithDescription("Fetch pod logs"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("container", mcp.Description("container name (optional)")),
		mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
		mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
		mcp.WithInteger("since_seconds", mcp.Description("only return logs newer than N seconds"), mcp.DefaultNumber(0)),
		mcp.WithOutputSchema[PodLogResult](),
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

		container := req.GetString("container", "")
		tail := req.GetInt("tail", 20)
		previous := req.GetBool("previous", false)
		sinceSeconds := req.GetInt("since_seconds", 0)

		log.DebugContext(ctx, "pods_log called",
			"namespace", namespace,
			"pod", name,
			"container", container,
			"tail", tail,
			"previous", previous,
			"since_seconds", sinceSeconds,
		)

		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Get pod to check container name
		stream, err := fetchPodLogStream(ctx, client, namespace, name, container, tail, sinceSeconds, previous)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s': %v", name, err), nil
		}

		// Build result
		result := PodLogResult{
			PodSummary: toPodSummary(ctx, client, pod),
			Streams:    []LogStream{stream},
		}
		fallback := fmt.Sprintf("Pod '%s' in namespace '%s' has %d log stream(s).", name, namespace, len(result.Streams))

		return mcp.NewToolResultStructured(result, fallback), nil
	})
}
