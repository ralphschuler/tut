# Testing Documentation

## Overview

This project includes automated testing via GitHub Actions that validates both TCP and UDP tunnel functionality using Docker containers to simulate a realistic multi-host environment.

## Test Workflow

The test workflow (`.github/workflows/test.yml`) runs automatically on:
- Every push to `main` or `master` branch
- Every pull request to `main` or `master` branch

## What Gets Tested

### TCP Tunnel Testing
- **Establishment**: Verifies that TCP tunnel can be created successfully
- **Data Flow**: Tests sending data from test client → remote VPS (via tunnel) → local service
- **Echo Response**: Validates echo round-trip communication
- **Stability**: Tests with several sequential messages to check basic stability

### UDP Tunnel Testing
- **Establishment**: Verifies that UDP tunnel can be created successfully (using TCP wrapper)
- **Packet Transmission**: Tests sending UDP packets through the tunnel
- **Multiple Packets**: Sends several packets to verify stability

## Test Architecture

The test uses **Docker Compose** to create 3 separate containers that simulate a realistic deployment:

### Container Setup

1. **remote (172.20.0.2)** - VPS/SSH Server:
   - Runs SSH server on port 22
   - Hosts TCP echo server on 127.0.0.1:8001
   - Hosts UDP echo server on 127.0.0.1:8002
   - Accepts SSH tunnel connections from local container
   - Exposes forwarded ports 9001 (TCP) and 9002 (UDP)

2. **local (172.20.0.3)** - Tunnel Client:
   - Builds and runs tut binary
   - Establishes SSH connection to remote
   - Creates port forwards:
     - TCP: remote:9001 → remote:127.0.0.1:8001
     - UDP: remote:9002 → remote:127.0.0.1:8002 (via TCP wrapper on port 10000)

3. **test-client (172.20.0.4)** - Test Client:
   - Sends test data to remote:9001 (TCP)
   - Sends test data to remote:9002 (UDP)
   - Verifies echo responses

### Why Docker?

The Docker-based approach provides:
- **Isolation**: Each component (client, server, test) runs in separate containers
- **Realistic simulation**: Mimics actual multi-host deployment
- **No port conflicts**: Uses internal Docker networking
- **Reproducibility**: Same environment every test run
- **Easier debugging**: Can inspect logs and exec into containers

## Viewing Test Results

1. Go to the repository's "Actions" tab on GitHub
2. Select the "Test TCP and UDP Tunnels" workflow
3. Click on a specific run to see detailed logs
4. Check individual steps for pass/fail status

If tests fail, the workflow automatically collects and displays:
- Local container logs (tunnel client)
- Remote container logs (SSH server)
- Test client logs
- Container status
- Running processes

## Running Tests Locally

See `test/README.md` for instructions on setting up a local test environment.

## Contributing

When making changes to the tunnel implementation:
1. Ensure existing tests still pass
2. Add new test cases for new functionality
3. Update this documentation if test coverage changes
