#!/bin/bash
set -e

# Patch the generated Helm deployment template to add optional affinity support
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

# Find the nodeSelector line and add affinity block after it
# The pattern matches the nodeSelector with its template expression
pattern = r'(      nodeSelector: \{\{- toYaml \.Values\.controllerManager\.nodeSelector \| nindent 8 \}\})\n'
replacement = r'\1\n      {{- with .Values.controllerManager.affinity }}\n      affinity: {{- toYaml . | nindent 8 }}\n      {{- end }}\n'

content = re.sub(pattern, replacement, content)

with open(DEPLOYMENT_FILE, 'w') as f:
    f.write(content)

print(f"Successfully patched {DEPLOYMENT_FILE} with optional affinity support")
PYTHON_SCRIPT
