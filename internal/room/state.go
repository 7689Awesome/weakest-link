// Package room holds the Chase game rules as a pure state machine (State) and
// the actor that drives it over the network (Room). This file defines the
// state; actions.go mutates it; sanitize.go turns it into per-viewer JSON.
//
// Nothing in this package imports networking or a database. Keep it that way:
// it is what makes the rules unit-testable.
package room

import (
	"crypto/rand"
	"encoding/hex"
	mrand "math/rand"
	"time"

	"chase/internal/bank"
)

// Phase is the stage of the game. The host drives most transitions; a few are
// timed (see State.Tick).
type Phase string

const (
	PhaseLobby Phase = "lobby" // players joining, host assigning the chaser

	PhaseBetween Phase = "between" // choosing who plays next

	PhaseCBReady   Phase = "cb_ready"   // cash builder: waiting for host to start the clock
	PhaseCBPlaying Phase = "cb_playing" // cash builder: 60s clock running
	PhaseCBDone    Phase = "cb_done"    // cash builder: clock ended, total shown

	PhaseOffersSet    Phase = "offers_set"    // chaser is setting higher/lower offers
	PhaseOffersChoose Phase = "offers_choose" // player is picking an offer

	PhaseH2HReady    Phase = "h2h_ready"    // board set, waiting for host to start
	PhaseH2HQuestion Phase = "h2h_question" // both locking in an answer
	PhaseH2HReveal   Phase = "h2h_reveal"   // answers revealed, board moves
	PhaseH2HOver     Phase = "h2h_over"     // player is home or caught

	PhaseFinalPick      Phase = "final_pick"       // finalists vote for set A or B
	PhaseFinalTeamReady Phase = "final_team_ready" // waiting to start the team's 2 minutes
	PhaseFinalTeam      Phase = "final_team"       // team clock running, buzzers live
	PhaseFinalTeamDone  Phase = "final_team_done"  // team's clock ended, score shown
	PhaseFinalChaser    Phase = "final_chaser"     // chaser clock running
	PhaseFinalPush      Phase = "final_push"       // chaser missed: clock stopped, team may push back
	PhaseGameOver       Phase = "gameover"
)

// PStatus is where a player stands in the game.
type PStatus string

const (
	StatusWaiting PStatus = "waiting"
	StatusHome    PStatus = "home"   // made it home in the head-to-head
	StatusCaught  PStatus = "caught" // caught by the chaser
)

// Config holds the tunable rules. DefaultConfig matches the TV show.
type Config struct {
	CashPerCorrect int           // money per correct cash-builder answer
	CBSeconds      int           // cash builder clock
	FinalSeconds   int           // final chase clock (team and chaser each get one)
	LockWindow     time.Duration // once one side locks in, the other has this long
	RevealFor      time.Duration // how long a head-to-head reveal stays up before the next question
	BoardSteps     int           // steps on the board; home is BoardSteps+1
	StartLower     int           // starting step for the lower offer (further from the chaser)
	StartMiddle    int           // starting step when playing for the cash-builder total
	StartHigher    int           // starting step for the higher offer (closer to the chaser)
	MaxContestants int           // players besides the chaser
}

func DefaultConfig() Config {
	return Config{
		CashPerCorrect: 1000,
		CBSeconds:      60,
		FinalSeconds:   120,
		LockWindow:     5 * time.Second,
		RevealFor:      4 * time.Second,
		BoardSteps:     7,
		StartLower:     4,
		StartMiddle:    3,
		StartHigher:    2,
		MaxContestants: 4,
	}
}

// Home returns the position that counts as reaching the bank.
func (c Config) Home() int { return c.BoardSteps + 1 }

// Player is one person in the room. Exactly one player is the chaser.
type Player struct {
	ID        string
	Name      string
	Token     string
	Status    PStatus
	CashBuilt int    // total from their cash builder
	Banked    int    // money they brought home (0 if caught)
	Vote      string // "A" or "B" in the final-set vote
	Connected bool   // maintained by the Room actor
}

type CBState struct {
	Correct int
	Asked   int
	EndsAt  time.Time
	Q       *bank.QA
}

type OffersState struct {
	Lower, Middle, Higher int
	SuggestLower          int
	SuggestHigher         int
	Choice                string // "lower" | "middle" | "higher" once chosen
}

// Amount returns the money riding on the chosen offer.
func (o OffersState) Amount() int {
	switch o.Choice {
	case "lower":
		return o.Lower
	case "higher":
		return o.Higher
	default:
		return o.Middle
	}
}

