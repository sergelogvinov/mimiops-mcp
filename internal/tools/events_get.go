package tools

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventsGetResult represents the result of getting events.
type EventsGetResult struct {
	Events []EventSummary `json:"events" jsonschema:"List of events"`
}

// RegisterEventsGet adds the events_get tool, which gets Kubernetes events from a specific namespace (or all namespaces), sorted by time (warnings first).
func RegisterEventsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("events_get",
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Get Events"),
		mcp.WithDescription("Get events from a specific namespace (or all namespaces), sorted by time (warnings first)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithInteger("limit", mcp.Description("maximum number of events to return"), mcp.DefaultNumber(50)),
		mcp.WithOutputSchema[EventsGetResult](),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")
		if namespace == "" {
			namespace = metav1.NamespaceAll
		}

		fieldSelector := req.GetString("field_selector", "")
		limit := req.GetInt("limit", 50)
		if limit <= 0 {
			limit = 50
		}
		if limit > 500 {
			limit = 500
		}

		log.DebugContext(ctx, "events_get called",
			"namespace", namespace,
			"field_selector", fieldSelector,
			"limit", limit,
		)

		// List events
		events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: fieldSelector})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("no events found in namespace '%s'", namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to list events in namespace '%s': %v", namespace, err), nil
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
				lastSeen = formatAge(event.LastTimestamp)
			}
			summary := EventSummary{
				LastSeen:  lastSeen,
				Type:      event.Type,
				Reason:    event.Reason,
				Object:    fmt.Sprintf("%s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Name),
				Message:   event.Message,
				Namespace: event.Namespace,
			}
			result.Events = append(result.Events, summary)
		}

		// Build fallback text
		var fallbackText string
		switch len(result.Events) {
		case 0:
			fallbackText = "No events found."
		case 1:
			fallbackText = fmt.Sprintf("Found 1 event: %s (%s)", result.Events[0].Reason, result.Events[0].Object)
		default:
			fallbackText = fmt.Sprintf("Found %d events", len(result.Events))
		}

		return mcp.NewToolResultStructured(result, fallbackText), nil
	})
}
