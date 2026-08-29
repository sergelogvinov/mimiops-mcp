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
	"fmt"

	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
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
		Age:         age.FormatAge(job.CreationTimestamp),
		Status:      deriveJobStatus(job),
	}
}

func toJobSpec(job *batchv1.Job) JobSpec {
	spec := JobSpec{
		PodSpec: PodSpec{
			RestartPolicy:     string(job.Spec.Template.Spec.RestartPolicy),
			ServiceAccount:    job.Spec.Template.Spec.ServiceAccountName,
			PriorityClassName: job.Spec.Template.Spec.PriorityClassName,
			InitContainers:    toContainerInfoList(job.Spec.Template.Spec.InitContainers),
			Containers:        toContainerInfoList(job.Spec.Template.Spec.Containers),
			Volumes:           extractVolumeNames("", job.Spec.Template.Spec.Volumes),
		},
	}

	if job.Spec.Parallelism != nil {
		spec.Parallelism = fmt.Sprintf("%d", *job.Spec.Parallelism)
	}
	if job.Spec.BackoffLimit != nil {
		spec.BackoffLimit = fmt.Sprintf("%d", *job.Spec.BackoffLimit)
	}
	if job.Spec.ActiveDeadlineSeconds != nil {
		spec.ActiveDeadlineSeconds = fmt.Sprintf("%d", *job.Spec.ActiveDeadlineSeconds)
	}

	return spec
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
