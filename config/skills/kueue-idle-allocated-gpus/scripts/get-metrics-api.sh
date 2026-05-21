#!/bin/bash
if [ -z "${METRICS_API_SERVER}" ]; then
  echo "METRICS_API_SERVER is not set"
  exit -1
fi
echo "${METRICS_API_SERVER}"
exit 0
