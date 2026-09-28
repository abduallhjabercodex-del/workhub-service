# ADR-005: Deterministic Feature Flags & Emergency Kill Switch

* **Status:** Accepted
* **Date:** 2026-09-20
* **Decision Makers:** Product Engineering, Release Management, SRE

---

## Context
Releasing new business features to 100% of production users simultaneously introduces heightened operational risk. If a defect manifests under production traffic, redeploying or rolling back container images requires several minutes, exposing users to extended failure windows.

## Decision
Implement a low-overhead, in-memory feature flagging engine in [internal/infrastructure/featureflag/flags.go](file:///home/abdullah-jaber/Desktop/WorkHup/workhub-dev/internal/infrastructure/featureflag/flags.go):
1. **Deterministic Gradual Rollout:** Evaluates user eligibility via `SHA-256(flagName + ":" + entityKey) % 100 < percentage`, ensuring consistent user experiences across requests without session affinity dependencies.
2. **Instant Emergency Kill Switch:** Exposes `TriggerKillSwitch()` allowing operators to disable an errant feature globally in sub-milliseconds without triggering container restarts or pipeline redeployments.

## Consequences & Trade-offs
* **Positive:** Granular blast radius control during feature rollouts; near-zero incident MTTR via instantaneous kill switches.
* **Negative:** Requires rigorous technical debt discipline to prune obsolete flag checks following successful releases.

## Verification Commands
```bash
go test -v -race ./internal/featureflag/...
```
