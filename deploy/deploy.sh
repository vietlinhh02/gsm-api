#!/usr/bin/env bash
set -euo pipefail
umask 077
cd /opt/gsm-api
exec 9>.deploy.lock
flock -n 9 || {
  echo 'Another GSM API deployment is running.' >&2
  exit 1
}

IFS= read -r API_IMAGE
IFS= read -r registry_token
if [[ ! "$API_IMAGE" =~ ^ghcr\.io/vietlinhh02/gsm-api@sha256:[a-f0-9]{64}$ ]]; then
  echo 'Expected an immutable ghcr.io/vietlinhh02/gsm-api image digest.' >&2
  exit 1
fi
if [[ -z "$registry_token" ]]; then
  echo 'A temporary GHCR token is required on the second input line.' >&2
  exit 1
fi
export API_IMAGE
registry_config=$(mktemp -d)
export DOCKER_CONFIG="$registry_config"
cleanup() {
  rm -f -- "$registry_config/config.json"
  rmdir -- "$registry_config"
}
trap cleanup EXIT
printf '%s' "$registry_token" |
  docker login ghcr.io --username vietlinhh02 --password-stdin >/dev/null
unset registry_token

compose() {
  docker compose --project-name gsm-api --env-file .env --env-file image.env -f compose.yml "$@"
}
previous_image=$(sed -n 's/^API_IMAGE=//p' image.env)
compose pull api
if ! compose up -d --wait --wait-timeout 150; then
  echo 'Deployment health check failed.' >&2
  if [[ -n "$previous_image" ]]; then
    export API_IMAGE="$previous_image"
    compose up -d --wait --wait-timeout 150
    echo 'Restored the previous application image.' >&2
  fi
  exit 1
fi
printf 'API_IMAGE=%s\n' "$API_IMAGE" >image.env.new
mv -f image.env.new image.env
printf 'Deployed %s\n' "$API_IMAGE"
