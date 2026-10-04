// Package bank loads and validates the question banks.
//
// Banks are plain text files, one question per line, so they are easy to
// edit and extend without touching code:
//
//	cashbuilder.txt  Question | Answer
//	final_a.txt      Question | Answer
//	final_b.txt      Question | Answer
//	headtohead.txt   Question | Correct answer | Wrong answer | Wrong answer
//
// Blank lines and lines starting with # are ignored.
package bank

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed data/*.txt
var embedded embed.FS

// QA is an open question (cash builder and final chase): the host reads it
// aloud and judges the spoken answer against A.
type QA struct {
	Q string
	A string
}

// MC is a three-choice question for the head-to-head. Options[Answer] is the
// correct option. As stored in the bank, Answer is always 0; the room shuffles
// the options each time a question is asked.
type MC struct {
	Q       string
	Options [3]string
	Answer  int
}

// Set is a complete question bank for one game.
type Set struct {
	CashBuilder []QA
	HeadToHead  []MC
	FinalA      []QA
	FinalB      []QA
}

// Minimum sizes so a normal game rarely sees a repeated question.
const (
	MinCashBuilder = 20
	MinHeadToHead  = 20
	MinFinal       = 30
)

func cleanLines(src string) []string {
	var out []string
	for _, ln := range strings.Split(src, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		out = append(out, ln)
	}
	return out
}

func splitFields(ln string, want int) ([]string, error) {
	parts := strings.Split(ln, "|")
	if len(parts) != want {
		return nil, fmt.Errorf("expected %d |-separated fields, got %d in %q", want, len(parts), ln)
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
		if parts[i] == "" {
			return nil, fmt.Errorf("empty field in %q", ln)
		}
	}
	return parts, nil
}

// ParseQA parses "Question | Answer" lines.
func ParseQA(src string) ([]QA, error) {
	var out []QA
	for _, ln := range cleanLines(src) {
		p, err := splitFields(ln, 2)
		if err != nil {
			return nil, err
		}
		out = append(out, QA{Q: p[0], A: p[1]})
	}
	return out, nil
}

// ParseMC parses "Question | Correct | Wrong | Wrong" lines.
func ParseMC(src string) ([]MC, error) {
	var out []MC
	for _, ln := range cleanLines(src) {
		p, err := splitFields(ln, 4)
		if err != nil {
			return nil, err
		}
		if p[1] == p[2] || p[1] == p[3] || p[2] == p[3] {
			return nil, fmt.Errorf("duplicate options in %q", ln)
		}
		out = append(out, MC{Q: p[0], Options: [3]string{p[1], p[2], p[3]}, Answer: 0})
	}
	return out, nil
}

// Load reads the banks from dir, or from the embedded defaults when dir is "".
func Load(dir string) (*Set, error) {
	read := func(name string) (string, error) {
		if dir == "" {
			b, err := embedded.ReadFile("data/" + name)
			return string(b), err
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		return string(b), err
	}
	var s Set
	var err error
	var src string

	if src, err = read("cashbuilder.txt"); err != nil {
		return nil, err
	}
	if s.CashBuilder, err = ParseQA(src); err != nil {
		return nil, fmt.Errorf("cashbuilder.txt: %w", err)
	}
	if src, err = read("headtohead.txt"); err != nil {
		return nil, err
	}
	if s.HeadToHead, err = ParseMC(src); err != nil {
		return nil, fmt.Errorf("headtohead.txt: %w", err)
	}
	if src, err = read("final_a.txt"); err != nil {
		return nil, err
	}
	if s.FinalA, err = ParseQA(src); err != nil {
		return nil, fmt.Errorf("final_a.txt: %w", err)
	}
	if src, err = read("final_b.txt"); err != nil {
		return nil, err
	}
	if s.FinalB, err = ParseQA(src); err != nil {
		return nil, fmt.Errorf("final_b.txt: %w", err)
	}
	return &s, s.Validate()
}

// Validate checks every bank is large enough to run a game.
func (s *Set) Validate() error {
	switch {
	case len(s.CashBuilder) < MinCashBuilder:
		return fmt.Errorf("cash builder bank has %d questions, need at least %d", len(s.CashBuilder), MinCashBuilder)
	case len(s.HeadToHead) < MinHeadToHead:
		return fmt.Errorf("head-to-head bank has %d questions, need at least %d", len(s.HeadToHead), MinHeadToHead)
	case len(s.FinalA) < MinFinal:
		return fmt.Errorf("final set A has %d questions, need at least %d", len(s.FinalA), MinFinal)
	case len(s.FinalB) < MinFinal:
		return fmt.Errorf("final set B has %d questions, need at least %d", len(s.FinalB), MinFinal)
	}
	return nil
}
