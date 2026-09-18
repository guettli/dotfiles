package main

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/BurntSushi/toml"
)

//go:embed all:templates/*
var templatesFS embed.FS

type Config struct {
	Source      string
	Destination string
	Mode        os.FileMode
	NoHeader    bool
}

type OrgConfig struct {
	URL   string `toml:"url"`
	Email string `toml:"email"`
	Name  string `toml:"-"` // last path segment of URL, computed after load
	Host  string `toml:"-"` // host portion of URL (before first /), computed after load
}

type UserConfig struct {
	Name          string      `toml:"name"`
	PersonalEmail string      `toml:"personal_email"`
	Orgs          []OrgConfig `toml:"orgs"`
}

type TemplateData struct {
	User          string
	Home          string
	Name          string
	PersonalEmail string
	Orgs          []OrgConfig
}

// configPaths returns the current (TOML) config path and the legacy (YAML) path.
func configPaths(homeDir string) (tomlPath, yamlPath string) {
	base := filepath.Join(homeDir, ".config", "dotfiles")
	return filepath.Join(base, "config.toml"), filepath.Join(base, "config.yaml")
}

// legacyConfigError refuses to run on the old YAML config and shows how to
// migrate to the new TOML format.
func legacyConfigError(found, tomlPath string) error {
	return fmt.Errorf(`old YAML config detected: %s
dotfiles now uses TOML. Move your settings into %s:

    name           = "Your Name"
    personal_email = "you@example.com"

    # one [[orgs]] table per org (was a YAML list):
    [[orgs]]
    url   = "github.com/your-company"
    email = "you@your-company.com"

The `+"`mise_tools`"+` key was removed. Declare extra tools directly with mise
(this writes ~/.config/mise/config.toml, which dotfiles no longer touches):

    mise use -g <tool>

See config.example.toml. Delete the old config.yaml once migrated`, found, tomlPath)
}

// unknownFieldError reports keys present in the config file that the tool does
// not understand, so a typo (or a removed key) fails loudly instead of being
// silently ignored.
func unknownFieldError(path string, keys []toml.Key) error {
	names := make([]string, 0, len(keys))
	miseTools := false
	for _, k := range keys {
		names = append(names, k.String())
		if k.String() == "mise_tools" {
			miseTools = true
		}
	}
	msg := fmt.Sprintf("unknown field(s) in %s: %s", path, strings.Join(names, ", "))
	if miseTools {
		msg += "\n`mise_tools` was removed — install extra tools directly: mise use -g <tool>"
	}
	return fmt.Errorf("%s", msg)
}

