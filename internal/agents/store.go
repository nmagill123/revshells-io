package agents

import (
	"fmt"
	"os"
	"path/filepath"
)

var allowedPlatforms = map[string]struct{}{
	"linux-amd64":  {},
	"linux-arm64":  {},
	"linux-386":    {},
	"darwin-amd64": {},
	"darwin-arm64": {},
}

// Platform names: linux-amd64, linux-arm64, linux-386, darwin-amd64, darwin-arm64
type Store struct {
	Dir string
}

func (s *Store) Get(platform string) ([]byte, error) {
	if s.Dir == "" {
		return nil, fmt.Errorf("no agents directory configured")
	}
	if _, ok := allowedPlatforms[platform]; !ok {
		return nil, fmt.Errorf("invalid agent platform %q", platform)
	}
	path := filepath.Join(s.Dir, platform)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agent binary %q not found (run: make agents)", platform)
	}
	return data, nil
}

func NormalizePlatform(osName, arch string) string {
	osName = toLower(osName)
	arch = toLower(arch)
	switch arch {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	default:
		return ""
	}
	switch osName {
	case "linux", "darwin":
		return osName + "-" + arch
	default:
		return "linux-" + arch
	}
}

func toLower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
