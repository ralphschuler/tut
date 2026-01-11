#!/bin/bash
set -e

echo "Starting SSH server (remote/VPS)..."

# Copy SSH public key for testuser
if [ -f /ssh-keys/id_ed25519.pub ]; then
    cp /ssh-keys/id_ed25519.pub /home/testuser/.ssh/authorized_keys
    chmod 600 /home/testuser/.ssh/authorized_keys
    chown testuser:testuser /home/testuser/.ssh/authorized_keys
    echo "SSH key installed for testuser"
fi

# Start SSH server (no echo servers here - they run on local machine)
echo "Starting SSH server..."
/usr/sbin/sshd -D -e
