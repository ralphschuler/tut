package main

import (
    "context"
    "errors"
    "flag"
    "fmt"
    "gopkg.in/yaml.v3"
    "net"
    "os"
    "os/exec"
    "os/signal"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"
    "time"
)

// Config represents the YAML configuration for the tunnel program.
// See config.example.yaml for a reference.
type Config struct {
    VPS struct {
        Host          string `yaml:"host"`
        User          string `yaml:"user"`
        Port          int    `yaml:"port"`
        SSHKey        string `yaml:"ssh_key"`
        StrictHostKey string `yaml:"strict_hostkey"`
    } `yaml:"vps"`
    ReconnectDelaySeconds int `yaml:"reconnect_delay_seconds"`
    TCPForwards []struct {
        RemotePort int    `yaml:"remote_port"`
        LocalHost  string `yaml:"local_host"`
        LocalPort  int    `yaml:"local_port"`
    } `yaml:"tcp_forwards"`
    UDPForwards []struct {
        UDPPublicPort int    `yaml:"udp_public_port"`
        LocalHost     string `yaml:"local_host"`
        LocalUDPPort  int    `yaml:"local_udp_port"`
        WrapTCPPort   int    `yaml:"wrap_tcp_port"`
    } `yaml:"udp_forwards"`
}

// logf prints a timestamped message to stdout.
func logf(format string, args ...any) {
    ts := time.Now().Format("2006-01-02T15:04:05-0700")
    fmt.Printf("%s %s\n", ts, fmt.Sprintf(format, args...))
}

// die prints an error message and exits the program.
func die(format string, args ...any) {
    fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
    os.Exit(1)
}

// isPort checks if a port is valid (1-65535).
func isPort(p int) bool {
    return p >= 1 && p <= 65535
}

// loadConfig reads and parses the YAML config at path.
// Defaults are applied for missing values.
func loadConfig(path string) (*Config, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var c Config
    if err := yaml.Unmarshal(b, &c); err != nil {
        return nil, err
    }
    if c.VPS.Port == 0 {
        c.VPS.Port = 22
    }
    if c.VPS.StrictHostKey == "" {
        c.VPS.StrictHostKey = "accept-new"
    }
    if c.ReconnectDelaySeconds <= 0 {
        c.ReconnectDelaySeconds = 2
    }
    return &c, nil
}

// validateConfig validates required config fields and value ranges.
func validateConfig(c *Config) error {
    if c.VPS.Host == "" || c.VPS.User == "" || c.VPS.SSHKey == "" {
        return errors.New("missing vps.host, vps.user or vps.ssh_key")
    }
    if !isPort(c.VPS.Port) {
        return fmt.Errorf("invalid vps.port: %d", c.VPS.Port)
    }
    if st, err := os.Stat(c.VPS.SSHKey); err != nil || st.IsDir() {
        return fmt.Errorf("SSH key not readable: %s", c.VPS.SSHKey)
    }
    for _, f := range c.TCPForwards {
        if !isPort(f.RemotePort) || !isPort(f.LocalPort) || f.LocalHost == "" {
            return fmt.Errorf("invalid tcp_forward: %+v", f)
        }
    }
    for _, u := range c.UDPForwards {
        if !isPort(u.UDPPublicPort) || !isPort(u.LocalUDPPort) || !isPort(u.WrapTCPPort) || u.LocalHost == "" {
            return fmt.Errorf("invalid udp_forward: %+v", u)
        }
    }
    return nil
}

// child wraps an exec.Cmd with context for clean termination.
type child struct {
    cmd   *exec.Cmd
    tag   string
    fifos []string // FIFO paths to clean up
}

// stop sends SIGTERM to the process and waits for a grace period before killing it.
func (c *child) stop(grace time.Duration) {
    if c == nil || c.cmd == nil || c.cmd.Process == nil {
        return
    }
    _ = c.cmd.Process.Signal(syscall.SIGTERM)
    done := make(chan struct{})
    go func() {
        _ = c.cmd.Wait()
        close(done)
    }()
    select {
    case <-done:
        return
    case <-time.After(grace):
        _ = c.cmd.Process.Kill()
        _ = c.cmd.Wait()
    }
    // Clean up FIFOs and directories
    for _, path := range c.fifos {
        info, err := os.Stat(path)
        if err == nil {
            if info.IsDir() {
                _ = os.RemoveAll(path)
            } else {
                _ = os.Remove(path)
            }
        }
    }
}

