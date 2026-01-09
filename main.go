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
    cmd *exec.Cmd
    tag string
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

// startLocalWrappers starts socat processes to wrap each UDP forward into a TCP listener.
func startLocalWrappers(cfg *Config) ([]*child, error) {
    if len(cfg.UDPForwards) == 0 {
        logf("No udp_forwards configured; skipping local UDP wrappers.")
        return nil, nil
    }
    var kids []*child
    for _, u := range cfg.UDPForwards {
        llog := fmt.Sprintf("/var/log/socat-local-udpwrap-%d.log", u.UDPPublicPort)
        _ = os.MkdirAll(filepath.Dir(llog), 0o755)
        args := []string{
            "-T", "30",
            fmt.Sprintf("TCP4-LISTEN:%d,bind=127.0.0.1,reuseaddr,fork", u.WrapTCPPort),
            fmt.Sprintf("UDP:%s:%d", u.LocalHost, u.LocalUDPPort),
        }
        cmd := exec.Command("socat", args...)
        f, err := os.OpenFile(llog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
            return nil, err
        }
        cmd.Stdout = f
        cmd.Stderr = f
        if err := cmd.Start(); err != nil {
            _ = f.Close()
            return nil, err
        }
        kids = append(kids, &child{cmd: cmd, tag: fmt.Sprintf("local-socat-udpwrap-%d", u.UDPPublicPort)})
        logf("Local wrapper pid=%d : TCP 127.0.0.1:%d <-> UDP %s:%d (VPS UDP %d) log=%s", cmd.Process.Pid, u.WrapTCPPort, u.LocalHost, u.LocalUDPPort, u.UDPPublicPort, llog)
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
// The script starts UDP listeners and forwards them to local TCP wrappers via socat.
func buildRemoteScript(cfg *Config) string {
    var b strings.Builder
    b.WriteString("set -eu; ")
    // ensure predictable PATH for non-interactive shells
    b.WriteString("export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$PATH; ")
    b.WriteString(`SOCAT_BIN="$(command -v socat || true)"; `)
    b.WriteString(`if [ -z "$SOCAT_BIN" ]; then echo "ERROR: socat not found on VPS. PATH=$PATH" >&2; exit 1; fi; `)
    b.WriteString(`pids=""; cleanup(){ for p in $pids; do kill "$p" 2>/dev/null || true; done; }; trap cleanup INT TERM EXIT; `)
    if len(cfg.UDPForwards) == 0 {
        // Nothing to run; keep the SSH session alive
        b.WriteString("while true; do sleep 3600; done")
        return b.String()
    }
    for _, u := range cfg.UDPForwards {
        // best-effort kill any existing listener on the public port if fuser exists
        b.WriteString(fmt.Sprintf(`if command -v fuser >/dev/null 2>&1; then fuser -k %d/udp 2>/dev/null || true; fi; `, u.UDPPublicPort))
        b.WriteString(fmt.Sprintf(`"$SOCAT_BIN" -T 30 UDP-LISTEN:%d,reuseaddr,fork TCP4:127.0.0.1:%d >>/var/log/socat-udpwrap-%d.log 2>&1 & `,
            u.UDPPublicPort, u.WrapTCPPort, u.UDPPublicPort))
        b.WriteString(`pids="$pids $!"; `)
    }
    // watchdog loop: if any child dies, exit to force reconnect
    b.WriteString(`while true; do `)
    b.WriteStrin
