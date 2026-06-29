#!/bin/bash
set -e

# Patch the generated Helm deployment template to make OTEL and MLflow env vars
# conditional on their respective values being set.
DEPLOYMENT_FILE="dist/khieron/templates/deployment.yaml"

if [ ! -f "$DEPLOYMENT_FILE" ]; then
  echo "Error: $DEPLOYMENT_FILE not found"
  exit 1
fi

python3 << 'PYTHON_SCRIPT'
import re

DEPLOYMENT_FILE = "dist/khieron/templates/deployment.yaml"

with open(DEPLOYMENT_FILE, 'r') as f:
    content = f.read()

# Wrap the OTEL_EXPORTER_OTLP_HEADERS env block in a conditional
content = re.sub(
    r'( +)(- name: OTEL_EXPORTER_OTLP_HEADERS\n\s+valueFrom:\n\s+secretKeyRef:\n\s+key: OTEL_EXPORTER_OTLP_HEADERS\n\s+name: .*?-otel-headers-secret\n\s+optional: true)',
    r'\1{{- if and .Values.otelHeadersSecret .Values.otelHeadersSecret.otelExporterOtlpHeaders }}\n\1\2\n\1{{- end }}',
    content,
    flags=re.DOTALL
)

# Wrap the OTEL_EXPORTER_OTLP_ENDPOINT env block in a conditional
# The value spans multiple lines due to helmify formatting
content = re.sub(
    r'( +)(- name: OTEL_EXPORTER_OTLP_ENDPOINT\n\s+value: .*?\}\})\n',
    r'\1{{- if .Values.controllerManager.manager.env.otelExporterOtlpEndpoint }}\n\1\2\n\1{{- end }}\n',
    content,
    flags=re.DOTALL
)

with open(DEPLOYMENT_FILE, 'w') as f:
    f.write(content)

print(f"Successfully patched {DEPLOYMENT_FILE} with conditional OTEL and MLflow env vars")
PYTHON_SCRIPT
