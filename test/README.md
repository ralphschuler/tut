# Testing SSH-Socat-Tunnel

This directory contains test infrastructure for the ssh-socat-tunnel project.

## GitHub Actions Testing

The `.github/workflows/test.yml` workflow automatically tests both TCP and UDP tunnel functionality on every push and pull request.

### Test Architecture

The test sets up a complete tunnel environment in GitHub Actions:

```
                ┌───────────────────────────────────┐
                │   Remote Test Environment         │
                │                                   │
                │   ┌───────────────────────────┐   │
                │   │ Test Client (TCP)         │   │
                │   │ connects to remote 9001   │   │
                │   └─────────────┬─────────────┘   │
                │                 │                 │
                │   ┌─────────────▼─────────────┐   │
                │   │ Test Client (UDP)         │   │
                │   │ connects to remote 9002   │   │
                │   └─────────────┬─────────────┘   │
                └─────────────────│─────────────────┘
                                  │
                                  │  SSH connection
                                  ▼
                      ┌───────────────────────────────┐
                      │ SSH Server (localhost:2222)   │
                      │ - Remote 9001 → 127.0.0.1:8001│
                      │ - Remote 9002 → 127.0.0.1:8002│
                      └───────────────┬───────────────┘
                                      │
                                      │  SSH tunnel
                                      ▼
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions Runner (localhost)                           │
│                                                              │
│  ┌──────────────┐         ┌──────────────┐                 │
│  │ TCP Server   │         │ UDP Server   │                 │
│  │ listens 8001 │         │ listens 8002 │                 │
│  └──────┬───────┘         └──────┬───────┘                 │
│         │                        │                          │
│         │                        │                          │
│  ┌──────▼─────────────────────┬──▼─────────────┐          │
│  │ Local socat wrappers       │                 │          │
│  │ (for UDP → TCP conversion) │                 │          │
│  └────────────┬───────────────┴─────────────────┘          │
│               │                                             │
│               │ ssh-socat-tunnel                            │
│               │ (manages SSH tunnel and wrappers)           │
└───────────────┴──────────────────────────────────────────────┘
```

### Test Procedure

1. **Setup Phase**:
   - Install socat and SSH server
   - Generate SSH keys for testing
   - Configure SSH server on port 2222
   - Build the tunnel binary

2. **Service Setup**:
   - Start TCP echo server on port 8001
   - Start UDP echo server on port 8002
   - Create test configuration file
   - Start the tunnel

3. **TCP Tests**:
   - Send data through TCP tunnel (port 9001 → 8001)
   - Verify echo round-trip communication
   - Test multiple sequential messages

4. **UDP Tests**:
   - Send data through UDP tunnel (port 9002 → 8002)
   - Verify packets can be transmitted
   - Test multiple packets

### Running Tests Locally

While the tests are designed to run in GitHub Actions, you can simulate the test environment locally:

```bash
# Install dependencies (Ubuntu/Debian)
sudo apt-get install socat openssh-server netcat-openbsd

# Set up SSH server on custom port
sudo ssh-keygen -A
ssh-keygen -t ed25519 -f ~/.ssh/test_key -N ""
cat ~/.ssh/test_key.pub >> ~/.ssh/authorized_keys

# Configure SSH server (e.g. add to /etc/ssh/sshd_config.d/test.conf):
#   Port 2222
#   PubkeyAuthentication yes

# Restart or start SSH on the custom port:
# If using systemd, restart the SSH service:
sudo systemctl restart ssh || sudo systemctl restart sshd
# Or run a one-off sshd instance on port 2222:
sudo /usr/sbin/sshd -p 2222

# Build the tunnel
go build -o ssh-socat-tunnel .

# Example: create a minimal test config
cat > /tmp/test-config.yaml << 'EOF'
vps:
  host: "localhost"
  user: "$USER"
  port: 2222
  ssh_key: "$HOME/.ssh/test_key"
  strict_hostkey: "no"

reconnect_delay_seconds: 2

tcp_forwards:
  - remote_port: 9001
    local_host: "127.0.0.1"
    local_port: 8001

udp_forwards:
  - udp_public_port: 9002
    local_host: "127.0.0.1"
    local_udp_port: 8002
    wrap_tcp_port: 10000
EOF

# Start local TCP and UDP echo servers (as in CI)
socat -v TCP-LISTEN:8001,bind=127.0.0.1,reuseaddr,fork EXEC:'/bin/cat' > /tmp/tcp-server.log 2>&1 &
TCP_SERVER_PID=$!
socat -v UDP-LISTEN:8002,bind=127.0.0.1,reuseaddr,fork EXEC:'/bin/cat' > /tmp/udp-server.log 2>&1 &
UDP_SERVER_PID=$!

# Run tunnel using the test config
sudo ./ssh-socat-tunnel -config /tmp/test-config.yaml &
TUNNEL_PID=$!

# Wait for tunnel to establish
sleep 10

# Test TCP tunnel
echo "HELLO_TCP" | nc -w 2 localhost 9001

# Test UDP tunnel
echo "HELLO_UDP" | nc -u -w 2 localhost 9002

# Cleanup background processes
sudo kill "$TUNNEL_PID"
kill "$TCP_SERVER_PID" "$UDP_SERVER_PID"
```

### Test Coverage

The workflow tests:
- ✅ TCP tunnel establishment
- ✅ TCP echo round-trip communication
- ✅ TCP multiple sequential messages
- ✅ UDP tunnel establishment
- ✅ UDP packet transmission
- ✅ UDP multiple packets
- ✅ Automatic cleanup

### Troubleshooting

If tests fail, check the logs output in the "Collect logs on failure" step:
- Tunnel logs show the main tunnel process output
- TCP/UDP server logs show the echo server activity
- Socat logs show the FIFO wrapper processes
- Network connections show active ports and listeners
