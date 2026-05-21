package agent

import "testing"

func TestWithEnvOverridesReplacesExistingKeys(t *testing.T) {
	base := []string{"PWD=/tmp", "TERM=xterm-256color", "HOME=/root"}
	got := withEnvOverrides(base, map[string]string{
		"PWD":  "/srv/app",
		"TERM": "dumb",
	})

	countPWD := 0
	countTERM := 0
	for _, kv := range got {
		switch kv {
		case "PWD=/srv/app":
			countPWD++
		case "TERM=dumb":
			countTERM++
		}
		if kv == "PWD=/tmp" || kv == "TERM=xterm-256color" {
			t.Fatalf("stale env value kept: %s", kv)
		}
	}
	if countPWD != 1 || countTERM != 1 {
		t.Fatalf("expected one overridden PWD and TERM, got %v", got)
	}
}
