package agents

import "testing"

func TestObjectKey(t *testing.T) {
	key, err := ObjectKey("agents/latest", "linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if key != "agents/latest/linux-amd64" {
		t.Fatalf("key = %q", key)
	}

	if _, err := ObjectKey("agents/latest", "../../etc/passwd"); err == nil {
		t.Fatal("expected traversal platform rejected")
	}
	if _, err := ObjectKey("agents/latest", "windows-amd64"); err == nil {
		t.Fatal("expected unknown platform rejected")
	}
}

func TestValidPlatform(t *testing.T) {
	if !ValidPlatform("linux-arm64") {
		t.Fatal("linux-arm64 should be valid")
	}
	if ValidPlatform("freebsd-amd64") {
		t.Fatal("freebsd-amd64 should be invalid")
	}
}
