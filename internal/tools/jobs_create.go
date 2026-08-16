package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math/rand"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterJobsCreate adds the jobs_create tool, which creates a one-off Job from a CronJob's job template.
func RegisterJobsCreate(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("jobs_create",
		mcp.WithDescription("Create a one-off Job from a CronJob's job template (CLI equivalent: kubectl create job --from=cronjob/<name>)."),
		mcp.WithString("cronjob", mcp.Description("CronJob name to source the template from"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("job_name", mcp.Description("Job name (optional, default: <cronjob>-manual-<random4>)")),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cronjobName := req.GetString("cronjob", "")
		if cronjobName == "" {
			return mcp.NewToolResultError("missing required parameter 'cronjob'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		jobName := req.GetString("job_name", "")
		format := req.GetString("format", "text")
		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "jobs_create called",
			"cronjob", cronjobName,
			"namespace", namespace,
			"job_name", jobName,
			"format", format,
		)

		// Get the CronJob
		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, cronjobName, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", cronjobName, namespace, err), nil
		}

		// Generate or use provided job name
		finalJobName := jobName
		if finalJobName == "" {
			finalJobName = generateJobName(cronjobName)
		}

		// Create the Job
		job, err := createJobFromCronJob(ctx, client, namespace, finalJobName, cronJob)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to create Job '%s': %v", finalJobName, err), nil
		}

		result, err := formatJobCreate(job, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// generateJobName generates a job name in the format <cronjob>-manual-<random4>.
func generateJobName(cronjobName string) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		suffix[i] = letters[rand.Intn(len(letters))]
	}
	return fmt.Sprintf("%s-manual-%s", cronjobName, string(suffix))
}

// createJobFromCronJob creates a Job from a CronJob's template.
func createJobFromCronJob(ctx context.Context, client *k8s.Client, namespace, jobName string, cronJob *batchv1.CronJob) (*batchv1.Job, error) {
	// Copy the job template spec
	jobTemplate := cronJob.Spec.JobTemplate
	jobSpec := jobTemplate.Spec

	// Create a new Job
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: jobSpec,
	}

	// Copy labels and annotations from CronJob
	maps.Copy(job.Labels, cronJob.Labels)
	maps.Copy(job.Annotations, cronJob.Annotations)

	// Add a label to identify this as a manual run
	job.Labels["mimiops-manual-run"] = "true"

	// Generate unique labels for the pod template to avoid adoption by the CronJob
	job.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"job-name": job.Name,
		},
	}

	// Update the pod template labels
	if job.Spec.Template.Labels == nil {
		job.Spec.Template.Labels = make(map[string]string)
	}
	job.Spec.Template.Labels["job-name"] = job.Name

	// Clear the suspend field (a manual run executes regardless of the CronJob's suspend state)
	// Note: Job doesn't have a suspend field, but we ensure the template is used as-is

	// Create the Job
	return client.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
}

// formatJobCreate formats the result of job creation.
func formatJobCreate(job *batchv1.Job, format string) (string, error) {
	if format == "json" {
		return formatJobCreateJSON(job)
	}
	return formatJobCreateText(job), nil
}

// formatJobCreateText formats job creation result as text.
func formatJobCreateText(job *batchv1.Job) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "Job '%s' in namespace '%s' has been created successfully.\n", job.Name, job.Namespace)
	fmt.Fprintf(&buf, "**Name:** %s\n", job.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", job.Namespace)
	fmt.Fprintf(&buf, "**Completions:** %d/%d\n", 0, *job.Spec.Completions)
	fmt.Fprintf(&buf, "**Parallelism:** %d\n", *job.Spec.Parallelism)
	fmt.Fprintf(&buf, "**Backoff Limit:** %d\n", *job.Spec.BackoffLimit)

	return buf.String()
}

// formatJobCreateJSON formats job creation result as JSON.
func formatJobCreateJSON(job *batchv1.Job) (string, error) {
	result := struct {
		Name         string `json:"name"`
		Namespace    string `json:"namespace"`
		Completions  int32  `json:"completions"`
		Parallelism  int32  `json:"parallelism"`
		BackoffLimit int32  `json:"backoffLimit"`
	}{
		Name:         job.Name,
		Namespace:    job.Namespace,
		Completions:  *job.Spec.Completions,
		Parallelism:  *job.Spec.Parallelism,
		BackoffLimit: *job.Spec.BackoffLimit,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
