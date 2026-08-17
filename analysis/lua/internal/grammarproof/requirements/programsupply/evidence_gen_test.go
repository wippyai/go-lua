package programsupply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/binder"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/programlaw"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/staticlaw"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

func TestGeneratedEvidenceIsCurrent(t *testing.T) {
	if err := Generate(filepath.Join(packageDir(t), "evidence_gen.go"), true); err != nil {
		t.Fatal(err)
	}
	current, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(current.ProgramLaws) != len(programlaw.Requirements()) || len(current.StaticLaws) != len(staticlaw.Requirements()) || len(current.BinderLaws) != len(binder.Required()) {
		t.Fatal("generated Program supply does not cover the typed law denominators")
	}
}

func TestEvidenceRejectsTypedDenominatorMutations(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{"missing", func(e *Evidence) { e.ProgramLaws = e.ProgramLaws[1:] }},
		{"reordered", func(e *Evidence) { e.ProgramLaws[0], e.ProgramLaws[1] = e.ProgramLaws[1], e.ProgramLaws[0] }},
		{"wrong-terminal", func(e *Evidence) { e.ProgramLaws[0].Terminals[0] = e.StaticLaws[0].Terminals[0] }},
		{"negative-made-positive", func(e *Evidence) {
			row := binderIndex(t, e.BinderLaws, binder.TransitionRuntimeShadowRejected)
			e.BinderLaws[row].Positive = append([]schema.EntryID(nil), e.BinderLaws[row].Forbidden...)
			e.BinderLaws[row].Forbidden = nil
		}},
		{"both-polarities", func(e *Evidence) {
			row := binderIndex(t, e.BinderLaws, binder.TransitionRuntimeShadowRejected)
			e.BinderLaws[row].Positive = append([]schema.EntryID(nil), e.BinderLaws[row].Forbidden...)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := clone(evidence)
			test.mutate(&changed)
			changed.Digest = resign(t, changed)
			if err := changed.Validate(denominator.GeneratedRelationEntries()); err == nil {
				t.Fatal("mutated Program supply evidence was accepted")
			}
		})
	}
}

func TestCanonicalEvidenceOwnsDigestAndIsDetached(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	first := evidence.Canonical()
	if len(first) == 0 || !bytes.Equal(first, clone(evidence).Canonical()) {
		t.Fatal("equivalent evidence has nondeterministic canonical bytes")
	}
	sum := sha256.Sum256(first)
	if got := hex.EncodeToString(sum[:]); got != evidence.Digest {
		t.Fatalf("evidence digest = %s, want %s", evidence.Digest, got)
	}
	want := append([]byte(nil), first...)
	first[0] ^= 0xff
	if got := evidence.Canonical(); !bytes.Equal(got, want) {
		t.Fatal("Canonical returned aliased bytes")
	}
}

func TestAnnotationTerminalUsesDenominatorParentClosure(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	var terminals []schema.EntryID
	for _, row := range evidence.StaticLaws {
		if row.Family == staticlaw.FamilyAnnotated {
			terminals = row.Terminals
			break
		}
	}
	annotation := idFor(t, "ProgramStatic@ProgramStaticAnnotation")
	values := idFor(t, "ProgramFlowValues@-")
	typeRef := idFor(t, "ProgramStatic@ProgramStaticTypeRef")
	if len(terminals) != 1 || terminals[0] != annotation {
		t.Fatalf("annotated terminal vector = %#v, want %#v", terminals, annotation)
	}
	closure, err := Closure(denominator.GeneratedRelationEntries(), terminals)
	if err != nil {
		t.Fatal(err)
	}
	if !hasOutput(closure, annotation) || !hasOutput(closure, values) || !hasOutput(closure, typeRef) {
		t.Fatalf("annotation closure omitted a declared denominator parent: %#v", closure)
	}
	for index, row := range closure {
		if row.Owner < denominator.RelationOwnerProgramSource || row.Owner > denominator.RelationOwnerProgramModule || !row.Form.Available() {
			t.Fatalf("closure contains non-Program relation: %#v", row)
		}
		if index != 0 && !less(closure[index-1].Relation, row.Relation) {
			t.Fatal("closure is not canonically ordered")
		}
	}
}

func TestClosureRejectsUnknownAndEmptyTerminals(t *testing.T) {
	entries := denominator.GeneratedRelationEntries()
	if _, err := Closure(entries, nil); err == nil {
		t.Fatal("empty terminal vector was accepted")
	}
	if _, err := Closure(entries, []schema.EntryID{{}}); err == nil {
		t.Fatal("unknown terminal ID was accepted")
	}
}

func TestExactTypedTerminalVectors(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range evidence.ProgramLaws {
		want := idFor(t, "ProgramFlowOperators@-")
		switch row.Requirement.Site {
		case programlaw.SiteUnary:
			switch row.Requirement.Unary {
			case flowkind.UnaryNeg, flowkind.UnaryBitNot:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowUnaryNumeric")
			case flowkind.UnaryLen:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowLength")
			}
		case programlaw.SiteBinary:
			switch row.Requirement.Binary {
			case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv, flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowArithmetic")
			case flowkind.BinaryConcat:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowConcat")
			case flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor, flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowBitwise")
			case flowkind.BinaryEqual, flowkind.BinaryNotEqual:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowEquality")
			case flowkind.BinaryLess, flowkind.BinaryLessEqual, flowkind.BinaryGreater, flowkind.BinaryGreaterEqual:
				want = idFor(t, "ProgramFlowOperators@ProgramFlowOrder")
			}
		case programlaw.SiteCall:
			want = idFor(t, "ProgramFlowCall@-")
		case programlaw.SiteValues:
			want = idFor(t, "ProgramFlowValues@-")
		case programlaw.SiteOutcome:
			want = idFor(t, "ProgramFlowOutcome@-")
		}
		if len(row.Terminals) != 1 || row.Terminals[0] != want {
			t.Fatalf("Program law %v terminals = %#v, want %#v", row.Requirement, row.Terminals, want)
		}
	}
}

func idFor(t *testing.T, key string) schema.EntryID {
	t.Helper()
	entry, ok := denominator.GeneratedRelationByKey(schema.Key(key))
	if !ok {
		t.Fatalf("missing denominator relation %q", key)
	}
	return entry.ID()
}

func binderIndex(t *testing.T, rows []BinderRow, transition binder.Transition) int {
	t.Helper()
	for index, row := range rows {
		if row.Requirement.Transition == transition {
			return index
		}
	}
	t.Fatalf("missing binder transition %d", transition)
	return -1
}

func hasOutput(rows []Output, want schema.EntryID) bool {
	for _, row := range rows {
		if row.Relation == want {
			return true
		}
	}
	return false
}

func resign(t *testing.T, evidence Evidence) string {
	t.Helper()
	digest, err := evidenceDigest(evidence)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Program supply evidence test")
	}
	return filepath.Dir(file)
}
