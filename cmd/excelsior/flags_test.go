package main

import (
	"testing"

	"excelsior/pkg/config"
)

func TestHarnessFlagsAreLocalToEachCommand(t *testing.T) {
	first, second := newRootCommand(config.Config{}), newRootCommand(config.Config{})
	if err := first.PersistentFlags().Parse([]string{"--context-tier=0", "--planning=false", "--context-window=64000", "--action-space=bash"}); err != nil {
		t.Fatal(err)
	}
	if err := second.PersistentFlags().Parse([]string{"--context-tier=2", "--planning=true"}); err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	applyHarnessFlags(&cfg, first)
	want := config.HarnessSettings{ContextTier: 0, Planning: false, ContextWindowTokens: 64000, ActionSpace: "bash"}
	if *cfg.Harness != want {
		t.Fatalf("first command: %+v; want %+v", *cfg.Harness, want)
	}
	cfg = config.Config{}
	applyHarnessFlags(&cfg, second)
	want = config.HarnessSettings{ContextTier: 2, Planning: true, ContextWindowTokens: 128000, ActionSpace: "predefined"}
	if *cfg.Harness != want {
		t.Fatalf("second command: %+v; want %+v", *cfg.Harness, want)
	}
}
