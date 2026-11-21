package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// convertLegacyConfig turns an old config.yaml into the TOML config. shell
// stays unset, so it follows the login shell (old versions had no setting
// for it). mise_tools, which no longer exists, is returned for a hint.
func convertLegacyConfig(data []byte) (UserConfig, []string, error) {
	var old legacyYAMLConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&old); err != nil {
		if errors.Is(err, io.EOF) {
			return UserConfig{}, nil, fmt.Errorf("the file is empty")
		}
		return UserConfig{}, nil, err
	}
	if old.Name == "" || old.PersonalEmail == "" {
		return UserConfig{}, nil, fmt.Errorf("name and personal_email are needed")
	}
	cfg := UserConfig{
		Name:          old.Name,
		PersonalEmail: old.PersonalEmail,
	}
	for _, o := range old.Orgs {
		cfg.Orgs = append(cfg.Orgs, OrgConfig{URL: o.URL, Email: o.Email})
	}
	return cfg, old.MiseTools, nil
}

// migrateLegacyConfig converts ~/.config/dotfiles/config.yaml to config.toml
// when there is no config.toml yet. apply writes config.toml and moves the
// YAML file to the backup directory. diff writes nothing: it returns the path
// of a temporary converted config to use instead (the caller removes its
// directory).
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
		miseHint := func() {
			if len(miseTools) > 0 {
				fmt.Printf("ℹ️  mise_tools no longer exists. Install those tools yourself (once):\n      mise use -g %s\n",
					strings.Join(miseTools, " "))
			}
		}
		if command != "apply" {
			// A private directory: the config holds your email addresses.
			dir, err := os.MkdirTemp("", "dotfiles-config-")
			if err != nil {
				return "", err
			}
			tmp := filepath.Join(dir, "config.toml")
			if err := writeUserConfig(tmp, cfg); err != nil {
				os.RemoveAll(dir)
				return "", err
			}
			if _, err := loadUserConfig(homeDir, tmp); err != nil {
				os.RemoveAll(dir)
				fmt.Printf("⚠️ Could not convert %s automatically: %v\n", tildePath(legacy), err)
				return "", nil
			}
			fmt.Printf("   would convert %s to %s\n", tildePath(legacy), tildePath(tomlPath))
			miseHint()
			return tmp, nil
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
		fmt.Printf("✅ Converted %s to %s. Old file: %s\n", tildePath(legacy), tildePath(tomlPath), bak)
		miseHint()
		return "", nil
	}
	return "", nil
}
