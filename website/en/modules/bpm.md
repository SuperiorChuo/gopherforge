# Workflow (BPM)

A **home-grown lightweight approval engine** — no Flowable/BPMN dependency; a native Go "single-cursor node tree + task expander" with capabilities matching mainstream workflow products.

## Capability matrix

| Dimension | Support |
|------|------|
| Node types | Start / approval / CC / conditional branch (exclusive, nestable) |
| Assignee rules | Users / roles / department leader (initiator's or form-field department) / initiator-selected |
| Multi-approver | AND (all) / OR (any approves, all must reject) / sequential |
| Actions | Approve / reject / transfer / return-to-initiator (edit & resubmit) / return-to-previous (runtime-path aware) / add-sign / delegate / cancel |
| Admin | Force terminate (incl. suspended instances), all-instances view, approval analytics — platform admins only |
| Timeout | Per-node hours + expiry action: remind / **auto-approve** / **auto-reject** (executed as system, logged; scanner interval `BPM_TIMEOUT_SCAN_INTERVAL`, default 5 m) |
| Fallbacks | Empty-assignee strategies (auto-pass / fallback users / suspend); condition evaluation failure suspends instead of mis-routing |
| Versioning | Multi-version definitions; in-flight instances freeze their version |

## Walkthrough: ship a leave-request flow

Create a definition → declare form fields (type select, date range, reason textarea) → add an approval node with the *department leader* rule (set leaders on departments first), then an HR node by *role* → publish (full-tree validation, old version archived, in-flight instances unaffected) → employees start it from "Start Request" → approvers act from their todo list. Available actions are computed **server-side** per node/state/actor.

## Two form modes

**Flow forms (no-code)**: declare fields in the definition and publish — leave/expense flows without a line of code. Eight field types: `input`/`textarea` (≤2000 chars), `number` (min/max), `amount` (**stored in cents**, entered in yuan), `select`/`radio` (value must be an option), `date` (`YYYY-MM-DD`), `switch`. Snapshots are validated server-side against the schema (required/type/options/range; undeclared fields stripped).

**Business forms (deep integration)**: the business backend starts instances via internal endpoints and receives terminal-state callbacks.

```text
1. Start:    POST /api/v1/bpm/internal/instances   (X-Internal-Token)
2. Callback: register BPM_CALLBACK_<BIZTYPE>=<url>; handle results idempotently
3. Lookup:   GET /api/v1/bpm/internal/instances/by-biz?biz_type=&biz_id=
```

The user-facing start endpoint only accepts flow-form definitions (business anchors are server-generated), so business approvals cannot be forged. With `BPM_INTERNAL_TOKEN` unset the internal endpoints return **503** (fail-closed); token comparison is constant-time.

## Task actions & constraints

| Action | Constraint |
|--------|------------|
| Approve | comment optional |
| Reject | **comment required**; per-node on-reject routing (terminal / back to initiator) |
| Transfer | task stays pending under the new assignee; original assignee recorded |
| Return to previous | requires node `allowBackPrev` + actual approval history |
| Add-sign | adds approvers to the current node, joins its multi-approver rule; **not available in sequential mode** |
| Delegate | hand to a delegatee (not yourself); on resolve the task **returns to the delegator** |
| Cancel | initiator only, before the first approval acts |

CC records (`GET /api/v1/bpm/cc/my`) support unread filtering and idempotent read marks.

## Field permissions

Approval nodes can hide specific form fields (e.g. salary) — filtered **server-side** across task detail, instance detail and the diagram.

## Endpoint quick reference

Definitions `GET/POST /api/v1/bpm/definitions` (+ `/publish`, `/new-version`, `/suspend`) · start `GET /startable`, `POST /instances` · mine `GET /instances/my`, `/tasks/todo`, `/tasks/done` · instance detail with `/timeline` and `/diagram` · admin `GET /instances`, `POST /instances/:id/terminate`, `GET /stats`. BPM validates Bearer JWTs itself (no ForwardAuth); menu visibility uses `bpm:definition:*` permission codes.

## Data model

Five self-managed tables, all tenant-scoped: `bpm_process_definition` (node tree + form schema, JSONB), `bpm_process_instance` (snapshot; a partial unique index on `(tenant, biz_type, biz_id, running)` prevents duplicates), `bpm_task` (rounds, add-sign/delegate fields), `bpm_cc_record`, `bpm_process_log` (operator 0 = system).

## Analytics & notifications

Status distribution, 30-day trend, per-definition approval rate & average duration, **Top-10 node bottlenecks**. Todo/CC/timeout/terminal notifications go through templates — silently skipped until `NOTIFY_API_BASE` + token are configured.
