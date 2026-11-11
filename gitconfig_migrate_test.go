package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const legacyBody = managedHeader + "[user]\n    name = Jane\n"

// legacySetup writes ~/.gitconfig (if gitconfig != "") and the legacy cache
// baseline (if baseline != "") and returns home and cacheDir.
func legacySetup(t *testing.T, gitconfig, baseline string) (string, string) {
	t.Helper()
	home := t.TempDir()
	cacheDir := filepath.Join(home, ".local", "state", "dotfiles", "installed_cache")
	if gitconfig != "" {
		if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte(gitconfig), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if baseline != "" {
		p := filepath.Join(cacheDir, legacyGitconfigSource)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(baseline), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return home, cacheDir
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func cacheGone(t *testing.T, cacheDir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(cacheDir, legacyGitconfigSource)); !os.IsNotExist(err) {
		t.Errorf("legacy cache baseline still there: %v", err)
	}
}

func TestMigrate_Unmodified(t *testing.T) {
	home, cacheDir := legacySetup(t, legacyBody, legacyBody)
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig")); got != userGitconfigHeader {
		t.Errorf("~/.gitconfig = %q", got)
	}
	cacheGone(t, cacheDir)
}

func TestMigrate_KeepsAppendedGhHelper(t *testing.T) {
	gh := "[credential \"https://github.com\"]\n\thelper = \n\thelper = !/usr/bin/gh auth git-credential\n"
	home, cacheDir := legacySetup(t, legacyBody+gh, legacyBody)
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(home, ".gitconfig"))
	if got != userGitconfigHeader+"\n"+gh {
		t.Errorf("~/.gitconfig = %q", got)
	}
	if strings.Contains(got, "name = Jane") {
		t.Error("managed settings left in ~/.gitconfig")
	}
	cacheGone(t, cacheDir)
}

func TestMigrate_EditedInTheMiddleFails(t *testing.T) {
	edited := strings.Replace(legacyBody, "Jane", "Janet", 1)
	home, cacheDir := legacySetup(t, edited, legacyBody)
	err := migrateLegacyGitconfig(home, cacheDir, "apply", false)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("want error mentioning --force, got %v", err)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig")); got != edited {
		t.Error("file changed although the migration failed")
	}
}

func TestMigrate_NoBaselineFails(t *testing.T) {
	home, cacheDir := legacySetup(t, legacyBody, "")
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err == nil {
		t.Fatal("want error without a recorded baseline")
	}
}

func TestMigrate_UserOwnedUntouchedEvenWithForce(t *testing.T) {
	own := "[user]\n    name = Me\n"
	home, cacheDir := legacySetup(t, own, legacyBody) // stale legacy cache, no header
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig")); got != own {
		t.Error("a user-owned file must not be touched, even with --force")
	}
}

func TestMigrate_ForceMovesToBak(t *testing.T) {
	edited := strings.Replace(legacyBody, "Jane", "Janet", 1)
	home, cacheDir := legacySetup(t, edited, legacyBody)
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig.bak")); got != edited {
		t.Errorf(".bak = %q", got)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig")); got != userGitconfigHeader {
		t.Errorf("~/.gitconfig = %q", got)
	}
	cacheGone(t, cacheDir)
}

func TestMigrate_ForceRefusesToOverwriteBak(t *testing.T) {
	edited := strings.Replace(legacyBody, "Jane", "Janet", 1)
	home, cacheDir := legacySetup(t, edited, legacyBody)
	bak := filepath.Join(home, ".gitconfig.bak")
	if err := os.WriteFile(bak, []byte("PRECIOUS"), 0644); err != nil {
		t.Fatal(err)
	}
	err := migrateLegacyGitconfig(home, cacheDir, "apply", true)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want error about the existing .bak, got %v", err)
	}
	if readFile(t, bak) != "PRECIOUS" || readFile(t, filepath.Join(home, ".gitconfig")) != edited {
		t.Error("files changed")
	}
}

// git appends a key to the end of its section: a key added to the last
// managed section lands after the managed part without a [section].
func TestMigrate_KeyWithoutSectionFails(t *testing.T) {
	home, cacheDir := legacySetup(t, legacyBody+"\tkeepBackup = false\n", legacyBody)
	err := migrateLegacyGitconfig(home, cacheDir, "apply", false)
	if err == nil || !strings.Contains(err.Error(), "not inside a [section]") {
		t.Fatalf("want section error, got %v", err)
	}
}

