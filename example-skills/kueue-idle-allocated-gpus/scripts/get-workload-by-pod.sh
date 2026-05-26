#!/bin/bash
# Usage: get-workload-by-pod.sh <namespace> <pod_name>
NAMESPACE=$1
POD_NAME=$2

if [ -z "$NAMESPACE" ] || [ -z "$POD_NAME" ]; then
  echo "Usage: get-workload-by-pod.sh <namespace> <pod_name>"
  exit -1
fi

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

# Step 1: Get the Job name from the pod's owner reference
JOB_NAME=$(curl -ks --cacert $CA -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" \
  "$API/api/v1/namespaces/$NAMESPACE/pods/$POD_NAME" | \
  jq -r '.metadata.ownerReferences[] | select(.kind=="Job") | .name')

if [ -z "$JOB_NAME" ]; then
  echo "No Job owner found for pod $POD_NAME"
  exit -1
fi

# Step 2: Get the Job details (name, creationTimestamp, spec, status only)
JOB=$(curl -ks --cacert $CA -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" \
  "$API/apis/batch/v1/namespaces/$NAMESPACE/jobs/$JOB_NAME" | \
  jq '{name: .metadata.name, createdAt: .metadata.creationTimestamp, spec: .spec, status: .status}')

# Step 3: Get the Kueue Workload for this Job (name, creationTimestamp, spec, status only)
WORKLOAD=$(curl -ks --cacert $CA -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/json" \
  "$API/apis/kueue.x-k8s.io/v1beta2/namespaces/$NAMESPACE/workloads" | \
  jq --arg job "$JOB_NAME" '[.items[] | select(.metadata.ownerReferences[]?.name == $job) | {name: .metadata.name, createdAt: .metadata.creationTimestamp, spec: .spec, status: .status}]')

echo "{\"pod\": \"$POD_NAME\", \"namespace\": \"$NAMESPACE\", \"job_name\": \"$JOB_NAME\", \"job\": $JOB, \"workloads\": $WORKLOAD}" | jq .
exit 0
