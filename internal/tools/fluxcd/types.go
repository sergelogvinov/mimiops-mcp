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

package toolsfluxcd

import (
	"time"
)

// SourceSummary is the trimmed representation of a Flux source
// (GitRepository or OCIRepository) used by the list tools.
type SourceSummary struct {
	Namespace     string `json:"namespace" jsonschema:"Namespace"`
	Name          string `json:"name" jsonschema:"Name"`
	URL           string `json:"url" jsonschema:"Source URL"`
	Revision      string `json:"revision,omitempty" jsonschema:"Last reconciled artifact revision"`
	Ready         bool   `json:"ready" jsonschema:"Whether the source is ready"`
	Message       string `json:"message,omitempty" jsonschema:"Ready condition message"`
	LastAppliedAt string `json:"lastAppliedAt,omitempty" jsonschema:"Last reconciliation timestamp"`
	Age           string `json:"age" jsonschema:"Age of the resource"`
}

// HelmReleaseSummary is the trimmed representation of a Flux HelmRelease
// used by flux_helmreleases_list.
type HelmReleaseSummary struct {
	Namespace string `json:"namespace" jsonschema:"Namespace"`
	Name      string `json:"name" jsonschema:"Name"`
	Version   string `json:"version,omitempty" jsonschema:"Last applied Helm chart version"`
	Revision  string `json:"revision,omitempty" jsonschema:"Last reconciled revision"`
	Ready     bool   `json:"ready" jsonschema:"Whether the release is ready"`
	Message   string `json:"message,omitempty" jsonschema:"Last condition message"`
	Age       string `json:"age" jsonschema:"Age of the resource"`
}

// KustomizationSummary is the trimmed representation of a Flux Kustomization
// used by flux_kustomizations_list.
type KustomizationSummary struct {
	Namespace string `json:"namespace" jsonschema:"Namespace"`
	Name      string `json:"name" jsonschema:"Name"`
	Revision  string `json:"revision,omitempty" jsonschema:"Last applied revision"`
	Ready     bool   `json:"ready" jsonschema:"Whether the kustomization is ready"`
	Message   string `json:"message,omitempty" jsonschema:"Last condition message"`
	Age       string `json:"age" jsonschema:"Age of the resource"`
}

// ConditionInfo represents information about a resource condition. It mirrors
// tools.ConditionInfo but is declared locally to avoid an import cycle
// (internal/tools imports the tool layer, which imports this package).
type ConditionInfo struct {
	Type    string `json:"type" jsonschema:"Type of the condition"`
	Status  string `json:"status" jsonschema:"Status"`
	Reason  string `json:"reason,omitempty" jsonschema:"Reason"`
	Message string `json:"message,omitempty" jsonschema:"Message description"`
}

// requestedAtTimestamp returns the timestamp written to the
// reconcile.fluxcd.io/requestedAt annotation, matching `flux reconcile`.
func requestedAtTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
