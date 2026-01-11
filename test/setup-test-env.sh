#!/bin/bash
set -e

echo "Setting up test environment..."

# Create directories (relative to test/ directory)
mkdir -p ssh-keys
mkdir -p config

# Generate SSH key pair if it doesn't exist
if [ ! -f ssh-keys/id_ed25519 ]; then
    echo "Generating SSH key pair..."
    ssh-keygen -t ed25519 -f ssh-keys/id_ed25519 -N "" -C "test@tut"
    echo "SSH key pair generated"
else
    echo "SSH key pair already exists"
fi

# Create test configuration file
cat > config/config.yaml <<EOF
vps:
  host: "remote"
  user: "testuser"
  port: 22
  ssh_key: "/ssh-keys/id_ed25519"
  strict_hostkey: "no"

reconnect_delay_seconds: 2

tcp_forwards:
  - remote_port: 9001
    local_host: "local-server"
    local_port: 8001

udp_forwards:
  - udp_public_port: 9002
    local_host: "local-server"
    local_udp_port: 8002
    wrap_tcp_port: 10000
EOF

echo "Test configuration created"
echo "Setup complete!"
