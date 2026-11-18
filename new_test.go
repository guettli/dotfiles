package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func bashrcConfig(home string) Config {
	return Config{
		Source:      "templates/bash/.bashrc",
		Destination: filepath.Join(home, ".bashrc"),
		LocalFile:   filepath.Join(home, ".bashrc.local"),
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// An installer (conda, nvm) appends to ~/.bashrc: apply moves those lines to
// ~/.bashrc.local instead of failing.
func TestApplyMovesAppendedLinesToLocalFile(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	managed := readFile(t, c.Destination)
	appended := "\n# >>> conda init >>>\nexport PATH=/opt/conda/bin:$PATH\n"
	mustWrite(t, c.Destination, managed+appended)

	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatalf("appended lines must not be an error: %v", err)
	}
	if got := readFile(t, c.Destination); got != managed {
		t.Error("~/.bashrc was not restored to the managed version")
	}
	local := readFile(t, c.LocalFile)
	if !strings.Contains(local, "export PATH=/opt/conda/bin:$PATH") || !strings.Contains(local, "Moved here from") {
		t.Errorf("~/.bashrc.local = %q", local)
	}
}

// Edits in the middle are still an error which names the local file; with
// --force the edited version is backed up, not lost.
func TestApplyEditedFileForceBacksUp(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	edited := "alias x=y\n" + readFile(t, c.Destination)
	mustWrite(t, c.Destination, edited)

	err := processConfig(c, TemplateData{}, cacheDir, "apply", false)
	if err == nil || !strings.Contains(err.Error(), ".bashrc.local") {
		t.Fatalf("want error naming ~/.bashrc.local, got %v", err)
	}
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", true); err != nil {
		t.Fatal(err)
	}
	if readFile(t, backupPath(cacheDir, c.Destination)) != edited {
		t.Error("the edited version was not backed up")
	}
}

// A pre-existing ~/.bashrc is backed up on the first apply, and uninstall
// restores it.
func TestAdoptAndUninstallRestores(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	orig := "# Ubuntu default bashrc\n"
	mustWrite(t, c.Destination, orig)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	if readFile(t, backupPath(cacheDir, c.Destination)) != orig {
		t.Fatal("original not backed up")
	}
	if err := uninstallFile(c, cacheDir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, c.Destination) != orig {
		t.Error("uninstall did not restore the original")
	}
}

// uninstall keeps a managed file the user edited.
func TestUninstallKeepsEditedFile(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	edited := readFile(t, c.Destination) + "alias x=y\n"
	mustWrite(t, c.Destination, edited)
	if err := uninstallFile(c, cacheDir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, c.Destination) != edited {
		t.Error("edited file was removed")
	}
}

// A symlinked ~/.bashrc (GNU stow) must not be written through.
func TestSymlinkIsNotWrittenThrough(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	target := filepath.Join(home, "stow", "bashrc")
	mustWrite(t, target, "mine\n")
	if err := os.Symlink(target, c.Destination); err != nil {
		t.Fatal(err)
	}
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	if readFile(t, target) != "mine\n" {
		t.Error("symlink target was overwritten")
	}
	if fi, err := os.Lstat(backupPath(cacheDir, c.Destination)); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink not backed up as a link: %v", err)
	}
}

