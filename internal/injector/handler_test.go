package injector

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func admissionBody(t *testing.T, pod Pod) []byte {
	t.Helper()
	podRaw, err := json.Marshal(pod)
	if err != nil {
		t.Fatal(err)
	}
	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-1",
			Object: json.RawMessage(podRaw),
		},
	}
	b, err := json.Marshal(review)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func callWebhook(t *testing.T, h *Handler, body []byte) AdmissionReview {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestNoInjectAnnotation(t *testing.T) {
	h := NewHandler("ghcr.io/nagenaev/tuck-agent:latest", nil)
	pod := Pod{
		Metadata: ObjectMeta{Annotations: map[string]string{}},
		Spec:     PodSpec{Containers: []Container{{Name: "app", Image: "nginx"}}},
	}
	resp := callWebhook(t, h, admissionBody(t, pod))
	if !resp.Response.Allowed {
		t.Fatal("expected allowed")
	}
	if resp.Response.Patch != nil {
		t.Fatal("expected no patch for pod without inject annotation")
	}
}

func TestInjectBasic(t *testing.T) {
	h := NewHandler("ghcr.io/nagenaev/tuck-agent:latest", nil)
	pod := Pod{
		Metadata: ObjectMeta{
			Annotations: map[string]string{
				AnnotationInject:  "true",
				AnnotationAddr:    "https://tuck.svc:8200",
				AnnotationSecrets: "db/password:password,db/user:user",
			},
		},
		Spec: PodSpec{
			Containers: []Container{{Name: "app", Image: "nginx"}},
		},
	}
	resp := callWebhook(t, h, admissionBody(t, pod))
	if !resp.Response.Allowed {
		t.Fatal("expected allowed")
	}
	if resp.Response.Patch == nil {
		t.Fatal("expected patch")
	}

	raw, err := base64.StdEncoding.DecodeString(string(resp.Response.Patch))
	if err != nil {
		t.Fatalf("decode patch base64: %v", err)
	}

	var patches []jsonPatch
	if err := json.Unmarshal(raw, &patches); err != nil {
		t.Fatalf("unmarshal patches: %v", err)
	}
	if len(patches) == 0 {
		t.Fatal("expected non-empty patch")
	}

	// Verify init container is in the patch.
	hasInitContainer := false
	for _, p := range patches {
		if p.Path == "/spec/initContainers" || p.Path == "/spec/initContainers/-" {
			hasInitContainer = true
		}
	}
	if !hasInitContainer {
		t.Fatalf("expected initContainers patch; got %+v", patches)
	}
}

func TestInjectIdempotent(t *testing.T) {
	h := NewHandler("ghcr.io/nagenaev/tuck-agent:latest", nil)
	// Pod already has the tuck-agent init container injected.
	pod := Pod{
		Metadata: ObjectMeta{
			Annotations: map[string]string{
				AnnotationInject:  "true",
				AnnotationSecrets: "db/password:password",
			},
		},
		Spec: PodSpec{
			InitContainers: []Container{{Name: agentContainerName, Image: "tuck-agent:v1"}},
			Containers:     []Container{{Name: "app", Image: "nginx"}},
		},
	}
	resp := callWebhook(t, h, admissionBody(t, pod))
	if !resp.Response.Allowed {
		t.Fatal("expected allowed")
	}
	if resp.Response.Patch != nil {
		t.Fatal("expected no patch on already-injected pod")
	}
}

func TestParseSecretsList(t *testing.T) {
	specs := ParseSecretsList("db/password:db-pass,db/user:db-user,api/key")
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}
	if specs[0].Path != "db/password" || specs[0].Filename != "db-pass" {
		t.Fatalf("spec 0 mismatch: %+v", specs[0])
	}
	if specs[2].Path != "api/key" || specs[2].Filename != "key" {
		t.Fatalf("spec 2 filename default mismatch: %+v", specs[2])
	}
}

func TestInjectCustomOutputDir(t *testing.T) {
	h := NewHandler("ghcr.io/nagenaev/tuck-agent:latest", nil)
	pod := Pod{
		Metadata: ObjectMeta{
			Annotations: map[string]string{
				AnnotationInject:    "true",
				AnnotationSecrets:   "db/pass:pass",
				AnnotationOutputDir: "/custom/secrets",
			},
		},
		Spec: PodSpec{Containers: []Container{{Name: "app", Image: "app:v1"}}},
	}
	resp := callWebhook(t, h, admissionBody(t, pod))
	if resp.Response.Patch == nil {
		t.Fatal("expected patch")
	}
	raw, _ := base64.StdEncoding.DecodeString(string(resp.Response.Patch))
	if !bytes.Contains(raw, []byte("/custom/secrets")) {
		t.Fatalf("expected /custom/secrets in patch, got: %s", raw)
	}
}

func TestInjectExistingVolumes(t *testing.T) {
	h := NewHandler("ghcr.io/nagenaev/tuck-agent:latest", nil)
	pod := Pod{
		Metadata: ObjectMeta{
			Annotations: map[string]string{
				AnnotationInject:  "true",
				AnnotationSecrets: "db/pass:pass",
			},
		},
		Spec: PodSpec{
			Volumes:    []Volume{{Name: "existing-vol", EmptyDir: &EmptyDirSrc{}}},
			Containers: []Container{{Name: "app", Image: "app:v1"}},
		},
	}
	resp := callWebhook(t, h, admissionBody(t, pod))
	if resp.Response.Patch == nil {
		t.Fatal("expected patch")
	}
	raw, _ := base64.StdEncoding.DecodeString(string(resp.Response.Patch))

	var patches []jsonPatch
	json.Unmarshal(raw, &patches)

	// When volumes already exist, we should use /spec/volumes/- not /spec/volumes.
	for _, p := range patches {
		if p.Path == "/spec/volumes" {
			t.Fatalf("expected /spec/volumes/- for existing volumes, got /spec/volumes")
		}
	}
}
