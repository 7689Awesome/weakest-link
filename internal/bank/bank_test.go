package bank

import "testing"

func TestEmbeddedBanksLoadAndValidate(t *testing.T) {
	s, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cash builder=%d head-to-head=%d finalA=%d finalB=%d",
		len(s.CashBuilder), len(s.HeadToHead), len(s.FinalA), len(s.FinalB))
}

func TestParseRejectsBadLines(t *testing.T) {
	if _, err := ParseQA("only a question"); err == nil {
		t.Fatal("expected error for missing answer")
	}
	if _, err := ParseMC("Q | a | b"); err == nil {
		t.Fatal("expected error for missing option")
	}
	if _, err := ParseMC("Q | a | a | b"); err == nil {
		t.Fatal("expected error for duplicate option")
	}
	got, err := ParseQA("# comment\n\nQ one? | A one\n")
	if err != nil || len(got) != 1 || got[0].A != "A one" {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestNoDuplicateQuestionsWithinABank(t *testing.T) {
	s, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	check := func(bank, q string) {
		if prev, dup := seen[q]; dup {
			t.Errorf("%q appears in both %s and %s", q, prev, bank)
		}
		seen[q] = bank
	}
	for _, q := range s.FinalA {
		check("final A", q.Q)
	}
	for _, q := range s.FinalB {
		check("final B", q.Q)
	}
}
