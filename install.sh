#!/bin/sh
# Install script for tut (TCP UDP TUNNEL)
# This script installs tut on various Unix-like systems with different package managers
# and service managers (systemd, openrc, runit, etc.)

set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    printf "${GREEN}[INFO]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS="$(uname -s)"
    ARCH="$(uname -m)"
    
    case "$OS" in
        Linux*)     OS="linux";;
        Darwin*)    OS="darwin";;
        FreeBSD*)   OS="freebsd";;
        *)          error "Unsupported OS: $OS";;
    esac
    
    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64";;
        aarch64|arm64)  ARCH="arm64";;
        armv7l)         ARCH="arm";;
        i386|i686)      ARCH="386";;
        *)              error "Unsupported architecture: $ARCH";;
    esac
    
    info "Detected platform: $OS/$ARCH"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Detect package manager
detect_package_manager() {
    if command_exists apt-get; then
        PKG_MANAGER="apt-get"
        PKG_INSTALL="apt-get install -y"
    elif command_exists yum; then
        PKG_MANAGER="yum"
        PKG_INSTALL="yum install -y"
    elif command_exists dnf; then
        PKG_MANAGER="dnf"
        PKG_INSTALL="dnf install -y"
    elif command_exists pacman; then
        PKG_MANAGER="pacman"
        PKG_INSTALL="pacman -S --noconfirm"
    elif command_exists apk; then
        PKG_MANAGER="apk"
        PKG_INSTALL="apk add"
    elif command_exists zypper; then
        PKG_MANAGER="zypper"
        PKG_INSTALL="zypper install -y"
    elif command_exists brew; then
        PKG_MANAGER="brew"
        PKG_INSTALL="brew install"
    else
        PKG_MANAGER="unknown"
        warn "Could not detect package manager"
    fi
    
    if [ "$PKG_MANAGER" != "unknown" ]; then
        info "Detected package manager: $PKG_MANAGER"
    fi
}

# Detect service manager
detect_service_manager() {
    if command_exists systemctl && [ -d /run/systemd/system ]; then
        SERVICE_MANAGER="systemd"
    elif command_exists rc-service && [ -f /etc/init.d/networking ]; then
        SERVICE_MANAGER="openrc"
    elif command_exists sv && [ -d /etc/sv ]; then
        SERVICE_MANAGER="runit"
    elif [ -f /etc/rc.conf ] && [ -d /usr/local/etc/rc.d ]; then
        SERVICE_MANAGER="bsd"
    elif command_exists launchctl && [ "$OS" = "darwin" ]; then
        SERVICE_MANAGER="launchd"
    else
        SERVICE_MANAGER="unknown"
        warn "Could not detect service manager"
    fi
    
    if [ "$SERVICE_MANAGER" != "unknown" ]; then
        info "Detected service manager: $SERVICE_MANAGER"
    fi
}

# Check and install dependencies
check_dependencies() {
    info "Checking dependencies..."
    
    MISSING_DEPS=""
    
    # Check for ssh
    if ! command_exists ssh; then
        MISSING_DEPS="$MISSING_DEPS openssh-client"
        warn "SSH client not found"
    fi
    
    # Check for socat
    if ! command_exists socat; then
        MISSING_DEPS="$MISSING_DEPS socat"
        warn "socat not found"
    fi
    
    # Check for go (needed for building)
    if ! command_exists go; then
        MISSING_DEPS="$MISSING_DEPS go"
        warn "Go not found (needed for building tut)"
    fi
    
    if [ -n "$MISSING_DEPS" ]; then
        if [ "$PKG_MANAGER" = "unknown" ]; then
            error "Missing dependencies: $MISSING_DEPS. Please install them manually."
        else
            info "Installing missing dependencies: $MISSING_DEPS"
            case "$PKG_MANAGER" in
                apt-get)
                    sudo $PKG_INSTALL openssh-client socat golang-go
                    ;;
                yum|dnf)
                    sudo $PKG_INSTALL openssh-clients socat golang
                    ;;
                pacman)
                    sudo $PKG_INSTALL openssh socat go
                    ;;
                apk)
                    sudo $PKG_INSTALL openssh-client socat go
                    ;;
                zypper)
                    sudo $PKG_INSTALL openssh socat go
                    ;;
                brew)
                    $PKG_INSTALL socat go
                    ;;
            esac
        fi
    else
        info "All dependencies are already installed"
    fi
}

