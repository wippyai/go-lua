package collector

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func TestCollectorAdmissionGapMatrixTerminalizes(t *testing.T) {
	const name = "collector-admission-gaps.lua"
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	body := func(c *Collector) Term { return c.Source().Order().Body(span) }
	assign := func(c *Collector, owner, cell Term) Term {
		value := c.Source().Literals().Bool(span, owner, true)
		values := c.Flow().Values().Values(span, owner, []Term{value}, 0)
		return c.Flow().Storage().Assign(span, owner, []Term{cell}, []source.Span{span}, values)
	}
	tests := []struct {
		name  string
		build func() (*Collector, func(*Collector))
	}{
		{
			name: "Body rejects non-direct literal root",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				literal := c.Source().Literals().String(span, owner, "leaf")
				return c, func(c *Collector) { c.Source().Order().SetBody(owner, literal) }
			},
		},
		{
			name: "Body rejects duplicate direct root",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				label := c.Flow().Control().Label(span, owner)
				return c, func(c *Collector) { c.Source().Order().SetBody(owner, label, label) }
			},
		},
		{
			name: "Body rejects direct root owned by another Body",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				first, second := body(c), body(c)
				label := c.Flow().Control().Label(span, second)
				return c, func(c *Collector) { c.Source().Order().SetBody(first, label) }
			},
		},
		{
			name: "LensExact requires candidate",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				integer := c.Source().Literals().Integer(span, owner, 7)
				nonExact := c.Flow().Operators().Unary(span, owner, kind.UnaryBitNot, integer)
				return c, func(c *Collector) { c.Flow().Access().LensExact(span, owner, integer, nonExact, kind.FieldExact) }
			},
		},
		{
			name: "TableField requires candidate",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				integer := c.Source().Literals().Integer(span, owner, 7)
				nonExact := c.Flow().Operators().Unary(span, owner, kind.UnaryBitNot, integer)
				values := c.Flow().Values().Values(span, owner, []Term{integer}, 0)
				table := c.Flow().Tables().DeclareTable(span, owner)
				return c, func(c *Collector) { c.Flow().Tables().TableField(span, table, nonExact, values, kind.FieldExact) }
			},
		},
		{
			name: "Alias generics require owned unique params",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				first := c.Static().Declarations().Alias(span, span, owner, "A")
				param := c.Static().Declarations().TypeParam(span, first, "T")
				return c, func(c *Collector) { c.Static().Declarations().AliasParams(first, []Term{param, param}) }
			},
		},
		{
			name: "TypeFunction generics require owned params",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				host := c.Static().Declarations().Alias(span, span, owner, "Host")
				first := c.Static().Signatures().TypeFunction(span, host)
				second := c.Static().Signatures().TypeFunction(span, host)
				param := c.Static().Declarations().TypeParam(span, first, "T")
				return c, func(c *Collector) { c.Static().Signatures().TypeFunctionGenerics(second, []Term{param}) }
			},
		},
		{
			name: "Flow Function generics require owned params",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				first := c.Flow().Functions().DeclareFunction(span, owner)
				second := c.Flow().Functions().DeclareFunction(span, owner)
				param := c.Static().Declarations().TypeParam(span, first, "T")
				return c, func(c *Collector) { c.Flow().Functions().SetFunctionGenerics(second, []Term{param}) }
			},
		},
		{
			name: "Function formals reject reused Cell",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Flow().Storage().Cell(span, owner)
				function := c.Flow().Functions().DeclareFunction(span, owner)
				return c, func(c *Collector) { c.Flow().Functions().FillFunction(function, owner, []Term{cell, cell}, 0, nil) }
			},
		},
		{
			name: "Bind rejects reused Cell",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Flow().Storage().Cell(span, owner)
				values := c.Flow().Values().Values(span, owner, nil, 0)
				return c, func(c *Collector) { c.Flow().Storage().Bind(span, owner, []Term{cell, cell}, values) }
			},
		},
		{
			name: "Interface method requires interface-scoped signature",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				first := c.Static().Declarations().Interface(span, span, owner, "I")
				second := c.Static().Declarations().Interface(span, span, owner, "J")
				signature := c.Static().Signatures().TypeFunction(span, first)
				member := StaticInterfaceMember{Kind: programstatic.InterfaceMethod, Name: "m", Span: span, Signature: signature}
				return c, func(c *Collector) {
					c.Static().Declarations().InterfaceMembers(second, []StaticInterfaceMember{member})
				}
			},
		},
		{
			name: "Publication rejects unresolved TypeRef",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Flow().Storage().Cell(span, owner)
				publicationAssign := assign(c, owner, cell)
				ref := c.Static().References().Unresolved(span, []string{"Missing"}, 0)
				return c, func(c *Collector) { c.Static().Publications().Type(span, publicationAssign, 0, ref) }
			},
		},
		{
			name: "Publication rejects duplicate Assign pair",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Flow().Storage().Cell(span, owner)
				publicationAssign := assign(c, owner, cell)
				primitive := c.Static().Types().Primitive(span, programstatic.PrimitiveString)
				alias := c.Static().Declarations().Alias(span, span, owner, "A")
				c.Static().Declarations().AliasTarget(alias, primitive)
				ref := c.Static().References().Declaration(span, []string{"A"}, 0, alias)
				first := c.Static().Publications().Type(span, publicationAssign, 0, ref)
				if first == 0 {
					return c, func(*Collector) {}
				}
				return c, func(c *Collector) { c.Static().Publications().Type(span, publicationAssign, 0, ref) }
			},
		},
		{
			name: "TypeValue requires runtime-loadable target",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				staticOnly := c.Static().Types().Primitive(span, programstatic.PrimitiveFunction)
				return c, func(c *Collector) { c.Flow().Operands().TypeValue(span, owner, staticOnly) }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, reject := test.build()
			if c.err != nil || c.terminal {
				t.Fatalf("setup failed: %v", failure(c))
			}
			reject(c)
			if c.err == nil || !c.terminal || !collectorScratchCleared(c) {
				t.Fatalf("gap rejection not terminal/cleared: err=%v terminal=%v", c.err, c.terminal)
			}
		})
	}
}
