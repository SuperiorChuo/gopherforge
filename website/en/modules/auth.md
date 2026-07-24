# Auth & Security

The **auth service** owns the whole chain from login to gateway verification.

## Capabilities

- **Password login** with captcha and Redis-backed rate limiting (lockout threshold/window hot-configurable in console settings).
- **Dual JWT tokens** (access + refresh) with rotation and revocation (Redis blacklist); the frontend refreshes transparently.
- **Concurrent-refresh protection**: one page reuses its in-flight refresh request; multiple tabs coordinate through Web Locks or a local lease, while the server atomically consumes the old refresh token in Redis to prevent duplicate rotation and false logouts.
- **TOTP two-factor** compatible with standard authenticator apps.
- **OAuth** third-party login providers.
- **Online users** registered in Redis; admins can force logout.
- **Login events** delivered via NATS JetStream to the audit service (durable).

## ForwardAuth: authenticate once

Protected routes attach Traefik's ForwardAuth middleware. The auth service verifies and injects identity headers — `X-Auth-User-ID`, `X-Auth-Username`, `X-Auth-Tenant-ID`, `X-Auth-Platform-Admin` — and downstream services **trust only these headers**, so new services get auth for free.

## Password policy

bcrypt storage, password-history reuse limits and max-age forced rotation, all hot-configurable under the `security.policy` settings key.

## OAuth2 authorization server + OIDC

Beyond being an OAuth *client* ("login with GitHub"), the scaffold is a full **OAuth2 authorization server** — third-party apps can "login with your platform account":

- **Grant types**: `authorization_code` (+ PKCE, enforced for public clients) and `client_credentials` (service-to-service).
- **Protocol endpoints**: `/oauth2/authorize` (consent page), `/oauth2/token`, `/oauth2/introspect`, `/oauth2/revoke`, `/oauth2/userinfo`.
- **OIDC**: the `openid` scope issues an **RS256 `id_token`**, with `/oauth2/.well-known/openid-configuration` discovery and `/oauth2/jwks` public-key endpoints — third parties integrate SSO with any off-the-shelf OIDC client library, verifying via JWKS with no shared secrets. Signing uses a dedicated RSA-2048 key (auto-generated, persisted in `system_settings` for multi-replica sharing, stable `kid`), fully isolated from the console's own HS256.
- **Console management**: "System → OAuth2 Apps" manages clients (redirect-URI scheme allowlist) and lets admins inspect and revoke issued tokens.
- **Hardening** (from adversarial review): the OIDC private key cannot be read through the generic settings API; `introspect` only inspects tokens issued to the caller; refresh rotation guards against concurrent double-spend, and reuse of a revoked refresh token revokes the whole token family (OAuth Security BCP).

## Refresh failure boundary

The browser first adopts a newer token pair published by another tab. It redirects to login only when no usable pair remains, the refresh lease times out, or the server confirms that the refresh token is invalid. Server-side rotation is still a one-time consume operation; callers must not retry the same old refresh token.

## Security review

The auth surface has been through several rounds of adversarial security review (supply chain, timing oracles, token-family attacks, redirect hijacking); fixes ship with regression tests. Please report issues privately per [SECURITY.md](https://github.com/SuperiorChuo/gopherforge/blob/main/SECURITY.md).
