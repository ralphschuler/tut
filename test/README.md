# Testing TUT (TCP UDP TUNNEL)

This directory contains test infrastructure for the tut project.

## GitHub Actions Testing

The `.github/workflows/test.yml` workflow automatically tests both TCP and UDP tunnel functionality on every push and pull request using Docker containers.

### Test Architecture

The test uses Docker Compose to create 4 separate containers that simulate a realistic tunnel environment:

```
┌──────────────────────────────┐
│  test-client (172.20.0.4)    │  Test Client
│  - Sends test data           │  (Simulates external users)
│  - Verifies connectivity     │
└────────────┬─────────────────┘
             │
             │ Connects to remote:9001 (TCP)
             │            remote:9002 (UDP)
             ▼
┌──────────────────────────────┐
│  remote (172.20.0.2)         │  VPS/SSH Server
│  - SSH server on port 22     │  (Simulates remote server)
│  - Accepts tunnel from local │
│  - Exposes 9001 (TCP)        │
│  - Exposes 9002 (UDP)        │
└────────────▲─────────────────┘
             │
             │ SSH tunnel connection
             │
┌────────────┴─────────────────┐
│  local (172.20.0.3)          │  Tunnel Client
│  - tut client                │  (Simulates user's machine)
│  - Establishes SSH tunnel    │
│  - Forwards to local-server  │
└────────────┬─────────────────┘
             │
             │ Forwards to local-server:8001/8002
             ▼
┌──────────────────────────────┐
│  local-server (172.20.0.5)   │  Local Server
│  - TCP echo server :8001     │  (Simulates separate local service)
│  - UDP echo server :8002     │
└──────────────────────────────┘
```

### How It Works

1. **Remote Container (VPS Simulation)**:
   - Runs SSH server on port 22
   - Exposes forwarded ports 9001 (TCP) and 9002 (UDP)
   - Configured to accept port forwarding from the tunnel

2. **Local Container (Tunnel Client)**:
   - Builds and runs tut
   - Connects to remote via SSH
   - Creates remote port forwards:
     - TCP: remote:9001 → local-server:8001 (via local tunnel client)
     - UDP: remote:9002 → local-server:8002 (via TCP wrapper)

3. **Local Server Container (Separate Local Service)**:
   - Runs TCP echo server on 0.0.0.0:8001
   - Runs UDP echo server on 0.0.0.0:8002
   - Represents a separate server on the local network (not the machine running tut)

4. **Test Client Container**:
   - Sends test data to remote:9001 (TCP)
   - Sends test data to remote:9002 (UDP)
   - Verifies echo responses

### Running Tests Locally

You can run the same tests locally using Docker:

```bash
# Setup test environment (generates SSH keys and config)
cd test
bash setup-test-env.sh

# Build and start containers
docker compose build
docker compose up -d

# Wait a few seconds for tunnel to establish
sleep 10

# Run tests
bash run-docker-tests.sh

# View logs
docker logs local    # Tunnel client logs
docker logs remote   # SSH server logs

# Stop and clean up
docker compose down -v
```

### Test Coverage

The workflow tests:
- ✅ TCP tunnel establishment from third-party client
- ✅ TCP echo round-trip communication through full chain (test-client → remote → local → local-server)
- ✅ TCP multiple sequential messages
- ✅ UDP tunnel establishment from third-party client (via TCP wrapper)
- ✅ UDP packet transmission through full chain (test-client → remote → local → local-server)
- ✅ UDP multiple packets
- ✅ Proper container isolation
- ✅ Realistic multi-host environment with separate local server
- ✅ Tunnel forwarding to a different machine (local-server) on the local network

### Advantages of Docker-Based Testing

- **Isolation**: Each component runs in its own container
- **Reproducibility**: Same environment every time
- **Realistic**: Simulates actual multi-host deployment
- **No port conflicts**: Containers use internal networking
- **Clean state**: Fresh environment for each test run
- **Easy debugging**: Can inspect logs and exec into containers

### Troubleshooting

If tests fail:

1. **Check container status**:
   ```bash
   cd test
   docker compose ps
   ```

2. **View logs**:
   ```bash
   docker logs local    # Tunnel client
   docker logs remote   # SSH server
   docker logs test-client
   ```

3. **Exec into containers**:
   ```bash
   docker exec -it local /bin/bash
   docker exec -it remote /bin/bash
   docker exec -it test-client /bin/bash
   ```

4. **Test connectivity manually**:
   ```bash
   # From test-client, try to connect to tunnel endpoints
   docker exec test-client nc -z remote 9001
   docker exec test-client nc -u -z remote 9002
   ```

5. **Rebuild containers**:
   ```bash
   docker compose down -v
   docker compose build --no-cache
   docker compose up -d
   ```
