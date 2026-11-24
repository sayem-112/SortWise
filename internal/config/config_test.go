package config

import (
	"path/filepath"
	"testing"
)

func TestCarriedFlagsSurviveAnotherFolder(t *testing.T) {
	cfg, err := Parse([]string{"--port", "8790", "--data-dir", "library", "--no-open"})
	if err != nil {
		t.Fatal(err)
	}
	given := map[string]string{}
	for index := 0; index+1 < len(cfg.Carried); index += 2 {
		given[cfg.Carried[index]] = cfg.Carried[index+1]
	}
	if len(cfg.Carried) != 4 || given["--port"] != "8790" || !filepath.IsAbs(given["--data-dir"]) || filepath.Base(given["--data-dir"]) != "library" {
		t.Fatalf("carried %q", cfg.Carried)
	}
	plain, _ := Parse(nil)
	if len(plain.Carried) != 0 {
		t.Fatalf("defaults were carried: %q", plain.Carried)
	}
}
