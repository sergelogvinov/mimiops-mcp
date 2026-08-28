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

package toolshelm

import (
	"fmt"
	"time"

	v1 "helm.sh/helm/v4/pkg/release/v1"
)

func toHelmSummary(r *v1.Release) ReleaseSummary {
	return ReleaseSummary{
		Name:         r.Name,
		Namespace:    r.Namespace,
		Revision:     r.Version,
		Updated:      r.Info.LastDeployed.String(),
		Status:       r.Info.Status.String(),
		Description:  r.Info.Description,
		Age:          formatAge(r.Info.LastDeployed),
		ChartName:    r.Chart.Metadata.Name,
		ChartVersion: r.Chart.Metadata.Version,
		AppVersion:   r.Chart.Metadata.AppVersion,
	}
}

func formatAge(created time.Time) string {
	now := time.Now()
	diff := now.Sub(created)

	if diff < time.Minute {
		return "0s"
	}
	if diff < time.Hour {
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	}
	if diff < 24*time.Hour {
		return fmt.Sprintf("%dh", int(diff.Hours()))
	}
	return fmt.Sprintf("%dd", int(diff.Hours()/24))
}
