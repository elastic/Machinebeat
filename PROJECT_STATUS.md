# Machinebeat — Project Status

**Last Updated:** 2026-02-02

## Overview
Elastic Machinebeat is a custom beat for industrial protocols (OPC UA, MQTT, PLC4X). Goal: modernize dependencies and target **Beats v9.x**.

## Current State

### Git Status
- **Branch:** master
- **Target:** Beats v9.2.4 (aligned with elastic/beats v9 branch)
- **Changes:**
    - `go.mod` adjusted to target Beats v9 (using pseudo-version `v7.0.0-alpha2...` mapped to v9.2.4 commit `fd909e2bd441`).
    - `replace` directives aligned with Beats v9.
    - Source code imports updated: `github.com/elastic/beats/v7/libbeat/logp` -> `github.com/elastic/elastic-agent-libs/logp` (moved in v9).
    - `go.sum` updated via `go mod tidy`.

### Versions Detected (Local Files)
| Component | go.mod Version |
|-----------|----------------|
| Go | 1.24.11 (Toolchain: 1.24.12) |
| libbeat | v7.0.0-alpha2... (v9.2.4 commit fd909e2) |
| elastic-agent-libs | v0.26.2 |
| paho.mqtt | v1.5.1 |
| golang.org/x/tools | v0.40.0 |

### Build Status
- ✅ **SUCCESS:** `go mod tidy` completed successfully. Dependencies are resolved.
- ❌ **Build:** `go build ./...` fails: `common.MapStr` undefined in `module/mqtt/topic/client.go` and `module/opcua/nodevalue/nodevalue.go`.
- ⚠️ `mage` not installed (optional).

## Upgrade Plan

### Phase 1: Dependency Update (Complete)
- [x] Adjust `go.mod` for Beats v9.x target.
- [x] Run `go mod tidy` successfully.
- [x] Fix `libbeat/logp` import paths.

### Phase 2: Build & Test
- [ ] Fix `common.MapStr` usage (replace with v9 equivalent, e.g. `mapstr.M` from `elastic-agent-libs/mapstr`).
- [ ] Re-run `go build` / `mage build`.

## Notes
- `go.mod` uses a pseudo-version for `beats/v7` and a `replace` directive to point to the v9 commit, because `beats` v9 still declares `v7` module path but tags are `v9.x.x`.
- `libbeat/logp` moved to `elastic-agent-libs/logp`, requiring code changes.
