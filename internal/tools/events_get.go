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
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/formatter"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventsGetResult represents the result of getting events.
type EventsGetResult struct {
	Events []EventSummary `json:"events" jsonschema:"List of events"`
}

// RegisterEventsGet adds the events_get tool, which gets Kubernetes events from a specific namespace (or all namespaces), sorted by time (warnings first).
func RegisterEventsGet(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Events"),
		mcp.WithDescription("Get events from a specific namespace (or all namespaces), sorted by time (warnings first)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("field_selector", mcp.Description("field selector filter, e.g., 'type==Warning'"), mcp.DefaultString("type==Warning")),
		mcp.WithInteger("limit", mcp.Description("maximum number of events to return"), mcp.DefaultNumber(50)),
		mcp.WithOutputSchema[EventsGetResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("events_get", opts...)
	s.AddTool(tool, handlerEventsGet(mc))
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// handlerEventsGet returns a handler function for the events_get tool.
func handlerEventsGet(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		client, err := clusters.ResolveCluster(mc, req)
		if err != nil {
			return mcp.NewToolResultErrorf("%v", err), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		fieldSelector := req.GetString("field_selector", "")

		limit := req.GetInt("limit", 50)
		switch {
		case limit <= 0:
			limit = 50
		case limit > 500:
			limit = 500
		}

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "events_get called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"field_selector", fieldSelector,
			"limit", limit,
		)

		// List events
		events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no Events found"), nil
			}
			return mcp.NewToolResultErrorf("failed to list events: %v", err), nil
		}

		// Sort events: warnings first, then by lastTimestamp descending
		sort.Slice(events.Items, func(i, j int) bool {
			// Warnings first
			if events.Items[i].Type != events.Items[j].Type {
				return events.Items[i].Type == corev1.EventTypeWarning
			}
			// Then by lastTimestamp descending (most recent first)
			return events.Items[i].LastTimestamp.After(events.Items[j].LastTimestamp.Time)
		})

		// Apply limit
		if len(events.Items) > limit {
			events.Items = events.Items[:limit]
		}

		result := EventsGetResult{
			Events: make([]EventSummary, 0, len(events.Items)),
		}

		// Build result
		for _, event := range events.Items {
			lastSeen := ""
			if !event.LastTimestamp.IsZero() {
				lastSeen = age.FormatAge(event.LastTimestamp)
			}
			summary := EventSummary{
				Age:       lastSeen,
				Type:      event.Type,
				Reason:    event.Reason,
				Object:    fmt.Sprintf("%s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Name),
				Message:   event.Message,
				Namespace: event.Namespace,
			}
			result.Events = append(result.Events, summary)
		}

		// Build fallback text
		fallbackText := "No events found"
		if len(result.Events) > 0 {
			fallbackText = formatter.ToMarkdown(result)
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	}
}