// dialLocal tries to connect to localhost:port with a timeout.
func dialLocal(port int, timeout time.Duration) error {
    d := net.Dialer{Timeout: timeout}
    conn, err := d.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
    if err != nil {
        return err
    }
    _ = conn.Close()
    return nil
}

// waitLocalListen waits up to maxWait for a local TCP port to start listening.
func waitLocalListen(port int, maxWait time.Duration) error {
    deadline := time.Now().Add(maxWait)
    for time.Now().Before(deadline) {
        if err := dialLocal(port, 150*time.Millisecond); err == nil {
            return nil
        }
        time.Sleep(75 * time.Millisecond)
    }
    return fmt.Errorf("port not listening: 127.0.0.1:%d", port)
}

// requireBinary asserts that the named binary exists in PATH.
func requireBinary(name string) {
    if _, err := exec.LookPath(name); err != nil {
        die("missing dependency: %s", name)
    }
}

// createFIFO creates a named pipe (FIFO) at the specified path.
// It removes any existing file at that path first.
func createFIFO(path string) error {
    _ = os.Remove(path)
    return syscall.Mkfifo(path, 0o600)
}

// startLocalWrappers starts socat processes to wrap each UDP forward using FIFO pipes.
// This implementation follows the stable FIFO-based approach from:
// https://superuser.com/questions/53103/udp-traffic-through-ssh-tunnel
// 
// Architecture: TCP-LISTEN ↔ UDP (to actual service) via PIPE for bidirectional flow
func startLocalWrappers(cfg *Config) ([]*child, error) {
    if len(cfg.UDPForwards) == 0 {
        logf("No udp_forwards configured; skipping local UDP wrappers.")
        return nil, nil
    }
    
    // Create secure temporary directory for FIFOs
    tmpDir, err := os.MkdirTemp("", "ssh-socat-tunnel-*")
    if err != nil {
        return nil, fmt.Errorf("failed to create temp directory: %w", err)
    }
    
    var kids []*child
    
    // Cleanup helper for error cases
    cleanup := func() {
        for _, k := range kids {
            k.stop(1 * time.Second)
        }
        _ = os.RemoveAll(tmpDir)
    }
    
    for _, u := range cfg.UDPForwards {
        // Create FIFO in secure temporary directory
        fifoPath := filepath.Join(tmpDir, fmt.Sprintf("pipe-%d", u.UDPPublicPort))
        
        // Create FIFO
        if err := createFIFO(fifoPath); err != nil {
            cleanup()
            return nil, fmt.Errorf("failed to create FIFO %s: %w", fifoPath, err)
        }
        
        llogTCP := fmt.Sprintf("/var/log/socat-local-tcp-%d.log", u.UDPPublicPort)
        llogUDP := fmt.Sprintf("/var/log/socat-local-udp-%d.log", u.UDPPublicPort)
        _ = os.MkdirAll(filepath.Dir(llogTCP), 0o755)
        
        // First socat: TCP-LISTEN → PIPE (receives from SSH tunnel)
        argsTCP := []string{
            "-T", "30",
            fmt.Sprintf("TCP4-LISTEN:%d,bind=127.0.0.1,reuseaddr,fork", u.WrapTCPPort),
            fmt.Sprintf("PIPE:%s", fifoPath),
        }
        cmdTCP := exec.Command("socat", argsTCP...)
        fTCP, err := os.OpenFile(llogTCP, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
            cleanup()
            return nil, err
        }
        cmdTCP.Stdout = fTCP
        cmdTCP.Stderr = fTCP
        if err := cmdTCP.Start(); err != nil {
            _ = fTCP.Close()
            cleanup()
            return nil, err
        }
        
        // Second socat: PIPE → UDP (forwards to actual local service)
        argsUDP := []string{
            "-T", "30",
            fmt.Sprintf("PIPE:%s", fifoPath),
            fmt.Sprintf("UDP:%s:%d", u.LocalHost, u.LocalUDPPort),
        }
        cmdUDP := exec.Command("socat", argsUDP...)
        fUDP, err := os.OpenFile(llogUDP, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
            (&child{cmd: cmdTCP, tag: "cleanup"}).stop(1 * time.Second)
            _ = fTCP.Close()
            cleanup()
            return nil, err
        }
        cmdUDP.Stdout = fUDP
        cmdUDP.Stderr = fUDP
        if err := cmdUDP.Start(); err != nil {
            (&child{cmd: cmdTCP, tag: "cleanup"}).stop(1 * time.Second)
            _ = fTCP.Close()
            _ = fUDP.Close()
            cleanup()
            return nil, err
        }
        
        kids = append(kids, &child{
            cmd:   cmdTCP,
            tag:   fmt.Sprintf("local-socat-tcp-%d", u.UDPPublicPort),
            fifos: []string{fifoPath},
        })
        kids = append(kids, &child{
            cmd: cmdUDP,
            tag: fmt.Sprintf("local-socat-udp-%d", u.UDPPublicPort),
        })
        
        logf("Local FIFO wrapper pid=%d/%d : TCP 127.0.0.1:%d <-> PIPE <-> UDP %s:%d (VPS UDP %d)",
            cmdTCP.Process.Pid, cmdUDP.Process.Pid, u.WrapTCPPort, u.LocalHost, u.LocalUDPPort, u.UDPPublicPort)
    }
    
    // Store tmpDir for cleanup on shutdown - add to first child
    if len(kids) > 0 {
        kids[0].fifos = append(kids[0].fifos, tmpDir)
    }
    
    return kids, nil
}

