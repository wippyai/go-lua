package wire

import (
	"encoding/json"
	"testing"

	"github.com/wippyai/go-lua/domain/constraint/expr"
)

// The expression codec writes a format other builds already read, so the bytes
// it produces for a term are a commitment, not an implementation detail. These
// laws hold the codec to that commitment from both ends: the exact bytes each
// representative term serializes to, and the term that comes back when those
// bytes are read again.
//
// The corpus reaches every form of the grammar and every operator the codec
// admits. The coverage checks are derived from the grammar catalog and from the
// codec's own operator vocabulary, so a member added to either without a corpus
// term is a verdict here rather than an untested spelling.

// exprWireCase is one representative term and the exact bytes the codec writes
// for it.
type exprWireCase struct {
	name string
	term expr.Expr
	wire string
}

func exprWireCorpus() []exprWireCase {
	return []exprWireCase{
		{name: "var", term: expr.V("i"), wire: `{"kind":"var","name":"i"}`},
		{name: "var/empty name", term: expr.V(""), wire: `{"kind":"var"}`},
		{name: "const/positive", term: expr.C(3), wire: `{"kind":"const","value":3}`},
		{name: "const/zero", term: expr.C(0), wire: `{"kind":"const"}`},
		{name: "const/negative", term: expr.C(-7), wire: `{"kind":"const","value":-7}`},
		{
			name: "binop/add",
			term: expr.Add(expr.V("i"), expr.C(1)),
			wire: `{"kind":"binop","op":"+","left":{"kind":"var","name":"i"},"right":{"kind":"const","value":1}}`,
		},
		{
			name: "binop/sub",
			term: expr.Sub(expr.L("arr"), expr.C(1)),
			wire: `{"kind":"binop","op":"-","left":{"kind":"len","name":"arr"},"right":{"kind":"const","value":1}}`,
		},
		{
			name: "binop/mul",
			term: expr.Mul(expr.C(2), expr.P(0)),
			wire: `{"kind":"binop","op":"*","left":{"kind":"const","value":2},"right":{"kind":"param","index":0}}`,
		},
		{
			name: "binop/div",
			term: expr.Div(expr.R(1), expr.C(4)),
			wire: `{"kind":"binop","op":"/","left":{"kind":"ret","index":1},"right":{"kind":"const","value":4}}`,
		},
		{
			name: "binop/mod",
			term: expr.Mod(expr.PL(2), expr.C(3)),
			wire: `{"kind":"binop","op":"%","left":{"kind":"paramLen","index":2},"right":{"kind":"const","value":3}}`,
		},
		{name: "len", term: expr.L("arr"), wire: `{"kind":"len","name":"arr"}`},
		{name: "param/zero", term: expr.P(0), wire: `{"kind":"param","index":0}`},
		{name: "param/positive", term: expr.P(3), wire: `{"kind":"param","index":3}`},
		{name: "ret/zero", term: expr.R(0), wire: `{"kind":"ret","index":0}`},
		{name: "ret/positive", term: expr.R(2), wire: `{"kind":"ret","index":2}`},
		{name: "paramLen/zero", term: expr.PL(0), wire: `{"kind":"paramLen","index":0}`},
		{name: "paramLen/positive", term: expr.PL(1), wire: `{"kind":"paramLen","index":1}`},
		{name: "retLen/zero", term: expr.RL(0), wire: `{"kind":"retLen","index":0}`},
		{name: "retLen/positive", term: expr.RL(4), wire: `{"kind":"retLen","index":4}`},
		{
			name: "min",
			term: expr.MinExpr(expr.V("i"), expr.C(1)),
			wire: `{"kind":"min","left":{"kind":"var","name":"i"},"right":{"kind":"const","value":1}}`,
		},
		{
			name: "max",
			term: expr.MaxExpr(expr.V("i"), expr.C(1)),
			wire: `{"kind":"max","left":{"kind":"var","name":"i"},"right":{"kind":"const","value":1}}`,
		},
		{
			name: "nested",
			term: expr.Add(
				expr.MinExpr(expr.PL(0), expr.C(4)),
				expr.Mul(expr.L("t"), expr.MaxExpr(expr.R(1), expr.RL(0))),
			),
			wire: `{"kind":"binop","op":"+","left":{"kind":"min","left":{"kind":"paramLen","index":0},"right":{"kind":"const","value":4}},"right":{"kind":"binop","op":"*","left":{"kind":"len","name":"t"},"right":{"kind":"max","left":{"kind":"ret","index":1},"right":{"kind":"retLen","index":0}}}}`,
		},
		{
			name: "pointer term",
			term: &expr.BinOp{Op: expr.OpAdd, Left: &expr.ParamLen{Index: 0}, Right: &expr.Const{Value: 2}},
			wire: `{"kind":"binop","op":"+","left":{"kind":"paramLen","index":0},"right":{"kind":"const","value":2}}`,
		},
	}
}

