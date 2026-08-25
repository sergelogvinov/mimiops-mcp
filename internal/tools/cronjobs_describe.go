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
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobDescribeResult represents the result of describing a CronJob.
type CronJobDescribeResult struct {
	CronJobSummary
	CronJobSpec

	Labels      map[string]string `json:"labels" jsonschema:"Labels of the CronJob"`
	Annotations map[string]string `json:"annotations" jsonschema:"Annotations of the CronJob"`

	ActiveJobs []string `json:"activeJobs" jsonschema:"List of active job names"`
}

// RegisterCronJobsDescribe adds the cronjobs_describe tool, which provides a structured CronJob summary.
func RegisterCronJobsDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe CronJob"),
		mcp.WithDescription("CronJob summary (schedule, suspend, concurrency policy, active jobs, last schedule, job template)"),
		mcp.WithString("name", mcp.Description("CronJob name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[CronJobDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("cronjobs_describe", opts...)
	s.AddTool(tool, handlerCronJobsDescribe(mc))
}

// handlerCronJobsDescribe returns a handler function for the cronjobs_describe tool.
func handlerCronJobsDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		log.DebugContext(ctx, "cronjobs_describe called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"cronjob", name,
		)

		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result, err := buildCronJobDescribeResult(cronJob)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to build result: %v", err), nil
		}

		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}

// buildCronJobDescribeResult builds a CronJobDescribeResult from a CronJob.
func buildCronJobDescribeResult(cj *batchv1.CronJob) (*CronJobDescribeResult, error) {
	result := &CronJobDescribeResult{
		CronJobSummary: toCronJobSummary(cj),
		CronJobSpec:    toCronJobSpec(cj),
		Annotations:    extractAnnotations(cj.Annotations),
		Labels:         extractLabels(cj.Labels),
		ActiveJobs:     make([]string, 0, len(cj.Status.Active)),
	}

	// Active Jobs
	for _, job := range cj.Status.Active {
		result.ActiveJobs = append(result.ActiveJobs, job.Name)
	}

	return result, nil
}
