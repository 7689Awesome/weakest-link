package room

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	mrand "math/rand"
	"sync"
	"time"

	"chase/internal/bank"
)

// Client is one live connection. The httpapi package implements it over a
// WebSocket; tests implement it with a channel. Implementations must not block.
type Client interface {
	Send(data []byte)
	Close(code int, reason string)
}

// Close codes sent to clients (4000-4999 is the application range).
const (
	CloseBadAuth  = 4003
	CloseReplaced = 4001
	CloseRoomGone = 4004
)

// Room is an actor: one goroutine owns the State and handles every event from
// a single inbox, so the game logic needs no locks.
type Room struct {
	Code string

	st      *State
	inbox   chan any
	stop    chan struct{}
	onClose func()

	clients    map[Client]Viewer
	playerConn map[string]Client
	lastActive time.Time
	created    time.Time
}

type (
	attachMsg struct {
		v      Viewer
		secret string // host key, or player token
		c      Client
		reply  chan error
	}
	detachMsg struct{ c Client }
	inMsg     struct {
		c    Client
		data []byte
	}
	joinMsg struct {
		name, rejoinID, rejoinToken string
		reply                       chan joinReply
	}
	infoMsg struct{ reply chan Info }
)

type joinReply struct {
	ID, Token string
	Err       error
}

// Info is the lightweight pre-flight data for the join screen.
type Info struct {
	Phase       Phase `json:"phase"`
	PlayerCount int   `json:"playerCount"`
	Full        bool  `json:"full"`
}

const (
	idleTimeout = 45 * time.Minute
	maxAge      = 12 * time.Hour
)

func newRoom(code, hostKey string, set *bank.Set, cfg Config, onClose func()) *Room {
	rng := mrand.New(mrand.NewSource(time.Now().UnixNano()))
	return &Room{
		Code:       code,
		st:         NewState(code, hostKey, set, cfg, rng),
		inbox:      make(chan any, 64),
		stop:       make(chan struct{}),
		onClose:    onClose,
		clients:    map[Client]Viewer{},
		playerConn: map[string]Client{},
		lastActive: time.Now(),
		created:    time.Now(),
	}
}

// Run is the actor loop. It returns when the room is closed.
func (r *Room) Run() {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	defer r.onClose()
	defer r.Shutdown() // unblock anyone still waiting on this room
	for {
		select {
		case m := <-r.inbox:
			r.lastActive = time.Now()
			if r.handle(m) {
				r.broadcast()
			}
		case <-tick.C:
			now := time.Now()
			if r.st.Tick(now) {
				r.broadcast()
			}
			if (len(r.clients) == 0 && now.Sub(r.lastActive) > idleTimeout) || now.Sub(r.created) > maxAge {
				for c := range r.clients {
					c.Close(CloseRoomGone, "room closed")
				}
				return
			}
		case <-r.stop:
			for c := range r.clients {
				c.Close(CloseRoomGone, "room closed")
			}
			return
		}
	}
}

// Shutdown stops the room.
func (r *Room) Shutdown() {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
}

// handle processes one event and reports whether state should be broadcast.
func (r *Room) handle(m any) bool {
	switch m := m.(type) {
	case attachMsg:
		err := r.attach(m)
		m.reply <- err
		return err == nil

	case detachMsg:
		v, ok := r.clients[m.c]
		if !ok {
			return false
		}
		delete(r.clients, m.c)
		if v.Role == "player" && r.playerConn[v.PlayerID] == m.c {
			delete(r.playerConn, v.PlayerID)
			if p := r.st.player(v.PlayerID); p != nil {
				p.Connected = false
			}
		}
		return true

	case joinMsg:
		m.reply <- r.join(m)
		return true

	case infoMsg:
		m.reply <- Info{
			Phase: r.st.Phase, PlayerCount: len(r.st.Players),
			Full: len(r.st.Players) >= r.st.cfg.MaxContestants+1,
		}
		return false

	case inMsg:
		return r.incoming(m)
	}
	return false
}

func (r *Room) attach(m attachMsg) error {
	switch m.v.Role {
	case "screen":
		// read-only, no secret
	case "host":
		if subtle.ConstantTimeCompare([]byte(m.secret), []byte(r.st.HostKey)) != 1 {
			return errors.New("bad host key")
		}
	case "player":
		p := r.st.Auth(m.v.PlayerID, m.secret)
		if p == nil {
			return errors.New("unknown player: rejoin from the join screen")
		}
		if old, ok := r.playerConn[p.ID]; ok && old != m.c {
			delete(r.clients, old)
			old.Close(CloseReplaced, "opened on another device")
		}
		r.playerConn[p.ID] = m.c
		p.Connected = true
	default:
		return errors.New("unknown role")
	}
	r.clients[m.c] = m.v
	return nil
}

