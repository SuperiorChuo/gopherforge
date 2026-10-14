# Sales CRM

The **crm service** spans lead-to-cash: leads / customers / contacts / opportunities / contracts / payments, interlocked with IM visitor cards and call-center contacts.

## Capabilities

- **Leads**: intake, duplicate detection and merge, assignment and claiming, conversion (`converted`); assignment logs are kept.
- **Customers & opportunities**: customer profiles, contacts, opportunity stages; contracts and payment records wired to BPM contract approvals.
- **Pools & rules**: lead pools / open-sea claiming, rotation rules, timeout reclaim policies.
- **Data scopes**: `my-scope` self / department / all, org filtering, strict tenant isolation.
- **Complete lists**: detail, projection and aggregate listings read the full result set in bounded batches — no hard truncation caps.

## Boundaries

- 🧪 Experimental line: APIs and schemas may still change.
- Integrations with IM / call center depend on those lines being deployed and configured.
