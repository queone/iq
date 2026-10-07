package main

import (
	"strings"
	"testing"

	"iq/internal/config"
	"iq/internal/model"
)

func TestApplyPickSetsModelLimitsAndTemplateArgs(t *testing.T) {
	cfg := &config.Config{Model: "old", MaxTokens: 4096}
	e := model.Entry{ID: "org/new", ContextWindow: 32768, ChatTemplateArgs: map[string]any{"enable_thinking": false}}
	applyPick(cfg, e)
	if cfg.Model != "org/new" || cfg.ContextWindow != 32768 || cfg.MaxTokens != 4096 {
		t.Errorf("cfg = %+v", cfg)
	}
	if got := cfg.ChatTemplateArgsJSON("org/new"); got != `{"enable_thinking":false}` {
		t.Errorf("ChatTemplateArgsJSON = %q", got)
	}
}

func TestFormatPickNamesBudgetAndEstimate(t *testing.T) {
	e := model.Entry{ID: "org/new", DiskGB: 2.8, ContextWindow: 32768, Notes: "seed"}
	out := formatPick(e, 24*(1<<30))
	for _, want := range []string{"memory     25.8 GB", "budget     18.0 GB (70% of memory)", "model      org/new", "2.8 GB on disk, about 5.2 GB in memory", "context    32768", "notes      seed"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
