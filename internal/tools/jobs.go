package tools

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
)

// toJobSummary converts a Job to a JobSummary.
func toJobSummary(job *batchv1.Job) JobSummary {
	completions := fmt.Sprintf("%d/1", job.Status.Succeeded)
	if job.Spec.Completions != nil {
		completions = fmt.Sprintf("%d/%d", job.Status.Succeeded, *job.Spec.Completions)
	}

	duration := ""
	if job.Status.StartTime != nil && job.Status.CompletionTime != nil {
		duration = formatDuration(*job.Status.CompletionTime, *job.Status.StartTime)
	}

	return JobSummary{
		Namespace:   job.Namespace,
		Name:        job.Name,
		Completions: completions,
		Duration:    duration,
		Age:         formatAge(job.CreationTimestamp),
		Status:      deriveJobStatus(job),
	}
}

// deriveJobStatus derives the status string for a Job.
func deriveJobStatus(job *batchv1.Job) string {
	if job.Spec.Completions != nil && job.Status.Succeeded > 0 && job.Status.Succeeded == *job.Spec.Completions {
		return "Complete"
	}
	if job.Status.Failed > 0 {
		return "Failed"
	}
	if job.Status.Active > 0 {
		return "Running"
	}
	return "Pending"
}
