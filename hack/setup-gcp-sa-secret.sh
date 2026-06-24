#!/bin/bash
set -e

# Conditionally add service account key secretGenerator to kustomization.yaml
# Only if the file exists

KUSTOMIZATION_FILE="config/default/kustomization.yaml"
SA_KEY_FILE="config/default/vertex-ai-svc-acct-gcp.json"
BACKUP_FILE="${KUSTOMIZATION_FILE}.backup"

# Function to restore kustomization.yaml
restore_kustomization() {
    if [ -f "$BACKUP_FILE" ]; then
        mv "$BACKUP_FILE" "$KUSTOMIZATION_FILE"
    fi
}

# Set up cleanup on exit
trap restore_kustomization EXIT

# Backup the original kustomization.yaml
cp "$KUSTOMIZATION_FILE" "$BACKUP_FILE"

# Check if the service account file exists
if [ -f "$SA_KEY_FILE" ]; then
    echo "Found $SA_KEY_FILE - adding to secretGenerator"

    # Add the service account key to secretGenerator using sed
    # Simpler approach: append to the secretGenerator list
    cat >> "$KUSTOMIZATION_FILE" << 'EOF'
- name: google-service-account-key
  files:
  - key.json=vertex-ai-svc-acct-gcp.json
EOF
    echo "Added google-service-account-key to secretGenerator"
else
    echo "Service account file $SA_KEY_FILE not found - skipping secretGenerator"
fi

# Return the path to the backup so caller can restore later
echo "$BACKUP_FILE"
