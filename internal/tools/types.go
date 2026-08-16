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

// WorkloadSummary is the trimmed, agent-friendly representation of a workload
// (Deployment, StatefulSet, or DaemonSet) used by workloads_list.
type WorkloadSummary struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Ready     string `json:"ready"`
	Desired   int    `json:"desired"`
	Age       string `json:"age"`
}

// WorkloadDetails is the detailed representation of a workload used by
// workloads_get and workloads_describe in JSON format.
type WorkloadDetails struct {
	Kind      string         `json:"kind"`
	Namespace string         `json:"namespace"`
	Name      string         `json:"name"`
	Replicas  Replicas       `json:"replicas"`
	Selector  string         `json:"selector"`
	Service   string         `json:"service,omitempty"`
	Strategy  string         `json:"update_strategy,omitempty"`
	Age       string         `json:"age"`
	Spec      map[string]any `json:"spec"`
	Status    map[string]any `json:"status"`
}

// Replicas represents ready and desired replica counts.
type Replicas struct {
	Ready   int `json:"ready"`
	Desired int `json:"desired"`
}

// ScaleResult represents the result of a scale operation.
type ScaleResult struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Replicas  int    `json:"replicas"`
}

// WorkloadDescribe is the structured output for workloads_describe in JSON format.
type WorkloadDescribe struct {
	Kind            string      `json:"kind"`
	Namespace       string      `json:"namespace"`
	Name            string      `json:"name"`
	Replicas        Replicas    `json:"replicas"`
	Selector        string      `json:"selector"`
	Service         string      `json:"service,omitempty"`
	UpdateStrategy  string      `json:"update_strategy"`
	Conditions      []Condition `json:"conditions,omitempty"`
	UpdateHistory   int         `json:"update_history_limit,omitempty"`
	RevisionHistory int         `json:"revision_history_limit,omitempty"`
	PodTemplate     PodTemplate `json:"pod_template"`
	Age             string      `json:"age"`
}

// Condition represents a workload condition.
type Condition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// PodTemplate represents the pod template specification.
type PodTemplate struct {
	Labels         map[string]string `json:"labels"`
	Containers     []Container       `json:"containers"`
	RestartPolicy  string            `json:"restart_policy"`
	ServiceAccount string            `json:"service_account,omitempty"`
}

// Container represents a container in the pod template.
type Container struct {
	Name  string   `json:"name"`
	Image string   `json:"image"`
	Ports []int32  `json:"ports,omitempty"`
	Args  []string `json:"args,omitempty"`
}
