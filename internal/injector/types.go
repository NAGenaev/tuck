// Package injector implements the Tuck mutating admission webhook.
// It intercepts Pod creation/updates and injects a tuck-agent init container
// that fetches secrets from the Tuck server and writes them to a shared
// in-memory (tmpfs) volume before the application containers start.
package injector

import "encoding/json"

// Admission webhook wire types — minimal subset of k8s.io/api/admission/v1.
// We avoid importing the full k8s API machinery to keep the binary small.

type AdmissionReview struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Request    *AdmissionRequest  `json:"request,omitempty"`
	Response   *AdmissionResponse `json:"response,omitempty"`
}

type AdmissionRequest struct {
	UID    string          `json:"uid"`
	Object json.RawMessage `json:"object"`
}

type AdmissionResponse struct {
	UID       string `json:"uid"`
	Allowed   bool   `json:"allowed"`
	PatchType string `json:"patchType,omitempty"`
	// Patch is base64-encoded JSON Patch (RFC 6902).
	Patch []byte `json:"patch,omitempty"`
}

// Minimal Pod wire types.

type Pod struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     PodSpec    `json:"spec"`
}

type ObjectMeta struct {
	Annotations map[string]string `json:"annotations,omitempty"`
}

type PodSpec struct {
	InitContainers []Container `json:"initContainers,omitempty"`
	Containers     []Container `json:"containers"`
	Volumes        []Volume    `json:"volumes,omitempty"`
}

type Container struct {
	Name         string        `json:"name"`
	Image        string        `json:"image"`
	Command      []string      `json:"command,omitempty"`
	Args         []string      `json:"args,omitempty"`
	Env          []EnvVar      `json:"env,omitempty"`
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`
	Resources    *Resources    `json:"resources,omitempty"`
	SecurityCtx  *SecurityCtx  `json:"securityContext,omitempty"`
}

type EnvVar struct {
	Name      string         `json:"name"`
	Value     string         `json:"value,omitempty"`
	ValueFrom *EnvVarSource  `json:"valueFrom,omitempty"`
}

type EnvVarSource struct {
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`
}

type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

type Volume struct {
	Name     string       `json:"name"`
	EmptyDir *EmptyDirSrc `json:"emptyDir,omitempty"`
}

type EmptyDirSrc struct {
	Medium string `json:"medium,omitempty"` // "Memory" = tmpfs
}

type Resources struct {
	Limits   map[string]string `json:"limits,omitempty"`
	Requests map[string]string `json:"requests,omitempty"`
}

type SecurityCtx struct {
	RunAsNonRoot bool  `json:"runAsNonRoot,omitempty"`
	RunAsUser    int64 `json:"runAsUser,omitempty"`
	ReadOnlyRFS  bool  `json:"readOnlyRootFilesystem,omitempty"`
	AllowPrivEsc *bool `json:"allowPrivilegeEscalation,omitempty"`
}

// jsonPatch is a single RFC 6902 patch operation.
type jsonPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}
