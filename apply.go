package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"golang.org/x/term"
)

// version is set by goreleaser (-ldflags "-X main.version=...").
var version = ""

func getVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

const usage = `dotfiles: a good default shell environment.

Usage:
  dotfiles init       Create ~/.config/dotfiles/config.toml (asks for name and email)
  dotfiles diff       Show what apply would change
  dotfiles apply      Install the tools and write the config files
  dotfiles doctor     Check the setup and show how to fix problems
  dotfiles tips       Show what the new shell can do
  dotfiles update     Update dotfiles itself, then apply
  dotfiles uninstall  Remove the managed files and restore the ones from before
  dotfiles version    Print the version

Run "dotfiles <command> -h" for the options of a command.
Docs: https://github.com/guettli/dotfiles
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Print(usage)
		return 1
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "apply", "diff":
		return runApplyOrDiff(cmd, rest)
	case "init":
		return runInit(rest, os.Stdin, os.Stdout)
	case "doctor":
		return runDoctor(rest)
	case "tips":
		fmt.Print(tipsText)
		return 0
	case "update":
		return runUpdate(rest)
	case "uninstall":
		return runUninstall(rest)
	case "version", "--version":
		fmt.Println(getVersion())
		return 0
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		fmt.Print(usage)
		return 1
	}
}

// isTerminal reports whether f is an interactive terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("dotfiles "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func runApplyOrDiff(command string, args []string) int {
	fs := newFlagSet(command)
	force := fs.Bool("force", false, "overwrite files with local modifications (the old version is backed up)")
	configPath := fs.String("config", "", "path to config.toml (default ~/.config/dotfiles/config.toml)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Failed to get home directory: %v\n", err)
		return 1
	}

	// First run without a config: ask for it instead of failing.
	defaultPath, _ := configPaths(homeDir)
	if command == "apply" && *configPath == "" && isTerminal(os.Stdin) {
		if _, err := os.Stat(defaultPath); os.IsNotExist(err) {
			fmt.Printf("No config yet (%s). Let's create it.\n\n", defaultPath)
			if code := runInit(nil, os.Stdin, os.Stdout); code != 0 {
				return code
			}
			fmt.Println()
		}
	}

	if command == "apply" {
		fmt.Println("🚀 Applying dotfiles...")
	} else {
		fmt.Println("🔍 Diffing dotfiles...")
	}

	userConfig, err := loadUserConfig(homeDir, *configPath)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 1
	}

	if userConfig.Shell == "zsh" {
		// Antidote (zsh plugin manager) is not in the mise registry; install it
		// as a plain git clone into ~/.antidote, matching what .zshrc sources.
		if err := ensureAntidote(homeDir, command); err != nil {
			fmt.Printf("⚠️ Could not set up antidote: %v\n", err)
		}
	}

	cacheDir := cacheDirFor(homeDir)
	configs := managedConfigs(homeDir, userConfig)

	hasErrors := false

	gitConfigOK := true
	if err := checkXDGConfigHome(homeDir); err != nil {
		fmt.Printf("❌ %v\n", err)
		hasErrors = true
		gitConfigOK = false
	}

	for _, config := range configs {
		if config.Source == gitConfigSource && !gitConfigOK {
			continue
		}
		err := processConfig(config, config.Data, cacheDir, command, *force)
		if err != nil {
			fmt.Printf("❌ Error processing %s: %v\n", config.Destination, err)
			hasErrors = true
			if config.Source == gitConfigSource {
				gitConfigOK = false
			}
		}
		// Older versions managed ~/.gitconfig. Hand it over to the user
		// (keeping what tools appended) only once ~/.config/git/config is in
		// place, so a failure never leaves the user without the managed
		// settings.
		if config.Source == gitConfigSource && gitConfigOK {
			if err := migrateLegacyGitconfig(homeDir, cacheDir, command, *force); err != nil {
				fmt.Printf("❌ Error migrating ~/.gitconfig: %v\n", err)
				hasErrors = true
			}
		}
	}

	ensureCompletions(homeDir, userConfig.Shell, command)

	// Tools are declared in ~/.config/mise/conf.d/dotfiles.toml (deployed above).
	// mise decides what is missing and installs it — no tool bookkeeping here.
	// A failed install is an error of apply (the fleet's timer alerts on it):
	// with pinned versions, a pin without a download would otherwise fail
	// silently everywhere.
	if err := runMise(command); err != nil {
		if command == "apply" {
			fmt.Printf("❌ %v\n", err)
			hasErrors = true
		} else {
			fmt.Printf("⚠️ %v\n", err)
		}
	}

	if hasErrors {
		fmt.Println("⚠️ Finished with errors.")
		return 1
	}

	if command == "diff" {
		fmt.Println("🎉 Diff complete!")
		return 0
	}
	fmt.Println("🎉 Apply complete!")
	fmt.Println()
	fmt.Println("Open a new terminal to use it. Then:")
	fmt.Println("   dotfiles tips     what the new shell can do")
	fmt.Println("   dotfiles doctor   check the setup")
	if latest, err := latestVersion(); err == nil && isNewer(latest, getVersion()) {
		fmt.Printf("\n⬆️  dotfiles %s is available (you have %s). Run: dotfiles update\n", latest, getVersion())
	}
	return 0
}

func cacheDirFor(homeDir string) string {
	return filepath.Join(homeDir, ".local", "state", "dotfiles", "installed_cache")
}

// profileFile returns the embedded path of a profile's file, or "" if the
// profile has no such file.
func profileFile(profile, name string) string {
	if profile == "" {
		return ""
	}
	p := "templates/profiles/" + profile + "/" + name
	if _, err := templatesFS.ReadFile(p); err != nil {
		return ""
	}
	return p
}

func nonEmpty(paths ...string) []string {
	var out []string
	for _, p := range paths {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// managedConfigs lists every file dotfiles writes for this user config.
func managedConfigs(homeDir string, userConfig UserConfig) []Config {
	data := TemplateData{
		User:          os.Getenv("USER"),
		Home:          homeDir,
		Name:          userConfig.Name,
		PersonalEmail: userConfig.PersonalEmail,
		Orgs:          userConfig.Orgs,
		Shell:         userConfig.Shell,
		Profile:       userConfig.Profile,
	}
	p := userConfig.Profile
	configs := []Config{
		{
			// Base tools live in a file dotfiles owns; `mise install` (run below)
			// installs them. The user's own tools go in ~/.config/mise/config.toml
			// via `mise use -g`, which dotfiles never touches.
			Source:      "templates/mise/dotfiles.toml",
			Destination: filepath.Join(homeDir, ".config", "mise", "conf.d", "dotfiles.toml"),
		},
		{
			Source:      "templates/bash/.bashrc",
			Destination: filepath.Join(homeDir, ".bashrc"),
			LocalFile:   filepath.Join(homeDir, ".bashrc.local"),
		},
		{
			Source:      "templates/shell/common.sh",
			Destination: filepath.Join(homeDir, ".config", "shell", "common.sh"),
			LocalFile:   filepath.Join(homeDir, ".config", "shell", "local.sh"),
		},
		{
			Source:      "templates/shell/profile.sh",
			Extra:       nonEmpty(profileFile(p, "shell.sh")),
			Destination: filepath.Join(homeDir, ".config", "shell", "profile.sh"),
		},
		{
			Source:      "templates/starship/starship.toml",
			Destination: filepath.Join(homeDir, ".config", "starship.toml"),
		},
		{
			Source:      "templates/atuin/config.toml",
			Destination: filepath.Join(homeDir, ".config", "atuin", "config.toml"),
		},
		{
			Source:      "templates/direnv/direnv.toml",
			Extra:       nonEmpty(profileFile(p, "direnv.toml")),
			Destination: filepath.Join(homeDir, ".config", "direnv", "direnv.toml"),
		},
		{
			Source:      "templates/tealdeer/config.toml",
			Destination: filepath.Join(homeDir, ".config", "tealdeer", "config.toml"),
		},
		{
			Source:      "templates/tmux/tmux.conf",
			Destination: filepath.Join(homeDir, ".tmux.conf"),
		},
		{
			Source:      "templates/ripgrep/ripgreprc",
			Destination: filepath.Join(homeDir, ".config", "ripgrep", "ripgreprc"),
			NoHeader:    true,
		},
		{
			// Git reads ~/.config/git/config AND ~/.gitconfig (the latter wins).
			// dotfiles owns the first; ~/.gitconfig belongs to the user and to
			// tools like `gh auth setup-git`, so they never collide with apply.
			Source:      gitConfigSource,
			Destination: filepath.Join(homeDir, ".config", "git", "config"),
			// git itself writes --global settings here if ~/.gitconfig is
			// missing: such a file holds the user's settings, don't overwrite
			// it silently.
			NoAdopt: true,
		},
		{
			Source:      "templates/git/profile",
			Extra:       nonEmpty(profileFile(p, "gitconfig")),
			Destination: filepath.Join(homeDir, ".config", "git", "profile"),
		},
		{
			Source:      "templates/git/hooks/prepare-commit-msg",
			Destination: filepath.Join(homeDir, ".config", "git", "hooks", "prepare-commit-msg"),
			Mode:        0755,
		},
		{
			Source:      "templates/git/hooks/pre-commit",
			Destination: filepath.Join(homeDir, ".config", "git", "hooks", "pre-commit"),
			Mode:        0755,
		},
	}
	if userConfig.Shell == "zsh" {
		configs = append(configs,
			Config{
				Source:      "templates/zsh/.zshrc",
				Destination: filepath.Join(homeDir, ".zshrc"),
				LocalFile:   filepath.Join(homeDir, ".zshrc.local"),
			},
			Config{
				Source:      "templates/zsh/plugins.txt",
				Destination: filepath.Join(homeDir, ".config", "zsh", "plugins.txt"),
			},
		)
	}
	for i := range configs {
		configs[i].Data = data
	}
	for _, org := range userConfig.Orgs {
		configs = append(configs, Config{
			Source:      "templates/git/gitconfig-org",
			CacheKey:    "templates/git/gitconfig-org-" + org.Name,
			Destination: filepath.Join(homeDir, ".gitconfig-org-"+org.Name),
			Data:        struct{ Email string }{org.Email},
		})
	}
	return configs
}

// tildePath shortens a path below the home directory to ~/...
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

// prompt asks a question on out and reads one line from in. An empty answer
// returns def.
func prompt(in *bufio.Reader, out io.Writer, question, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(out, "%s: ", question)
	}
	line, err := in.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", fmt.Errorf("no answer (stdin closed). Pass the values as flags instead")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}
