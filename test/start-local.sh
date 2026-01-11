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

# Wait for local-server to be ready
echo "Waiting for local-server to be ready..."
until nc -z local-server 8001; do
    echo "Local-server not ready yet, waiting..."
    sleep 1
done
echo "Local-server is ready!"

# Start the tunnel
echo "Starting SSH tunnel to forward remote ports to local-server..."
exec /app/tut -config /config/config.yaml
