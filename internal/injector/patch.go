package injector

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// AnnotationInject enables Tuck secret injection on a Pod.
	AnnotationInject = "tuck.io/inject"
	// AnnotationAddr is the Tuck server HTTP address.
	AnnotationAddr = "tuck.io/addr"
	// AnnotationSecrets is a comma-separated list of "tuckPath:filename" pairs.
	// Example: "db/password:db-password,db/user:db-user"
	AnnotationSecrets = "tuck.io/secrets"
	// AnnotationTokenSecret is the name of the K8s Secret whose "token" key holds
	// the Tuck bearer token. Defaults to "tuck-token".
	AnnotationTokenSecret = "tuck.io/token-secret"
	// AnnotationImage overrides the tuck-agent container image.
	AnnotationImage = "tuck.io/agent-image"
	// AnnotationOutputDir overrides the secrets output directory inside the Pod.
	// Defaults to "/tuck/secrets".
	AnnotationOutputDir = "tuck.io/output-dir"
	// AnnotationInsecure skips TLS verification for the Tuck server.
	AnnotationInsecure = "tuck.io/insecure"

	defaultSecretsDir  = "/tuck/secrets"
	defaultTokenSecret = "tuck-token"
	secretsVolumeName  = "tuck-secrets"
	tokenVolumeName    = "tuck-token"
	agentContainerName = "tuck-agent"
)

// InjectConfig is derived from a Pod's annotations.
type InjectConfig struct {
	Addr        string
	Secrets     string // raw annotation value
	TokenSecret string
	Image       string
	OutputDir   string
	Insecure    bool
}

// ParseAnnotations extracts injection config from pod annotations.
// Returns (config, true) when injection is requested, (zero, false) otherwise.
func ParseAnnotations(ann map[string]string, defaultImage string) (InjectConfig, bool) {
	if ann[AnnotationInject] != "true" {
		return InjectConfig{}, false
	}
	cfg := InjectConfig{
		Addr:        ann[AnnotationAddr],
		Secrets:     ann[AnnotationSecrets],
		TokenSecret: ann[AnnotationTokenSecret],
		Image:       ann[AnnotationImage],
		OutputDir:   ann[AnnotationOutputDir],
		Insecure:    ann[AnnotationInsecure] == "true",
	}
	if cfg.TokenSecret == "" {
		cfg.TokenSecret = defaultTokenSecret
	}
	if cfg.Image == "" {
		cfg.Image = defaultImage
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = defaultSecretsDir
	}
	return cfg, true
}

// BuildPatch returns a base64-encoded JSON Patch (RFC 6902) that injects the
// tuck-agent init container and shared tmpfs volume into the Pod.
// alreadyInjected prevents double-injection on updates.
func BuildPatch(pod *Pod, cfg InjectConfig) ([]byte, error) {
	// Check idempotency — skip if already injected.
	for _, ic := range pod.Spec.InitContainers {
		if ic.Name == agentContainerName {
			return nil, nil
		}
	}

	var patches []jsonPatch

	// 1. Shared secrets tmpfs volume.
	secretsVol := Volume{
		Name:     secretsVolumeName,
		EmptyDir: &EmptyDirSrc{Medium: "Memory"},
	}
	if len(pod.Spec.Volumes) == 0 {
		patches = append(patches, jsonPatch{Op: "add", Path: "/spec/volumes", Value: []Volume{secretsVol}})
	} else {
		patches = append(patches, jsonPatch{Op: "add", Path: "/spec/volumes/-", Value: secretsVol})
	}

	// 2. tuck-agent init container.
	falseVal := false
	agent := Container{
		Name:  agentContainerName,
		Image: cfg.Image,
		Env:   buildEnv(cfg),
		VolumeMounts: []VolumeMount{
			{Name: secretsVolumeName, MountPath: cfg.OutputDir},
		},
		Resources: &Resources{
			Limits:   map[string]string{"cpu": "100m", "memory": "64Mi"},
			Requests: map[string]string{"cpu": "10m", "memory": "16Mi"},
		},
		SecurityCtx: &SecurityCtx{
			RunAsNonRoot: true,
			RunAsUser:    65532,
			ReadOnlyRFS:  true,
			AllowPrivEsc: &falseVal,
		},
	}

	// Mount token from K8s Secret as a file.
	agent.VolumeMounts = append(agent.VolumeMounts, VolumeMount{
		Name:      tokenVolumeName,
		MountPath: "/tuck/token",
		ReadOnly:  true,
	})
	tokenVol := Volume{
		Name: tokenVolumeName,
		EmptyDir: nil,
	}
	// Use a projected secret volume for the token.
	// We encode it as a raw JSON object since we're not importing k8s API types.
	tokenVolRaw := map[string]interface{}{
		"name": tokenVolumeName,
		"secret": map[string]interface{}{
			"secretName": cfg.TokenSecret,
			"items": []map[string]interface{}{
				{"key": "token", "path": "token"},
			},
		},
	}
	_ = tokenVol

	patches = append(patches, jsonPatch{Op: "add", Path: "/spec/volumes/-", Value: tokenVolRaw})

	if len(pod.Spec.InitContainers) == 0 {
		patches = append(patches, jsonPatch{Op: "add", Path: "/spec/initContainers", Value: []Container{agent}})
	} else {
		patches = append(patches, jsonPatch{Op: "add", Path: "/spec/initContainers/-", Value: agent})
	}

	// 3. Mount the secrets volume into every app container (read-only).
	mount := VolumeMount{
		Name:      secretsVolumeName,
		MountPath: cfg.OutputDir,
		ReadOnly:  true,
	}
	for i, c := range pod.Spec.Containers {
		if len(c.VolumeMounts) == 0 {
			patches = append(patches, jsonPatch{
				Op:    "add",
				Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts", i),
				Value: []VolumeMount{mount},
			})
		} else {
			patches = append(patches, jsonPatch{
				Op:    "add",
				Path:  fmt.Sprintf("/spec/containers/%d/volumeMounts/-", i),
				Value: mount,
			})
		}
	}

	raw, err := json.Marshal(patches)
	if err != nil {
		return nil, fmt.Errorf("marshal patches: %w", err)
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(encoded, raw)
	return encoded, nil
}

// ParseSecretsList parses the tuck.io/secrets annotation value into
// (tuckPath, filename) pairs. Format: "path/to/secret:filename,...".
func ParseSecretsList(s string) []SecretSpec {
	var out []SecretSpec
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		sp := SecretSpec{Path: parts[0]}
		if len(parts) == 2 {
			sp.Filename = parts[1]
		} else {
			// Default filename: last path component.
			segs := strings.Split(parts[0], "/")
			sp.Filename = segs[len(segs)-1]
		}
		out = append(out, sp)
	}
	return out
}

// SecretSpec is a (tuckPath, filename) pair from the tuck.io/secrets annotation.
type SecretSpec struct {
	Path     string
	Filename string
}

func buildEnv(cfg InjectConfig) []EnvVar {
	env := []EnvVar{
		{Name: "TUCK_OUTPUT_DIR", Value: cfg.OutputDir},
		{Name: "TUCK_SECRETS", Value: cfg.Secrets},
		{Name: "TUCK_TOKEN_FILE", Value: "/tuck/token/token"},
	}
	if cfg.Addr != "" {
		env = append(env, EnvVar{Name: "TUCK_ADDR", Value: cfg.Addr})
	}
	if cfg.Insecure {
		env = append(env, EnvVar{Name: "TUCK_INSECURE", Value: "true"})
	}
	return env
}
