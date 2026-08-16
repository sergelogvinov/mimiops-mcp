package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterJobsLog adds the jobs_log tool, which fetches logs from a Job's pods.
func RegisterJobsLog(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_log",
		mcp.WithDescription("Fetch logs from a Job's pods."),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("container", mcp.Description("container name (optional)")),
		mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
		mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
		mcp.WithBoolean("all_pods", mcp.Description("fetch logs from all owned pods"), mcp.DefaultBool(false)),
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

		container := req.GetString("container", "")
		tail := req.GetInt("tail", 20)
		previous := req.GetBool("previous", false)
		allPods := req.GetBool("all_pods", false)
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

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

		if len(ownedPods) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("Job '%s' has no pods yet (not started or already cleaned up).", name)), nil
		}

		// Fetch logs from pods
		var logBuffer bytes.Buffer
		var logLines []LogLine

		if allPods {
			// Fetch logs from all pods
			for _, pod := range ownedPods {
				podLog, err := fetchPodLogs(ctx, client, namespace, pod.Name, container, tail, previous)
				if err != nil {
					return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s': %v", pod.Name, err), nil
				}
				if format == "text" {
					fmt.Fprintf(&logBuffer, "=== %s ===\n%s\n", pod.Name, podLog)
				} else {
					// Parse JSON logs and append
					var podLogs struct {
						Logs []LogLine `json:"logs"`
					}
					if err := json.Unmarshal([]byte(podLog), &podLogs); err == nil {
						logLines = append(logLines, podLogs.Logs...)
					}
				}
			}
		} else {
			// Fetch logs from the most recently created pod
			latestPod := ownedPods[0]
			for _, pod := range ownedPods {
				if pod.CreationTimestamp.After(latestPod.CreationTimestamp.Time) {
					latestPod = pod
				}
			}
			podLog, err := fetchPodLogs(ctx, client, namespace, latestPod.Name, container, tail, previous)
			if err != nil {
				return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s': %v", latestPod.Name, err), nil
			}
			logBuffer.WriteString(podLog)
		}

		if format == "json" {
			result := struct {
				Logs []LogLine `json:"logs"`
			}{
				Logs: logLines,
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		}

		return mcp.NewToolResultText(logBuffer.String()), nil
	})
}

// fetchPodLogs fetches logs from a single pod (reuses pods_log logic).
func fetchPodLogs(ctx context.Context, client *k8s.Client, namespace, podName, container string, tail int, previous bool) (string, error) {
	// Get pod to check container names
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get pod '%s': %v", podName, err)
	}

	// Determine container name if not specified
	if container == "" {
		if len(pod.Spec.Containers) == 1 {
			container = pod.Spec.Containers[0].Name
		}
		if pod.Annotations != nil {
			if defaultContainer, ok := pod.Annotations[defaultContainerAnnotation]; ok && defaultContainer != "" {
				container = defaultContainer
			}
		}
		if len(pod.Spec.Containers) > 0 {
			container = pod.Spec.Containers[0].Name
		}
	}

	// Get log options
	tailInt64 := int64(tail)
	logOpts := &corev1.PodLogOptions{
		Container: container,
		TailLines: &tailInt64,
		Previous:  previous,
	}

	// Fetch logs
	logReq := client.CoreV1().Pods(namespace).GetLogs(podName, logOpts)
	logStream, err := logReq.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to fetch logs for pod '%s' container '%s': %v", podName, container, err)
	}
	defer logStream.Close() //nolint:errcheck

	// Read all logs
	var logBuffer bytes.Buffer
	buf := make([]byte, 1024)
	for {
		n, err := logStream.Read(buf)
		if n > 0 {
			logBuffer.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	return logBuffer.String(), nil
}
