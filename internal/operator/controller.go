package operator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	defaultRefreshInterval = 5 * time.Minute
	// watchReconnectDelay is how long to wait before re-listing after a watch
	// error (410 Gone, network drop, etc.).
	watchReconnectDelay = 5 * time.Second
)

// KubeClientIface is the subset of KubeClient used by the controller.
// Having an interface lets tests supply a mock.
type KubeClientIface interface {
	List(ctx context.Context, namespace string) (*TuckSecretList, error)
	Watch(ctx context.Context, namespace, resourceVersion string) (<-chan WatchEvent, error)
	ApplySecret(ctx context.Context, secret *KubeSecret) error
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
func (ctrl *Controller) Run(ctx context.Context) error {
	log.Println("operator: starting controller")
	for {
		if err := ctrl.runOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("operator: watch cycle error: %v — reconnecting in %s", err, watchReconnectDelay)
			select {
			case <-time.After(watchReconnectDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
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
	log.Printf("operator: listed %d TuckSecret(s)", len(list.Items))

	for _, ts := range list.Items {
		ctrl.addTracked(ts)
		if err := ctrl.reconcile(ctx, ts); err != nil {
			log.Printf("operator: reconcile %s/%s: %v",
				ts.Metadata.Namespace, ts.Metadata.Name, err)
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
				log.Printf("operator: handle event %q for %s/%s: %v",
					ev.Type, ev.Object.Metadata.Namespace, ev.Object.Metadata.Name, err)
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
		return ctrl.reconcile(ctx, ev.Object)

	case "DELETED":
		ctrl.removeTracked(ev.Object)
		// The operator does NOT delete the K8s Secret on TuckSecret deletion.
		// It only manages Secret content, not its lifecycle. This is
		// conservative: the user deletes the K8s Secret explicitly if desired.
		log.Printf("operator: TuckSecret %s/%s deleted — K8s Secret left in place",
			ev.Object.Metadata.Namespace, ev.Object.Metadata.Name)
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
		log.Printf("operator: unknown event type %q — ignoring", ev.Type)
		return nil
	}
}

// reconcile fetches the secret value from Tuck and writes it to the K8s Secret.
func (ctrl *Controller) reconcile(ctx context.Context, ts TuckSecret) error {
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

	log.Printf("operator: synced %s/%s → k8s secret %s/%s[%s]",
		ts.Metadata.Namespace, ts.Metadata.Name,
		ts.Metadata.Namespace, spec.SecretName, spec.SecretKey)

	ctrl.mu.Lock()
	key := resourceKey(ts)
	if r, ok := ctrl.tracked[key]; ok {
		r.nextRefresh = time.Now().Add(ts.Spec.RefreshDuration())
		ctrl.tracked[key] = r
	}
	ctrl.mu.Unlock()

	return nil
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
			log.Printf("operator: periodic refresh %s/%s: %v",
				ts.Metadata.Namespace, ts.Metadata.Name, err)
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
