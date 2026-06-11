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

type TuckSecretStatus struct {
	LastSyncTime   time.Time `json:"lastSyncTime,omitempty"`
	LastSyncError  string    `json:"lastSyncError,omitempty"`
	SyncedRevision string    `json:"syncedRevision,omitempty"`
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
