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

// Package age provides utility functions to calculate the age of Kubernetes resources based on their timestamps.
package age

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FormatAge calculates the age from creation time.
func FormatAge(created metav1.Time) string {
	diff := time.Since(created.Time)

	switch {
	case diff < time.Minute:
		return "0s"
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

// FormatAgeMin calculates the age from creation time with hours and minutes granularity.
func FormatAgeMin(created metav1.Time) string {
	diff := time.Since(created.Time)

	switch {
	case diff < time.Minute:
		return "0s"
	case diff < time.Hour:
		return fmt.Sprintf("%dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		minutes := int(diff.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	default:
		return fmt.Sprintf("%dd", int(diff.Hours()/24))
	}
}

// FormatEventAge formats the age of a Kubernetes event, including the count and first seen timestamp.
func FormatEventAge(event corev1.Event) string {
	firstSeen := ""
	if !event.FirstTimestamp.IsZero() {
		firstSeen = FormatAge(event.FirstTimestamp)
	}

	age := ""
	if !event.LastTimestamp.IsZero() {
		age = FormatAge(event.LastTimestamp)
	}

	return fmt.Sprintf("%s (x%d over %s)", age, event.Count, firstSeen)
}
