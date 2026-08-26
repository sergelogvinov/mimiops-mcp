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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// PodLogResult is the structured result of pods_log.
type PodLogResult struct {
	Streams []LogStream `json:"streams" jsonschema:"Log streams from the pod's containers"`
}

// RegisterPodsLog adds the pods_log tool, which fetches pod logs.
func RegisterPodsLog(s *server.MCPServer, mc *k8s.MultiClusterClient) {
	opts := append([]mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithToolTitle("Get Pod Logs"),
		mcp.WithDescription("Fetch pod logs"),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("container", mcp.Description("container name")),
		// mcp.WithString("stream", mcp.Description("stream name, default is all"), mcp.Enum("All", "Stdout", "Stderr")),
		mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
		mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
		mcp.WithInteger("since_seconds", mcp.Description("only return logs newer than N seconds"), mcp.DefaultNumber(0)),
	}, clusters.ClusterOptions(mc)...)

	tool := mcp.NewTool("pods_log", opts...)
	s.AddTool(tool, handlerPodsLog(mc))
}

// handlerPodsLog returns a handler function for the pods_log tool.
func handlerPodsLog(mc *k8s.MultiClusterClient) func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

		container := req.GetString("container", "")
		streamName := req.GetString("stream", "") // Alpha feature: PodLogsQuerySplitStreams
		tail := req.GetInt("tail", 20)
		previous := req.GetBool("previous", false)
		sinceSeconds := req.GetInt("since_seconds", 0)

		log := logger.FromContext(ctx)
		log.DebugContext(ctx, "pods_log called",
			"cluster", client.ClusterName,
			"namespace", namespace,
			"pod", name,
			"container", container,
			"stream", streamName,
			"tail", tail,
			"previous", previous,
			"since_seconds", sinceSeconds,
		)

		// Get pod to check container name
		stream, err := fetchPodLogStream(ctx, client, namespace, name, container, streamName, tail, sinceSeconds, previous)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return mcp.NewToolResultErrorf("pod '%s' in namespace '%s' not found", name, namespace), nil
			}
			return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		return mcp.NewToolResultText(stream.Logs), nil
	}
}
