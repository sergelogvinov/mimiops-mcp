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

// PriorityClassesListResult represents the result of listing priority classes.
type PriorityClassesListResult struct {
	PriorityClasses []PriorityClassSummary `json:"priorityclasses" jsonschema:"List of priority classes"`
}

// RegisterPriorityClassesList adds the priorityclasses_list tool, which lists PriorityClasses in the cluster.
func RegisterPriorityClassesList(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("priorityclasses_list",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("List PriorityClasses"),
		mcp.WithDescription("List PriorityClasses in the cluster"),
		mcp.WithOutputSchema[PriorityClassesListResult](),
	)
	s.AddTool(tool, handlerPriorityClassesList(client))
}

// handlerPriorityClassesList returns a handler function for the priorityclasses_list tool.
func handlerPriorityClassesList(client *k8s.Client) func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "priorityclasses_list called")

		// List priority classes
		classes, err := client.SchedulingV1().PriorityClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Priority Classes found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list priority classes: %v", err), nil
		}

		result := PriorityClassesListResult{
			PriorityClasses: make([]PriorityClassSummary, 0, len(classes.Items)),
		}

		// Build result
		for _, pc := range classes.Items {
			result.PriorityClasses = append(result.PriorityClasses, PriorityClassSummary{
				Name:          pc.Name,
				Value:         pc.Value,
				GlobalDefault: pc.GlobalDefault,
				Description:   pc.Description,
				Age:           formatAge(pc.CreationTimestamp),
			})
		}

		// Build fallback text
		fallbackText := "No priority classes found"
		if len(result.PriorityClasses) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
