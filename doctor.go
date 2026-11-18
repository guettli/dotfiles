package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type doctor struct {
	problems int
}

func (d *doctor) ok(format string, a ...any) {
	fmt.Printf("✅ "+format+"\n", a...)
}

// warn reports a problem and how to fix it.
func (d *doctor) warn(fix string, format string, a ...any) {
	d.problems++
	fmt.Printf("⚠️  "+format+"\n", a...)
	if fix != "" {
		for _, line := range strings.Split(fix, "\n") {
			fmt.Printf("     %s\n", line)
		}
	}
}

func runDoctor(args []string) int {
	fs := newFlagSet("doctor")
	configPath := fs.String("config", "", "path to config.toml (default ~/.config/dotfiles/config.toml)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	d := &doctor{}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 1
	}

	// Version
	current := getVersion()
	if latest, err := latestVersion(); err != nil {
		d.ok("dotfiles %s (could not check for updates: %v)", current, err)
	} else if isNewer(latest, current) {
		d.warn("dotfiles update", "dotfiles %s is installed, %s is available", current, latest)
	} else {
		d.ok("dotfiles %s", current)
	}

	// Config
	cfg, err := loadUserConfig(homeDir, *configPath)
	if err != nil {
		d.warn("dotfiles init", "%v", err)
		return finishDoctor(d)
	}
	d.ok("config: shell=%s, name=%q, email=%q", cfg.Shell, cfg.Name, cfg.PersonalEmail)

	checkShell(d, cfg)
	checkMise(d)
	checkManagedFiles(d, homeDir, cfg)
	checkGit(d, homeDir, cfg)
	checkClipboard(d)

	return finishDoctor(d)
}

func finishDoctor(d *doctor) int {
	fmt.Println()
	if d.problems == 0 {
		fmt.Println("🎉 Everything looks good.")
		return 0
	}
	fmt.Printf("Found %d problem(s). Run the commands shown above to fix them.\n", d.problems)
	return 1
}

func checkShell(d *doctor, cfg UserConfig) {
	login := loginShell()
	if filepath.Base(login) == cfg.Shell {
		d.ok("login shell is %s", login)
	} else {
		shellPath, err := exec.LookPath(cfg.Shell)
		if err != nil {
			d.warn(installHint(cfg.Shell), "%s is not installed", cfg.Shell)
		} else {
			d.warn("chsh -s "+shellPath+"\nor set shell = \""+filepath.Base(login)+"\" in ~/.config/dotfiles/config.toml",
				"your login shell is %q, but the config says shell = %q", login, cfg.Shell)
		}
	}

	if cfg.Shell == "bash" {
		if !fileExists("/usr/share/bash-completion/bash_completion") && !fileExists("/etc/bash_completion") &&
			!fileExists("/opt/homebrew/etc/profile.d/bash_completion.sh") {
			d.warn(installHint("bash-completion"), "bash-completion is not installed: Tab completes only file names")
		}
		// Login shells (ssh, macOS Terminal) read ~/.bash_profile instead of
		// ~/.bashrc. It must load ~/.bashrc, or the setup is missing there.
		home, _ := os.UserHomeDir()
		for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
			b, err := os.ReadFile(filepath.Join(home, name))
			if err != nil {
				continue
			}
			if !bytes.Contains(b, []byte(".bashrc")) {
				d.warn("echo '[ -f ~/.bashrc ] && . ~/.bashrc' >> ~/"+name,
					"~/%s does not load ~/.bashrc, so login shells (ssh) miss the setup", name)
			}
			break // bash reads only the first one that exists
		}
	}
}

func installHint(pkg string) string {
	if runtime.GOOS == "darwin" {
		return "brew install " + pkg
	}
	switch {
	case fileExists("/usr/bin/apt-get"):
		return "sudo apt-get install " + pkg
	case fileExists("/usr/bin/dnf"):
		return "sudo dnf install " + pkg
	case fileExists("/usr/bin/pacman"):
		return "sudo pacman -S " + pkg
	}
	return "install " + pkg + " with your package manager"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func checkMise(d *doctor) {
	if _, err := exec.LookPath("mise"); err != nil {
		d.warn("curl https://mise.run | sh\ndotfiles apply", "mise is not installed")
		return
	}
	tools, err := baseMiseTools()
	if err != nil {
		d.warn("", "could not read the tool list: %v", err)
		return
	}
	var missing []string
	for _, tool := range tools {
		if !miseToolInstalled(tool) {
			missing = append(missing, tool.String())
		}
	}
	if len(missing) > 0 {
		d.warn("dotfiles apply", "tools missing or outdated: %s", strings.Join(missing, ", "))
	} else {
		d.ok("all %d tools installed", len(tools))
	}
}

// checkManagedFiles finds managed files which are missing, edited, or out of
// date compared to this version's templates.
func checkManagedFiles(d *doctor, homeDir string, cfg UserConfig) {
	cacheDir := cacheDirFor(homeDir)
	var missing, edited, outdated []string
	for _, c := range managedConfigs(homeDir, cfg) {
		dest, err := os.ReadFile(c.Destination)
		if err != nil {
			missing = append(missing, tildePath(c.Destination))
			continue
		}
		rendered, err := renderConfig(c, c.Data)
		if err != nil || bytes.Equal(dest, rendered) {
			continue
		}
		if cached, err := os.ReadFile(cacheFileFor(c, cacheDir)); err == nil && bytes.Equal(cached, dest) {
			outdated = append(outdated, tildePath(c.Destination))
		} else {
			edited = append(edited, tildePath(c.Destination))
		}
	}
	switch {
	case len(missing) > 0:
		d.warn("dotfiles apply", "not installed yet: %s", strings.Join(missing, ", "))
	case len(outdated) > 0:
		d.warn("dotfiles apply", "out of date: %s", strings.Join(outdated, ", "))
	}
	if len(edited) > 0 {
		d.warn("dotfiles diff   (shows the changes)\nMove your own settings to ~/.bashrc.local, ~/.zshrc.local or ~/.config/shell/local.sh",
			"edited by hand (apply will stop): %s", strings.Join(edited, ", "))
	}
	if len(missing)+len(outdated)+len(edited) == 0 {
		d.ok("all config files are up to date")
	}
}

func checkGit(d *doctor, homeDir string, cfg UserConfig) {
	if _, err := exec.LookPath("git"); err != nil {
		d.warn(installHint("git"), "git is not installed")
		return
	}
	if err := checkXDGConfigHome(homeDir); err != nil {
		d.warn("", "%v", err)
	}
	// ~/.gitconfig wins over the managed ~/.config/git/config, so an email
	// there disables the per-org emails.
	userFile := filepath.Join(homeDir, ".gitconfig")
	out, err := exec.Command("git", "config", "--file", userFile, "user.email").Output()
	email := strings.TrimSpace(string(out))
	if err == nil && (len(cfg.Orgs) > 0 || email != cfg.PersonalEmail) {
		d.warn("git config --file ~/.gitconfig --unset user.email",
			"~/.gitconfig sets user.email = %s, which overrides the emails from the dotfiles config", email)
	} else {
		d.ok("git identity comes from the dotfiles config")
	}
}

func checkClipboard(d *doctor) {
	if runtime.GOOS != "linux" {
		return
	}
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		if _, err := exec.LookPath("wl-copy"); err != nil {
			d.warn(installHint("wl-clipboard"), "toClipboard/fromClipboard need wl-clipboard")
		}
	case os.Getenv("DISPLAY") != "":
		if _, err := exec.LookPath("xclip"); err != nil {
			d.warn(installHint("xclip"), "toClipboard/fromClipboard need xclip")
		}
	}
}
