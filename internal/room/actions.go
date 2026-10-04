package room

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Args is the loosely-typed argument bag that arrives with a WebSocket action.
type Args map[string]any

func (a Args) Str(k string) string {
	s, _ := a[k].(string)
	return s
}

func (a Args) Int(k string) (int, bool) {
	f, ok := a[k].(float64)
	return int(f), ok
}

func (a Args) Bool(k string) bool {
	b, _ := a[k].(bool)
	return b
}

// --- lobby -----------------------------------------------------------------

// Join adds a contestant (or the chaser-to-be) while the room is in the lobby.
func (s *State) Join(name string) (*Player, error) {
	if s.Phase != PhaseLobby {
		return nil, errors.New("the game has already started")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("enter a name")
	}
	if utf8.RuneCountInString(name) > 16 {
		return nil, errors.New("names are limited to 16 characters")
	}
	for _, p := range s.Players {
		if strings.EqualFold(p.Name, name) {
			return nil, errors.New("that name is taken")
		}
	}
	if len(s.Players) >= s.cfg.MaxContestants+1 {
		return nil, errors.New("the room is full")
	}
	p := &Player{ID: NewID(), Name: name, Token: NewID(), Status: StatusWaiting}
	s.Players = append(s.Players, p)
	s.logf(name + " joined")
	return p, nil
}

// Auth returns the player matching id+token, or nil.
func (s *State) Auth(id, token string) *Player {
	p := s.player(id)
	if p == nil || subtle.ConstantTimeCompare([]byte(p.Token), []byte(token)) != 1 {
		return nil
	}
	return p
}

func (s *State) setChaser(id string) error {
	if s.Phase != PhaseLobby {
		return errors.New("the chaser can only be changed in the lobby")
	}
	p := s.player(id)
	if p == nil {
		return errors.New("unknown player")
	}
	if s.ChaserID == id {
		s.ChaserID = ""
		return nil
	}
	s.ChaserID = id
	s.logf(p.Name + " is the chaser")
	return nil
}

func (s *State) kick(id string) error {
	if s.Phase != PhaseLobby {
		return errors.New("players can only be removed in the lobby")
	}
	for i, p := range s.Players {
		if p.ID == id {
			s.Players = append(s.Players[:i], s.Players[i+1:]...)
			if s.ChaserID == id {
				s.ChaserID = ""
			}
			s.logf(p.Name + " was removed")
			return nil
		}
	}
	return errors.New("unknown player")
}

func (s *State) startGame() error {
	if s.Phase != PhaseLobby {
		return errors.New("the game has already started")
	}
	if s.chaser() == nil {
		return errors.New("assign a chaser first")
	}
	s.Queue = nil
	for _, p := range s.Players {
		p.Status, p.CashBuilt, p.Banked, p.Vote = StatusWaiting, 0, 0, ""
		if p.ID != s.ChaserID {
			s.Queue = append(s.Queue, p.ID)
		}
	}
	if len(s.Queue) == 0 {
		return errors.New("you need at least one contestant besides the chaser")
	}
	s.Result = nil
	s.Phase = PhaseBetween
	s.logf("game started")
	return nil
}

func (s *State) playAgain() error {
	if s.Phase != PhaseGameOver {
		return errors.New("the game is not over yet")
	}
	for _, p := range s.Players {
		p.Status, p.CashBuilt, p.Banked, p.Vote = StatusWaiting, 0, 0, ""
	}
	s.Queue, s.CurrentID, s.Result = nil, "", nil
	s.CB, s.Offers, s.H2H, s.Final = CBState{}, OffersState{}, H2HState{}, FinalState{}
	s.Phase = PhaseLobby
	return nil
}

// --- cash builder ----------------------------------------------------------

func (s *State) startPlayer(id string) error {
	if s.Phase != PhaseBetween {
		return errors.New("not between players")
	}
	idx := -1
	for i, q := range s.Queue {
		if id == "" || q == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("that player has already played")
	}
	s.CurrentID = s.Queue[idx]
	s.Queue = append(s.Queue[:idx], s.Queue[idx+1:]...)
	s.CB, s.Offers, s.H2H = CBState{}, OffersState{}, H2HState{}
	s.Phase = PhaseCBReady
	s.logf(s.current().Name + " is up")
	return nil
}

