package tools

// OwnerReference describes the owning workload of a pod, mirroring
// metav1.OwnerReference for the JSON/summary output.
type OwnerReference struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// NamespaceSummary is the trimmed representation of a namespace used by namespaces_list.
type NamespaceSummary struct {
	Name   string `json:"name" jsonschema:"Name of the namespace"`
	Status string `json:"status" jsonschema:"Status of the namespace"`
	Age    string `json:"age" jsonschema:"Age of the namespace"`
}

// LimitRangeSummary is the trimmed representation of a limit range used by limitranges_list.
type LimitRangeSummary struct {
	Name      string `json:"name" jsonschema:"Name of the LimitRange"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the LimitRange"`
	Types     string `json:"types" jsonschema:"Resource types in the LimitRange"`
	Age       string `json:"age" jsonschema:"Age of the LimitRange"`
}

// ResourceQuotaSummary is the trimmed representation of a resource quota used by resourcequotas_list.
type ResourceQuotaSummary struct {
	Name           string `json:"name" jsonschema:"Name of the resource quota"`
	Namespace      string `json:"namespace" jsonschema:"Namespace of the resource quota"`
	RequestsCPU    string `json:"requests_cpu,omitempty" jsonschema:"CPU requests (used/hard)"`
	RequestsMemory string `json:"requests_memory,omitempty" jsonschema:"Memory requests (used/hard)"`
	LimitsCPU      string `json:"limits_cpu,omitempty" jsonschema:"CPU limits (used/hard)"`
	LimitsMemory   string `json:"limits_memory,omitempty" jsonschema:"Memory limits (used/hard)"`
	Age            string `json:"age" jsonschema:"Age of the resource quota"`
}

// PriorityClassSummary is the trimmed representation of a priority class used by priorityclasses_list.
type PriorityClassSummary struct {
	Name          string `json:"name" jsonschema:"Name of the priority class"`
	Value         int32  `json:"value" jsonschema:"Value of the priority class"`
	GlobalDefault bool   `json:"global_default" jsonschema:"Whether this is the global default priority class"`
	Description   string `json:"description" jsonschema:"Description of the priority class"`
	Age           string `json:"age" jsonschema:"Age of the priority class"`
}

// StorageClassSummary is the trimmed representation of a storage class used by storageclasses_list.
type StorageClassSummary struct {
	Name                 string `json:"name" jsonschema:"Name of the storage class"`
	Provisioner          string `json:"provisioner" jsonschema:"Provisioner of the storage class"`
	ReclaimPolicy        string `json:"reclaim_policy" jsonschema:"Reclaim policy of the storage class"`
	VolumeBindingMode    string `json:"volume_binding_mode" jsonschema:"Volume binding mode of the storage class"`
	AllowVolumeExpansion bool   `json:"allow_volume_expansion" jsonschema:"Whether volume expansion is allowed"`
	Age                  string `json:"age" jsonschema:"Age of the storage class"`
}

// EventSummary is the trimmed representation of an event used by events_get.
type EventSummary struct {
	Namespace string `json:"namespace,omitempty" jsonschema:"Namespace of the event"`
	LastSeen  string `json:"last_seen" jsonschema:"Last seen time of the event"`
	Type      string `json:"type" jsonschema:"Type of the event (Normal or Warning)"`
	Reason    string `json:"reason" jsonschema:"Reason for the event"`
	Object    string `json:"object" jsonschema:"Object involved in the event"`
	Message   string `json:"message" jsonschema:"Message of the event"`
}

// NodeSummary is the trimmed representation of a node used by nodes_list.
type NodeSummary struct {
	Name           string           `json:"name" jsonschema:"Name of the node"`
	Status         string           `json:"status" jsonschema:"Status of the node"`
	Roles          []string         `json:"roles" jsonschema:"Roles of the node"`
	Age            string           `json:"age" jsonschema:"Age of the node"`
	KubeletVersion string           `json:"kubelet_version" jsonschema:"Kubelet version of the node"`
	ImageVersion   string           `json:"image_version" jsonschema:"OS image version of the node"`
	InternalIP     string           `json:"internal_ip" jsonschema:"Internal IP address of the node"`
	Capacity       NodeCapacityInfo `json:"capacity" jsonschema:"Capacity of the node"`
}

// NodeCapacityInfo is the trimmed representation of a node's capacity used by nodes_list.
type NodeCapacityInfo struct {
	CPU    string `json:"cpu,omitempty" jsonschema:"CPU capacity of the node"`
	Memory string `json:"memory,omitempty" jsonschema:"Memory capacity of the node"`
	Pods   int    `json:"pods,omitempty" jsonschema:"Maximum number of pods the node can run"`
}

