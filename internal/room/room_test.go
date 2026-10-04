package room

import (
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"strings"
	"testing"
	"time"

	"chase/internal/bank"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func testBank() *bank.Set {
	set := &bank.Set{}
	for i := 0; i < 40; i++ {
		set.CashBuilder = append(set.CashBuilder, bank.QA{Q: fmt.Sprintf("cb%d?", i), A: fmt.Sprintf("cba%d", i)})
		set.HeadToHead = append(set.HeadToHead, bank.MC{Q: fmt.Sprintf("hh%d?", i), Options: [3]string{"right", "w1", "w2"}})
		set.FinalA = append(set.FinalA, bank.QA{Q: fmt.Sprintf("fa%d?", i), A: fmt.Sprintf("faa%d", i)})
		set.FinalB = append(set.FinalB, bank.QA{Q: fmt.Sprintf("fb%d?", i), A: fmt.Sprintf("fba%d", i)})
	}
	return set
}

// lobby with a chaser and n contestants, game started, first contestant up.
func started(t *testing.T, n int) (*State, *Player, []*Player) {
	t.Helper()
	s := NewState("TEST", "key", testBank(), DefaultConfig(), mrand.New(mrand.NewSource(1)))
	chaser, err := s.Join("Chaser")
	if err != nil {
		t.Fatal(err)
	}
	var cons []*Player
	for i := 0; i < n; i++ {
		p, err := s.Join(fmt.Sprintf("P%d", i+1))
		if err != nil {
			t.Fatal(err)
		}
		cons = append(cons, p)
	}
	must(t, s.Control("setChaser", Args{"id": chaser.ID}, t0))
	must(t, s.Control("startGame", nil, t0))
	return s, chaser, cons
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantErr(t *testing.T, err error, contains string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("want error containing %q, got %v", contains, err)
	}
}

// cashBuild runs the current player's cash builder with n correct answers.
func cashBuild(t *testing.T, s *State, id string, n int) {
	t.Helper()
	must(t, s.Control("startPlayer", Args{"id": id}, t0))
	must(t, s.Control("startCashBuilder", nil, t0))
	for i := 0; i < n; i++ {
		must(t, s.Control("markCB", Args{"correct": true}, t0.Add(time.Second)))
	}
	must(t, s.Control("markCB", Args{"correct": false}, t0.Add(2*time.Second)))
	if !s.Tick(t0.Add(61 * time.Second)) {
		t.Fatal("cash builder should end after 60s")
	}
}

// toBoard takes the current player through offers to a ready board.
func toBoard(t *testing.T, s *State, chaser *Player, choice string) {
	t.Helper()
	must(t, s.Control("toOffers", nil, t0))
	must(t, s.Act(chaser.ID, "setOffers", Args{"lower": float64(s.Offers.SuggestLower), "higher": float64(s.Offers.SuggestHigher)}, t0))
	must(t, s.Act(s.CurrentID, "chooseOffer", Args{"choice": choice}, t0))
	must(t, s.Control("startH2H", nil, t0))
}

// question has both sides answer. right: true = correct, false = wrong.
func question(t *testing.T, s *State, chaser *Player, playerRight, chaserRight bool, now time.Time) {
	t.Helper()
	ans := s.H2H.Q.Answer
	pick := func(right bool) float64 {
		if right {
			return float64(ans)
		}
		return float64((ans + 1) % 3)
	}
	must(t, s.Act(s.CurrentID, "lock", Args{"choice": pick(playerRight)}, now))
	must(t, s.Act(chaser.ID, "lock", Args{"choice": pick(chaserRight)}, now))
	if s.Phase != PhaseH2HReveal {
		t.Fatalf("expected reveal, got %s", s.Phase)
	}
	s.Tick(now.Add(s.cfg.RevealFor)) // next question, or over
}

func TestLobbyRules(t *testing.T) {
	s := NewState("TEST", "key", testBank(), DefaultConfig(), mrand.New(mrand.NewSource(1)))
	_, err := s.Join("  ")
	wantErr(t, err, "name")
	a, _ := s.Join("Ann")
	_, err = s.Join("ann")
	wantErr(t, err, "taken")
	wantErr(t, s.Control("startGame", nil, t0), "chaser")
	must(t, s.Control("setChaser", Args{"id": a.ID}, t0))
	wantErr(t, s.Control("startGame", nil, t0), "contestant")
	for i := 0; i < 4; i++ {
		_, err = s.Join(fmt.Sprintf("P%d", i))
		must(t, err)
	}
	_, err = s.Join("Extra")
	wantErr(t, err, "full")
}

func TestCashBuilderTotals(t *testing.T) {
	s, _, cons := started(t, 2)
	cashBuild(t, s, cons[0].ID, 9)
	if s.Phase != PhaseCBDone || cons[0].CashBuilt != 9000 {
		t.Fatalf("phase=%s built=%d", s.Phase, cons[0].CashBuilt)
	}
	// marks after the clock are rejected
	wantErr(t, s.Control("markCB", Args{"correct": true}, t0.Add(90*time.Second)), "not running")
}

func TestOfferValidation(t *testing.T) {
	s, chaser, cons := started(t, 1)
	cashBuild(t, s, cons[0].ID, 9)
	must(t, s.Control("toOffers", nil, t0))
	if s.Offers.Middle != 9000 || s.Offers.Higher <= 9000 || s.Offers.Lower >= 9000 {
		t.Fatalf("bad suggestions %+v", s.Offers)
	}
	wantErr(t, s.Act(cons[0].ID, "setOffers", Args{"lower": 1000.0, "higher": 20000.0}, t0), "only the chaser")
	wantErr(t, s.Act(chaser.ID, "setOffers", Args{"lower": 1000.0, "higher": 9000.0}, t0), "higher")
	wantErr(t, s.Act(chaser.ID, "setOffers", Args{"lower": 9000.0, "higher": 20000.0}, t0), "lower")
	must(t, s.Act(chaser.ID, "setOffers", Args{"lower": 3000.0, "higher": 25000.0}, t0))
	wantErr(t, s.Act(chaser.ID, "chooseOffer", Args{"choice": "higher"}, t0), "hot seat")
	must(t, s.Act(cons[0].ID, "chooseOffer", Args{"choice": "higher"}, t0))
	if s.H2H.PlayerPos != 2 || s.Offers.Amount() != 25000 {
		t.Fatalf("higher offer should start on step 2 for 25000, got %d / %d", s.H2H.PlayerPos, s.Offers.Amount())
	}
}

func TestStepsToHome(t *testing.T) {
	// The show: lower offer = 4 steps down (needs 4 correct), standard = 3 (5), higher = 2 (6).
	// Home is step 8 on a 7-step board.
	cases := map[string]int{"lower": 4, "middle": 5, "higher": 6}
	for choice, need := range cases {
		t.Run(choice, func(t *testing.T) {
			s, chaser, cons := started(t, 1)
			cashBuild(t, s, cons[0].ID, 6)
			toBoard(t, s, chaser, choice)
			now := t0.Add(100 * time.Second)
			for i := 1; i <= need; i++ {
				if s.Phase != PhaseH2HQuestion {
					t.Fatalf("q%d: phase %s", i, s.Phase)
				}
				// chaser also answers right so only the player's progress decides it
				question(t, s, chaser, true, true, now)
			}
			if s.Phase != PhaseH2HOver || cons[0].Status != StatusHome {
				t.Fatalf("after %d correct want home, phase=%s status=%s", need, s.Phase, cons[0].Status)
			}
			if cons[0].Banked != s.Offers.Amount() || s.bankTotal() != s.Offers.Amount() {
				t.Fatal("banked amount should equal the chosen offer")
			}
		})
	}
}

func TestCaughtWhenChaserReachesPlayer(t *testing.T) {
	s, chaser, cons := started(t, 1)
	cashBuild(t, s, cons[0].ID, 6)
	toBoard(t, s, chaser, "middle") // player on 3, chaser on 0
	now := t0.Add(100 * time.Second)
	question(t, s, chaser, false, true, now)
	question(t, s, chaser, false, true, now)
	if s.Phase != PhaseH2HQuestion {
		t.Fatalf("chaser on 2, player on 3: still alive, got %s", s.Phase)
	}
	question(t, s, chaser, false, true, now) // chaser lands on 3
	if s.Phase != PhaseH2HOver || cons[0].Status != StatusCaught || cons[0].Banked != 0 {
		t.Fatalf("want caught, phase=%s status=%s", s.Phase, cons[0].Status)
	}
}

func TestPlayerGetsAwayWhenBothAnswerCorrectly(t *testing.T) {
	// A correct answer from the chaser only matters if the player doesn't match it.
	s, chaser, cons := started(t, 1)
	cashBuild(t, s, cons[0].ID, 6)
	toBoard(t, s, chaser, "middle")
	now := t0.Add(100 * time.Second)
	question(t, s, chaser, true, false, now)
	question(t, s, chaser, false, true, now)
	question(t, s, chaser, true, true, now)
	if s.H2H.PlayerPos != 5 || s.H2H.ChaserPos != 2 {
		t.Fatalf("positions %d/%d", s.H2H.PlayerPos, s.H2H.ChaserPos)
	}
}

func TestLockWindowAndLockOut(t *testing.T) {
	s, chaser, cons := started(t, 1)
	cashBuild(t, s, cons[0].ID, 6)
	toBoard(t, s, chaser, "middle")
	now := t0.Add(100 * time.Second)
	ans := float64(s.H2H.Q.Answer)

	must(t, s.Act(cons[0].ID, "lock", Args{"choice": ans}, now))
	wantErr(t, s.Act(cons[0].ID, "lock", Args{"choice": ans}, now), "already")
	if s.Tick(now.Add(4900 * time.Millisecond)) {
		t.Fatal("chaser still has time")
	}
	if !s.Tick(now.Add(5 * time.Second)) {
		t.Fatal("chaser should be locked out after 5s")
	}
	if s.H2H.ChaserPick != pickTimed || !s.H2H.PlayerRight || s.H2H.ChaserRight {
		t.Fatalf("lock-out not applied: %+v", s.H2H)
	}
	if s.H2H.PlayerPos != 4 {
		t.Fatalf("player should have moved to 4, got %d", s.H2H.PlayerPos)
	}
	// a non-participant cannot lock in
	s.Tick(now.Add(10 * time.Second))
	other := NewID()
	wantErr(t, s.Act(other, "lock", Args{"choice": 0.0}, now), "not in this")
	// host can force the reveal
	must(t, s.Control("lockOut", nil, now))
	if s.H2H.PlayerRight || s.H2H.ChaserRight {
		t.Fatal("nobody answered, nobody should be right")
	}
}

func TestSnapshotRedaction(t *testing.T) {
	s, chaser, cons := started(t, 1)
	cashBuild(t, s, cons[0].ID, 6)
	toBoard(t, s, chaser, "middle")
	now := t0.Add(100 * time.Second)

	// Mid-question: only the host may see the answer; picks stay private.
	ans := s.H2H.Q.Answer
	must(t, s.Act(cons[0].ID, "lock", Args{"choice": float64((ans + 1) % 3)}, now))
	host := s.Snapshot(Viewer{Role: "host"}, now)
	screen := s.Snapshot(Viewer{Role: "screen"}, now)
	chaserV := s.Snapshot(Viewer{Role: "player", PlayerID: chaser.ID}, now)
	playerV := s.Snapshot(Viewer{Role: "player", PlayerID: cons[0].ID}, now)

	if host.H2H.Correct == nil {
		t.Fatal("host should see the answer")
	}
	for name, v := range map[string]View{"screen": screen, "chaser": chaserV, "player": playerV} {
		if v.H2H.Correct != nil || v.H2H.Reveal != nil {
			t.Fatalf("%s must not see the answer before the reveal", name)
		}
	}
	if !screen.H2H.PlayerLocked || screen.H2H.ChaserLocked {
		t.Fatal("lock-in indicators wrong")
	}
	if chaserV.H2H.MyPick != nil {
		t.Fatal("chaser must not see the player's pick")
	}
	if playerV.H2H.MyPick == nil {
		t.Fatal("player should see their own pick")
	}

	// After the reveal everyone sees the answer.
	must(t, s.Act(chaser.ID, "lock", Args{"choice": float64(ans)}, now))
	screen = s.Snapshot(Viewer{Role: "screen"}, now)
	if screen.H2H.Correct == nil || screen.H2H.Reveal == nil {
		t.Fatal("reveal should publish the answer")
	}

	// Open-question answers never reach the screen or phones.
	s2, _, cons2 := started(t, 1)
	must(t, s2.Control("startPlayer", Args{"id": cons2[0].ID}, t0))
	must(t, s2.Control("startCashBuilder", nil, t0))
	b, _ := json.Marshal(s2.Snapshot(Viewer{Role: "screen"}, t0))
	if strings.Contains(string(b), "cba") {
		t.Fatal("cash builder answer leaked to the screen")
	}
	hb, _ := json.Marshal(s2.Snapshot(Viewer{Role: "host"}, t0))
	if !strings.Contains(string(hb), "cba") {
		t.Fatal("host should see the cash builder answer")
	}
}

// finalReady puts two home players into the final-chase vote.
func finalReady(t *testing.T) (*State, *Player, []*Player) {
	t.Helper()
	s, chaser, cons := started(t, 3)
	for i, p := range cons {
		cashBuild(t, s, p.ID, 6)
		toBoard(t, s, chaser, "middle")
		now := t0.Add(100 * time.Second)
		right := i < 2 // first two get home, third is caught
		for s.Phase == PhaseH2HQuestion {
			question(t, s, chaser, right, !right, now)
		}
		must(t, s.Control("afterH2H", nil, now))
	}
	if s.Phase != PhaseFinalPick || len(s.Final.Finalists) != 2 || s.Final.Head != 2 {
		t.Fatalf("phase=%s finalists=%d", s.Phase, len(s.Final.Finalists))
	}
	return s, chaser, cons
}

func TestFinalChaseFlow(t *testing.T) {
	s, _, cons := finalReady(t)
	if s.bankTotal() != 12000 { // 6,000 each (middle offer)
		t.Fatalf("bank %d", s.bankTotal())
	}

	wantErr(t, s.Act(cons[2].ID, "pickSet", Args{"set": "A"}, t0), "still in")
	must(t, s.Act(cons[0].ID, "pickSet", Args{"set": "B"}, t0))
	must(t, s.Act(cons[1].ID, "pickSet", Args{"set": "B"}, t0))
	must(t, s.Control("confirmSets", nil, t0))
	if s.Final.TeamSet != "B" || s.Final.ChaserSet != "A" {
		t.Fatalf("sets %s/%s", s.Final.TeamSet, s.Final.ChaserSet)
	}

	start := t0.Add(200 * time.Second)
	must(t, s.Control("startFinalTeam", nil, start))
	if !strings.HasPrefix(s.Final.Q.Q, "fb") {
		t.Fatalf("team should get set B, got %q", s.Final.Q.Q)
	}

	// Buzzer lockout: first buzz wins, no steals; eliminated players can't buzz.
	must(t, s.Act(cons[1].ID, "buzz", nil, start.Add(time.Second)))
	wantErr(t, s.Act(cons[0].ID, "buzz", nil, start.Add(time.Second)), "already buzzed")
	wantErr(t, s.Act(cons[2].ID, "buzz", nil, start.Add(time.Second)), "not in the final")
	if s.Final.BuzzedBy != cons[1].ID {
		t.Fatal("wrong buzzer owner")
	}
	must(t, s.Control("markFinal", Args{"correct": true}, start.Add(2*time.Second)))
	if s.Final.BuzzedBy != "" || s.Final.TeamCorrect != 1 {
		t.Fatal("marking should clear the buzzer and score")
	}
	must(t, s.Control("markFinal", Args{"correct": false}, start.Add(3*time.Second)))
	must(t, s.Control("markFinal", Args{"correct": true}, start.Add(4*time.Second)))
	must(t, s.Control("markFinal", Args{"correct": true}, start.Add(5*time.Second)))
	if s.Final.TeamCorrect != 3 || s.Final.Target() != 5 { // 3 correct + 2 head start
		t.Fatalf("team=%d target=%d", s.Final.TeamCorrect, s.Final.Target())
	}

	if !s.Tick(start.Add(120*time.Second)) || s.Phase != PhaseFinalTeamDone {
		t.Fatalf("team clock should end at 2 minutes, phase=%s", s.Phase)
	}

	// Chaser round with a pushback.
	cs := start.Add(130 * time.Second)
	must(t, s.Control("startChaserRound", nil, cs))
	if !strings.HasPrefix(s.Final.Q.Q, "fa") {
		t.Fatalf("chaser should get set A, got %q", s.Final.Q.Q)
	}
	must(t, s.Control("markFinal", Args{"correct": true}, cs.Add(5*time.Second)))
	// chaser misses at 30s: clock stops with 90s left
	must(t, s.Control("markFinal", Args{"correct": false}, cs.Add(30*time.Second)))
	if s.Phase != PhaseFinalPush || s.Final.Running || s.Final.Remaining != 90*time.Second {
		t.Fatalf("phase=%s running=%v remaining=%v", s.Phase, s.Final.Running, s.Final.Remaining)
	}
	if s.Tick(cs.Add(500 * time.Second)) {
		t.Fatal("clock must not run during a pushback")
	}
	// team buzzes and gets it right: chaser pushed back one step
	must(t, s.Act(cons[0].ID, "buzz", nil, cs.Add(40*time.Second)))
	resume := cs.Add(60 * time.Second)
	must(t, s.Control("markFinal", Args{"correct": true}, resume))
	if s.Final.Pushbacks != 1 || s.Final.Target() != 6 || s.Phase != PhaseFinalChaser {
		t.Fatalf("pushbacks=%d target=%d phase=%s", s.Final.Pushbacks, s.Final.Target(), s.Phase)
	}
	if !s.Final.EndsAt.Equal(resume.Add(90 * time.Second)) {
		t.Fatalf("clock should resume with 90s left, ends %v", s.Final.EndsAt)
	}

	// Clock runs out: team wins and splits the bank.
	s.Tick(resume.Add(90 * time.Second))
	if s.Phase != PhaseGameOver || s.Result.Winner != "team" || s.Result.Bank != 12000 || s.Result.PerPlayer != 6000 {
		t.Fatalf("result %+v phase %s", s.Result, s.Phase)
	}
}

func TestChaserCatchesTeam(t *testing.T) {
	s, _, cons := finalReady(t)
	must(t, s.Act(cons[0].ID, "pickSet", Args{"set": "A"}, t0))
	must(t, s.Control("confirmSets", nil, t0))
	start := t0.Add(200 * time.Second)
	must(t, s.Control("startFinalTeam", nil, start))
	must(t, s.Control("markFinal", Args{"correct": true}, start.Add(time.Second)))
	s.Tick(start.Add(120 * time.Second))
	cs := start.Add(130 * time.Second)
	must(t, s.Control("startChaserRound", nil, cs))
	// target is 2 head start + 1 = 3. Two correct isn't enough, the third catches.
	must(t, s.Control("markFinal", Args{"correct": true}, cs.Add(time.Second)))
	must(t, s.Control("markFinal", Args{"correct": true}, cs.Add(2*time.Second)))
	if s.Phase != PhaseFinalChaser {
		t.Fatalf("chaser still short, phase %s", s.Phase)
	}
	must(t, s.Control("markFinal", Args{"correct": true}, cs.Add(3*time.Second)))
	if s.Phase != PhaseGameOver || s.Result.Winner != "chaser" || s.Result.PerPlayer != 0 {
		t.Fatalf("chaser should win: %+v", s.Result)
	}
}

func TestEveryoneCaughtEndsGame(t *testing.T) {
	s, chaser, cons := started(t, 1)
	cashBuild(t, s, cons[0].ID, 6)
	toBoard(t, s, chaser, "middle")
	now := t0.Add(100 * time.Second)
	for s.Phase == PhaseH2HQuestion {
		question(t, s, chaser, false, true, now)
	}
	must(t, s.Control("afterH2H", nil, now))
	if s.Phase != PhaseGameOver || s.Result.Winner != "chaser" {
		t.Fatalf("phase %s", s.Phase)
	}
	must(t, s.Control("playAgain", nil, now))
	if s.Phase != PhaseLobby || s.ChaserID != chaser.ID {
		t.Fatal("play again should return to the lobby with the chaser kept")
	}
}