# Build the binary
build_binary() {
    info "Building tut binary..."
    
    if ! command_exists go; then
        error "Go compiler not found. Cannot build tut."
    fi
    
    # Build in the current directory
    CGO_ENABLED=0 go build -o tut .
    
    if [ ! -f "./tut" ]; then
        error "Build failed"
    fi
    
    info "Build successful"
}

# Install binary
install_binary() {
    info "Installing tut binary to /usr/local/bin..."
    
    if [ ! -f "./tut" ]; then
        error "Binary not found. Build first."
    fi
    
    sudo install -m 755 tut /usr/local/bin/tut
    
    info "Binary installed successfully"
}

# Create config directory and file
setup_config() {
    info "Setting up configuration..."
    
    # Determine config directory based on whether running as root or user
    if [ "$(id -u)" = "0" ]; then
        # Running as root, use /etc/tut
        CONFIG_DIR="/etc/tut"
    else
        # Running as normal user, use $HOME/.config/tut
        CONFIG_DIR="${HOME}/.config/tut"
    fi
    
    mkdir -p "$CONFIG_DIR"
    
    CONFIG_FILE="$CONFIG_DIR/config.yaml"
    
    if [ -f "$CONFIG_FILE" ]; then
        warn "Config file already exists at $CONFIG_FILE"
        warn "Backing up to $CONFIG_FILE.backup"
        cp "$CONFIG_FILE" "$CONFIG_FILE.backup"
    fi
    
    # Copy example config
    if [ -f "config.yaml.example" ]; then
        cp config.yaml.example "$CONFIG_FILE"
        info "Configuration template created at $CONFIG_FILE"
        info "Please edit $CONFIG_FILE with your VPS details"
    else
        error "config.yaml.example not found"
    fi
    
    # Create log directory
    sudo mkdir -p /var/log
    sudo chmod 755 /var/log
}

# Setup SSH configuration
setup_ssh() {
    info "Configuring SSH..."
    
    SSH_DIR="${HOME}/.ssh"
    mkdir -p "$SSH_DIR"
    chmod 700 "$SSH_DIR"
    
    # Check if SSH key exists
    if [ ! -f "$SSH_DIR/id_ed25519" ] && [ ! -f "$SSH_DIR/id_rsa" ]; then
        warn "No SSH key found in $SSH_DIR"
        info "Generating SSH key pair..."
        ssh-keygen -t ed25519 -f "$SSH_DIR/id_ed25519" -N "" -C "tut@$(hostname)"
        info "SSH key generated at $SSH_DIR/id_ed25519"
        info "Please copy the public key to your VPS:"
        info "  ssh-copy-id -i $SSH_DIR/id_ed25519.pub user@your.vps.hostname"
    else
        info "SSH key already exists"
    fi
    
    # Update SSH config to be more permissive for automation
    SSH_CONFIG="$SSH_DIR/config"
    if [ ! -f "$SSH_CONFIG" ]; then
        cat > "$SSH_CONFIG" << 'EOF'
# SSH configuration for tut
Host *
    ServerAliveInterval 15
    ServerAliveCountMax 3
    StrictHostKeyChecking accept-new
EOF
        chmod 600 "$SSH_CONFIG"
        info "SSH config created at $SSH_CONFIG"
    fi
}

# Create systemd service
create_systemd_service() {
    info "Creating systemd service..."
    
    # Determine config path
    if [ "$(id -u)" = "0" ]; then
        CONFIG_PATH="/etc/tut/config.yaml"
    else
        CONFIG_PATH="${HOME}/.config/tut/config.yaml"
    fi
    
    SERVICE_FILE="/etc/systemd/system/tut.service"
    
    sudo tee "$SERVICE_FILE" > /dev/null << EOF
[Unit]
Description=TUT - TCP UDP Tunnel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/tut -config $CONFIG_PATH
Restart=always
RestartSec=2
User=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    sudo systemctl daemon-reload
    info "Systemd service created at $SERVICE_FILE"
}