const (
	pickNone  = -1 // hasn't locked in yet
	pickTimed = -2 // ran out of time after the other side locked in
)

type H2HState struct {
	Start       int
	PlayerPos   int
	ChaserPos   int
	QNum        int
	Q           *bank.MC // options already shuffled; Q.Answer indexes the correct one
	PlayerPick  int
	ChaserPick  int
	Deadline    time.Time // when the slower side is locked out (zero until someone locks in)
	RevealUntil time.Time
	PlayerRight bool
	ChaserRight bool
	Outcome     string // "", "home", "caught"
}

type FinalState struct {
	Finalists     []string
	TeamSet       string // "A" or "B"
	ChaserSet     string
	Head          int // head start: one step per finalist
	TeamCorrect   int
	ChaserCorrect int
	Pushbacks     int
	Q             *bank.QA
	QNum          int
	BuzzedBy      string
	Running       bool
	EndsAt        time.Time
	Remaining     time.Duration // frozen time while the clock is stopped
}

// Target is how many correct answers the chaser must reach to catch the team.
func (f *FinalState) Target() int { return f.Head + f.TeamCorrect + f.Pushbacks }

type Result struct {
	Winner    string   `json:"winner"` // "team" or "chaser"
	Bank      int      `json:"bank"`
	PerPlayer int      `json:"perPlayer"`
	Finalists []string `json:"finalists"`
}

// State is the whole game. Only one goroutine (the Room actor) touches it.
type State struct {
	cfg     Config
	rng     *mrand.Rand
	Code    string
	HostKey string
	Phase   Phase

	Players   []*Player
	ChaserID  string
	Queue     []string // contestants still to play, in order
	CurrentID string

	CB     CBState
	Offers OffersState
	H2H    H2HState
	Final  FinalState
	Result *Result
	Log    []string

	cbDeck    *deck[bank.QA]
	h2hDeck   *deck[bank.MC]
	finalDeck map[string]*deck[bank.QA]
}

// NewState builds a fresh lobby using the given question bank.
func NewState(code, hostKey string, set *bank.Set, cfg Config, rng *mrand.Rand) *State {
	return &State{
		cfg:     cfg,
		rng:     rng,
		Code:    code,
		HostKey: hostKey,
		Phase:   PhaseLobby,
		cbDeck:  newDeck(set.CashBuilder, rng),
		h2hDeck: newDeck(set.HeadToHead, rng),
		finalDeck: map[string]*deck[bank.QA]{
			"A": newDeck(set.FinalA, rng),
			"B": newDeck(set.FinalB, rng),
		},
	}
}

// --- helpers ---------------------------------------------------------------

// NewID returns a random hex id/token.
func NewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenRoomCode returns a 4-letter room code.
func GenRoomCode() string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // no I or O: too easy to misread
	b := make([]byte, 4)
	for i := range b {
		b[i] = letters[mrand.Intn(len(letters))]
	}
	return string(b)
}

func (s *State) player(id string) *Player {
	for _, p := range s.Players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *State) chaser() *Player  { return s.player(s.ChaserID) }
func (s *State) current() *Player { return s.player(s.CurrentID) }

func (s *State) logf(msg string) {
	s.Log = append(s.Log, msg)
	if len(s.Log) > 60 {
		s.Log = s.Log[len(s.Log)-60:]
	}
}

// bankTotal is the money brought home so far.
func (s *State) bankTotal() int {
	t := 0
	for _, p := range s.Players {
		if p.Status == StatusHome {
			t += p.Banked
		}
	}
	return t
}

// deck deals items in a shuffled order and reshuffles when exhausted.
type deck[T any] struct {
	items []T
	order []int
	pos   int
	rng   *mrand.Rand
}

func newDeck[T any](items []T, rng *mrand.Rand) *deck[T] {
	d := &deck[T]{items: items, rng: rng}
	d.reshuffle()
	return d
}

func (d *deck[T]) reshuffle() {
	d.order = d.rng.Perm(len(d.items))
	d.pos = 0
}

func (d *deck[T]) next() T {
	if d.pos >= len(d.order) {
		d.reshuffle()
	}
	v := d.items[d.order[d.pos]]
	d.pos++
	return v
}

// shuffleMC randomises option order and tracks where the correct one lands.
func shuffleMC(q bank.MC, rng *mrand.Rand) bank.MC {
	out := bank.MC{Q: q.Q}
	for i, p := range rng.Perm(3) {
		out.Options[i] = q.Options[p]
		if p == q.Answer {
			out.Answer = i
		}
	}
	return out
}
