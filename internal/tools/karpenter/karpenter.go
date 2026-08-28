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

import (
	"context"
	"fmt"
	"strings"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	// nodePoolLabel is the label Karpenter sets on nodes owned by a NodePool.
	nodePoolLabel = "karpenter.sh/nodepool"

	// defaultNodePoolWeight is the weight Karpenter applies when spec.weight
	// is unset (the CRD schema default).
	defaultNodePoolWeight = int32(50)
)

// nodePoolGVR identifies the Karpenter NodePool resource (karpenter.sh/v1).
var nodePoolGVR = schema.GroupVersionResource{
	Group:    "karpenter.sh",
	Version:  "v1",
	Resource: "nodepools",
}

// Client wraps a dynamic client for Karpenter CRDs plus the core client used
// to count the nodes owned by NodePools.
type Client struct {
	dynamic dynamic.Interface
	core    kubernetes.Interface
}

// NewKarpenterClient creates a Karpenter client from the resolved identity of
// the given k8s.Client, so authentication, impersonation, and per-cluster
// targeting stay consistent with every other tool.
func NewKarpenterClient(kclient *k8s.Client) (*Client, error) {
	dyn, err := dynamic.NewForConfig(kclient.RESTConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{dynamic: dyn, core: kclient}, nil
}

// ListNodePools lists NodePool resources, optionally filtered by label/field
// selectors, and enriches each with the number of nodes Karpenter manages for
// it. CPU/memory usage is the total provisioned by the pool (status.resources)
// and limits come from spec.limits.
func (c *Client) ListNodePools(ctx context.Context, labelSelector, fieldSelector string) ([]NodePoolSummary, error) {
	list, err := c.dynamic.Resource(nodePoolGVR).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	})
	if err != nil {
		return nil, listError("NodePool", err)
	}

	summaries := make([]NodePoolSummary, 0, len(list.Items))
	if len(list.Items) == 0 {
		return summaries, nil
	}

	nodeCounts, err := c.nodeCounts(ctx)
	if err != nil {
		return nil, err
	}

	for i := range list.Items {
		summaries = append(summaries, nodePoolSummary(&list.Items[i], nodeCounts))
	}
	return summaries, nil
}

// nodeCounts counts nodes per NodePool using the karpenter.sh/nodepool label.
func (c *Client) nodeCounts(ctx context.Context) (map[string]int, error) {
	nodes, err := c.core.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: nodePoolLabel})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	counts := make(map[string]int)
	for _, node := range nodes.Items {
		if pool := node.Labels[nodePoolLabel]; pool != "" {
			counts[pool]++
		}
	}
	return counts, nil
}

// nodePoolSummary converts an unstructured NodePool into a NodePoolSummary.
func nodePoolSummary(obj *unstructured.Unstructured, nodeCounts map[string]int) NodePoolSummary {
	summary := NodePoolSummary{
		Name:   obj.GetName(),
		Nodes:  nodeCounts[obj.GetName()],
		Ready:  readyCondition(obj.Object),
		Age:    age.FormatAge(obj.GetCreationTimestamp()),
		Weight: defaultNodePoolWeight,
	}

	if nodeClass, found := nestedString(obj.Object, "spec", "template", "spec", "nodeClassRef", "name"); found {
		summary.NodeClass = nodeClass
	}
	if weight, found := nestedInt64(obj.Object, "spec", "weight"); found {
		summary.Weight = int32(weight)
	}

	if resources := nestedMap(obj.Object, "status", "resources"); resources != nil {
		summary.CPUUsage = resourceValue(resources, corev1.ResourceCPU)
		summary.MemoryUsage = resourceValue(resources, corev1.ResourceMemory)
	}
	if limits := nestedMap(obj.Object, "spec", "limits"); limits != nil {
		summary.CPULimit = resourceValue(limits, corev1.ResourceCPU)
		summary.MemoryLimit = resourceValue(limits, corev1.ResourceMemory)
	}

	return summary
}

// readyCondition reports whether the Ready condition of the NodePool is True.
func readyCondition(obj map[string]any) bool {
	for _, cond := range nestedSlice(obj, "status", "conditions") {
		condition, ok := cond.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == "Ready" {
			return condition["status"] == "True"
		}
	}
	return false
}

// resourceValue extracts a canonical quantity string for the given resource
// from a ResourceList-shaped map such as status.resources or spec.limits.
func resourceValue(resources map[string]any, name corev1.ResourceName) string {
	raw, ok := resources[string(name)]
	if !ok {
		return ""
	}

	if s, ok := raw.(string); ok {
		if quantity, err := resource.ParseQuantity(s); err == nil {
			return quantity.String()
		}
		return s
	}

	return fmt.Sprintf("%v", raw)
}

// nestedString returns the string at the given path in an unstructured object.
func nestedString(obj map[string]any, fields ...string) (string, bool) {
	val, found := nestedField(obj, fields...)
	if !found {
		return "", false
	}
	s, ok := val.(string)
	return s, ok
}

// nestedInt64 returns the integer at the given path in an unstructured object.
func nestedInt64(obj map[string]any, fields ...string) (int64, bool) {
	val, found := nestedField(obj, fields...)
	if !found {
		return 0, false
	}
	switch v := val.(type) {
	case int64:
		return v, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// nestedMap returns the map at the given path in an unstructured object.
func nestedMap(obj map[string]any, fields ...string) map[string]any {
	val, found := nestedField(obj, fields...)
	if !found {
		return nil
	}
	m, ok := val.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

// nestedSlice returns the slice at the given path in an unstructured object.
func nestedSlice(obj map[string]any, fields ...string) []any {
	val, found := nestedField(obj, fields...)
	if !found {
		return nil
	}
	s, ok := val.([]any)
	if !ok {
		return nil
	}
	return s
}

// nestedField traverses an unstructured object and returns the value at the
// given path.
func nestedField(obj map[string]any, fields ...string) (any, bool) {
	var current any = obj

	for _, field := range fields {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[field]
		if !ok {
			return nil, false
		}
	}

	return current, true
}

func listError(kind string, err error) error {
	if apierrors.IsNotFound(err) || isNoMatchError(err) {
		return fmt.Errorf("karpenter CRDs not installed in cluster: %w", err)
	}
	return fmt.Errorf("failed to list %s resources: %w", kind, err)
}

// isNoMatchError reports whether the error is a "no matches for kind" API
// error, which means the Karpenter CRDs are not served by the cluster.
func isNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no matches for kind") || strings.Contains(msg, "the server could not find the requested resource")
}
