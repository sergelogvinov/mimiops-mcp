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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobLogResult is the structured result of jobs_log.
type JobLogResult struct {
	Streams []LogStream `json:"streams" jsonschema:"Log streams from the Job's pods"`
}

// RegisterJobsLog adds the jobs_log tool, which fetches logs from a Job's pods.
func RegisterJobsLog(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Get Job Logs"),
		mcp.WithDescription("Fetch logs from a Job's pods"),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("container", mcp.Description("container name (optional)")),
		mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
		mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
		mcp.WithBoolean("all_pods", mcp.Description("fetch logs from all owned pods"), mcp.DefaultBool(false)),
		mcp.WithOutputSchema[JobLogResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("jobs_log", opts...)
	s.AddTool(tool, handlerJobsLog(mc))
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get;list;watch

// handlerJobsLog returns a handler function for the jobs_log tool.
func handlerJobsLog(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(ctx, mc, req)
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

		container := req.GetString("container", "")
		tail := req.GetInt("tail", 20)
		previous := req.GetBool("previous", false)
		allPods := req.GetBool("all_pods", false)

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "jobs_log called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"job", name,
			"container", container,
			"tail", tail,
			"previous", previous,
			"all_pods", allPods,
		)

		// Get the Job
		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// List pods owned by this Job
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list pods in namespace '%s': %v", namespace, err), nil
		}

		// Filter pods by Job owner reference
		var ownedPods []corev1.Pod
		for _, pod := range pods.Items {
			if metav1.GetControllerOf(&pod) != nil && metav1.GetControllerOf(&pod).UID == job.UID {
				ownedPods = append(ownedPods, pod)
			}
		}

		// Build result with empty streams if no pods
		if len(ownedPods) == 0 {
			return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' has no pods yet (not started or already cleaned up)", name, namespace), nil
		}

		// Determine which pods to fetch logs from
		var podsToFetch []corev1.Pod
		if allPods {
			podsToFetch = ownedPods
		} else {
			// Fetch logs from the most recently created pod
			latestPod := ownedPods[0]
			for _, pod := range ownedPods {
				if pod.CreationTimestamp.After(latestPod.CreationTimestamp.Time) {
					latestPod = pod
				}
			}
			podsToFetch = []corev1.Pod{latestPod}
		}

		// Fetch logs from pods
		streams := make([]LogStream, 0, len(podsToFetch))
		for _, pod := range podsToFetch {
			stream, err := fetchPodLogStream(ctx, client, namespace, pod.Name, container, "", tail, 0, previous)
			if err != nil {
				return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s': %v", pod.Name, err), nil
			}
			streams = append(streams, stream)
		}

		return mcp.NewToolResultStructuredOnly(JobLogResult{Streams: streams}), nil
	}
}
