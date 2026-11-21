package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// legacyYAMLConfig is the config.yaml format of versions before #10.
type legacyYAMLConfig struct {
	Name          string `yaml:"name"`
	PersonalEmail string `yaml:"personal_email"`
	Orgs          []struct {
		URL   string `yaml:"url"`
		Email string `yaml:"email"`
	} `yaml:"orgs"`
	MiseTools []string `yaml:"mise_tools"`
}

// convertLegacyConfig turns an old config.yaml into the TOML config. The
// shell is the login shell (old versions had no setting for it), and
// mise_tools, which no longer exists, is returned for a hint.
func convertLegacyConfig(data []byte) (UserConfig, []string, error) {
	var old legacyYAMLConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&old); err != nil {
		return UserConfig{}, nil, err
	}
	cfg := UserConfig{
		Name:          old.Name,
		PersonalEmail: old.PersonalEmail,
		Shell:         defaultShell(),
	}
	for _, o := range old.Orgs {
		cfg.Orgs = append(cfg.Orgs, OrgConfig{URL: o.URL, Email: o.Email})
	}
	return cfg, old.MiseTools, nil
}

// migrateLegacyConfig converts ~/.config/dotfiles/config.yaml to config.toml
// when there is no config.toml yet. apply writes config.toml and moves the
// YAML file to the backup directory. diff writes nothing: it returns the path
// of a temporary converted config to use instead (the caller removes it).
// On a YAML file it cannot convert it returns "", nil, and loadUserConfig
// explains the format change as before.
func migrateLegacyConfig(homeDir, command string) (tmpConfig string, err error) {
	tomlPath, legacyPaths := configPaths(homeDir)
	if _, err := os.Stat(tomlPath); !os.IsNotExist(err) {
		return "", nil
	}
	for _, legacy := range legacyPaths {
		data, err := os.ReadFile(legacy)
		if err != nil {
			continue
		}
		cfg, miseTools, err := convertLegacyConfig(data)
		if err != nil {
			fmt.Printf("⚠️ Could not convert %s automatically: %v\n", tildePath(legacy), err)
			return "", nil
		}
		if len(miseTools) > 0 {
			fmt.Printf("ℹ️  mise_tools no longer exists. Install those tools yourself (once):\n      mise use -g %s\n",
				strings.Join(miseTools, " "))
		}
		if command != "apply" {
			f, err := os.CreateTemp("", "dotfiles-config-*.toml")
			if err != nil {
				return "", err
			}
			f.Close()
			if err := writeUserConfig(f.Name(), cfg); err != nil {
				os.Remove(f.Name())
				return "", err
			}
			if _, err := loadUserConfig(homeDir, f.Name()); err != nil {
				os.Remove(f.Name())
				fmt.Printf("⚠️ Could not convert %s automatically: %v\n", tildePath(legacy), err)
				return "", nil
			}
			fmt.Printf("   would convert %s to %s\n", tildePath(legacy), tildePath(tomlPath))
			return f.Name(), nil
		}
		if err := writeUserConfig(tomlPath, cfg); err != nil {
			return "", err
		}
		if _, err := loadUserConfig(homeDir, tomlPath); err != nil {
			os.Remove(tomlPath)
			fmt.Printf("⚠️ Could not convert %s automatically: %v\n", tildePath(legacy), err)
			return "", nil
		}
		bak, err := backupFile(cacheDirFor(homeDir), legacy)
		if err != nil {
			os.Remove(tomlPath)
			return "", err
		}
		fmt.Printf("✅ Converted %s to %s (shell = %q). Old file: %s\n",
			tildePath(legacy), tildePath(tomlPath), cfg.Shell, bak)
		return "", nil
	}
	return "", nil
}
