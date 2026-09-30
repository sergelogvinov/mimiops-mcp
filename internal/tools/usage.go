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
	"context"
	"fmt"
	"time"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"k8s.io/metrics/pkg/client/custom_metrics"
)

const (
	// customMetricsMaxAge is the maximum age of a custom.metrics.k8s.io value.
	// Older values are stale and fall back to metrics.k8s.io.
	customMetricsMaxAge = 10 * time.Minute
)

// ContainerUsage holds the current resource usage of one container from the metrics API.
type ContainerUsage struct {
	Name   string `json:"name" jsonschema:"Name of the container"`
	CPU    string `json:"cpu" jsonschema:"Current CPU usage (millicores)"`
	Memory string `json:"memory" jsonschema:"Current memory usage"`
}

// ResourceUsage holds the resource usage of a pod, workload, Job or CronJob from the metrics API.
type ResourceUsage struct {
	Window     string           `json:"window,omitempty" jsonschema:"Averaging window of the usage values (custom.metrics.k8s.io only)"`
	CPU        string           `json:"cpu" jsonschema:"CPU usage (millicores)"`
	Memory     string           `json:"memory" jsonschema:"Memory usage"`
	Containers []ContainerUsage `json:"containers,omitempty" jsonschema:"Per-container usage"`
}

// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=nodes/*,verbs=get;list;watch
// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=pods/*,verbs=get;list;watch
// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=deployments.apps/*,verbs=get;list;watch
// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=statefulsets.apps/*,verbs=get;list;watch
// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=daemonsets.apps/*,verbs=get;list;watch
// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=jobs.batch/*,verbs=get;list;watch
// +kubebuilder:rbac:groups=custom.metrics.k8s.io,resources=cronjobs.batch/*,verbs=get;list;watch

// customUsage holds the windowed CPU and memory averages of an object from
// custom.metrics.k8s.io.
type customUsage struct {
	window string
	cpu    resource.Quantity
	mem    resource.Quantity
}

// fetchCustomUsage fetches the object's windowed CPU and memory averages from
// custom.metrics.k8s.io/v1beta2. The API aggregates containers (and pods for
// workloads), so the result has no per-container breakdown. It returns nil
// when no custom-metrics adapter serves the object's metrics.
func fetchCustomUsage(ctx context.Context, client *k8s.Client, gk schema.GroupKind, namespace, name string, uid types.UID) *ResourceUsage {
	usage := fetchCustomQuantities(ctx, client, gk, namespace, name, uid)
	if usage == nil {
		return nil
	}

	return &ResourceUsage{
		Window: usage.window,
		CPU:    formatCPU(usage.cpu),
		Memory: formatMemory(usage.mem),
	}
}

