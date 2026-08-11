package collector

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestFlowRowsRangeIsHalfOpenAndBounded(t *testing.T) {
	got, ok := rangeFor(4, 3)
	if !ok || got != (flow.Range{Start: 4, End: 7}) {
		t.Fatalf("rangeFor(4,3) = %#v/%v", got, ok)
	}
	if _, ok := rangeFor(-1, 1); ok {
		t.Fatal("negative pool length accepted")
	}
	if _, ok := rangeFor(0, int(keyspace.MaxTermOrdinal)+1); ok {
		t.Fatal("overflowing pool accepted")
	}
}

func TestFlowRowsFreezeUsesSourceGlobalKey(t *testing.T) {
	const name = "collector-global.lua"
	stmts, err := parse.ParseString("local value = global", name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	binding := bind.BindChunk(stmts)
	ident := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	identity, ok := binding.GlobalIdentity(ident)
	if !ok {
		t.Fatal("binder did not produce global identity")
	}
	c := New(name, 0, binding.GlobalCensus())
	span := source.Span{File: name}
	body := c.Source().Order().Body(span)
	if body == 0 || !c.Source().Order().SetBody(body) || !c.Source().Order().SetEntry(body) {
		t.Fatalf("Body/Entry construction failed: %v", failure(c))
	}
	cell := c.Flow().Storage().Global(identity)
	if cell != keyspace.MakeTerm(keyspace.FamilyCell, 1) {
		t.Fatalf("Global = %v, want Cell/1: %v", cell, failure(c))
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	preimage := preparedSourcePreimage(t, prepared)
	if key, ok := preimage.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"}); !ok || key == 0 {
		t.Fatal("global raw name was not admitted to the Source exact denominator")
	}
	assembly, err := prepared.Assemble()
	if err != nil || assembly == nil {
		t.Fatalf("Prepared.Assemble = %v/%v", assembly, err)
	}
	_, flowComponent, _, _, err := assembly.Take()
	if err != nil || flowComponent == nil {
		t.Fatalf("Assembly.Take = %v/%v", flowComponent, err)
	}
	_, cellBody, key, ok := flowComponent.View().Authored().Storage().Cells().Get(cell)
	if !ok || key == 0 || cellBody != 0 {
		t.Fatalf("global cell was not resolved through Source: body=%v key=%v ok=%v", cellBody, key, ok)
	}
}

func TestFlowModuleRequestFollowsCallValuesToSourceString(t *testing.T) {
	const name = "collector-module.lua"
	c := New(name, 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(source.Span{File: name})
	if body == 0 || !c.Source().Order().SetEntry(body) {
		t.Fatalf("Body/Entry construction failed: %v", failure(c))
	}
	request := c.Source().Literals().String(source.Span{File: name}, body, "dep")
	if request == 0 {
		t.Fatalf("String construction failed: %v", failure(c))
	}
	values := c.Flow().Values().Values(source.Span{File: name}, body, []keyspace.Term{request}, 0)
	if values == 0 {
		t.Fatalf("Values construction failed: %v", failure(c))
	}
	call := c.Flow().Calls().DeclareCall(source.Span{File: name}, body, request, 0, values)
	if call == 0 {
		t.Fatalf("Call construction failed: %v", failure(c))
	}
	got, ok := c.Flow().Calls().moduleRequestTerm(call)
	if !ok || got != request {
		t.Fatalf("moduleRequestTerm = %v/%v, want %v", got, ok, request)
	}
}

func TestFlowExactAccessAdmitsSourceAtomWithoutCandidateStorage(t *testing.T) {
	const name = "collector-exact-access.lua"
	span := source.Span{File: name}
	c := New(name, 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(span)
	if body == 0 || !c.Source().Order().SetBody(body) || !c.Source().Order().SetEntry(body) {
		t.Fatalf("Source setup failed: %v", failure(c))
	}
	key := c.Source().Literals().Integer(span, body, 7)
	if key == 0 {
		t.Fatalf("integer setup failed: %v", failure(c))
	}
	lens := c.Flow().Access().LensExact(span, body, key, key, kind.FieldExact)
	if lens == 0 {
		t.Fatalf("LensExact setup failed: %v", failure(c))
	}
	table := c.Flow().Tables().DeclareTable(span, body)
	values := c.Flow().Values().Values(span, body, []keyspace.Term{key}, 0)
	field := c.Flow().Tables().TableField(span, table, key, values, kind.FieldExact)
	if table == 0 || values == 0 || field == 0 || !c.Flow().Tables().FillTable(table, []keyspace.Term{field}) {
		t.Fatalf("TableField setup failed: %v", failure(c))
	}
	found := false
	for _, atom := range c.source.exact {
		if atom == (keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7}) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("FieldExact did not admit Source atom: %#v", c.source.exact)
	}
}

func TestFlowGlobalRejectsForeignIdentityAndKeepsReservedTerm(t *testing.T) {
	const name = "global-owner.lua"
	firstStmts, err := parse.ParseString("local value = shared", name)
	if err != nil {
		t.Fatalf("parse first: %v", err)
	}
	firstBinding := bind.BindChunk(firstStmts)
	firstIdent := firstStmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	firstIdentity, ok := firstBinding.GlobalIdentity(firstIdent)
	if !ok {
		t.Fatal("first binding did not produce global identity")
	}
	c := New(name, 0, firstBinding.GlobalCensus())
	first := c.Flow().Storage().Global(firstIdentity)
	if first == 0 {
		t.Fatalf("first Global construction failed: %v", failure(c))
	}
	if got := c.Flow().Storage().Global(firstIdentity); got != first {
		t.Fatalf("same-file repeated Global = %v, want %v", got, first)
	}
	foreignStmts, err := parse.ParseString("local value = shared", "foreign.lua")
	if err != nil {
		t.Fatalf("parse foreign: %v", err)
	}
	foreignBinding := bind.BindChunk(foreignStmts)
	foreignIdent := foreignStmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.IdentExpr)
	foreignIdentity, ok := foreignBinding.GlobalIdentity(foreignIdent)
	if !ok {
		t.Fatal("foreign binding did not produce global identity")
	}
	if got := c.Flow().Storage().Global(foreignIdentity); got != 0 {
		t.Fatalf("foreign Global = %v, want rejection", got)
	}
}

func TestFlowRejectsFutureTermsBeforeLaterMint(t *testing.T) {
	const name = "future-terms.lua"
	span := source.Span{File: name}
	cases := []struct {
		name   string
		reject func(*Collector, Term)
	}{
		{"Values future String", func(c *Collector, body Term) {
			c.Flow().Values().Values(span, body, []Term{keyspace.MakeTerm(keyspace.FamilyString, 1)}, 0)
		}},
		{"Table future Body", func(c *Collector, body Term) {
			c.Flow().Tables().DeclareTable(span, keyspace.MakeTerm(keyspace.FamilyBody, 2))
		}},
		{"Vararg future Cell", func(c *Collector, body Term) {
			c.Flow().Storage().Vararg(span, body, keyspace.MakeTerm(keyspace.FamilyCell, 1))
		}},
		{"Goto future Label", func(c *Collector, body Term) {
			c.Flow().Control().Goto(span, body, keyspace.MakeTerm(keyspace.FamilyLabel, 1))
		}},
		{"TypeValue future Static", func(c *Collector, body Term) {
			c.Flow().Operands().TypeValue(span, body, keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := New(name, 0, bind.GlobalCensus{})
			body := c.Source().Order().Body(span)
			test.reject(c, body)
			if c.err == nil || !c.terminal || !collectorScratchCleared(c) {
				t.Fatalf("future-term rejection was not terminal/cleared: err=%v terminal=%v", c.err, c.terminal)
			}
			cause := c.err
			if c.Source().Order().Body(span) != 0 || c.err != cause {
				t.Fatalf("future-term rejection reopened or replaced cause: err=%v", c.err)
			}
		})
	}
}

func TestFlowValueClaimOneShotAndDeclareFill(t *testing.T) {
	const name = "claims.lua"
	span := source.Span{File: name}
	c := New(name, 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(span)
	operand := c.Source().Literals().Bool(span, body, true)
	target := c.Static().Types().Primitive(span, 1)
	if body == 0 || operand == 0 || target == 0 {
		t.Fatalf("claim setup failed: %v", failure(c))
	}

	oneShot := c.Flow().Operands().ValueClaim(span, body, kind.ValueClaimTypeAs, operand, target)
	if oneShot == 0 {
		t.Fatalf("direct TypeAs claim failed: %v", failure(c))
	}

	declared := c.Flow().Operands().DeclareValueClaim(span, body, kind.ValueClaimTypeColonColon, operand)
	if declared == 0 {
		t.Fatalf("declare TypeIs claim failed: %v", failure(c))
	}
	if !c.Flow().Operands().FillValueClaimTarget(declared, target) {
		t.Fatalf("declared TypeIs target fill failed: %v", failure(c))
	}
	nonNil := c.Flow().Operands().DeclareValueClaim(span, body, kind.ValueClaimNonNil, operand)
	if nonNil == 0 {
		t.Fatalf("NonNil claim without target failed: %v", failure(c))
	}
	if got := c.Flow().Operands().ValueClaim(span, body, kind.ValueClaimTypeAs, operand, 0); got != 0 {
		t.Fatalf("one-shot TypeAs missing target = %v, want rejection", got)
	}
	futureTarget := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)
	if got := c.Flow().Operands().ValueClaim(span, body, kind.ValueClaimTypeAs, operand, futureTarget); got != 0 {
		t.Fatalf("future TypeAs target = %v, want rejection", got)
	}
}

func TestFlowValueClaimOneShotRejectsSecondFill(t *testing.T) {
	const name = "claim-one-shot.lua"
	span := source.Span{File: name}
	c := New(name, 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(span)
	operand := c.Source().Literals().Bool(span, body, true)
	target := c.Static().Types().Primitive(span, 1)
	claim := c.Flow().Operands().ValueClaim(span, body, kind.ValueClaimTypeAs, operand, target)
	if claim == 0 {
		t.Fatalf("direct TypeAs claim failed: %v", failure(c))
	}
	if c.Flow().Operands().FillValueClaimTarget(claim, target) {
		t.Fatal("direct one-shot claim accepted a second target fill")
	}
}

func collectorFamilySpans(counts [keyspace.FamilyCount]uint32) []source.FamilySpans {
	rows := make([]source.FamilySpans, 0, int(keyspace.FamilyCount)-1)
	for family := keyspace.FamilyNil; family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		rows = append(rows, source.FamilySpans{Family: family, Spans: spans})
	}
	return rows
}