# Create OpenRC service
create_openrc_service() {
    info "Creating OpenRC service..."
    
    if [ "$(id -u)" = "0" ]; then
        CONFIG_PATH="/etc/tut/config.yaml"
    else
        CONFIG_PATH="${HOME}/.config/tut/config.yaml"
    fi
    
    SERVICE_FILE="/etc/init.d/tut"
    
    sudo tee "$SERVICE_FILE" > /dev/null << EOF
#!/sbin/openrc-run

name="tut"
description="TUT - TCP UDP Tunnel"
command="/usr/local/bin/tut"
command_args="-config $CONFIG_PATH"
command_background=true
pidfile="/var/run/tut.pid"

depend() {
    need net
    after firewall
}
EOF
    
    sudo chmod +x "$SERVICE_FILE"
    info "OpenRC service created at $SERVICE_FILE"
}

# Create runit service
create_runit_service() {
    info "Creating runit service..."
    
    if [ "$(id -u)" = "0" ]; then
        CONFIG_PATH="/etc/tut/config.yaml"
    else
        CONFIG_PATH="${HOME}/.config/tut/config.yaml"
    fi
    
    SERVICE_DIR="/etc/sv/tut"
    sudo mkdir -p "$SERVICE_DIR"
    
    sudo tee "$SERVICE_DIR/run" > /dev/null << EOF
#!/bin/sh
exec 2>&1
exec /usr/local/bin/tut -config $CONFIG_PATH
EOF
    
    sudo chmod +x "$SERVICE_DIR/run"
    
    # Enable service
    sudo ln -sf "$SERVICE_DIR" /var/service/
    
    info "Runit service created at $SERVICE_DIR"
}

# Create BSD rc.d service
create_bsd_service() {
    info "Creating BSD rc.d service..."
    
    if [ "$(id -u)" = "0" ]; then
        CONFIG_PATH="/etc/tut/config.yaml"
    else
        CONFIG_PATH="${HOME}/.config/tut/config.yaml"
    fi
    
    SERVICE_FILE="/usr/local/etc/rc.d/tut"
    
    sudo tee "$SERVICE_FILE" > /dev/null << EOF
#!/bin/sh
#
# PROVIDE: tut
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="tut"
rcvar=tut_enable
command="/usr/local/bin/tut"
command_args="-config $CONFIG_PATH"
pidfile="/var/run/tut.pid"

load_rc_config \$name
run_rc_command "\$1"
EOF
    
    sudo chmod +x "$SERVICE_FILE"
    
    # Add to rc.conf
    if ! grep -q "tut_enable" /etc/rc.conf 2>/dev/null; then
        echo 'tut_enable="YES"' | sudo tee -a /etc/rc.conf
    fi
    
    info "BSD rc.d service created at $SERVICE_FILE"
}

# Create launchd service (macOS)
create_launchd_service() {
    info "Creating launchd service..."
    
    CONFIG_PATH="${HOME}/.config/tut/config.yaml"
    PLIST_FILE="${HOME}/Library/LaunchAgents/com.tut.plist"
    LOG_DIR="${HOME}/Library/Logs"
    
    mkdir -p "${HOME}/Library/LaunchAgents"
    mkdir -p "$LOG_DIR"
    
    cat > "$PLIST_FILE" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.tut</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/tut</string>
        <string>-config</string>
        <string>$CONFIG_PATH</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>$LOG_DIR/tut.log</string>
    <key>StandardErrorPath</key>
    <string>$LOG_DIR/tut.err</string>
</dict>
</plist>
EOF
    
    info "Launchd plist created at $PLIST_FILE"
}

# Setup service
setup_service() {
    case "$SERVICE_MANAGER" in
        systemd)
            create_systemd_service
            ;;
        openrc)
            create_openrc_service
            ;;
        runit)
            create_runit_service
            ;;
        bsd)
            create_bsd_service
            ;;
        launchd)
            create_launchd_service
            ;;
        *)
            warn "Could not setup service automatically"
            warn "Please create a service file manually"
            return
            ;;
    esac
}

