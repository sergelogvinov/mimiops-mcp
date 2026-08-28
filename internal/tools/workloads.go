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
	"strings"

	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get

// resolveWorkloadKind returns the kind of the workload named `name` in `namespace`.
// If `kind` is provided it is validated and returned directly.
// Otherwise it probes deployment → statefulset → daemonset via typed Get.
// Returns the resolved kind and any error encountered.
func resolveWorkloadKind(ctx context.Context, client *k8s.Client, namespace, name, kind string) (string, error) {
	// If kind is provided, validate it and return directly
	if kind != "" {
		if kind != "deployment" && kind != "statefulset" && kind != "daemonset" {
			return "", fmt.Errorf("invalid parameter 'kind': must be one of deployment, statefulset, daemonset")
		}
		return kind, nil
	}

	// kind omitted → probe each kind in order: deployment → statefulset → daemonset
	matches := []string{}

	_, err := client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to check deployment '%s': %v", name, err)
		}
	} else {
		matches = append(matches, "deployment")
	}

	_, err = client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to check statefulset '%s': %v", name, err)
		}
	} else {
		matches = append(matches, "statefulset")
	}

	_, err = client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return "", fmt.Errorf("failed to check daemonset '%s': %v", name, err)
		}
	} else {
		matches = append(matches, "daemonset")
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("workload '%s' not found in namespace '%s' (checked deployment, statefulset, daemonset)", name, namespace)
	}

	if len(matches) > 1 {
		// Build ambiguous error message
		var details strings.Builder
		for _, m := range matches {
			fmt.Fprintf(&details, "  - %s/%s\n", m, name)
		}
		return "", fmt.Errorf("ambiguous workload '%s' in namespace '%s':\n%sPlease retry with an explicit 'kind' parameter", name, namespace, details.String())
	}

	return matches[0], nil
}

// getWorkloadByKind gets a workload by kind and name.
func getWorkloadByKind(ctx context.Context, client *k8s.Client, namespace, name, kind string) (any, error) {
	switch kind {
	case "deployment":
		return client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	case "statefulset":
		return client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	case "daemonset":
		return client.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	default:
		return nil, fmt.Errorf("invalid kind '%s'", kind)
	}
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=list
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=list
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=list

// listWorkloadsByKind lists workloads of a specific kind.
func listWorkloadsByKind(ctx context.Context, client *k8s.Client, namespace, kind string, labelSelector string) (summaries []WorkloadSummary, err error) {
	opts := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	switch kind {
	case "deployment":
		deployments, err := client.AppsV1().Deployments(namespace).List(ctx, opts)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("no deployments found")
			}
			return nil, fmt.Errorf("failed to list deployments: %v", err)
		}
		for _, d := range deployments.Items {
			summaries = append(summaries, toWorkloadSummaryDeployment(&d))
		}
	case "statefulset":
		statefulsets, err := client.AppsV1().StatefulSets(namespace).List(ctx, opts)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("no statefulsets found")
			}
			return nil, fmt.Errorf("failed to list statefulsets: %v", err)
		}
		for _, s := range statefulsets.Items {
			summaries = append(summaries, toWorkloadSummaryStatefulSet(&s))
		}
	case "daemonset":
		daemonsets, err := client.AppsV1().DaemonSets(namespace).List(ctx, opts)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("no daemonsets found")
			}
			return nil, fmt.Errorf("failed to list daemonsets: %v", err)
		}
		for _, d := range daemonsets.Items {
			summaries = append(summaries, toWorkloadSummaryDaemonSet(&d))
		}
	}

	return summaries, nil
}

// listAllWorkloads lists all three kinds of workloads in a namespace.
func listAllWorkloads(ctx context.Context, client *k8s.Client, namespace string, labelSelector string) ([]WorkloadSummary, error) {
	opts := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	var summaries []WorkloadSummary

	// List deployments
	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, opts)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to list deployments: %v", err)
	}
	for _, d := range deployments.Items {
		summaries = append(summaries, toWorkloadSummaryDeployment(&d))
	}

	// List statefulsets
	statefulsets, err := client.AppsV1().StatefulSets(namespace).List(ctx, opts)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to list statefulsets: %v", err)
	}
	for _, s := range statefulsets.Items {
		summaries = append(summaries, toWorkloadSummaryStatefulSet(&s))
	}

	// List daemonsets
	daemonsets, err := client.AppsV1().DaemonSets(namespace).List(ctx, opts)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to list daemonsets: %v", err)
	}
	for _, d := range daemonsets.Items {
		summaries = append(summaries, toWorkloadSummaryDaemonSet(&d))
	}

	return summaries, nil
}

// toWorkloadSummary converts a Deployment to a WorkloadSummary.
func toWorkloadSummaryDeployment(deployment *appsv1.Deployment) WorkloadSummary {
	return WorkloadSummary{
		Kind:      "deployment",
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
		Ready:     formatDeploymentReady(deployment),
		Desired:   int(*deployment.Spec.Replicas),
		Age:       age.FormatAge(deployment.CreationTimestamp),
	}
}

// toWorkloadSummary converts a StatefulSet to a WorkloadSummary.
func toWorkloadSummaryStatefulSet(statefulset *appsv1.StatefulSet) WorkloadSummary {
	return WorkloadSummary{
		Kind:      "statefulset",
		Namespace: statefulset.Namespace,
		Name:      statefulset.Name,
		Ready:     formatStatefulSetReady(statefulset),
		Desired:   int(*statefulset.Spec.Replicas),
		Age:       age.FormatAge(statefulset.CreationTimestamp),
	}
}

// toWorkloadSummary converts a DaemonSet to a WorkloadSummary.
func toWorkloadSummaryDaemonSet(daemonset *appsv1.DaemonSet) WorkloadSummary {
	return WorkloadSummary{
		Kind:      "daemonset",
		Namespace: daemonset.Namespace,
		Name:      daemonset.Name,
		Ready:     formatDaemonSetReady(daemonset),
		Desired:   int(daemonset.Status.DesiredNumberScheduled),
		Age:       age.FormatAge(daemonset.CreationTimestamp),
	}
}

// toWorkloadSummary converts a workload object to a WorkloadSummary based on its kind.
func toWorkloadSummary(workload any) WorkloadSummary {
	switch w := workload.(type) {
	case *appsv1.Deployment:
		return toWorkloadSummaryDeployment(w)
	case *appsv1.StatefulSet:
		return toWorkloadSummaryStatefulSet(w)
	case *appsv1.DaemonSet:
		return toWorkloadSummaryDaemonSet(w)
	default:
		return WorkloadSummary{}
	}
}

// formatDeploymentReady returns the ready/desired replicas string for a Deployment.
func formatDeploymentReady(deployment *appsv1.Deployment) string {
	ready := deployment.Status.ReadyReplicas
	desired := *deployment.Spec.Replicas
	return fmt.Sprintf("%d/%d", ready, desired)
}

// formatStatefulSetReady returns the ready/desired replicas string for a StatefulSet.
func formatStatefulSetReady(statefulset *appsv1.StatefulSet) string {
	ready := statefulset.Status.ReadyReplicas
	desired := *statefulset.Spec.Replicas
	return fmt.Sprintf("%d/%d", ready, desired)
}

// formatDaemonSetReady returns the ready/desired replicas string for a DaemonSet.
func formatDaemonSetReady(daemonset *appsv1.DaemonSet) string {
	ready := daemonset.Status.NumberReady
	desired := daemonset.Status.DesiredNumberScheduled
	return fmt.Sprintf("%d/%d", ready, desired)
}