// fetchCustomQuantities fetches the object's windowed CPU and memory averages
// from custom.metrics.k8s.io/v1beta2. Metric names follow the
// kubernetes-custom-metrics gateway grammar (<base>_avg_<window>, with
// node_cpu/node_memory bases for Nodes). An empty namespace selects the
// root-scoped (Node) metrics. It returns nil when custom metrics are disabled,
// or no adapter serves fresh values for the object.
func fetchCustomQuantities(ctx context.Context, client *k8s.Client, gk schema.GroupKind, namespace, name string, uid types.UID) *customUsage {
	window := client.UsageWindow()
	if window == "" {
		return nil
	}

	log := logger.FromContext(ctx)

	customClient, err := client.CustomMetrics()
	if err != nil {
		log.DebugContext(ctx, "failed to create custom metrics client", "kind", gk.Kind, "name", name, "err", err)
		return nil
	}

	var metrics custom_metrics.MetricsInterface
	if namespace == "" {
		metrics = customClient.RootScopedMetrics()
	} else {
		metrics = customClient.NamespacedMetrics(namespace)
	}

	prefix := ""
	if gk.Kind == "Node" {
		prefix = "node_"
	}

	usage := &customUsage{window: window}
	for _, m := range []struct {
		name  string
		value *resource.Quantity
	}{
		{prefix + "cpu_avg_" + window, &usage.cpu},
		{prefix + "memory_avg_" + window, &usage.mem},
	} {
		v, err := metrics.GetForObject(gk, name, m.name, labels.Everything())
		if err != nil {
			log.DebugContext(ctx, "custom metrics not available", "kind", gk.Kind, "name", name, "metric", m.name, "err", err)
			return nil
		}

		// Reject values that belong to a previous object with the same name.
		if v.DescribedObject.UID != "" && v.DescribedObject.UID != uid {
			log.DebugContext(ctx, "custom metrics belong to another object instance", "kind", gk.Kind, "name", name, "metric", m.name, "uid", v.DescribedObject.UID)
			return nil
		}

		if !v.Timestamp.IsZero() && time.Since(v.Timestamp.Time) > customMetricsMaxAge {
			log.DebugContext(ctx, "custom metrics are stale", "kind", gk.Kind, "name", name, "metric", m.name, "timestamp", v.Timestamp.Time)
			return nil
		}

		// Report the window the adapter actually averaged over, which may be
		// shorter than requested (e.g. for a recently created object).
		if v.WindowSeconds != nil && *v.WindowSeconds > 0 {
			usage.window = formatWindow(time.Duration(*v.WindowSeconds) * time.Second)
		}

		*m.value = v.Value
	}

	return usage
}

// formatWindow formats a window duration in the largest whole unit, e.g. 30m or 90s.
func formatWindow(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	default:
		return fmt.Sprintf("%ds", d/time.Second)
	}
}

// formatCPU formats a CPU quantity in millicores.
func formatCPU(q resource.Quantity) string {
	return fmt.Sprintf("%dm", q.MilliValue())
}

// formatMemory formats a memory quantity rounded up to whole bytes, so
// adapters that encode averages as milli-quantities do not print milli-bytes.
func formatMemory(q resource.Quantity) string {
	return resource.NewQuantity(q.Value(), resource.BinarySI).String()
}

// sumContainerUsage sums the CPU and memory usage of containers. With
// perContainer set, the result also lists each container's usage.
func sumContainerUsage(containers []metricsv1beta1.ContainerMetrics, perContainer bool) *ResourceUsage {
	usage := &ResourceUsage{}
	if perContainer {
		usage.Containers = make([]ContainerUsage, 0, len(containers))
	}

	var cpu, mem resource.Quantity
	for _, c := range containers {
		cu := ContainerUsage{Name: c.Name}
		if q, ok := c.Usage[corev1.ResourceCPU]; ok {
			cu.CPU = formatCPU(q)
			cpu.Add(q)
		}
		if q, ok := c.Usage[corev1.ResourceMemory]; ok {
			cu.Memory = formatMemory(q)
			mem.Add(q)
		}

		if perContainer {
			usage.Containers = append(usage.Containers, cu)
		}
	}

	usage.CPU = formatCPU(cpu)
	usage.Memory = formatMemory(mem)

	return usage
}

// fetchSelectorMetricsUsage sums the current resource usage of all pods matching
// selector from metrics.k8s.io. It returns nil when the metrics API is unavailable
// or no pod metrics match.
func fetchSelectorMetricsUsage(ctx context.Context, client *k8s.Client, namespace, selector string) *ResourceUsage {
	log := logger.FromContext(ctx)

	metricsClient, err := client.Metrics()
	if err != nil {
		log.DebugContext(ctx, "failed to create metrics client", "namespace", namespace, "err", err)
		return nil
	}

	podMetrics, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		log.DebugContext(ctx, "pod metrics not available", "namespace", namespace, "selector", selector, "err", err)
		return nil
	}

	if len(podMetrics.Items) == 0 {
		return nil
	}

	containers := make([]metricsv1beta1.ContainerMetrics, 0, len(podMetrics.Items))
	for _, pm := range podMetrics.Items {
		containers = append(containers, pm.Containers...)
	}

	return sumContainerUsage(containers, false)
}
