#!/bin/bash

set -ex

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

# Get namespaces labelled with kueue.openshift.io/managed=true
NAMESPACES=$(curl -sS -k -f ${CACERT} \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Accept: application/json" \
  -G --data-urlencode 'labelSelector=kueue.openshift.io/managed=true' \
  "${API}/api/v1/namespaces")

if [ $? -ne 0 ]; then
  echo "Failed to query namespaces: ${NAMESPACES}"
  exit -1
fi

NS_LIST=$(echo "${NAMESPACES}" | jq -r '.items[].metadata.name')

if [ -z "${NS_LIST}" ]; then
  echo "[]"
  exit 0
fi

# Collect non-running pods from each managed namespace
RESULTS="[]"
for NS in ${NS_LIST}; do
  PODS=$(curl -sS -k -f ${CACERT} \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Accept: application/json" \
    "${API}/api/v1/namespaces/${NS}/pods?fieldSelector=status.phase%21%3DRunning%2Cstatus.phase%21%3DSucceeded")

  if [ $? -ne 0 ]; then
    echo "Failed to query pods in namespace ${NS}: ${PODS}"
    continue
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
done

echo "${RESULTS}"

exit 0
