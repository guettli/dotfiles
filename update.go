package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const repoURL = "https://github.com/guettli/dotfiles"

// latestVersion returns the tag of the latest GitHub release. It reads the
// redirect of /releases/latest, which needs no API token and has no API rate
// limit.
func latestVersion() (string, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(repoURL + "/releases/latest")
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	i := strings.LastIndex(loc, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("no release found (%s)", resp.Status)
	}
	return loc[i+len("/tag/"):], nil
}

// parseSemver parses "v1.2.3" (pre-release suffixes are ignored).
func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// isNewer reports whether latest is a newer release than current. A current
// version which is not a release (dev build, go run) is never outdated.
func isNewer(latest, current string) bool {
	l, ok1 := parseSemver(latest)
	c, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func runUpdate(args []string) int {
	fs := newFlagSet("update")
	noApply := fs.Bool("no-apply", false, "only update the program, do not run apply")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		fmt.Printf("❌ cannot find the running program: %v\n", err)
		return 1
	}
	if strings.Contains(exe, "go-build") {
		fmt.Println("You run dotfiles with `go run`. That always uses the newest version:")
		fmt.Println("    go run github.com/guettli/dotfiles@latest apply")
		return 0
	}

	latest, err := latestVersion()
	if err != nil {
		fmt.Printf("❌ could not find the latest release: %v\n", err)
		return 1
	}
	current := getVersion()
	if !isNewer(latest, current) && current != "dev" {
		fmt.Printf("✅ dotfiles %s is the newest version.\n", current)
	} else {
		fmt.Printf("⬇️  Updating dotfiles %s -> %s\n", current, latest)
		if err := downloadRelease(latest, exe); err != nil {
			fmt.Printf("❌ %v\n", err)
			return 1
		}
		fmt.Printf("✅ Updated %s\n", exe)
	}
	if *noApply {
		return 0
	}
	// Apply with the new program, so the new templates are used.
	cmd := exec.Command(exe, "apply")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Printf("❌ %v\n", err)
		return 1
	}
	return 0
}

// archiveName is the release asset for this platform (see .goreleaser.yaml).
func archiveName() string {
	return fmt.Sprintf("dotfiles_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// downloadRelease fetches the release archive for tag, checks it against the
// release's checksums.txt and replaces exe with the binary inside.
func downloadRelease(tag, exe string) error {
	base := repoURL + "/releases/download/" + tag + "/"
	name := archiveName()
	archive, err := download(base + name)
	if err != nil {
		return err
	}
	sums, err := download(base + "checksums.txt")
	if err != nil {
		return err
	}
	if err := verifyChecksum(sums, name, archive); err != nil {
		return err
	}
	bin, err := extractBinary(archive, "dotfiles")
	if err != nil {
		return err
	}
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, bin, 0755); err != nil {
		return err
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func verifyChecksum(sums []byte, name string, data []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[1] == name {
			if fields[0] != got {
				return fmt.Errorf("checksum mismatch for %s", name)
			}
			return nil
		}
	}
	return fmt.Errorf("%s not listed in checksums.txt", name)
}

func extractBinary(archive []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("%s not found in the release archive", name)
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == name && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(io.LimitReader(tr, 200<<20))
		}
	}
}
