package wire

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/constraint"
	"github.com/wippyai/go-lua/domain/constraint/expr"
)

// The expression codec spells the grammar a second time, as the wire kinds it
// writes and reads. These laws pin that spelling to the sealed constraint form
// vocabulary: every declared form is reachable through the codec under the
// kind pinned to it, and the codec admits no kind the vocabulary does not
// declare. A form added to the grammar and not to the codec, or a wire kind
// renamed on one side alone, is a verdict here rather than a term that fails to
// serialize at write time.
//
// The pin names the codec as the drifted spelling, because the sealed
// vocabulary is the authority and the wire strings are the codec's own
// serialization commitment.

// pinnedWireKind is the codec's boundary spelling of one grammar form. It is
// the statement being pinned, held against what the codec actually writes.
func pinnedWireKind(form expr.Form) string {
	switch form {
	case expr.FormVar:
		return "var"
	case expr.FormConst:
		return "const"
	case expr.FormBinOp:
		return "binop"
	case expr.FormLen:
		return "len"
	case expr.FormParam:
		return "param"
	case expr.FormRet:
		return "ret"
	case expr.FormParamLen:
		return "paramLen"
	case expr.FormRetLen:
		return "retLen"
	case expr.FormMin:
		return "min"
	case expr.FormMax:
		return "max"
	default:
		return ""
	}
}

// termOfForm is one term the codec is exercised with per grammar form. Its own
// form is checked against the form it stands for, so a sample cannot drift away
// from the member it exercises.
func termOfForm(form expr.Form) expr.Expr {
	switch form {
	case expr.FormVar:
		return expr.V("i")
	case expr.FormConst:
		return expr.C(3)
	case expr.FormBinOp:
		return expr.Add(expr.V("i"), expr.C(1))
	case expr.FormLen:
		return expr.L("arr")
	case expr.FormParam:
		return expr.P(0)
	case expr.FormRet:
		return expr.R(1)
	case expr.FormParamLen:
		return expr.PL(0)
	case expr.FormRetLen:
		return expr.RL(1)
	case expr.FormMin:
		return expr.MinExpr(expr.V("i"), expr.C(1))
	case expr.FormMax:
		return expr.MaxExpr(expr.V("i"), expr.C(1))
	default:
		return nil
	}
}

func sealedConstraintForms(t *testing.T) structure.Table {
	t.Helper()
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("declaration table rejected: contributor=%d law=%d disposition=%s", failure.Contributor, failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("sealed table holds no structural vocabulary")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("sealed structural vocabulary did not project")
	}
	return table
}

// TestCodecWireKindsArePinnedToTheSealedFormVocabulary states the forward half
// of the pin: every member of the sealed vocabulary round-trips through the
// codec, and the kind the codec writes for it is the kind pinned to that
// member.
func TestCodecWireKindsArePinnedToTheSealedFormVocabulary(t *testing.T) {
	table := sealedConstraintForms(t)
	if declared := table.Count(structure.CategoryConstraintForm); declared != expr.FormCount {
		t.Fatalf("sealed vocabulary declares %d forms for a grammar of %d", declared, expr.FormCount)
	}
	pinned := make(map[string]schema.Key, expr.FormCount)
	for _, form := range expr.Forms() {
		entry, ok := table.At(structure.CategoryConstraintForm, uint16(form))
		if !ok {
			t.Fatalf("grammar form %d names no member of the sealed vocabulary", form)
		}
		kind := pinnedWireKind(form)
		if kind == "" {
			t.Fatalf("sealed member %q has no pinned wire kind, so the codec spelling is unstated for it", entry.Key())
		}
		if prior, duplicate := pinned[kind]; duplicate {
			t.Fatalf("sealed members %q and %q are both pinned to wire kind %q", prior, entry.Key(), kind)
		}
		pinned[kind] = entry.Key()

		term := termOfForm(form)
		if expr.FormOf(term) != form {
			t.Fatalf("the term exercising sealed member %q is form %d, not %d", entry.Key(), expr.FormOf(term), form)
		}
		wire, err := encodeExpr(term)
		if err != nil || wire == nil {
			t.Fatalf("codec does not encode sealed member %q: %v", entry.Key(), err)
		}
		if wire.Kind != kind {
			t.Fatalf("codec writes sealed member %q as wire kind %q, but it is pinned to %q", entry.Key(), wire.Kind, kind)
		}
		decoded, err := decodeExpr(wire)
		if err != nil {
			t.Fatalf("codec does not decode wire kind %q of sealed member %q: %v", kind, entry.Key(), err)
		}
		if expr.FormOf(decoded) != form {
			t.Fatalf("wire kind %q decodes to form %d, but it is pinned to sealed member %q at form %d", kind, expr.FormOf(decoded), entry.Key(), form)
		}
	}
}

// TestCodecAdmitsNoWireKindOutsideTheSealedVocabulary states the closing half:
// a kind the vocabulary does not declare is rejected at the boundary, so the
// codec's read side is the sealed catalog and not a superset of it.
func TestCodecAdmitsNoWireKindOutsideTheSealedVocabulary(t *testing.T) {
	table := sealedConstraintForms(t)
	for _, form := range expr.Forms() {
		entry, ok := table.At(structure.CategoryConstraintForm, uint16(form))
		if !ok {
			t.Fatalf("grammar form %d names no member of the sealed vocabulary", form)
		}
		// A row's surface key is the vocabulary's own spelling, never the wire's.
		// The codec must not accept it, or the two spellings would be one and the
		// boundary would carry a declaration identity it does not own.
		if _, err := decodeExpr(&exprWire{Kind: string(entry.Key())}); err == nil {
			t.Fatalf("codec admitted the surface key %q as a wire kind", entry.Key())
		}
		if _, err := decodeExpr(&exprWire{Kind: string(constraint.FormKey(form))}); err == nil {
			t.Fatalf("codec admitted the declared name of form %d as a wire kind", form)
		}
	}
	if _, err := decodeExpr(&exprWire{Kind: "unsealed"}); err == nil {
		t.Fatal("codec admitted a wire kind no sealed member declares")
	}
}
