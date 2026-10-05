package operator

import (
	"fmt"
	"time"
)

// TuckSecret is the Go representation of the tuck.io/v1alpha1 TuckSecret CRD.
type TuckSecret struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   ObjectMeta       `json:"metadata"`
	Spec       TuckSecretSpec   `json:"spec"`
	Status     TuckSecretStatus `json:"status,omitempty"`
}

type TuckSecretSpec struct {
	TuckPath   string `json:"tuckPath,omitempty"`
	SecretName string `json:"secretName"`
	SecretKey  string `json:"secretKey,omitempty"`
	// Data maps additional Secret keys to Tuck paths, so a single TuckSecret
	// can populate several keys of one K8s Secret. It may be combined with
	// tuckPath/secretKey.
	Data            map[string]string `json:"data,omitempty"`
	TuckServer      string            `json:"tuckServer,omitempty"`
	RefreshInterval string            `json:"refreshInterval,omitempty"`
	// DeletionPolicy controls what happens to the K8s Secret when the TuckSecret
	// is deleted. "Retain" (default) leaves the K8s Secret in place. "Delete"
	// removes it together with the TuckSecret.
	DeletionPolicy string `json:"deletionPolicy,omitempty"`
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

// FinalizerCleanup is added to every TuckSecret. When deletion is requested the
// controller removes the managed K8s Secret (if DeletionPolicy=="Delete") then
// strips the finalizer so Kubernetes can garbage-collect the TuckSecret.
const FinalizerCleanup = "tuck.io/cleanup"

type ObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	DeletionTimestamp *string           `json:"deletionTimestamp,omitempty"`
	Finalizers        []string          `json:"finalizers,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
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
	Type       string            `json:"type,omitempty"`
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

// Entries returns the Secret key -> Tuck path mapping declared by the spec:
// the legacy tuckPath/secretKey pair plus every item of data. It reports an
// error when the spec declares nothing, a key is empty, a path is empty, or the
// legacy pair collides with a data key.
func (s TuckSecretSpec) Entries() (map[string]string, error) {
	if (s.TuckPath == "") != (s.SecretKey == "") {
		return nil, fmt.Errorf("tuckPath and secretKey must be set together")
	}
	out := make(map[string]string, len(s.Data)+1)
	if s.TuckPath != "" {
		out[s.SecretKey] = s.TuckPath
	}
	for k, p := range s.Data {
		if k == "" || p == "" {
			return nil, fmt.Errorf("data: key and tuck path must both be non-empty")
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("secret key %q is declared twice (secretKey and data)", k)
		}
		out[k] = p
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("spec must declare tuckPath+secretKey or at least one data entry")
	}
	return out, nil
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
