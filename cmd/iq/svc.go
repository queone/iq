package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"iq/internal/color"
	"iq/internal/config"
	"iq/internal/model"
	"iq/internal/sidecar"
)

// loadConfig loads config.yaml and prints a schema migration notice once.
func loadConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.MigrationNotice != "" {
		fmt.Fprintln(os.Stderr, color.Yel5(cfg.MigrationNotice))
	}
	return cfg, nil
}

// resolveModel returns the model to act on: the argument, else the configured
// model, else the catalog pick, which is written to config.yaml.
func resolveModel(arg string, cfg *config.Config, out io.Writer) (string, error) {
	if arg != "" {
		return arg, nil
	}
	if cfg.Model != "" {
		return cfg.Model, nil
	}
	e, mem, err := pickEntry()
	if err != nil {
		return "", err
	}
	fmt.Fprintf(out, "no model configured; picked %s for %s of memory\n", e.ID, model.FormatGB(mem))
	applyPick(cfg, e)
	if err := config.Save(cfg); err != nil {
		return "", fmt.Errorf("saving config.yaml: %w", err)
	}
	return e.ID, nil
}

// ensureSnapshot returns the model's local snapshot path, downloading it with
// the hf CLI when the Hugging Face cache does not have it yet.
func ensureSnapshot(modelID string, out io.Writer) (string, error) {
	if path, err := model.SnapshotDir(modelID); err == nil {
		return path, nil
	}
	fmt.Fprintf(out, "  %s is not in the Hugging Face cache; downloading with hf\n", modelID)
	if err := model.Download(modelID, out); err != nil {
		return "", err
	}
	return model.SnapshotDir(modelID)
}

// startSidecar resolves the model and Python paths and delegates to sidecar.StartInfer.
func startSidecar(modelID string, out io.Writer) error {
	path, err := ensureSnapshot(modelID, out)
	if err != nil {
		return fmt.Errorf("cannot resolve model path: %w", err)
	}
	py, err := sidecar.MlxVenvPython()
	if err != nil {
		return fmt.Errorf("cannot resolve Python interpreter: %w", err)
	}
	_, err = sidecar.StartInfer(modelID, path, py)
	return err
}

// stopAll stops the given model's sidecar and sweeps orphaned servers.
func stopModel(modelID string, sweep bool) {
	if modelID != "" {
		if err := sidecar.Stop(modelID); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", modelID, err.Error())
		}
	}
	if sweep {
		sidecar.KillOrphanSidecars()
	}
}

// ── start / stop / restart ────────────────────────────────────────────────────

func newStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "start [model]",
		Short:        "Start the model's mlx_lm.server sidecar, picking and downloading the model when needed; writes run state",
		SilenceUsage: true,
		Args:         argsUsage(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			modelID, err := resolveModel(arg, cfg, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if cfg.MigrationNotice != "" {
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("saving migrated config.yaml: %w", err)
				}
			}
			if state, _ := sidecar.ReadState(modelID); state != nil && sidecar.PidAlive(state.PID) {
				fmt.Printf("  pid %-7d  %s  %s\n", state.PID, sidecar.Endpoint(state.Port), color.Gra5("already running"))
				return nil
			}
			return startSidecar(modelID, cmd.OutOrStdout())
		},
	}
}

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "stop [model]",
		Short:        "Stop the model's sidecar and sweep orphaned servers; writes run state",
		SilenceUsage: true,
		Args:         argsUsage(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			modelID := cfg.Model
			if len(args) > 0 {
				modelID = args[0]
			}
			stopModel(modelID, len(args) == 0)
			return nil
		},
	}
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "restart [model]",
		Short:        "Stop then start the model's sidecar; writes run state",
		SilenceUsage: true,
		Args:         argsUsage(cobra.MaximumNArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			arg := ""
			if len(args) > 0 {
				arg = args[0]
			}
			modelID, err := resolveModel(arg, cfg, cmd.OutOrStdout())
			if err != nil {
				return err
			}
			stopModel(modelID, arg == "")
			return startSidecar(modelID, cmd.OutOrStdout())
		},
	}
}

// ── status ────────────────────────────────────────────────────────────────────

// newStatusCmd returns the `iq status` / `iq st` command.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "status",
		Aliases:      []string{"st"},
		Short:        "Show running sidecars and memory use",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return printStatus()
		},
	}
}

// formatMB renders bytes as whole megabytes with thousands separators.
func formatMB(bytes int64) string {
	mb := bytes / (1024 * 1024)
	s := fmt.Sprintf("%d", mb)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s + "MB"
}

