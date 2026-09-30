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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		name string
		q    resource.Quantity
		want string
	}{
		{name: "milli-quantity rounds to bytes", q: *resource.NewMilliQuantity(524288123456, resource.DecimalSI), want: "524288124"},
		{name: "binary quantity", q: resource.MustParse("512Mi"), want: "512Mi"},
		{name: "zero", q: resource.Quantity{}, want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatMemory(tt.q))
		})
	}
}

func TestFormatWindow(t *testing.T) {
	assert.Equal(t, "30m", formatWindow(30*time.Minute))
	assert.Equal(t, "1h", formatWindow(time.Hour))
	assert.Equal(t, "90s", formatWindow(90*time.Second))
}

func TestFormatLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector *metav1.LabelSelector
		want     string
	}{
		{name: "nil", selector: nil, want: ""},
		{name: "match labels sorted", selector: &metav1.LabelSelector{MatchLabels: map[string]string{"tier": "api", "app": "web"}}, want: "app=web,tier=api"},
		{
			name: "match expressions only",
			selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{"web"}},
			}},
			want: "app in (web)",
		},
		{
			name: "invalid operator",
			selector: &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "app", Operator: "Bogus"},
			}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatLabelSelector(tt.selector))
		})
	}
}

func TestSumContainerUsage(t *testing.T) {
	containers := []metricsv1beta1.ContainerMetrics{
		{Name: "app", Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("128Mi")}},
		{Name: "sidecar", Usage: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")}},
	}

	usage := sumContainerUsage(containers, true)
	assert.Equal(t, "300m", usage.CPU)
	assert.Equal(t, "160Mi", usage.Memory)
	assert.Equal(t, []ContainerUsage{{Name: "app", CPU: "250m", Memory: "128Mi"}, {Name: "sidecar", CPU: "50m", Memory: "32Mi"}}, usage.Containers)

	assert.Nil(t, sumContainerUsage(containers, false).Containers)
}