// WorkloadSummary is the trimmed, agent-friendly representation of a workload
// (Deployment, StatefulSet, or DaemonSet) used by workloads_list.
type WorkloadSummary struct {
	Kind      string `json:"kind" jsonschema:"Kind of the workload (Deployment, StatefulSet, or DaemonSet)"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the workload"`
	Name      string `json:"name" jsonschema:"Name of the workload"`
	Ready     string `json:"ready,omitempty" jsonschema:"Number of ready replicas of the workload"`
	Desired   int    `json:"desired" jsonschema:"Number of desired replicas of the workload"`
	Age       string `json:"age,omitempty" jsonschema:"Age of the workload"`
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

// WorkloadDescribe is the structured output for workloads_describe in JSON format.
type WorkloadDescribe struct {
	Kind           string          `json:"kind"`
	Namespace      string          `json:"namespace"`
	Name           string          `json:"name"`
	Replicas       Replicas        `json:"replicas"`
	Selector       string          `json:"selector"`
	Service        string          `json:"service,omitempty"`
	UpdateStrategy string          `json:"update_strategy"`
	Conditions     []ConditionInfo `json:"conditions,omitempty"`
	PodTemplate    PodTemplate     `json:"pod_template"`
	Age            string          `json:"age"`
}

// CronJobSummary is the trimmed, agent-friendly representation of a CronJob
// used by cronjobs_list (and available in the JSON output of other cronjob tools).
type CronJobSummary struct {
	Namespace    string `json:"namespace" jsonschema:"Namespace of the CronJob"`
	Name         string `json:"name" jsonschema:"Name of the CronJob"`
	Schedule     string `json:"schedule" jsonschema:"Schedule of the CronJob"`
	Suspend      bool   `json:"suspend" jsonschema:"Whether the CronJob is suspended"`
	Status       string `json:"status" jsonschema:"Status of the CronJob"`
	LastSchedule string `json:"lastSchedule,omitempty" jsonschema:"Last schedule time of the CronJob"`
	Age          string `json:"age" jsonschema:"Age of the CronJob"`
}

// JobSummary is the trimmed, agent-friendly representation of a Job used by
// jobs_list (and available in the JSON output of other job tools).
type JobSummary struct {
	Namespace   string `json:"namespace" jsonschema:"Namespace of the Job"`
	Name        string `json:"name" jsonschema:"Name of the Job"`
	Completions string `json:"completions,omitempty" jsonschema:"Completions of the Job"`
	Duration    string `json:"duration,omitempty" jsonschema:"Duration of the Job"`
	Age         string `json:"age" jsonschema:"Age of the Job"`
	Status      string `json:"status" jsonschema:"Status of the Job"`
}

// PodSummary is the trimmed, agent-friendly representation of a pod used by
// pods_list (and available in the JSON output of other pod tools).
type PodSummary struct {
	Namespace       string           `json:"namespace"`
	Name            string           `json:"name"`
	Ready           string           `json:"ready"`
	Restarts        int32            `json:"restarts"`
	Age             string           `json:"age"`
	Status          string           `json:"status"`
	Node            string           `json:"node"`
	OwnerReferences []OwnerReference `json:"ownerReferences,omitempty"`
}

// PodTemplate represents the pod template specification.
type PodTemplate struct {
	Labels         map[string]string `json:"labels"`
	Containers     []ContainerInfo   `json:"containers"`
	RestartPolicy  string            `json:"restart_policy"`
	ServiceAccount string            `json:"service_account,omitempty"`
}

// PodInfo represents information about a pod.
type PodInfo struct {
	Name      string `json:"name" jsonschema:"Name of the pod"`
	Phase     string `json:"phase" jsonschema:"Phase of the pod"`
	Ready     string `json:"ready" jsonschema:"Ready status (e.g., 1/2)"`
	StartTime string `json:"startTime,omitempty" jsonschema:"Start time of the pod"`
}

// ContainerInfo represents information about a container.
type ContainerInfo struct {
	Name  string  `json:"name" jsonschema:"Name of the container"`
	Image string  `json:"image" jsonschema:"Image of the container"`
	Ports []int32 `json:"ports,omitempty" jsonschema:"List of ports exposed by the container"`
}

// Replicas represents ready and desired replica counts.
type Replicas struct {
	Ready   int `json:"ready"`
	Desired int `json:"desired"`
}

// ConditionInfo represents information about a Job condition.
type ConditionInfo struct {
	Type    string `json:"type" jsonschema:"Type of the condition"`
	Status  string `json:"status" jsonschema:"Status of the condition"`
	Reason  string `json:"reason,omitempty" jsonschema:"Reason for the condition"`
	Message string `json:"message,omitempty" jsonschema:"Message describing the condition"`
}

// LogStream is one pod/container log stream.
type LogStream struct {
	Pod       string `json:"pod" jsonschema:"Name of the pod"`
	Container string `json:"container" jsonschema:"Name of the container"`
	Logs      string `json:"logs" jsonschema:"Raw log output from the container"`
}
