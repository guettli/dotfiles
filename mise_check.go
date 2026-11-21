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

// unknownMiseTools returns the base tools which the installed mise does not
// know. An old mise lacks newer registry entries (mise 2026.5 has no `micro`)
// and `mise install` then fails with "not found in mise tool registry", which
// tells a beginner nothing. Tools with an explicit backend ("aqua:...") need
// no registry entry.
func unknownMiseTools(tools []miseTool) ([]string, error) {
	cmd := exec.Command("mise", "registry")
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) > 0 {
			known[f[0]] = true
		}
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("`mise registry` printed nothing")
	}
	var unknown []string
	for _, t := range tools {
		if strings.Contains(t.Name, ":") || known[t.Name] {
			continue
		}
		// Not in the built-in registry, but maybe mapped by the user (a
		// [tool_alias] or a plugin): `mise tool` resolves those.
		check := exec.Command("mise", "tool", t.Name, "--backend")
		check.Dir = cmd.Dir
		if check.Run() == nil {
			continue
		}
		unknown = append(unknown, t.Name)
	}
	return unknown, nil
}

// miseVersion is the first word of `mise --version`, or "?".
func miseVersion() string {
	out, err := exec.Command("mise", "--version").Output()
	if err != nil {
		return "?"
	}
	if f := strings.Fields(string(out)); len(f) > 0 {
		return f[0]
	}
	return "?"
}

// misePath is a mise on PATH and the file it resolves to.
type misePath struct{ path, real string }

// misesOnPath lists every mise executable on PATH, first (the one used) first.
func misesOnPath() []misePath {
	var found []misePath
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "mise")
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Mode()&0111 == 0 {
			continue
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			real = p
		}
		if !seen[real] {
			seen[real] = true
			found = append(found, misePath{p, real})
		}
	}
	return found
}

// miseUpdateHint says how to update the mise found at path (as on PATH) which
// resolves to real, by how it was installed.
func miseUpdateHint(path, real string) string {
	_, aptErr := exec.LookPath("apt-get")
	_, dnfErr := exec.LookPath("dnf")
	switch {
	case strings.HasPrefix(real, "/nix/store/") &&
		(strings.HasPrefix(path, "/run/current-system/") || strings.HasPrefix(path, "/etc/profiles/")):
		return "update mise in your NixOS / home-manager configuration"
	case strings.HasPrefix(real, "/nix/store/"):
		return "nix profile upgrade mise   (or remove it: nix profile remove mise)"
	case strings.HasPrefix(path, "/snap/"):
		return "sudo snap refresh mise"
	case strings.Contains(real, "/Cellar/") || strings.HasPrefix(real, "/opt/homebrew/") || strings.HasPrefix(real, "/home/linuxbrew/"):
		return "brew upgrade mise"
	case strings.HasPrefix(real, "/usr/bin/") && aptErr == nil:
		return "sudo apt update && sudo apt install --only-upgrade mise"
	case strings.HasPrefix(real, "/usr/bin/") && dnfErr == nil:
		return "sudo dnf upgrade mise"
	case strings.HasPrefix(real, "/usr/") && !strings.HasPrefix(real, "/usr/local/"):
		return "update the mise package with your system's package manager"
	default:
		return "mise self-update"
	}
}

// miseTooOld explains an outdated mise. Empty when all tools are known (or
// mise could not be asked: then `mise install` reports the problem itself).
func miseTooOld(tools []miseTool) (problem, fix string) {
	unknown, err := unknownMiseTools(tools)
	if err != nil || len(unknown) == 0 {
		return "", ""
	}
	mises := misesOnPath()
	used := misePath{"mise", "mise"}
	if len(mises) > 0 {
		used = mises[0]
	}
	problem = fmt.Sprintf("mise %s (%s) is too old: it does not know %s", miseVersion(), used.path, strings.Join(unknown, ", "))
	fix = miseUpdateHint(used.path, used.real)
	if len(mises) > 1 {
		var all []string
		for _, m := range mises {
			all = append(all, m.real)
		}
		problem += fmt.Sprintf(". Several mise are on PATH (%s); the first one is used, so update or remove it",
			strings.Join(all, ", "))
	}
	return problem, fix + "\ndotfiles apply"
}
