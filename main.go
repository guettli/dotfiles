package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
)

//go:embed all:templates/*
var templatesFS embed.FS

type Config struct {
	Source string
	// Extra embedded files appended to Source before rendering (profile
	// additions).
	Extra       []string
	Destination string
	// CacheKey names the baseline in the cache dir; defaults to Source. Needed
	// when one template is deployed to several destinations.
	CacheKey string
	Mode     os.FileMode
	NoHeader bool
	// NoAdopt: an existing file which dotfiles did not write is an error
	// (not backed up and replaced), unless --force.
	NoAdopt bool
	// LocalFile is the user's own, unmanaged companion file (~/.bashrc.local).
	// Lines appended to Destination (by installers like conda or nvm) are
	// moved there by apply, and the "local modification" error points to it.
	LocalFile string
	// Data is the template data; nil means the default TemplateData.
	Data any
}

type OrgConfig struct {
	URL   string `toml:"url"`
	Email string `toml:"email"`
	Name  string `toml:"-"` // last path segment of URL, computed after load
	Host  string `toml:"-"` // host portion of URL (before first /), computed after load
}

type UserConfig struct {
	Name          string `toml:"name"`
	PersonalEmail string `toml:"personal_email"`
	// Shell is "bash" or "zsh". Empty: zsh if that is the login shell,
	// otherwise bash. zsh files (and antidote) are only installed for zsh.
	Shell string `toml:"shell,omitempty"`
	// Profile adds the files from templates/profiles/<name>/ (personal PATH
	// entries, git settings, ...). Empty: no profile.
	Profile string      `toml:"profile,omitempty"`
	Orgs    []OrgConfig `toml:"orgs,omitempty"`
}

type TemplateData struct {
	User          string
	Home          string
	Name          string
	PersonalEmail string
	Orgs          []OrgConfig
	Shell         string
	Profile       string
}

// configPaths returns the current (TOML) config path and the legacy (YAML)
// paths we refuse, in the base config directory.
func configPaths(homeDir string) (tomlPath string, legacy []string) {
	base := filepath.Join(homeDir, ".config", "dotfiles")
	return filepath.Join(base, "config.toml"), []string{
		filepath.Join(base, "config.yaml"),
		filepath.Join(base, "config.yml"),
	}
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
	tomlPath, legacyPaths := configPaths(homeDir)
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
			for _, legacy := range legacyPaths {
				if _, yErr := os.Stat(legacy); yErr == nil {
					return UserConfig{}, legacyConfigError(legacy, tomlPath)
				}
			}
		}
	}

	var cfg UserConfig
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		if os.IsNotExist(err) {
			return UserConfig{}, fmt.Errorf("could not read %s: %w\nCreate it with: dotfiles init", path, err)
		}
		return UserConfig{}, fmt.Errorf("could not parse %s: %w", path, err)
	}
	// KnownFields-style strictness: any key in the file that did not map onto a
	// struct field is a mistake (a typo, or a key removed in a format change).
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return UserConfig{}, unknownFieldError(path, undecoded)
	}

	switch cfg.Shell {
	case "":
		cfg.Shell = defaultShell()
	case "bash", "zsh":
	default:
		return UserConfig{}, fmt.Errorf("invalid shell %q in %s: use \"bash\" or \"zsh\"", cfg.Shell, path)
	}
	if cfg.Profile != "" {
		if _, err := fs.ReadDir(templatesFS, "templates/profiles/"+cfg.Profile); err != nil || strings.ContainsAny(cfg.Profile, "/.") {
			return UserConfig{}, fmt.Errorf("unknown profile %q in %s. Available: %s", cfg.Profile, path, strings.Join(availableProfiles(), ", "))
		}
	}

	for i, org := range cfg.Orgs {
		// Trim stray slashes (a trailing "/" is easy to paste) so we never build
		// an empty, colliding ~/.gitconfig-org- filename; require host/name.
		u := strings.Trim(org.URL, "/")
		if !strings.Contains(u, "/") {
			return UserConfig{}, fmt.Errorf("invalid org url %q in %s: expected host/name, e.g. github.com/your-company", org.URL, path)
		}
		parts := strings.Split(u, "/")
		cfg.Orgs[i].Name = parts[len(parts)-1]
		cfg.Orgs[i].Host = parts[0]
	}
	return cfg, nil
}

