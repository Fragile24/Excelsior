package engine

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"excelsior/internal/chat"
	"excelsior/pkg/config"
	"excelsior/pkg/llm"
	"excelsior/pkg/protocol"
	"github.com/gorilla/websocket"
)

func TestClientsKeepIndependentWorkspaces(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	hub := NewHub(config.Config{}, first)
	hub.Token = "test-token"
	t.Cleanup(hub.Close)
	server := httptest.NewServer(hub.Handler())
	t.Cleanup(server.Close)
	read := func(ws *websocket.Conn, typ string) protocol.Envelope {
		t.Helper()
		var env protocol.Envelope
		if err := ws.ReadJSON(&env); err != nil {
			t.Fatal(err)
		}
		if env.Type != typ {
			t.Fatalf("response = %s: %s; want %s", env.Type, env.Payload, typ)
		}
		return env
	}
	request := func(ws *websocket.Conn, typ string, payload any) protocol.Envelope {
		t.Helper()
		if err := ws.WriteJSON(protocol.NewEnvelope(typ, payload)); err != nil {
			t.Fatal(err)
		}
		return read(ws, typ)
	}
	connect := func() *websocket.Conn {
		t.Helper()
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ws.Close() })
		_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		request(ws, protocol.TypeAuth, map[string]string{"token": hub.Token})
		return ws
	}
	a, b := connect(), connect()
	request(a, protocol.TypeWorkspaceSet, protocol.WorkspaceSetReq{Workspace: second})
	read(a, protocol.TypeSessionList)
	request(a, protocol.TypeSessionCreate, protocol.SessionCreateReq{Title: "Second workspace"})
	for _, tc := range []struct {
		client    *websocket.Conn
		workspace string
		count     int
	}{{a, second, 1}, {b, first, 0}} {
		env := request(tc.client, protocol.TypeSessionList, nil)
		var list protocol.SessionListResp
		if err := env.Decode(&list); err != nil {
			t.Fatal(err)
		}
		if env.Workspace != canonicalWorkspace(tc.workspace) || len(list.Sessions) != tc.count {
			t.Fatalf("workspace = %q, sessions = %v; want %q with %d sessions", env.Workspace, list.Sessions, tc.workspace, tc.count)
		}
	}
}

func TestNewTurnTitleIsAvailableBeforeGeneration(t *testing.T) {
	workspace := t.TempDir()
	hub := NewHub(config.Config{}, workspace)
	t.Cleanup(hub.Close)
	coord := hub.Coordinator()
	handle, err := coord.ReserveTurn(workspace, "conversation", llm.Message{Role: "user", Content: "  Refactor the parser  "})
	if err != nil {
		t.Fatal(err)
	}
	defer coord.ExecuteTurn(chat.StartCommand{}, handle)
	metas, err := coord.ListSessions(workspace)
	if err != nil || len(metas) != 1 || metas[0].Title != "Refactor the parser" {
		t.Fatalf("ListSessions before generation = %v, %v", metas, err)
	}
	record, err := coord.Store(workspace).Load("conversation")
	if err != nil || len(record.Messages) != 0 {
		t.Fatalf("unfinished turn persisted messages: %+v, %v", record, err)
	}
}
