package operatorinput

import "testing"

func TestParseResize(t *testing.T) {
	c, r, ok := ParseResize([]byte(`{"type":"resize","cols":80,"rows":24}`))
	if !ok || c != 80 || r != 24 {
		t.Fatalf("parse failed: %d %d %v", c, r, ok)
	}
}

func TestStripResizeMessages(t *testing.T) {
	in := []byte(`{"type":"resize","cols":144,"rows":23}id`)
	out := StripResizeMessages(in)
	if string(out) != "id" {
		t.Fatalf("got %q", out)
	}
	concat := []byte(`{"type":"resize","cols":1,"rows":2}{"type":"resize","cols":3,"rows":4}ls`)
	out = StripResizeMessages(concat)
	if string(out) != "ls" {
		t.Fatalf("got %q", out)
	}
}
