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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobsListResult represents the result of listing Jobs.
type JobsListResult struct {
	Jobs []JobSummary `json:"jobs" jsonschema:"List of jobs"`
}

// RegisterJobsList adds the jobs_list tool, which lists Jobs in a namespace (or all namespaces).
func RegisterJobsList(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List Jobs"),
		mcp.WithDescription("List Jobs in a namespace (or all namespaces)"),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("label_selector", mcp.Description("label selector filter")),
		mcp.WithOutputSchema[JobsListResult](),
	}, clusterOptions(mc)...)

	tool := mcp.NewTool("jobs_list", opts...)
	s.AddTool(tool, handlerJobsList(mc))
}

// handlerJobsList returns a handler function for the jobs_list tool.
func handlerJobsList(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := resolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		labelSelector := req.GetString("label_selector", "")

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "jobs_list called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"label_selector", labelSelector,
		)

		jobs, err := client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Jobs found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list Jobs: %v", err), nil
		}

		result := JobsListResult{
			Jobs: make([]JobSummary, 0, len(jobs.Items)),
		}

		// Build result
		for _, job := range jobs.Items {
			result.Jobs = append(result.Jobs, toJobSummary(&job))
		}

		// Build fallback text
		fallbackText := "No Jobs found"
		if len(result.Jobs) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
