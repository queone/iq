// Package usagetest holds test helpers that check a utility's help output against the shared
// CLI usage contract enforced by build.sh before installation.
package usagetest

import (
	"io"
	"os"
	"strings"
	"testing"
)

// RunCLI runs run with os.Args set to name plus args and returns the captured stdout and stderr.
func RunCLI(t testing.TB, name string, run func(), args ...string) (string, string) {
	t.Helper()

	oldArgs, oldStdout, oldStderr := os.Args, os.Stdout, os.Stderr
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		stdoutR.Close()
		stdoutW.Close()
		t.Fatalf("create stderr pipe: %v", err)
	}

	os.Args = append([]string{name}, args...)
	os.Stdout, os.Stderr = stdoutW, stderrW
	run()
	os.Args, os.Stdout, os.Stderr = oldArgs, oldStdout, oldStderr
	stdoutW.Close()
	stderrW.Close()

	stdout, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	stderr, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	stdoutR.Close()
	stderrR.Close()
	return string(stdout), string(stderr)
}

// CaptureStdout returns what fn wrote to os.Stdout.
func CaptureStdout(t testing.TB, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	r.Close()
	return string(out)
}

// Shape fails the test when out breaks the help-page contract: exact header lines, a blank
// line four, Usage on line five, ordered one-word section headings, indented bodies, an
// Options section, and no escape sequence.
func Shape(t testing.TB, out, name, version, url string) {
	t.Helper()
	if strings.Contains(out, "\x1b") {
		t.Errorf("help carries an escape sequence without a terminal")
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 6 {
		t.Fatalf("help has %d lines, want at least 6", len(lines))
	}
	if want := name + " v" + version; lines[0] != want {
		t.Errorf("line 1 = %q, want %q", lines[0], want)
	}
	if lines[1] == "" || strings.HasSuffix(lines[1], ".") {
		t.Errorf("line 2 = %q, want a one-line description with no trailing period", lines[1])
	}
	if lines[2] != url {
		t.Errorf("line 3 = %q, want %q", lines[2], url)
	}
	if lines[3] != "" {
		t.Errorf("line 4 = %q, want blank", lines[3])
	}
	if lines[4] != "Usage" {
		t.Errorf("line 5 = %q, want Usage", lines[4])
	}
	rank := map[string]int{"Usage": 1, "Commands": 2, "Options": 3, "Examples": 4}
	last, seenOptions := 0, false
	for _, line := range lines[4:] {
		if line == "" || strings.HasPrefix(line, "  ") {
			continue
		}
		r, ok := rank[line]
		if !ok {
			t.Errorf("help text outside a section: %q", line)
			continue
		}
		if r <= last {
			t.Errorf("section %s is out of order", line)
		}
		last = r
		if line == "Options" {
			seenOptions = true
		}
	}
	if !seenOptions {
		t.Errorf("help has no Options section")
	}
}
