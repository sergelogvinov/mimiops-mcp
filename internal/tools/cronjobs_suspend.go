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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
)

// RegisterCronJobsSuspend adds the cronjobs_suspend tool, which suspends a CronJob (disables future scheduled runs).
func RegisterCronJobsSuspend(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Suspend CronJob"),
		mcp.WithDescription("Suspend a CronJob (stops future scheduled runs)."),
		mcp.WithString("name", mcp.Description("CronJob name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[CronJobSummary](),
	}, clusterOptions(mc)...)

	tool := mcp.NewTool("cronjobs_suspend", opts...)
	s.AddTool(tool, handlerCronJobsSuspend(mc))
}

// handlerCronJobsSuspend returns a handler function for the cronjobs_suspend tool.
func handlerCronJobsSuspend(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := resolveCluster(mc, req)
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
		log.DebugContext(ctx, "cronjobs_suspend called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"cronjob", name,
		)

		// Get the CronJob first
		cronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Check if already suspended
		if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
			result := toCronJobSummary(cronJob)
			return mcp.NewToolResultStructured(result, fmt.Sprintf("CronJob '%s' in namespace '%s' is already suspended.", name, namespace)), nil
		}

		// Patch to suspend
		patch := []byte(`{"spec":{"suspend":true}}`)
		_, err = client.BatchV1().CronJobs(namespace).Patch(ctx, name, k8stypes.MergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to suspend CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// Get the updated CronJob after patch
		updatedCronJob, err := client.BatchV1().CronJobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("CronJob '%s' in namespace '%s' not found after patch", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to get updated CronJob '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := toCronJobSummary(updatedCronJob)
		return mcp.NewToolResultStructured(result, formatter.ToMarkdown(result)), nil
	}
}
