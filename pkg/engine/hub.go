package engine

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"excelsior/internal/chat"
	"excelsior/pkg/agent"
	"excelsior/pkg/config"
	"excelsior/pkg/session"
)

// Hub is the WS daemon. One Hub serves many clients; run lifecycle lives in the
// chat coordinator. The hub owns authentication, connections, and wire mapping.
type Hub struct {
	Addr           string // e.g. :17812
	Config         config.Config
	Logger         *slog.Logger
	NewAgent       func(model, workspace string) (agent.Runner, error) // injectable (defaults to built-in)
	SessionStore   session.Store                                       // Injectable store for tests and embedded use.
	Token          string
	AllowedOrigins []string
	// PermissionOverride is a runtime-only CLI override.
	// Persisted permission lives in workspace settings (env-seeded).
	PermissionOverride config.PermissionMode

	mu          sync.RWMutex
	clients     map[*Conn]struct{}
	stopped     bool
	workspace   string
	coordinator *chat.Coordinator
}

// Coordinator returns the transport-neutral coordinator instance for this hub.
func (h *Hub) Coordinator() *chat.Coordinator {
	return h.coordinator
}

// NewHub initializes a Hub with configuration and workspace.
func NewHub(cfg config.Config, workspace string) *Hub {
	if workspace == "" {
		workspace = cfg.Workspace
	}
	h := &Hub{
		Config:    cfg,
		Addr:      "127.0.0.1:17812",
		Logger:    slog.Default(),
		clients:   make(map[*Conn]struct{}),
		workspace: canonicalWorkspace(workspace),
	}
	h.coordinator = chat.NewCoordinator(chat.Config{
		NewAgent: func(model, workspace string) (agent.Runner, error) {
			if h.NewAgent != nil {
				return h.NewAgent(model, workspace)
			}
			return nil, errors.New("no agent factory supplied by application startup")
		},
		StoreFactory: func(workspace string) session.Store {
			if h.SessionStore != nil {
				return h.SessionStore
			}
			return session.NewDirStore(filepath.Join(workspace, ".excelsior", "sessions"))
		},
		PermissionOverrideFunc: func() config.PermissionMode {
			return h.PermissionOverride
		},
		Logger: h.logger(),
	})
	return h
}

func (h *Hub) logger() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Workspace returns the current workspace root directory.
func (h *Hub) Workspace() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.workspace
}

// Close stops runs and sockets. Client disconnects never call this.
func (h *Hub) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Shutdown(ctx); err != nil {
		h.logger().Error("engine shutdown incomplete", "error", err)
	}
}

func (h *Hub) Shutdown(ctx context.Context) error {
	h.mu.Lock()
	h.stopped = true
	clients := make([]*Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	err := h.Coordinator().Shutdown(ctx)
	for _, c := range clients {
		c.close()
	}
	return err
}

// Register registers a connection with the hub.
func (h *Hub) Register(c *Conn) {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		c.close()
		return
	}
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

// Unregister removes a connection from the hub.
func (h *Hub) Unregister(c *Conn) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// Handler returns the HTTP handler for the engine daemon.
func (h *Hub) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/ws", h.serveWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) //nolint:errcheck
	})
	return mux
}

// Serve runs the engine on an existing listener (e.g. 127.0.0.1:0 for embedded use).
func (h *Hub) Serve(ctx context.Context, ln net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	srv := &http.Server{Handler: h.Handler()}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	h.logger().Info("engine serving", "addr", ln.Addr(), "workspace", h.Workspace(), "model", h.Config.Model)
	serveErr := srv.Serve(ln)
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	return errors.Join(serveErr, h.Shutdown(shutdownCtx))
}

// ListenAndServe starts the HTTP and WebSocket server.
func (h *Hub) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", h.Addr)
	if err != nil {
		return err
	}
	return h.Serve(ctx, ln)
}

func (h *Hub) serveWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: h.checkOrigin, ReadBufferSize: 4096, WriteBufferSize: 4096}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if !h.authenticate(ws) {
		_ = ws.Close()
		return
	}
	c := newConn(h, ws)
	h.Register(c)
	h.logger().Info("ws client connected", "remote", r.RemoteAddr)

	go c.writePump()
	c.readPump(r.Context())

	h.Unregister(c)
	c.close()
	h.logger().Info("ws client disconnected", "remote", r.RemoteAddr)
}
