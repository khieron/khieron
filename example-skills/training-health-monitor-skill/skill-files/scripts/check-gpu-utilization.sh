#!/bin/bash
# Queries Prometheus for GPU utilization metrics of a training job's pods.
# Usage: check-gpu-utilization.sh <job-name> <namespace>
#
# Requires: curl, a Prometheus/Thanos endpoint accessible from the cluster.
# Returns JSON with gpu_utilization_pct and gpu_memory_used_pct per pod,
# or an empty array if metrics are unavailable.

JOB_NAME="${1:?Usage: check-gpu-utilization.sh <job-name> <namespace>}"
NAMESPACE="${2:?Usage: check-gpu-utilization.sh <job-name> <namespace>}"

PROM_HOST="${PROMETHEUS_HOST:-https://thanos-querier.openshift-monitoring.svc.cluster.local:9091}"
TOKEN_PATH="/var/run/secrets/kubernetes.io/serviceaccount/token"

if [ ! -f "$TOKEN_PATH" ]; then
    echo "[]"
    exit 0
fi

TOKEN=$(cat "$TOKEN_PATH")

# Query GPU utilization (DCGM exporter metric) for pods matching the training job
QUERY="DCGM_FI_DEV_GPU_UTIL{namespace=\"${NAMESPACE}\",pod=~\"${JOB_NAME}.*\"}"
RESPONSE=$(curl -s -k -G \
    -H "Authorization: Bearer ${TOKEN}" \
    --data-urlencode "query=${QUERY}" \
    "${PROM_HOST}/api/v1/query" 2>/dev/null)

if [ $? -ne 0 ] || [ -z "$RESPONSE" ]; then
    echo "[]"
    exit 0
fi

# Query GPU memory utilization
MEM_QUERY="DCGM_FI_DEV_MEM_COPY_UTIL{namespace=\"${NAMESPACE}\",pod=~\"${JOB_NAME}.*\"}"
MEM_RESPONSE=$(curl -s -k -G \
    -H "Authorization: Bearer ${TOKEN}" \
    --data-urlencode "query=${MEM_QUERY}" \
    "${PROM_HOST}/api/v1/query" 2>/dev/null)

if [ $? -ne 0 ] || [ -z "$MEM_RESPONSE" ]; then
    MEM_RESPONSE=""
fi

# Parse and combine results into a simple JSON array
# If jq is not available, output raw responses
if command -v jq &> /dev/null; then
    GPU_UTILS=$(echo "$RESPONSE" | jq -r '.data.result[]? | {pod: .metric.pod, gpu: .metric.gpu, gpu_utilization_pct: (.value[1] | tonumber)}')
    MEM_UTILS=$(echo "$MEM_RESPONSE" | jq -r '.data.result[]? | {pod: .metric.pod, gpu: .metric.gpu, gpu_memory_used_pct: (.value[1] | tonumber)}')

    if [ -z "$GPU_UTILS" ] && [ -z "$MEM_UTILS" ]; then
        echo "[]"
        exit 0
    fi

    # Merge GPU and memory utilization by pod
    echo "$RESPONSE" | jq --argjson mem "$( echo "$MEM_RESPONSE" | jq '[.data.result[]? | {key: (.metric.pod + "-" + .metric.gpu), value: (.value[1] | tonumber)}] | from_entries' 2>/dev/null || echo '{}')" \
        '[.data.result[]? | {
            pod: .metric.pod,
            gpu: .metric.gpu,
            gpu_utilization_pct: (.value[1] | tonumber),
            gpu_memory_used_pct: ($mem[(.metric.pod + "-" + .metric.gpu)] // null)
        }]' 2>/dev/null || echo "[]"
else
    echo "$RESPONSE"
fi