func loadUserConfig(homeDir string, configPath string) (UserConfig, error) {
	tomlPath, yamlPath := configPaths(homeDir)
	path := configPath
	if path == "" {
		path = tomlPath
	}

	// Refuse the old format with a migration hint, whether it is the default
	// legacy file or an explicit --config pointing at a YAML file.
	if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
		return UserConfig{}, legacyConfigError(path, tomlPath)
	}
	if path == tomlPath {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if _, yErr := os.Stat(yamlPath); yErr == nil {
				return UserConfig{}, legacyConfigError(yamlPath, tomlPath)
			}
		}
	}

	var cfg UserConfig
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if os.IsNotExist(err) {
			return UserConfig{}, fmt.Errorf("could not read %s: %w\nCreate it from config.example.toml in the dotfiles repo, or pass --config <path>", path, err)
		}
		return UserConfig{}, fmt.Errorf("could not parse %s: %w", path, err)
	}
	// KnownFields-style strictness: any key in the file that did not map onto a
	// struct field is a mistake (a typo, or a key removed in a format change).
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return UserConfig{}, unknownFieldError(path, undecoded)
	}

	for i, org := range cfg.Orgs {
		parts := strings.Split(org.URL, "/")
		cfg.Orgs[i].Name = parts[len(parts)-1]
		cfg.Orgs[i].Host = parts[0]
	}
	return cfg, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: dotfiles [apply|diff] [--force] [--config <path>]")
		os.Exit(1)
	}

	command := ""
	force := false
	configPath := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--force":
			force = true
		case "--config":
			i++
			if i >= len(args) {
				fmt.Println("--config requires a path argument")
				os.Exit(1)
			}
			configPath = args[i]
		default:
			if command == "" {
				command = args[i]
			}
		}
	}

	if command != "apply" && command != "diff" {
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Usage: dotfiles [apply|diff] [--force] [--config <path>]")
		os.Exit(1)
	}

	if command == "apply" {
		fmt.Println("🚀 Applying dotfiles...")
	} else {
		fmt.Println("🔍 Diffing dotfiles...")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	userConfig, err := loadUserConfig(homeDir, configPath)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}

	// Antidote (zsh plugin manager) is not in the mise registry; install it as a
	// plain git clone into ~/.antidote, matching what .zshrc sources.
	if err := ensureAntidote(homeDir, command); err != nil {
		fmt.Printf("⚠️ Could not set up antidote: %v\n", err)
	}

	cacheDir := filepath.Join(homeDir, ".local", "state", "dotfiles", "installed_cache")

	templateData := TemplateData{
		User:          os.Getenv("USER"),
		Home:          homeDir,
		Name:          userConfig.Name,
		PersonalEmail: userConfig.PersonalEmail,
		Orgs:          userConfig.Orgs,
	}

	configs := []Config{
		{
			// Base tools live in a file dotfiles owns; `mise install` (run below)
			// installs them. The user's own tools go in ~/.config/mise/config.toml
			// via `mise use -g`, which dotfiles never touches.
			Source:      "templates/mise/dotfiles.toml",
			Destination: filepath.Join(homeDir, ".config", "mise", "conf.d", "dotfiles.toml"),
		},
		{
			Source:      "templates/zsh/.zshrc",
			Destination: filepath.Join(homeDir, ".zshrc"),
		},
		{
			Source:      "templates/zsh/plugins.txt",
			Destination: filepath.Join(homeDir, ".config", "zsh", "plugins.txt"),
		},
		{
			Source:      "templates/bash/.bashrc",
			Destination: filepath.Join(homeDir, ".bashrc"),
		},
		{
			Source:      "templates/shell/common.sh",
			Destination: filepath.Join(homeDir, ".config", "shell", "common.sh"),
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
			Destination: filepath.Join(homeDir, ".config", "direnv", "direnv.toml"),
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
			Source:      "templates/git/gitconfig",
			Destination: filepath.Join(homeDir, ".gitconfig"),
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

	hasErrors := false

	for _, config := range configs {
		err := processConfig(config, templateData, cacheDir, command, force)
		if err != nil {
			fmt.Printf("❌ Error processing %s: %v\n", config.Source, err)
			hasErrors = true
		}
	}

	for _, org := range userConfig.Orgs {
		orgConfig := Config{
			Source:      "templates/git/gitconfig-org",
			Destination: filepath.Join(homeDir, ".gitconfig-org-"+org.Name),
		}
		err := processConfig(orgConfig, struct{ Email string }{org.Email}, cacheDir, command, force)
		if err != nil {
			fmt.Printf("❌ Error processing org config for %s: %v\n", org.URL, err)
			hasErrors = true
		}
	}

	// Tools are declared in ~/.config/mise/conf.d/dotfiles.toml (deployed above).
	// mise decides what is missing and installs it — no tool bookkeeping here.
	if err := runMise(command); err != nil {
		fmt.Printf("⚠️ %v\n", err)
	}

	if hasErrors {
		fmt.Println("⚠️ Finished with errors.")
		os.Exit(1)
	}

	if command == "apply" {
		fmt.Println("🎉 Apply complete!")
	} else {
		fmt.Println("🎉 Diff complete!")
	}
}

// runMise installs the tools declared in mise's config. On diff it only reports
// what is missing; on apply it runs `mise install` (idempotent).
func runMise(command string) error {
	if _, err := exec.LookPath("mise"); err != nil {
		return fmt.Errorf("mise not found on PATH — install it first: https://mise.jdx.dev/getting-started.html")
	}
	if command == "diff" {
		out, err := exec.Command("mise", "ls", "--missing").Output()
		if err != nil {
			return fmt.Errorf("could not list mise tools: %w", err)
		}
		if len(bytes.TrimSpace(out)) == 0 {
			fmt.Println("   All mise tools are already installed.")
			return nil
		}
		fmt.Println("\n--- mise tools to install (via `mise install`) ---")
		fmt.Print(string(out))
		fmt.Println()
		return nil
	}
	// command == "apply"
	fmt.Println("📦 Installing mise tools...")
	cmd := exec.Command("mise", "install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mise install failed: %w", err)
	}
	fmt.Println("✅ mise tools installed!")
	return nil
}

// ensureAntidote makes sure the antidote zsh plugin manager is available at
// ~/.antidote (it is not in the mise registry, so we clone it directly).
func ensureAntidote(homeDir string, command string) error {
	dest := filepath.Join(homeDir, ".antidote")
	if _, err := os.Stat(dest); err == nil {
		if command == "diff" {
			fmt.Println("   antidote already installed at ~/.antidote")
		}
		return nil
	}
	if command == "diff" {
		fmt.Println("+ antidote (git clone https://github.com/mattmc3/antidote ~/.antidote)")
		return nil
	}
	// command == "apply"
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH, cannot install antidote: %w", err)
	}
	fmt.Println("📦 Installing antidote via git clone...")
	cmd := exec.Command("git", "clone", "--depth=1", "https://github.com/mattmc3/antidote.git", dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone antidote: %w", err)
	}
	fmt.Println("✅ antidote installed at ~/.antidote")
	return nil
}

const managedHeader = "# DO NOT EDIT\n" +
	"# Managed by dotfiles: https://github.com/guettli/dotfiles\n\n"

