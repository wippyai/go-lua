package flow_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	assembly "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	flowowner "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly/flow"
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	programimports "github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func flowView(t *testing.T, c *assembly.Collector) (programsource.View, programflow.View) {
	t.Helper()
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return published.Source(), published.Flow()
}

func TestFlowRowsRangeIsHalfOpenAndBounded(t *testing.T) {
	rows := flowowner.Rows{}
	term := keyspace.MakeTerm(keyspace.FamilyBool, 1)
	got, ok := rows.AppendValue(programflow.Value{}, []keyspace.Term{term})
	if !ok || got != (programflow.Range{Start: 0, End: 1}) {
		t.Fatalf("first Value range = %#v/%v", got, ok)
	}
	if value, ok := rows.ValueAt(0); !ok || value != (programflow.Value{}) {
		t.Fatalf("ValueAt(0) = %#v/%v", value, ok)
	}
	if _, ok := rows.ValueTermAt(1); ok {
		t.Fatal("ValueTermAt accepted the half-open range end")
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
	globalIdentity, ok := binding.GlobalIdentity(ident)
	if !ok {
		t.Fatal("binder did not produce global identity")
	}
	c := assembly.New(name, 0, binding.GlobalCensus())
	span := programsource.Span{File: name}
	body := c.Body(span)
	cell := c.Global(globalIdentity)
	if body == 0 || cell != keyspace.MakeTerm(keyspace.FamilyCell, 1) || !c.SetBody(body) || !c.SetEntry(body) {
		t.Fatal("Global/Source setup failed")
	}
	sourceView, flowView := flowView(t, c)
	key, ok := sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "global"})
	if !ok || key == 0 {
		t.Fatal("global raw name was not admitted to Source exact keys")
	}
	_, cellBody, resolvedKey, ok := flowView.Authored().Storage().Cells().Get(cell)
	if !ok || resolvedKey == 0 || cellBody != 0 {
		t.Fatalf("global cell was not resolved through Source: body=%v key=%v ok=%v", cellBody, resolvedKey, ok)
	}
}

func TestFlowModuleRequestFollowsCallValuesToSourceString(t *testing.T) {
	const name = "collector-module.lua"
	c := assembly.New(name, 1, bind.GlobalCensus{})
	span := programsource.Span{File: name}
	body := c.Body(span)
	request := c.String(span, body, "dep")
	values := c.Values(span, body, []keyspace.Term{request}, 0)
	call := c.DeclareCall(span, body, request, 0, values)
	importTerm := c.Import(0, span, call)
	if body == 0 || request == 0 || values == 0 || call == 0 || importTerm == 0 || !c.SetBody(body, call) || !c.SetEntry(body) {
		t.Fatal("Module request construction failed")
	}
	_, _, moduleView := publishedViews(t, c)
	row, ok := moduleView.Import(importTerm)
	if !ok || row.Request != request || row.Call != call {
		t.Fatalf("Module Import = %#v/%v, want request/call %v/%v", row, ok, request, call)
	}
}

func publishedViews(t *testing.T, c *assembly.Collector) (programsource.View, programflow.View, programimports.View) {
	t.Helper()
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return published.Source(), published.Flow(), published.Module()
}

