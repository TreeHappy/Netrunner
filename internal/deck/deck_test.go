package deck

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"boardy/netrunner/internal/carddb"
)

func ni(v int64) sql.NullInt64 {
	return sql.NullInt64{Int64: v, Valid: true}
}

func mkCard(code, side, faction, typ string, infCost, ap int) carddb.Card {
	c := carddb.Card{Code: code, Title: "T" + code, Side: side, Faction: faction, Type: typ}
	if infCost >= 0 {
		c.InfluenceCost = ni(int64(infCost))
	}
	if ap > 0 {
		c.AgendaPoints = ni(int64(ap))
	}
	return c
}

func runnerIdentity() carddb.Card {
	id := mkCard("00001", "runner", "anarch", "identity", -1, 0)
	id.InfluenceLimit = ni(15)
	return id
}

func corpIdentity() carddb.Card {
	id := mkCard("00002", "corp", "haas-bioroid", "identity", -1, 0)
	id.InfluenceLimit = ni(15)
	return id
}

func TestValidateRunner(t *testing.T) {
	d := &Deck{Identity: runnerIdentity()}
	if len(d.Validate()) == 0 {
		t.Fatal("empty deck should fail")
	}
	inFaction := mkCard("10001", "runner", "anarch", "resource", 0, 0)
	d.SetQty(inFaction, 3)
	for i := 0; i < 14; i++ {
		d.SetQty(mkCard(fmt.Sprintf("1%04d", 100+i), "runner", "anarch", "resource", 0, 0), 3)
	}
	outFaction := mkCard("20001", "runner", "criminal", "program", 2, 0)
	d.Add(outFaction)
	d.Add(outFaction)

	if issues := d.Validate(); len(issues) != 0 {
		t.Fatalf("expected legal deck, got %v", issues)
	}
	if d.Influence() != 4 {
		t.Fatalf("influence = %d, want 4", d.Influence())
	}

	d.SetQty(outFaction, 15)
	issues := d.Validate()
	if len(issues) == 0 || !contains(issues, "influence") {
		t.Fatalf("expected influence issue, got %v", issues)
	}
	if !contains(issues, "limit") {
		t.Fatalf("expected copy-limit issue, got %v", issues)
	}
}

func TestCopiesLimitAndUnique(t *testing.T) {
	d := &Deck{Identity: runnerIdentity()}
	c := mkCard("10002", "runner", "anarch", "event", 0, 0)
	for i := 0; i < 3; i++ {
		if !d.Add(c) {
			t.Fatal("add should succeed up to limit")
		}
	}
	if d.Add(c) {
		t.Fatal("4th copy should be rejected")
	}
	if d.SetQty(c, 5); d.CopiesOf(c.Code) != 5 {
		t.Fatal("SetQty should bypass Add's cap")
	}
	u := mkCard("10003", "runner", "anarch", "hardware", 0, 0)
	u.Uniqueness = true
	d.SetQty(u, 2)
	d.SetQty(c, 3)
	issues := d.Validate()
	if !contains(issues, "unique") {
		t.Fatalf("expected uniqueness issue, got %v", issues)
	}
}

func TestAgendaRange(t *testing.T) {
	d := &Deck{Identity: corpIdentity()}
	ag := mkCard("30001", "corp", "haas-bioroid", "agenda", -1, 2)
	for _, n := range []struct{ size, lo int }{{45, 18}, {49, 18}, {50, 20}, {54, 20}, {55, 22}} {
		d.Entries = nil
		fill := mkCard("30002", "corp", "haas-bioroid", "operation", 0, 0)
		// agendas contribute 2 points each; pad with fillers to exact size
		nAgenda := n.size / 2
		if n.size%2 != 0 {
			nAgenda-- // keep agenda count under the 3-copy limit for small sizes
		}
		d.SetQty(ag, min(nAgenda, 3))
		d.SetQty(fill, n.size-min(nAgenda, 3))
		lo, _ := d.AgendaRange()
		if lo != n.lo {
			t.Errorf("size %d: agenda min = %d, want %d", d.Size(), lo, n.lo)
		}
	}
}

func contains(issues []Issue, sub string) bool {
	for _, i := range issues {
		if strings.Contains(i.Msg, sub) {
			return true
		}
	}
	return false
}
