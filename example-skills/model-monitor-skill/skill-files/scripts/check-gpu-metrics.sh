#!/bin/bash
# Queries Prometheus for DCGM GPU metrics of a deployed model's pods.
# Usage: check-gpu-metrics.sh <model-name> <namespace>
#
# Requires: curl, jq, a Prometheus/Thanos endpoint accessible from the cluster.
# Returns JSON array with GPU utilization, memory, and temperature per pod/GPU,
# or an empty array if metrics are unavailable.

set -e
if [ "$DEBUG" = "1" ] || [ "$DEBUG" = "true" ]; then
  set -x
fi

MODEL_NAME="${1:?Usage: check-gpu-metrics.sh <model-name> <namespace>}"
NAMESPACE="${2:?Usage: check-gpu-metrics.sh <model-name> <namespace>}"

PROM_HOST="${PROMETHEUS_HOST:-https://thanos-querier.openshift-monitoring.svc.cluster.local:9091}"
TOKEN_PATH="/var/run/secrets/kubernetes.io/serviceaccount/token"

if [ ! -f "$TOKEN_PATH" ]; then
    echo "[]"
    exit 0
fi

TOKEN=$(cat "$TOKEN_PATH")

query_prometheus() {
    local query="$1"
    curl -s -k -G \
        -H "Authorization: Bearer ${TOKEN}" \
        --data-urlencode "query=${query}" \
        "${PROM_HOST}/api/v1/query" 2>/dev/null
}

POD_SELECTOR="exported_namespace=\"${NAMESPACE}\",exported_pod=~\"${MODEL_NAME}.*\""

GPU_UTIL=$(query_prometheus "DCGM_FI_DEV_GPU_UTIL{${POD_SELECTOR}}")
MEM_UTIL=$(query_prometheus "DCGM_FI_DEV_MEM_COPY_UTIL{${POD_SELECTOR}}")
GPU_TEMP=$(query_prometheus "DCGM_FI_DEV_GPU_TEMP{${POD_SELECTOR}}")
FB_USED=$(query_prometheus "DCGM_FI_DEV_FB_USED{${POD_SELECTOR}}")
FB_FREE=$(query_prometheus "DCGM_FI_DEV_FB_FREE{${POD_SELECTOR}}")

if ! command -v jq &> /dev/null; then
    echo "$GPU_UTIL"
    exit 0
fi

# Check if we got any GPU utilization data
HAS_DATA=$(echo "$GPU_UTIL" | jq -r '.data.result | length' 2>/dev/null)
if [ -z "$HAS_DATA" ] || [ "$HAS_DATA" = "0" ]; then
    echo "[]"
    exit 0
fi

# Build lookup maps for memory, temperature, and framebuffer
MEM_MAP=$(echo "$MEM_UTIL" | jq '[.data.result[]? | {key: (.metric.exported_pod + "-" + .metric.gpu), value: (.value[1] | tonumber)}] | from_entries' 2>/dev/null || echo '{}')
TEMP_MAP=$(echo "$GPU_TEMP" | jq '[.data.result[]? | {key: (.metric.exported_pod + "-" + .metric.gpu), value: (.value[1] | tonumber)}] | from_entries' 2>/dev/null || echo '{}')
FB_USED_MAP=$(echo "$FB_USED" | jq '[.data.result[]? | {key: (.metric.exported_pod + "-" + .metric.gpu), value: (.value[1] | tonumber)}] | from_entries' 2>/dev/null || echo '{}')
FB_FREE_MAP=$(echo "$FB_FREE" | jq '[.data.result[]? | {key: (.metric.exported_pod + "-" + .metric.gpu), value: (.value[1] | tonumber)}] | from_entries' 2>/dev/null || echo '{}')

echo "$GPU_UTIL" | jq \
    --argjson mem "$MEM_MAP" \
    --argjson temp "$TEMP_MAP" \
    --argjson fb_used "$FB_USED_MAP" \
    --argjson fb_free "$FB_FREE_MAP" \
    '[.data.result[]? | {
        pod: .metric.exported_pod,
        gpu: .metric.gpu,
        gpu_utilization_pct: (.value[1] | tonumber),
        gpu_memory_bandwidth_pct: ($mem[(.metric.exported_pod + "-" + .metric.gpu)] // null),
        gpu_temperature_c: ($temp[(.metric.exported_pod + "-" + .metric.gpu)] // null),
        gpu_fb_used_mb: ($fb_used[(.metric.exported_pod + "-" + .metric.gpu)] // null),
        gpu_fb_free_mb: ($fb_free[(.metric.exported_pod + "-" + .metric.gpu)] // null)
    }]' 2>/dev/null || echo "[]"
