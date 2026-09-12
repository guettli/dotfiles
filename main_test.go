package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeConfig writes body to <home>/.config/dotfiles/config.yaml and returns home.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "dotfiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestLoadUserConfig_KnownFields(t *testing.T) {
	home := writeConfig(t, "name: n\npersonal_email: e@x.com\nmise_tools:\n  - claude\n")
	cfg, err := loadUserConfig(home, "")
	if err != nil {
		t.Fatalf("valid config returned error: %v", err)
	}
	if !reflect.DeepEqual(cfg.MiseTools, []string{"claude"}) {
		t.Errorf("MiseTools = %v, want [claude]", cfg.MiseTools)
	}
}

func TestLoadUserConfig_UnknownFieldIsError(t *testing.T) {
	// `mise_tool` (typo, missing the trailing s) must fail loudly, not be ignored.
	home := writeConfig(t, "name: n\npersonal_email: e@x.com\nmise_tool:\n  - claude\n")
	if _, err := loadUserConfig(home, ""); err == nil {
		t.Fatal("expected an error for an unknown config field, got nil")
	}
}

func TestMergeTools(t *testing.T) {
	base := []string{"starship", "atuin", "direnv", "tmux"}
	tests := []struct {
		name  string
		base  []string
		extra []string
		want  []string
	}{
		{
			name:  "nil extra keeps base",
			base:  base,
			extra: nil,
			want:  base,
		},
		{
			name:  "appends extra after base",
			base:  base,
			extra: []string{"claude"},
			want:  []string{"starship", "atuin", "direnv", "tmux", "claude"},
		},
		{
			name:  "drops duplicates and blanks, trims space, preserves order",
			base:  []string{"starship", "atuin"},
			extra: []string{"atuin", " claude ", "", "node", "claude"},
			want:  []string{"starship", "atuin", "claude", "node"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeTools(tt.base, tt.extra)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeTools(%v, %v) = %v, want %v", tt.base, tt.extra, got, tt.want)
			}
		})
	}
}