# Enable and start service
enable_service() {
    info "Enabling and starting tut service..."
    
    case "$SERVICE_MANAGER" in
        systemd)
            sudo systemctl enable tut.service
            info "Service enabled. To start now, run: sudo systemctl start tut"
            info "To check status: sudo systemctl status tut"
            ;;
        openrc)
            sudo rc-update add tut default
            info "Service enabled. To start now, run: sudo rc-service tut start"
            info "To check status: sudo rc-service tut status"
            ;;
        runit)
            info "Service enabled automatically with runit"
            info "To check status: sudo sv status tut"
            ;;
        bsd)
            info "Service enabled in /etc/rc.conf"
            info "To start now, run: sudo service tut start"
            info "To check status: sudo service tut status"
            ;;
        launchd)
            launchctl load "$PLIST_FILE"
            info "Service loaded. To check status: launchctl list | grep tut"
            ;;
        *)
            warn "Could not enable service automatically"
            ;;
    esac
}

# Print final instructions
print_instructions() {
    echo ""
    info "============================================"
    info "Installation complete!"
    info "============================================"
    echo ""
    info "Next steps:"
    echo ""
    
    if [ "$(id -u)" = "0" ]; then
        CONFIG_FILE="/etc/tut/config.yaml"
    else
        CONFIG_FILE="${HOME}/.config/tut/config.yaml"
    fi
    
    echo "  1. Edit the configuration file:"
    echo "     $CONFIG_FILE"
    echo ""
    echo "  2. Update these settings:"
    echo "     - vps.host: Your VPS hostname or IP"
    echo "     - vps.user: SSH user on your VPS"
    echo "     - vps.ssh_key: Path to your SSH private key"
    echo "     - Configure your TCP and UDP forwards"
    echo ""
    echo "  3. Copy your SSH public key to your VPS:"
    if [ -f "${HOME}/.ssh/id_ed25519.pub" ]; then
        echo "     ssh-copy-id -i ${HOME}/.ssh/id_ed25519.pub user@your.vps.hostname"
    elif [ -f "${HOME}/.ssh/id_rsa.pub" ]; then
        echo "     ssh-copy-id -i ${HOME}/.ssh/id_rsa.pub user@your.vps.hostname"
    else
        echo "     ssh-copy-id user@your.vps.hostname"
    fi
    echo ""
    echo "  4. Ensure 'socat' is installed on your VPS:"
    echo "     SSH to your VPS and run: sudo apt-get install socat"
    echo "     (or equivalent for your VPS's package manager)"
    echo ""
    
    case "$SERVICE_MANAGER" in
        systemd)
            echo "  5. Start the service:"
            echo "     sudo systemctl start tut"
            echo ""
            echo "  6. Check the service status:"
            echo "     sudo systemctl status tut"
            echo "     sudo journalctl -u tut -f"
            ;;
        openrc)
            echo "  5. Start the service:"
            echo "     sudo rc-service tut start"
            echo ""
            echo "  6. Check the service status:"
            echo "     sudo rc-service tut status"
            ;;
        runit)
            echo "  5. Check the service status:"
            echo "     sudo sv status tut"
            echo "     sudo sv up tut"
            ;;
        bsd)
            echo "  5. Start the service:"
            echo "     sudo service tut start"
            echo ""
            echo "  6. Check the service status:"
            echo "     sudo service tut status"
            ;;
        launchd)
            echo "  5. Service is already loaded"
            echo ""
            echo "  6. Check the service status:"
            echo "     launchctl list | grep tut"
            echo "     tail -f ${HOME}/Library/Logs/tut.log"
            ;;
        *)
            echo "  5. Start tut manually or create a service file"
            echo "     /usr/local/bin/tut -config $CONFIG_FILE"
            ;;
    esac
    
    echo ""
    info "============================================"
}

# Main installation flow
main() {
    info "Starting tut installation..."
    
    # Check if we're in the tut repository
    if [ ! -f "main.go" ] || [ ! -f "config.yaml.example" ]; then
        error "Please run this script from the tut repository directory"
    fi
    
    detect_platform
    detect_package_manager
    detect_service_manager
    check_dependencies
    build_binary
    install_binary
    setup_config
    setup_ssh
    setup_service
    enable_service
    print_instructions
}

# Run main
main
