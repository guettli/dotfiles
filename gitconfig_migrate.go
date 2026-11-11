package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitConfigSource is the template deployed to ~/.config/git/config.
const gitConfigSource = "templates/git/config"

// legacyGitconfigSource is the cache key (template path) under which older
// versions recorded the ~/.gitconfig they wrote.
const legacyGitconfigSource = "templates/git/gitconfig"

// userGitconfigHeader starts the ~/.gitconfig which dotfiles hands to the
// user. The file must exist: `git config --global` (and so `gh auth
// setup-git`) writes to ~/.config/git/config when ~/.gitconfig is missing,
// which would modify the managed file again.
const userGitconfigHeader = "# Your own git settings. Not managed by dotfiles: it manages\n" +
	"# ~/.config/git/config, which git reads first; settings here win.\n"

// migrateLegacyGitconfig moves ~/.gitconfig out of dotfiles' management.
//
//   - ~/.gitconfig without the managed header: user-owned, left alone.
//   - managed and equal to the last applied version: replaced by the user
//     header (the settings now come from ~/.config/git/config).
//   - managed, last applied version plus appended lines (what `gh auth
//     setup-git` does): only the appended lines are kept.
//   - managed but edited otherwise, or no record of what was applied: error,
//     unless force, which moves the file to ~/.gitconfig.bak.
//
// Finally ~/.gitconfig is created with the user header if it is missing.
func migrateLegacyGitconfig(homeDir, cacheDir, command string, force bool) error {
	path := filepath.Join(homeDir, ".gitconfig")
	legacyCache := filepath.Join(cacheDir, legacyGitconfigSource)

	current, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ensureUserGitconfig(path, command)
	}
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(current, []byte(managedHeader)) {
		return nil // the user's own file
	}

	baseline, err := os.ReadFile(legacyCache)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var kept []byte
	var problem string
	switch {
	case baseline == nil:
		problem = fmt.Sprintf("there is no record of what dotfiles wrote (%s is missing)", legacyCache)
	case !bytes.HasPrefix(current, baseline):
		problem = fmt.Sprintf("its managed part was edited (compare with what dotfiles wrote: diff %s %s)", legacyCache, path)
	default:
		kept = bytes.TrimLeft(current[len(baseline):], "\n")
		if err := validateKept(kept); err != nil {
			problem = fmt.Sprintf("the lines added after the managed part can't stand alone: %v", err)
		}
	}
	if problem != "" {
		if !force {
			return fmt.Errorf("%s was managed by an older dotfiles version, which now manages ~/.config/git/config instead, "+
				"but %s. Leave only your own settings, as complete [sections], at the end of the file, "+
				"or use --force to move it to %s.bak", path, problem, path)
		}
		if command == "diff" {
			fmt.Printf("   would move %s to %s.bak (--force)\n", path, path)
			return nil
		}
		if err := backup(path); err != nil {
			return err
		}
		if err := removeIfExists(legacyCache); err != nil {
			return err
		}
		return ensureUserGitconfig(path, command)
	}

	content := []byte(userGitconfigHeader)
	if len(kept) > 0 {
		content = append(append(content, '\n'), kept...)
	}
	if command == "diff" {
		fmt.Printf("   would hand %s over to you (managed settings move to ~/.config/git/config, %d bytes of your own settings kept)\n", path, len(kept))
		return nil
	}
	mode := os.FileMode(0644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := writeFileAtomic(path, content, mode); err != nil {
		return err
	}
	if err := removeIfExists(legacyCache); err != nil {
		return err
	}
	fmt.Printf("✅ Handed %s over to you; dotfiles now manages ~/.config/git/config (%d bytes of your own settings kept)\n", path, len(kept))
	return nil
}

func ensureUserGitconfig(path, command string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if command == "diff" {
		fmt.Printf("   would create %s (your own git settings)\n", path)
		return nil
	}
	if err := writeFileAtomic(path, []byte(userGitconfigHeader), 0644); err != nil {
		return err
	}
	fmt.Printf("✅ Created %s for your own git settings\n", path)
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// validateKept checks that the lines kept from an old ~/.gitconfig are a
// valid git config on their own. git appends a key to the end of its
// section, so a key added to the last managed section (e.g. `git config
// --global mergetool.conflict-overview.keepBackup false`) lands after the
// managed part without its [section] header.
func validateKept(kept []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(kept))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if !strings.HasPrefix(line, "[") {
			return fmt.Errorf("%q is not inside a [section]", line)
		}
		break
	}
	if len(kept) == 0 {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil // no git, nothing reads the file
	}
	tmp, err := os.CreateTemp("", "dotfiles-gitconfig-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(kept); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if out, err := exec.Command("git", "config", "--file", tmp.Name(), "--list").CombinedOutput(); err != nil {
		return fmt.Errorf("git rejects them: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// checkXDGConfigHome: git reads $XDG_CONFIG_HOME/git/config, dotfiles
// writes ~/.config/git/config. If they differ, git would silently not see
// the managed settings (identity, org emails, hooks).
func checkXDGConfigHome(homeDir string) error {
	x := os.Getenv("XDG_CONFIG_HOME")
	if x == "" || filepath.Clean(x) == filepath.Join(homeDir, ".config") {
		return nil
	}
	return fmt.Errorf("XDG_CONFIG_HOME is %s: git would read $XDG_CONFIG_HOME/git/config, but dotfiles writes ~/.config/git/config. "+
		"Unset XDG_CONFIG_HOME or set it to ~/.config; the git config was not deployed", x)
}

// backup moves path to path.bak. It refuses to overwrite an existing .bak.
func backup(path string) error {
	bak := path + ".bak"
	if _, err := os.Lstat(bak); err == nil {
		return fmt.Errorf("cannot back up %s: %s already exists. Move it away first", path, bak)
	} else if !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("   Backing up %s to %s\n", path, bak)
	if err := os.Rename(path, bak); err != nil {
		return fmt.Errorf("back up %s: %w", path, err)
	}
	return nil
}

// adoptHint names where the settings of an unadoptable file should go.
func adoptHint(c Config) string {
	if c.Source == gitConfigSource {
		return "~/.gitconfig"
	}
	return "the dotfiles templates"
}

// writeFileAtomic writes via a temp file in the same directory and a
// rename, so a crash never leaves a truncated file.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	return os.Rename(name, path)
}
