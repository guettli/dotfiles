package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

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
	switch {
	case baseline != nil && bytes.HasPrefix(current, baseline):
		kept = bytes.TrimLeft(current[len(baseline):], "\n")
	case !force:
		return fmt.Errorf("%s is managed by an older dotfiles version but was edited (or its applied version is unknown). "+
			"dotfiles now manages ~/.config/git/config instead. Move your own settings to the end of %s, "+
			"or use --force to move it to %s.bak", path, path, path)
	default:
		if command == "diff" {
			fmt.Printf("   would move %s to %s.bak (--force)\n", path, path)
			return nil
		}
		fmt.Printf("   --force: moving %s to %s.bak\n", path, path)
		if err := os.Rename(path, path+".bak"); err != nil {
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
	if err := os.WriteFile(path, content, 0644); err != nil {
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
	if err := os.WriteFile(path, []byte(userGitconfigHeader), 0644); err != nil {
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