func (s *State) startCashBuilder(now time.Time) error {
	if s.Phase != PhaseCBReady {
		return errors.New("cash builder is not ready")
	}
	q := s.cbDeck.next()
	s.CB = CBState{EndsAt: now.Add(time.Duration(s.cfg.CBSeconds) * time.Second), Asked: 1, Q: &q}
	s.Phase = PhaseCBPlaying
	return nil
}

func (s *State) markCB(correct bool, now time.Time) error {
	if s.Phase != PhaseCBPlaying {
		return errors.New("the cash builder clock is not running")
	}
	if !now.Before(s.CB.EndsAt) {
		s.finishCB()
		return errors.New("time was already up")
	}
	if correct {
		s.CB.Correct++
	}
	q := s.cbDeck.next()
	s.CB.Q = &q
	s.CB.Asked++
	return nil
}

func (s *State) finishCB() {
	p := s.current()
	p.CashBuilt = s.CB.Correct * s.cfg.CashPerCorrect
	s.CB.Q = nil
	s.Phase = PhaseCBDone
	s.logf(fmt.Sprintf("%s built %d", p.Name, p.CashBuilt))
}

// --- offers ----------------------------------------------------------------

func roundTo(v, unit int) int { return (v + unit/2) / unit * unit }

func (s *State) toOffers() error {
	if s.Phase != PhaseCBDone {
		return errors.New("the cash builder is not finished")
	}
	mid := s.current().CashBuilt
	hi := roundTo(max(mid*3, mid+5000), 1000)
	lo := roundTo(mid/3, 1000)
	if mid > 0 && lo >= mid {
		lo = mid - 1000
	}
	s.Offers = OffersState{Lower: lo, Middle: mid, Higher: hi, SuggestLower: lo, SuggestHigher: hi}
	s.Phase = PhaseOffersSet
	return nil
}

func (s *State) setOffers(lower, higher int) error {
	if s.Phase != PhaseOffersSet {
		return errors.New("offers are not being set")
	}
	mid := s.Offers.Middle
	if higher <= mid {
		return errors.New("the higher offer must be more than the cash builder total")
	}
	if lower < 0 || lower > mid || (lower == mid && mid != 0) {
		return errors.New("the lower offer must be less than the cash builder total")
	}
	s.Offers.Lower, s.Offers.Higher = lower, higher
	s.Phase = PhaseOffersChoose
	s.logf(fmt.Sprintf("offers: %d / %d / %d", lower, mid, higher))
	return nil
}

func (s *State) chooseOffer(choice string) error {
	if s.Phase != PhaseOffersChoose {
		return errors.New("no offer is on the table")
	}
	var start int
	switch choice {
	case "lower":
		start = s.cfg.StartLower
	case "middle":
		start = s.cfg.StartMiddle
	case "higher":
		start = s.cfg.StartHigher
	default:
		return errors.New("unknown offer")
	}
	s.Offers.Choice = choice
	s.H2H = H2HState{Start: start, PlayerPos: start, ChaserPos: 0, PlayerPick: pickNone, ChaserPick: pickNone}
	s.Phase = PhaseH2HReady
	s.logf(fmt.Sprintf("%s plays for %d", s.current().Name, s.Offers.Amount()))
	return nil
}

// --- head-to-head ----------------------------------------------------------

func (s *State) startH2H() error {
	if s.Phase != PhaseH2HReady {
		return errors.New("the head-to-head is not ready")
	}
	s.nextH2HQuestion()
	return nil
}

func (s *State) nextH2HQuestion() {
	q := shuffleMC(s.h2hDeck.next(), s.rng)
	s.H2H.Q = &q
	s.H2H.PlayerPick, s.H2H.ChaserPick = pickNone, pickNone
	s.H2H.Deadline = time.Time{}
	s.H2H.QNum++
	s.Phase = PhaseH2HQuestion
}

