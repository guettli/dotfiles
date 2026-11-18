package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runUninstall(args []string) int {
	fs := newFlagSet("uninstall")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 1
	}
	cacheDir := cacheDirFor(homeDir)

	cfg, err := loadUserConfig(homeDir, "")
	if err != nil {
		// Without a config, still remove everything dotfiles may have written.
		cfg = UserConfig{}
	}
	cfg.Shell = "zsh" // include the zsh files, whatever the shell is
	cfg.Orgs = mergeOrgs(cfg.Orgs, cachedOrgNames(cacheDir))
	configs := managedConfigs(homeDir, cfg)

	fmt.Println("This removes the files dotfiles wrote and restores the ones which were")
	fmt.Println("there before (from " + tildePath(backupRoot(cacheDir)) + ").")
	fmt.Println("Files you edited, your *.local files and ~/.gitconfig are kept.")
	if !*yes {
		if !isTerminal(os.Stdin) {
			fmt.Println("Run with --yes to confirm.")
			return 1
		}
		answer, err := prompt(bufio.NewReader(os.Stdin), os.Stdout, "Continue? [y/N]", "")
		if err != nil || !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Println("Aborted.")
			return 1
		}
	}

	problems := 0
	for _, c := range configs {
		if err := uninstallFile(c, cacheDir); err != nil {
			fmt.Printf("❌ %s: %v\n", tildePath(c.Destination), err)
			problems++
		}
	}
	// Remove empty directories of the cache, so a later apply starts fresh.
	removeEmptyDirs(cacheDir)

	fmt.Println()
	fmt.Println("The tools installed by mise are still there. To remove them: mise prune")
	fmt.Println("Open a new terminal to get the old shell back.")
	if problems > 0 {
		return 1
	}
	return 0
}

func uninstallFile(c Config, cacheDir string) error {
	cacheFile := cacheFileFor(c, cacheDir)
	cached, cacheErr := os.ReadFile(cacheFile)
	dest := c.Destination

	if info, err := os.Lstat(dest); err == nil {
		current, rerr := os.ReadFile(dest)
		rendered, _ := renderConfig(c, c.Data)
		managed := info.Mode().IsRegular() && rerr == nil &&
			((cacheErr == nil && bytes.Equal(current, cached)) || bytes.Equal(current, rendered))
		if !managed {
			if cacheErr == nil || bytes.HasPrefix(current, []byte(managedHeader)) {
				fmt.Printf("   kept %s (edited by hand)\n", tildePath(dest))
			}
			return nil
		}
		if err := os.Remove(dest); err != nil {
			return err
		}
		fmt.Printf("🗑  removed %s\n", tildePath(dest))
	} else if !os.IsNotExist(err) {
		return err
	}

	bak := backupPath(cacheDir, dest)
	if _, err := os.Lstat(bak); os.IsNotExist(err) && cacheErr == nil {
		// Versions before the backup directory saved the original next to
		// the file.
		bak = dest + ".bak"
	}
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		if _, err := os.Lstat(bak); err == nil {
			if err := os.Rename(bak, dest); err != nil {
				return err
			}
			fmt.Printf("↩️  restored %s\n", tildePath(dest))
		}
	}
	if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// cachedOrgNames finds the orgs dotfiles wrote ~/.gitconfig-org-<name> for.
func cachedOrgNames(cacheDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(cacheDir, "templates", "git", "gitconfig-org-*"))
	var names []string
	for _, m := range matches {
		names = append(names, strings.TrimPrefix(filepath.Base(m), "gitconfig-org-"))
	}
	return names
}

func mergeOrgs(orgs []OrgConfig, names []string) []OrgConfig {
	have := map[string]bool{}
	for _, o := range orgs {
		have[o.Name] = true
	}
	for _, n := range names {
		if !have[n] {
			orgs = append(orgs, OrgConfig{Name: n})
			have[n] = true
		}
	}
	return orgs
}

// removeEmptyDirs removes dir and its subdirectories, as far as they are empty.
func removeEmptyDirs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			removeEmptyDirs(filepath.Join(dir, e.Name()))
		}
	}
	_ = os.Remove(dir) // fails if not empty, which is fine
}
