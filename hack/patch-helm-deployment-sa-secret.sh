#!/bin/bash
set -e

# Patch the generated Helm deployment template to use the correct secret name
DEPLOYMENT_FILE="dist/khieron/templates/deployment.yaml"

if [ ! -f "$DEPLOYMENT_FILE" ]; then
  echo "Error: $DEPLOYMENT_FILE not found"
  exit 1
fi

# Use sed to replace the hardcoded secret name with the Helm template variable
sed -i 's/secretName: google-service-account-key$/secretName: {{ include "khieron.fullname" . }}-google-service-account-key/' "$DEPLOYMENT_FILE"

echo "Patched deployment.yaml to use templated service account secret name"
