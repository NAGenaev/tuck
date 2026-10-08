package injector

import (
	"encoding/base64"
	"strings"
	"testing"
)

func patchJSON(t *testing.T, pod *Pod, cfg InjectConfig) string {
	t.Helper()
	enc, err := BuildPatch(pod, cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(string(enc))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCAConfigMapInjected(t *testing.T) {
	ann := map[string]string{
		AnnotationInject:      "true",
		AnnotationSecrets:     "team/db:db",
		AnnotationAuth:        "kubernetes",
		AnnotationCAConfigMap: "tuck-ca",
	}
	cfg, ok := ParseAnnotations(ann, "agent:1")
	if !ok || cfg.CAConfigMap != "tuck-ca" {
		t.Fatalf("CAConfigMap not parsed: %+v", cfg)
	}
	pod := &Pod{Spec: PodSpec{Containers: []Container{{Name: "app", Image: "x"}}}}
	p := patchJSON(t, pod, cfg)
	for _, want := range []string{`"configMap"`, `"name":"tuck-ca"`, `"key":"ca.crt"`, `"TUCK_CA_FILE"`, `/tuck/ca/ca.crt`} {
		if !strings.Contains(p, want) {
			t.Errorf("patch missing %s:\n%s", want, p)
		}
	}
}

func TestNoCAConfigMapByDefault(t *testing.T) {
	cfg, _ := ParseAnnotations(map[string]string{AnnotationInject: "true", AnnotationSecrets: "a:b", AnnotationAuth: "kubernetes"}, "agent:1")
	p := patchJSON(t, &Pod{Spec: PodSpec{Containers: []Container{{Name: "app"}}}}, cfg)
	if strings.Contains(p, "TUCK_CA_FILE") || strings.Contains(p, "configMap") {
		t.Errorf("CA wiring must be opt-in:\n%s", p)
	}
}
