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
	Namespace       string            `json:"namespace" jsonschema:"namespace"`
	Name            string            `json:"name" jsonschema:"name"`
	Labels          map[string]string `json:"labels,omitempty" jsonschema:"labels"`
	Ready           string            `json:"ready" jsonschema:"ready"`
	Restarts        int32             `json:"restarts" jsonschema:"restarts"`
	OwnerReferences []OwnerReference  `json:"ownerReferences,omitempty" jsonschema:"ownerReferences"`
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
			expectedOutput: `- **namespace**: default
- **name**: my-pod
- **ready**: 1/1
- **restarts**: 0
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
			expectedOutput: `- **namespace**: default
- **name**: my-pod
- **labels**: app=my-app
- **ready**: 1/1
- **restarts**: 0
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
			expectedOutput: `- **namespace**: default
- **name**: my-pod
- **ready**: 1/1
- **restarts**: 0
- **ownerReferences**: 
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
