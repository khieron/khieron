#!/bin/bash
set -e

# Patch the generated Helm otel-headers-secret template to make it conditional
# on otelHeadersSecret.otelExporterOtlpHeaders being set.
SECRET_FILE="dist/khieron/templates/otel-headers-secret.yaml"

if [ ! -f "$SECRET_FILE" ]; then
  echo "Error: $SECRET_FILE not found"
  exit 1
fi

python3 << 'PYTHON_SCRIPT'
import re

SECRET_FILE = "dist/khieron/templates/otel-headers-secret.yaml"

with open(SECRET_FILE, 'r') as f:
    content = f.read()

# Wrap entire file in conditional on otelExporterOtlpHeaders being set
content = re.sub(r'^(apiVersion:)', r'{{- if and .Values.otelHeadersSecret .Values.otelHeadersSecret.otelExporterOtlpHeaders }}\n\1', content, flags=re.MULTILINE)

# Replace the required directive with a simple b64enc since the if-guard handles it
content = re.sub(
    r'\{\{ required "otelHeadersSecret\.otelExporterOtlpHeaders is required" \.Values\.otelHeadersSecret\.otelExporterOtlpHeaders \| b64enc \| quote\s*\}\}',
    '{{ .Values.otelHeadersSecret.otelExporterOtlpHeaders | b64enc | quote }}',
    content
)

# Add closing end tag
content = content.rstrip('\n') + '\n{{- end }}\n'

with open(SECRET_FILE, 'w') as f:
    f.write(content)

print(f"Successfully patched {SECRET_FILE} with conditional generation")
PYTHON_SCRIPT