// TestConstraintExprWireBytesAreStable states the written commitment: each term
// serializes to exactly the recorded bytes, kind spelling, field placement and
// field omission included.
func TestConstraintExprWireBytesAreStable(t *testing.T) {
	for _, tc := range exprWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := encodeExpr(tc.term)
			if err != nil {
				t.Fatalf("encodeExpr(%s): %v", tc.term, err)
			}
			if wire == nil {
				t.Fatalf("encodeExpr(%s) wrote nothing", tc.term)
			}
			data, err := json.Marshal(wire)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("wire bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestConstraintExprRoundTripsThroughItsOwnBytes states the read commitment:
// the recorded bytes parse back into the term they were written from, compared
// by the grammar's own structural equality rather than by a spelling of it here.
func TestConstraintExprRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, tc := range exprWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			var read exprWire
			if err := json.Unmarshal([]byte(tc.wire), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			decoded, err := decodeExpr(&read)
			if err != nil {
				t.Fatalf("decodeExpr: %v", err)
			}
			if !expr.ExprEquals(decoded, tc.term) {
				t.Fatalf("decoded term = %s, want %s", decoded, tc.term)
			}
			rewritten, err := encodeExpr(decoded)
			if err != nil {
				t.Fatalf("encodeExpr(decoded): %v", err)
			}
			data, err := json.Marshal(rewritten)
			if err != nil {
				t.Fatalf("marshal rewritten: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("rewritten bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestConstraintExprCorpusReachesEveryForm derives coverage from the grammar
// catalog, so a form the codec serializes without a corpus term is unproven and
// says so here.
func TestConstraintExprCorpusReachesEveryForm(t *testing.T) {
	reached := make(map[expr.Form]bool, expr.FormCount)
	for _, tc := range exprWireCorpus() {
		collectExprForms(t, tc.term, reached)
	}
	for _, form := range expr.Forms() {
		if !reached[form] {
			t.Fatalf("grammar form %d is serialized by the codec but no corpus term exercises it", form)
		}
	}
}

// TestConstraintExprCorpusReachesEveryOperator derives coverage from the
// codec's own operator vocabulary: every operator the write side admits is
// exercised by a corpus term, and every operator it rejects stays rejected.
func TestConstraintExprCorpusReachesEveryOperator(t *testing.T) {
	admitted := make(map[string]bool)
	for ordinal := 0; ordinal < 256; ordinal++ {
		token, err := encodeExprOp(expr.Op(ordinal))
		if err != nil {
			continue
		}
		admitted[token] = true
		op, err := decodeExprOp(token)
		if err != nil {
			t.Fatalf("codec writes operator token %q that it does not read: %v", token, err)
		}
		if op != expr.Op(ordinal) {
			t.Fatalf("operator token %q reads back as operator %d, written for operator %d", token, op, ordinal)
		}
	}
	if len(admitted) == 0 {
		t.Fatal("codec admits no operator at all")
	}

	exercised := make(map[string]bool, len(admitted))
	for _, tc := range exprWireCorpus() {
		collectExprOps(t, tc.term, exercised)
	}
	for token := range admitted {
		if !exercised[token] {
			t.Fatalf("codec admits operator token %q but no corpus term exercises it", token)
		}
	}
}

func collectExprForms(t *testing.T, e expr.Expr, into map[expr.Form]bool) {
	t.Helper()
	wire, err := encodeExpr(e)
	if err != nil || wire == nil {
		t.Fatalf("encodeExpr(%s): %v", e, err)
	}
	form := expr.FormOf(e)
	if !form.Valid() {
		t.Fatalf("corpus term %s is outside the grammar", e)
	}
	into[form] = true
	collectExprFormsOfWire(t, wire, into)
}

func collectExprFormsOfWire(t *testing.T, w *exprWire, into map[expr.Form]bool) {
	t.Helper()
	for _, child := range []*exprWire{w.Left, w.Right} {
		if child == nil {
			continue
		}
		decoded, err := decodeExpr(child)
		if err != nil {
			t.Fatalf("decodeExpr(operand): %v", err)
		}
		into[expr.FormOf(decoded)] = true
		collectExprFormsOfWire(t, child, into)
	}
}

func collectExprOps(t *testing.T, e expr.Expr, into map[string]bool) {
	t.Helper()
	wire, err := encodeExpr(e)
	if err != nil || wire == nil {
		t.Fatalf("encodeExpr(%s): %v", e, err)
	}
	collectExprOpsOfWire(wire, into)
}

func collectExprOpsOfWire(w *exprWire, into map[string]bool) {
	if w.Op != "" {
		into[w.Op] = true
	}
	for _, child := range []*exprWire{w.Left, w.Right} {
		if child != nil {
			collectExprOpsOfWire(child, into)
		}
	}
}
