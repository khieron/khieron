#!/bin/bash
# Queries Prometheus for vLLM serving metrics of a deployed model.
# Usage: check-vllm-metrics.sh <model-name> <namespace>
#
# Requires: curl, jq, a Prometheus/Thanos endpoint accessible from the cluster.
# Returns JSON with TTFT, ITL, E2E latency percentiles, queue depth,
# KV cache usage, and request counts, or an error message if unavailable.

set -e
if [ "$DEBUG" = "1" ] || [ "$DEBUG" = "true" ]; then
  set -x
fi

MODEL_NAME="${1:?Usage: check-vllm-metrics.sh <model-name> <namespace>}"
NAMESPACE="${2:?Usage: check-vllm-metrics.sh <model-name> <namespace>}"

PROM_HOST="${PROMETHEUS_HOST:-https://thanos-querier.openshift-monitoring.svc.cluster.local:9091}"
TOKEN_PATH="/var/run/secrets/kubernetes.io/serviceaccount/token"

if [ ! -f "$TOKEN_PATH" ]; then
    echo '{"error": "No service account token available"}'
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

extract_value() {
    local response="$1"
    echo "$response" | jq -r '.data.result[0].value[1] // empty' 2>/dev/null
}

METRIC_SELECTOR="model_name=\"${MODEL_NAME}\",namespace=\"${NAMESPACE}\""

# TTFT — time to first token (histogram, get p50/p95/p99)
TTFT_P50=$(extract_value "$(query_prometheus "histogram_quantile(0.50, sum(rate(vllm:time_to_first_token_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")
TTFT_P95=$(extract_value "$(query_prometheus "histogram_quantile(0.95, sum(rate(vllm:time_to_first_token_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")
TTFT_P99=$(extract_value "$(query_prometheus "histogram_quantile(0.99, sum(rate(vllm:time_to_first_token_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")

# ITL — inter-token latency / time per output token
ITL_P50=$(extract_value "$(query_prometheus "histogram_quantile(0.50, sum(rate(vllm:time_per_output_token_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")
ITL_P95=$(extract_value "$(query_prometheus "histogram_quantile(0.95, sum(rate(vllm:time_per_output_token_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")

# E2E — end-to-end request latency
E2E_P50=$(extract_value "$(query_prometheus "histogram_quantile(0.50, sum(rate(vllm:e2e_request_latency_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")
E2E_P95=$(extract_value "$(query_prometheus "histogram_quantile(0.95, sum(rate(vllm:e2e_request_latency_seconds_bucket{${METRIC_SELECTOR}}[5m])) by (le))")")

# Queue depth — requests waiting and running
REQUESTS_WAITING=$(extract_value "$(query_prometheus "sum(vllm:num_requests_waiting{${METRIC_SELECTOR}})")")
REQUESTS_RUNNING=$(extract_value "$(query_prometheus "sum(vllm:num_requests_running{${METRIC_SELECTOR}})")")

# KV cache utilization
KV_CACHE_USAGE=$(extract_value "$(query_prometheus "avg(vllm:gpu_cache_usage_perc{${METRIC_SELECTOR}})")")

# Preemptions
PREEMPTIONS=$(extract_value "$(query_prometheus "sum(vllm:num_preemptions_total{${METRIC_SELECTOR}})")")

# Request success/failure rates (per second over last 5 minutes)
REQUEST_SUCCESS_RATE=$(extract_value "$(query_prometheus "sum(rate(vllm:request_success_total{${METRIC_SELECTOR}}[5m]))")")
REQUEST_FAILURE_RATE=$(extract_value "$(query_prometheus "sum(rate(vllm:request_failure_total{${METRIC_SELECTOR}}[5m]))")")

# Tokens generated per second
TOKENS_PER_SEC=$(extract_value "$(query_prometheus "sum(rate(vllm:generation_tokens_total{${METRIC_SELECTOR}}[5m]))")")

# Build JSON output
jq -n \
    --arg ttft_p50 "${TTFT_P50:-null}" \
    --arg ttft_p95 "${TTFT_P95:-null}" \
    --arg ttft_p99 "${TTFT_P99:-null}" \
    --arg itl_p50 "${ITL_P50:-null}" \
    --arg itl_p95 "${ITL_P95:-null}" \
    --arg e2e_p50 "${E2E_P50:-null}" \
    --arg e2e_p95 "${E2E_P95:-null}" \
    --arg requests_waiting "${REQUESTS_WAITING:-null}" \
    --arg requests_running "${REQUESTS_RUNNING:-null}" \
    --arg kv_cache_usage_pct "${KV_CACHE_USAGE:-null}" \
    --arg preemptions "${PREEMPTIONS:-null}" \
    --arg request_success_rate "${REQUEST_SUCCESS_RATE:-null}" \
    --arg request_failure_rate "${REQUEST_FAILURE_RATE:-null}" \
    --arg tokens_per_sec "${TOKENS_PER_SEC:-null}" \
    '{
        ttft_seconds: { p50: (if $ttft_p50 == "null" then null else ($ttft_p50 | tonumber) end), p95: (if $ttft_p95 == "null" then null else ($ttft_p95 | tonumber) end), p99: (if $ttft_p99 == "null" then null else ($ttft_p99 | tonumber) end) },
        itl_seconds: { p50: (if $itl_p50 == "null" then null else ($itl_p50 | tonumber) end), p95: (if $itl_p95 == "null" then null else ($itl_p95 | tonumber) end) },
        e2e_seconds: { p50: (if $e2e_p50 == "null" then null else ($e2e_p50 | tonumber) end), p95: (if $e2e_p95 == "null" then null else ($e2e_p95 | tonumber) end) },
        queue: { requests_waiting: (if $requests_waiting == "null" then null else ($requests_waiting | tonumber) end), requests_running: (if $requests_running == "null" then null else ($requests_running | tonumber) end) },
        kv_cache_usage_pct: (if $kv_cache_usage_pct == "null" then null else ($kv_cache_usage_pct | tonumber) end),
        preemptions_total: (if $preemptions == "null" then null else ($preemptions | tonumber) end),
        request_rate: { success_per_sec: (if $request_success_rate == "null" then null else ($request_success_rate | tonumber) end), failure_per_sec: (if $request_failure_rate == "null" then null else ($request_failure_rate | tonumber) end) },
        tokens_per_sec: (if $tokens_per_sec == "null" then null else ($tokens_per_sec | tonumber) end)
    }' 2>/dev/null || echo '{"error": "Failed to build metrics JSON"}'
