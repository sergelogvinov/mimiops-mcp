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
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/server"
	"github.com/sergelogvinov/mimiops-mcp/internal/k8s"
	toolshelm "github.com/sergelogvinov/mimiops-mcp/internal/tools/helm"
)

// Extension describes a named, optional group of tools.
type Extension struct {
	Name     string
	Register func(srv *server.MCPServer, mc *k8s.MultiClusterClient, allowDestructive bool)
}

// extensionsRegistry is the catalog of available extensions. Core tools are not
// listed here — they are always registered by registerCore.
var extensionsRegistry = []Extension{
	{Name: "helm", Register: registerHelm},
	// future: {Name: "fluxcd", Register: registerFluxCD},
}

// RegisterTools wires the core tools and the requested extensions into the MCP
// server. extensions is the raw --extensions value ("all" expands to every
// registered extension). All activation logic lives here.
func RegisterTools(srv *server.MCPServer, mc *k8s.MultiClusterClient, extensions string, allowDestructive bool) error {
	registerCore(srv, mc, allowDestructive)

	names, err := ResolveExtensions(extensions)
	if err != nil {
		return err
	}

	for _, name := range names {
		for _, ext := range extensionsRegistry {
			if ext.Name == name {
				ext.Register(srv, mc, allowDestructive)
				break
			}
		}
	}
	return nil
}

// ResolveExtensions expands the raw --extensions value into a list of extension
// names. "all" returns every registered extension. Unknown names are
// a startup error so typos surface at startup.
func ResolveExtensions(raw string) ([]string, error) {
	if raw == "all" {
		names := make([]string, 0, len(extensionsRegistry))
		for _, ext := range extensionsRegistry {
			names = append(names, ext.Name)
		}
		return names, nil
	}

	var names []string
	for name := range strings.SplitSeq(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !isRegistered(name) {
			return nil, fmt.Errorf("unknown extension %q (available: %s)", name, strings.Join(availableExtensions(), ", "))
		}
		names = append(names, name)
	}
	return names, nil
}

func isRegistered(name string) bool {
	for _, ext := range extensionsRegistry {
		if ext.Name == name {
			return true
		}
	}
	return false
}

func availableExtensions() []string {
	names := make([]string, 0, len(extensionsRegistry))
	for _, ext := range extensionsRegistry {
		names = append(names, ext.Name)
	}
	return names
}

// registerCore wires all core (always-on) tools into the MCP server.
// Core tools are never gated by --extensions.
func registerCore(srv *server.MCPServer, mc *k8s.MultiClusterClient, allowDestructive bool) {
	if mc.IsMultiCluster() {
		RegisterClustersList(srv, mc)
	}
	RegisterClustersDescribe(srv, mc)
	RegisterPodsList(srv, mc)
	RegisterPodsGet(srv, mc)
	RegisterPodsDescribe(srv, mc)
	RegisterPodsLog(srv, mc)
	RegisterJobsList(srv, mc)
	RegisterJobsGet(srv, mc)
	RegisterJobsDescribe(srv, mc)
	RegisterJobsLog(srv, mc)
	RegisterJobsCreate(srv, mc)
	RegisterCronJobsList(srv, mc)
	RegisterCronJobsGet(srv, mc)
	RegisterCronJobsDescribe(srv, mc)
	RegisterNodesList(srv, mc)
	RegisterNodesGet(srv, mc)
	RegisterNamespacesList(srv, mc)
	RegisterNamespacesGet(srv, mc)
	RegisterResourceQuotasList(srv, mc)
	RegisterResourceQuotasGet(srv, mc)
	RegisterLimitRangesList(srv, mc)
	RegisterLimitRangesGet(srv, mc)
	RegisterStorageClassesList(srv, mc)
	RegisterPriorityClassesList(srv, mc)
	RegisterEventsGet(srv, mc)
	RegisterWorkloadsList(srv, mc)
	RegisterWorkloadsGet(srv, mc)
	RegisterWorkloadsDescribe(srv, mc)
	RegisterServicesList(srv, mc)
	RegisterServicesDescribe(srv, mc)
	if allowDestructive {
		RegisterPodsDelete(srv, mc)
		RegisterJobsDelete(srv, mc)
		RegisterCronJobsSuspend(srv, mc)
		RegisterCronJobsResume(srv, mc)
		RegisterWorkloadsScale(srv, mc)
	}
}

// registerHelm wires all Helm tools into the MCP server.
func registerHelm(srv *server.MCPServer, mc *k8s.MultiClusterClient, allowDestructive bool) {
	toolshelm.RegisterHelmList(srv, mc)
	toolshelm.RegisterHelmStatus(srv, mc)
	if allowDestructive {
		toolshelm.RegisterHelmRollback(srv, mc)
	}
}
