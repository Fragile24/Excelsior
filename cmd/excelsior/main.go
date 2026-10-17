package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"excelsior/internal/app"
	"excelsior/internal/chat"
	"excelsior/internal/permissions"
	"excelsior/pkg/agent"
	"excelsior/pkg/config"
	"excelsior/pkg/engine"
	"excelsior/pkg/llm"
	"excelsior/pkg/protocol"
	"excelsior/pkg/tools"
	"excelsior/pkg/util"
)

// version is set via ldflags: go build -ldflags "-X main.version=v1.2.3"
var version = "v0.1.0"

func main() {
	setupLogger()
	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("invalid environment configuration", "err", err)
		os.Exit(1)
	}
	root := newRootCommand(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}

func newRootCommand(cfg config.Config) *cobra.Command {
	var model, workspace, system, sessionID, engineURL, permission string
	var verbose bool
	// permissionOverride is a runtime-only CLI override (never persisted);
	// persisted permission lives in Settings (workspace settings.json).
	var permissionOverride config.PermissionMode
	root := &cobra.Command{
		Use:   "excelsior",
		Short: "Excelsior — DeepSeek-native coding agent (Go)",
		Long: `Excelsior is a DeepSeek-first coding agent library + CLI.

Examples:
  excelsior engine                     # start the WebSocket engine (desktop/mobile clients)
  excelsior "add tests for pkg/tools"
  excelsior -m deepseek-v4-pro "explain this repo"
  echo "fix the bug in main.go" | excelsior
  excelsior --permission allow "run all commands without asking"`,
		Args: cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if verbose {
				slog.SetLogLoggerLevel(slog.LevelDebug)
			}
			// Runtime-only permission override. Resolved with
			// LoadSettings (env-seeded) via permissions.Resolve at use sites.
			if permission != "" {
				pm, err := config.ParsePermissionMode(permission)
				if err != nil {
					return err
				}
				permissionOverride = pm
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := resolvePrompt(args)
			applyHarnessFlags(&cfg, cmd)
			if prompt == "" {
				return cmd.Help()
			}
			if engineURL != "" {
				token := os.Getenv("EXCELSIOR_ENGINE_TOKEN")
				if token == "" {
					remote, err := url.Parse(engineURL)
					if err != nil {
						return err
					}
					switch remote.Hostname() {
					case "localhost", "127.0.0.1", "::1":
					default:
						return fmt.Errorf("set EXCELSIOR_ENGINE_TOKEN for a remote engine")
					}
					token, err = engine.OwnerToken(false)
					if err != nil {
						return err
					}
				}
				client := engine.WSClient{URL: engineURL, Token: token, Workspace: workspace}
				return client.StreamRemote(cmd.Context(), protocol.ChatReq{SessionID: sessionID, Model: normalizeModel(model, cfg.Model), Messages: []llm.Message{{Role: "user", Content: prompt}}}, func(d protocol.Delta) error {
					chatEventPrinter(chat.Event{Type: d.Type, Text: d.Text, Reasoning: d.Reasoning, ToolName: d.ToolName, ToolResult: d.ToolResult})
					return nil
				}, nil, nil)
			}
			return runAgent(cmd.Context(), cfg, permissionOverride, model, workspace, system, sessionID, prompt)
		},
	}
	root.PersistentFlags().StringVarP(&model, "model", "m", "", "DeepSeek model (deepseek-v4-flash, deepseek-v4-pro)")
	root.PersistentFlags().StringVarP(&workspace, "workspace", "w", "", "Workspace root (default: cwd)")
	root.PersistentFlags().StringVar(&system, "system", "", "Override system prompt")
	root.PersistentFlags().StringVar(&sessionID, "session", "", "Session ID for persistence (.excelsior/sessions)")
	root.PersistentFlags().StringVar(&engineURL, "engine", "", "WebSocket engine URL (e.g. ws://localhost:17812/v1/ws)")
	root.PersistentFlags().StringVar(&permission, "permission", "", "Permission mode: ask|allow|deny (default ask, overrides EXCELSIOR_PERMISSION)")
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose logging (debug)")
	root.PersistentFlags().Int("context-tier", 0, "Context-management tier 0-4 (default: env EXCELSIOR_CONTEXT_TIER or 4)")
	root.PersistentFlags().Int("context-window", 0, "Total input + output context tokens (default: env EXCELSIOR_CONTEXT_WINDOW or 128000)")
	root.PersistentFlags().String("token-encoding", "", "Prompt/tokenizer profile: deepseek-v4 or deepseek-v4.1 (default: infer known model; env EXCELSIOR_TOKEN_ENCODING)")
	root.PersistentFlags().Bool("planning", false, "Enable planning scaffold (default: env EXCELSIOR_PLANNING or true)")
	root.PersistentFlags().String("action-space", "", "Action space: predefined or bash (default: env EXCELSIOR_ACTION_SPACE or predefined)")

	root.AddCommand(newEngineCommand(cfg, &workspace))
	root.AddCommand(newModelsCommand())
	root.AddCommand(newVersionCommand())
	return root
}

