package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"excelsior/internal/app"
	"excelsior/pkg/agent"
	"excelsior/pkg/config"
	"excelsior/pkg/engine"
	"excelsior/pkg/session"
)

func newEngineCommand(cfg config.Config, workspaceFlag *string) *cobra.Command {
	var addr string
	var parentPipe bool
	var origins []string
	cmd := &cobra.Command{
		Use:   "engine",
		Short: "Start WebSocket engine hub (for desktop/mobile clients)",
		Long: `Start the engine daemon. Clients connect via WebSocket:

  excelsior engine --workspace .`,
		RunE: func(cmd *cobra.Command, args []string) error {
			applyHarnessFlags(&cfg, cmd)
			// Runtime-only permission override; persisted
			// permission is resolved per-request from LoadSettings.
			var override config.PermissionMode
			if v, _ := cmd.Root().PersistentFlags().GetString("permission"); v != "" {
				if pm, err := config.ParsePermissionMode(v); err == nil {
					override = pm
				}
			}
			ws, err := config.ResolveWorkspace(*workspaceFlag, cfg.Workspace)
			if err != nil {
				return err
			}
			if addr == "" {
				addr = "127.0.0.1:17812"
			}
			// Validate config (needs API key for LLM)
			if err := cfg.Validate(); err != nil {
				if !errors.Is(err, config.ErrMissingAPIKey) {
					return err
				}
				slog.Warn("engine config", "err", err)
			}
			h := engine.NewHub(cfg, ws)
			h.NewAgent = func(model, workspace string) (agent.Runner, error) {
				return app.NewAgent(cfg, workspace, normalizeModel(model, cfg.Model), agent.DefaultSystemPrompt, slog.Default()), nil
			}
			if err := session.CheckLease(h.Coordinator().Store(ws)); err != nil {
				return err
			}
			h.Addr = addr
			h.PermissionOverride = override
			token, err := engine.OwnerToken(false)
			if err != nil {
				return fmt.Errorf("owner token: %w", err)
			}
			h.Token, h.AllowedOrigins = token, origins
			slog.Info("starting engine", "addr", addr, "workspace", ws, "model", cfg.Model)
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			if parentPipe {
				go func() { _, _ = io.Copy(io.Discard, os.Stdin); cancel() }()
			}
			return h.ListenAndServe(ctx)
		},
	}
	cmd.Flags().BoolVar(&parentPipe, "parent-pipe", false, "Stop when the owning desktop closes stdin")
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:17812", "Listen address for WS hub (e.g. :17812)")
	cmd.Flags().StringSliceVar(&origins, "origin", []string{"null", "excelsior://desktop", "http://localhost:3000"}, "Allowed browser origins (repeat or comma-separate)")
	cmd.AddCommand(&cobra.Command{Use: "token", Short: "Print the owner token (keep private)", RunE: func(cmd *cobra.Command, args []string) error {
		token, err := engine.OwnerToken(false)
		if err == nil {
			fmt.Fprintln(cmd.OutOrStdout(), token)
		}
		return err
	}})
	cmd.AddCommand(&cobra.Command{Use: "rotate-token", Short: "Rotate owner token; restart engine to revoke connections", RunE: func(cmd *cobra.Command, args []string) error {
		_, err := engine.OwnerToken(true)
		if err == nil {
			fmt.Fprintln(cmd.OutOrStdout(), "Token rotated. Restart the engine and reconnect clients.")
		}
		return err
	}})
	return cmd
}
