package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Versions before #10 installed the base tools with
// `mise use -g starship atuin direnv tmux`, which wrote `<tool> = "latest"`
// into the user's ~/.config/mise/config.toml. That file wins over
// ~/.config/mise/conf.d/dotfiles.toml, so those four tools never follow the
// pinned versions. Remove exactly these leftovers, once: a user who later
// adds one back on purpose keeps it.
var legacyMiseLine = regexp.MustCompile(`^\s*(atuin|direnv|starship|tmux)\s*=\s*["']latest["']\s*(#.*)?$`)

// legacyMiseMarker records that the cleanup ran.
func legacyMiseMarker(cacheDir string) string {
	return filepath.Join(filepath.Dir(cacheDir), "migrated-mise-globals")
}

// removeLegacyMiseLines returns data without the leftover lines of the
// [tools] table, and the names of the removed tools.
func removeLegacyMiseLines(data []byte) ([]byte, []string) {
	var out bytes.Buffer
	var removed []string
	inTools := false
	for _, line := range strings.SplitAfter(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			header, _, _ := strings.Cut(trimmed, "#")
			inTools = strings.TrimSpace(header) == "[tools]"
		}
		if inTools {
			if m := legacyMiseLine.FindStringSubmatch(strings.TrimRight(line, "\r\n")); m != nil {
				removed = append(removed, m[1])
				continue
			}
		}
		out.WriteString(line)
	}
	return out.Bytes(), removed
}

func migrateLegacyMiseGlobals(homeDir, cacheDir, command string) error {
	marker := legacyMiseMarker(cacheDir)
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	path := filepath.Join(homeDir, ".config", "mise", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var removed []string
	var cleaned []byte
	if err == nil {
		var check map[string]any
		if toml.Unmarshal(data, &check) == nil {
			cleaned, removed = removeLegacyMiseLines(data)
			if len(removed) > 0 && toml.Unmarshal(cleaned, &check) != nil {
				removed = nil
			}
		}
	}
	if command != "apply" {
		if len(removed) > 0 {
			fmt.Printf("\n--- %s ---\n", tildePath(path))
			fmt.Printf("Leftovers of an old dotfiles version would be removed (the pinned versions apply then): %s = \"latest\"\n",
				strings.Join(removed, ", "))
		}
		return nil
	}
	if len(removed) > 0 {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		tmp := path + ".dotfiles-new"
		if err := os.WriteFile(tmp, cleaned, info.Mode().Perm()); err != nil {
			return err
		}
		bak, err := backupFile(cacheDir, path)
		if err != nil {
			os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Rename(bak, path)
			return err
		}
		fmt.Printf("   Removed %s = \"latest\" from %s (left there by an old dotfiles version; the pinned versions apply now). Old file: %s\n",
			strings.Join(removed, ", "), tildePath(path), bak)
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0755); err != nil {
		return err
	}
	return os.WriteFile(marker, nil, 0644)
}