// runMise installs the tools declared in mise's config. On diff it only reports
// what is missing; on apply it runs `mise install` (idempotent).
func runMise(command string) error {
	if _, err := exec.LookPath("mise"); err != nil {
		return fmt.Errorf("mise not found on PATH — install it first: https://mise.jdx.dev/getting-started.html")
	}
	if command == "diff" {
		tools, err := baseMiseTools()
		if err != nil {
			return fmt.Errorf("could not read base tool list: %w", err)
		}
		// Check each base tool directly rather than via `mise ls --missing`, so
		// the answer is correct on a fresh machine (diff writes nothing, so the
		// conf.d file is not deployed yet) and is not polluted by the user's own
		// global/project mise config.
		var missing []string
		for _, tool := range tools {
			if !miseToolInstalled(tool) {
				missing = append(missing, tool.String())
			}
		}
		if len(missing) == 0 {
			fmt.Println("   All mise tools are already installed.")
			return nil
		}
		fmt.Println("\n--- mise tools to install (via `mise install`) ---")
		for _, tool := range missing {
			fmt.Printf("+ %s\n", tool)
		}
		fmt.Println()
		return nil
	}
	// command == "apply"
	fmt.Println("📦 Installing mise tools...")
	cmd := exec.Command("mise", "install")
	// In the home directory: a project's mise.toml in the current directory
	// is not part of the base set.
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mise install failed: %w", err)
	}
	fmt.Println("✅ mise tools installed!")
	pruneOldVersions()
	return nil
}

// pruneOldVersions removes versions of the base tools which no config uses any
// more, so the weekly Renovate bumps don't pile up. `mise prune` keeps every
// version a tracked config (a project's mise.toml) still asks for. Failures
// only cost disk space, so they are reported, not fatal.
func pruneOldVersions() {
	tools, err := baseMiseTools()
	if err != nil {
		return
	}
	args := []string{"prune", "--yes"}
	for _, t := range tools {
		args = append(args, t.Name)
	}
	cmd := exec.Command("mise", args...)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("⚠️ mise prune failed (old tool versions stay installed): %v\n%s", err, out)
	}
}

// miseTool is a tool of the base set and its pinned version.
type miseTool struct {
	Name    string
	Version string
}

func (t miseTool) String() string { return t.Name + "@" + t.Version }

// baseMiseTools returns the tools declared in the embedded
// templates/mise/dotfiles.toml [tools] table — the single source of truth for
// the base set, sorted by name for stable output. Tools limited to other
// operating systems (`os = [...]`) are left out, as mise does.
func baseMiseTools() ([]miseTool, error) {
	data, err := templatesFS.ReadFile("templates/mise/dotfiles.toml")
	if err != nil {
		return nil, err
	}
	return parseMiseTools(data, miseOS(runtime.GOOS))
}

// miseOS maps a GOOS to the name mise uses in a tool's `os` option.
func miseOS(goos string) string {
	if goos == "darwin" {
		return "macos"
	}
	return goos
}

