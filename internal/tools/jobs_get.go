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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterJobsGet adds the jobs_get tool, which gets a single Job's full spec and status.
func RegisterJobsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_get",
		mcp.WithDescription("Get a single Job's full spec and status."),
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

		log.DebugContext(ctx, "jobs_get called",
			"namespace", namespace,
			"job", name,
		)

		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := formatJobDescribe(job, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatJobDescribe formats a Job's detailed information.
func formatJobDescribe(job *batchv1.Job, format string) (string, error) {
	if format == "json" {
		return formatJobDescribeJSON(job)
	}
	return formatJobDescribeText(job), nil
}

// formatJobDescribeText formats a Job's detailed information as key-value blocks.
func formatJobDescribeText(job *batchv1.Job) string {
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

// formatJobDescribeJSON formats a Job as JSON.
func formatJobDescribeJSON(job *batchv1.Job) (string, error) {
	type JobInfo struct {
		Metadata map[string]any `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
		Summary  JobSummary     `json:"summary"`
	}

	info := JobInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = job.Name
	info.Metadata["namespace"] = job.Namespace
	info.Metadata["uid"] = string(job.UID)
	info.Metadata["creationTimestamp"] = job.CreationTimestamp.String()
	info.Metadata["labels"] = job.Labels
	info.Metadata["annotations"] = job.Annotations

	// Spec (simplified)
	info.Spec = make(map[string]any)
	info.Spec["completions"] = job.Spec.Completions
	info.Spec["parallelism"] = job.Spec.Parallelism
	info.Spec["backoffLimit"] = job.Spec.BackoffLimit
	info.Spec["activeDeadlineSeconds"] = job.Spec.ActiveDeadlineSeconds
	info.Spec["template"] = job.Spec.Template

	// Status (simplified)
	info.Status = make(map[string]any)
	info.Status["succeeded"] = job.Status.Succeeded
	info.Status["failed"] = job.Status.Failed
	info.Status["active"] = job.Status.Active
	info.Status["startTime"] = job.Status.StartTime
	info.Status["completionTime"] = job.Status.CompletionTime
	info.Status["conditions"] = job.Status.Conditions

	// Summary
	info.Summary = toJobSummary(*job)

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
