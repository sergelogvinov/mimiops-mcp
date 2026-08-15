package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogLine represents a single log line with timestamp.
type LogLine struct {
	Timestamp time.Time `json:"timestamp"`
	Line      string    `json:"line"`
}

// RegisterPodsLog adds the pods_log tool, which fetches pod logs.
func RegisterPodsLog(s *server.MCPServer, client *k8s.Client) {
	tool := mcp.NewTool("pods_log",
		mcp.WithDescription("Fetch pod logs."),
		mcp.WithString("name", mcp.Description("pod name"), mcp.Required()),
		mcp.WithString("namespace", mcp.Description("namespace"), mcp.Required()),
		mcp.WithString("container", mcp.Description("container name (optional)")),
		mcp.WithInteger("tail", mcp.Description("number of lines to show from end of logs"), mcp.DefaultNumber(20)),
		mcp.WithBoolean("previous", mcp.Description("return previous terminated container logs"), mcp.DefaultBool(false)),
		mcp.WithInteger("since_seconds", mcp.Description("only return logs newer than N seconds"), mcp.DefaultNumber(0)),
		mcp.WithString("format", mcp.Description(`"text" or "json"`), mcp.DefaultString("text")),
	)
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.GetString("name", "")
		if name == "" {
			return mcp.NewToolResultError("missing required parameter 'name'"), nil
		}

		namespace := req.GetString("namespace", "")
		if namespace == "" {
			return mcp.NewToolResultError("missing required parameter 'namespace'"), nil
		}

		container := req.GetString("container", "")
		tail := req.GetInt("tail", 20)
		previous := req.GetBool("previous", false)
		sinceSeconds := req.GetInt("since_seconds", 0)
		format := req.GetString("format", "text")

		if format != "text" && format != "json" {
			return mcp.NewToolResultErrorf("invalid format '%s', must be 'text' or 'json'", format), nil
		}

		// Get pod to check container names
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return mcp.NewToolResultErrorf("failed to get pod '%s' in namespace '%s': %v", name, namespace, err), nil
		}

		// If container is omitted, check for kubectl.kubernetes.io/default-container annotation
		// If not found and pod has multiple containers, return error listing available containers
		if container == "" {
			// Check for default container annotation
			if defaultContainer, ok := pod.Annotations["kubectl.kubernetes.io/default-container"]; ok {
				container = defaultContainer
			} else if len(pod.Spec.Containers) > 1 {
				var containerNames []string
				for _, c := range pod.Spec.Containers {
					containerNames = append(containerNames, c.Name)
				}
				return mcp.NewToolResultErrorf("pod has multiple containers. Specify one of: %v, or set kubectl.kubernetes.io/default-container annotation", containerNames), nil
			} else {
				container = pod.Spec.Containers[0].Name
			}
		}

		// Validate container exists
		containerFound := false
		for _, c := range pod.Spec.Containers {
			if c.Name == container {
				containerFound = true
				break
			}
		}
		if !containerFound {
			var containerNames []string
			for _, c := range pod.Spec.Containers {
				containerNames = append(containerNames, c.Name)
			}
			return mcp.NewToolResultErrorf("container '%s' not found. Available containers: %v", container, containerNames), nil
		}

		// Get log options - convert int to int64
		tailInt64 := int64(tail)
		logOpts := &corev1.PodLogOptions{
			Container: container,
			TailLines: &tailInt64,
			Previous:  previous,
		}

		if sinceSeconds > 0 {
			sinceSecondsInt64 := int64(sinceSeconds)
			logOpts.SinceSeconds = &sinceSecondsInt64
		}

		// Fetch logs
		logReq := client.CoreV1().Pods(namespace).GetLogs(name, logOpts)
		logStream, err := logReq.Stream(ctx)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to fetch logs for pod '%s' container '%s': %v", name, container, err), nil
		}
		defer logStream.Close() //nolint:errcheck

		// Read all logs
		var logBuffer bytes.Buffer
		buf := make([]byte, 1024)
		for {
			n, err := logStream.Read(buf)
			if n > 0 {
				logBuffer.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}

		logContent := logBuffer.String()

		// Format output
		result, err := formatPodLog(logContent, format)
		if err != nil {
			return mcp.NewToolResultErrorf("failed to format output: %v", err), nil
		}

		return mcp.NewToolResultText(result), nil
	})
}

// formatPodLog formats pod log output.
func formatPodLog(log string, format string) (string, error) {
	if format == "json" {
		return formatPodLogJSON(log)
	}
	return log, nil
}

// formatPodLogJSON formats logs as JSON array of LogLine objects.
func formatPodLogJSON(log string) (string, error) {
	var lines []LogLine
	for _, line := range splitLines(log) {
		if line == "" {
			continue
		}
		lines = append(lines, LogLine{
			Timestamp: time.Now(),
			Line:      line,
		})
	}

	result := struct {
		Logs []LogLine `json:"logs"`
	}{
		Logs: lines,
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// splitLines splits log content into lines.
func splitLines(log string) []string {
	var lines []string
	var currentLine bytes.Buffer
	for _, r := range log {
		if r == '\n' {
			if currentLine.Len() > 0 {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
			}
		} else {
			currentLine.WriteRune(r)
		}
	}
	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}
	return lines
}
