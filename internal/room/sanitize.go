package room

import "time"

// Viewer identifies who a snapshot is for. This file is the ONE place answer
// redaction happens: only the host ever receives answers before they are
// revealed. Do not hand-build a different payload elsewhere.
type Viewer struct {
	Role     string // "screen" | "host" | "player"
	PlayerID string // set for role "player"
}

type PlayerView struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	IsChaser  bool    `json:"isChaser"`
	Connected bool    `json:"connected"`
	Status    PStatus `json:"status"`
	CashBuilt int     `json:"cashBuilt"`
	Banked    int     `json:"banked"`
	Queued    bool    `json:"queued"` // still to play
}

type MeView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsChaser  bool   `json:"isChaser"`
	IsCurrent bool   `json:"isCurrent"` // in the hot seat right now
	Finalist  bool   `json:"finalist"`
	Vote      string `json:"vote,omitempty"`
}

type CfgView struct {
	BoardSteps  int `json:"boardSteps"`
	Home        int `json:"home"`
	CBSeconds   int `json:"cbSeconds"`
	FinalSecs   int `json:"finalSeconds"`
	StartLower  int `json:"startLower"`
	StartMiddle int `json:"startMiddle"`
	StartHigher int `json:"startHigher"`
	CashPerQ    int `json:"cashPerCorrect"`
}

type CBView struct {
	Correct  int    `json:"correct"`
	Total    int    `json:"total"`
	Asked    int    `json:"asked"`
	EndsAt   int64  `json:"endsAt,omitempty"`
	Question string `json:"question,omitempty"`
	Answer   string `json:"answer,omitempty"` // host only
}

type OffersView struct {
	Lower         int    `json:"lower"`
	Middle        int    `json:"middle"`
	Higher        int    `json:"higher"`
	SuggestLower  int    `json:"suggestLower"`
	SuggestHigher int    `json:"suggestHigher"`
	Choice        string `json:"choice,omitempty"`
	Amount        int    `json:"amount"`
	Set           bool   `json:"set"` // offers have been made
}

type H2HView struct {
	Start        int      `json:"start"`
	PlayerPos    int      `json:"playerPos"`
	ChaserPos    int      `json:"chaserPos"`
	QNum         int      `json:"qNum"`
	Question     string   `json:"question,omitempty"`
	Options      []string `json:"options,omitempty"`
	Correct      *int     `json:"correct,omitempty"` // host always; everyone after the reveal
	PlayerLocked bool     `json:"playerLocked"`
	ChaserLocked bool     `json:"chaserLocked"`
	DeadlineAt   int64    `json:"deadlineAt,omitempty"`
	MyPick       *int     `json:"myPick,omitempty"`
	Reveal       *RevealV `json:"reveal,omitempty"`
	Outcome      string   `json:"outcome,omitempty"`
}

type RevealV struct {
	PlayerPick  int  `json:"playerPick"` // -2 = locked out
	ChaserPick  int  `json:"chaserPick"`
	PlayerRight bool `json:"playerRight"`
	ChaserRight bool `json:"chaserRight"`
}

type FinalView struct {
	Finalists     []string          `json:"finalists"`
	Voted         []string          `json:"voted"`
	Votes         map[string]string `json:"votes,omitempty"` // host only
	TeamSet       string            `json:"teamSet,omitempty"`
	ChaserSet     string            `json:"chaserSet,omitempty"`
	Head          int               `json:"head"`
	TeamCorrect   int               `json:"teamCorrect"`
	ChaserCorrect int               `json:"chaserCorrect"`
	Pushbacks     int               `json:"pushbacks"`
	Target        int               `json:"target"`
	Question      string            `json:"question,omitempty"`
	Answer        string            `json:"answer,omitempty"` // host only
	QNum          int               `json:"qNum"`
	BuzzedBy      string            `json:"buzzedBy,omitempty"`
	Running       bool              `json:"running"`
	EndsAt        int64             `json:"endsAt,omitempty"`
	RemainingMs   int64             `json:"remainingMs,omitempty"`
}

