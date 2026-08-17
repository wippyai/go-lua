package composite

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/census"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// TestGrammarDispositionJoinSeals is the account itself: every row the parser
// census derives from parser.go.y and the compiler AST declarations has a
// disposition here, every disposition names a row the census derives, and every
// mounted rule role is reached by at least one of them.
func TestGrammarDispositionJoinSeals(t *testing.T) {
	value := grammarCensus(t)
	if failure := JoinGrammarCensus(value); failure.Available() {
		t.Fatalf("grammar disposition join rejected: row=%q reason=%s role=%d", failure.Row, failure.Reason, failure.Role)
	}
	if len(value.Rows) != len(grammarDispositions) {
		t.Fatalf("census states %d rows, the table declares %d dispositions", len(value.Rows), len(grammarDispositions))
	}
}

// TestGrammarJoinRejectsRemovedCensusRow is the first catch arm. A parser
// alternative that leaves the language leaves its disposition behind, and a
// disposition for a row the language no longer has is exactly the stale
// accounting the census exists to expose.
func TestGrammarJoinRejectsRemovedCensusRow(t *testing.T) {
	value := grammarCensus(t)
	removed := GrammarRow(census.ProductionRow("stat#2"))
	kept := make([]GrammarRow, 0, len(value.Rows))
	for _, row := range value.Rows {
		if row == removed {
			continue
		}
		kept = append(kept, row)
	}
	if len(kept) != len(value.Rows)-1 {
		t.Fatalf("census does not state row %q", removed)
	}
	value.Rows = kept
	delete(value.Builds, removed)

	failure := JoinGrammarCensus(value)
	if !failure.Available() || failure.Reason != GrammarJoinForeignDisposition || failure.Row != removed {
		t.Fatalf("removed census row was accepted: row=%q reason=%s", failure.Row, failure.Reason)
	}
}

// TestGrammarJoinRejectsNewCensusRow is the second catch arm, and the one the
// deletion gate rests on. A parser alternative added tomorrow reaches the join
// as a census row nobody has accounted for, and the join must refuse to seal
// rather than quietly widen the language.
func TestGrammarJoinRejectsNewCensusRow(t *testing.T) {
	value := grammarCensus(t)
	added := GrammarRow(census.ProductionRow("stat#20"))
	for _, row := range value.Rows {
		if row == added {
			t.Fatalf("census already states %q; pick a row the parser does not have", added)
		}
	}
	value.Rows = append(append([]GrammarRow(nil), value.Rows...), added)

	failure := JoinGrammarCensus(value)
	if !failure.Available() || failure.Reason != GrammarJoinMissingDisposition || failure.Row != added {
		t.Fatalf("new census row was accepted: row=%q reason=%s", failure.Row, failure.Reason)
	}
}

// TestGrammarJoinRejectsDuplicateDisposition is the third catch arm. Two
// accounts of one row is not a redundant account: whichever of them the join
// happened to read would decide the row, so the table would state something no
// author had settled.
func TestGrammarJoinRejectsDuplicateDisposition(t *testing.T) {
	value := grammarCensus(t)
	duplicated := grammarDispositions[0]
	restore := grammarDispositions
	grammarDispositions = append(append([]grammarEntry(nil), grammarDispositions...), duplicated)
	t.Cleanup(func() { grammarDispositions = restore })

	failure := JoinGrammarCensus(value)
	if !failure.Available() || failure.Reason != GrammarJoinDuplicateDisposition || failure.Row != duplicated.Row {
		t.Fatalf("duplicate disposition was accepted: row=%q reason=%s", failure.Row, failure.Reason)
	}
}

// TestGrammarJoinRejectsRegeneratedCensus is the fourth catch arm. A semantic
// action rewritten in place keeps its production key, so row totality alone
// would still close; the pinned census authority is what makes that change
// reach an author. Regenerating the census without re-reading the rows it
// describes must therefore be refused.
func TestGrammarJoinRejectsRegeneratedCensus(t *testing.T) {
	value := grammarCensus(t)
	if value.Digest != grammarCensusAuthority {
		t.Fatalf("census digest %q is not the pinned authority %q", value.Digest, grammarCensusAuthority)
	}
	value.Digest = "0000000000000000000000000000000000000000000000000000000000000000"

	failure := JoinGrammarCensus(value)
	if !failure.Available() || failure.Reason != GrammarJoinCensusAuthorityChanged {
		t.Fatalf("a re-derived census was accepted: reason=%s", failure.Reason)
	}
}

