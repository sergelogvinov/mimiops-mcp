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

// LimitRangeLimit is one entry of a LimitRange's spec.limits.
type LimitRangeLimit struct {
	Type                 string            `json:"type" jsonschema:"Type of the limit (Container, Pod, or PersistentVolumeClaim)"`
	Min                  map[string]string `json:"min,omitempty" jsonschema:"Min resource constraints"`
	Max                  map[string]string `json:"max,omitempty" jsonschema:"Max resource constraints"`
	Default              map[string]string `json:"default,omitempty" jsonschema:"Default resource constraints"`
	DefaultRequest       map[string]string `json:"defaultRequest,omitempty" jsonschema:"Default request resource constraints"`
	MaxLimitRequestRatio map[string]string `json:"maxLimitRequestRatio,omitempty" jsonschema:"Max limit to request ratio"`
}

// LimitRangeSpec is the trimmed representation of a LimitRange spec.
type LimitRangeSpec struct {
	Limits []LimitRangeLimit `json:"limits" jsonschema:"List of limits"`
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

// ResourceQuota is the representation of a resource quota's resources.
type ResourceQuota struct {
	Resource string `json:"resource" jsonschema:"Resource name"`
	Used     string `json:"used" jsonschema:"Used quantity"`
	Hard     string `json:"hard" jsonschema:"Hard limit"`
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
	Type      string `json:"type" jsonschema:"Type of the event (Normal or Warning)"`
	Reason    string `json:"reason" jsonschema:"Reason for the event"`
	FirstSeen string `json:"first_seen,omitempty" jsonschema:"First seen time of the event"`
	Age       string `json:"age,omitempty" jsonschema:"Last seen time of the event"`
	Object    string `json:"object,omitempty" jsonschema:"Object involved in the event"`
	Message   string `json:"message" jsonschema:"Message of the event"`
}

// NodeSummary is the trimmed representation of a node used by nodes_list.
type NodeSummary struct {
	NodeCapacityInfo

	Name           string   `json:"name" jsonschema:"Name of the node"`
	Status         string   `json:"status" jsonschema:"Status of the node"`
	Roles          []string `json:"roles" jsonschema:"Roles of the node"`
	Age            string   `json:"age" jsonschema:"Age of the node"`
	KubeletVersion string   `json:"kubelet_version" jsonschema:"Kubelet version of the node"`
	ImageVersion   string   `json:"image_version" jsonschema:"OS image version of the node"`
	InternalIP     string   `json:"internal_ip" jsonschema:"Internal IP address of the node"`
}

// NodeSpec is the trimmed representation of a node spec used by nodes_get.
type NodeSpec struct {
	Unschedulable bool        `json:"unschedulable" jsonschema:"Whether the node is unschedulable"`
	Taints        []TaintInfo `json:"taints,omitempty" jsonschema:"List of taints"`
}

// NodeCapacityInfo is the trimmed representation of a node's capacity used by nodes_list.
type NodeCapacityInfo struct {
	CPU    string `json:"cpu,omitempty" jsonschema:"CPU capacity of the node"`
	Memory string `json:"memory,omitempty" jsonschema:"Memory capacity of the node"`
	Pods   int    `json:"pods,omitempty" jsonschema:"Maximum number of pods the node can run"`
}

// NodeAddressInfo represents a node address.
type NodeAddressInfo struct {
	Type    string `json:"type" jsonschema:"Type of the address"`
	Address string `json:"address" jsonschema:"Address"`
}

// NodePodInfo is a pod running on a node with its resource requests and limits.
type NodePodInfo struct {
	Namespace      string `json:"namespace" jsonschema:"Namespace of the pod"`
	Name           string `json:"name" jsonschema:"Name of the pod"`
	Phase          string `json:"phase" jsonschema:"Phase of the pod"`
	CPURequests    string `json:"cpu_requests,omitempty" jsonschema:"CPU requests of the pod"`
	CPULimits      string `json:"cpu_limits,omitempty" jsonschema:"CPU limits of the pod"`
	MemoryRequests string `json:"memory_requests,omitempty" jsonschema:"Memory requests of the pod"`
	MemoryLimits   string `json:"memory_limits,omitempty" jsonschema:"Memory limits of the pod"`
}

// WorkloadSummary is the trimmed, agent-friendly representation of a workload
// (Deployment, StatefulSet, or DaemonSet) used by workloads_list.
type WorkloadSummary struct {
	Kind      string `json:"kind" jsonschema:"Kind of the workload (Deployment, StatefulSet, or DaemonSet)"`
	Namespace string `json:"namespace" jsonschema:"Namespace of the workload"`
	Name      string `json:"name" jsonschema:"Name of the workload"`
	Ready     string `json:"ready,omitempty" jsonschema:"Ready status (e.g., 1/2)"`
	Desired   int    `json:"desired" jsonschema:"Number of desired replicas of the workload"`
	Age       string `json:"age,omitempty" jsonschema:"Age of the workload"`
}

// WorkloadSpec is the trimmed representation of a workload spec used by workloads_get and workloads_describe.
type WorkloadSpec struct {
	PodSpec

	Selector       string `json:"selector,omitempty" jsonschema:"Selector of the workload"`
	UpdateStrategy string `json:"update_strategy,omitempty" jsonschema:"Update strategy of the workload"`
}

// CronJobSummary is the trimmed, agent-friendly representation of a CronJob
// used by cronjobs_list (and available in the JSON output of other cronjob tools).
type CronJobSummary struct {
	Namespace    string `json:"namespace" jsonschema:"namespace"`
	Name         string `json:"name" jsonschema:"name"`
	Schedule     string `json:"schedule" jsonschema:"schedule"`
	Suspend      bool   `json:"suspend" jsonschema:"suspend"`
	LastSchedule string `json:"lastSchedule,omitempty" jsonschema:"lastSchedule"`
	Age          string `json:"age" jsonschema:"age"`
	Status       string `json:"status" jsonschema:"status"`
}

// CronJobSpec is the trimmed representation of a CronJob spec used by cronjobs_get.
type CronJobSpec struct {
	PodSpec

	Selector                   string `json:"selector,omitempty" jsonschema:"Selector of the jobs and pods created by the CronJob"`
	StartingDeadlineSeconds    string `json:"startingDeadlineSeconds,omitempty" jsonschema:"startingDeadlineSeconds"`
	SuccessfulJobsHistoryLimit string `json:"successfulJobsHistoryLimit,omitempty" jsonschema:"successfulJobsHistoryLimit"`
	FailedJobsHistoryLimit     string `json:"failedJobsHistoryLimit,omitempty" jsonschema:"failedJobsHistoryLimit"`
	ConcurrencyPolicy          string `json:"concurrencyPolicy,omitempty" jsonschema:"concurrencyPolicy"`
}

// JobSummary is the trimmed, agent-friendly representation of a Job used by
// jobs_list (and available in the JSON output of other job tools).
type JobSummary struct {
	Namespace   string `json:"namespace" jsonschema:"namespace"`
	Name        string `json:"name" jsonschema:"name"`
	Completions string `json:"completions,omitempty" jsonschema:"completions"`
	Duration    string `json:"duration,omitempty" jsonschema:"duration"`
	Age         string `json:"age" jsonschema:"age"`
	Status      string `json:"status" jsonschema:"status"`
}

// JobSpec is the trimmed representation of a Job spec used by jobs_get.
type JobSpec struct {
	PodSpec

	Parallelism           string `json:"parallelism,omitempty" jsonschema:"parallelism"`
	BackoffLimit          string `json:"backoffLimit,omitempty" jsonschema:"backoffLimit"`
	ActiveDeadlineSeconds string `json:"activeDeadlineSeconds,omitempty" jsonschema:"activeDeadlineSeconds"`
}

// ServiceSummary is the trimmed, agent-friendly representation of a service
// used by services_list (and available in the JSON output of other service tools).
type ServiceSummary struct {
	Name       string     `json:"name" jsonschema:"Name of the service"`
	Namespace  string     `json:"namespace" jsonschema:"Namespace of the service"`
	Type       string     `json:"type" jsonschema:"Type of the service (ClusterIP, NodePort, LoadBalancer, ExternalName)"`
	ClusterIP  string     `json:"cluster_ip" jsonschema:"Cluster IP address"`
	ExternalIP string     `json:"external_ip,omitempty" jsonschema:"External IP address"`
	Ports      []PortInfo `json:"ports" jsonschema:"List of ports"`
	Selector   string     `json:"selector" jsonschema:"Label selector as comma-separated string"`
	Age        string     `json:"age" jsonschema:"Age of the service"`
}

// PodSummary is the trimmed, agent-friendly representation of a pod used by
// pods_list (and available in the JSON output of other pod tools).
type PodSummary struct {
	Namespace       string           `json:"namespace,omitempty" jsonschema:"Namespace"`
	Name            string           `json:"name" jsonschema:"Name"`
	Ready           string           `json:"ready" jsonschema:"Ready status (e.g., 1/2)"`
	Restarts        int32            `json:"restarts" jsonschema:"Restarts"`
	Age             string           `json:"age" jsonschema:"Age"`
	Status          string           `json:"status" jsonschema:"Status"`
	Node            string           `json:"node" jsonschema:"Node"`
	OwnerReferences []OwnerReference `json:"ownerReferences,omitempty" jsonschema:"Owner References"`
}

// PodSpec is the trimmed representation of a pod spec used by pods_get and jobs_get and workloads_get.
type PodSpec struct {
	RestartPolicy     string            `json:"restart_policy,omitempty" jsonschema:"restart policy"`
	ServiceAccount    string            `json:"service_account,omitempty" jsonschema:"ServiceAccount"`
	PriorityClassName string            `json:"priority_class_name,omitempty" jsonschema:"PriorityClassName"`
	InitContainers    []ContainerInfo   `json:"init_containers,omitempty" jsonschema:"List of init containers"`
	Containers        []ContainerInfo   `json:"containers" jsonschema:"List of containers"`
	Volumes           []string          `json:"volumes,omitempty" jsonschema:"List of volumes names"`
	NodeSelector      map[string]string `json:"nodeSelector,omitempty" jsonschema:"Node selector"`
	Tolerations       []TolerationInfo  `json:"tolerations,omitempty" jsonschema:"Tolerations"`
	QOSClass          string            `json:"qos_class,omitempty" jsonschema:"Quality of Service class"`
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
	Name  string   `json:"name" jsonschema:"Name"`
	Image string   `json:"image" jsonschema:"Image"`
	Ports []string `json:"ports,omitempty" jsonschema:"List of ports"`
}

// ConditionInfo represents information about a Job condition.
type ConditionInfo struct {
	Type    string `json:"type" jsonschema:"Type of the condition"`
	Status  string `json:"status" jsonschema:"Status"`
	Reason  string `json:"reason,omitempty" jsonschema:"Reason"`
	Message string `json:"message,omitempty" jsonschema:"Message description"`
}

// TaintInfo represents a node taint.
type TaintInfo struct {
	Key    string `json:"key" jsonschema:"Key"`
	Value  string `json:"value,omitempty" jsonschema:"Value"`
	Effect string `json:"effect" jsonschema:"Effect"`
}

// TolerationInfo represents a pod toleration.
type TolerationInfo struct {
	Key      string `json:"key" jsonschema:"Key"`
	Operator string `json:"operator" jsonschema:"Operator"`
	Value    string `json:"value,omitempty" jsonschema:"Value"`
	Effect   string `json:"effect" jsonschema:"Effect"`
}

// LogStream is one pod/container log stream.
type LogStream struct {
	Pod       string `json:"pod" jsonschema:"Name of the pod"`
	Container string `json:"container" jsonschema:"Name of the container"`
	Logs      string `json:"logs" jsonschema:"Raw log output from the container"`
}

// PortInfo represents information about a service port.
type PortInfo struct {
	Name string `json:"name,omitempty" jsonschema:"Name of the port"`
	Port string `json:"port" jsonschema:"Port number/(TCP/UDP)"`
}

// EndpointInfo represents information about a service endpoint.
type EndpointInfo struct {
	IP        string `json:"ip" jsonschema:"Endpoint IP address"`
	Port      string `json:"port" jsonschema:"Endpoint port"`
	TargetRef string `json:"target_ref,omitempty" jsonschema:"Reference to the target pod"`
	NodeName  string `json:"node_name,omitempty" jsonschema:"Node where the endpoint is located"`
	Ready     bool   `json:"ready" jsonschema:"Whether the endpoint is ready"`
}
