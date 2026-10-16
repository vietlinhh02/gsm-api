# Backend deployment

GitHub Actions tests the Go API against a disposable PostgreSQL database, builds the backend
image, publishes it to `ghcr.io/vietlinhh02/gsm-api`, then deploys the immutable image digest.
Pushes to `master` and manual runs of `backend.yml` use the same pipeline.

The frontend source is included in this repository but is not deployed by this workflow.
The API runs as a non-root user in a read-only container. PostgreSQL uses its own persistent
volume and is not exposed on a host port. Both containers have memory, CPU and log limits
for the current small VPS. SSE uses one API instance; do not scale API replicas without a
shared notification channel.

## Server files and credentials

The server keeps these root-owned files in `/opt/gsm-api`:

- `compose.yml` and `deploy.sh`: reviewed copies from this directory.
- `.env`: runtime credentials and database URL; mode `0600`, never committed to Git.
- `image.env`: the most recently healthy `API_IMAGE` digest.

Use `deploy/runtime.env.example` as a schema for `.env`. Generate unique passwords rather
than using the local development defaults. Preserve `POSTGRES_PASSWORD` after initializing
the volume; changing just the environment does not rotate an existing PostgreSQL password.
`DATABASE_URL` must use the same password. Back up the database before schema changes.

The current endpoint is `http://103.176.113.163:8082`; `/healthz` is unauthenticated.
A domain and HTTPS proxy can be added later. Use HTTPS before sending real credentials or
passenger data over a public network; SSH tunnelling is available for private testing now:

```sh
ssh -L 8082:127.0.0.1:8082 root@103.176.113.163
```

The existing Coolify installation and its applications are independent of this Compose project.
No Coolify services or volumes are modified by deployment.

## GitHub configuration

Repository variable: `DEPLOY_HOST` is the server IP.
Repository secrets: `DEPLOY_SSH_KEY` is the dedicated Ed25519 private key and
`DEPLOY_KNOWN_HOSTS` is the verified SSH host-key record.

The matching public key in `/root/.ssh/authorized_keys` is restricted to
`command="/opt/gsm-api/deploy.sh",restrict`. It cannot open an interactive shell or forward
ports. The deploy script only accepts an immutable image digest from this repository.
The workflow passes its temporary `GITHUB_TOKEN` over encrypted SSH standard input for
GHCR pull access. Registry credentials are deleted after the run, so a long-lived registry
personal access token is not needed. Runtime application credentials stay on the server.

Changes to `compose.yml` or `deploy.sh` require an administrator to synchronize those files
explicitly before running the workflow; the restricted deploy key cannot replace them.

## Operations

Run from `/opt/gsm-api` as an administrator:

```sh
docker compose --project-name gsm-api --env-file .env --env-file image.env -f compose.yml ps
docker compose --project-name gsm-api --env-file .env --env-file image.env -f compose.yml logs --tail=100 api
```

To deploy the current GitHub revision again:

```sh
gh workflow run backend.yml --repo vietlinhh02/gsm-api --ref master
```

The script serializes deployments, checks container health, and records the new digest only
on success. If an application health check fails and a previous image exists, it restores
that image. Image rollback does not reverse database schema changes. Never delete the
`gsm-api_postgres_data` volume when updating the app.

For a database backup:

```sh
umask 077
docker compose --project-name gsm-api --env-file .env --env-file image.env -f compose.yml \
  exec -T postgres pg_dump -U gsm_api gsm_api > gsm-api-backup.sql
```
