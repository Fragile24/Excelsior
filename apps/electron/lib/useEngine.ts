"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";
import { EngineClient } from "./engineClient";
import { activeConversation, initialEngineState, reduce } from "./engineState";

// React owns the projection. The client owns commands, selection and reconnects.
export function useEngine(engineUrl: string) {
  const [state, dispatch] = useReducer(reduce, undefined, initialEngineState);
  const [connectionVersion, reconnect] = useReducer((v: number) => v + 1, 0);
  const clientRef = useRef<EngineClient | null>(null);

  useEffect(() => {
    dispatch({ type: "reset" });
    if (!engineUrl) return;
    let alive = true;
    const client = new EngineClient(engineUrl, {
      onMessage: (msg) => { if (alive) dispatch({ type: "server", msg }); },
      onSelection: (id) => { if (alive) dispatch({ type: "active", id }); },
      onStatus: (status) => { if (alive) dispatch({ type: "status", status }); },
      onError: (message) => { if (alive) dispatch({ type: "error", message }); },
      getToken: async (url) =>
        sessionStorage.getItem("engine-token:" + url) ||
        (await window.electronAPI?.getEngineToken(url)) || "",
    });
    clientRef.current = client;
    return () => {
      alive = false;
      client.close();
      if (clientRef.current === client) clientRef.current = null;
    };
  }, [engineUrl, connectionVersion]);

  const run = useCallback(async <T,>(operation: (client: EngineClient) => Promise<T>): Promise<T | undefined> => {
    const client = clientRef.current;
    try {
      if (!client) throw new Error("Engine disconnected; command was not sent.");
      dispatch({ type: "error", message: null });
      const result = await operation(client);
      return client === clientRef.current ? result : undefined;
    } catch (error) {
      if (client === clientRef.current) {
        dispatch({ type: "error", message: error instanceof Error ? error.message : String(error) });
      }
      return undefined;
    }
  }, []);

  const reportError = useCallback((message: string) => dispatch({ type: "error", message }), []);
  const setCompactNext = useCallback((enabled: boolean) => {
    if (state.activeId) dispatch({ type: "compact-next", workspace: state.workspace, id: state.activeId, enabled });
  }, [state.workspace, state.activeId]);
  return { state, conversation: activeConversation(state), run, reconnect, reportError, setCompactNext };
}
