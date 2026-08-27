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

package toolshelm

// ReleaseSummary is the trimmed representation of a Helm release used by helm_list.
type ReleaseSummary struct {
	Name         string `json:"name" jsonschema:"Helm release"`
	Namespace    string `json:"namespace" jsonschema:"Namespace"`
	Revision     int    `json:"revision" jsonschema:"Revision number"`
	Age          string `json:"age" jsonschema:"Last updated age"`
	Updated      string `json:"updated" jsonschema:"Last updated time"`
	Status       string `json:"status" jsonschema:"Status (deployed, failed, pending, etc.)"`
	Description  string `json:"description" jsonschema:"Description"`
	ChartName    string `json:"chart" jsonschema:"Chart name"`
	ChartVersion string `json:"chart_version" jsonschema:"Chart version"`
	AppVersion   string `json:"app_version" jsonschema:"Application version deployed"`
}

// RollbackResult represents the result of a helm_rollback operation.
type RollbackResult struct {
	Name             string `json:"name" jsonschema:"Name of the rolled back Helm release"`
	Namespace        string `json:"namespace" jsonschema:"Namespace of the rolled back Helm release"`
	PreviousRevision int    `json:"previous_revision" jsonschema:"Previous revision number before rollback"`
	NewRevision      int    `json:"new_revision" jsonschema:"New revision number after rollback"`
	Status           string `json:"status" jsonschema:"Status after rollback"`
}