// assertLocalWrappers checks that the local socat TCP listeners are up.
func assertLocalWrappers(cfg *Config) error {
    for _, u := range cfg.UDPForwards {
        if err := waitLocalListen(u.WrapTCPPort, 3*time.Second); err != nil {
            return fmt.Errorf("local socat not listening for udp_public_port=%d wrap_tcp_port=%d: %w", u.UDPPublicPort, u.WrapTCPPort, err)
        }
    }
    logf("Local listener health-check OK")
    return nil
}

// buildSSHArgs assembles the arguments for the SSH command and returns them along with the target user@host.
func buildSSHArgs(cfg *Config) ([]string, string) {
    base := []string{
        "-i", cfg.VPS.SSHKey,
        "-p", strconv.Itoa(cfg.VPS.Port),
        "-o", "BatchMode=yes",
        "-o", "ExitOnForwardFailure=yes",
        "-o", "ServerAliveInterval=15",
        "-o", "ServerAliveCountMax=3",
        "-o", "StrictHostKeyChecking=" + cfg.VPS.StrictHostKey,
        "-T",
    }
    // Add TCP forwards
    for _, f := range cfg.TCPForwards {
        base = append(base, "-R", fmt.Sprintf("0.0.0.0:%d:%s:%d", f.RemotePort, f.LocalHost, f.LocalPort))
    }
    // Add UDP wrappers as TCP forwards
    for _, u := range cfg.UDPForwards {
        base = append(base, "-R", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", u.WrapTCPPort, u.WrapTCPPort))
    }
    target := fmt.Sprintf("%s@%s", cfg.VPS.User, cfg.VPS.Host)
    return base, target
}

// buildRemoteScript generates a POSIX shell script to run on the remote VPS via SSH.
// The script creates FIFO pipes and starts socat processes using the stable FIFO-based approach
// for bidirectional UDP tunneling.
func buildRemoteScript(cfg *Config) string {
    var b strings.Builder
    b.WriteString("set -eu; ")
    // ensure predictable PATH for non-interactive shells
    b.WriteString("export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$PATH; ")
    b.WriteString(`SOCAT_BIN="$(command -v socat || true)"; `)
    b.WriteString(`if [ -z "$SOCAT_BIN" ]; then echo "ERROR: socat not found on VPS. PATH=$PATH" >&2; exit 1; fi; `)
    b.WriteString(`pids=""; `)
    // Create secure temporary directory for FIFOs
    b.WriteString(`FIFO_DIR="$(mktemp -d -t ssh-socat-tunnel-XXXXXX)"; `)
    b.WriteString(`cleanup(){ for p in $pids; do kill "$p" 2>/dev/null || true; done; rm -rf "$FIFO_DIR" 2>/dev/null || true; }; `)
    b.WriteString(`trap cleanup INT TERM EXIT; `)
    if len(cfg.UDPForwards) == 0 {
        // Nothing to run; keep the SSH session alive
        b.WriteString("while true; do sleep 3600; done")
        return b.String()
    }
    for _, u := range cfg.UDPForwards {
        // best-effort kill any existing listener on the public port if fuser exists
        b.WriteString(fmt.Sprintf(`if command -v fuser >/dev/null 2>&1; then fuser -k %d/udp 2>/dev/null || true; fi; `, u.UDPPublicPort))
        
        // Create FIFO in secure temp directory
        b.WriteString(fmt.Sprintf(`FIFO_PATH="$FIFO_DIR/pipe-%d"; `, u.UDPPublicPort))
        b.WriteString(`mkfifo -m 600 "$FIFO_PATH"; `)
        
        // First socat: UDP-LISTEN → PIPE (receives from public UDP, writes to FIFO)
        b.WriteString(fmt.Sprintf(`"$SOCAT_BIN" -T 30 UDP-LISTEN:%d,bind=0.0.0.0,reuseaddr,fork PIPE:"$FIFO_PATH" >>/var/log/socat-udp-%d.log 2>&1 & `,
            u.UDPPublicPort, u.UDPPublicPort))
        b.WriteString(`pids="$pids $!"; `)
        
        // Second socat: PIPE → TCP (reads from FIFO, forwards to SSH tunnel)
        b.WriteString(fmt.Sprintf(`"$SOCAT_BIN" -T 30 PIPE:"$FIFO_PATH" TCP:127.0.0.1:%d >>/var/log/socat-tcp-%d.log 2>&1 & `,
            u.WrapTCPPort, u.UDPPublicPort))
        b.WriteString(`pids="$pids $!"; `)
    }
    // watchdog loop: if any child dies, exit to force reconnect
    b.WriteString(`while true; do `)
    b.WriteString(`for p in $pids; do if ! kill -0 "$p" 2>/dev/null; then echo "Child process $p died; exiting to reconnect" >&2; exit 1; fi; done; `)
    b.WriteString(`sleep 5; done`)
    return b.String()
}

// runTunnel starts the SSH tunnel and monitors it, restarting on failure.
func runTunnel(ctx context.Context, cfg *Config, localWrappers []*child) error {
    sshArgs, target := buildSSHArgs(cfg)
    script := buildRemoteScript(cfg)
    fullArgs := append(sshArgs, target, script)
    
    logf("Starting SSH tunnel to %s", target)
    cmd := exec.CommandContext(ctx, "ssh", fullArgs...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("failed to start SSH: %w", err)
    }
    
    logf("SSH tunnel running (PID %d)", cmd.Process.Pid)
    return cmd.Wait()
}

func main() {
    configPath := flag.String("config", "/etc/ssh-tunnel/config.yaml", "Path to config file")
    flag.Parse()
    
    requireBinary("ssh")
    requireBinary("socat")
    
    cfg, err := loadConfig(*configPath)
    if err != nil {
        die("Failed to load config: %v", err)
    }
    
    if err := validateConfig(cfg); err != nil {
        die("Invalid config: %v", err)
    }
    
    logf("Loaded config from %s", *configPath)
    
    // Start local UDP wrappers
    localWrappers, err := startLocalWrappers(cfg)
    if err != nil {
        die("Failed to start local wrappers: %v", err)
    }
    defer func() {
        for _, w := range localWrappers {
            w.stop(2 * time.Second)
        }
    }()
    
    // Verify local wrappers are listening
    if err := assertLocalWrappers(cfg); err != nil {
        die("Local wrapper health check failed: %v", err)
    }
    
    // Setup signal handling
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer cancel()
    
    // Main reconnect loop
    for {
        if ctx.Err() != nil {
            logf("Shutting down gracefully")
            return
        }
        
        if err := runTunnel(ctx, cfg, localWrappers); err != nil {
            if ctx.Err() != nil {
                logf("Tunnel terminated by signal")
                return
            }
            logf("Tunnel failed: %v", err)
        }
        
        logf("Reconnecting in %d seconds...", cfg.ReconnectDelaySeconds)
        select {
        case <-time.After(time.Duration(cfg.ReconnectDelaySeconds) * time.Second):
            // Continue to reconnect
        case <-ctx.Done():
            logf("Shutting down gracefully")
            return
        }
    }
}
