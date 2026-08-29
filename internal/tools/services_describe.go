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
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/logger"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools/clusters"
	"github.com/sergelogvinov/mimiops-mcp/pkg/age"
	"github.com/sergelogvinov/mimiops-mcp/pkg/formatter"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceDescribeResult represents the result of describing a service.
type ServiceDescribeResult struct {
	ServiceSummary

	Annotations map[string]string `json:"annotations" jsonschema:"Annotations (filtered for user-relevant keys)"`
	Labels      map[string]string `json:"labels" jsonschema:"Labels (filtered for user-relevant keys)"`

	Endpoints []EndpointInfo `json:"endpoints,omitempty" jsonschema:"List of endpoints matching the service selector"`
	Events    []EventSummary `json:"events,omitempty" jsonschema:"List of events related to the service"`
}

// RegisterServicesDescribe adds the services_describe tool, which provides a structured service description.
func RegisterServicesDescribe(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithToolTitle("Describe Service"),
		mcp.WithDescription("Service summary (annotations, labels, selector, endpoints)"),
		mcp.WithString("name", mcp.Description("service name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithOutputSchema[ServiceDescribeResult](),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("services_describe", opts...)
	s.AddTool(tool, handlerServicesDescribe(mc))
}

// +kubebuilder:rbac:groups="",resources=services,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// handlerServicesDescribe returns a handler function for the services_describe tool.
func handlerServicesDescribe(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "services_describe called",
			"cluster", client.ClusterName,
			"user", client.User.Name,
			"namespace", namespace,
			"service", name,
		)

		service, err := client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("service '%s' in namespace '%s' not found", name, namespace), nil
			}

			return mcp.NewToolResultErrorf("failed to get service '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		result := buildServiceDescribeResult(ctx, client, service)
		return mcp.NewToolResultStructured(result, formatter.ToText(result)), nil
	}
}

// +kubebuilder:rbac:groups="discovery.k8s.io",resources=endpointslices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch

// buildServiceDescribeResult builds a ServiceDescribeResult from a Service.
func buildServiceDescribeResult(ctx context.Context, client *k8s.Client, svc *corev1.Service) *ServiceDescribeResult {
	result := &ServiceDescribeResult{
		ServiceSummary: toServiceSummary(svc),
		Annotations:    extractAnnotations(svc.Annotations),
		Labels:         extractLabels(svc.Labels),
	}

	// Get endpoint slices for this service using label selector
	// Services select endpoint slices by the service name label
	endpointSlices, err := client.DiscoveryV1().EndpointSlices(svc.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", v1.LabelServiceName, svc.Name),
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return result
	}

	result.Endpoints = toEndpointInfoFromSlice(endpointSlices.Items)

	// List events for this service
	events, err := client.CoreV1().Events(svc.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return result
	}

	result.Events = make([]EventSummary, 0, len(events.Items))
	for _, e := range events.Items {
		if e.InvolvedObject.Kind == "Service" && e.InvolvedObject.Name == svc.Name {
			firstSeen := ""
			if !e.FirstTimestamp.IsZero() {
				firstSeen = age.FormatAge(e.FirstTimestamp)
			}

			result.Events = append(result.Events, EventSummary{
				FirstSeen: firstSeen,
				Age:       age.FormatEventAge(e),
				Message:   e.Message,
				Reason:    e.Reason,
				Type:      e.Type,
			})
		}
	}

	if len(result.Events) > 50 {
		result.Events = result.Events[:50]
	}

	return result
}

// toEndpointInfoFromSlice converts endpoint slices to EndpointInfo.
func toEndpointInfoFromSlice(slices []v1.EndpointSlice) []EndpointInfo {
	endpoints := make([]EndpointInfo, 0)

	for _, slice := range slices {
		for _, endpoint := range slice.Endpoints {
			for _, port := range slice.Ports {
				endpointInfo := EndpointInfo{
					IP:   "",
					Port: fmt.Sprintf("%d", *port.Port),
				}

				// Get the first address from the endpoint
				if len(endpoint.Addresses) > 0 {
					endpointInfo.IP = endpoint.Addresses[0]
				}

				// Add target reference if available
				if endpoint.TargetRef != nil {
					endpointInfo.TargetRef = fmt.Sprintf("%s/%s", endpoint.TargetRef.Kind, endpoint.TargetRef.Name)
				}

				// Add node name if available
				if endpoint.NodeName != nil {
					endpointInfo.NodeName = *endpoint.NodeName
				}

				// Check if endpoint is ready
				if endpoint.Conditions.Ready != nil {
					endpointInfo.Ready = *endpoint.Conditions.Ready
				} else {
					endpointInfo.Ready = true
				}

				endpoints = append(endpoints, endpointInfo)
			}
		}
	}

	return endpoints
}