type View struct {
	Code      string       `json:"code"`
	Phase     Phase        `json:"phase"`
	Now       int64        `json:"now"`
	Cfg       CfgView      `json:"cfg"`
	Players   []PlayerView `json:"players"`
	ChaserID  string       `json:"chaserId"`
	CurrentID string       `json:"currentId"`
	Bank      int          `json:"bank"`
	Me        *MeView      `json:"me,omitempty"`
	CB        *CBView      `json:"cb,omitempty"`
	Offers    *OffersView  `json:"offers,omitempty"`
	H2H       *H2HView     `json:"h2h,omitempty"`
	Final     *FinalView   `json:"final,omitempty"`
	Result    *Result      `json:"result,omitempty"`
	Log       []string     `json:"log,omitempty"` // host only
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// Snapshot builds the JSON-ready view of the game for one viewer.
func (s *State) Snapshot(v Viewer, now time.Time) View {
	host := v.Role == "host"
	out := View{
		Code: s.Code, Phase: s.Phase, Now: now.UnixMilli(),
		ChaserID: s.ChaserID, CurrentID: s.CurrentID, Bank: s.bankTotal(), Result: s.Result,
		Cfg: CfgView{
			BoardSteps: s.cfg.BoardSteps, Home: s.cfg.Home(),
			CBSeconds: s.cfg.CBSeconds, FinalSecs: s.cfg.FinalSeconds,
			StartLower: s.cfg.StartLower, StartMiddle: s.cfg.StartMiddle, StartHigher: s.cfg.StartHigher,
			CashPerQ: s.cfg.CashPerCorrect,
		},
	}

	queued := map[string]bool{}
	for _, id := range s.Queue {
		queued[id] = true
	}
	for _, p := range s.Players {
		out.Players = append(out.Players, PlayerView{
			ID: p.ID, Name: p.Name, IsChaser: p.ID == s.ChaserID, Connected: p.Connected,
			Status: p.Status, CashBuilt: p.CashBuilt, Banked: p.Banked, Queued: queued[p.ID],
		})
	}
	if out.Players == nil {
		out.Players = []PlayerView{}
	}

	if v.Role == "player" {
		if p := s.player(v.PlayerID); p != nil {
			out.Me = &MeView{
				ID: p.ID, Name: p.Name, IsChaser: p.ID == s.ChaserID, IsCurrent: p.ID == s.CurrentID,
				Finalist: s.isFinalist(p.ID), Vote: p.Vote,
			}
		}
	}
	if host {
		out.Log = s.Log
	}

	switch s.Phase {
	case PhaseCBReady, PhaseCBPlaying, PhaseCBDone:
		cb := &CBView{
			Correct: s.CB.Correct, Total: s.CB.Correct * s.cfg.CashPerCorrect,
			Asked: s.CB.Asked, EndsAt: ms(s.CB.EndsAt),
		}
		if s.CB.Q != nil {
			cb.Question = s.CB.Q.Q
			if host {
				cb.Answer = s.CB.Q.A
			}
		}
		out.CB = cb
	}

	switch s.Phase {
	case PhaseOffersSet, PhaseOffersChoose, PhaseH2HReady, PhaseH2HQuestion, PhaseH2HReveal, PhaseH2HOver:
		out.Offers = &OffersView{
			Lower: s.Offers.Lower, Middle: s.Offers.Middle, Higher: s.Offers.Higher,
			SuggestLower: s.Offers.SuggestLower, SuggestHigher: s.Offers.SuggestHigher,
			Choice: s.Offers.Choice, Amount: s.Offers.Amount(),
			Set: s.Phase != PhaseOffersSet,
		}
	}

	switch s.Phase {
	case PhaseH2HReady, PhaseH2HQuestion, PhaseH2HReveal, PhaseH2HOver:
		h := s.H2H
		hv := &H2HView{
			Start: h.Start, PlayerPos: h.PlayerPos, ChaserPos: h.ChaserPos, QNum: h.QNum,
			PlayerLocked: h.PlayerPick != pickNone, ChaserLocked: h.ChaserPick != pickNone,
			DeadlineAt: ms(h.Deadline), Outcome: "",
		}
		if h.Q != nil && s.Phase != PhaseH2HReady {
			hv.Question = h.Q.Q
			hv.Options = h.Q.Options[:]
		}
		revealed := s.Phase == PhaseH2HReveal || s.Phase == PhaseH2HOver
		if revealed {
			hv.Reveal = &RevealV{PlayerPick: h.PlayerPick, ChaserPick: h.ChaserPick,
				PlayerRight: h.PlayerRight, ChaserRight: h.ChaserRight}
			ans := h.Q.Answer
			hv.Correct = &ans
			hv.Outcome = h.Outcome
		} else if host && h.Q != nil {
			ans := h.Q.Answer
			hv.Correct = &ans
		}
		// Players see their own pick before the reveal, never the other side's.
		if v.Role == "player" && h.Q != nil {
			pick := pickNone
			switch v.PlayerID {
			case s.ChaserID:
				pick = h.ChaserPick
			case s.CurrentID:
				pick = h.PlayerPick
			}
			if pick >= 0 {
				hv.MyPick = &pick
			}
		}
		out.H2H = hv
	}

	switch s.Phase {
	case PhaseFinalPick, PhaseFinalTeamReady, PhaseFinalTeam, PhaseFinalTeamDone,
		PhaseFinalChaser, PhaseFinalPush, PhaseGameOver:
		f := s.Final
		fv := &FinalView{
			Finalists: append([]string{}, f.Finalists...), Voted: []string{},
			TeamSet: f.TeamSet, ChaserSet: f.ChaserSet, Head: f.Head,
			TeamCorrect: f.TeamCorrect, ChaserCorrect: f.ChaserCorrect, Pushbacks: f.Pushbacks,
			Target: f.Target(), QNum: f.QNum, BuzzedBy: f.BuzzedBy,
			Running: f.Running, EndsAt: ms(f.EndsAt), RemainingMs: f.Remaining.Milliseconds(),
		}
		if !f.Running {
			fv.EndsAt = 0
		}
		for _, id := range f.Finalists {
			if p := s.player(id); p != nil && p.Vote != "" {
				fv.Voted = append(fv.Voted, id)
			}
		}
		if host {
			fv.Votes = map[string]string{}
			for _, id := range f.Finalists {
				if p := s.player(id); p != nil && p.Vote != "" {
					fv.Votes[id] = p.Vote
				}
			}
		}
		if f.Q != nil {
			fv.Question = f.Q.Q
			if host {
				fv.Answer = f.Q.A
			}
		}
		out.Final = fv
	}
	return out
}