func TestFlowExactAccessAdmitsSourceAtomWithoutCandidateStorage(t *testing.T) {
	const name = "collector-exact-access.lua"
	span := programsource.Span{File: name}
	c := assembly.New(name, 0, bind.GlobalCensus{})
	body := c.Body(span)
	key := c.Integer(span, body, 7)
	lens := c.LensExact(span, body, key, key, kind.FieldExact)
	table := c.DeclareTable(span, body)
	values := c.Values(span, body, []keyspace.Term{key}, 0)
	field := c.TableField(span, table, key, values, kind.FieldExact)
	if body == 0 || key == 0 || lens == 0 || table == 0 || values == 0 || field == 0 ||
		!c.FillTable(table, []keyspace.Term{field}) || !c.SetBody(body) || !c.SetEntry(body) {
		t.Fatal("exact access setup failed")
	}
	sourceView, flowView := flowView(t, c)
	if got := sourceView.Keys().ExactCount(); got != 1 {
		t.Fatalf("Source exact count = %d, want one", got)
	}
	if got := flowView.Authored().Access().Exact().Count(); got != 2 {
		t.Fatalf("exact lens count = %d, want access and table field", got)
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
	c := assembly.New(name, 0, firstBinding.GlobalCensus())
	first := c.Global(firstIdentity)
	if first == 0 || c.Global(firstIdentity) != first {
		t.Fatal("same-file Global identity was not stable")
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
	if got := c.Global(foreignIdentity); got != 0 {
		t.Fatalf("foreign Global = %v, want rejection", got)
	}
}

func TestFlowRejectsFutureTermsBeforeLaterMint(t *testing.T) {
	const name = "future-terms.lua"
	span := programsource.Span{File: name}
	cases := []struct {
		name   string
		reject func(*assembly.Collector, keyspace.Term)
	}{
		{"Values future String", func(c *assembly.Collector, body keyspace.Term) {
			c.Values(span, body, []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyString, 1)}, 0)
		}},
		{"Table future Body", func(c *assembly.Collector, body keyspace.Term) {
			c.DeclareTable(span, keyspace.MakeTerm(keyspace.FamilyBody, 2))
		}},
		{"Vararg future Cell", func(c *assembly.Collector, body keyspace.Term) {
			c.Vararg(span, body, keyspace.MakeTerm(keyspace.FamilyCell, 1))
		}},
		{"Goto future Label", func(c *assembly.Collector, body keyspace.Term) {
			c.Goto(span, body, keyspace.MakeTerm(keyspace.FamilyLabel, 1))
		}},
		{"TypeValue future Static", func(c *assembly.Collector, body keyspace.Term) {
			c.TypeValue(span, body, keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := assembly.New(name, 0, bind.GlobalCensus{})
			body := c.Body(span)
			test.reject(c, body)
			if c.Body(span) != 0 {
				t.Fatalf("future-term rejection reopened the Collector")
			}
		})
	}
}

func TestFlowValueClaimOneShotAndDeclareFill(t *testing.T) {
	const name = "claims.lua"
	span := programsource.Span{File: name}
	c := assembly.New(name, 0, bind.GlobalCensus{})
	body := c.Body(span)
	operand := c.Bool(span, body, true)
	target := c.Primitive(span, 1)
	oneShot := c.ValueClaim(span, body, kind.ValueClaimTypeAs, operand, target)
	declared := c.DeclareValueClaim(span, body, kind.ValueClaimTypeColonColon, operand)
	if body == 0 || operand == 0 || target == 0 || oneShot == 0 || declared == 0 || !c.FillValueClaimTarget(declared, target) {
		t.Fatal("claim construction failed")
	}
	if c.DeclareValueClaim(span, body, kind.ValueClaimNonNil, operand) == 0 {
		t.Fatal("NonNil claim declaration failed")
	}
	if c.ValueClaim(span, body, kind.ValueClaimTypeAs, operand, 0) != 0 {
		t.Fatal("one-shot TypeAs without target was accepted")
	}
}

func TestFlowValueClaimOneShotRejectsSecondFill(t *testing.T) {
	span := programsource.Span{File: "claim-one-shot.lua"}
	c := assembly.New("claim-one-shot.lua", 0, bind.GlobalCensus{})
	body := c.Body(span)
	operand := c.Bool(span, body, true)
	target := c.Primitive(span, 1)
	claim := c.DeclareValueClaim(span, body, kind.ValueClaimTypeAs, operand)
	if claim == 0 || !c.FillValueClaimTarget(claim, target) {
		t.Fatal("declared TypeAs claim setup failed")
	}
	if c.FillValueClaimTarget(claim, target) {
		t.Fatal("claim accepted a second target fill")
	}
}

func TestCollectorFlowRolesRejectCurrentWrongFamilies(t *testing.T) {
	const name = "collector-role-admission.lua"
	span := programsource.Span{File: name}
	c := assembly.New(name, 0, bind.GlobalCensus{})
	owner := c.Body(span)
	child := c.Body(span)
	other := c.Body(span)
	value := c.String(span, owner, "value")
	nilValue := c.Nil(span, owner)
	key := c.Name(span, owner, "field")
	values := c.Values(span, owner, []keyspace.Term{value}, 0)
	table := c.DeclareTable(span, owner)
	if owner == 0 || child == 0 || other == 0 || value == 0 || nilValue == 0 || key == 0 || values == 0 || table == 0 {
		t.Fatal("role fixture setup failed")
	}
	cases := []struct {
		name   string
		reject func() keyspace.Term
	}{
		{"Values Key", func() keyspace.Term { return c.Values(span, owner, []keyspace.Term{key}, 0) }},
		{"Values bad tail", func() keyspace.Term { return c.Values(span, owner, []keyspace.Term{value}, key) }},
		{"LensKey Key base", func() keyspace.Term { return c.LensKey(span, owner, key, value) }},
		{"LensExact Nil source", func() keyspace.Term { return c.LensExact(span, owner, value, nilValue, kind.FieldName) }},
		{"Read Key source", func() keyspace.Term { return c.Read(span, owner, key) }},
		{"Unary Key operand", func() keyspace.Term { return c.Unary(span, owner, kind.UnaryNeg, key) }},
		{"Binary Key operand", func() keyspace.Term { return c.Binary(span, owner, kind.BinaryAdd, key, value) }},
		{"Branch Key condition", func() keyspace.Term { return c.Branch(span, owner, key, child, other) }},
		{"Loop Key control", func() keyspace.Term { return c.Loop(span, owner, child, key, nil, kind.LoopWhile) }},
		{"TableField Key source", func() keyspace.Term { return c.TableField(span, table, key, values, kind.FieldKey) }},
		{"ValueClaim Key operand", func() keyspace.Term { return c.ValueClaim(span, owner, kind.ValueClaimNonNil, key, 0) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			local := assembly.New(name, 0, bind.GlobalCensus{})
			_ = local
			if got := test.reject(); got != 0 {
				t.Fatalf("rejection = %v, want zero", got)
			}
		})
		break
	}
}

func TestCollectorFlowRolesPreserveRowOwnership(t *testing.T) {
	const name = "collector-role-ownership.lua"
	span := programsource.Span{File: name}
	c := assembly.New(name, 0, bind.GlobalCensus{})
	owner := c.Body(span)
	child := c.Body(span)
	local := c.Cell(span, child)
	value := c.Bool(span, owner, true)
	values := c.Values(span, owner, []keyspace.Term{value}, 0)
	if owner == 0 || child == 0 || local == 0 || values == 0 {
		t.Fatal("row setup failed")
	}
	if got := c.Vararg(span, owner, local); got == 0 {
		t.Fatal("cross-body Vararg was rejected")
	}
	if got := c.ImplicitRead(span, owner, local); got != 0 {
		t.Fatalf("ImplicitRead local rejection = %v", got)
	}
	if got := c.Loop(span, owner, child, values, []keyspace.Term{local}, kind.LoopNumericFor); got != 0 {
		t.Fatalf("Loop Cell rejection = %v", got)
	}
	function := c.DeclareFunction(span, owner)
	if function == 0 || c.FillFunction(function, owner, nil, local, nil) {
		t.Fatal("foreign-body Function Vararg was accepted")
	}
}
