# Testing SSH-Socat-Tunnel

This directory contains test infrastructure for the ssh-socat-tunnel project.

## GitHub Actions Testing

The `.github/workflows/test.yml` workflow automatically tests both TCP and UDP tunnel functionality on every push and pull request.

### Test Architecture

The test sets up a complete tunnel environment in GitHub Actions:

```
┌─────────────────────────────────────────────────────────────┐
│ GitHub Actions Runner (localhost)                           │
│                                                              │
│  ┌──────────────┐         ┌──────────────┐                 │
│  │ TCP Server   │         │ UDP Server   │                 │
│  │ Port 8001    │         │ Port 8002    │                 │
│  └──────┬───────┘         └──────┬───────┘                 │
│         │                        │                          │
│         │                        │                          │
│  ┌──────▼─────────────────────┬──▼─────────────┐          │
│  │ Local socat wrappers       │                 │          │
│  │ (for UDP → TCP conversion) │                 │          │
│  └────────────┬───────────────┴─────────────────┘          │
│               │                                             │
│               │                                             │
│         ┌─────▼─────────────────────────────────┐          │
│         │ ssh-socat-tunnel                      │          │
│         │ (establishes SSH tunnel)              │          │
│         └─────┬─────────────────────────────────┘          │
│               │                                             │
│               │ SSH tunnel (port forwarding)               │
│         ┌─────▼─────────────────────────────────┐          │
│         │ SSH Server (localhost:2222)           │          │
│         │ - Forwards port 9001 → 127.0.0.1:8001│          │
│         │ - Forwards port 9002 (UDP via wrapper)│          │
│         └───────────────────────────────────────┘          │
│               │                                             │
│  ┌────────────▼───────────┐    ┌─────────────────┐        │
│  │ Remote socat wrappers  │    │                  │        │
│  │ (TCP → UDP conversion) │    │                  │        │
│  └────────────┬───────────┘    └──────────────────┘        │
│               │                                             │
│  ┌────────────▼──────────┐     ┌──────────────────┐       │
│  │ Test Client           │     │ Test Client      │        │
│  │ TCP Port 9001         │     │ UDP Port 9002    │        │
│  └───────────────────────┘     └──────────────────┘        │
└─────────────────────────────────────────────────────────────┘
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
   - Verify bidirectional communication
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

# Configure SSH server (add to /etc/ssh/sshd_config.d/test.conf)
# Then restart SSH

# Build the tunnel
go build -o ssh-socat-tunnel .

# Create test config (see .github/workflows/test.yml)
# Start test servers
# Run tunnel
# Run tests
```

### Test Coverage

The workflow tests:
- ✅ TCP tunnel establishment
- ✅ TCP bidirectional data flow
- ✅ TCP multiple sequential messages
- ✅ UDP tunnel establishment
- ✅ UDP packet transmission
- ✅ UDP multiple packets
- ✅ Tunnel resilience and stability
- ✅ Automatic cleanup

### Troubleshooting

If tests fail, check the logs output in the "Collect logs on failure" step:
- Tunnel logs show the main tunnel process output
- TCP/UDP server logs show the echo server activity
- Socat logs show the FIFO wrapper processes
- Network connections show active ports and listeners
