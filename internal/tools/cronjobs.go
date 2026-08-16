package tools

import (
	"bytes"
	"encoding/json"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
)

// formatCronJobSuspendResume formats the result of suspend/resume operations.
func formatCronJobSuspendResume(cj *batchv1.CronJob, suspended bool, format string) (string, error) {
	if format == "json" {
		return formatCronJobSuspendResumeJSON(cj, suspended)
	}
	return formatCronJobSuspendResumeText(cj, suspended), nil
}

// formatCronJobSuspendResumeText formats suspend/resume result as text.
func formatCronJobSuspendResumeText(cj *batchv1.CronJob, suspended bool) string {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "CronJob '%s' in namespace '%s' has been %s.\n", cj.Name, cj.Namespace, map[bool]string{true: "suspended", false: "resumed"}[suspended])
	fmt.Fprintf(&buf, "**Name:** %s\n", cj.Name)
	fmt.Fprintf(&buf, "**Namespace:** %s\n", cj.Namespace)
	fmt.Fprintf(&buf, "**Schedule:** %s\n", cj.Spec.Schedule)
	fmt.Fprintf(&buf, "**Suspend:** %v\n", suspended)

	return buf.String()
}

// formatCronJobSuspendResumeJSON formats suspend/resume result as JSON.
func formatCronJobSuspendResumeJSON(cj *batchv1.CronJob, suspended bool) (string, error) {
	result := struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Schedule  string `json:"schedule"`
		Suspended bool   `json:"suspended"`
	}{
		Name:      cj.Name,
		Namespace: cj.Namespace,
		Schedule:  cj.Spec.Schedule,
		Suspended: suspended,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
