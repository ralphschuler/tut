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

# Start TCP echo server on port 8001 (local service to tunnel)
echo "Starting TCP echo server on port 8001..."
socat -v TCP-LISTEN:8001,bind=127.0.0.1,reuseaddr,fork EXEC:'/bin/cat' &

# Start UDP echo server on port 8002 (local service to tunnel)
echo "Starting UDP echo server on port 8002..."
socat -v UDP-LISTEN:8002,bind=127.0.0.1,reuseaddr,fork EXEC:'/bin/cat' &

# Wait a bit for echo servers to start
sleep 2

# Start SSH server
echo "Starting SSH server..."
/usr/sbin/sshd -D -e