func TestLoadUserConfig_ShellAndProfile(t *testing.T) {
	home := writeConfig(t, "config.toml", "name = \"J\"\npersonal_email = \"j@x\"\nshell = \"fish\"\n")
	if _, err := loadUserConfig(home, ""); err == nil {
		t.Error("shell = fish must be rejected")
	}
	home = writeConfig(t, "config.toml", "name = \"J\"\npersonal_email = \"j@x\"\nprofile = \"nope\"\n")
	if _, err := loadUserConfig(home, ""); err == nil || !strings.Contains(err.Error(), "guettli") {
		t.Errorf("unknown profile must be rejected and list the available ones, got %v", err)
	}
	home = writeConfig(t, "config.toml", "name = \"J\"\npersonal_email = \"j@x\"\nprofile = \"guettli\"\nshell = \"zsh\"\n")
	cfg, err := loadUserConfig(home, "")
	if err != nil {
		t.Fatal(err)
	}
	var sawZsh, sawProfile bool
	for _, c := range managedConfigs(home, cfg) {
		if strings.HasSuffix(c.Destination, ".zshrc") {
			sawZsh = true
		}
		if strings.HasSuffix(c.Destination, "shell/profile.sh") {
			b, err := renderConfig(c, c.Data)
			if err != nil {
				t.Fatal(err)
			}
			sawProfile = bytes.Contains(b, []byte("git-tips"))
		}
	}
	if !sawZsh || !sawProfile {
		t.Errorf("zsh=%v profile=%v", sawZsh, sawProfile)
	}
}

func TestBashIsDefaultWithoutZshFiles(t *testing.T) {
	home := writeConfig(t, "config.toml", "name = \"J\"\npersonal_email = \"j@x\"\nshell = \"bash\"\n")
	cfg, err := loadUserConfig(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Shell != "bash" {
		t.Fatalf("shell = %q", cfg.Shell)
	}
	for _, c := range managedConfigs(home, cfg) {
		if strings.Contains(c.Destination, "zsh") {
			t.Errorf("bash user gets %s", c.Destination)
		}
	}
}

// The default (no profile) must not trust every .envrc.
func TestDirenvDoesNotTrustEverythingByDefault(t *testing.T) {
	home := t.TempDir()
	for _, c := range managedConfigs(home, UserConfig{Shell: "bash"}) {
		b, err := renderConfig(c, c.Data)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "whitelist") || strings.Contains(string(b), "git-tips") {
			t.Errorf("%s contains personal settings without a profile", c.Destination)
		}
	}
}

func TestInitWritesLoadableConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	home, _ := os.UserHomeDir()
	in := bufio.NewReader(strings.NewReader(""))
	cfg, err := buildConfig(in, &bytes.Buffer{}, false, initValues{
		Name: `Jane "JJ" Doe`, Email: "jane@example.com",
		OrgURL: "https://github.com/acme/", OrgEmail: "jane@acme.com", Shell: "bash",
	})
	if err != nil {
		t.Fatal(err)
	}
	path, _ := configPaths(home)
	if err := writeUserConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := loadUserConfig(home, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != `Jane "JJ" Doe` || got.Orgs[0].Name != "acme" || got.Orgs[0].Host != "github.com" {
		t.Errorf("got %+v", got)
	}
	// Non-interactive without a name is an error that says what to pass.
	if _, err := buildConfig(in, &bytes.Buffer{}, false, initValues{}); err == nil || !strings.Contains(err.Error(), "--name") {
		t.Errorf("want --name hint, got %v", err)
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.2.0", "v0.1.9", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.10.0", false},
		{"v1.0.0", "dev", false},
		{"v1.0.0", "v0.0.0-20261118-abcdef", true},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v", c.latest, c.current, got)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello")
	sums := []byte("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824  dotfiles_linux_amd64.tar.gz\n")
	if err := verifyChecksum(sums, "dotfiles_linux_amd64.tar.gz", data); err != nil {
		t.Error(err)
	}
	if err := verifyChecksum(sums, "dotfiles_linux_amd64.tar.gz", []byte("evil")); err == nil {
		t.Error("mismatch not detected")
	}
	if err := verifyChecksum(sums, "other.tar.gz", data); err == nil {
		t.Error("missing entry not detected")
	}
}

