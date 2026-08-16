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

// RegisterCronJobsGet adds the cronjobs_get tool, which gets a single CronJob's full spec and status.
func RegisterCronJobsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("cronjobs_get",
		mcp.WithDescription("Get a single CronJob's full spec and status."),
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

		log.DebugContext(ctx, "cronjobs_get called",
			"namespace", namespace,
			"cronjob", name,
		)

		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := formatCronJobDescribe(cronJob, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatCronJobDescribe formats a CronJob's detailed information.
func formatCronJobDescribe(cj *batchv1.CronJob, format string) (string, error) {
	if format == "json" {
		return formatCronJobDescribeJSON(cj)
	}
	return formatCronJobDescribeText(cj), nil
}

// formatCronJobDescribeText formats a CronJob's detailed information as key-value blocks.
func formatCronJobDescribeText(cj *batchv1.CronJob) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "**Name:** %s\n", cj.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", cj.Namespace)
	fmt.Fprintf(&buf, "**Schedule:** %s\n", cj.Spec.Schedule)
	fmt.Fprintf(&buf, "**Suspend:** %v\n", cj.Spec.Suspend != nil && *cj.Spec.Suspend)
	fmt.Fprintf(&buf, "**Concurrency Policy:** %s\n", cj.Spec.ConcurrencyPolicy)
	fmt.Fprintf(&buf, "**Start Deadline Seconds:** %v\n", cj.Spec.StartingDeadlineSeconds)
	fmt.Fprintf(&buf, "**Successful Jobs History Limit:** %v\n", cj.Spec.SuccessfulJobsHistoryLimit)
	fmt.Fprintf(&buf, "**Failed Jobs History Limit:** %v\n", cj.Spec.FailedJobsHistoryLimit)
	fmt.Fprintf(&buf, "**Age:** %s\n", formatAge(cj.CreationTimestamp))

	if cj.Status.LastScheduleTime != nil {
		fmt.Fprintf(&buf, "**Last Schedule Time:** %s\n", cj.Status.LastScheduleTime.String())
	}

	// Job Template
	fmt.Fprintf(&buf, "\n### Job Template\n\n")
	jobSpec := cj.Spec.JobTemplate.Spec
	fmt.Fprintf(&buf, "- **Completions:** %d\n", jobSpec.Completions)
	fmt.Fprintf(&buf, "- **Parallelism:** %d\n", jobSpec.Parallelism)
	fmt.Fprintf(&buf, "- **Backoff Limit:** %d\n", jobSpec.BackoffLimit)

	if jobSpec.ActiveDeadlineSeconds != nil {
		fmt.Fprintf(&buf, "- **Active Deadline Seconds:** %d\n", *jobSpec.ActiveDeadlineSeconds)
	}

	fmt.Fprintf(&buf, "\n### Job Template Containers\n\n")
	for _, container := range jobSpec.Template.Spec.Containers {
		restartPolicy := "Always"
		if container.RestartPolicy != nil {
			restartPolicy = string(*container.RestartPolicy)
		}
		fmt.Fprintf(&buf, "- **%s**: image=%s, restartPolicy=%s\n", container.Name, container.Image, restartPolicy)
	}

	return buf.String()
}

// formatCronJobDescribeJSON formats a CronJob as JSON.
func formatCronJobDescribeJSON(cj *batchv1.CronJob) (string, error) {
	type CronJobInfo struct {
		Metadata map[string]any `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
		Summary  CronJobSummary `json:"summary"`
	}

	info := CronJobInfo{}

	// Metadata
	info.Metadata = make(map[string]any)
	info.Metadata["name"] = cj.Name
	info.Metadata["namespace"] = cj.Namespace
	info.Metadata["uid"] = string(cj.UID)
	info.Metadata["creationTimestamp"] = cj.CreationTimestamp.String()
	info.Metadata["labels"] = cj.Labels
	info.Metadata["annotations"] = cj.Annotations

	// Spec (simplified)
	info.Spec = make(map[string]any)
	info.Spec["schedule"] = cj.Spec.Schedule
	info.Spec["suspend"] = cj.Spec.Suspend
	info.Spec["concurrencyPolicy"] = cj.Spec.ConcurrencyPolicy
	info.Spec["startingDeadlineSeconds"] = cj.Spec.StartingDeadlineSeconds
	info.Spec["successfulJobsHistoryLimit"] = cj.Spec.SuccessfulJobsHistoryLimit
	info.Spec["failedJobsHistoryLimit"] = cj.Spec.FailedJobsHistoryLimit
	info.Spec["jobTemplate"] = cj.Spec.JobTemplate

	// Status (simplified)
	info.Status = make(map[string]any)
	info.Status["lastScheduleTime"] = cj.Status.LastScheduleTime
	info.Status["active"] = cj.Status.Active

	// Summary
	info.Summary = toCronJobSummary(*cj)

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