// TestGrammarJoinRejectsIncoherentProduction states the law that keeps an
// authored role set tied to the census evidence. A production accounts for
// exactly the roles of the forms its reduction builds, so a hand edit that
// widens or narrows one row without the form it is derived from is refused.
func TestGrammarJoinRejectsIncoherentProduction(t *testing.T) {
	value := grammarCensus(t)
	target := GrammarRow(census.ProductionRow("stat#2"))
	restore := grammarDispositions
	edited := append([]grammarEntry(nil), grammarDispositions...)
	found := false
	for index := range edited {
		if edited[index].Row != target {
			continue
		}
		edited[index].Roles |= roleHeapEmpty
		found = true
	}
	if !found {
		t.Fatalf("no disposition declares %q", target)
	}
	grammarDispositions = edited
	t.Cleanup(func() { grammarDispositions = restore })

	failure := JoinGrammarCensus(value)
	if !failure.Available() || failure.Reason != GrammarJoinIncoherentProduction || failure.Row != target {
		t.Fatalf("incoherent production was accepted: row=%q reason=%s", failure.Row, failure.Reason)
	}
}

// TestGrammarJoinRejectsUndeclaredRole states the join's binding to the sealed
// rule surface. A disposition may only name a role the declaration table
// actually declares, so the account cannot claim a rule the analyzer does not
// have.
func TestGrammarJoinRejectsUndeclaredRole(t *testing.T) {
	value := grammarCensus(t)
	restore := grammarDispositions
	edited := append([]grammarEntry(nil), grammarDispositions...)
	// Every ordinal the artifact carries is declared by the table, so the one
	// role a disposition can name and the table cannot resolve is the invalid
	// ordinal. Naming it must be refused rather than read as an empty set.
	absent := programartifact.RuleRoleInvalid
	if _, declared := templateForRole(absent); declared {
		t.Fatalf("role %d is declared; pick a role the table does not declare", absent)
	}
	edited[0].Disposition = grammarRuleOwned
	edited[0].Reason = grammarReasonInvalid
	edited[0].Roles = grammarRoles(1) << absent
	grammarDispositions = edited
	t.Cleanup(func() { grammarDispositions = restore })

	failure := JoinGrammarCensus(value)
	if !failure.Available() || failure.Reason != GrammarJoinUndeclaredRole || failure.Role != absent {
		t.Fatalf("undeclared role was accepted: row=%q reason=%s role=%d", failure.Row, failure.Reason, failure.Role)
	}
}

// TestGrammarDispositionsReachEveryMountedRole states the reverse totality the
// join enforces, measured against the artifact's own mounted vocabulary rather
// than against the walk that produced it. A mounted rule no grammar row feeds
// would be a rule with no source in the language.
func TestGrammarDispositionsReachEveryMountedRole(t *testing.T) {
	reached := roleNone
	owned := 0
	for _, entry := range grammarDispositions {
		reached |= entry.Roles
		if entry.Disposition == grammarRuleOwned {
			owned++
		}
	}
	if owned == 0 {
		t.Fatal("no census row is owned by a rule")
	}
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, ok := programartifact.MountedRuleRoleAt(index)
		if !ok {
			t.Fatalf("mounted role %d is absent from the artifact vocabulary", index)
		}
		if !reached.has(role) {
			t.Fatalf("mounted role %d is reached by no grammar row", role)
		}
	}
	// The Link-owned bootstrap roles are admitted at the binding boundary and
	// are constructed by no parser alternative, so a grammar row claiming one
	// would be an invented provenance.
	for _, role := range LinkRoles() {
		if reached.has(role) {
			t.Fatalf("link-owned role %d is claimed by a grammar row", role)
		}
	}
}

// grammarCensus builds the join's input from the checked-in parser census,
// after that census has been validated against the current parser and AST
// source. A stale census fails here, so the join never closes against a
// language the compiler no longer parses.
func grammarCensus(t *testing.T) GrammarCensus {
	t.Helper()
	projection, err := census.CurrentProjection(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	rows := projection.Rows
	value := GrammarCensus{
		Digest:      projection.Digest,
		Rows:        make([]GrammarRow, 0, len(rows)),
		Builds:      make(map[GrammarRow][]GrammarRow),
		Declares:    make(map[GrammarRow]GrammarRow),
		Coordinates: make(map[GrammarRow]bool),
		Components:  make(map[GrammarRow]bool),
	}
	for _, row := range rows {
		key := GrammarRow(row.Key)
		value.Rows = append(value.Rows, key)
		switch row.Kind {
		case census.RowProduction:
			builds := make([]GrammarRow, 0, len(row.Builds))
			for _, form := range row.Builds {
				builds = append(builds, GrammarRow(form))
			}
			value.Builds[key] = builds
		case census.RowForm:
			if row.Class == parsersource.ConstructorStructural {
				value.Components[key] = true
			}
		case census.RowCarrier:
			value.Declares[key] = GrammarRow(row.Owner)
			if row.Coordinate {
				value.Coordinates[key] = true
			}
		default:
			t.Fatalf("census row %q has no grain", row.Key)
		}
	}
	return value
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate composite source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
