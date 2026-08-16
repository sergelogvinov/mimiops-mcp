package tools

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterJobsDescribe adds the jobs_describe tool, which provides a human-readable Job summary.
func RegisterJobsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_describe",
		mcp.WithDescription("Human-readable Job summary (conditions, parallelism, completions, backoff, pod selector, active pods)."),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
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

		log.DebugContext(ctx, "jobs_describe called",
			"namespace", namespace,
			"job", name,
		)

		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := formatJobDescribeWithPods(ctx, job, format, client, namespace)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatJobDescribeWithPods formats a Job's detailed information with active pods.
func formatJobDescribeWithPods(ctx context.Context, job *batchv1.Job, format string, client *k8s.Client, namespace string) (string, error) {
	if format == "json" {
		return formatJobDescribeJSON(job)
	}
	return formatJobDescribeTextWithPods(ctx, job, client, namespace), nil
}

// formatJobDescribeTextWithPods formats a Job's detailed information with active pods.
func formatJobDescribeTextWithPods(ctx context.Context, job *batchv1.Job, client *k8s.Client, namespace string) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", job.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", job.Namespace)
	fmt.Fprintf(&buf, "**Status:** %s\n", deriveJobStatus(*job))
	fmt.Fprintf(&buf, "**Completions:** %d/%d\n", job.Status.Succeeded, *job.Spec.Completions)
	fmt.Fprintf(&buf, "**Parallelism:** %d\n", *job.Spec.Parallelism)
	fmt.Fprintf(&buf, "**Backoff Limit:** %d\n", *job.Spec.BackoffLimit)
	fmt.Fprintf(&buf, "**Age:** %s\n", formatAge(job.CreationTimestamp))

	if job.Status.StartTime != nil {
		fmt.Fprintf(&buf, "**Start Time:** %s\n", job.Status.StartTime.String())
	}
	if job.Status.CompletionTime != nil {
		fmt.Fprintf(&buf, "**Completion Time:** %s\n", job.Status.CompletionTime.String())
	}

	// Conditions
	if len(job.Status.Conditions) > 0 {
		fmt.Fprintf(&buf, "\n### Conditions\n\n")
		for _, cond := range job.Status.Conditions {
			fmt.Fprintf(&buf, "- **%s**: %s (%s)\n", cond.Type, cond.Status, cond.Reason)
		}
	}

	// Active Pods (capped at 5)
	fmt.Fprintf(&buf, "\n### Active Pods\n\n")
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(&buf, "Failed to list pods: %v\n", err)
	} else {
		// Filter pods by Job owner reference
		var ownedPods []corev1.Pod
		for _, pod := range pods.Items {
			if metav1.GetControllerOf(&pod) != nil && metav1.GetControllerOf(&pod).UID == job.UID {
				ownedPods = append(ownedPods, pod)
			}
		}

		if len(ownedPods) == 0 {
			fmt.Fprintf(&buf, "No active pods.\n")
		} else {
			// Limit to 5 pods
			maxPods := 5
			if len(ownedPods) > maxPods {
				maxPods = 5
			}
			for _, pod := range ownedPods[:maxPods] {
				podStatus := string(pod.Status.Phase)
				if pod.Status.Phase == corev1.PodRunning {
					podStatus = fmt.Sprintf("Running (%d/%d)", countReadyContainers(pod.Status), len(pod.Status.ContainerStatuses))
				}
				fmt.Fprintf(&buf, "- %s: %s\n", pod.Name, podStatus)
			}
			if len(ownedPods) > maxPods {
				fmt.Fprintf(&buf, "... and %d more\n", len(ownedPods)-maxPods)
			}
		}
	}

	// Pod Template
	fmt.Fprintf(&buf, "\n### Pod Template\n\n")
	template := job.Spec.Template
	fmt.Fprintf(&buf, "- **Labels:** %v\n", template.Labels)
	fmt.Fprintf(&buf, "- **Restart Policy:** %s\n", template.Spec.RestartPolicy)

	if template.Spec.ActiveDeadlineSeconds != nil {
		fmt.Fprintf(&buf, "- **Active Deadline Seconds:** %d\n", *template.Spec.ActiveDeadlineSeconds)
	}

	fmt.Fprintf(&buf, "\n### Containers\n\n")
	for _, container := range template.Spec.Containers {
		fmt.Fprintf(&buf, "- **%s**: image=%s\n", container.Name, container.Image)
	}

	return buf.String()
}

// countReadyContainers counts the number of ready containers in a pod.
func countReadyContainers(status corev1.PodStatus) int {
	ready := 0
	for _, cs := range status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return ready
}
