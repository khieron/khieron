#!/bin/bash
set -e

# Patch the generated Helm OpenAI secret template to only create the secret
# when openaiApiKeySecret.openaiApiKey is provided. Without this, helmify
# generates a required() call that fails when no key is set.

SECRET_FILE="dist/khieron/templates/openai-api-key-secret.yaml"

if [ ! -f "$SECRET_FILE" ]; then
  echo "Error: $SECRET_FILE not found"
  exit 1
fi

cat > "$SECRET_FILE" << 'TEMPLATE'
{{- if and .Values.openaiApiKeySecret .Values.openaiApiKeySecret.openaiApiKey }}
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "khieron.fullname" . }}-openai-api-key-secret
  labels:
  {{- include "khieron.labels" . | nindent 4 }}
data:
  OPENAI_API_KEY: {{ .Values.openaiApiKeySecret.openaiApiKey | b64enc | quote }}
type: Opaque
{{- end }}
TEMPLATE

echo "Successfully patched $SECRET_FILE with conditional generation"
