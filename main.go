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

	"gopkg.in/yaml.v3"
)

//go:embed all:templates/*
var templatesFS embed.FS

type Config struct {
	Source      string
	Destination string
	Mode        os.FileMode
}

type OrgConfig struct {
	URL   string `yaml:"url"`
	Email string `yaml:"email"`
	Name  string `yaml:"-"` // last path segment of URL, computed after load
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
	data, err := os.ReadFile(path)
	if err != nil {
		return UserConfig{}, fmt.Errorf("could not read %s: %w\nCreate it from config.example.yaml in the dotfiles repo, or pass --config <path>", path, err)
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

	requiredPackages := []string{
		"nixpkgs#starship",
		"nixpkgs#atuin",
		"nixpkgs#direnv",
		"nixpkgs#nix-direnv",
		"nixpkgs#tmux",
		"nixpkgs#antidote",
		"nixpkgs#bash-completion",
	}

	if command == "apply" {
		fmt.Println("🚀 Applying dotfiles...")
	} else {
		fmt.Println("🔍 Diffing dotfiles...")
	}

	missingPkgs, err := getMissingPackages(requiredPackages)
	if err != nil {
		fmt.Printf("⚠️ Could not check nix profiles (is nix installed?): %v\n", err)
	} else {
		if len(missingPkgs) > 0 {
			if command == "diff" {
				fmt.Println("\n--- Dependencies to Install ---")
				for _, pkg := range missingPkgs {
					fmt.Printf("+ %s\n", pkg)
				}
				fmt.Println()
			} else if command == "apply" {
				fmt.Println("📦 Installing missing dependencies via Nix...")
				args := append([]string{"--extra-experimental-features", "nix-command flakes", "profile", "add"}, missingPkgs...)
				cmd := exec.Command("nix", args...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("❌ Failed to install dependencies: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("✅ Dependencies installed!\n")
			}
		} else {
			if command == "diff" {
				fmt.Println("   All Nix dependencies are already installed.")
			}
		}
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
			Source:      "templates/git/gitconfig",
			Destination: filepath.Join(homeDir, ".gitconfig"),
		},
		{
			Source:      "templates/git/hooks/prepare-commit-msg",
			Destination: filepath.Join(homeDir, ".config", "git", "hooks", "prepare-commit-msg"),
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

func getMissingPackages(packages []string) ([]string, error) {
	cmd := exec.Command("nix", "--extra-experimental-features", "nix-command flakes", "profile", "list")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("nix command failed: %v\nStderr: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	output := string(out)
	var missing []string
	for _, pkg := range packages {
		parts := strings.Split(pkg, "#")
		name := pkg
		if len(parts) > 1 {
			name = parts[1]
		}
		if !strings.Contains(output, name) {
			missing = append(missing, pkg)
		}
	}
	return missing, nil
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
	renderedBuffer.WriteString("# DO NOT EDIT\n")
	renderedBuffer.WriteString("# Managed by dotfiles: https://github.com/guettli/dotfiles\n\n")
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

	// 2. Check for local modifications if destination exists
	if fileInfo, err := os.Stat(config.Destination); err == nil {
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			// It's a symlink. We'll replace it.
			os.Remove(config.Destination)
		} else {
			// Real file exists. Check against cache.
			if _, err := os.Stat(cacheFile); err == nil {
				// Cache exists. Diff cache against destination.
				cacheBytes, _ := os.ReadFile(cacheFile)
				destBytes, _ := os.ReadFile(config.Destination)
				if !bytes.Equal(cacheBytes, destBytes) {
					fmt.Printf("\n⚠️ LOCAL MODIFICATION DETECTED: %s\n", config.Destination)
					runDiffCommand(config.Destination, destBytes, "Installed Baseline", "Current Local File")
					if !force {
						return fmt.Errorf("local modifications detected in %s. Please sync them to templates or revert them (or use --force to overwrite)", config.Destination)
					}
					fmt.Println("   Continuing anyway because --force was used.")
				}
			} else {
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
		return nil
	}

	err = os.WriteFile(config.Destination, renderedContent, mode)
	if err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}

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
