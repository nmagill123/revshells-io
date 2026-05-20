package payload

import (
	"bytes"
	"strings"
	"testing"
)

func TestBootstrapUsesLocalAgentURL(t *testing.T) {
	var buf bytes.Buffer
	_ = bootstrapSh.Execute(&buf, shimData{
		BaseURL:   "https://revshells.io",
		SessionID: "sess-uuid",
		Secret:    "secret",
	})
	out := buf.String()
	if strings.Contains(out, "GIT_BASE") || strings.Contains(out, "github.com") {
		t.Fatalf("bootstrap should not reference GitHub: %s", out)
	}
	if !strings.Contains(out, `url="$SERVER/$SESSION/agent/$PLATFORM"`) {
		t.Fatalf("missing local agent url")
	}
}

func TestBootstrapNoPTYUsesUnameFallback(t *testing.T) {
	var buf bytes.Buffer
	_ = bootstrapNoPTY.Execute(&buf, shimData{
		BaseURL:   "https://revshells.io",
		SessionID: "sess-uuid",
		Secret:    "secret",
	})
	out := buf.String()
	if !strings.Contains(out, `ARCH=$(uname -m 2>/dev/null || echo unknown)`) {
		t.Fatalf("nopty bootstrap lost uname fallback: %s", out)
	}
	if strings.Contains(out, `ARCH=$(uname -m 2>/dev/null | echo unknown)`) {
		t.Fatalf("nopty bootstrap still contains broken pipe fallback: %s", out)
	}
}
