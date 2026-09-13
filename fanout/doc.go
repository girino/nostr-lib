// Copyright (c) 2026 Girino Vey.
//
// This software is licensed under Girino's Anarchist License (GAL).
// See LICENSE file for full license text.
// License available at: https://license.girino.org/
//
// Package fanout isolates slow khatru websocket clients so they cannot stall
// the whole relay.
//
// khatru.Relay.BroadcastEvent / notifyListeners write sockets in series with
// no deadline. One stalled TCP send blocks every other listener and, when
// used from MirrorManager, backs up go-nostr dispatchEvent goroutines.
//
// Usage:
//
//	relay := khatru.NewRelay()
//	hub := fanout.Attach(relay, fanout.WithMaxConnections(256))
//	defer hub.Close()
//
//	mm := mirror.NewMirrorManager(queryURLs)
//	_ = mm.Init()
//	_ = mm.StartMirroringHub(relay, hub)
//
// Attach hooks PreventBroadcast (skip khatru's sync WriteJSON), OnEventSaved
// (async fan-out of stored EVENTs), RejectFilter (track REQs without using
// the racy GetListeningFilters), OnDisconnect, and an optional connection cap.
package fanout
