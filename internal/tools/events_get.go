package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterEventsGet adds the events_get tool, which gets Kubernetes events from a specific namespace (or all namespaces), sorted by time (warnings first).
func RegisterEventsGet(s *server.MCPServer, client *k8s.Client, log *slog.Logger) {
	tool := mcp.NewTool("events_get",
		mcp.WithDescription("Get Kubernetes events from a specific namespace (or all namespaces), sorted by time (warnings first)."),
		mcp.WithString("namespace", mcp.Description("namespace; leave empty for all namespaces")),
		mcp.WithString("field_selector", mcp.Description("field selector filter")),
		mcp.WithInteger("limit", mcp.Description("maximum number of events to return"), mcp.DefaultNumber(50)),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		namespace := req.GetString("namespace", "")

		fieldSelector := req.GetString("field_selector", "")
		limit := req.GetInt("limit", 50)
		if limit <= 0 {
			limit = 50
		}
		if limit > 500 {
			limit = 500
		}

		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		log.DebugContext(ctx, "events_get called",
			"namespace", namespace,
			"field_selector", fieldSelector,
			"limit", limit,
		)

		// Use metav1.NamespaceAll for empty namespace (all namespaces)
		ns := namespace
		if ns == "" {
			ns = metav1.NamespaceAll
		}

		// Build list options
		opts := metav1.ListOptions{}
		if fieldSelector != "" {
			opts.FieldSelector = fieldSelector
		}

		// List events
		events, err := client.CoreV1().Events(ns).List(ctx, opts)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to list events in namespace '%s': %v", ns, err), nil
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

		result, err := formatEventsGet(events.Items, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatEventsGet formats events for MCP tool output.
func formatEventsGet(events []corev1.Event, format string) (string, error) {
	if format == "json" {
		return formatEventsGetJSON(events)
	}
	return formatEventsGetText(events), nil
}

// formatEventsGetText formats events as a markdown table.
func formatEventsGetText(events []corev1.Event) string {
	if len(events) == 0 {
		return "No events found."
	}

	var buf bytes.Buffer
	buf.WriteString("| LAST SEEN | TYPE | REASON | OBJECT | MESSAGE |\n")
	buf.WriteString("|-----------|------|--------|--------|---------|\n")

	for _, event := range events {
		lastSeen := "-"
		if !event.LastTimestamp.IsZero() {
			lastSeen = formatAge(event.LastTimestamp)
		}
		eventType := event.Type
		reason := event.Reason
		object := fmt.Sprintf("%s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Name)
		if event.InvolvedObject.Namespace != "" && event.InvolvedObject.Namespace != event.Namespace {
			object = fmt.Sprintf("%s/%s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Namespace, event.InvolvedObject.Name)
		}
		message := event.Message
		if message == "" {
			message = "-"
		}

		fmt.Fprintf(&buf, "| %s | %s | %s | %s | %s |\n",
			lastSeen, eventType, reason, object, message)
	}

	return buf.String()
}

// formatEventsGetJSON formats events as JSON.
func formatEventsGetJSON(events []corev1.Event) (string, error) {
	summaries := make([]EventSummary, 0, len(events))
	for _, event := range events {
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
		summaries = append(summaries, summary)
	}

	result := struct {
		Events []EventSummary `json:"events"`
	}{
		Events: summaries,
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// EventSummary is the trimmed representation of an event used by events_get.
type EventSummary struct {
	LastSeen  string `json:"last_seen"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Object    string `json:"object"`
	Message   string `json:"message"`
	Namespace string `json:"namespace,omitempty"`
}