func (r *Room) join(m joinMsg) joinReply {
	if m.rejoinID != "" {
		if p := r.st.Auth(m.rejoinID, m.rejoinToken); p != nil {
			return joinReply{ID: p.ID, Token: p.Token}
		}
	}
	p, err := r.st.Join(m.name)
	if err != nil {
		return joinReply{Err: err}
	}
	return joinReply{ID: p.ID, Token: p.Token}
}

type envelope struct {
	Type   string `json:"type"`
	Action string `json:"action"`
	Arg    Args   `json:"arg"`
}

func (r *Room) incoming(m inMsg) bool {
	v, ok := r.clients[m.c]
	if !ok {
		return false
	}
	var env envelope
	if err := json.Unmarshal(m.data, &env); err != nil {
		r.sendErr(m.c, "malformed message")
		return false
	}
	if env.Arg == nil {
		env.Arg = Args{}
	}
	now := time.Now()
	var err error
	switch {
	case env.Type == "control" && v.Role == "host":
		err = r.st.Control(env.Action, env.Arg, now)
	case env.Type == "act" && v.Role == "player":
		err = r.st.Act(v.PlayerID, env.Action, env.Arg, now)
	default:
		err = errors.New("not allowed")
	}
	if err != nil {
		r.sendErr(m.c, err.Error())
		// A failed action can still have changed state (e.g. late mark ended
		// the clock), so broadcast regardless.
	}
	return true
}

func (r *Room) sendErr(c Client, msg string) {
	b, _ := json.Marshal(map[string]string{"type": "error", "message": msg})
	c.Send(b)
}

func (r *Room) broadcast() {
	now := time.Now()
	for c, v := range r.clients {
		b, err := json.Marshal(struct {
			Type  string `json:"type"`
			State View   `json:"state"`
		}{"state", r.st.Snapshot(v, now)})
		if err == nil {
			c.Send(b)
		}
	}
}

// --- public API (safe to call from any goroutine) --------------------------

// Attach registers a connection. It returns an error if the credentials are
// wrong; on success the client immediately receives the current state.
func (r *Room) Attach(v Viewer, secret string, c Client) error {
	reply := make(chan error, 1)
	if !r.send(attachMsg{v: v, secret: secret, c: c, reply: reply}) {
		return errRoomClosed
	}
	return wait(r, reply, errRoomClosed)
}

func (r *Room) Detach(c Client) { r.send(detachMsg{c: c}) }

func (r *Room) Incoming(c Client, data []byte) { r.send(inMsg{c: c, data: data}) }

// Join adds a player or re-authenticates a returning one.
func (r *Room) Join(name, rejoinID, rejoinToken string) (id, token string, err error) {
	reply := make(chan joinReply, 1)
	if !r.send(joinMsg{name: name, rejoinID: rejoinID, rejoinToken: rejoinToken, reply: reply}) {
		return "", "", errRoomClosed
	}
	jr := wait(r, reply, joinReply{Err: errRoomClosed})
	return jr.ID, jr.Token, jr.Err
}

func (r *Room) Info() (Info, bool) {
	reply := make(chan Info, 1)
	if !r.send(infoMsg{reply: reply}) {
		return Info{}, false
	}
	select {
	case in := <-reply:
		return in, true
	case <-r.stop:
		return Info{}, false
	}
}

var errRoomClosed = errors.New("room closed")

// wait returns the actor's reply, or fallback if the room shuts down first.
func wait[T any](r *Room, reply chan T, fallback T) T {
	select {
	case v := <-reply:
		return v
	case <-r.stop:
		return fallback
	}
}

func (r *Room) send(m any) bool {
	select {
	case r.inbox <- m:
		return true
	case <-r.stop:
		return false
	}
}

// --- manager ---------------------------------------------------------------

// Manager is the room-code -> Room directory.
type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
	set   *bank.Set
	cfg   Config
	max   int
}

func NewManager(set *bank.Set, cfg Config, maxRooms int) *Manager {
	return &Manager{rooms: map[string]*Room{}, set: set, cfg: cfg, max: maxRooms}
}

// Create makes a room and returns it with its secret host key.
func (m *Manager) Create() (*Room, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rooms) >= m.max {
		return nil, "", errors.New("the server is busy, try again later")
	}
	var code string
	for {
		code = GenRoomCode()
		if _, taken := m.rooms[code]; !taken {
			break
		}
	}
	key := NewID()
	r := newRoom(code, key, m.set, m.cfg, func() { m.remove(code) })
	m.rooms[code] = r
	go r.Run()
	return r, key, nil
}

func (m *Manager) remove(code string) {
	m.mu.Lock()
	delete(m.rooms, code)
	m.mu.Unlock()
}

func (m *Manager) Get(code string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rooms[code]
}

// Shutdown stops every room.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	rs := make([]*Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		rs = append(rs, r)
	}
	m.mu.Unlock()
	for _, r := range rs {
		r.Shutdown()
	}
}