// lockIn records one side's answer. When the first side locks in, the other
// gets LockWindow to answer or is locked out. When both are in, we reveal.
func (s *State) lockIn(chaserSide bool, choice int, now time.Time) error {
	if s.Phase != PhaseH2HQuestion {
		return errors.New("no question is open")
	}
	if choice < 0 || choice > 2 {
		return errors.New("pick one of the three answers")
	}
	mine, other := &s.H2H.PlayerPick, &s.H2H.ChaserPick
	if chaserSide {
		mine, other = other, mine
	}
	if *mine != pickNone {
		return errors.New("you have already locked in")
	}
	*mine = choice
	if *other == pickNone {
		s.H2H.Deadline = now.Add(s.cfg.LockWindow)
		return nil
	}
	s.reveal(now)
	return nil
}

// lockOut ends the question now; anyone who hasn't answered is locked out.
func (s *State) lockOut(now time.Time) error {
	if s.Phase != PhaseH2HQuestion {
		return errors.New("no question is open")
	}
	s.reveal(now)
	return nil
}

func (s *State) reveal(now time.Time) {
	h := &s.H2H
	if h.PlayerPick == pickNone {
		h.PlayerPick = pickTimed
	}
	if h.ChaserPick == pickNone {
		h.ChaserPick = pickTimed
	}
	h.PlayerRight = h.PlayerPick == h.Q.Answer
	h.ChaserRight = h.ChaserPick == h.Q.Answer
	if h.PlayerRight {
		h.PlayerPos++
	}
	if h.ChaserRight {
		h.ChaserPos++
	}
	switch {
	case h.PlayerPos >= s.cfg.Home():
		h.Outcome = "home"
	case h.ChaserPos >= h.PlayerPos:
		h.Outcome = "caught"
	}
	h.RevealUntil = now.Add(s.cfg.RevealFor)
	h.Deadline = time.Time{}
	s.Phase = PhaseH2HReveal
}

func (s *State) finishH2H() {
	p := s.current()
	if s.H2H.Outcome == "home" {
		p.Status, p.Banked = StatusHome, s.Offers.Amount()
		s.logf(fmt.Sprintf("%s made it home with %d", p.Name, p.Banked))
	} else {
		p.Status, p.Banked = StatusCaught, 0
		s.logf(p.Name + " was caught")
	}
	s.Phase = PhaseH2HOver
}

func (s *State) afterH2H(now time.Time) error {
	if s.Phase != PhaseH2HOver {
		return errors.New("the head-to-head is not over")
	}
	if len(s.Queue) > 0 {
		s.CurrentID = ""
		s.Phase = PhaseBetween
		return nil
	}
	s.startFinal()
	return nil
}

// --- final chase -----------------------------------------------------------

func (s *State) startFinal() {
	s.CurrentID = ""
	var fin []string
	for _, p := range s.Players {
		p.Vote = ""
		if p.Status == StatusHome {
			fin = append(fin, p.ID)
		}
	}
	s.Final = FinalState{Finalists: fin, Head: len(fin)}
	if len(fin) == 0 {
		s.gameOver("chaser")
		return
	}
	s.Phase = PhaseFinalPick
	s.logf("final chase: pick a question set")
}

func (s *State) isFinalist(id string) bool {
	for _, f := range s.Final.Finalists {
		if f == id {
			return true
		}
	}
	return false
}

func (s *State) pickSet(pid, set string) error {
	if s.Phase != PhaseFinalPick {
		return errors.New("sets are not being chosen")
	}
	if !s.isFinalist(pid) {
		return errors.New("only players still in the game can vote")
	}
	if set != "A" && set != "B" {
		return errors.New("choose set A or B")
	}
	s.player(pid).Vote = set
	return nil
}

