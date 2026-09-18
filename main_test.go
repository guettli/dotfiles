package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeConfig writes body to <home>/.config/dotfiles/<name> and returns home.
func writeConfig(t *testing.T, name, body string) string {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "dotfiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestLoadUserConfig_Valid(t *testing.T) {
	home := writeConfig(t, "config.toml", `
name = "Jane"
personal_email = "jane@example.com"

[[orgs]]
url   = "github.com/acme"
email = "jane@acme.com"
`)
	cfg, err := loadUserConfig(home, "")
	if err != nil {
		t.Fatalf("valid config returned error: %v", err)
	}
	if cfg.Name != "Jane" || cfg.PersonalEmail != "jane@example.com" {
		t.Errorf("got name=%q email=%q", cfg.Name, cfg.PersonalEmail)
	}
	if len(cfg.Orgs) != 1 {
		t.Fatalf("want 1 org, got %d", len(cfg.Orgs))
	}
	// Host and Name are computed from the URL after load.
	if cfg.Orgs[0].Host != "github.com" || cfg.Orgs[0].Name != "acme" {
		t.Errorf("org host/name = %q/%q, want github.com/acme", cfg.Orgs[0].Host, cfg.Orgs[0].Name)
	}
}

func TestLoadUserConfig_UnknownFieldIsError(t *testing.T) {
	// `personal_emial` (typo) must fail loudly, not be silently ignored.
	home := writeConfig(t, "config.toml", "name = \"n\"\npersonal_emial = \"e@x.com\"\n")
	if _, err := loadUserConfig(home, ""); err == nil {
		t.Fatal("expected an error for an unknown config field, got nil")
	}
}

func TestLoadUserConfig_MiseToolsRemovedHint(t *testing.T) {
	// The removed `mise_tools` key must error with a pointer to `mise use -g`.
	home := writeConfig(t, "config.toml", "name = \"n\"\npersonal_email = \"e@x.com\"\nmise_tools = [\"claude\"]\n")
	_, err := loadUserConfig(home, "")
	if err == nil {
		t.Fatal("expected an error for the removed mise_tools field, got nil")
	}
	if !strings.Contains(err.Error(), "mise use -g") {
		t.Errorf("error should hint at `mise use -g`, got: %v", err)
	}
}

func TestLoadUserConfig_LegacyYamlRefused(t *testing.T) {
	// An old config.yaml (and no config.toml) must be refused with a migration
	// hint rather than parsed.
	home := writeConfig(t, "config.yaml", "name: n\npersonal_email: e@x.com\n")
	_, err := loadUserConfig(home, "")
	if err == nil {
		t.Fatal("expected legacy YAML config to be refused, got nil")
	}
	if !strings.Contains(err.Error(), "config.toml") {
		t.Errorf("error should point at config.toml, got: %v", err)
	}
}

func TestLoadUserConfig_ExplicitYamlPathRefused(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, "somewhere.yaml")
	if err := os.WriteFile(p, []byte("name: n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadUserConfig(home, p); err == nil {
		t.Fatal("expected --config pointing at a .yaml file to be refused, got nil")
	}
}

func TestAddManagedHeader(t *testing.T) {
	// Header goes on top for a normal file...
	got := string(addManagedHeader([]byte("body\n")))
	if !strings.HasPrefix(got, managedHeader) {
		t.Errorf("header not at top: %q", got)
	}
	// ...but below a shebang.
	got = string(addManagedHeader([]byte("#!/bin/sh\nbody\n")))
	want := "#!/bin/sh\n" + managedHeader + "body\n"
	if got != want {
		t.Errorf("shebang handling: got %q want %q", got, want)
	}
	if !reflect.DeepEqual(got[:len("#!/bin/sh")], "#!/bin/sh") {
		t.Errorf("shebang must stay first line")
	}
}

func TestLoadUserConfig_UnknownKeyUnderOrg(t *testing.T) {
	home := writeConfig(t, "config.toml", `
name = "n"
personal_email = "e@x.com"

[[orgs]]
url = "github.com/acme"
email = "j@acme.com"
bogus = "z"
`)
	if _, err := loadUserConfig(home, ""); err == nil {
		t.Fatal("expected an error for an unknown key under [[orgs]], got nil")
	}
}

func TestLoadUserConfig_TrailingSlashOrgRejected(t *testing.T) {
	// A trailing slash used to yield an empty org name (~/.gitconfig-org-).
	home := writeConfig(t, "config.toml", `
name = "n"
personal_email = "e@x.com"

[[orgs]]
url = "github.com/acme/"
email = "j@acme.com"
`)
	// After trimming, "github.com/acme" is valid and Name must be "acme".
	cfg, err := loadUserConfig(home, "")
	if err != nil {
		t.Fatalf("trailing slash should be trimmed, not error: %v", err)
	}
	if cfg.Orgs[0].Name != "acme" || cfg.Orgs[0].Host != "github.com" {
		t.Errorf("got host/name %q/%q, want github.com/acme", cfg.Orgs[0].Host, cfg.Orgs[0].Name)
	}
}

func TestLoadUserConfig_OrgWithoutSlashRejected(t *testing.T) {
	home := writeConfig(t, "config.toml", `
name = "n"
personal_email = "e@x.com"

[[orgs]]
url = "github.com"
email = "j@acme.com"
`)
	if _, err := loadUserConfig(home, ""); err == nil {
		t.Fatal("expected an error for an org url without host/name, got nil")
	}
}
