package tools

// OwnerReference describes the owning workload of a pod, mirroring
// metav1.OwnerReference for the JSON/summary output.
type OwnerReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// PodSummary is the trimmed, agent-friendly representation of a pod used by
// pods_list (and available in the JSON output of other pod tools).
type PodSummary struct {
	Namespace       string           `json:"namespace"`
	Name            string           `json:"name"`
	Ready           string           `json:"ready"`
	Status          string           `json:"status"`
	Restarts        int32            `json:"restarts"`
	Age             string           `json:"age"`
	Node            string           `json:"node"`
	OwnerReferences []OwnerReference `json:"ownerReferences,omitempty"`
}
