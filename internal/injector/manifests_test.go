package injector

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// MutatingWebhookConfiguration keeps its webhooks at the top level; nesting
// them under spec makes the API server reject the object, so the injector
// could never be installed.
func TestWebhookManifestsHaveTopLevelWebhooks(t *testing.T) {
	for _, path := range []string{
		"../../deploy/webhook/webhook.yaml",
		"../../deploy/helm/tuck/templates/webhook/webhook.yaml",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := strings.ReplaceAll(string(raw), "\r\n", "\n")
		s = regexp.MustCompile(`(?m)^\s*\{\{.*\}\}\s*$\n?`).ReplaceAllString(s, "")
		s = regexp.MustCompile(`\{\{[^}]*\}\}`).ReplaceAllString(s, "x")
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if _, ok := doc["webhooks"]; !ok {
			t.Errorf("%s: webhooks must be a top-level field", path)
		}
		if _, ok := doc["spec"]; ok {
			t.Errorf("%s: MutatingWebhookConfiguration has no spec", path)
		}
	}
}
