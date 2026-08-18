package tools

import (
	"context"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobLogResult is the structured result of jobs_log.
type JobLogResult struct {
	Streams []LogStream `json:"streams" jsonschema:"Log streams from the Job's pods"`
}

// RegisterJobsLog adds the jobs_log tool, which fetches logs from a Job's pods.
func RegisterJobsLog(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_log",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Get Job Logs"),
		mcp.WithDescription("Fetch logs from a Job's pods"),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("container", mcp.Description("container name (optional)")),
		mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
		mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
		mcp.WithBoolean("all_pods", mcp.Description("fetch logs from all owned pods"), mcp.DefaultBool(false)),
		mcp.WithOutputSchema[JobLogResult](),
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
		allPods := req.GetBool("all_pods", false)

		log.DebugContext(ctx, "jobs_log called",
			"namespace", namespace,
			"job", name,
			"container", container,
			"tail", tail,
			"previous", previous,
			"all_pods", allPods,
		)

		// Get the Job
		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// List pods owned by this Job
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list pods in namespace '%s': %v", namespace, err), nil
		}

		// Filter pods by Job owner reference
		var ownedPods []corev1.Pod
		for _, pod := range pods.Items {
			if metav1.GetControllerOf(&pod) != nil && metav1.GetControllerOf(&pod).UID == job.UID {
				ownedPods = append(ownedPods, pod)
			}
		}

		// Build result with empty streams if no pods
		if len(ownedPods) == 0 {
			return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' has no pods yet (not started or already cleaned up)", name, namespace), nil
		}

		// Determine which pods to fetch logs from
		var podsToFetch []corev1.Pod
		if allPods {
			podsToFetch = ownedPods
		} else {
			// Fetch logs from the most recently created pod
			latestPod := ownedPods[0]
			for _, pod := range ownedPods {
				if pod.CreationTimestamp.After(latestPod.CreationTimestamp.Time) {
					latestPod = pod
				}
			}
			podsToFetch = []corev1.Pod{latestPod}
		}

		// Fetch logs from pods
		streams := make([]LogStream, 0, len(podsToFetch))
		for _, pod := range podsToFetch {
			stream, err := fetchPodLogStream(ctx, client, namespace, pod.Name, container, tail, 0, previous)
			if err != nil {
				return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s': %v", pod.Name, err), nil
			}
			streams = append(streams, stream)
		}

		return mcp.NewToolResultStructuredOnly(JobLogResult{Streams: streams}), nil
	})
}
