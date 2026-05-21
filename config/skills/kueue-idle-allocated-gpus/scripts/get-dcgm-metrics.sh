#!/bin/bash

set -ex

if [ -z "${METRICS_API_SERVER}" ]; then
  echo "METRICS_API_SERVER is not set"
  exit -1
fi
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

QUERY='query=DCGM_FI_DEV_GPU_UTIL{exported_pod!=""}'
RESPONSE=$(curl -sS -k -f ${CACERT} -H "Authorization: Bearer ${TOKEN}" -H "Accept: application/json" --data-urlencode "${QUERY}" "${METRICS_API_SERVER}/api/v1/query")

if [ $? -ne 0 ]; then
  echo "Failed to query metrics: ${RESPONSE}"
  exit -1
fi

echo "${RESPONSE}" | jq '[.data.result[] | select((.value[1] | tonumber) < 20) | {
  modelName: .metric.modelName,
  gpu: .metric.gpu,
  exported_pod: .metric.exported_pod,
  exported_namespace: .metric.exported_namespace,
  exported_container: .metric.exported_container,
  device: .metric.device,
  Hostname: .metric.Hostname,
  UUID: .metric.UUID,
  utilization: (.value[1] | tonumber)
}]'

exit 0
