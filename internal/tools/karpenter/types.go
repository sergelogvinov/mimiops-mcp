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

package toolskarpenter

// refs: https://github.com/kubernetes-sigs/karpenter/blob/main/pkg/apis/v1/nodepool.go

// NodePoolSummary is the trimmed representation of a Karpenter NodePool
// (karpenter.sh/v1) used by karpenter_nodepools_list.
type NodePoolSummary struct {
	Name        string `json:"name" jsonschema:"Name"`
	NodeClass   string `json:"nodeClass" jsonschema:"NodeClass referenced by the NodePool"`
	Nodes       int    `json:"nodes" jsonschema:"Number of nodes owned by the NodePool"`
	Ready       bool   `json:"ready" jsonschema:"Whether the NodePool is ready"`
	Age         string `json:"age" jsonschema:"Age of the NodePool"`
	Weight      int32  `json:"weight" jsonschema:"Scheduling weight of the NodePool"`
	CPUUsage    string `json:"cpuUsage,omitempty" jsonschema:"Total CPU provisioned by the NodePool (status.resources)"`
	CPULimit    string `json:"cpuLimit,omitempty" jsonschema:"CPU provisioning limit (spec.limits)"`
	MemoryUsage string `json:"memoryUsage,omitempty" jsonschema:"Total memory provisioned by the NodePool (status.resources)"`
	MemoryLimit string `json:"memoryLimit,omitempty" jsonschema:"Memory provisioning limit (spec.limits)"`
}
