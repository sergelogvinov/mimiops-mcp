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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobDescribeResult represents the result of describing a Job.
type JobDescribeResult struct {
	JobSummary
	JobSpec

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels"`

	Conditions []ConditionInfo `json:"conditions,omitempty" jsonschema:"List of conditions of the Job"`
	Pods       []PodSummary    `json:"pods,omitempty" jsonschema:"List of pods owned by the Job"`
}

// RegisterJobsDescribe adds the jobs_describe tool, which provides a structured Job summary.
func RegisterJobsDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Job"),
		mcp.WithDescription("Job summary (conditions, parallelism, completions, backoff, active pods list)."),
		mcp.WithString("name", mcp.Description("Job name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[JobDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("jobs_describe", opts...)
	s.AddTool(tool, handlerJobsDescribe(mc))
}

// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

// handlerJobsDescribe returns a handler function for the jobs_describe tool.
func handlerJobsDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "jobs_describe called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"job", name,
		)

		job, err := client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("Job '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get Job '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := buildJobDescribeResult(ctx, job, client)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to build result: %v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildJobDescribeResult builds a JobDescribeResult from a Job.
func buildJobDescribeResult(ctx context.Context, job *batchv1.Job, client *k8s.Client) (*JobDescribeResult, error) {
	result := &JobDescribeResult{
		JobSummary:  toJobSummary(job),
		JobSpec:     toJobSpec(job),
		Annotations: extractAnnotations(job.Annotations),
		Labels:      extractLabels(job.Labels),
	}

	// Conditions
	result.Conditions = make([]ConditionInfo, 0, len(job.Status.Conditions))
	for _, cond := range job.Status.Conditions {
		result.Conditions = append(result.Conditions, ConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	// Pods - list pods by Job
	pods, err := client.CoreV1().Pods(job.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("batch.kubernetes.io/job-name=%s", job.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	result.Pods = make([]PodSummary, 0, len(pods.Items))
	for _, pod := range pods.Items {
		node := pod.Spec.NodeName
		if node == "" {
			node = "<pending>"
		}

		podInfo := PodSummary{
			Name:     pod.Name,
			Ready:    formatReady(pod.Status),
			Status:   string(pod.Status.Phase),
			Restarts: containerRestartCount(pod.Status),
			Age:      age.FormatAge(pod.CreationTimestamp),
			Node:     node,
		}

		result.Pods = append(result.Pods, podInfo)
	}

	return result, nil
}