func printStatus() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	type statusRow struct {
		model, endpoint string
		pid             int
		uptime          string
		running         bool
		mem             string
	}
	var rows []statusRow
	var totalKB int64
	live, _ := sidecar.AllLiveStates()
	configuredSeen := false
	for _, st := range live {
		rss := sidecar.ProcessRSSKB(st.PID)
		totalKB += rss
		mem := formatMB(rss * 1024)
		if rss == 0 {
			mem = "?"
		}
		rows = append(rows, statusRow{st.Model, sidecar.Endpoint(st.Port), st.PID, sidecar.FormatUptime(st.Started), true, mem})
		if st.Model == cfg.Model {
			configuredSeen = true
		}
	}
	if cfg.Model != "" && !configuredSeen {
		rows = append(rows, statusRow{cfg.Model, "", 0, "—", false, "—"})
	}
	cfgPath, _ := config.Path()
	fmt.Printf("CONFIG  %s\n", cfgPath)
	if len(rows) == 0 {
		fmt.Println("No model configured and no sidecar running — run: iq pick -w && iq start")
		return nil
	}
	modelW := len("MODEL")
	for _, r := range rows {
		modelW = max(modelW, len(r.model))
	}
	modelW += 2
	fmt.Printf("%-*s  %-28s  %-7s  %-8s  %-7s  %8s\n", modelW, "MODEL", "ENDPOINT", "PID", "UPTIME", "RUNNING", "MEM")
	for _, r := range rows {
		// Pad before colorizing — ANSI codes inflate len() and break alignment.
		runDisplay := color.Gra5(fmt.Sprintf("%-7s", "no"))
		if r.running {
			runDisplay = color.Grn5(fmt.Sprintf("%-7s", "yes"))
			fmt.Printf("%-*s  %-28s  %-7d  %-8s  %s  %8s\n", modelW, r.model, r.endpoint, r.pid, r.uptime, runDisplay, r.mem)
			continue
		}
		fmt.Printf("%-*s  %-28s  %-7s  %-8s  %s  %8s\n", modelW, r.model, r.endpoint, "—", r.uptime, runDisplay, r.mem)
	}
	iqRSS := sidecar.ProcessRSSKB(os.Getpid())
	totalKB += iqRSS
	lineW := modelW + 2 + 28 + 2 + 7 + 2 + 8 + 2 + 7 + 2 + 8
	left := fmt.Sprintf("%-20s %s", "IQ process mem:", formatMB(iqRSS*1024))
	right := fmt.Sprintf("%s  %8s", "Total mem:", formatMB(totalKB*1024))
	gap := max(lineW-len(left)-len(right), 2)
	fmt.Printf("%s%s%s\n", left, strings.Repeat(" ", gap), right)
	return nil
}

// ── doc ───────────────────────────────────────────────────────────────────────

type docCheck struct {
	label, detail string
	ok, warn      bool
}

// runDocChecks probes every runtime dependency and the configured model.
func runDocChecks(cfg *config.Config) []docCheck {
	var checks []docCheck
	add := func(label, detail string, ok, warn bool) {
		checks = append(checks, docCheck{label, detail, ok, warn})
	}
	add("apple silicon", runtime.GOOS+"/"+runtime.GOARCH, runtime.GOOS == "darwin" && runtime.GOARCH == "arm64", false)
	if p, err := exec.LookPath("python3"); err == nil {
		add("python3", p, true, false)
	} else {
		add("python3", "not found", false, false)
	}
	if serverPath, err := sidecar.MlxServerPath(); err != nil {
		add("mlx_lm.server", "not found — install with: pipx install mlx-lm", false, false)
	} else {
		add("mlx_lm.server", serverPath, true, false)
		help, _ := exec.Command(serverPath, "--help").CombinedOutput()
		for _, flag := range []string{"--model", "--chat-template-args"} {
			has := strings.Contains(string(help), flag)
			detail := flag + " flag supported"
			if !has {
				detail = flag + " flag not found — upgrade mlx_lm to 0.30.7 or newer"
			}
			add("  "+flag+" flag", detail, has, false)
		}
	}
	if p, err := exec.LookPath("hf"); err == nil {
		add("hf", p, true, false)
	} else {
		add("hf", "not found — install with: pipx install huggingface_hub", false, false)
	}
	switch {
	case cfg.Model == "":
		add("model", "none configured — run: iq pick -w", true, true)
	default:
		if path, err := model.SnapshotDir(cfg.Model); err == nil {
			add("model", path, true, false)
		} else {
			add("model", cfg.Model+" is not in the cache — iq start downloads it", true, true)
		}
	}
	return checks
}

func newDocCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "doc",
		Short:        "Check runtime dependencies and model readiness",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			checks := runDocChecks(cfg)
			fmt.Printf("%-28s %-7s %s\n", "CHECK", "STATUS", "DETAIL")
			failed := false
			for _, c := range checks {
				status := color.Grn5("ok     ")
				switch {
				case !c.ok:
					status = color.Yel5("FAIL   ")
					failed = true
				case c.warn:
					status = color.Yel5("warn   ")
				}
				fmt.Printf("%-28s %s %s\n", c.label, status, color.Gra5(c.detail))
			}
			if failed {
				return errors.New("some checks failed")
			}
			fmt.Println("All checks passed.")
			return nil
		},
	}
}
