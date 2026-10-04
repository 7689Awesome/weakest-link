# Chase Night: project guide

A "The Chase"-style quiz party game. Go backend, buildless ES-module frontend. Structure follows the same ideas as the Weakest Link ("Chain Reaction") project this was based on.

## Architecture

**The Go server is authoritative for all game state.** Each room is an actor: one goroutine owns a `*room.State` and handles every event from a single inbox channel plus a 250ms ticker (clock expiry, reveal timing). Only that goroutine touches the state, so the game rules need no locks.

Four client roles, all over WebSocket:

| Role | Who | Can do |
|---|---|---|
| `screen` | The shared TV | Read-only. Same redacted view as a phone. |
| `host` | The quizmaster's controller | The only role that sees answers and sends `control` actions. |
| `player` (contestant) | A phone | `lock`, `chooseOffer`, `buzz`, `pickSet` |
| `player` (chaser) | A phone, flagged by `isChaser` | `lock`, `setOffers`, plus the same view-only roles |

The chaser is not a separate connection type; they join like anyone else and the host assigns the role in the lobby. The frontend picks the chaser or contestant UI from `state.me`.

State is in memory only. A restart loses live games; redeploy between sessions.

### Go layout

```
cmd/server/main.go        env config, question bank load, wiring, graceful shutdown
internal/
  bank/                   parses and validates the plain-text question files (embedded by default)
  room/                   zero network or DB imports: keep it that way
    state.go                types: Phase, Config, Player, CB/Offers/H2H/Final state, decks
    actions.go              every rule: Control (host actions), Act (player actions), Tick (clocks)
    sanitize.go             Snapshot(viewer): the ONE place answers are redacted
    room.go                 the Room actor and the room-code Manager
    room_test.go            rules tested directly against State, no networking
  httpapi/                server.go (REST + static), ws.go (gorilla/websocket adapter), e2e_test.go
```

### Game flow (phases)

```
lobby -> between -> cb_ready -> cb_playing -> cb_done -> offers_set -> offers_choose
      -> h2h_ready -> h2h_question <-> h2h_reveal -> h2h_over -> (between | final_pick)
final_pick -> final_team_ready -> final_team -> final_team_done
           -> final_chaser <-> final_push -> gameover -> (lobby via playAgain)
```

`h2h_reveal` and the 5-second lock window advance on the ticker; everything else waits for the host or a player action. The board has `BoardSteps`=7 steps; the Chaser starts at position 0, home is position 8. Start positions are 2/3/4 for higher/cash-builder/lower.

### WebSocket protocol

Room lifecycle is REST; gameplay is WebSocket. Identity comes from the connection URL, so there is no join handshake and the server pushes state immediately.

- `POST /api/rooms` returns `{roomCode, hostKey}`. `hostKey` is the secret that grants host control; the 4-letter code is public and never enough on its own.
- `GET /api/rooms/{code}` returns `{exists, phase, playerCount, full}`.
- `POST /api/rooms/{code}/players` with `{name, rejoinId?, rejoinToken?}` returns `{playerId, playerToken}`.
- `GET /ws?role=screen&code=X`, `?role=host&code=X&key=K`, `?role=player&code=X&playerId=Y&token=Z`

Client to server: `{type:"control"|"act", action, arg}`. Server to client: `{type:"state", state}` (a per-viewer snapshot) or `{type:"error", message}`. A rejected connection is reported in the close frame (codes 4001 replaced, 4003 bad auth, 4004 room gone), which the frontend treats as fatal (no retry).

Clocks need no ticking from the server. Snapshots carry absolute `endsAt` epoch-ms timestamps and a `now`; clients correct for skew and count down locally (`web/js/core/clock.js`).

### Frontend (buildless)

```
web/index.html            single entry; the URL picks the role (see js/game/main.js)
web/css/                  base, board, screen, host, player
web/js/core/              dom.js (html`` templates, auto-escaped), net.js, clock.js, sound.js
web/js/game/              components.js (board, track, options), screen.js, host.js, player.js, main.js
```

Views are functions from a snapshot to an `html` string; `main.js` re-renders only when the string changes. Buttons are wired by delegation: `data-send="control|act" data-action data-arg`.

## Conventions worth preserving

- Keep the frontend buildless. The only external asset is Google Fonts.
- Keep `internal/room` free of network and database imports. It is what makes the rules unit-testable.
- Never send an answer to the screen or a phone before it is revealed. Redaction lives only in `sanitize.go`; a test in `room_test.go` and the e2e test both check it.
- Rooms are intentionally not persisted.

## Ideas not built yet

- Admin-curated question bank in SQLite with public submissions (the Weakest Link project has this).
- A solo "nominated player" final chase when everyone is caught.
- Chaser-side "super offer", and negative lower offers once money is in the bank.
- Host-uploaded custom question sets per room.
