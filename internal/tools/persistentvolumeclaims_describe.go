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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCDescribeResult represents the result of describing a PersistentVolumeClaim.
type PVCDescribeResult struct {
	PVCSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`
	Finalizers  []string          `json:"finalizers" jsonschema:"Finalizers"`

	AccessModes string         `json:"access_modes,omitempty" jsonschema:"Access modes of the PVC (e.g., RWO)"`
	VolumeMode  string         `json:"volume_mode,omitempty" jsonschema:"Volume mode of the PVC (Filesystem or Block)"`
	UsedBy      []string       `json:"used_by,omitempty" jsonschema:"Pods using the PVC"`
	Events      []EventSummary `json:"events,omitempty" jsonschema:"List of events"`
}

// RegisterPersistentVolumeClaimsDescribe adds the persistentvolumeclaims_describe tool, which
// describes a single PersistentVolumeClaim.
func RegisterPersistentVolumeClaimsDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe PersistentVolumeClaim"),
		mcp.WithDescription("Describe a single PersistentVolumeClaim (access modes, volume mode, pods using it, events)."),
		mcp.WithString("name", mcp.Description("PersistentVolumeClaim name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace name"), mcp.Required()),
		mcp.WithOutputSchema[PVCDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("persistentvolumeclaims_describe", opts...)
	s.AddTool(tool, handlerPersistentVolumeClaimsDescribe(mc))
}

// handlerPersistentVolumeClaimsDescribe returns a handler function for the persistentvolumeclaims_describe tool.
func handlerPersistentVolumeClaimsDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "persistentvolumeclaims_describe called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"name", name,
		)

		// Get the persistent volume claim
		pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("persistent volume claim '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get persistent volume claim '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// List pods in the namespace to find the ones using this PVC
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.WarnContext(ctx, "failed to list pods for persistent volume claim", "namespace", namespace, "err", err)
		}

		result := buildPVCDescribeResult(ctx, client, pvc, pods.Items)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildPVCDescribeResult builds a PVCDescribeResult from a PersistentVolumeClaim and the pods
// in its namespace.
func buildPVCDescribeResult(ctx context.Context, client *k8s.Client, pvc *corev1.PersistentVolumeClaim, pods []corev1.Pod) *PVCDescribeResult {
	volumeMode := "Filesystem"
	if pvc.Spec.VolumeMode != nil {
		volumeMode = string(*pvc.Spec.VolumeMode)
	}

	result := &PVCDescribeResult{
		PVCSummary:  toPVCSummary(pvc),
		Annotations: extractAnnotations(pvc.Annotations),
		Labels:      extractLabels(pvc.Labels),
		Finalizers:  pvc.Finalizers,
		AccessModes: accessModesShort(pvc.Spec.AccessModes),
		VolumeMode:  volumeMode,
		UsedBy:      podsUsingPVC(pods, pvc.Name),
	}

	// List events for the persistent volume claim
	events, err := client.CoreV1().Events(pvc.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=PersistentVolumeClaim,involvedObject.name=%s", pvc.Name),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return result
	}

	result.Events = make([]EventSummary, 0, len(events.Items))
	for _, e := range events.Items {
		firstSeen := ""
		if !e.FirstTimestamp.IsZero() {
			firstSeen = formatAge(e.FirstTimestamp)
		}

		result.Events = append(result.Events, EventSummary{
			Namespace: e.Namespace,
			FirstSeen: firstSeen,
			Age:       formatEventAge(e),
			Message:   e.Message,
			Reason:    e.Reason,
			Type:      e.Type,
		})
	}

	if len(result.Events) > 50 {
		result.Events = result.Events[:50]
	}

	return result
}

// accessModesShort converts access modes to the kubectl short form (e.g., "RWO").
func accessModesShort(modes []corev1.PersistentVolumeAccessMode) string {
	if len(modes) == 0 {
		return ""
	}

	short := map[corev1.PersistentVolumeAccessMode]string{
		corev1.ReadWriteOnce:    "RWO",
		corev1.ReadOnlyMany:     "ROX",
		corev1.ReadWriteMany:    "RWX",
		corev1.ReadWriteOncePod: "RWOP",
	}

	parts := make([]string, 0, len(modes))
	for _, mode := range modes {
		if s, ok := short[mode]; ok {
			parts = append(parts, s)
		} else {
			parts = append(parts, string(mode))
		}
	}

	return strings.Join(parts, ",")
}

// podsUsingPVC returns the names of non-terminated pods that mount the given PVC.
func podsUsingPVC(pods []corev1.Pod, claimName string) []string {
	var usedBy []string

	for _, pod := range pods {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}

		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claimName {
				usedBy = append(usedBy, pod.Name)
				break
			}
		}
	}

	return usedBy
}