// addManagedHeader puts the "do not edit" comment at the top of the file, but
// below a shebang line. A shebang only counts if it is the very first line, so
// nothing may go above it.
func addManagedHeader(content []byte) []byte {
	shebang := []byte(nil)
	body := content
	if bytes.HasPrefix(content, []byte("#!")) {
		end := bytes.IndexByte(content, '\n')
		if end < 0 {
			// A shebang without a trailing newline. Add one, so the header does
			// not end up on the same line.
			shebang, body = append(content[:len(content):len(content)], '\n'), nil
		} else {
			shebang, body = content[:end+1], content[end+1:]
		}
	}

	out := make([]byte, 0, len(shebang)+len(managedHeader)+len(body))
	out = append(out, shebang...)
	out = append(out, managedHeader...)
	out = append(out, body...)
	return out
}

func processConfig(config Config, data any, cacheDir string, command string, force bool) error {
	// 1. Render the template
	contentBytes, err := templatesFS.ReadFile(config.Source)
	if err != nil {
		return fmt.Errorf("failed to read embedded file: %w", err)
	}

	tmpl, err := template.New(filepath.Base(config.Source)).Parse(string(contentBytes))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	var renderedBuffer bytes.Buffer
	if err := tmpl.Execute(&renderedBuffer, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	renderedContent := renderedBuffer.Bytes()
	if !config.NoHeader {
		renderedContent = addManagedHeader(renderedContent)
	}

	// Ensure destination directory exists
	destDir := filepath.Dir(config.Destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", destDir, err)
	}

	cacheFile := filepath.Join(cacheDir, config.Source)

	// Create cache dir structure
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if command == "diff" {
		return runDiffCommand(config.Destination, renderedContent, "Current File", "Template Render")
	}

	// command == "apply"

	newFile := config.Destination + ".new"

	// 2. Check for local modifications if destination exists
	if fileInfo, err := os.Stat(config.Destination); err == nil {
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink. We'll replace it.
			os.Remove(config.Destination)
		} else if destBytes, err := os.ReadFile(config.Destination); err == nil && !bytes.Equal(destBytes, renderedContent) {
			// Destination differs from what we'd write. Only a problem if it also
			// diverges from the last applied baseline — otherwise it's just a
			// normal template update (or the cache is stale) and is safe to overwrite.
			if cacheBytes, err := os.ReadFile(cacheFile); err == nil {
				if !bytes.Equal(cacheBytes, destBytes) {
					fmt.Printf("\n⚠️ LOCAL MODIFICATION DETECTED: %s\n", config.Destination)
					if err := os.WriteFile(newFile, renderedContent, 0644); err != nil {
						return fmt.Errorf("failed to write %s for diffing: %w", newFile, err)
					}
					fmt.Printf("   To see diff execute:\n   diff %s %s\n", config.Destination, newFile)
					if !force {
						return fmt.Errorf("local modifications detected in %s. Please sync them to templates or revert them (or use --force to overwrite)", config.Destination)
					}
					fmt.Println("   Continuing anyway because --force was used.")
				}
			} else if os.IsNotExist(err) {
				// Destination exists but no cache. First run for this tool? Back it up.
				backupPath := config.Destination + ".bak"
				fmt.Printf("   Backing up unmanaged file %s to %s\n", config.Destination, backupPath)
				os.Rename(config.Destination, backupPath)
			}
		}
	}

	mode := config.Mode
	if mode == 0 {
		mode = 0644
	}

	// 3. Write destination file (skip if already up to date)
	if destBytes, err := os.ReadFile(config.Destination); err == nil && bytes.Equal(destBytes, renderedContent) {
		fmt.Printf("   unchanged %s\n", config.Destination)
		os.Remove(newFile)
		// The destination already matches; make sure the cache baseline agrees too,
		// so a stale cache doesn't cause a false "local modification" next time.
		if err := os.WriteFile(cacheFile, renderedContent, 0644); err != nil {
			return fmt.Errorf("failed to update cache: %w", err)
		}
		return nil
	}

	err = os.WriteFile(config.Destination, renderedContent, mode)
	if err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}
	os.Remove(newFile)

	// 4. Update cache baseline
	err = os.WriteFile(cacheFile, renderedContent, 0644)
	if err != nil {
		return fmt.Errorf("failed to update cache: %w", err)
	}

	fmt.Printf("✅ Applied %s\n", config.Destination)
	return nil
}

func runDiffCommand(destPath string, compareContent []byte, labelA string, labelB string) error {
	// Create a temporary file for the compareContent
	tmpFile, err := os.CreateTemp("", "dotfile-compare-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for diff: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(compareContent); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	tmpFile.Close()

	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		fmt.Printf("   File %s does not exist yet. It will be created.\n", destPath)
		return nil
	}

	cmd := exec.Command("diff", "-u", "--label", labelA, "--label", labelB, destPath, tmpFile.Name())

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		// diff exits with 1 if there are differences
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			fmt.Printf("\n--- Diff for %s ---\n", destPath)
			fmt.Println(out.String())
			return nil
		}
		return fmt.Errorf("diff command failed: %w", err)
	}

	fmt.Printf("   %s is up to date.\n", destPath)
	return nil
}
