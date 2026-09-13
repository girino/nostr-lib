# nostr-lib

Shared pieces for Girino's Nostr relays (khatru + go-nostr).

## Packages

| Package | Role |
|---|---|
| `broadcast` | Relay ranking, discovery, publish workers |
| `eventstore/relaystore` | Query remotes |
| `eventstore/broadcaststore` | SaveEvent → publish |
| `mirror` | Subscribe to query remotes, inject into a khatru relay |
| `fanout` | Per-websocket write queues for khatru (slow clients cannot stall others) |
| `stats` / `json` / `logging` | Collector, JSON, logs |

## Safe khatru relay (recommended)

khatru `BroadcastEvent` writes sockets in series with no deadline. Combined with go-nostr `dispatchEvent`, a slow client leaks goroutines.

```go
import (
    "github.com/fiatjaf/khatru"
    "github.com/girino/nostr-lib/fanout"
    "github.com/girino/nostr-lib/mirror"
)

relay := khatru.NewRelay()
hub := fanout.Attach(relay) // optional: fanout.WithMaxConnections(256)
defer hub.Close()

mm := mirror.NewMirrorManager(queryURLs)
if err := mm.Init(); err != nil { /* ... */ }
if err := mm.StartMirroringHub(relay, hub); err != nil { /* ... */ }
```

`StartMirroring(relay)` still exists and uses khatru's sync `BroadcastEvent`. Prefer `StartMirroringHub`.

`fanout.Attach` also:

- skips khatru's sequential `WriteJSON` for sockets it owns (`PreventBroadcast`)
- fans stored EVENTs out asynchronously (`OnEventSaved`)
- tracks REQs itself (do not use khatru `GetListeningFilters` — it races)
- caps concurrent websockets (default 256)
- logs slow writes and disconnects a socket stuck >3s

## Tests

```bash
go test ./...
```
