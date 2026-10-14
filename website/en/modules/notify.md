# Notifications

The **notify service** unifies in-console messages and multi-channel delivery: messages, templates, channels and delivery logs. Platform alerts and business reminders converge here.

## Capabilities

- **Inbox**: message lists / unread counts / read state, isolated per user and tenant.
- **Templates**: template codes (e.g. `login_alert`) with variables; console-managed.
- **Channels**: pluggable email / SMS / enterprise-IM webhooks; channel failures degrade to in-console messages.
- **Delivery logs**: non-inbox channels (email / SMS) are logged per attempt (status, target, error detail).
- **Internal sending contract**: service-to-service sends are gated by `NOTIFY_INTERNAL_TOKEN`; an empty value disables the client (fail closed).

## Typical flows

- **Abnormal-login alert**: audit detects login from a new IP / device → authenticated notify client sends the `login_alert` inbox message (on by default, toggle in the security policy).
- **Monitoring alerts**: Prometheus rules → Alertmanager grouping & dedup → notify webhook inbox message.

## Boundaries

- External channel credentials (SMS / email vendors) are user-configured; missing channels follow the degradation policy.