// parseMiseTools reads a mise [tools] table whose values are a version string
// or a table like { version = "1.2.3", os = ["linux"] }, and keeps the tools
// mise installs on osName.
func parseMiseTools(data []byte, osName string) ([]miseTool, error) {
	var doc struct {
		Tools map[string]any `toml:"tools"`
	}
	if err := toml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	tools := make([]miseTool, 0, len(doc.Tools))
	for name, value := range doc.Tools {
		var version string
		switch v := value.(type) {
		case string:
			version = v
		case map[string]any:
			version, _ = v["version"].(string)
			if oses, ok := v["os"].([]any); ok && !slices.Contains(oses, any(osName)) {
				continue
			}
		}
		if version == "" {
			return nil, fmt.Errorf("tool %s: no version", name)
		}
		tools = append(tools, miseTool{Name: name, Version: version})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools, nil
}

// miseToolInstalled reports whether the tool is installed in the version
// mise uses for it. That is the pinned version, unless the user's own mise
// config (`mise use -g`) asks for another one: then that is the user's choice
// and only has to be installed. mise runs in the home directory, so a project's
// mise.toml in the current directory does not count.
func miseToolInstalled(tool miseTool) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	cmd := exec.Command("mise", "ls", tool.Name, "-J")
	cmd.Dir = home
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	var entries []struct {
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
		Active    bool   `json:"active"`
		Source    struct {
			Path string `json:"path"`
		} `json:"source"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return false
	}
	ours := filepath.Join(home, ".config", "mise", "conf.d", "dotfiles.toml")
	for _, e := range entries {
		if e.Active && e.Source.Path != "" && e.Source.Path != ours {
			return e.Installed
		}
	}
	for _, e := range entries {
		if e.Installed && (tool.Version == "latest" || e.Version == tool.Version) {
			return true
		}
	}
	return false
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

// renderConfig renders config.Source (plus its Extra files) with data and adds
// the managed header.
func renderConfig(config Config, data any) ([]byte, error) {
	contentBytes, err := templatesFS.ReadFile(config.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded file: %w", err)
	}
	for _, extra := range config.Extra {
		b, err := templatesFS.ReadFile(extra)
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded file: %w", err)
		}
		contentBytes = append(append(contentBytes[:len(contentBytes):len(contentBytes)], '\n'), b...)
	}

	tmpl, err := template.New(filepath.Base(config.Source)).Parse(string(contentBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var renderedBuffer bytes.Buffer
	if err := tmpl.Execute(&renderedBuffer, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}
	renderedContent := renderedBuffer.Bytes()
	if !config.NoHeader {
		renderedContent = addManagedHeader(renderedContent)
	}
	return renderedContent, nil
}

func cacheFileFor(config Config, cacheDir string) string {
	key := config.CacheKey
	if key == "" {
		key = config.Source
	}
	return filepath.Join(cacheDir, key)
}

func processConfig(config Config, data any, cacheDir string, command string, force bool) error {
	if config.Data != nil {
		data = config.Data
	}
	// 1. Render the template
	renderedContent, err := renderConfig(config, data)
	if err != nil {
		return err
	}

	cacheFile := cacheFileFor(config, cacheDir)

	if command == "diff" {
		return runDiffCommand(config.Destination, renderedContent, "Current File", "Template Render")
	}

	// command == "apply"

	// Ensure destination directory exists
	destDir := filepath.Dir(config.Destination)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", destDir, err)
	}
	// Create cache dir structure
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	newFile := config.Destination + ".new"
	// Lines appended to the destination, to be moved to config.LocalFile.
	var appendedTail []byte

	// 2. Check for local modifications if destination exists
	if fileInfo, err := os.Lstat(config.Destination); err == nil {
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			// A symlink (GNU stow, another dotfiles tool). Writing would change
			// the link target; move the link itself to the backup instead.
			if _, err := backupFile(cacheDir, config.Destination); err != nil {
				return err
			}
		} else if destBytes, err := os.ReadFile(config.Destination); err == nil && !bytes.Equal(destBytes, renderedContent) {
			// Destination differs from what we'd write. Only a problem if it also
			// diverges from the last applied baseline — otherwise it's just a
			// normal template update (or the cache is stale) and is safe to overwrite.
			if cacheBytes, err := os.ReadFile(cacheFile); err == nil {
				if !bytes.Equal(cacheBytes, destBytes) {
					if appendedTail, err = handleLocalModification(config, cacheDir, cacheBytes, destBytes, renderedContent, newFile, force); err != nil {
						return err
					}
				}
			} else if os.IsNotExist(err) {
				// Destination exists but no cache: a file dotfiles did not write.
				if config.NoAdopt && !force {
					return fmt.Errorf("%s exists and was not written by dotfiles. Move its settings to %s and delete it (or use --force to back it up and replace it)",
						config.Destination, adoptHint(config))
				}
				bak, err := backupFile(cacheDir, config.Destination)
				if err != nil {
					return err
				}
				if config.LocalFile != "" {
					fmt.Printf("   Your old %s was saved to %s. If it had settings you need (conda, nvm, ...), copy them to %s\n",
						tildePath(config.Destination), tildePath(bak), tildePath(config.LocalFile))
				}
			} else {
				return fmt.Errorf("read cache %s: %w", cacheFile, err)
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

	// Move appended lines to the local file BEFORE rewriting the destination:
	// if anything fails in between, the lines are still in the destination,
	// and appendToLocalFile skips a block that is already there.
	if len(appendedTail) > 0 {
		if err := appendToLocalFile(config.LocalFile, config.Destination, appendedTail); err != nil {
			return fmt.Errorf("move appended lines to %s: %w", config.LocalFile, err)
		}
		fmt.Printf("   Moved the lines appended to %s into %s\n", tildePath(config.Destination), tildePath(config.LocalFile))
	}

	if err := writeFileAtomic(config.Destination, renderedContent, mode); err != nil {
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

// handleLocalModification deals with a destination which differs from what
// dotfiles wrote last time (cacheBytes). Lines appended at the end (what
// installers do) are moved to config.LocalFile. Other edits are an error,
// unless force: then the edited file is backed up and replaced.
func handleLocalModification(config Config, cacheDir string, cacheBytes, destBytes, renderedContent []byte, newFile string, force bool) (appended []byte, err error) {
	if config.LocalFile != "" && bytes.HasPrefix(destBytes, cacheBytes) {
		tail := destBytes[len(cacheBytes):]
		if len(bytes.TrimSpace(tail)) > 0 {
			return tail, nil
		}
		return nil, nil
	}

	fmt.Printf("\n⚠️ LOCAL MODIFICATION DETECTED: %s\n", config.Destination)
	if err := os.WriteFile(newFile, renderedContent, 0644); err != nil {
		return nil, fmt.Errorf("failed to write %s for diffing: %w", newFile, err)
	}
	fmt.Printf("   To see diff execute:\n   diff %s %s\n", config.Destination, newFile)
	if !force {
		where := "the dotfiles templates"
		if config.LocalFile != "" {
			where = config.LocalFile + " (dotfiles never touches that file)"
		}
		return nil, fmt.Errorf("local modifications detected in %s. Move your changes to %s, or use --force to back up your version and overwrite it", config.Destination, where)
	}
	bak, err := backupFile(cacheDir, config.Destination)
	if err != nil {
		return nil, err
	}
	fmt.Printf("   Continuing anyway because --force was used. Your version was saved to %s\n", bak)
	return nil, nil
}

// appendToLocalFile appends lines which were added to dest to the user's own
// local file.
func appendToLocalFile(localFile, dest string, tail []byte) error {
	if err := os.MkdirAll(filepath.Dir(localFile), 0755); err != nil {
		return err
	}
	// Already moved by an earlier, interrupted apply: don't add it twice.
	if existing, err := os.ReadFile(localFile); err == nil &&
		bytes.HasSuffix(bytes.TrimRight(existing, "\n"), bytes.TrimRight(bytes.TrimLeft(tail, "\n"), "\n")) {
		return nil
	}
	f, err := os.OpenFile(localFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	block := fmt.Sprintf("\n# Moved here from %s by `dotfiles apply` on %s\n", tildePath(dest), time.Now().Format("2006-01-02"))
	block += strings.TrimLeft(string(tail), "\n")
	if !strings.HasSuffix(block, "\n") {
		block += "\n"
	}
	if _, err := f.WriteString(block); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// backupRoot is where dotfiles keeps copies of files it replaced.
func backupRoot(cacheDir string) string {
	return filepath.Join(filepath.Dir(cacheDir), "backup")
}

// backupPath is the backup location of the first version of path that
// dotfiles replaced (the one `dotfiles uninstall` restores).
func backupPath(cacheDir, path string) string {
	return filepath.Join(backupRoot(cacheDir), path)
}

// backupFile moves path below the backup directory, mirroring its absolute
// path. The first backup of a path keeps the plain name (uninstall restores
// it); later ones get a timestamp suffix. Symlinks are moved as links.
func backupFile(cacheDir, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	dst := backupPath(cacheDir, abs)
	if _, err := os.Lstat(dst); err == nil {
		dst += "." + time.Now().Format("20060102-150405.000000000")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return "", err
	}
	if err := os.Rename(abs, dst); err != nil {
		// Different file system: copy, then remove.
		info, lerr := os.Lstat(abs)
		if lerr != nil {
			return "", fmt.Errorf("back up %s: %w", abs, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(abs)
			if err != nil {
				return "", err
			}
			if err := os.Symlink(target, dst); err != nil {
				return "", err
			}
		} else {
			b, err := os.ReadFile(abs)
			if err != nil {
				return "", err
			}
			if err := os.WriteFile(dst, b, info.Mode().Perm()); err != nil {
				return "", err
			}
		}
		if err := os.Remove(abs); err != nil {
			return "", err
		}
	}
	fmt.Printf("   Backed up %s to %s\n", tildePath(abs), tildePath(dst))
	return dst, nil
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

// defaultShell is zsh if that is the login shell, otherwise bash.
func defaultShell() string {
	if filepath.Base(loginShell()) == "zsh" {
		return "zsh"
	}
	return "bash"
}

// loginShell returns the user's login shell from the passwd database, falling
// back to $SHELL.
func loginShell() string {
	u := os.Getenv("USER")
	if cur, err := user.Current(); err == nil {
		u = cur.Username
	}
	if u != "" {
		if out, err := exec.Command("getent", "passwd", u).Output(); err == nil {
			fields := strings.Split(strings.TrimSpace(string(out)), ":")
			if len(fields) >= 7 && fields[6] != "" {
				return fields[6]
			}
		}
	}
	return os.Getenv("SHELL")
}

func availableProfiles() []string {
	entries, _ := fs.ReadDir(templatesFS, "templates/profiles")
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}
