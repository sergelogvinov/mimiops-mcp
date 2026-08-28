/*
Copyright 2026 Serge Logvinov.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tools

import (
	"context"
	"fmt"
	"maps"
	"math/rand"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobsCreateResult represents the result of creating a Job.
type JobsCreateResult struct {
	JobSummary
}

// RegisterJobsCreate adds the jobs_create tool, which creates a one-off Job from a CronJob's job template.
func RegisterJobsCreate(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Create Job from CronJob"),
		mcp.WithDescription("Create a one-off Job from a CronJob's job template (CLI equivalent: kubectl create job --from=cronjob/<name>)"),
		mcp.WithString("cronjob", mcp.Description("CronJob name to source the template from"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("job_name", mcp.Description("Job name (optional, default: <cronjob>-manual-<random4>)")),
		mcp.WithOutputSchema[JobsCreateResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("jobs_create", opts...)
	s.AddTool(tool, handlerJobsCreate(mc))
}

// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=create

// handlerJobsCreate returns a handler function for the jobs_create tool.
func handlerJobsCreate(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		cronjobName := req.GetString("cronjob", "")
		if cronjobName == "" {
			return mcp.NewToolResultError("missing required parameter 'cronjob'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		jobName := req.GetString("job_name", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "jobs_create called",
			"cluster", client.ClusterName,
			"cronjob", cronjobName,
			"namespace", namespace,
			"job_name", jobName,
		)

		// Get the CronJob
		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, cronjobName, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found", cronjobName, namespace), nil
			}
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

		result := JobsCreateResult{
			JobSummary: toJobSummary(job),
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
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

	// Create the Job
	return client.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
}
