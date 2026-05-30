#!/bin/bash

set -ex

if [ -z "$1" ]; then
  echo "Usage: $0 <namespace>"
  exit 1
fi

NS="$1"

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

# Collect non-running pods from the specified namespace
RESULTS="[]"
if ! PODS=$(curl -sS -k -f "${CACERT}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Accept: application/json" \
  "${API}/api/v1/namespaces/${NS}/pods?fieldSelector=status.phase%21%3DRunning%2Cstatus.phase%21%3DSucceeded"); then
  echo "Failed to query pods in namespace ${NS}: ${PODS}"
  exit 1
fi

NS_RESULTS=$(echo "${PODS}" | jq '[.items[] | {
  name: .metadata.name,
  namespace: .metadata.namespace,
  job: ([.metadata.ownerReferences[]? | select(.kind == "Job") | .name][0] // ""),
  workload: (.metadata.annotations["kueue.x-k8s.io/workload"] // ""),
  phase: .status.phase,
  reason: (.status.reason // ""),
  message: (.status.message // ""),
  conditions: [.status.conditions[]? | {type: .type, status: .status, reason: (.reason // ""), message: (.message // "")}],
  createdAt: .metadata.creationTimestamp
}]')

RESULTS=$(echo "${RESULTS}" "${NS_RESULTS}" | jq -s '.[0] + .[1]')

echo "${RESULTS}"

exit 0
