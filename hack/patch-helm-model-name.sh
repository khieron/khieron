#!/bin/bash
set -e

# Patch the generated Helm chart to extract --model-name from the args array
# into a dedicated top-level value, making it easy to override at install time.

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

# Extract the model name value from the args list
match = re.search(r'--model-name=(\S+)', content)
if not match:
    print("Warning: --model-name not found in values.yaml args, skipping")
    exit(0)

model_name = match.group(1)

# Remove the --model-name line from the args list
content = re.sub(r'\n\s*- --model-name=\S+', '', content)

# Add modelName as a top-level value (before controllerManager)
content = re.sub(
    r'^(controllerManager:)',
    f'modelName: {model_name}\n\\1',
    content,
    flags=re.MULTILINE
)

with open(VALUES_FILE, 'w') as f:
    f.write(content)

print(f"Patched {VALUES_FILE}: extracted modelName={model_name}")

# --- Patch deployment.yaml ---
with open(DEPLOYMENT_FILE, 'r') as f:
    content = f.read()

# After the toYaml args line, add the templated --model-name arg
content = content.replace(
    '{{- toYaml .Values.controllerManager.manager.args | nindent 8 }}',
    '{{- toYaml .Values.controllerManager.manager.args | nindent 8 }}\n        - --model-name={{ .Values.modelName }}'
)

with open(DEPLOYMENT_FILE, 'w') as f:
    f.write(content)

print(f"Patched {DEPLOYMENT_FILE}: added templated --model-name arg")
PYTHON_SCRIPT
