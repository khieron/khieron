#!/bin/bash
# Health-checks a vLLM model serving endpoint via the OpenAI-compatible API.
# Usage: check-model-endpoint.sh <model-name> <namespace>
#
# Checks the /v1/models endpoint to verify the model is loaded and responding.
# Returns JSON with health status, response time, and loaded model info.

MODEL_NAME="${1:?Usage: check-model-endpoint.sh <model-name> <namespace>}"
NAMESPACE="${2:?Usage: check-model-endpoint.sh <model-name> <namespace>}"

TOKEN_PATH="/var/run/secrets/kubernetes.io/serviceaccount/token"
TIMEOUT_SECS=10

# Construct the internal service URL for the InferenceService
# KServe predictor services follow the pattern: <name>-predictor.<namespace>.svc.cluster.local
ENDPOINT="http://${MODEL_NAME}-predictor.${NAMESPACE}.svc.cluster.local/v1/models"

CURL_OPTS="-s --max-time ${TIMEOUT_SECS} -o /tmp/model-health-response.json -w %{http_code}\\n%{time_total}"

# Add auth token if available
if [ -f "$TOKEN_PATH" ]; then
    TOKEN=$(cat "$TOKEN_PATH")
    CURL_OPTS="${CURL_OPTS} -H 'Authorization: Bearer ${TOKEN}'"
fi

START_TIME=$(date +%s%N 2>/dev/null || date +%s)
HTTP_RESPONSE=$(eval curl ${CURL_OPTS} "${ENDPOINT}" 2>/dev/null)
CURL_EXIT=$?

HTTP_CODE=$(echo "$HTTP_RESPONSE" | head -1)
RESPONSE_TIME=$(echo "$HTTP_RESPONSE" | tail -1)

if [ $CURL_EXIT -ne 0 ]; then
    jq -n \
        --arg model "$MODEL_NAME" \
        --arg ns "$NAMESPACE" \
        --arg endpoint "$ENDPOINT" \
        '{
            status: "unreachable",
            model: $model,
            namespace: $ns,
            endpoint: $endpoint,
            error: "Connection failed or timed out",
            response_time_seconds: null,
            models_loaded: []
        }' 2>/dev/null || echo "{\"status\": \"unreachable\", \"error\": \"Connection failed\"}"
    rm -f /tmp/model-health-response.json
    exit 0
fi

RESPONSE_BODY=$(cat /tmp/model-health-response.json 2>/dev/null || echo "{}")
rm -f /tmp/model-health-response.json

if [ "$HTTP_CODE" = "200" ]; then
    MODELS_LOADED=$(echo "$RESPONSE_BODY" | jq '[.data[]?.id // empty]' 2>/dev/null || echo "[]")
    MODEL_COUNT=$(echo "$MODELS_LOADED" | jq 'length' 2>/dev/null || echo "0")

    if [ "$MODEL_COUNT" = "0" ]; then
        STATUS="no-models-loaded"
    else
        STATUS="healthy"
    fi

    jq -n \
        --arg status "$STATUS" \
        --arg model "$MODEL_NAME" \
        --arg ns "$NAMESPACE" \
        --arg code "$HTTP_CODE" \
        --arg rt "$RESPONSE_TIME" \
        --argjson models "${MODELS_LOADED:-[]}" \
        '{
            status: $status,
            model: $model,
            namespace: $ns,
            http_status: ($code | tonumber),
            response_time_seconds: ($rt | tonumber),
            models_loaded: $models
        }' 2>/dev/null
else
    jq -n \
        --arg model "$MODEL_NAME" \
        --arg ns "$NAMESPACE" \
        --arg code "$HTTP_CODE" \
        --arg rt "$RESPONSE_TIME" \
        --arg body "$RESPONSE_BODY" \
        '{
            status: "error",
            model: $model,
            namespace: $ns,
            http_status: ($code | tonumber),
            response_time_seconds: ($rt | tonumber),
            error: $body
        }' 2>/dev/null || echo "{\"status\": \"error\", \"http_status\": ${HTTP_CODE}}"
fi
