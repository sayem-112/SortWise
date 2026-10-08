package xembed

import "testing"

func TestOnlyLinks(t *testing.T) {
	for text, want := range map[string]bool{"": true, " https://t.co/idagezHlAJ ": true, "https://t.co/a https://t.co/b": true, "read this https://t.co/a": false, "hello": false} {
		if got := OnlyLinks(text); got != want {
			t.Errorf("OnlyLinks(%q) = %v", text, got)
		}
	}
}

func TestRepairTextFixesDoubleEncodedUTF8(t *testing.T) {
	if got := repairText("Iâ€™ve been"); got != "I’ve been" {
		t.Fatalf("got %q", got)
	}
	if got := repairText("Déjà vu — fine"); got != "Déjà vu — fine" {
		t.Fatalf("correct text changed: %q", got)
	}
}
