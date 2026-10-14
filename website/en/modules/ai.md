# AI & Agents

The **ai service** covers model configuration, the agent kernel and the embedded worker: encrypted model keys, full run traces, usage and quotas.

## Capabilities

- **Model management**: provider / base_url / model / capabilities; API keys are stored with **AES-256-GCM** (ciphertext and nonce separated), every list/detail response masks them as `********`.
- **Duplicate-name protection**: model names are unique; create/rename check for conflicts and propagate lookup failures (only `not found` counts as free).
- **Agent kernel**: definitions (agent_key `^[a-z][a-z0-9_-]{1,63}$`) → versions (instructions / model / tools / max_steps / timeout) → runs (run / steps / events persisted).
- **Idempotent runs**: `Idempotency-Key` reuses the original run for the same input and answers 409 for conflicting input.
- **Embedded worker**: the worker runs inside the ai-service process — there is **no standalone worker container**; run requests are logged as `agent-run-<id>` with token/cost/latency usage.
- **Usage & quotas**: `ai_usage_logs` / `ai_quotas` per model and tenant; platform admins see all tenants, regular users are pinned to their own.

## Auth & data scope

Model and run APIs go through the gateway (`ai:agent:*` permissions) with strict tenant scoping. Plaintext keys never leave the service.

## Boundaries

- 🧪 External model vendors are user-configured; the repository stores no vendor credentials.
- Isolated verification uses a local protocol fixture — **not** external vendor qualification.
