#!/bin/bash
set -e

# Patch the generated Helm chart to extract --model-backend and --openai-base-url
# from the args array into dedicated top-level values, making them easy to override
# at install time. Follows the same pattern as patch-helm-model-name.sh.

VALUES_FILE="dist/khieron/values.yaml"
DEPLOYMENT_FILE="dist/khieron/templates/deployment.yaml"

if [ ! -f "$VALUES_FILE" ]; then
  echo "Error: $VALUES_FILE not found"
  exit 1
fi

if [ ! -f "$DEPLOYMENT_FILE" ]; then
  echo "Error: $DEPLOYMENT_FILE not found"
  exit 1
fi

python3 << 'PYTHON_SCRIPT'
import re

VALUES_FILE = "dist/khieron/values.yaml"
DEPLOYMENT_FILE = "dist/khieron/templates/deployment.yaml"

# --- Patch values.yaml ---
with open(VALUES_FILE, 'r') as f:
    content = f.read()

# Extract --model-backend value
backend_match = re.search(r'--model-backend=(\S*)', content)
if not backend_match:
    print("Warning: --model-backend not found in values.yaml args, skipping")
    exit(0)

model_backend = backend_match.group(1) or "gemini"

# Extract --openai-base-url value (may be empty)
url_match = re.search(r'--openai-base-url=(\S*)', content)
openai_base_url = url_match.group(1) if url_match else ""

# Remove both lines from the args list
content = re.sub(r'\n\s*- --model-backend=\S*', '', content)
content = re.sub(r'\n\s*- --openai-base-url=\S*', '', content)

# Add as top-level values after modelName
content = re.sub(
    r'^(modelName: .+)$',
    f'\\1\nmodelBackend: {model_backend}\nopenaiBaseUrl: "{openai_base_url}"',
    content,
    flags=re.MULTILINE
)

with open(VALUES_FILE, 'w') as f:
    f.write(content)

print(f"Patched {VALUES_FILE}: extracted modelBackend={model_backend}, openaiBaseUrl={openai_base_url}")

# --- Patch deployment.yaml ---
with open(DEPLOYMENT_FILE, 'r') as f:
    content = f.read()

# Add templated args after the --model-name line (which was added by patch-helm-model-name.sh)
content = content.replace(
    '- --model-name={{ .Values.modelName }}',
    '- --model-name={{ .Values.modelName }}\n'
    '        - --model-backend={{ .Values.modelBackend }}\n'
    '        {{- if .Values.openaiBaseUrl }}\n'
    '        - --openai-base-url={{ .Values.openaiBaseUrl }}\n'
    '        {{- end }}'
)

with open(DEPLOYMENT_FILE, 'w') as f:
    f.write(content)

print(f"Patched {DEPLOYMENT_FILE}: added templated --model-backend and --openai-base-url args")
PYTHON_SCRIPT
