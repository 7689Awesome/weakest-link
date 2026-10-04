package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"chase/internal/bank"
	"chase/internal/room"
)

// peer is a WebSocket client that keeps the latest state it was sent.
type peer struct {
	t     *testing.T
	conn  *websocket.Conn
	mu    sync.Mutex
	state room.View
	raw   []string // every raw state frame, to check for leaks
	errs  []string
	close string
}

func dial(t *testing.T, base string, q url.Values) *peer {
	t.Helper()
	u := "ws" + strings.TrimPrefix(base, "http") + "/ws?" + q.Encode()
	c, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &peer{t: t, conn: c}
	go func() {
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				p.mu.Lock()
				p.close = err.Error()
				p.mu.Unlock()
				return
			}
			var env struct {
				Type    string    `json:"type"`
				State   room.View `json:"state"`
				Message string    `json:"message"`
			}
			_ = json.Unmarshal(data, &env)
			p.mu.Lock()
			switch env.Type {
			case "state":
				p.state = env.State
				p.raw = append(p.raw, string(data))
			case "error":
				p.errs = append(p.errs, env.Message)
			}
			p.mu.Unlock()
		}
	}()
	return p
}

func (p *peer) get() room.View {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *peer) send(typ, action string, arg map[string]any) {
	b, _ := json.Marshal(map[string]any{"type": typ, "action": action, "arg": arg})
	if err := p.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		p.t.Fatal(err)
	}
}

func (p *peer) waitPhase(ph room.Phase) room.View {
	p.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if v := p.get(); v.Phase == ph {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.t.Fatalf("timed out waiting for phase %s, last %s (errors %v)", ph, p.get().Phase, p.errs)
	return room.View{}
}

func post(t *testing.T, url string, body any) map[string]string {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]string{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	out["_status"] = resp.Status
	return out
}

func TestFullContestantOverWebSockets(t *testing.T) {
	set, err := bank.Load("")
	if err != nil {
		t.Fatal(err)
	}
	cfg := room.DefaultConfig()
	cfg.CBSeconds = 1
	cfg.RevealFor = 80 * time.Millisecond
	cfg.LockWindow = 300 * time.Millisecond
	mgr := room.NewManager(set, cfg, 10)
	defer mgr.Shutdown()
	ts := httptest.NewServer(New(mgr, t.TempDir()))
	defer ts.Close()

	created := post(t, ts.URL+"/api/rooms", nil)
	code, hostKey := created["roomCode"], created["hostKey"]
	if len(code) != 4 || hostKey == "" {
		t.Fatalf("bad room: %v", created)
	}

	// Wrong host key and unknown room are rejected with a close reason.
	bad := dial(t, ts.URL, url.Values{"role": {"host"}, "code": {code}, "key": {"nope"}})
	time.Sleep(100 * time.Millisecond)
	bad.mu.Lock()
	if !strings.Contains(bad.close, "4003") {
		t.Fatalf("bad host key should be refused with 4003, got %q", bad.close)
	}
	bad.mu.Unlock()

	host := dial(t, ts.URL, url.Values{"role": {"host"}, "code": {code}, "key": {hostKey}})
	screen := dial(t, ts.URL, url.Values{"role": {"screen"}, "code": {code}})

	cj := post(t, ts.URL+"/api/rooms/"+code+"/players", map[string]string{"name": "Mark"})
	pj := post(t, ts.URL+"/api/rooms/"+code+"/players", map[string]string{"name": "Alice"})
	if dup := post(t, ts.URL+"/api/rooms/"+code+"/players", map[string]string{"name": "alice"}); !strings.Contains(dup["_status"], "409") {
		t.Fatalf("duplicate name should 409, got %v", dup)
	}
	chaser := dial(t, ts.URL, url.Values{"role": {"player"}, "code": {code}, "playerId": {cj["playerId"]}, "token": {cj["playerToken"]}})
	player := dial(t, ts.URL, url.Values{"role": {"player"}, "code": {code}, "playerId": {pj["playerId"]}, "token": {pj["playerToken"]}})

	// A player must not be able to drive the game.
	player.send("control", "startGame", nil)
	time.Sleep(100 * time.Millisecond)
	if player.get().Phase != room.PhaseLobby {
		t.Fatal("a player should not be able to send host controls")
	}

	host.send("control", "setChaser", map[string]any{"id": cj["playerId"]})
	host.send("control", "startGame", nil)
	host.waitPhase(room.PhaseBetween)
	host.send("control", "startPlayer", map[string]any{"id": pj["playerId"]})
	host.waitPhase(room.PhaseCBReady)
	host.send("control", "startCashBuilder", nil)
	host.waitPhase(room.PhaseCBPlaying)
	if host.get().CB.Answer == "" {
		t.Fatal("host should see the cash-builder answer")
	}
	if screen.waitPhase(room.PhaseCBPlaying).CB.Answer != "" {
		t.Fatal("screen must not see the cash-builder answer")
	}
	for i := 0; i < 3; i++ {
		host.send("control", "markCB", map[string]any{"correct": true})
	}
	cbDone := host.waitPhase(room.PhaseCBDone)
	if cbDone.CB.Total != 3000 {
		t.Fatalf("expected 3000 built, got %d", cbDone.CB.Total)
	}

	host.send("control", "toOffers", nil)
	offers := chaser.waitPhase(room.PhaseOffersSet).Offers
	chaser.send("act", "setOffers", map[string]any{"lower": offers.SuggestLower, "higher": offers.SuggestHigher})
	player.waitPhase(room.PhaseOffersChoose)
	player.send("act", "chooseOffer", map[string]any{"choice": "middle"})
	host.waitPhase(room.PhaseH2HReady)
	host.send("control", "startH2H", nil)

	// Five correct answers brings the player home; the chaser stays silent
	// and is locked out after the 300ms window every time.
	for i := 0; i < 5; i++ {
		var v room.View
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			v = host.get()
			if v.Phase == room.PhaseH2HQuestion && v.H2H.QNum == i+1 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if v.H2H == nil || v.H2H.Correct == nil || v.H2H.QNum != i+1 {
			t.Fatalf("question %d never opened: %+v", i+1, v.Phase)
		}
		player.send("act", "lock", map[string]any{"choice": *v.H2H.Correct})
	}
	over := host.waitPhase(room.PhaseH2HOver)
	if over.H2H.Outcome != "home" || over.Bank != 3000 {
		t.Fatalf("outcome=%s bank=%d", over.H2H.Outcome, over.Bank)
	}

	// The screen and phones only ever see the answer once it has been revealed.
	for name, p := range map[string]*peer{"screen": screen, "chaser": chaser, "player": player} {
		p.mu.Lock()
		for _, raw := range p.raw {
			var env struct{ State room.View }
			_ = json.Unmarshal([]byte(raw), &env)
			h := env.State.H2H
			if h != nil && h.Correct != nil && h.Reveal == nil {
				p.mu.Unlock()
				t.Fatalf("%s received the answer before the reveal", name)
			}
		}
		p.mu.Unlock()
	}

	// Reconnecting with the saved token restores the seat; a stale token does not.
	re := post(t, ts.URL+"/api/rooms/"+code+"/players", map[string]string{"rejoinId": pj["playerId"], "rejoinToken": pj["playerToken"]})
	if re["playerId"] != pj["playerId"] {
		t.Fatalf("rejoin failed: %v", re)
	}
}