// Without a pre-commit config, the managed pre-commit hook runs the
// repository's own hook from .git/hooks, which git skips because
// core.hooksPath is set.
func TestPreCommitRunsRepoHook(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	hooksDir := filepath.Join(dir, "hooks")
	hook := filepath.Join(hooksDir, "pre-commit")
	body, err := templatesFS.ReadFile("templates/git/hooks/pre-commit")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hook, body, 0755); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	run("init", "-q")
	run("config", "core.hooksPath", hooksDir)
	marker := filepath.Join(dir, "ran")
	mustWrite(t, filepath.Join(repo, ".git", "hooks", "pre-commit"), "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(filepath.Join(repo, ".git", "hooks", "pre-commit"), 0755); err != nil {
		t.Fatal(err)
	}
	run("-c", "user.name=t", "-c", "user.email=t@x", "commit", "-q", "--allow-empty", "-m", "x")
	if _, err := os.Stat(marker); err != nil {
		t.Error("the repository's pre-commit hook did not run")
	}
}

// Versions before the backup directory saved the original as <file>.bak;
// uninstall must restore that, not leave the user without ~/.bashrc.
func TestUninstallRestoresLegacyBak(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, c.Destination+".bak", "# old\n")
	if err := uninstallFile(c, cacheDir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, c.Destination) != "# old\n" {
		t.Error("legacy .bak not restored")
	}
}

// If apply failed after moving the appended lines but before rewriting the
// managed file, the next apply must succeed and not duplicate them.
func TestAppendedLinesInterruptedApply(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	managed := readFile(t, c.Destination)
	mustWrite(t, c.Destination, managed+"export A=1\n")
	// Simulate the interruption: lines already in the local file.
	if err := appendToLocalFile(c.LocalFile, c.Destination, []byte("export A=1\n")); err != nil {
		t.Fatal(err)
	}
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(readFile(t, c.LocalFile), "export A=1"); n != 1 {
		t.Errorf("appended line is %d times in the local file", n)
	}
	if readFile(t, c.Destination) != managed {
		t.Error("managed file not restored")
	}
}

// A failing move to the local file leaves the destination untouched, so a
// later apply (after the problem is fixed) succeeds.
func TestAppendedLinesLocalFileError(t *testing.T) {
	home := t.TempDir()
	cacheDir := cacheDirFor(home)
	c := bashrcConfig(home)
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatal(err)
	}
	managed := readFile(t, c.Destination)
	mustWrite(t, c.Destination, managed+"export A=1\n")
	if err := os.Mkdir(c.LocalFile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err == nil {
		t.Fatal("want error, local file is a directory")
	}
	if readFile(t, c.Destination) != managed+"export A=1\n" {
		t.Error("destination changed although the move failed")
	}
	if err := os.Remove(c.LocalFile); err != nil {
		t.Fatal(err)
	}
	if err := processConfig(c, TemplateData{}, cacheDir, "apply", false); err != nil {
		t.Fatalf("apply after fixing the problem: %v", err)
	}
	if !strings.Contains(readFile(t, c.LocalFile), "export A=1") {
		t.Error("line not moved")
	}
}

// Renovate bumps exact versions in templates/mise/dotfiles.toml; "latest"
// would never be updated and differs between machines.
func TestBaseToolsArePinned(t *testing.T) {
	tools, err := baseMiseTools()
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) == 0 {
		t.Fatal("no tools")
	}
	for _, tool := range tools {
		if _, ok := parseSemverLoose(tool.Version); !ok {
			t.Errorf("%s: version %q is not pinned", tool.Name, tool.Version)
		}
	}
}

