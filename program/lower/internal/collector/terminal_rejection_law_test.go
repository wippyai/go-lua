package collector

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Representative public mutations all obey one terminal rejection law. The
// cases intentionally exercise different owner verticals without turning the
// test into a horizontal inventory of every writer method.
func TestRepresentativeMutationRejectionIsTerminalAndRetainsFirstCause(t *testing.T) {
	span := source.Span{}
	tests := []struct {
		name  string
		build func() (*Collector, func(*Collector))
	}{
		{
			name: "Source invalid span",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-source.lua", 0, bind.GlobalCensus{})
				body := c.Source().Order().Body(span)
				return c, func(c *Collector) {
					c.Source().Literals().String(source.Span{File: "foreign.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, body, "x")
				}
			},
		},
		{
			name: "Flow future operand",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-flow.lua", 0, bind.GlobalCensus{})
				body := c.Source().Order().Body(span)
				future := keyspace.MakeTerm(keyspace.FamilyString, 1)
				return c, func(c *Collector) {
					c.Flow().Values().Values(span, body, []Term{future}, 0)
				}
			},
		},
		{
			name: "Static wrong family",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-static.lua", 0, bind.GlobalCensus{})
				_ = c.Source().Order().Body(span)
				future := keyspace.MakeTerm(keyspace.FamilyString, 1)
				return c, func(c *Collector) {
					c.Static().Types().Optional(span, future)
				}
			},
		},
		{
			name: "Module invalid call",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-module.lua", 1, bind.GlobalCensus{})
				future := keyspace.MakeTerm(keyspace.FamilyCall, 1)
				return c, func(c *Collector) {
					c.Module().Import(0, span, future)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, reject := test.build()
			sourceRoot, flowRoot, staticRoot, moduleRoot := c.Source(), c.Flow(), c.Static(), c.Module()
			reject(c)
			cause := c.err
			if cause == nil || !c.terminal {
				t.Fatalf("rejection did not retain cause/terminal state: cause=%v terminal=%v", cause, c.terminal)
			}
			if !collectorScratchCleared(c) {
				t.Fatal("rejection retained construction scratch")
			}
			if sourceRoot.Order().Body(span) != 0 || flowRoot.Storage().Cell(span, 0) != 0 || staticRoot.Types().Primitive(span, 1) != 0 || moduleRoot.Import(0, span, 0) != 0 {
				t.Fatal("captured writer root mutated a terminal Collector")
			}
			_ = sourceRoot.Literals().String(span, 0, "post-terminal")
			_ = sourceRoot.Keys().Name(span, 0, "post-terminal")
			_ = staticRoot.Types().LiteralString(span, "post-terminal")
			if c.err != cause {
				t.Fatalf("later mutation replaced first cause: got %v want %v", c.err, cause)
			}
			prepared, err := c.Prepare()
			if prepared.state != nil || err != cause {
				t.Fatalf("Prepare after rejection = %#v/%v, want exact first cause", prepared, err)
			}
		})
	}
}

func TestSuccessfulPrepareLeavesCauseNilAcrossLaterMutations(t *testing.T) {
	const name = "terminal-success.lua"
	span := source.Span{File: name}
	c := New(name, 0, bind.GlobalCensus{})
	body := c.Source().Order().Body(span)
	if body == 0 || !c.Source().Order().SetBody(body) || !c.Source().Order().SetEntry(body) {
		t.Fatalf("setup failed: %v", failure(c))
	}
	if _, err := c.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if c.err != nil || !c.terminal {
		t.Fatalf("successful Prepare cause/lifecycle = %v/%v", c.err, c.terminal)
	}
	_ = c.Source().Literals().String(span, body, "late")
	_ = c.Source().Keys().Name(span, body, "late")
	_ = c.Static().Types().LiteralString(span, "late")
	_ = c.Source().Order().Body(span)
	if c.err != nil {
		t.Fatalf("later successful-terminal mutations poisoned Collector: %v", c.err)
	}
	if _, err := c.Prepare(); !errors.Is(err, errCollectorTerminal) {
		t.Fatalf("second Prepare = %v, want terminal lifecycle error", err)
	}
}

