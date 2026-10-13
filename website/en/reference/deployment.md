# Deploy the complete product

The v0.8.0 candidate includes 16 API services, an AI worker and the React application. Start the media stack and Hermes client separately. [Releases](https://github.com/SuperiorChuo/gopherforge/releases) defines published versions and image availability; a candidate branch is not a release.

## Prerequisites

- Linux, Docker Engine **24+**, Compose v2. Start capacity planning around 8 CPU / 16 GB, then load-test your actual workload.
- A TLS domain, strong random secrets and tested database/upload backup and recovery.
- Configure `APP_ENV=production`, JWT, PostgreSQL, Redis and internal tokens from `.env.example`. Do not use example passwords.
- Bind service ports to loopback; route public requests through HTTPS to Traefik.
- Configure your own AI, official-account, SMS and telephony providers. Included source code does not activate these integrations.
- Pay currently provides only a mock channel for isolated tests/demos. WeChat Pay and Alipay adapters are not implemented; merchant credentials alone cannot enable real payments. Keep `PAY_DEMO_MODE` disabled in production.

## Build from source

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge
cp microservices/.env.example microservices/.env
# Edit security and integration settings first.
make compose-up
```

This creates the shared network, starts the separate infrastructure stack, and uses the `compose-fallback` profile for the application. The gateway defaults to `http://localhost:8000`; add TLS for production. Initial credentials are `admin` / `admin123`; change the password immediately.

## Published images

The release workflow derives its matrix from Compose: 17 unique main-application images (16 APIs and frontend). The worker and migrate job reuse API images. Media and client delivery are separate. Dual-architecture releases started with **v0.3.0**, but always verify the actual manifests and public commit SHA for each release.

Use this only after v0.8.0 images are published:

```bash
cd microservices
export IMAGE_PREFIX=ghcr.io/superiorchuo/gopherforge/go-admin-kit
export IMAGE_TAG=v0.8.0
docker compose --profile compose-fallback pull
docker compose --profile compose-fallback up -d --no-build
```

## Optional components

```bash
docker compose --profile compose-fallback --profile monitoring up -d
docker compose --profile compose-fallback --profile tracing up -d
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml --profile storage up -d
```

The [complete deployment guide](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/deployment.md) includes Nginx/HTTPS and independent media/client instructions. Do not expose administrative dashboards publicly.

## Upgrade and recover

- Pin the version and full public commit SHA; back up and rehearse on an isolated copy.
- Preserve published SQL paths and bytes. Complete-product upgrades append forward-only migrations; never run production `down/reset`.
- Application rollback is not schema rollback. Restore the tested, matching database and application versions rather than simply selecting an old image.
- Health checks do not replace API, worker, UI, live-monitoring and required external-provider acceptance.
- Same-host isolation requires independent networks, volumes, container names, ports and credentials. Never reuse production data.

See [Upgrades](/en/reference/upgrade), [Observability](/en/modules/observability) and [Release validation](https://github.com/SuperiorChuo/gopherforge/blob/main/docs/release-validation.md).
