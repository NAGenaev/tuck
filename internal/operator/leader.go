package operator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

const (
	defaultLeaseDuration = 15 * time.Second
	defaultRenewPeriod   = 5 * time.Second
	defaultRetryPeriod   = 2 * time.Second
	leaseTimeFormat      = time.RFC3339Nano
)

// LeaderConfig controls lease acquisition and renewal behavior.
type LeaderConfig struct {
	LeaseName      string
	LeaseNamespace string
	// HolderIdentity uniquely identifies this replica (default: os.Hostname).
	HolderIdentity string
	LeaseDuration  time.Duration // how long a lease is valid without renewal (default 15s)
	RenewPeriod    time.Duration // how often to renew while holding (default 5s)
	RetryPeriod    time.Duration // retry interval when not the leader (default 2s)
}

// LeaderElector implements Kubernetes Lease-based leader election.
// Only one replica holds the lease at a time; others poll until the lease
// expires or is voluntarily released.
type LeaderElector struct {
	cfg  LeaderConfig
	kube *KubeClient
}

// NewLeaderElector builds a LeaderElector, deriving HolderIdentity from
// os.Hostname when cfg.HolderIdentity is empty.
func NewLeaderElector(kube *KubeClient, cfg LeaderConfig) (*LeaderElector, error) {
	if cfg.LeaseDuration == 0 {
		cfg.LeaseDuration = defaultLeaseDuration
	}
	if cfg.RenewPeriod == 0 {
		cfg.RenewPeriod = defaultRenewPeriod
	}
	if cfg.RetryPeriod == 0 {
		cfg.RetryPeriod = defaultRetryPeriod
	}
	if cfg.LeaseName == "" {
		cfg.LeaseName = "tuck-operator-leader"
	}
	if cfg.HolderIdentity == "" {
		h, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("leader election: determine holder identity: %w", err)
		}
		cfg.HolderIdentity = h
	}
	return &LeaderElector{cfg: cfg, kube: kube}, nil
}

// Run blocks until ctx is cancelled. While this instance holds the lease,
// onLeading is invoked with a child context; that context is cancelled when
// the lease is lost (or ctx expires). Run returns ctx.Err() on cancellation.
func (le *LeaderElector) Run(ctx context.Context, onLeading func(ctx context.Context)) error {
	for {
		// Wait until we can acquire the lease.
		if err := le.acquireLoop(ctx); err != nil {
			return err
		}

		slog.Info("operator: leader acquired", "identity", le.cfg.HolderIdentity)

		leadCtx, leadCancel := context.WithCancel(ctx)
		leadDone := make(chan struct{})
		go func() {
			defer close(leadDone)
			onLeading(leadCtx)
		}()

		lost := le.renewLoop(ctx, leadCancel)
		<-leadDone

		if lost {
			slog.Warn("operator: leader lease lost — re-acquiring", "identity", le.cfg.HolderIdentity)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// acquireLoop retries tryAcquire until successful or ctx is cancelled.
func (le *LeaderElector) acquireLoop(ctx context.Context) error {
	for {
		if err := le.tryAcquire(ctx); err == nil {
			return nil
		}
		select {
		case <-time.After(le.cfg.RetryPeriod):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// tryAcquire attempts a single lease acquisition. Returns nil on success.
func (le *LeaderElector) tryAcquire(ctx context.Context) error {
	now := time.Now().UTC()

	lease, err := le.kube.GetLease(ctx, le.cfg.LeaseNamespace, le.cfg.LeaseName)
	if err != nil {
		return fmt.Errorf("get lease: %w", err)
	}

	if lease == nil {
		// Lease doesn't exist yet — create it.
		newLease := le.buildLease(now, now, 0)
		_, err = le.kube.CreateLease(ctx, le.cfg.LeaseNamespace, newLease)
		return err
	}

	holderIsUs := leasePtrStr(lease.Spec.HolderIdentity) == le.cfg.HolderIdentity
	if !holderIsUs && !le.isExpired(lease) {
		return fmt.Errorf("lease held by %s", leasePtrStr(lease.Spec.HolderIdentity))
	}

	transitions := leasePtrInt32(lease.Spec.LeaseTransitions)
	if !holderIsUs {
		transitions++
	}
	lease.Spec = le.buildSpec(now, now, transitions)
	_, err = le.kube.UpdateLease(ctx, le.cfg.LeaseNamespace, lease)
	return err
}

// renewLoop renews the lease periodically. Cancels leadCtx if the lease is
// lost. Returns true when the lease was lost, false when ctx was cancelled.
func (le *LeaderElector) renewLoop(ctx context.Context, leadCancel context.CancelFunc) bool {
	defer leadCancel()
	ticker := time.NewTicker(le.cfg.RenewPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if err := le.tryRenew(ctx); err != nil {
				slog.Warn("operator: failed to renew leader lease — stepping down", "err", err)
				return true
			}
		}
	}
}

// tryRenew updates the RenewTime on the existing lease.
func (le *LeaderElector) tryRenew(ctx context.Context) error {
	now := time.Now().UTC()
	lease, err := le.kube.GetLease(ctx, le.cfg.LeaseNamespace, le.cfg.LeaseName)
	if err != nil || lease == nil {
		return fmt.Errorf("lease disappeared: %v", err)
	}
	if leasePtrStr(lease.Spec.HolderIdentity) != le.cfg.HolderIdentity {
		return fmt.Errorf("lease taken by %s", leasePtrStr(lease.Spec.HolderIdentity))
	}
	acquireTime := leaseParseTime(leasePtrStr(lease.Spec.AcquireTime), now)
	lease.Spec = le.buildSpec(acquireTime, now, leasePtrInt32(lease.Spec.LeaseTransitions))
	_, err = le.kube.UpdateLease(ctx, le.cfg.LeaseNamespace, lease)
	return err
}

func (le *LeaderElector) isExpired(lease *Lease) bool {
	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}
	rt := leaseParseTime(leasePtrStr(lease.Spec.RenewTime), time.Time{})
	if rt.IsZero() {
		return true
	}
	expiry := rt.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
	return time.Now().UTC().After(expiry)
}

func (le *LeaderElector) buildLease(acquireTime, renewTime time.Time, transitions int32) *Lease {
	return &Lease{
		APIVersion: "coordination.k8s.io/v1",
		Kind:       "Lease",
		Metadata:   ObjectMeta{Name: le.cfg.LeaseName, Namespace: le.cfg.LeaseNamespace},
		Spec:       le.buildSpec(acquireTime, renewTime, transitions),
	}
}

func (le *LeaderElector) buildSpec(acquireTime, renewTime time.Time, transitions int32) LeaseSpec {
	dur := int32(le.cfg.LeaseDuration.Seconds())
	at := acquireTime.UTC().Format(leaseTimeFormat)
	rt := renewTime.UTC().Format(leaseTimeFormat)
	return LeaseSpec{
		HolderIdentity:       &le.cfg.HolderIdentity,
		LeaseDurationSeconds: &dur,
		AcquireTime:          &at,
		RenewTime:            &rt,
		LeaseTransitions:     &transitions,
	}
}

// --- helpers ---

func leasePtrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func leasePtrInt32(n *int32) int32 {
	if n == nil {
		return 0
	}
	return *n
}

func leaseParseTime(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	t, err := time.Parse(leaseTimeFormat, s)
	if err != nil {
		return fallback
	}
	return t
}
