# Getting Started (15 minutes)

GopherForge is an open-source, enterprise-grade Go microservices admin scaffold. This guide takes you from zero to a running full stack: gateway + 7 Go services + React frontend + PostgreSQL/Redis/NATS.

## Requirements

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (the only hard requirement)
- Optional local development: Go **1.26.3+**, Node.js **20.19+ / 22.12+**

## One-command startup

```bash
git clone https://github.com/SuperiorChuo/gopherforge.git
cd gopherforge
cp microservices/.env.example microservices/.env
make compose-up
```

First build takes ~3 minutes. Then:

| Entry | URL |
|------|------|
| Unified gateway (recommended) | http://localhost:8000 |
| Frontend direct | http://localhost:3000 |
| Health check | http://localhost:8000/api/v1/health/ready |

## Default account

| Username | Password |
|--------|------|
| `admin` | `admin123` |

::: warning Change before going live
Rotate the default password, `JWT_SECRET` and all datastore credentials in `.env`. See [Production Deployment](/en/reference/deployment).
:::

## Verify the stack

```bash
cd microservices
(cd services/monitor && go test ./... && go vet ./...)
(cd web && npm run lint && npm run build)
npm run test:smoke:unit && npm run test:contract
npm run api:contract
docker compose config --quiet
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml config --quiet
API_BASE_URL=http://127.0.0.1:8000/api/v1 npm run smoke:api
```

## Local dev mode (optional)

Run only the infra containers and hot-reload services/frontend on your machine:

Run the long-lived commands in three separate terminals:

```bash
# Terminal 1: dependency containers
cd microservices
docker compose -p go-admin-kit-infra -f docker-compose.infra.yml up -d go-admin-kit-postgres go-admin-kit-redis go-admin-kit-nats

# Terminal 2: the Go service under development (auth as an example)
cd microservices/services/auth
go run ./cmd

# Terminal 3: frontend HMR
cd microservices/web
npm ci
npm run dev
```

## Port conflicts

Defaults: frontend `3000`, gateway `8000`, PostgreSQL `5432`, Redis `6379`. Change the matching `*_PORT` in `microservices/.env`.

## Next

- How the services are split → [Architecture](/en/guide/architecture)
- Add your first business module → [Extending](/en/guide/extend)
- Or explore the [Live Demo](https://superiorchuo.github.io/gopherforge/) (frontend-only fake data, any account logs in)
