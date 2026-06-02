#!/bin/bash

set -ex

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

if [ -z "$1" ]; then
  echo "Usage: $0 <namespace>"
  exit 1
fi

NS="$1"

if ! PODS=$(curl -sS -k -f ${CACERT} \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Accept: application/json" \
  "${API}/api/v1/namespaces/${NS}/pods?fieldSelector=status.phase%21%3DRunning%2Cstatus.phase%21%3DSucceeded"); then

  echo "Failed to query pods in namespace ${NS}: ${PODS}"
  exit 1
fi

NS_RESULTS=$(echo "${PODS}" | jq '[.items[] | {
  name: .metadata.name,
  namespace: .metadata.namespace,
  phase: .status.phase,
  reason: (.status.reason // ""),
  message: (.status.message // ""),
  conditions: [.status.conditions[]? | {type: .type, status: .status, reason: (.reason // ""), message: (.message // "")}],
  createdAt: .metadata.creationTimestamp
}]')

echo "$NS_RESULTS"

exit 0