func TestMigrate_InvalidKeptFails(t *testing.T) {
	if _, err := lookGit(); err != nil {
		t.Skip("git not installed")
	}
	home, cacheDir := legacySetup(t, legacyBody+"[broken\n", legacyBody)
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err == nil || !strings.Contains(err.Error(), "git rejects") {
		t.Fatalf("want git error, got %v", err)
	}
}

func TestMigrate_KeepsFileMode(t *testing.T) {
	home, cacheDir := legacySetup(t, legacyBody, legacyBody)
	p := filepath.Join(home, ".gitconfig")
	if err := os.Chmod(p, 0600); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestCheckXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		val     string
		wantErr bool
	}{{"", false}, {filepath.Join(home, ".config"), false}, {filepath.Join(home, ".config") + "/", false}, {"/elsewhere", true}} {
		t.Setenv("XDG_CONFIG_HOME", tc.val)
		if err := checkXDGConfigHome(home); (err != nil) != tc.wantErr {
			t.Errorf("XDG_CONFIG_HOME=%q: err = %v", tc.val, err)
		}
	}
}

func gitConfigEntry(home string) Config {
	return Config{Source: gitConfigSource, Destination: filepath.Join(home, ".config", "git", "config"), NoAdopt: true}
}

func TestUnmanagedXDGGitConfigIsAnError(t *testing.T) {
	home := t.TempDir()
	cacheDir := filepath.Join(home, "cache")
	dest := filepath.Join(home, ".config", "git", "config")
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		t.Fatal(err)
	}
	own := "[core]\n    editor = vim\n"
	if err := os.WriteFile(dest, []byte(own), 0644); err != nil {
		t.Fatal(err)
	}
	data := TemplateData{Home: home, Name: "Jane", PersonalEmail: "j@example.com"}
	err := processConfig(gitConfigEntry(home), data, cacheDir, "apply", false)
	if err == nil || !strings.Contains(err.Error(), "~/.gitconfig") {
		t.Fatalf("want error pointing to ~/.gitconfig, got %v", err)
	}
	if readFile(t, dest) != own {
		t.Error("user's file changed")
	}
	// --force backs it up and deploys.
	if err := processConfig(gitConfigEntry(home), data, cacheDir, "apply", true); err != nil {
		t.Fatal(err)
	}
	if readFile(t, dest+".bak") != own || !strings.Contains(readFile(t, dest), "name = Jane") {
		t.Error("force did not back up and deploy")
	}
}

func TestMigrate_CreatesMissingGitconfig(t *testing.T) {
	home, cacheDir := legacySetup(t, "", "")
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig")); got != userGitconfigHeader {
		t.Errorf("~/.gitconfig = %q", got)
	}
}

func TestMigrate_DiffChangesNothing(t *testing.T) {
	home, cacheDir := legacySetup(t, legacyBody, legacyBody)
	if err := migrateLegacyGitconfig(home, cacheDir, "diff", false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(home, ".gitconfig")); got != legacyBody {
		t.Error("diff modified ~/.gitconfig")
	}
	home, cacheDir = legacySetup(t, "", "")
	if err := migrateLegacyGitconfig(home, cacheDir, "diff", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".gitconfig")); !os.IsNotExist(err) {
		t.Error("diff created ~/.gitconfig")
	}
}

// After the migration, `git config --global` must still write to
// ~/.gitconfig, not into the managed ~/.config/git/config.
func TestGitConfigGlobalWritesToUserFile(t *testing.T) {
	if _, err := lookGit(); err != nil {
		t.Skip("git not installed")
	}
	home, cacheDir := legacySetup(t, legacyBody, legacyBody)
	if err := migrateLegacyGitconfig(home, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	xdg := filepath.Join(home, ".config", "git", "config")
	if err := os.MkdirAll(filepath.Dir(xdg), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xdg, []byte(legacyBody), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, home, "config", "--global", "credential.helper", "test-helper")
	if strings.Contains(readFile(t, xdg), "test-helper") {
		t.Error("git config --global wrote into the managed ~/.config/git/config")
	}
	if !strings.Contains(readFile(t, filepath.Join(home, ".gitconfig")), "test-helper") {
		t.Error("git config --global did not write ~/.gitconfig")
	}
	// Both files are read; the managed setting is visible.
	if out := runGit(t, home, "config", "user.name"); strings.TrimSpace(out) != "Jane" {
		t.Errorf("user.name from ~/.config/git/config = %q", out)
	}
}

func lookGit() (string, error) { return exec.LookPath("git") }

func runGit(t *testing.T, home string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = home
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") && !strings.HasPrefix(e, "XDG_CONFIG_HOME=") && !strings.HasPrefix(e, "HOME=") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "HOME="+home, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
