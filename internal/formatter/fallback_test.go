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

package formatter_test

import (
	"testing"

	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/stretchr/testify/assert"
)

type OwnerReference struct {
	APIVersion string `json:"apiVersion" jsonschema:"apiVersion"`
	Kind       string `json:"kind" jsonschema:"kind"`
	Name       string `json:"name" jsonschema:"name"`
}

type Replicas struct {
	Ready   int `json:"ready" jsonschema:"ready"`
	Desired int `json:"desired" jsonschema:"desired"`
}

type PodSummary struct {
	PodSpec

	Namespace       string            `json:"namespace" jsonschema:"namespace"`
	Name            string            `json:"name" jsonschema:"name"`
	Labels          map[string]string `json:"labels,omitempty" jsonschema:"labels"`
	Ready           string            `json:"ready" jsonschema:"ready"`
	Restarts        int32             `json:"restarts" jsonschema:"restarts"`
	OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty" jsonschema:"ownerReferences"`
}

type PodSpec struct {
	RestartPolicy     string   `json:"restart_policy,omitempty" jsonschema:"restart policy"`
	ServiceAccount    string   `json:"service_account,omitempty" jsonschema:"ServiceAccount"`
	PriorityClassName string   `json:"priority_class_name,omitempty" jsonschema:"PriorityClassName"`
	Volumes           []string `json:"volumes,omitempty" jsonschema:"List of volumes names"`
}

func TestToMarkdown(t *testing.T) {
	for _, tt := range []struct {
		name           string
		input          any
		expectedOutput string
	}{
		{
			name:           "example test",
			input:          nil,
			expectedOutput: "",
		},
		{
			name: "simple struct",
			input: PodSummary{
				Namespace: "default",
				Name:      "my-pod",
				Ready:     "1/1",
				Restarts:  0,
			},
			expectedOutput: `namespace: default

name: my-pod

ready: 1/1

restarts: 0

`,
		},
		{
			name: "struct with labels",
			input: PodSummary{
				Namespace: "default",
				Name:      "my-pod",
				Labels: map[string]string{
					"app": "my-app",
				},
				Ready:    "1/1",
				Restarts: 0,
			},
			expectedOutput: `namespace: default

name: my-pod

labels: app=my-app

ready: 1/1

restarts: 0

`,
		},
		{
			name: "struct with owner references",
			input: PodSummary{
				Namespace: "default",
				Name:      "my-pod",
				Ready:     "1/1",
				Restarts:  0,
				OwnerReferences: []OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "Deployment",
						Name:       "my-deployment",
					},
				},
			},
			expectedOutput: `namespace: default

name: my-pod

ready: 1/1

restarts: 0

ownerReferences:
| apiVersion | kind | name |
| --- | --- | --- |
| v1 | Deployment | my-deployment |


`,
		},
		{
			name: "struct with pod spec",
			input: PodSummary{
				Namespace: "default",
				Name:      "my-pod",
				Ready:     "1/1",
				Restarts:  0,
				OwnerReferences: []OwnerReference{
					{
						APIVersion: "v1",
						Kind:       "Deployment",
						Name:       "my-deployment",
					},
				},
				PodSpec: PodSpec{
					RestartPolicy:     "Always",
					ServiceAccount:    "default",
					PriorityClassName: "high-priority",
					Volumes:           []string{"volume1", "volume2"},
				},
			},
			expectedOutput: `restart policy: Always

ServiceAccount: default

PriorityClassName: high-priority

List of volumes names: volume1, volume2

namespace: default

name: my-pod

ready: 1/1

restarts: 0

ownerReferences:
| apiVersion | kind | name |
| --- | --- | --- |
| v1 | Deployment | my-deployment |


`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := formatter.ToMarkdown(tt.input)

			assert.EqualValues(t, tt.expectedOutput, res)
		})
	}
}
