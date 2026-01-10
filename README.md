# ssh-socat-tunnel

**ssh-socat-tunnel** is a lightweight tool to expose local TCP and UDP services through a remote VPS using `ssh` and `socat`.

The program reads a YAML configuration describing your VPS and a set of TCP and UDP forward definitions. It then:

* Starts `socat` locally with FIFO (named pipes) to wrap UDP listeners into TCP listeners for stable bidirectional communication.
* Opens an SSH connection to your VPS with reverse port forwards for both TCP and the wrapped UDP TCP ports.
* Runs a small shell script on the VPS to start remote UDP listeners via `socat` with FIFO pipes and forward them back through the SSH tunnel.
* Keeps the connection alive and automatically reconnects if the tunnel drops.

The result is that services running on your local machine (even those listening on private addresses) become accessible via the public ports on your VPS.

## Features

* Works on Linux, macOS and other platforms where `ssh` and `socat` are available.
* Supports multiple TCP and UDP forwards simultaneously.
* **FIFO-based UDP tunneling** for improved stability and bidirectional communication (following best practices from [this guide](https://superuser.com/questions/53103/udp-traffic-through-ssh-tunnel)).
* Health checks to ensure local listeners are active before connecting.
* Automatic reconnection if the SSH tunnel drops.

## Requirements

* Go 1.21 or newer to build the binary.
* `ssh` installed locally to establish the tunnel.
* `socat` installed on both local and remote hosts to wrap UDP and forward traffic.

## Usage

1. Copy `config.yaml.example` to `/etc/ssh-tunnel/config.yaml` (or any path you prefer) and edit the values:

   ```yaml
   vps:
     host: "vps.example.com"
     user: "root"
     ssh_key: "/home/user/.ssh/id_ed25519"
   reconnect_delay_seconds: 2
   tcp_forwards:
     - remote_port: 25565
       local_host: "192.168.1.50"
       local_port: 25565
   udp_forwards:
     - udp_public_port: 19132
       local_host: "192.168.1.50"
       local_udp_port: 19132
       wrap_tcp_port: 10000
   ```

2. Build the binary:

   ```bash
   go build -o ssh-socat-tunnel ./ssh-socat-tunnel
   ```

3. Run the tunnel as root (binding low ports requires privileges):

   ```bash
   sudo ./ssh-socat-tunnel -config /etc/ssh-tunnel/config.yaml
   ```

The program will log its actions and reconnect if the SSH session drops.

### Running as a service

For production use you should run the tunnel as a supervised service. On systemd systems you can use the following unit definition:

```ini
[Unit]
Description=SSH Socat UDP Tunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ssh-socat-tunnel -config /etc/ssh-tunnel/config.yaml
Restart=always
RestartSec=2
User=root

[Install]
WantedBy=multi-user.target
```

Reload systemd and enable the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now ssh-socat-tunnel
```

### Cross‑compilation

The code does not use any cgo features, so Go can cross‑compile it easily. Example for Linux ARM64:

```bash
GOOS=linux GOARCH=arm64 go build -o ssh-socat-tunnel-linux-arm64 ./ssh-socat-tunnel
```

## GitHub Actions

This repository includes a GitHub Actions workflow (`.github/workflows/release.yml`) that builds the binary for several common OS/architecture combinations and attaches them as artifacts when you push a tag like `v1.0.0`.

To release a new version:

1. Ensure your repository has a valid [GitHub token](https://docs.github.com/en/actions/security-guides/encrypted-secrets) to create releases.
2. Create a git tag following semver (e.g. `git tag v1.0.0 && git push --tags`).
3. GitHub Actions will build binaries for Linux, Windows and macOS (amd64 and arm64 variants) and attach them to the release.

You can download the artifacts from the release page once the workflow completes.

## License

This project is provided under the MIT License. See `LICENSE` for details.
