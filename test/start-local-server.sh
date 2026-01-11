#!/bin/bash
set -e

echo "Starting local server (separate from tunnel client)..."

# Start TCP echo server on port 8001
echo "Starting TCP echo server on 0.0.0.0:8001..."
socat -v TCP-LISTEN:8001,bind=0.0.0.0,reuseaddr,fork EXEC:'/bin/cat' &

# Start UDP echo server on port 8002
echo "Starting UDP echo server on 0.0.0.0:8002..."
socat -v UDP-LISTEN:8002,bind=0.0.0.0,reuseaddr,fork EXEC:'/bin/cat' &

echo "Local server echo services started"
echo "TCP echo: 0.0.0.0:8001"
echo "UDP echo: 0.0.0.0:8002"

# Keep container running and monitor the processes
wait