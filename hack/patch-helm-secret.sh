#!/bin/bash
set -e

# Patch the generated Helm secret template to add conditional validation
SECRET_FILE="dist/khieron/templates/google-api-key-secret.yaml"

if [ ! -f "$SECRET_FILE" ]; then
  echo "Error: $SECRET_FILE not found"
  exit 1
fi

# Use Python for easier multi-line handling
python3 << 'PYTHON_SCRIPT'
import re

SECRET_FILE = "dist/khieron/templates/google-api-key-secret.yaml"

with open(SECRET_FILE, 'r') as f:
    content = f.read()

# Add validation header before apiVersion
validation = '''{{- if not (or .Values.googleApiKeySecret.googleApiKey .Values.googleApiKeySecret.googleCloudProject) }}
{{- fail "Either googleApiKeySecret.googleApiKey or googleApiKeySecret.googleCloudProject must be provided" }}
{{- end }}
{{- if and .Values.googleApiKeySecret.googleApiKey .Values.googleApiKeySecret.googleCloudProject }}
{{- fail "Only one of googleApiKeySecret.googleApiKey or googleApiKeySecret.googleCloudProject can be provided, not both" }}
{{- end }}
'''

content = re.sub(r'^(apiVersion:)', validation + r'\1', content, flags=re.MULTILINE)

# Find and replace GOOGLE_API_KEY field (with multi-line value)
api_key_pattern = r'(  GOOGLE_API_KEY:.*?)(\n  [A-Z_]+:|$)'
api_key_replacement = r'  {{- if .Values.googleApiKeySecret.googleApiKey }}\n\1\n  {{- end }}\2'
content = re.sub(api_key_pattern, api_key_replacement, content, flags=re.DOTALL)

# Find and replace GOOGLE_CLOUD_LOCATION field (with multi-line value)
# Replace with conditional that includes default value
location_pattern = r'  GOOGLE_CLOUD_LOCATION:.*?(\n  [A-Z_]+:|$)'
location_replacement = r'  {{- if .Values.googleApiKeySecret.googleCloudProject }}\n  GOOGLE_CLOUD_LOCATION: {{ .Values.googleApiKeySecret.googleCloudLocation | default "global" | b64enc | quote }}\n  {{- end }}\1'
content = re.sub(location_pattern, location_replacement, content, flags=re.DOTALL)

# Find and replace GOOGLE_CLOUD_PROJECT field (with multi-line value)
project_pattern = r'(  GOOGLE_CLOUD_PROJECT:.*?)(\ntype:)'
project_replacement = r'  {{- if .Values.googleApiKeySecret.googleCloudProject }}\n\1\n  {{- end }}\2'
content = re.sub(project_pattern, project_replacement, content, flags=re.DOTALL)

with open(SECRET_FILE, 'w') as f:
    f.write(content)

print(f"Successfully patched {SECRET_FILE} with conditional validation")
PYTHON_SCRIPT
