package collector

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestCollectorFlowRolesRejectCurrentWrongFamilies(t *testing.T) {
	const name = "collector-role-admission.lua"
	span := source.Span{File: name}
	type fixture struct {
		c                                   *Collector
		owner, child, other                 Term
		value, nilValue, key, values, table Term
	}
	setup := func() fixture {
		c := New(name, 0, bind.GlobalCensus{})
		owner := c.Source().Order().Body(span)
		child := c.Source().Order().Body(span)
		other := c.Source().Order().Body(span)
		value := c.Source().Literals().String(span, owner, "value")
		nilValue := c.Source().Literals().Nil(span, owner)
		key := c.Source().Keys().Name(span, owner, "field")
		values := c.Flow().Values().Values(span, owner, []keyspace.Term{value}, 0)
		table := c.Flow().Tables().DeclareTable(span, owner)
		return fixture{c: c, owner: owner, child: child, other: other, value: value, nilValue: nilValue, key: key, values: values, table: table}
	}
	cases := []struct {
		name   string
		reject func(fixture) keyspace.Term
	}{
		{"Values Key", func(f fixture) keyspace.Term {
			return f.c.Flow().Values().Values(span, f.owner, []keyspace.Term{f.key}, 0)
		}},
		{"Values bad tail", func(f fixture) keyspace.Term {
			return f.c.Flow().Values().Values(span, f.owner, []keyspace.Term{f.value}, f.key)
		}},
		{"LensKey Key base", func(f fixture) keyspace.Term { return f.c.Flow().Access().LensKey(span, f.owner, f.key, f.value) }},
		{"LensExact Nil source", func(f fixture) keyspace.Term {
			return f.c.Flow().Access().LensExact(span, f.owner, f.value, f.nilValue, kind.FieldName)
		}},
		{"Read Key source", func(f fixture) keyspace.Term { return f.c.Flow().Storage().Read(span, f.owner, f.key) }},
		{"Unary Key operand", func(f fixture) keyspace.Term {
			return f.c.Flow().Operators().Unary(span, f.owner, kind.UnaryNeg, f.key)
		}},
		{"Binary Key operand", func(f fixture) keyspace.Term {
			return f.c.Flow().Operators().Binary(span, f.owner, kind.BinaryAdd, f.key, f.value)
		}},
		{"Branch Key condition", func(f fixture) keyspace.Term {
			return f.c.Flow().Control().Branch(span, f.owner, f.key, f.child, f.other)
		}},
		{"Loop Key control", func(f fixture) keyspace.Term {
			return f.c.Flow().Control().Loop(span, f.owner, f.child, f.key, nil, kind.LoopWhile)
		}},
		{"TableField Key source", func(f fixture) keyspace.Term {
			return f.c.Flow().Tables().TableField(span, f.table, f.key, f.values, kind.FieldKey)
		}},
		{"ValueClaim Key operand", func(f fixture) keyspace.Term {
			return f.c.Flow().Operands().ValueClaim(span, f.owner, kind.ValueClaimNonNil, f.key, 0)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := setup()
			if got := test.reject(f); got != 0 || f.c.err == nil || !f.c.terminal {
				t.Fatalf("rejection = %v/%v terminal=%v", got, f.c.err, f.c.terminal)
			}
		})
	}
}

