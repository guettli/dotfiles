package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const configFileHeader = `# dotfiles config. Docs: https://github.com/guettli/dotfiles
# After editing, run: dotfiles apply
#
# shell = "bash" or "zsh"
# [[orgs]]: repos whose remote URL matches url use email for git commits.

`

// runInit writes ~/.config/dotfiles/config.toml. Values come from flags; on a
// terminal, missing ones are asked for (defaults from the existing git
// config).
func runInit(args []string, stdin io.Reader, out io.Writer) int {
	fs := newFlagSet("init")
	name := fs.String("name", "", "your full name (for git commits)")
	email := fs.String("email", "", "your personal email (for git commits)")
	orgURL := fs.String("org-url", "", "work organization, e.g. github.com/your-company (optional)")
	orgEmail := fs.String("org-email", "", "email for commits in the work organization's repos")
	shell := fs.String("shell", "", `"bash" or "zsh" (default: zsh if that is your login shell, else bash)`)
	profile := fs.String("profile", "", "profile from templates/profiles (optional)")
	force := fs.Bool("force", false, "overwrite an existing config")
	configPath := fs.String("config", "", "path to write (default ~/.config/dotfiles/config.toml)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(out, "❌ %v\n", err)
		return 1
	}
	path := *configPath
	if path == "" {
		path, _ = configPaths(homeDir)
	}
	if _, err := os.Stat(path); err == nil && !*force {
		fmt.Fprintf(out, "Config %s already exists. Edit it, or use --force to create a new one.\n", path)
		return 0
	}

	interactive := false
	if f, ok := stdin.(*os.File); ok {
		interactive = isTerminal(f)
	}
	cfg, err := buildConfig(bufio.NewReader(stdin), out, interactive, initValues{
		Name: *name, Email: *email, OrgURL: *orgURL, OrgEmail: *orgEmail,
		Shell: *shell, Profile: *profile,
	})
	if err != nil {
		fmt.Fprintf(out, "❌ %v\n", err)
		return 1
	}

	if err := writeUserConfig(path, cfg); err != nil {
		fmt.Fprintf(out, "❌ %v\n", err)
		return 1
	}
	// Validate what we wrote with the same code apply uses.
	if _, err := loadUserConfig(homeDir, path); err != nil {
		fmt.Fprintf(out, "❌ %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "✅ Wrote %s\n", path)
	return 0
}

type initValues struct {
	Name, Email, OrgURL, OrgEmail, Shell, Profile string
}

func gitConfigValue(key string) string {
	out, err := exec.Command("git", "config", "--global", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func buildConfig(in *bufio.Reader, out io.Writer, interactive bool, v initValues) (UserConfig, error) {
	var err error
	ask := func(current *string, question, def string) error {
		if *current != "" {
			return nil
		}
		if !interactive {
			*current = def
			return nil
		}
		*current, err = prompt(in, out, question, def)
		return err
	}

	if err := ask(&v.Name, "Your full name (for git commits)", gitConfigValue("user.name")); err != nil {
		return UserConfig{}, err
	}
	if err := ask(&v.Email, "Your personal email (for git commits)", gitConfigValue("user.email")); err != nil {
		return UserConfig{}, err
	}
	if v.Name == "" || v.Email == "" {
		return UserConfig{}, fmt.Errorf("name and email are needed: dotfiles init --name 'Your Name' --email you@example.com")
	}
	if interactive && v.OrgURL == "" {
		fmt.Fprintln(out, "\nUse a different email for work repositories? Enter the organization,")
		fmt.Fprintln(out, "for example github.com/your-company (leave empty to skip).")
		if v.OrgURL, err = prompt(in, out, "Work organization", ""); err != nil {
			return UserConfig{}, err
		}
	}
	v.OrgURL = strings.Trim(strings.TrimPrefix(strings.TrimPrefix(v.OrgURL, "https://"), "http://"), "/")
	cfg := UserConfig{Name: v.Name, PersonalEmail: v.Email, Shell: v.Shell, Profile: v.Profile}
	if v.OrgURL != "" {
		if err := ask(&v.OrgEmail, "Work email for repos in "+v.OrgURL, ""); err != nil {
			return UserConfig{}, err
		}
		if v.OrgEmail == "" {
			return UserConfig{}, fmt.Errorf("--org-email is needed with --org-url")
		}
		cfg.Orgs = []OrgConfig{{URL: v.OrgURL, Email: v.OrgEmail}}
	}
	if cfg.Shell == "" {
		cfg.Shell = defaultShell()
	}
	return cfg, nil
}

func writeUserConfig(path string, cfg UserConfig) error {
	var buf bytes.Buffer
	buf.WriteString(configFileHeader)
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes(), 0644)
}
