package engine

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// boundaryFixture builds a lowered body whose formals carry exactly the given
// declared types, keyed by the entry term each formal binds.
func boundaryFixture(declared map[string]typ.Type) front.Compilation {
	body := wir.NewBody("fixture")
	terms := make([]string, 0, len(declared))
	for term := range declared {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	boundary := wir.BodyBoundary{}
	for _, term := range terms {
		var symbol wir.SymbolID
		if _, err := fmt.Sscanf(term, "path/sym%d", &symbol); err != nil {
			continue
		}
		boundary.Parameters = append(boundary.Parameters, wir.BoundaryParameter{
			Symbol: symbol, Name: term, Type: body.InternType(declared[term]),
		})
	}
	body.SetBoundary(boundary)
	return front.Compilation{WIR: body, Boundary: body.Boundary()}
}

// TestDeclaredRecordIsAnIndexableContainer pins the declared types that denote a
// table cell. A record's fields are slots of a caller-owned table, so a write
// through a declared record formal reaches the same cell an array or map store
// would.
func TestDeclaredRecordIsAnIndexableContainer(t *testing.T) {
	record := typetable.NewRecord().Field("value", typ.String).Build()
	if !declaredIndexableContainer(record) {
		t.Fatal("declared record is not recognized as an indexable container")
	}
	if declaredIndexableContainer(typ.String) {
		t.Fatal("a scalar declaration was recognized as an indexable container")
	}
}

// TestDeclaredBoundaryIdentitySeedsMintOneSubjectPerContainer pins the subject a
// declaration-owned entry states for its own boundary cells: one per declared
// container, derived from the body and the term so the same declaration always
// names the same subject.
func TestDeclaredBoundaryIdentitySeedsMintOneSubjectPerContainer(t *testing.T) {
	body := equation.BodyID{7}
	seeds := []entrySeed{{Term: "path/sym1", Value: []byte("scalar/top")}, {Term: "path/sym2", Value: []byte("scalar/top")}}
	child := boundaryFixture(map[string]typ.Type{
		"path/sym1": typetable.NewRecord().Field("value", typ.String).Build(),
		"path/sym2": typ.String,
	})
	minted := declaredBoundaryIdentitySeeds(body, child, seeds, nil)
	if len(minted) != 1 || minted[0].Term != "path/sym1" {
		t.Fatalf("minted identity seeds = %#v, want one for the declared record", minted)
	}
	if string(minted[0].Identity) != string(declaredBoundaryIdentity(body, "path/sym1")) {
		t.Fatalf("minted identity = %q, want the declaration-derived subject", minted[0].Identity)
	}
}

// TestDeclaredBoundaryIdentitySeedsKeepAWithheldMapping pins the fail-closed
// half: an entry built from caller arguments carries the caller's own
// identities, so a term that already holds one keeps it and no second subject is
// invented for it.
func TestDeclaredBoundaryIdentitySeedsKeepAWithheldMapping(t *testing.T) {
	body := equation.BodyID{7}
	seeds := []entrySeed{{Term: "path/sym1", Value: []byte("scalar/top")}}
	child := boundaryFixture(map[string]typ.Type{"path/sym1": typetable.NewRecord().Field("value", typ.String).Build()})
	existing := []entryTableIdentitySeed{{Term: "path/sym1", Identity: []byte("caller-table/1")}}
	minted := declaredBoundaryIdentitySeeds(body, child, seeds, existing)
	if len(minted) != 1 || string(minted[0].Identity) != "caller-table/1" {
		t.Fatalf("identity seeds = %#v, want the caller mapping unchanged", minted)
	}
}

// TestDiagnosticOperationReadsTheCoordinate pins the operation coordinate a
// diagnostic key carries, which is what scopes the member-write obligation.
func TestDiagnosticOperationReadsTheCoordinate(t *testing.T) {
	for key, want := range map[string]string{
		"type.assignment/op-00000006":                                  "op-00000006",
		"type.call.direct.argument_type/op-00000007/argument-00000000": "op-00000007",
	} {
		got, found := diagnosticOperation(key)
		if !found || got != want {
			t.Fatalf("diagnosticOperation(%q) = %q/%v, want %q", key, got, found, want)
		}
	}
	if _, found := diagnosticOperation("type.assignment/claim"); found {
		t.Fatal("a key without an operation coordinate reported one")
	}
}

// TestFormalMemberWriteDiagnosticIsScopedToItsObligation pins the surface of a
// sealed-environment entry: a covered operation publishes, an operation outside
// the obligation stays dormant, and a family the declaration cannot discharge
// stays withheld either way.
func TestFormalMemberWriteDiagnosticIsScopedToItsObligation(t *testing.T) {
	obligations := map[string]bool{"op-00000006": true}
	if !formalMemberWriteDiagnostic(obligations, "type.assignment/op-00000006") {
		t.Fatal("a covered assignment refutation was withheld")
	}
	if formalMemberWriteDiagnostic(obligations, "type.assignment/op-00000009") {
		t.Fatal("an operation outside the obligation was published")
	}
	if formalMemberWriteDiagnostic(obligations, "claim/unproven/op-00000006") {
		t.Fatal("an absence family was published from a declaration-owned entry")
	}
}
