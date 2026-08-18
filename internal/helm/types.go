package helm

// ReleaseSummary is the trimmed representation of a Helm release used by helm_list.
type ReleaseSummary struct {
	Name         string `json:"name" jsonschema:"Name of the Helm release"`
	Namespace    string `json:"namespace" jsonschema:"Namespace of the Helm release"`
	Revision     int    `json:"revision" jsonschema:"Revision number of the Helm release"`
	Updated      string `json:"updated" jsonschema:"Last updated time of the Helm release"`
	Status       string `json:"status" jsonschema:"Status of the Helm release (deployed, failed, pending, etc.)"`
	ChartName    string `json:"chart" jsonschema:"Chart name (e.g., nginx)"`
	ChartVersion string `json:"chart_version" jsonschema:"Chart version (e.g., 1.2.3)"`
	AppVersion   string `json:"app_version" jsonschema:"Application version deployed by the Helm release"`
}

// ReleaseStatus is the detailed representation of a Helm release used by helm_status.
type ReleaseStatus struct {
	Name         string         `json:"name" jsonschema:"Name of the Helm release"`
	Namespace    string         `json:"namespace" jsonschema:"Namespace of the Helm release"`
	Revision     int            `json:"revision" jsonschema:"Current revision number"`
	Status       string         `json:"status" jsonschema:"Current status (deployed, failed, pending, etc.)"`
	LastDeployed string         `json:"last_deployed" jsonschema:"Last deployed time"`
	Description  string         `json:"description" jsonschema:"Description message from the Helm release"`
	History      []HistoryEntry `json:"history" jsonschema:"Last 3 revisions of the Helm release"`
}

// HistoryEntry represents a single revision in the Helm release history.
type HistoryEntry struct {
	Revision    int    `json:"revision" jsonschema:"Revision number"`
	Updated     string `json:"updated" jsonschema:"Updated time"`
	Status      string `json:"status" jsonschema:"Status (deployed, superseded, failed, pending, etc.)"`
	Chart       string `json:"chart" jsonschema:"Chart name and version"`
	AppVersion  string `json:"app_version" jsonschema:"Application version"`
	Description string `json:"description" jsonschema:"Description message"`
}

// RollbackResult represents the result of a helm_rollback operation.
type RollbackResult struct {
	Name             string `json:"name" jsonschema:"Name of the rolled back Helm release"`
	Namespace        string `json:"namespace" jsonschema:"Namespace of the rolled back Helm release"`
	PreviousRevision int    `json:"previous_revision" jsonschema:"Previous revision number before rollback"`
	NewRevision      int    `json:"new_revision" jsonschema:"New revision number after rollback"`
	Status           string `json:"status" jsonschema:"Status after rollback"`
}
