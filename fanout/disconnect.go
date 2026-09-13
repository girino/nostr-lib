// Copyright (c) 2026 Girino Vey.
package fanout

import (
	"github.com/fasthttp/websocket"
	"github.com/fiatjaf/khatru"
	"github.com/girino/nostr-lib/logging"
)

func disconnectClient(ws *khatru.WebSocket, reason string) {
	if ws == nil {
		return
	}
	ip := ""
	if ws.Request != nil {
		ip = khatru.GetIPFromRequest(ws.Request)
	}
	logging.Info("fanout: disconnecting client from %s: %s", ip, reason)
	payload := websocket.FormatCloseMessage(websocket.ClosePolicyViolation, reason)
	_ = ws.WriteMessage(websocket.CloseMessage, payload)
}
