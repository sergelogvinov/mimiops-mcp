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

	batchv1 "k8s.io/api/batch/v1"
)

// toCronJobSummary converts a CronJob to a CronJobSummary.
func toCronJobSummary(cj *batchv1.CronJob) CronJobSummary {
	suspend := false
	if cj.Spec.Suspend != nil {
		suspend = *cj.Spec.Suspend
	}

	lastSchedule := ""
	if cj.Status.LastScheduleTime != nil {
		lastSchedule = formatAgeMin(*cj.Status.LastScheduleTime)
	}

	return CronJobSummary{
		Namespace:    cj.Namespace,
		Name:         cj.Name,
		Schedule:     cj.Spec.Schedule,
		Suspend:      suspend,
		LastSchedule: lastSchedule,
		Age:          formatAge(cj.CreationTimestamp),
		Status:       deriveCronJobStatus(cj),
	}
}

func toCronJobSpec(cj *batchv1.CronJob) CronJobSpec {
	spec := CronJobSpec{
		ConcurrencyPolicy: string(cj.Spec.ConcurrencyPolicy),
		Selector:          formatMatchLabels(cj.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels),
		PodSpec: PodSpec{
			RestartPolicy:     string(cj.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy),
			ServiceAccount:    cj.Spec.JobTemplate.Spec.Template.Spec.ServiceAccountName,
			PriorityClassName: cj.Spec.JobTemplate.Spec.Template.Spec.PriorityClassName,
			InitContainers:    toContainerInfoList(cj.Spec.JobTemplate.Spec.Template.Spec.InitContainers),
			Containers:        toContainerInfoList(cj.Spec.JobTemplate.Spec.Template.Spec.Containers),
			Volumes:           extractVolumeNames(cj.Spec.JobTemplate.Spec.Template.Spec.Volumes),
		},
	}

	if cj.Spec.StartingDeadlineSeconds != nil {
		spec.StartingDeadlineSeconds = fmt.Sprintf("%d", *cj.Spec.StartingDeadlineSeconds)
	}
	if cj.Spec.SuccessfulJobsHistoryLimit != nil {
		spec.SuccessfulJobsHistoryLimit = fmt.Sprintf("%d", *cj.Spec.SuccessfulJobsHistoryLimit)
	}
	if cj.Spec.FailedJobsHistoryLimit != nil {
		spec.FailedJobsHistoryLimit = fmt.Sprintf("%d", *cj.Spec.FailedJobsHistoryLimit)
	}

	return spec
}

// deriveCronJobStatus derives the status string for a CronJob.
func deriveCronJobStatus(cj *batchv1.CronJob) string {
	if cj.Spec.Suspend != nil && *cj.Spec.Suspend {
		// Check for active jobs
		if len(cj.Status.Active) == 0 {
			return "Suspended"
		}
		return fmt.Sprintf("Suspended (%d/%d)", len(cj.Status.Active), len(cj.Status.Active))
	}
	return "Active"
}