func applyHarnessFlags(cfg *config.Config, cmd *cobra.Command) {
	settings := config.HarnessSettings{ContextWindowTokens: 128000, ContextTier: 4, Planning: true, ActionSpace: "predefined"}
	if cfg.Harness != nil {
		settings = *cfg.Harness
	}
	flags := cmd.Root().PersistentFlags()
	if flags.Changed("token-encoding") {
		cfg.TokenEncoding, _ = flags.GetString("token-encoding")
	}
	if flags.Changed("context-tier") {
		settings.ContextTier, _ = flags.GetInt("context-tier")
	}
	if flags.Changed("context-window") {
		settings.ContextWindowTokens, _ = flags.GetInt("context-window")
	}
	if flags.Changed("planning") {
		settings.Planning, _ = flags.GetBool("planning")
	}
	if flags.Changed("action-space") {
		settings.ActionSpace, _ = flags.GetString("action-space")
	}
	cfg.Harness = &settings
}

func newModelsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "models", Short: "List recommended DeepSeek models",
		Run: func(cmd *cobra.Command, args []string) {
			for _, line := range []string{
				"deepseek-v4-flash  — V4 Flash, fast, tool-calling (default)",
				"deepseek-v4-pro    — V4 Pro, reasoning",
			} {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
		},
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use: "version", Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "excelsior %s (go + deepseek-native)\n", version)
		},
	}
}

func resolvePrompt(args []string) string {
	if p := strings.TrimSpace(strings.Join(args, " ")); p != "" {
		return p
	}
	if stat, _ := os.Stdin.Stat(); stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		if b, err := io.ReadAll(os.Stdin); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return ""
}

func setupLogger() {
	level := slog.LevelInfo
	if v := strings.ToLower(os.Getenv("EXCELSIOR_LOG_LEVEL")); v == "debug" {
		level = slog.LevelDebug
	}
	var handler slog.Handler
	if strings.ToLower(os.Getenv("EXCELSIOR_LOG")) == "json" {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(handler))
}

func runAgent(ctx context.Context, cfg config.Config, override config.PermissionMode, model, workspace, system, sessionID, prompt string) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	var err error
	workspace, err = config.ResolveWorkspace(workspace, cfg.Workspace)
	if err != nil {
		return err
	}
	model = normalizeModel(model, cfg.Model)
	if system == "" {
		system = agent.DefaultSystemPrompt
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return fmt.Errorf("prompt is empty")
	}
	if len(prompt) > 200_000 {
		return fmt.Errorf("prompt too large (%d > 200000 chars)", len(prompt))
	}

	// Permission: flag override > workspace settings.json > user global > env > ask.
	perm := permissions.Resolve(override, config.LoadSettings(workspace))
	// Permission handler for headless CLI: resolved mode, once per call, sequential.
	var permHandler tools.PermissionHandler
	switch perm {
	case config.PermissionAllow:
		permHandler = func(ctx context.Context, req tools.PermissionRequest) (tools.PermissionResponse, error) {
			return tools.PermissionResponse{Approved: true}, nil
		}
	case config.PermissionDeny:
		permHandler = func(ctx context.Context, req tools.PermissionRequest) (tools.PermissionResponse, error) {
			return tools.PermissionResponse{Approved: false}, nil
		}
	default: // ask but no TUI in headless mode: deny with hint
		permHandler = func(ctx context.Context, req tools.PermissionRequest) (tools.PermissionResponse, error) {
			if permissions.DefaultDecision(req) == config.PermissionAllow {
				return tools.PermissionResponse{Approved: true}, nil
			}
			slog.Warn("permission denied (no TUI, use --permission allow)", "tool", req.Tool, "file", req.FilePath, "command", req.Command)
			return tools.PermissionResponse{Approved: false}, nil
		}
	}
	ag := app.NewAgent(cfg, workspace, model, system, slog.Default())

	messages := []llm.Message{{Role: "user", Content: prompt}}
	slog.Info("agent run", "model", model, "workspace", workspace, "session", sessionID, "permission", perm)

	coord := chat.NewCoordinator(chat.Config{
		NewAgent:  func(string, string) (agent.Runner, error) { return ag, nil },
		Ephemeral: sessionID == "", PermissionHandler: permHandler,
		QuestionHandler: func(context.Context, tools.AskRequest) (tools.AskResponse, error) {
			return tools.AskResponse{Selected: -1}, fmt.Errorf("question needs an interactive client")
		},
	})
	defer coord.Close()
	if _, _, err := coord.Run(ctx, chat.StartCommand{Workspace: workspace, SessionID: sessionID, Model: model, Messages: messages}, chatEventPrinter); err != nil {
		return fmt.Errorf("agent: %w", err)
	}
	fmt.Fprintln(os.Stderr, "")
	return nil
}

func normalizeModel(flagModel, cfgModel string) string {
	m := strings.TrimSpace(flagModel)
	if m == "" {
		m = strings.TrimSpace(cfgModel)
	}
	if m == "" {
		m = config.DefaultModel
	}
	return m
}

func chatEventPrinter(ev chat.Event) {
	switch ev.Type {
	case "reasoning":
		fmt.Fprint(os.Stderr, ev.Reasoning)
	case "text":
		fmt.Fprint(os.Stdout, ev.Text)
	case "tool_start":
		if ev.ToolName != "" {
			fmt.Fprintf(os.Stderr, "\n[tool:%s]\n", ev.ToolName)
		}
	case "tool_result":
		fmt.Fprintf(os.Stderr, "[%s result: %s]\n", ev.ToolName, util.Truncate(ev.ToolResult, 400))
	case "error":
		fmt.Fprintf(os.Stderr, "\n[error: %s]\n", ev.Text)
	case "done":
		fmt.Fprintln(os.Stderr, "")
	}
}