func TestCollectorFlowRolesPreserveRowOwnership(t *testing.T) {
	const name = "collector-role-ownership.lua"
	span := source.Span{File: name}
	setup := func() (*Collector, Term, Term, Term, Term) {
		c := New(name, 0, bind.GlobalCensus{})
		owner := c.Source().Order().Body(span)
		child := c.Source().Order().Body(span)
		local := c.Flow().Storage().Cell(span, child)
		value := c.Source().Literals().Bool(span, owner, true)
		values := c.Flow().Values().Values(span, owner, []keyspace.Term{value}, 0)
		if owner == 0 || child == 0 || local == 0 || values == 0 {
			t.Fatalf("row setup failed: %v", failure(c))
		}
		return c, owner, child, local, values
	}
	t.Run("cross-body chunk Vararg is admitted", func(t *testing.T) {
		c, owner, _, local, _ := setup()
		if got := c.Flow().Storage().Vararg(span, owner, local); got == 0 {
			t.Fatalf("cross-body Vararg rejected: %v", failure(c))
		}
	})
	t.Run("ImplicitRead rejects local Cell", func(t *testing.T) {
		c, owner, _, local, _ := setup()
		if got := c.Flow().Storage().ImplicitRead(span, owner, local); got != 0 || c.err == nil {
			t.Fatalf("ImplicitRead local rejection = %v/%v", got, failure(c))
		}
	})
	t.Run("Loop rejects Cell outside loop Body", func(t *testing.T) {
		c, owner, child, local, values := setup()
		if got := c.Flow().Control().Loop(span, owner, child, values, []keyspace.Term{local}, kind.LoopNumericFor); got != 0 || c.err == nil {
			t.Fatalf("Loop Cell rejection = %v/%v", got, failure(c))
		}
	})
	t.Run("Function activation rejects foreign-body Vararg", func(t *testing.T) {
		c, owner, _, local, _ := setup()
		function := c.Flow().Functions().DeclareFunction(span, owner)
		if function == 0 {
			t.Fatalf("Function setup failed: %v", failure(c))
		}
		if c.Flow().Functions().FillFunction(function, owner, nil, local, nil) || c.err == nil {
			t.Fatalf("Function foreign-body Vararg rejection = %v/%v", function, failure(c))
		}
	})
}

func TestCollectorStaticRolesRejectWrongFamiliesAndKeepTypeOfOperandOpen(t *testing.T) {
	const name = "collector-static-role-admission.lua"
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	setup := func() (*Collector, keyspace.Term, keyspace.Term, keyspace.Term, keyspace.Term) {
		c := New(name, 0, bind.GlobalCensus{})
		body := c.Source().Order().Body(span)
		primitive := c.Static().Types().Primitive(span, 1)
		stringTerm := c.Source().Literals().String(span, body, "operand")
		cell := c.Flow().Storage().Cell(span, body)
		if body == 0 || primitive == 0 || stringTerm == 0 || cell == 0 {
			t.Fatalf("static setup failed: %v", failure(c))
		}
		return c, body, primitive, stringTerm, cell
	}

	c, _, _, stringTerm, _ := setup()
	if got := c.Static().Types().Optional(span, stringTerm); got != 0 {
		t.Fatalf("Optional accepted non-Node String %v", got)
	}
	c, _, primitive, _, _ := setup()
	if got := c.Static().References().Declaration(span, []string{"Alias"}, 0, primitive); got != 0 {
		t.Fatalf("TypeRef Declaration accepted Primitive target %v", got)
	}
	c, _, primitive, _, _ = setup()
	if got := c.Static().Declarations().TypeParam(span, primitive, "T"); got != 0 {
		t.Fatalf("TypeParam accepted Primitive owner %v", got)
	}
	c, _, _, stringTerm, cell := setup()
	if got := c.Static().Operands().Annotation(span, cell, stringTerm, "note"); got != 0 {
		t.Fatalf("Annotation accepted String target %v", got)
	}
	c, _, primitive, stringTerm, _ = setup()
	if got := c.Static().Operators().TypeOf(span, primitive, stringTerm); got != 0 {
		t.Fatalf("TypeOf accepted Primitive scope %v", got)
	}
	c, _, _, stringTerm, cell = setup()
	if got := c.Static().Operators().TypeOf(span, cell, stringTerm); got == 0 {
		t.Fatalf("TypeOf rejected current non-Import operand: %v", failure(c))
	}
	reserved := New(name, 1, bind.GlobalCensus{})
	reservedBody := reserved.Source().Order().Body(span)
	reservedCell := reserved.Flow().Storage().Cell(span, reservedBody)
	if reservedBody == 0 || reservedCell == 0 {
		t.Fatalf("reserved Import setup failed: %v", failure(reserved))
	}
	if got := reserved.Static().Operators().TypeOf(span, reservedCell, keyspace.MakeTerm(keyspace.FamilyImport, 1)); got != 0 {
		t.Fatalf("TypeOf accepted reserved Import operand %v", got)
	}

	sourceOnly := New(name, 1, bind.GlobalCensus{})
	sourceBody := sourceOnly.Source().Order().Body(span)
	if sourceBody == 0 || sourceOnly.Source().Order().SetBody(sourceBody, keyspace.MakeTerm(keyspace.FamilyImport, 1)) {
		t.Fatal("Source Body accepted reserved Import child")
	}
}
