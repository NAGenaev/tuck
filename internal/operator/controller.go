package operator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultRefreshInterval = 5 * time.Minute
	// reconnectBase is the initial backoff delay after a watch-cycle error.
	reconnectBase = 1 * time.Second
	// reconnectMax caps the exponential backoff.
	reconnectMax = 30 * time.Second
)

// KubeClientIface is the subset of KubeClient used by the controller.
// Having an interface lets tests supply a mock.
type KubeClientIface interface {
	List(ctx context.Context, namespace string) (*TuckSecretList, error)
	Watch(ctx context.Context, namespace, resourceVersion string) (<-chan WatchEvent, error)
	ApplySecret(ctx context.Context, secret *KubeSecret) error
	DeleteSecret(ctx context.Context, namespace, name string) error
	UpdateStatus(ctx context.Context, ts *TuckSecret) error
	AddFinalizer(ctx context.Context, ts *TuckSecret, finalizer string) error
	RemoveFinalizer(ctx context.Context, ts *TuckSecret, finalizer string) error
}

// TuckClientIface is the subset of TuckClient used by the controller.
type TuckClientIface interface {
	GetSecret(ctx context.Context, path string) ([]byte, error)
}

// Controller reconciles TuckSecret resources into K8s Secrets.
type Controller struct {
	kube      KubeClientIface
	tuck      TuckClientIface
	namespace string // empty = all namespaces

	mu      sync.Mutex
	tracked map[string]trackedResource
}

type trackedResource struct {
	ts          TuckSecret
	nextRefresh time.Time
}

// New builds a Controller.
func New(kube KubeClientIface, tuck TuckClientIface, namespace string) *Controller {
	return &Controller{
		kube:      kube,
		tuck:      tuck,
		namespace: namespace,
		tracked:   make(map[string]trackedResource),
	}
}

// Run starts the controller loop and blocks until ctx is cancelled.
// Reconnect delay is exponentially backed off (1s → 30s) on repeated errors
// and reset to the base delay on a successful cycle.
func (ctrl *Controller) Run(ctx context.Context) error {
	slog.Info("operator: starting controller")
	delay := reconnectBase
	for {
		if err := ctrl.runOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("operator: watch cycle error — reconnecting", "err", err, "delay", delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
			delay = min(delay*2, reconnectMax)
		} else {
			delay = reconnectBase // reset on clean exit
		}
	}
}

// runOnce performs one list+watch cycle. It returns when the watch stream
// ends or encounters an irrecoverable error.
func (ctrl *Controller) runOnce(ctx context.Context) error {
	// 1. List all current TuckSecrets to populate local state and get the
	//    resourceVersion to start watching from.
	list, err := ctrl.kube.List(ctx, ctrl.namespace)
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	slog.Info("operator: listed TuckSecrets", "count", len(list.Items))

	for _, ts := range list.Items {
		ctrl.addTracked(ts)
		if ts.Metadata.DeletionTimestamp != nil {
			if err := ctrl.runDeletion(ctx, ts); err != nil {
				slog.Error("operator: deletion", "namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name, "err", err)
			}
			continue
		}
		ctrl.ensureFinalizer(ctx, ts)
		if err := ctrl.reconcile(ctx, ts); err != nil {
			slog.Error("operator: reconcile", "namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name, "err", err)
		}
	}

	// 2. Periodic refresh ticker fires every 30 s; the controller reconciles
	//    any resource whose refreshInterval has elapsed since last sync.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 3. Watch from the resourceVersion returned by the list.
	rv := list.Metadata.ResourceVersion
	watchCh, err := ctrl.kube.Watch(ctx, ctrl.namespace, rv)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-watchCh:
			if !ok {
				// Stream closed — return to trigger re-list.
				return nil
			}
			if err := ctrl.handleEvent(ctx, ev); err != nil {
				slog.Error("operator: handle event", "type", ev.Type, "namespace", ev.Object.Metadata.Namespace, "name", ev.Object.Metadata.Name, "err", err)
			}

		case <-ticker.C:
			ctrl.runDueRefreshes(ctx)
		}
	}
}

