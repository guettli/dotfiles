package main

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
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
	URL   string `yaml:"url"`
	Email string `yaml:"email"`
	Name  string `yaml:"-"` // last path segment of URL, computed after load
	Host  string `yaml:"-"` // host portion of URL (before first /), computed after load
}

type UserConfig struct {
	Name          string      `yaml:"name"`
	PersonalEmail string      `yaml:"personal_email"`
	Orgs          []OrgConfig `yaml:"orgs"`
}

type TemplateData struct {
	User          string
	Home          string
	Name          string
	PersonalEmail string
	Orgs          []OrgConfig
}

func loadUserConfig(homeDir string, configPath string) (UserConfig, error) {
	defaultPath := filepath.Join(homeDir, ".config", "dotfiles", "config.yaml")
	path := configPath
	if path == "" {
		path = defaultPath
	}
	var data []byte
	var err error
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		resp, err := http.Get(path)
		if err != nil {
			return UserConfig{}, fmt.Errorf("could not fetch %s: %w", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return UserConfig{}, fmt.Errorf("could not fetch %s: HTTP %d", path, resp.StatusCode)
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return UserConfig{}, fmt.Errorf("could not read response from %s: %w", path, err)
		}
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			if mkdirErr := os.MkdirAll(filepath.Dir(path), 0755); mkdirErr != nil {
				return UserConfig{}, fmt.Errorf("could not read %s: %w\ncould not create %s: %v", path, err, filepath.Dir(path), mkdirErr)
			}
			return UserConfig{}, fmt.Errorf("could not read %s: %w\nCreate it from config.example.yaml in the dotfiles repo, or pass --config <path>", path, err)
		}
	}
	if configPath != "" && configPath != defaultPath {
		if err := os.MkdirAll(filepath.Dir(defaultPath), 0755); err != nil {
			return UserConfig{}, fmt.Errorf("could not create config directory: %w", err)
		}
		if err := os.WriteFile(defaultPath, data, 0644); err != nil {
			return UserConfig{}, fmt.Errorf("could not copy config to %s: %w", defaultPath, err)
		}
		fmt.Printf("   Copied config to %s\n", defaultPath)
	}
	var cfg UserConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return UserConfig{}, fmt.Errorf("could not parse %s: %w", path, err)
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
		fmt.Println("Usage: dotfiles [apply|diff] [--force]")
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

	// Tools installed globally via mise (mise itself is the installer, so it is
	// not listed here). Everything below resolves from the mise registry.
	requiredTools := []string{
		"starship",
		"atuin",
		"direnv",
		"tmux",
	}

	if command == "apply" {
		fmt.Println("🚀 Applying dotfiles...")
	} else {
		fmt.Println("🔍 Diffing dotfiles...")
	}

	missingTools, err := getMissingTools(requiredTools)
	if err != nil {
		fmt.Printf("⚠️ Could not check mise tools (is mise installed?): %v\n", err)
	} else {
		if len(missingTools) > 0 {
			if command == "diff" {
				fmt.Println("\n--- Dependencies to Install ---")
				for _, tool := range missingTools {
					fmt.Printf("+ %s\n", tool)
				}
				fmt.Println()
			} else if command == "apply" {
				fmt.Println("📦 Installing missing dependencies via mise...")
				args := append([]string{"use", "-g"}, missingTools...)
				cmd := exec.Command("mise", args...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("❌ Failed to install dependencies: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("✅ Dependencies installed!")
				fmt.Println()
			}
		} else {
			if command == "diff" {
				fmt.Println("   All mise dependencies are already installed.")
			}
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Failed to get home directory: %v\n", err)
		os.Exit(1)
	}

	// Antidote (zsh plugin manager) is not in the mise registry; install it as a
	// plain git clone into ~/.antidote, matching what .zshrc sources.
	if err := ensureAntidote(homeDir, command); err != nil {
		fmt.Printf("⚠️ Could not set up antidote: %v\n", err)
	}

	userConfig, err := loadUserConfig(homeDir, configPath)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
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

// getMissingTools returns the subset of tools that mise does not currently
// resolve to a binary. A tool is considered installed if `mise which <tool>`
// succeeds (it prints the shim/install path and exits 0).
func getMissingTools(tools []string) ([]string, error) {
	// Fail fast with a clear error if mise is not on PATH at all.
	if _, err := exec.LookPath("mise"); err != nil {
		return nil, fmt.Errorf("mise not found on PATH: %w", err)
	}
	var missing []string
	for _, tool := range tools {
		cmd := exec.Command("mise", "which", tool)
		if err := cmd.Run(); err != nil {
			missing = append(missing, tool)
		}
	}
	return missing, nil
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
	if !config.NoHeader {
		renderedBuffer.WriteString("# DO NOT EDIT\n")
		renderedBuffer.WriteString("# Managed by dotfiles: https://github.com/guettli/dotfiles\n\n")
	}
	if err := tmpl.Execute(&renderedBuffer, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	renderedContent := renderedBuffer.Bytes()

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
