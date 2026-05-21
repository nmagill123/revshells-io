package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreGetValidatesPlatform(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "linux-amd64"), []byte("agent"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := &Store{Dir: dir}
	if _, err := s.Get("linux-amd64"); err != nil {
		t.Fatalf("expected valid platform, got %v", err)
	}

	cases := []string{
		"../../etc/passwd",
		"linux/amd64",
		"linux-unknown",
		"",
	}
	for _, platform := range cases {
		if _, err := s.Get(platform); err == nil {
			t.Fatalf("expected platform %q to be rejected", platform)
		}
	}
}
