#!/usr/bin/env bash
set -euo pipefail
umask 077
cd /opt/gsm-api
exec 9>.coolify-deploy.lock
flock -n 9 || {
  echo 'Another Coolify deployment is running.' >&2
  exit 1
}

IFS= read -r image
if [[ ! "$image" =~ ^ghcr\.io/vietlinhh02/gsm-api@sha256:[a-f0-9]{64}$ ]]; then
  echo 'Expected an immutable ghcr.io/vietlinhh02/gsm-api image digest.' >&2
  exit 1
fi
application_uuid=jsqliuzreybxzqv9vjfcdilc
image_digest=${image##*@sha256:}
headers=/opt/gsm-api/coolify-api.headers
api_origin=http://127.0.0.1:8000/api/v1

api() {
  curl --fail --show-error --silent --max-time 30 --header "@$headers" "$@"
}
api --request PATCH --data "{\"docker_registry_image_tag\":\"sha256-$image_digest\"}" \
  "$api_origin/applications/$application_uuid" >/dev/null
deployment_uuid=$(api --request POST --data "{\"uuid\":\"$application_uuid\",\"force\":false}" \
  "$api_origin/deploy" |
  python3 -c 'import json,sys; print(json.load(sys.stdin)["deployments"][0]["deployment_uuid"])')
printf 'Coolify deployment: %s\n' "$deployment_uuid"

for ((attempt = 0; attempt < 60; attempt++)); do
  status=$(api "$api_origin/deployments/$deployment_uuid" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
  case "$status" in
  finished)
    curl --fail --show-error --silent --max-time 15 \
      https://gsm-api.aaisee.me/healthz >/dev/null
    printf 'Coolify deployed %s\n' "$image"
    exit 0
    ;;
  queued | in_progress)
    sleep 5
    ;;
  *)
    printf 'Coolify deployment %s ended with status %s. Check its deployment log.\n' \
      "$deployment_uuid" "$status" >&2
    exit 1
    ;;
  esac
done
printf 'Timed out waiting for Coolify deployment %s. Check its status before retrying.\n' \
  "$deployment_uuid" >&2
exit 1