func TestNilCollectorMutationsAreInert(t *testing.T) {
	var c *Collector
	if got := c.Source().Order().Body(source.Span{}); got != 0 {
		t.Fatalf("nil Source Body = %v, want zero", got)
	}
	if got := c.Flow().Storage().Cell(source.Span{}, 0); got != 0 {
		t.Fatalf("nil Flow Cell = %v, want zero", got)
	}
	if got := c.Static().Types().Optional(source.Span{}, 0); got != 0 {
		t.Fatalf("nil Static Optional = %v, want zero", got)
	}
	if got := c.Module().SetImportAlias(0, 0); got {
		t.Fatal("nil Module alias unexpectedly succeeded")
	}
}

// Copied terms are admitted by their owning vertical before the sidecar or
// Source order is touched. Keep this matrix compact, but cover each public
// mutator family which carries a child/sidecar term across that boundary.
func TestCopiedTermMutationsRejectAtAdmission(t *testing.T) {
	type testCase struct {
		name  string
		build func() (*Collector, func(*Collector))
	}
	const name = "copied-term-admission.lua"
	span := source.Span{File: name}
	newBody := func(c *Collector) Term { return c.Source().Order().Body(span) }
	cases := []testCase{
		{
			name: "Function generics future TypeParam",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				function := c.Flow().Functions().DeclareFunction(span, body)
				future := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
				return c, func(c *Collector) { c.Flow().Functions().SetFunctionGenerics(function, []Term{future}) }
			},
		},
		{
			name: "Function returns future Static node",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				function := c.Flow().Functions().DeclareFunction(span, body)
				future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
				return c, func(c *Collector) { c.Flow().Functions().SetFunctionReturns(function, true, []Term{future}) }
			},
		},
		{
			name: "Call type arguments future Static node",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				value := c.Source().Literals().String(span, body, "callee")
				values := c.Flow().Values().Values(span, body, []Term{value}, 0)
				call := c.Flow().Calls().DeclareCall(span, body, value, 0, values)
				future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
				return c, func(c *Collector) { c.Flow().Calls().SetCallTypeArgs(call, []Term{future}) }
			},
		},
		{
			name: "TypeValue future Static node",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
				return c, func(c *Collector) { c.Flow().Operands().TypeValue(span, body, future) }
			},
		},
		{
			name: "Function formal foreign local",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner, foreign := newBody(c), newBody(c)
				cell := c.Flow().Storage().Cell(span, foreign)
				function := c.Flow().Functions().DeclareFunction(span, owner)
				return c, func(c *Collector) { c.Flow().Functions().FillFunction(function, owner, []Term{cell}, 0, nil) }
			},
		},
		{
			name: "Bind foreign local",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner, foreign := newBody(c), newBody(c)
				value := c.Source().Literals().Bool(span, owner, true)
				values := c.Flow().Values().Values(span, owner, []Term{value}, 0)
				cell := c.Flow().Storage().Cell(span, foreign)
				return c, func(c *Collector) { c.Flow().Storage().Bind(span, owner, []Term{cell}, values) }
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c, reject := test.build()
			if c.err != nil || c.terminal {
				t.Fatalf("test setup failed: %v", failure(c))
			}
			reject(c)
			cause := c.err
			if cause == nil || !c.terminal || !collectorScratchCleared(c) {
				t.Fatalf("copied-term rejection was not terminal/cleared: err=%v terminal=%v", cause, c.terminal)
			}
			if c.Source().Order().Body(span) != 0 || c.err != cause {
				t.Fatalf("terminal copied-term collector reopened or replaced cause: %v", c.err)
			}
		})
	}
}

func TestNewFailureStopsConstructionAndPrepareRetainsCause(t *testing.T) {
	tests := []struct {
		name string
		new  func() *Collector
	}{
		{name: "empty source name", new: func() *Collector { return New("", 0, bind.GlobalCensus{}) }},
		{name: "negative import census", new: func() *Collector { return New("invalid.lua", -1, bind.GlobalCensus{}) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := test.new()
			cause := c.err
			if cause == nil || !c.terminal || !collectorScratchCleared(c) {
				t.Fatalf("New failure did not terminalize/clear: err=%v terminal=%v", cause, c.terminal)
			}
			if c.Source().Order().Body(source.Span{}) != 0 || c.err != cause {
				t.Fatalf("post-New mutation changed terminal cause: %v", c.err)
			}
			prepared, err := c.Prepare()
			if prepared.state != nil || err != cause {
				t.Fatalf("Prepare after New failure = %#v/%v, want exact cause", prepared, err)
			}
		})
	}
}
