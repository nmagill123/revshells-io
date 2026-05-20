package agents

import "testing"

func TestReleaseDownloadBase(t *testing.T) {
	cfg := GitSyncConfig{
		Repo: "https://github.com/nmagill123/revshells-io",
		Tag:  "v0.1.0",
	}
	want := "https://github.com/nmagill123/revshells-io/releases/download/v0.1.0"
	if got := ReleaseDownloadBase(cfg); got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	cfg.Tag = ""
	wantLatest := "https://github.com/nmagill123/revshells-io/releases/latest/download"
	if got := ReleaseDownloadBase(cfg); got != wantLatest {
		t.Fatalf("latest: got %q want %q", got, wantLatest)
	}
}
