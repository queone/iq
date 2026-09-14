package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"iq/internal/usage/usagetest"
)

func TestHelpFlagsPrintIdenticalPages(t *testing.T) {
	want, stderr := usagetest.RunCLI(t, programName, runCLI, "-h")
	if stderr != "" {
		t.Fatalf("-h wrote to stderr: %q", stderr)
	}
	usagetest.Shape(t, want, programName, programVersion, programURL)
	for _, args := range [][]string{{"-?"}, {"--help"}, {"help"}, {}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			got, stderr := usagetest.RunCLI(t, programName, runCLI, args...)
			if got != want || stderr != "" {
				t.Fatalf("output differs from -h (stderr=%q):\n%s", stderr, got)
			}
		})
	}
}

func TestHelpCommandRoutesToSubcommand(t *testing.T) {
	want, _ := usagetest.RunCLI(t, programName, runCLI, "cue", "list", "-h")
	for _, args := range [][]string{{"help", "cue", "list"}, {"-h", "cue", "list"}, {"cue", "list", "-?"}} {
		got, stderr := usagetest.RunCLI(t, programName, runCLI, args...)
		if got != want || stderr != "" {
			t.Errorf("%q differs from 'cue list -h' (stderr=%q):\n%s", args, stderr, got)
		}
	}
	if !strings.Contains(want, "\nUsage\n  iq cue list [options]\n") {
		t.Errorf("cue list help lacks its synopsis:\n%s", want)
	}
}

func TestVersionFlagWorksOnEveryCommand(t *testing.T) {
	want := programName + " v" + programVersion + "\n"
	for _, args := range [][]string{{"-v"}, {"--version"}, {"version"}, {"ver"}, {"doc", "-v"}, {"cue", "list", "--version"}} {
		got, stderr := usagetest.RunCLI(t, programName, runCLI, args...)
		if got != want || stderr != "" {
			t.Errorf("%q: stdout=%q stderr=%q, want %q", args, got, stderr, want)
		}
	}
}

func TestEveryCommandPageSharesHeaderAndListsFlags(t *testing.T) {
	root := newRootCmd()
	header := strings.SplitN(usagetest.CaptureStdout(t, func() { root.Help() }), "\n", 4)[:3]
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		out := usagetest.CaptureStdout(t, func() { cmd.Help() })
		lines := strings.SplitN(out, "\n", 4)
		if len(lines) < 4 || strings.Join(lines[:3], "\n") != strings.Join(header, "\n") {
			t.Errorf("%s: header differs from root:\n%s", cmd.CommandPath(), out)
		}
		if !strings.Contains(out, "\nUsage\n  "+cmd.CommandPath()+" ") {
			t.Errorf("%s: synopsis missing:\n%s", cmd.CommandPath(), out)
		}
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if !strings.Contains(out, "--"+f.Name) {
				t.Errorf("%s: flag --%s missing from help", cmd.CommandPath(), f.Name)
			}
			if f.Shorthand == "" && f.Name != "version" && f.Name != "help" {
				t.Errorf("%s: flag --%s has no one-letter short form", cmd.CommandPath(), f.Name)
			}
		})
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
