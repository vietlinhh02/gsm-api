# Deployment

The backend and PostgreSQL are managed by Coolify in the **GSM API / production** project.
The frontend is deployed separately with Vercel CLI.

- Frontend: https://gsm-api-eight.vercel.app
- Backend: https://gsm-api.103.176.113.163.sslip.io
- Coolify project: http://103.176.113.163:8000/project/bssz9vpwhtx8qst1pib26xea
- Backend application: `jsqliuzreybxzqv9vjfcdilc` (`gsm-api-backend`).
- Managed PostgreSQL: `vvnm9dnlurb8dahdflt1niyh` (`gsm-api-postgres`).

The backend hostname uses sslip.io DNS and a certificate managed by the Coolify proxy.
A custom domain can replace it later. Vercel's server-side `API_ORIGIN` points to the HTTPS
backend; browsers call the same-origin `/api/backend` proxy, including for SSE.

## Backend pipeline

Pushes to `master` and manual runs of `backend.yml` run formatting, static analysis and
PostgreSQL integration tests, then build and publish an image to `ghcr.io/vietlinhh02/gsm-api`.
GitHub Actions passes the immutable image digest to `deploy/coolify-deploy.sh` over SSH.
The script updates this Coolify application through the local API, requests deployment,
waits for its result and checks HTTPS health. Coolify owns container creation, routing,
health checks, logs and deployment history. No image is built on the VPS.

Repository variable: `DEPLOY_HOST` is `103.176.113.163`.
Repository secrets: `DEPLOY_SSH_KEY` and `DEPLOY_KNOWN_HOSTS` hold the dedicated deploy key
and verified SSH host-key record. The public key is restricted to
`command="/opt/gsm-api/coolify-deploy.sh",restrict` in `/root/.ssh/authorized_keys`.
It cannot open an interactive shell, forward ports or select another application/image repository.

Root-owned server files, outside Git:

- `/opt/gsm-api/coolify-deploy.sh`: deployed copy of the bridge in this directory.
- `/opt/gsm-api/coolify-api.headers`: mode `0600`, a team-bound Coolify integration token.
- `/opt/gsm-api/backups/`: database and configuration backups from the migration.

The integration token has `read`, `write` and `deploy` permissions, expires after 365 days,
and is used only over loopback HTTP on the VPS. It has no `read:sensitive` or `root` ability.
Rotate it in Coolify before expiration and replace the private header file. Synchronize
bridge changes through an administrator SSH session; the restricted deploy key cannot edit it.

## Coolify runtime settings

The application uses port `8080`, `/healthz`, a `128m` memory limit and `0.5` CPU.
Its image runs as UID `65532`; custom Docker options drop all capabilities.
There are no public host port mappings. Keep one API instance because SSE notifications
are currently process-local. Preserve the managed database's hostname in `DATABASE_URL`.
The application environment schema is in `runtime.env.example`; values belong in Coolify,
never in Git or frontend JavaScript.

PostgreSQL uses the pinned PostgreSQL 18 image, a `160m` memory limit, `0.5` CPU,
`shared_buffers=16MB`, `max_connections=30`, `work_mem=1MB` and
`maintenance_work_mem=16MB`. It is private on the Coolify network.

The migration preserved the stored orders, quotes and access tokens. The original standalone
volume `gsm-api_postgres_data` and the pre-migration dump are retained as backups; the active
application uses the Coolify-managed database. The former standalone containers and deployment
script are retired. Do not delete the managed database or its volume when updating the API.
Use Coolify's backup configuration before production schema changes. Application image rollback
does not reverse database schema migrations.

## Frontend with Vercel CLI

From `frontend/`:

```sh
vercel link --yes --project gsm-api --scope peanuttt666s-7352s-projects
vercel env add API_ORIGIN production --value https://gsm-api.103.176.113.163.sslip.io --yes
vercel deploy --prod --yes --scope peanuttt666s-7352s-projects
```

`vercel.json` runs the project's pnpm build, including preparation of the MapLibre worker.
The project uses Node.js 22. The proxy route allows up to 300 seconds for streaming requests;
SSE clients reconnect if a function lifetime ends. Set client credentials in the playground's
**Xác thực** tab; they are not embedded in the deployed JavaScript.

To deploy the current backend revision again:

```sh
gh workflow run backend.yml --repo vietlinhh02/gsm-api --ref master
```
