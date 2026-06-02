#!/bin/bash

set -e
if [ "$DEBUG" = "1" ] || [ "$DEBUG" = "true" ]; then
  set -x
fi

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

if [ -z "$2" ]; then
  echo "Usage: $0 <namespace> <pod>"
  exit 1
fi

NS="$1"
POD="$2"

pluralize() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | sed 's/$/s/'
}

api_path() {
  local api_version="$1"
  local kind_plural="$2"
  local ns="$3"
  local name="$4"
  if [ "$api_version" = "v1" ]; then
    echo "/api/v1/namespaces/${ns}/${kind_plural}/${name}"
  else
    echo "/apis/${api_version}/namespaces/${ns}/${kind_plural}/${name}"
  fi
}

fetch() {
  local path="$1"
  curl -sS -f ${CACERT} \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Accept: application/json" \
    "${API}${path}"
}

CURRENT_API_VERSION="v1"
CURRENT_KIND="Pod"
CURRENT_NAME="$POD"
CHAIN="Pod ${POD}"

while true; do
  PLURAL=$(pluralize "$CURRENT_KIND")
  PATH_URL=$(api_path "$CURRENT_API_VERSION" "$PLURAL" "$NS" "$CURRENT_NAME")

  OBJ=$(fetch "$PATH_URL") || {
    echo "Failed to fetch ${CURRENT_KIND}/${CURRENT_NAME}"
    exit 1
  }

  OWNER_REF=$(echo "$OBJ" | jq -r '.metadata.ownerReferences[0] // empty')

  if [ -z "$OWNER_REF" ]; then
    echo "Root owner found: ${CURRENT_KIND}/${CURRENT_NAME}"
    echo "Chain: ${CHAIN}"
    echo "Deleting ${CURRENT_KIND}/${CURRENT_NAME}..."

    curl -sS -f ${CACERT} \
      -X DELETE \
      -H "Authorization: Bearer ${TOKEN}" \
      -H "Accept: application/json" \
      "${API}${PATH_URL}"

    echo "Deleted ${CURRENT_KIND}/${CURRENT_NAME}"
    exit 0
  fi

  CURRENT_API_VERSION=$(echo "$OWNER_REF" | jq -r '.apiVersion')
  CURRENT_KIND=$(echo "$OWNER_REF" | jq -r '.kind')
  CURRENT_NAME=$(echo "$OWNER_REF" | jq -r '.name')
  CHAIN="${CHAIN} -> ${CURRENT_KIND} ${CURRENT_NAME}"
  echo "Following owner: ${CURRENT_KIND}/${CURRENT_NAME}"
done
