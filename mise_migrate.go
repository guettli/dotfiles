package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

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

// onlyToolsRemoved reports whether after equals before minus the named
// [tools] entries, so a line removed by text matching (e.g. inside a
// multi-line string) can never change anything else.
func onlyToolsRemoved(before, after []byte, removed []string) bool {
	var b, a map[string]any
	if toml.Unmarshal(before, &b) != nil || toml.Unmarshal(after, &a) != nil {
		return false
	}
	tools, ok := b["tools"].(map[string]any)
	if !ok {
		return false
	}
	for _, name := range removed {
		if tools[name] != "latest" {
			return false
		}
		delete(tools, name)
	}
	return reflect.DeepEqual(a, b)
}

// writeBackupCopy stores data below the backup directory like backupFile,
// but as a copy, so the original stays in place until it is replaced.
func writeBackupCopy(cacheDir, path string, data []byte) (string, error) {
	dst := backupPath(cacheDir, path)
	if _, err := os.Lstat(dst); err == nil {
		dst += "." + time.Now().Format("20060102-150405.000000000")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	return dst, os.WriteFile(dst, data, 0600)
}

func migrateLegacyMiseGlobals(homeDir, cacheDir, command string) error {
	marker := legacyMiseMarker(cacheDir)
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	path := filepath.Join(homeDir, ".config", "mise", "config.toml")
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var removed []string
	var cleaned []byte
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			// Managed elsewhere (stow, home-manager, ...): do not replace the
			// link with a file. Only tell.
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if _, r := removeLegacyMiseLines(data); len(r) > 0 {
				fmt.Printf("ℹ️  %s (a symlink) pins %s to \"latest\", an old dotfiles leftover. Remove these lines to use the pinned versions.\n",
					tildePath(path), strings.Join(r, ", "))
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var check map[string]any
		if toml.Unmarshal(data, &check) != nil {
			// mise cannot read it either; try again once it is fixed.
			return nil
		}
		cleaned, removed = removeLegacyMiseLines(data)
		if len(removed) > 0 && !onlyToolsRemoved(data, cleaned, removed) {
			return fmt.Errorf("%s: unexpected layout, not changed", tildePath(path))
		}
		if command == "apply" && len(removed) > 0 {
			tmp := path + ".dotfiles-new"
			if err := os.WriteFile(tmp, cleaned, info.Mode().Perm()); err != nil {
				return err
			}
			bak, err := writeBackupCopy(cacheDir, path, data)
			if err != nil {
				os.Remove(tmp)
				return err
			}
			if err := os.Rename(tmp, path); err != nil {
				os.Remove(tmp)
				return err
			}
			fmt.Printf("   Removed %s = \"latest\" from %s (left there by an old dotfiles version; the pinned versions apply now). Old file: %s\n",
				strings.Join(removed, ", "), tildePath(path), bak)
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
	if err := os.MkdirAll(filepath.Dir(marker), 0755); err != nil {
		return err
	}
	return os.WriteFile(marker, nil, 0644)
}
