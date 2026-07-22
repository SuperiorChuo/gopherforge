# Auth & Security

The **auth service** owns the whole chain from login to gateway verification.

## Capabilities

- **Password login** with captcha and Redis-backed rate limiting (lockout threshold/window hot-configurable in console settings).
- **Dual JWT tokens** (access + refresh) with rotation and revocation (Redis blacklist); the frontend refreshes transparently.
- **TOTP two-factor** compatible with standard authenticator apps.
- **OAuth** third-party login providers.
- **Online users** registered in Redis; admins can force logout.
- **Login events** delivered via NATS JetStream to the audit service (durable).

## ForwardAuth: authenticate once

Protected routes attach Traefik's ForwardAuth middleware. The auth service verifies and injects identity headers — `X-Auth-User-ID`, `X-Auth-Username`, `X-Auth-Tenant-ID`, `X-Auth-Platform-Admin` — and downstream services **trust only these headers**, so new services get auth for free.

## Password policy

bcrypt storage, password-history reuse limits and max-age forced rotation, all hot-configurable under the `security.policy` settings key.
