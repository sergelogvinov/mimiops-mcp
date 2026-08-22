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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	"github.com/sergelogvinov/mimiops-mcp/internal/tools"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// OutputFormat specifies the output format for tool results.
type OutputFormat string

const (
	OutputText OutputFormat = "text"
	OutputJSON OutputFormat = "json"
	OutputYAML OutputFormat = "yaml"
)

// newToolsCmd creates the `tools` subcommand that lets users invoke MCP tools
// directly from the CLI without going through an MCP client.
func newToolsCmd(flags *Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools [tool-name] [key=value ...]",
		Short: "Invoke MCP tools from the CLI",
		Long:  "Invoke MCP tools from the CLI. If no tool-name is provided, lists available tools.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTools(cmd.Context(), flags, args)
		},
	}

	flags.AddToolFlags(cmd.Flags())

	return cmd
}

// runTools executes the tools command.
func runTools(ctx context.Context, f *Flags, args []string) error {
	cfg := f.Config()

	client, err := k8s.NewClient(cfg)
	if err != nil {
		return err
	}

	srv := server.NewMCPServer("mimiops-mcp", version)
	tools.RegisterTools(srv, client, cfg.AllowDestructive)
	toolsMap := srv.ListTools()

	// No arguments: list available tools
	if len(args) == 0 {
		fmt.Println(formatToolList(toolsMap))

		return nil
	}

	// Look up the tool
	toolName := args[0]
	tool, ok := toolsMap[toolName]
	if !ok {
		return fmt.Errorf("unknown tool %q\n\nAvailable tools:\n%s", toolName, formatToolList(toolsMap))
	}

	// Parse key=value arguments
	arguments, err := parseArguments(args[1:])
	if err != nil {
		return err
	}

	// Validate required parameters
	if err := validateRequiredParams(tool.Tool, arguments); err != nil {
		return err
	}

	// Build the call request
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: arguments,
		},
	}

	// Call the tool handler
	result, err := tool.Handler(ctx, req)
	if err != nil {
		return fmt.Errorf("tool %q failed: %w", toolName, err)
	}

	// Render the result
	outputFormat := OutputFormat(f.Output)
	if err := renderResult(os.Stdout, result, outputFormat); err != nil {
		return fmt.Errorf("failed to render result: %w", err)
	}

	// Exit with error if the tool reported an error
	if result.IsError {
		return fmt.Errorf("tool %q reported an error", toolName)
	}

	return nil
}

// formatToolList returns a human-readable list of tool names and descriptions.
func formatToolList(toolsMap map[string]*server.ServerTool) string {
	lines := make([]string, 0, len(toolsMap))

	// Sort tool names for stable output
	names := make([]string, 0, len(toolsMap))
	for name := range toolsMap {
		names = append(names, name)
	}
	// Simple bubble sort since we don't have external deps for sorting
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}

	for _, name := range names {
		tool := toolsMap[name]
		desc := tool.Tool.Description
		if desc == "" {
			desc = "(no description)"
		}
		lines = append(lines, fmt.Sprintf("%s - %s", name, desc))
	}

	return strings.Join(lines, "\n")
}

// parseArguments parses key=value pairs from command line arguments.
// Values may contain '='; we split on the first '=' only.
// A bare token without '=' is an error (suggest quoting).
// key= means empty string value.
func parseArguments(args []string) (map[string]any, error) {
	result := make(map[string]any)

	for _, arg := range args {
		// Split on first '='
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("argument %q must be in key=value format", arg)
		}

		key := parts[0]
		value := parts[1]

		if key == "" {
			return nil, fmt.Errorf("argument %q has empty key", arg)
		}

		result[key] = value
	}

	return result, nil
}

// renderResult prints the tool result in the specified format.
func renderResult(w io.Writer, result *mcp.CallToolResult, format OutputFormat) error {
	switch format { //nolint:exhaustive
	case OutputJSON:
		return printJSONResult(w, result)
	case OutputYAML:
		return printYAMLResult(w, result)
	default:
		return printTextResult(w, result)
	}
}

// printTextResult prints the result in human-readable text format.
func printTextResult(w io.Writer, result *mcp.CallToolResult) error {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok && tc.Text != "" {
			fmt.Fprintln(w, tc.Text)
		}
	}

	return nil
}

// printJSONResult prints the result as JSON.
func printJSONResult(w io.Writer, result *mcp.CallToolResult) error {
	var toEncode any

	// Prefer structured content
	if result.StructuredContent != nil {
		toEncode = result.StructuredContent
	}

	if toEncode == nil {
		return nil
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(toEncode)
}

// printYAMLResult prints the result as YAML.
func printYAMLResult(w io.Writer, result *mcp.CallToolResult) error {
	var toEncode any

	// Prefer structured content
	if result.StructuredContent != nil {
		toEncode = result.StructuredContent
	}

	if toEncode == nil {
		return nil
	}

	out, err := yaml.Marshal(toEncode)
	if err != nil {
		return err
	}
	fmt.Fprint(w, string(out))
	return nil
}

// validateRequiredParams checks that all required parameters are provided.
func validateRequiredParams(tool mcp.Tool, args map[string]any) error {
	requiredParams := getRequiredParams(tool)
	if len(requiredParams) == 0 {
		return nil
	}

	missing := []string{}
	for _, param := range requiredParams {
		if _, ok := args[param]; !ok {
			missing = append(missing, param)
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Error: missing required parameter(s): %s\n\n", strings.Join(missing, ", "))
		fmt.Fprintf(&buf, "Usage: mimiops-mcp tools %s ", tool.Name)

		// Show required params as examples
		for i, param := range missing {
			if i > 0 {
				buf.WriteString(" ")
			}
			fmt.Fprintf(&buf, "%s=<value>", param)
		}
		return fmt.Errorf("%s", buf.String())
	}

	return nil
}

// getRequiredParams returns a list of required parameter names from the tool schema.
func getRequiredParams(tool mcp.Tool) []string {
	required := tool.InputSchema.Required
	sort.Strings(required)
	return required
}
