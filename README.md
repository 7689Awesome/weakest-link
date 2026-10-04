# Chase Night

A "The Chase"-style quiz party game. One person is the Chaser, up to four are contestants, and a quizmaster runs the show. A Go server holds the game and drives three kinds of screen over WebSockets: a shared big screen (TV), the quizmaster's controller, and everyone's phone.

## How a game plays

1. **Lobby.** The quizmaster creates a room and gets a 4-letter code. Open the big screen on the TV, and everyone else joins from their phone with the code and a name. The quizmaster picks the Chaser; everyone else is a contestant.
2. **For each contestant, in turn:**
   - **Cash builder.** 60 seconds. The quizmaster reads questions and marks them right or wrong. Each correct answer is worth £1,000.
   - **Offers.** The Chaser sets a *higher* offer (start one step closer to the Chaser) and a *lower* offer (one step further away). The contestant picks one, or sticks with their cash-builder total.
   - **Head-to-head.** Contestant and Chaser answer the same three-choice questions on their phones. The first to lock in starts a 5-second countdown for the other. Each correct answer moves that person one step down a 7-step board. The Chaser starts at the top; the contestant starts on step 2 (higher offer, 6 correct to get home), step 3 (cash builder, 5 correct) or step 4 (lower offer, 4 correct). Reach home and the money goes in the team bank. If the Chaser lands on you, you're caught.
3. **Final chase** (players who got home).
   - Finalists vote for question set A or B. The majority's set is the team's; the Chaser gets the other.
   - **Team round:** 2 minutes. Buzz in on your phone; the first buzz locks everyone else out; the quizmaster rules right or wrong. No steals. Every correct answer is a step, plus a one-step head start per finalist.
   - **Chaser round:** 2 minutes to answer as many as the team scored. If the Chaser gets one wrong the clock stops and the team can buzz in and answer the same question. A correct team answer pushes the Chaser back a step.
   - Catch the team and the Chaser wins; run out the clock and the team splits the bank.

### Assumptions to know about

These are choices made where the brief was open. Each is a small change in `internal/room/actions.go`.

- Only players who made it home vote, buzz and share the prize. Caught players are out of the final chase.
- A tie in the set vote is settled at random.
- Head start in the final chase is one step per finalist (as on the TV show). Set `Head` to `0` in `startFinal` to remove it.
- The final chase has no solo "nominated player" rescue round: if everyone is caught, the Chaser wins.
- The lower offer can be £0 but never negative.

## URLs

| URL | Who it's for |
|---|---|
| `/host` | The quizmaster. Bookmark it: **Start a new game**, or **Resume** a room this device was running. |
| `/screen` | The TV. Asks for the room code. |
| `/` | Everyone else: join with the code and a name. `/?join=ABCD` pre-fills the code. |

Players are normally sent `/?join=CODE` (the controller has a **Copy join link** button). The controller link itself contains a secret key, so only share it with a second quizmaster device.

There is no admin login in this version; the questions are text files (see Questions below).

## Local development

Requires Go 1.22+. There is no Node or build step; the frontend is plain ES modules served as static files.

```sh
go run ./cmd/server
```

Open <http://localhost:8080>.

To test a full game on your own, open these in separate browser windows (use private windows for the players so their seats don't collide):

1. **Quizmaster:** `/` then **Create a room**.
2. **Big screen:** `/?screen=CODE` (or **Open big screen** in the controller).
3. **Chaser and contestants:** `/` then **Join a game**, one window each.

To try it on real phones, find your computer's local IP (`ipconfig` on Windows, `ip addr` on Linux, `ifconfig` on macOS), then browse to `http://<that-ip>:8080` from the phones on the same Wi-Fi.

Add `FAST_MODE=true` to shorten every clock (6s cash builder, 8s final chase) for quick testing:

```sh
FAST_MODE=true go run ./cmd/server
```

### Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `STATIC_DIR` | `./web` | Where the frontend is served from |
| `QUESTIONS_DIR` | built-in | Folder containing your own question files (see below) |
| `FAST_MODE` | off | `true` shortens all clocks, for testing only |

### Questions

Question banks are plain text, one per line, in `internal/bank/data/`:

| File | Format |
|---|---|
| `cashbuilder.txt` | `Question \| Answer` |
| `final_a.txt`, `final_b.txt` | `Question \| Answer` |
| `headtohead.txt` | `Question \| Correct \| Wrong \| Wrong` |

Lines starting with `#` are comments. The head-to-head options are shuffled every time, so always write the correct one first. The server refuses to start if a bank is too small (20 cash builder, 20 head-to-head, 30 per final set). To use your own files without rebuilding, point `QUESTIONS_DIR` at a folder with the same four filenames.

### Tests

```sh
go test -race ./...
```

`internal/room` holds the game rules as a pure state machine and has the most coverage (steps to home for each offer, the 5-second lock window, the final chase pushback, and that answers never reach the screen or phones early). `internal/httpapi` has an end-to-end test over real WebSockets.

## Deploying to a small VPS

Docker Compose runs the Go binary behind Caddy, which gets an HTTPS certificate automatically. Rooms live in memory only, so redeploy between games, not during one.

1. Point an `A` record for your domain at the server's IP and wait for it to resolve (`dig +short chase.example.com`). This must work before Caddy starts.
2. Install Docker on the server, then:
   ```sh
   git clone <your-repo-url> /opt/chase && cd /opt/chase
   cp .env.example .env     # set DOMAIN
   docker compose up -d --build
   ```
3. Visit `https://your-domain`.

### Running alongside Weakest Link

Easiest is a subdomain, such as `chase.gamenights.lol`, because the game uses root-relative paths (`/css`, `/ws`, `/api`) and doesn't work under a sub-path like `/chase/`.

1. Add an `A` record for `chase.gamenights.lol` pointing at the same server.
2. Put both apps on one Docker network and add a second block to the **existing** Caddyfile, pointing at this app's container name and port (the name below is an example; use whatever your compose file calls it):
   ```
   chase.gamenights.lol {
   	encode gzip
   	reverse_proxy chase-app:8080
   }
   ```
3. Run this project's `app` service without its own `caddy` service (two Caddy instances can't both bind ports 80 and 443). Delete the `caddy` service and the `DOMAIN` line from this `docker-compose.yml`, and attach `app` to the network the existing Caddy uses.

Then the quizmaster bookmarks `https://chase.gamenights.lol/host`.

To update: `git pull && docker compose up -d --build`. Logs: `docker compose logs -f app`.

## Project layout

See `CLAUDE.md` for the architecture and the WebSocket protocol.
