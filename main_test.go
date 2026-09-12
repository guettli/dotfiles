package main

import (
	"reflect"
	"testing"
)

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
