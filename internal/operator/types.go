package operator

import "time"

// TuckSecret is the Go representation of the tuck.io/v1alpha1 TuckSecret CRD.
type TuckSecret struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   ObjectMeta       `json:"metadata"`
	Spec       TuckSecretSpec   `json:"spec"`
	Status     TuckSecretStatus `json:"status,omitempty"`
}

type TuckSecretSpec struct {
	TuckPath        string `json:"tuckPath"`
	SecretName      string `json:"secretName"`
	SecretKey       string `json:"secretKey"`
	TuckServer      string `json:"tuckServer,omitempty"`
	RefreshInterval string `json:"refreshInterval,omitempty"`
}

// StatusConditionType identifies a condition on a TuckSecret.
type StatusConditionType = string

const (
	ConditionSynced StatusConditionType = "Synced"
	ConditionReady  StatusConditionType = "Ready"
)

// StatusCondition mirrors the standard Kubernetes condition pattern.
type StatusCondition struct {
	Type               StatusConditionType `json:"type"`
	Status             string              `json:"status"` // "True" | "False" | "Unknown"
	LastTransitionTime time.Time           `json:"lastTransitionTime"`
	Reason             string              `json:"reason,omitempty"`
	Message            string              `json:"message,omitempty"`
}

type TuckSecretStatus struct {
	LastSyncTime   time.Time         `json:"lastSyncTime,omitempty"`
	LastSyncError  string            `json:"lastSyncError,omitempty"`
	SyncedRevision string            `json:"syncedRevision,omitempty"`
	Conditions     []StatusCondition `json:"conditions,omitempty"`
}

type ObjectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
}

// TuckSecretList is returned by the k8s list API.
type TuckSecretList struct {
	APIVersion string       `json:"apiVersion"`
	Kind       string       `json:"kind"`
	Metadata   ListMeta     `json:"metadata"`
	Items      []TuckSecret `json:"items"`
}

type ListMeta struct {
	ResourceVersion string `json:"resourceVersion"`
}

// WatchEvent wraps an event from the k8s watch stream.
type WatchEvent struct {
	Type   string     `json:"type"` // "ADDED", "MODIFIED", "DELETED", "ERROR"
	Object TuckSecret `json:"object"`
}

// KubeSecret is a minimal representation of a K8s v1/Secret.
type KubeSecret struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   ObjectMeta        `json:"metadata"`
	Data       map[string][]byte `json:"data,omitempty"`
}

// Lease is the coordination.k8s.io/v1 Lease resource used for leader election.
type Lease struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   ObjectMeta `json:"metadata"`
	Spec       LeaseSpec  `json:"spec"`
}

// LeaseSpec mirrors the K8s coordination.k8s.io/v1 LeaseSpec.
type LeaseSpec struct {
	HolderIdentity       *string `json:"holderIdentity,omitempty"`
	LeaseDurationSeconds *int32  `json:"leaseDurationSeconds,omitempty"`
	AcquireTime          *string `json:"acquireTime,omitempty"` // RFC3339Nano
	RenewTime            *string `json:"renewTime,omitempty"`   // RFC3339Nano
	LeaseTransitions     *int32  `json:"leaseTransitions,omitempty"`
}

// RefreshDuration parses the spec's refreshInterval or returns the default.
func (s TuckSecretSpec) RefreshDuration() time.Duration {
	if s.RefreshInterval == "" {
		return 5 * time.Minute
	}
	d, err := time.ParseDuration(s.RefreshInterval)
	if err != nil {
		return 5 * time.Minute
	}
	return d
}
