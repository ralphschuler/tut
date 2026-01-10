# Testing Documentation

## Overview

This project includes automated testing via GitHub Actions that validates both TCP and UDP tunnel functionality.

## Test Workflow

The test workflow (`.github/workflows/test.yml`) runs automatically on:
- Every push to `main` or `master` branch
- Every pull request to `main` or `master` branch

## What Gets Tested

### TCP Tunnel Testing
- **Establishment**: Verifies that TCP tunnel can be created successfully
- **Data Flow**: Tests sending data from client → tunnel → server
- **Echo Response**: Validates echo round-trip communication
- **Stability**: Tests with several sequential messages to check basic stability

### UDP Tunnel Testing
- **Establishment**: Verifies that UDP tunnel can be created successfully  
- **Packet Transmission**: Tests sending UDP packets through the tunnel
- **Multiple Packets**: Sends several packets to verify stability

## Test Architecture

The test creates a complete tunnel environment on a single GitHub Actions runner:

1. **SSH Server**: Runs on localhost:2222 to simulate a VPS
2. **Test Services**: 
   - TCP echo server on port 8001
   - UDP echo server on port 8002
3. **Tunnel Configuration**:
   - TCP forward: 9001 (remote) → 8001 (local)
   - UDP forward: 9002 (remote) → 8002 (local) via wrap port 10000
4. **Test Clients**: Connect to forwarded ports (9001, 9002) to verify functionality

## Viewing Test Results

1. Go to the repository's "Actions" tab on GitHub
2. Select the "Test TCP and UDP Tunnels" workflow
3. Click on a specific run to see detailed logs
4. Check individual steps for pass/fail status

If tests fail, the workflow automatically collects and displays:
- Tunnel process logs
- TCP/UDP server logs  
- Socat wrapper logs
- Network connection status
- Running processes

## Running Tests Locally

See `test/README.md` for instructions on setting up a local test environment.

## Contributing

When making changes to the tunnel implementation:
1. Ensure existing tests still pass
2. Add new test cases for new functionality
3. Update this documentation if test coverage changes