// parseSemverLoose accepts "1", "1.2" and "1.2.3", plus a trailing letter
// (tmux: "3.8", "3.8a").
func parseSemverLoose(v string) ([]int, bool) {
	var out []int
	if n := len(v); n > 1 && v[n-1] >= 'a' && v[n-1] <= 'z' {
		v = v[:n-1]
	}
	for _, p := range strings.Split(v, ".") {
		n := 0
		if p == "" {
			return nil, false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return nil, false
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out, len(out) >= 1 && len(out) <= 3
}

func TestWriteGenerated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "kubectl.bash")
	changed, err := writeGenerated(path, []byte("x\n"))
	if err != nil || !changed {
		t.Fatalf("first write: changed=%v err=%v", changed, err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0644 {
		t.Errorf("mode: %v %v", fi.Mode(), err)
	}
	if changed, err := writeGenerated(path, []byte("x\n")); err != nil || changed {
		t.Errorf("same content: changed=%v err=%v", changed, err)
	}
	if changed, err := writeGenerated(path, []byte("y\n")); err != nil || !changed || readFile(t, path) != "y\n" {
		t.Errorf("new content: changed=%v err=%v", changed, err)
	}
}

// The alias `k` is completed by __start_kubectl, so .bashrc must source the
// kubectl completion script before registering it (#13).
func TestBashrcLoadsKubectlCompletionBeforeK(t *testing.T) {
	body, err := templatesFS.ReadFile("templates/bash/.bashrc")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	load := strings.Index(s, `source "$HOME/.cache/dotfiles/completions/kubectl.bash"`)
	reg := strings.Index(s, "complete -o default -F __start_kubectl k")
	if load < 0 || reg < 0 || reg < load {
		t.Errorf("load=%d register=%d", load, reg)
	}
}

// The zsh completion must be loaded after compinit (compdef needs it).
func TestZshrcLoadsKubectlCompletionAfterCompinit(t *testing.T) {
	body, err := templatesFS.ReadFile("templates/zsh/.zshrc")
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	ci := strings.Index(s, "compinit -u")
	load := strings.Index(s, `source "$HOME/.cache/dotfiles/completions/kubectl.zsh"`)
	if ci < 0 || load < 0 || load < ci {
		t.Errorf("compinit=%d load=%d", ci, load)
	}
}

// fakeKubectl puts a kubectl script with the given body first on PATH.
func fakeKubectl(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "kubectl"), "#!/bin/sh\n"+body+"\n")
	if err := os.Chmod(filepath.Join(dir, "kubectl"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
}

func TestEnsureCompletions(t *testing.T) {
	home := t.TempDir()
	bashFile := filepath.Join(completionsDir(home), "kubectl.bash")
	zshFile := filepath.Join(completionsDir(home), "kubectl.zsh")

	fakeKubectl(t, `echo "# $2"; echo "__start_kubectl() { :; }"; echo "_kubectl() { :; }"`)
	ensureCompletions(home, "bash", "diff")
	if _, err := os.Stat(bashFile); err == nil {
		t.Fatal("diff wrote a file")
	}
	ensureCompletions(home, "bash", "apply")
	if !strings.Contains(readFile(t, bashFile), "# bash") {
		t.Error("bash completion not cached")
	}
	if _, err := os.Stat(zshFile); err == nil {
		t.Error("zsh completion cached for a bash user")
	}
	ensureCompletions(home, "zsh", "apply")
	if !strings.Contains(readFile(t, zshFile), "# zsh") {
		t.Error("zsh completion not cached")
	}

	// A failing kubectl keeps the old file.
	fakeKubectl(t, `echo "mise ERROR not installed" >&2; exit 1`)
	ensureCompletions(home, "bash", "apply")
	if !strings.Contains(readFile(t, bashFile), "# bash") {
		t.Error("failing kubectl changed the cached file")
	}

	// Output that is no completion script is not cached.
	fakeKubectl(t, `echo "Please upgrade kubectl"`)
	ensureCompletions(home, "bash", "apply")
	if strings.Contains(readFile(t, bashFile), "upgrade") {
		t.Error("garbage cached")
	}
}

func TestKubectlCompletionTimeout(t *testing.T) {
	old := completionTimeout
	completionTimeout = 200 * time.Millisecond
	defer func() { completionTimeout = old }()
	// No exec: a wrapper whose child keeps stdout open.
	fakeKubectl(t, `sleep 30`)
	start := time.Now()
	_, err := kubectlCompletion("bash")
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Errorf("want timeout, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Error("did not stop at the timeout")
	}
}

func TestKubectlCompletionErrorShowsStderr(t *testing.T) {
	fakeKubectl(t, `echo "mise ERROR kubectl@1.2.3 not installed" >&2; exit 1`)
	_, err := kubectlCompletion("bash")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("want stderr in the error, got %v", err)
	}
}
