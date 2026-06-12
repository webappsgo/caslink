# TODO.AI.md — Caslink Outstanding Work

Tracks remaining unimplemented or spec-violating items.
Items are removed once fully implemented and committed.

---

## PART 10 — Database Schema: sessions table names

The spec (PART 10/PART 11) requires:
- `admin_sessions` in server.db for admin web sessions
- `user_sessions` in users.db for regular-user web sessions

**Current state:** a unified `sessions` table with a `user_type TEXT`
column lives in users.db and serves both admin and user sessions.

**Functional impact:** sessions work correctly; the column correctly
scopes lookups. The schema name deviates from the spec.

**Fix required:** rename to separate tables and split the auth service.
This is a large refactor — auth.go, middleware, and all session-related
callers must be updated. Schema migration needed.

---

## PART 10 — Missing `metrics` field in server.db `nodes` table

The spec requires a `nodes` table in server.db for cluster heartbeats.
Currently not implemented — single-node only, no cluster node tracking.

---

## PART 21 — /metrics request counters not hooked to Prometheus

The `requests_total` and `active_connections` fields in `/server/healthz`
stats come from in-memory atomics (correct). The Prometheus `/metrics`
endpoint uses a separate set of counters via `s.metrics.Middleware`.
These two counter sets are not unified — Prometheus has richer
per-method/per-path labels, the health atomics are single counters.

No action required for spec compliance (both are correct per their specs);
note here in case they need to be unified.

---

## PART 28 — CI/CD Workflows

User said "No — leave empty for now" (session 2026-06-04).
Workflows are intentionally absent.

---

## Federation (Out-of-scope for v1)

`FederationConfig` struct present; no service, no `/.well-known/federation`,
no discovery or sync. Deferred.

---

Last refreshed: 2026-06-12