func (s *State) confirmSets() error {
	if s.Phase != PhaseFinalPick {
		return errors.New("sets are not being chosen")
	}
	a, b := 0, 0
	for _, id := range s.Final.Finalists {
		switch s.player(id).Vote {
		case "A":
			a++
		case "B":
			b++
		}
	}
	team := "A"
	switch {
	case b > a:
		team = "B"
	case a == b && s.rng.Intn(2) == 1:
		team = "B"
	}
	s.Final.TeamSet = team
	s.Final.ChaserSet = map[string]string{"A": "B", "B": "A"}[team]
	s.Phase = PhaseFinalTeamReady
	s.logf("team plays set " + team)
	return nil
}

func (s *State) nextFinalQ(set string) {
	q := s.finalDeck[set].next()
	s.Final.Q = &q
	s.Final.QNum++
	s.Final.BuzzedBy = ""
}

func (s *State) startClock(now time.Time) {
	s.Final.Running = true
	s.Final.EndsAt = now.Add(time.Duration(s.cfg.FinalSeconds) * time.Second)
	s.Final.Remaining = 0
}

func (s *State) startFinalTeam(now time.Time) error {
	if s.Phase != PhaseFinalTeamReady {
		return errors.New("the final chase is not ready")
	}
	s.Final.TeamCorrect, s.Final.QNum = 0, 0
	s.startClock(now)
	s.nextFinalQ(s.Final.TeamSet)
	s.Phase = PhaseFinalTeam
	return nil
}

func (s *State) startChaserRound(now time.Time) error {
	if s.Phase != PhaseFinalTeamDone {
		return errors.New("the team round is not finished")
	}
	s.Final.ChaserCorrect, s.Final.Pushbacks, s.Final.QNum = 0, 0, 0
	s.startClock(now)
	s.nextFinalQ(s.Final.ChaserSet)
	s.Phase = PhaseFinalChaser
	s.logf(fmt.Sprintf("chaser needs %d", s.Final.Target()))
	return nil
}

// buzz gives the floor to the first finalist to press. No steals: once the
// host marks the answer the question moves on.
func (s *State) buzz(pid string) error {
	if s.Phase != PhaseFinalTeam && s.Phase != PhaseFinalPush {
		return errors.New("buzzers are not live")
	}
	if !s.isFinalist(pid) {
		return errors.New("you are not in the final chase")
	}
	if s.Final.BuzzedBy != "" {
		return errors.New("someone has already buzzed")
	}
	s.Final.BuzzedBy = pid
	return nil
}

// markFinal is the host's right/wrong ruling in every final-chase phase.
func (s *State) markFinal(correct bool, now time.Time) error {
	f := &s.Final
	switch s.Phase {
	case PhaseFinalTeam:
		if !now.Before(f.EndsAt) {
			s.endTeamRound()
			return errors.New("time was already up")
		}
		if correct {
			f.TeamCorrect++
		}
		s.nextFinalQ(f.TeamSet)

	case PhaseFinalChaser:
		if !now.Before(f.EndsAt) {
			s.gameOver("team")
			return errors.New("time was already up")
		}
		if correct {
			f.ChaserCorrect++
			if f.ChaserCorrect >= f.Target() {
				s.gameOver("chaser")
				return nil
			}
			s.nextFinalQ(f.ChaserSet)
			return nil
		}
		// Chaser missed: stop the clock; the team may answer the same question.
		f.Remaining = f.EndsAt.Sub(now)
		f.Running = false
		f.BuzzedBy = ""
		s.Phase = PhaseFinalPush

	case PhaseFinalPush:
		if correct {
			f.Pushbacks++
		}
		s.nextFinalQ(f.ChaserSet)
		f.Running = true
		f.EndsAt = now.Add(f.Remaining)
		f.Remaining = 0
		s.Phase = PhaseFinalChaser

	default:
		return errors.New("nothing to mark right now")
	}
	return nil
}

func (s *State) endTeamRound() {
	s.Final.Running = false
	s.Final.Q = nil
	s.Final.BuzzedBy = ""
	s.Phase = PhaseFinalTeamDone
	s.logf(fmt.Sprintf("team scored %d (target %d)", s.Final.TeamCorrect, s.Final.Target()))
}

