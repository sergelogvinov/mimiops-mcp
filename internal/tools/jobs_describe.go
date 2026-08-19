package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobDescribeResult represents the result of describing a Job.
type JobDescribeResult struct {
	JobSummary

	Labels      map[string]string `json:"labels" jsonschema:"labels of the Job"`
	Annotations map[string]string `json:"annotations" jsonschema:"annotations of the Job"`
	Spec        map[string]any    `json:"spec" jsonschema:"Spec of the Job"`

	// Containers []ContainerInfo `json:"containers" jsonschema:"List of containers in the Job's pod template"`
	Conditions []ConditionInfo `json:"conditions" jsonschema:"List of conditions of the Job"`
	Pods       []PodInfo       `json:"pods" jsonschema:"List of pods owned by the Job"`
}

// RegisterJobsDescribe adds the jobs_describe tool, which provides a structured Job summary.
func RegisterJobsDescribe(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_describe",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Job"),
		mcp.WithDescription("Job summary (conditions, parallelism, completions, backoff, active pods list)."),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[JobDescribeResult](),
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

		log.DebugContext(ctx, "jobs_describe called",
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

		result, err := buildJobDescribeResult(ctx, job, client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to build result: %v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	})
}

// buildJobDescribeResult builds a JobDescribeResult from a Job.
func buildJobDescribeResult(ctx context.Context, job *batchv1.Job, client *k8s.Client) (*JobDescribeResult, error) {
	result := &JobDescribeResult{
		JobSummary:  toJobSummary(job),
		Annotations: extractAnnotations(job.Annotations),
		Labels:      extractLabels(job.Labels),
		Spec:        make(map[string]any),
	}

	// Conditions
	result.Conditions = make([]ConditionInfo, 0, len(job.Status.Conditions))
	for _, cond := range job.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// Pods - list pods by Job owner reference
	pods, err := client.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Filter pods by Job owner reference
	var ownedPods []corev1.Pod
	for _, pod := range pods.Items {
		if metav1.GetControllerOf(&pod) != nil && metav1.GetControllerOf(&pod).UID == job.UID {
			ownedPods = append(ownedPods, pod)
		}
	}

	result.Pods = make([]PodInfo, 0, len(ownedPods))
	for _, pod := range ownedPods {
		podInfo := PodInfo{
			Name:  pod.Name,
			Phase: string(pod.Status.Phase),
		}

		// Calculate ready status
		readyCount := 0
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready {
				readyCount++
			}
		}
		podInfo.Ready = fmt.Sprintf("%d/%d", readyCount, len(pod.Status.ContainerStatuses))

		if pod.Status.StartTime != nil {
			podInfo.StartTime = pod.Status.StartTime.String()
		}

		result.Pods = append(result.Pods, podInfo)
	}

	return result, nil
}
