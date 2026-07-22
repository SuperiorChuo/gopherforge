# Workflow (BPM)

A **home-grown lightweight approval engine** — no Flowable/BPMN dependency; a native Go "single-cursor node tree + task expander" with capabilities matching mainstream workflow products.

## Capability matrix

| Dimension | Support |
|------|------|
| Node types | Start / approval / CC / conditional branch (exclusive, nestable) |
| Assignee rules | Users / roles / department leader (initiator's or form-field department) / initiator-selected |
| Multi-approver | AND (all) / OR (any approves, all must reject) / sequential |
| Actions | Approve / reject / transfer / return-to-initiator (edit & resubmit) / return-to-previous (runtime-path aware) / cancel |
| Admin | Force terminate (incl. suspended instances), all-instances view, approval analytics |
| Timeout | Per-node hours + expiry action: remind / **auto-approve** / **auto-reject** (executed as system, logged) |
| Fallbacks | Empty-assignee strategies (auto-pass / fallback users / suspend); condition evaluation failure suspends instead of mis-routing |
| Versioning | Multi-version definitions; in-flight instances freeze their version |

## DingTalk-style designer

A vertical card-flow configurator (not a free canvas): insert approval/CC/condition nodes between cards; branches fan out into columns with a fixed default branch; the condition editor offers declared form fields with AND/OR groups and 7 operators.

## Two form modes

**Flow forms (no-code)**: declare fields in the definition ("Form Design", 8 field types, money stored in cents), publish, and users start from the "Start Request" page — leave/expense flows without a line of code. Snapshots are validated server-side against the schema.

**Business forms (deep integration)**: the business backend starts instances via internal endpoints and receives terminal-state callbacks.

### Business-form mode integration

```text
1. Start: POST /api/v1/bpm/internal/instances  (X-Internal-Token)
2. Callback: register BPM_CALLBACK_<BIZTYPE>=<url>; on terminal state you
   receive { instance_id, biz_type, biz_id, result, ... } — handle idempotently
3. Reverse lookup: GET /api/v1/bpm/internal/instances/by-biz
```

The user-facing start endpoint only accepts flow-form definitions (business anchors are server-generated), so business approvals cannot be forged.

## Field permissions

Approval nodes can hide specific form fields (e.g. salary) — filtered **server-side** across task detail, instance detail and the diagram.

## Analytics

Status distribution, 30-day trend, per-definition approval rate & average duration, and a **Top-10 node bottleneck** table (average handling time).
