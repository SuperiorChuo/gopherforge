# IM Customer Support

The **im service** ships a visitor widget, a visitor page and an agent desk: site onboarding, queued conversations, messaging, knowledge-base feedback and sensitive-word controls.

## Capabilities

- **Sites & embedding**: app_key, allowed-origin whitelist, welcome text, bot switch, ready-to-paste embed snippet (`/im/visitor`, `/im/widget`).
- **Conversations**: visitor starts → queue / auto-assign → transfer to human / skill group → satisfaction rating; idle conversations auto-close (configurable).
- **Messaging**: text / image / file with `client_msg_id` idempotency, read receipts, recall; WebSocket (`/im/ws`) and REST share the same semantics.
- **Bot & knowledge base**: optional OpenAI-compatible bot, knowledge-base miss feedback to agents, hit statistics.
- **Agent tools**: quick replies, visitor cards/notes, history, search, mute, low-rating review, KB-miss handling, sensitive-word management (mask / block, hot reload).

## Authentication boundaries (important)

- **Visitors** use IM-issued guest tokens (not auth JWTs) and can only touch their own conversations; origins are checked against the site whitelist.
- **Agent** access tokens are issued by auth-service; IM verifies **validity, revocation state and current IM permissions** with auth's `/internal/verify` on WebSockets and mixed visitor/agent endpoints. Users without `im:agent:access` / `im:admin:manage` cannot enter agent flows, and forged `X-Auth-*` headers are rejected.
- The gateway routes real IM paths (API / WS / visitor / widget / uploads) to the service and keeps management pages on the frontend.

## Boundaries

- 🧪 Experimental line: APIs and schemas may still change.
- Bot answers depend on external model configuration; without it the bot degrades to a stub while human chat keeps working.