// handleEvent dispatches a watch event.
func (ctrl *Controller) handleEvent(ctx context.Context, ev WatchEvent) error {
	switch ev.Type {
	case "ADDED", "MODIFIED":
		ctrl.addTracked(ev.Object)
		// If the object is being deleted (DeletionTimestamp set) run cleanup.
		if ev.Object.Metadata.DeletionTimestamp != nil {
			return ctrl.runDeletion(ctx, ev.Object)
		}
		// Ensure our finalizer is present before syncing.
		if err := ctrl.ensureFinalizer(ctx, ev.Object); err != nil {
			return err
		}
		return ctrl.reconcile(ctx, ev.Object)

	case "DELETED":
		ctrl.removeTracked(ev.Object)
		slog.Info("operator: TuckSecret fully deleted", "namespace", ev.Object.Metadata.Namespace, "name", ev.Object.Metadata.Name)
		return nil

	case "BOOKMARK":
		// Carries a new resourceVersion; nothing else to do.
		return nil

	case "ERROR":
		// The API server sends type=ERROR with a Status object when our
		// resourceVersion has been garbage-collected (HTTP 410 Gone).
		// Returning an error causes runOnce to re-list.
		return fmt.Errorf("watch error event (likely 410 Gone) — re-listing")

	default:
		slog.Warn("operator: unknown event type — ignoring", "type", ev.Type)
		return nil
	}
}

