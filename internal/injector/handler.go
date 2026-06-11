package injector

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

const maxBody = 1 << 20 // 1 MiB

// Handler is the mutating admission webhook HTTP handler.
type Handler struct {
	// DefaultImage is the tuck-agent container image used when the Pod does not
	// override it via the tuck.io/agent-image annotation.
	DefaultImage string
	log          *slog.Logger
}

// NewHandler returns a Handler that injects tuck-agent using defaultImage.
func NewHandler(defaultImage string, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{DefaultImage: defaultImage, log: log}
}

// ServeHTTP handles POST /mutate — the path registered with the
// MutatingWebhookConfiguration.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	var review AdmissionReview
	if err := json.Unmarshal(body, &review); err != nil {
		http.Error(w, "decode admission review", http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "missing request", http.StatusBadRequest)
		return
	}

	resp := h.mutate(review.Request)
	out := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Response:   resp,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.log.Error("encode admission response", "err", err)
	}
}

func (h *Handler) mutate(req *AdmissionRequest) *AdmissionResponse {
	resp := &AdmissionResponse{UID: req.UID, Allowed: true}

	var pod Pod
	if err := json.Unmarshal(req.Object, &pod); err != nil {
		h.log.Error("decode pod", "uid", req.UID, "err", err)
		return resp // allow but don't inject on parse failure
	}

	cfg, inject := ParseAnnotations(pod.Metadata.Annotations, h.DefaultImage)
	if !inject {
		return resp
	}

	patch, err := BuildPatch(&pod, cfg)
	if err != nil {
		h.log.Error("build patch", "uid", req.UID, "err", err)
		return resp // allow without injection; don't block Pod creation
	}
	if patch == nil {
		// Already injected.
		return resp
	}

	patchType := "JSONPatch"
	resp.PatchType = patchType
	resp.Patch = patch

	h.log.Info("injected tuck-agent", "uid", req.UID, "secrets", cfg.Secrets)
	return resp
}
