#!/bin/bash

echo "=== Starting sats.mobi with Breez Lib ==="
echo ""

# Check if Breez is enabled in config
if grep -q "enabled: true" config.yaml 2>/dev/null; then
    echo -n "Enter Breez encryption key (64 hex characters): "
    read -s ENCRYPTION_KEY
    echo ""
    
    # Validate length
    if [ ${#ENCRYPTION_KEY} -ne 64 ]; then
        echo "❌ Error: key must be 64 hex characters (32 bytes)"
        exit 1
    fi
    
    # Validate hex characters
    if ! echo "$ENCRYPTION_KEY" | grep -qE '^[0-9a-fA-F]{64}$'; then
        echo "❌ Error: key must contain only hexadecimal characters (0-9, a-f, A-F)"
        exit 1
    fi
    
    export ENCRYPTION_KEY
fi

echo "🚀 Starting container..."
docker compose up -d

# Cleanup
unset ENCRYPTION_KEY

echo ""
echo "✅ Container started successfully"
echo "📋 Use 'docker compose logs -f' to view logs"