// reconcile fetches the secret value from Tuck and writes it to the K8s Secret.
// It always updates the TuckSecret status via the status subresource on return.
func (ctrl *Controller) reconcile(ctx context.Context, ts TuckSecret) (err error) {
	defer func() {
		ts.Status = buildStatus(ts.Status, err)
		if statusErr := ctrl.kube.UpdateStatus(ctx, &ts); statusErr != nil {
			slog.Warn("operator: update status", "namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name, "err", statusErr)
		}
	}()

	spec := ts.Spec
	if spec.TuckPath == "" || spec.SecretName == "" || spec.SecretKey == "" {
		return fmt.Errorf("invalid TuckSecret spec: tuckPath, secretName, secretKey are all required")
	}

	value, err := ctrl.tuck.GetSecret(ctx, spec.TuckPath)
	if err != nil {
		return fmt.Errorf("fetch tuck secret %q: %w", spec.TuckPath, err)
	}

	secret := &KubeSecret{
		APIVersion: "v1",
		Kind:       "Secret",
		Metadata: ObjectMeta{
			Name:      spec.SecretName,
			Namespace: ts.Metadata.Namespace,
			Labels:    map[string]string{"managed-by": "tuck-operator"},
		},
		Data: map[string][]byte{spec.SecretKey: value},
	}

	if err := ctrl.kube.ApplySecret(ctx, secret); err != nil {
		return fmt.Errorf("apply k8s secret %s/%s: %w",
			ts.Metadata.Namespace, spec.SecretName, err)
	}

	slog.Info("operator: synced TuckSecret", "namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name, "secret", spec.SecretName, "key", spec.SecretKey)

	ctrl.mu.Lock()
	key := resourceKey(ts)
	if r, ok := ctrl.tracked[key]; ok {
		r.nextRefresh = time.Now().Add(ts.Spec.RefreshDuration())
		ctrl.tracked[key] = r
	}
	ctrl.mu.Unlock()

	return nil
}

// buildStatus constructs a new TuckSecretStatus from the current status and
// the outcome of a reconcile attempt. It preserves LastTransitionTime when
// the condition status hasn't changed.
func buildStatus(current TuckSecretStatus, syncErr error) TuckSecretStatus {
	now := time.Now().UTC()
	st := current
	st.LastSyncTime = now

	cond := StatusCondition{
		Type:               ConditionSynced,
		LastTransitionTime: now,
	}
	if syncErr == nil {
		cond.Status = "True"
		cond.Reason = "SyncSucceeded"
		cond.Message = ""
		st.LastSyncError = ""
	} else {
		cond.Status = "False"
		cond.Reason = "SyncFailed"
		cond.Message = syncErr.Error()
		st.LastSyncError = syncErr.Error()
	}
	st.Conditions = setCondition(st.Conditions, cond)
	return st
}

// setCondition upserts c into conditions, preserving LastTransitionTime when
// the status value is unchanged.
func setCondition(conditions []StatusCondition, c StatusCondition) []StatusCondition {
	for i, existing := range conditions {
		if existing.Type == c.Type {
			if existing.Status == c.Status {
				c.LastTransitionTime = existing.LastTransitionTime
			}
			conditions[i] = c
			return conditions
		}
	}
	return append(conditions, c)
}

// runDueRefreshes reconciles all tracked resources whose refresh interval
// has elapsed.
func (ctrl *Controller) runDueRefreshes(ctx context.Context) {
	ctrl.mu.Lock()
	var due []TuckSecret
	now := time.Now()
	for _, r := range ctrl.tracked {
		if now.After(r.nextRefresh) {
			due = append(due, r.ts)
		}
	}
	ctrl.mu.Unlock()

	for _, ts := range due {
		if err := ctrl.reconcile(ctx, ts); err != nil {
			slog.Error("operator: periodic refresh", "namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name, "err", err)
		}
	}
}

func (ctrl *Controller) addTracked(ts TuckSecret) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	ctrl.tracked[resourceKey(ts)] = trackedResource{
		ts:          ts,
		nextRefresh: time.Now().Add(ts.Spec.RefreshDuration()),
	}
}

func (ctrl *Controller) removeTracked(ts TuckSecret) {
	ctrl.mu.Lock()
	defer ctrl.mu.Unlock()
	delete(ctrl.tracked, resourceKey(ts))
}

func resourceKey(ts TuckSecret) string {
	return ts.Metadata.Namespace + "/" + ts.Metadata.Name
}

// ensureFinalizer adds FinalizerCleanup to ts if it's not already present.
func (ctrl *Controller) ensureFinalizer(ctx context.Context, ts TuckSecret) error {
	for _, f := range ts.Metadata.Finalizers {
		if f == FinalizerCleanup {
			return nil
		}
	}
	if ts.Metadata.ResourceVersion == "" {
		return nil // skip in tests without a real cluster
	}
	if err := ctrl.kube.AddFinalizer(ctx, &ts, FinalizerCleanup); err != nil {
		slog.Warn("operator: add finalizer", "namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name, "err", err)
	}
	return nil
}

// runDeletion handles TuckSecret deletion: cleans up the K8s Secret if
// DeletionPolicy=="Delete", then removes our finalizer.
func (ctrl *Controller) runDeletion(ctx context.Context, ts TuckSecret) error {
	ctrl.removeTracked(ts)

	if ts.Spec.DeletionPolicy == "Delete" {
		if err := ctrl.kube.DeleteSecret(ctx, ts.Metadata.Namespace, ts.Spec.SecretName); err != nil {
			slog.Warn("operator: delete K8s Secret on TuckSecret deletion",
				"namespace", ts.Metadata.Namespace, "secret", ts.Spec.SecretName, "err", err)
		} else {
			slog.Info("operator: deleted K8s Secret (DeletionPolicy=Delete)",
				"namespace", ts.Metadata.Namespace, "secret", ts.Spec.SecretName)
		}
	} else {
		slog.Info("operator: TuckSecret deleted — K8s Secret retained (DeletionPolicy=Retain)",
			"namespace", ts.Metadata.Namespace, "name", ts.Metadata.Name)
	}

	if ts.Metadata.ResourceVersion == "" {
		return nil
	}
	return ctrl.kube.RemoveFinalizer(ctx, &ts, FinalizerCleanup)
}