func (s *State) gameOver(winner string) {
	f := &s.Final
	f.Running, f.Q, f.BuzzedBy = false, nil, ""
	r := &Result{Winner: winner, Finalists: f.Finalists}
	r.Bank = s.bankTotal()
	if winner == "team" && len(f.Finalists) > 0 {
		r.PerPlayer = r.Bank / len(f.Finalists)
	} else {
		r.Bank, r.PerPlayer = 0, 0
	}
	s.Result = r
	s.Phase = PhaseGameOver
	s.logf(winner + " wins")
}

// --- dispatch --------------------------------------------------------------

// Tick advances anything that depends on the clock. It reports whether the
// state changed so the caller knows whether to broadcast.
func (s *State) Tick(now time.Time) bool {
	switch s.Phase {
	case PhaseCBPlaying:
		if !now.Before(s.CB.EndsAt) {
			s.finishCB()
			return true
		}
	case PhaseH2HQuestion:
		if !s.H2H.Deadline.IsZero() && !now.Before(s.H2H.Deadline) {
			s.reveal(now)
			return true
		}
	case PhaseH2HReveal:
		if !now.Before(s.H2H.RevealUntil) {
			if s.H2H.Outcome != "" {
				s.finishH2H()
			} else {
				s.nextH2HQuestion()
			}
			return true
		}
	case PhaseFinalTeam:
		if !now.Before(s.Final.EndsAt) {
			s.endTeamRound()
			return true
		}
	case PhaseFinalChaser:
		if !now.Before(s.Final.EndsAt) {
			s.gameOver("team")
			return true
		}
	}
	return false
}

// Control runs a quizmaster action.
func (s *State) Control(action string, a Args, now time.Time) error {
	switch action {
	case "setChaser":
		return s.setChaser(a.Str("id"))
	case "kick":
		return s.kick(a.Str("id"))
	case "startGame":
		return s.startGame()
	case "startPlayer":
		return s.startPlayer(a.Str("id"))
	case "startCashBuilder":
		return s.startCashBuilder(now)
	case "markCB":
		return s.markCB(a.Bool("correct"), now)
	case "toOffers":
		return s.toOffers()
	case "setOffers":
		lo, ok1 := a.Int("lower")
		hi, ok2 := a.Int("higher")
		if !ok1 || !ok2 {
			return errors.New("enter both offers")
		}
		return s.setOffers(lo, hi)
	case "chooseOffer":
		return s.chooseOffer(a.Str("choice"))
	case "startH2H":
		return s.startH2H()
	case "lockOut":
		return s.lockOut(now)
	case "afterH2H":
		return s.afterH2H(now)
	case "confirmSets":
		return s.confirmSets()
	case "startFinalTeam":
		return s.startFinalTeam(now)
	case "startChaserRound":
		return s.startChaserRound(now)
	case "markFinal":
		return s.markFinal(a.Bool("correct"), now)
	case "playAgain":
		return s.playAgain()
	}
	return errors.New("unknown action")
}

// Act runs a player-phone action. pid has already been authenticated.
func (s *State) Act(pid, action string, a Args, now time.Time) error {
	isChaser := pid == s.ChaserID
	isCurrent := pid == s.CurrentID
	switch action {
	case "lock":
		if !isChaser && !isCurrent {
			return errors.New("you are not in this head-to-head")
		}
		c, ok := a.Int("choice")
		if !ok {
			return errors.New("pick an answer")
		}
		return s.lockIn(isChaser, c, now)
	case "setOffers":
		if !isChaser {
			return errors.New("only the chaser sets offers")
		}
		lo, ok1 := a.Int("lower")
		hi, ok2 := a.Int("higher")
		if !ok1 || !ok2 {
			return errors.New("enter both offers")
		}
		return s.setOffers(lo, hi)
	case "chooseOffer":
		if !isCurrent {
			return errors.New("only the player in the hot seat chooses")
		}
		return s.chooseOffer(a.Str("choice"))
	case "buzz":
		return s.buzz(pid)
	case "pickSet":
		return s.pickSet(pid, a.Str("set"))
	}
	return errors.New("unknown action")
}
