#!/bin/bash
set -e

echo "Starting local machine (tunnel client)..."

# Wait for SSH server to be ready
echo "Waiting for SSH server to be ready..."
until nc -z remote 22; do
    echo "SSH server not ready yet, waiting..."
    sleep 2
done
echo "SSH server is ready!"

# Set up SSH key permissions
if [ -f /ssh-keys/id_ed25519 ]; then
    mkdir -p /root/.ssh
    cp /ssh-keys/id_ed25519 /root/.ssh/
    chmod 600 /root/.ssh/id_ed25519
    chmod 700 /root/.ssh
    echo "SSH key configured"
fi

# Add remote to known hosts
ssh-keyscan -H remote >> /root/.ssh/known_hosts 2>/dev/null

# Start the tunnel
echo "Starting SSH tunnel..."
exec /app/ssh-socat-tunnel -config /config/config.yaml
