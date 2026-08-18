package assembly

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// Source suite restored from admission_gap_law_test.go.
func TestCollectorAdmissionGapMatrixTerminalizes(t *testing.T) {
	const name = "collector-admission-gaps.lua"
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}
	body := func(c *Collector) keyspace.Term { return c.Body(span) }
	assign := func(c *Collector, owner, cell keyspace.Term) keyspace.Term {
		value := c.Bool(span, owner, true)
		values := c.Values(span, owner, []keyspace.Term{value}, 0)
		return c.Assign(span, owner, []keyspace.Term{cell}, []source.Span{span}, values)
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
				literal := c.String(span, owner, "leaf")
				return c, func(c *Collector) { c.SetBody(owner, literal) }
			},
		},
		{
			name: "Body rejects duplicate direct root",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				label := c.Label(span, owner)
				return c, func(c *Collector) { c.SetBody(owner, label, label) }
			},
		},
		{
			name: "Body rejects direct root owned by another Body",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				first, second := body(c), body(c)
				label := c.Label(span, second)
				return c, func(c *Collector) { c.SetBody(first, label) }
			},
		},
		{
			name: "LensExact requires candidate",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				integer := c.Integer(span, owner, 7)
				nonExact := c.Unary(span, owner, kind.UnaryBitNot, integer)
				return c, func(c *Collector) { c.LensExact(span, owner, integer, nonExact, kind.FieldExact) }
			},
		},
		{
			name: "TableField requires candidate",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				integer := c.Integer(span, owner, 7)
				nonExact := c.Unary(span, owner, kind.UnaryBitNot, integer)
				values := c.Values(span, owner, []keyspace.Term{integer}, 0)
				table := c.DeclareTable(span, owner)
				return c, func(c *Collector) { c.TableField(span, table, nonExact, values, kind.FieldExact) }
			},
		},
		{
			name: "Alias generics require owned unique params",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				first := c.Alias(span, span, owner, "A")
				param := c.TypeParam(span, first, "T")
				return c, func(c *Collector) { c.AliasParams(first, []keyspace.Term{param, param}) }
			},
		},
		{
			name: "TypeFunction generics require owned params",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				host := c.Alias(span, span, owner, "Host")
				first := c.TypeFunction(span, host)
				second := c.TypeFunction(span, host)
				param := c.TypeParam(span, first, "T")
				return c, func(c *Collector) { c.TypeFunctionGenerics(second, []keyspace.Term{param}) }
			},
		},
		{
			name: "Flow Function generics require owned params",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				first := c.DeclareFunction(span, owner)
				second := c.DeclareFunction(span, owner)
				param := c.TypeParam(span, first, "T")
				return c, func(c *Collector) { c.SetFunctionGenerics(second, []keyspace.Term{param}) }
			},
		},
		{
			name: "Function formals reject reused Cell",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Cell(span, owner, "")
				function := c.DeclareFunction(span, owner)
				return c, func(c *Collector) { c.FillFunction(function, owner, []keyspace.Term{cell, cell}, 0, nil) }
			},
		},
		{
			name: "Bind rejects reused Cell",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Cell(span, owner, "")
				values := c.Values(span, owner, nil, 0)
				return c, func(c *Collector) { c.Bind(span, owner, []keyspace.Term{cell, cell}, values) }
			},
		},
		{
			name: "Interface method requires interface-scoped signature",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				first := c.Interface(span, span, owner, "I")
				second := c.Interface(span, span, owner, "J")
				signature := c.TypeFunction(span, first)
				member := StaticInterfaceMember{Kind: programstatic.InterfaceMethod, Name: "m", Span: span, Signature: signature}
				return c, func(c *Collector) {
					c.InterfaceMembers(second, []StaticInterfaceMember{member})
				}
			},
		},
		{
			name: "Publication rejects unresolved TypeRef",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Cell(span, owner, "")
				publicationAssign := assign(c, owner, cell)
				ref := c.Unresolved(span, []string{"Missing"}, 0)
				return c, func(c *Collector) { c.Type(span, publicationAssign, 0, ref) }
			},
		},
		{
			name: "Publication rejects duplicate Assign pair",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				cell := c.Cell(span, owner, "")
				publicationAssign := assign(c, owner, cell)
				primitive := c.Primitive(span, programstatic.PrimitiveString)
				alias := c.Alias(span, span, owner, "A")
				c.AliasTarget(alias, primitive)
				ref := c.Declaration(span, []string{"A"}, 0, alias)
				first := c.Type(span, publicationAssign, 0, ref)
				if first == 0 {
					return c, func(*Collector) {}
				}
				return c, func(c *Collector) { c.Type(span, publicationAssign, 0, ref) }
			},
		},
		{
			name: "TypeValue requires runtime-loadable target",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner := body(c)
				staticOnly := c.Primitive(span, programstatic.PrimitiveFunction)
				return c, func(c *Collector) { c.TypeValue(span, owner, staticOnly) }
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
			if c.err == nil || !c.terminal {
				t.Fatalf("gap rejection not terminal: err=%v terminal=%v", c.err, c.terminal)
			}
		})
	}
}

// Source suite restored from global_law_test.go.
func TestGlobalCensusReservesPrefixAndSeedsSource(t *testing.T) {
	const name = "global-prefix.lua"
	binding := bindFixture(t, name, "local first = alpha\nlocal second = beta")
	census := binding.GlobalCensus()
	if census.Len() != 2 {
		t.Fatalf("global census length = %d, want 2", census.Len())
	}
	c := New(name, 0, census)
	span := source.Span{File: name}
	body := c.Body(span)
	for index, want := range []string{"alpha", "beta"} {
		cell, ok := census.At(index)
		if !ok || cell.Slot() != uint32(index) || cell.Ordinal() != uint32(index+1) {
			t.Fatalf("census slot %d = %#v/%v", index, cell, ok)
		}
		if cell.Name() != want {
			t.Fatalf("census slot %d name = %q, want %q", index, cell.Name(), want)
		}
	}
	local := c.Cell(span, body, "")
	if want := keyspace.MakeTerm(keyspace.FamilyCell, 3); local != want {
		t.Fatalf("first local Cell = %v, want reserved-prefix successor %v", local, want)
	}
	value := c.Bool(span, body, true)
	values := c.Values(span, body, []keyspace.Term{value}, 0)
	bindTerm := c.Bind(span, body, []keyspace.Term{local}, values)
	if body == 0 || value == 0 || values == 0 || bindTerm == 0 || !c.SetBody(body, bindTerm) || !c.SetEntry(body) {
		t.Fatalf("Source body setup failed: %v", failure(c))
	}
	published := publishGlobalLaw(t, c)
	sourceView := published.Source()
	identity := sourceView.Identity()
	if got := identity.FamilyCount(keyspace.FamilyCell); got != 3 {
		t.Fatalf("Cell family count = %d, want global prefix plus local", got)
	}
	for index, want := range []string{"alpha", "beta"} {
		cell, _ := census.At(index)
		term := keyspace.MakeTerm(keyspace.FamilyCell, cell.Ordinal())
		gotSpan, ok := identity.Span(term)
		wantSpan, err := globalOriginSpan(name, cell.Origin())
		if err != nil || !ok || gotSpan != wantSpan {
			t.Fatalf("global %q span = %#v/%v, want %#v: %v", want, gotSpan, ok, wantSpan, err)
		}
		if key, ok := sourceView.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: want}); !ok || key == 0 {
			t.Fatalf("Source exact census omitted global %q", want)
		}
	}
	if gotSpan, ok := identity.Span(local); !ok || gotSpan != span {
		t.Fatalf("local Cell span = %#v/%v, want %#v", gotSpan, ok, span)
	}
}

func TestGlobalCensusExcludesRuntimeTypeOnlyIdentity(t *testing.T) {
	const name = "global-type-only.lua"
	stmts, err := parse.ParseString("type Shape = number\nlocal value = Shape(1)", name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	binding := bind.BindChunk(stmts)
	assign := stmts[1].(*ast.LocalAssignStmt)
	base := assign.Exprs[0].(*ast.FuncCallExpr).Func.(*ast.IdentExpr)
	identity, ok := binding.GlobalIdentity(base)
	if !ok {
		t.Fatal("runtime type-only identity missing")
	}
	if got := binding.GlobalCensus().Len(); got != 0 {
		t.Fatalf("runtime type-only census length = %d, want 0", got)
	}
	c := New(name, 0, binding.GlobalCensus())
	if got := c.Global(identity); got != 0 {
		t.Fatalf("runtime type-only Global = %v, want rejection", got)
	}
	if published, err := c.Publish(); err == nil || published != nil {
		t.Fatalf("Publish after rejected runtime type-only Global = %#v/%v, want terminal failure", published, err)
	}
}

func TestGlobalCensusCopiesHaveDeterministicCollectorLayout(t *testing.T) {
	const name = "global-deterministic.lua"
	first := bindFixture(t, name, "local first = alpha\nlocal second = beta")
	second := bindFixture(t, name, "local first = alpha\nlocal second = beta")
	firstCensus := first.GlobalCensus()
	secondCensus := second.GlobalCensus()
	if firstCensus.Len() != secondCensus.Len() {
		t.Fatalf("census lengths = %d/%d", firstCensus.Len(), secondCensus.Len())
	}
	for index := 0; index < firstCensus.Len(); index++ {
		left, leftOK := firstCensus.At(index)
		right, rightOK := secondCensus.At(index)
		if !leftOK || !rightOK || left.Name() != right.Name() || left.Slot() != right.Slot() ||
			left.Ordinal() != right.Ordinal() || left.Origin() != right.Origin() {
			t.Fatalf("census copy slot %d = %#v/%v and %#v/%v", index, left, leftOK, right, rightOK)
		}
	}
	left := New(name, 0, firstCensus)
	right := New(name, 0, secondCensus)
	span := source.Span{File: name}
	for _, c := range []*Collector{left, right} {
		body := c.Body(span)
		if body == 0 || !c.SetBody(body) || !c.SetEntry(body) {
			t.Fatalf("Source setup failed: %v", failure(c))
		}
	}
	leftPublished := publishGlobalLaw(t, left)
	rightPublished := publishGlobalLaw(t, right)
	leftSource := leftPublished.Source()
	rightSource := rightPublished.Source()
	leftIdentity := leftSource.Identity()
	rightIdentity := rightSource.Identity()
	if leftIdentity.FamilyCount(keyspace.FamilyCell) != rightIdentity.FamilyCount(keyspace.FamilyCell) ||
		leftIdentity.ContentID() != rightIdentity.ContentID() {
		t.Fatalf("published Source identities differ: %v/%v", leftIdentity.ContentID(), rightIdentity.ContentID())
	}
	for index := 0; index < firstCensus.Len(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
		leftSpan, leftOK := leftIdentity.Span(term)
		rightSpan, rightOK := rightIdentity.Span(term)
		if !leftOK || !rightOK || leftSpan != rightSpan {
			t.Fatalf("published Cell span %d differs: %#v/%v and %#v/%v", index+1, leftSpan, leftOK, rightSpan, rightOK)
		}
	}
	if leftSource.Keys().ExactCount() != rightSource.Keys().ExactCount() {
		t.Fatalf("published exact counts differ: %d/%d", leftSource.Keys().ExactCount(), rightSource.Keys().ExactCount())
	}
	for index := 0; index < leftSource.Keys().ExactCount(); index++ {
		leftKey, leftAtom, leftOK := leftSource.Keys().ExactAt(index)
		rightKey, rightAtom, rightOK := rightSource.Keys().ExactAt(index)
		if !leftOK || !rightOK || leftKey != rightKey || leftAtom != rightAtom {
			t.Fatalf("published exact atom %d differs: %v/%#v/%v and %v/%#v/%v", index, leftKey, leftAtom, leftOK, rightKey, rightAtom, rightOK)
		}
	}
}

func TestGlobalCensusSelectionAcrossLargeReservedSet(t *testing.T) {
	const (
		name  = "global-large.lua"
		count = 512
	)
	var text strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&text, "local value%d = global%d\n", index, index)
	}
	stmts, err := parse.ParseString(text.String(), name)
	if err != nil {
		t.Fatalf("parse large census: %v", err)
	}
	binding := bind.BindChunk(stmts)
	if got := binding.GlobalCensus().Len(); got != count {
		t.Fatalf("large census length = %d, want %d", got, count)
	}
	c := New(name, 0, binding.GlobalCensus())
	for index, stmt := range stmts {
		assign := stmt.(*ast.LocalAssignStmt)
		identity, ok := binding.GlobalIdentity(assign.Exprs[0].(*ast.IdentExpr))
		if !ok {
			t.Fatalf("global identity %d missing", index)
		}
		got := c.Global(identity)
		want := keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
		if got != want {
			t.Fatalf("global %d = %v, want reserved %v", index, got, want)
		}
	}
	order := c
	body := order.Body(source.Span{File: name})
	if body == 0 || !order.SetBody(body) || !order.SetEntry(body) {
		t.Fatalf("large Source setup failed: %v", failure(c))
	}
	published := publishGlobalLaw(t, c)
	sourceView := published.Source()
	if got := sourceView.Identity().FamilyCount(keyspace.FamilyCell); got != count {
		t.Fatalf("published large Cell count = %d, want %d", got, count)
	}
	if got := sourceView.Keys().ExactCount(); got != count {
		t.Fatalf("published large exact count = %d, want %d", got, count)
	}
}

func publishGlobalLaw(t *testing.T, c *Collector) *program.Program {
	t.Helper()
	published, err := c.Publish()
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return published
}

func bindFixture(t testing.TB, name, text string) *bind.Result {
	t.Helper()
	stmts, err := parse.ParseString(text, name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return bind.BindChunk(stmts)
}

// Source suite restored from terminal_rejection_law_test.go.
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
				body := c.Body(span)
				return c, func(c *Collector) {
					c.String(source.Span{File: "foreign.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, body, "x")
				}
			},
		},
		{
			name: "Flow future operand",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-flow.lua", 0, bind.GlobalCensus{})
				body := c.Body(span)
				future := keyspace.MakeTerm(keyspace.FamilyString, 1)
				return c, func(c *Collector) {
					c.Values(span, body, []keyspace.Term{future}, 0)
				}
			},
		},
		{
			name: "Static wrong family",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-static.lua", 0, bind.GlobalCensus{})
				_ = c.Body(span)
				future := keyspace.MakeTerm(keyspace.FamilyString, 1)
				return c, func(c *Collector) {
					c.Optional(span, future)
				}
			},
		},
		{
			name: "Module invalid call",
			build: func() (*Collector, func(*Collector)) {
				c := New("terminal-module.lua", 1, bind.GlobalCensus{})
				future := keyspace.MakeTerm(keyspace.FamilyCall, 1)
				return c, func(c *Collector) {
					c.Import(0, span, future)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, reject := test.build()
			sourceRoot, flowRoot, staticRoot, moduleRoot := c, c, c, c
			reject(c)
			cause := c.err
			if cause == nil || !c.terminal {
				t.Fatalf("rejection did not retain cause/terminal state: cause=%v terminal=%v", cause, c.terminal)
			}
			if sourceRoot.Body(span) != 0 || flowRoot.Cell(span, 0, "") != 0 || staticRoot.Primitive(span, 1) != 0 || moduleRoot.Import(0, span, 0) != 0 {
				t.Fatal("captured writer root mutated a terminal Collector")
			}
			_ = sourceRoot.String(span, 0, "post-terminal")
			_ = sourceRoot.Name(span, 0, "post-terminal")
			_ = staticRoot.LiteralString(span, "post-terminal")
			if c.err != cause {
				t.Fatalf("later mutation replaced first cause: got %v want %v", c.err, cause)
			}
			published, err := c.Publish()
			if published != nil || err != cause {
				t.Fatalf("Publish after rejection = %#v/%v, want exact first cause", published, err)
			}
		})
	}
}

func TestSuccessfulPublishLeavesCauseNilAcrossLaterMutations(t *testing.T) {
	const name = "terminal-success.lua"
	span := source.Span{File: name}
	c := New(name, 0, bind.GlobalCensus{})
	body := c.Body(span)
	if body == 0 || !c.SetBody(body) || !c.SetEntry(body) {
		t.Fatalf("setup failed: %v", failure(c))
	}
	if _, err := c.Publish(); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if c.err != nil || !c.terminal {
		t.Fatalf("successful Publish cause/lifecycle = %v/%v", c.err, c.terminal)
	}
	_ = c.String(span, body, "late")
	_ = c.Name(span, body, "late")
	_ = c.LiteralString(span, "late")
	_ = c.Body(span)
	if c.err != nil {
		t.Fatalf("later successful-terminal mutations poisoned Collector: %v", c.err)
	}
	if _, err := c.Publish(); !errors.Is(err, errCollectorTerminal) {
		t.Fatalf("second Publish = %v, want terminal lifecycle error", err)
	}
}

func TestNilCollectorMutationsAreInert(t *testing.T) {
	var c *Collector
	if got := c.Body(source.Span{}); got != 0 {
		t.Fatalf("nil Source Body = %v, want zero", got)
	}
	if got := c.Cell(source.Span{}, 0, ""); got != 0 {
		t.Fatalf("nil Flow Cell = %v, want zero", got)
	}
	if got := c.Optional(source.Span{}, 0); got != 0 {
		t.Fatalf("nil Static Optional = %v, want zero", got)
	}
	if got := c.SetImportAlias(0, 0); got {
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
	newBody := func(c *Collector) keyspace.Term { return c.Body(span) }
	cases := []testCase{
		{
			name: "Function generics future TypeParam",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				function := c.DeclareFunction(span, body)
				future := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
				return c, func(c *Collector) { c.SetFunctionGenerics(function, []keyspace.Term{future}) }
			},
		},
		{
			name: "Function returns future Static node",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				function := c.DeclareFunction(span, body)
				future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
				return c, func(c *Collector) { c.SetFunctionReturns(function, true, []keyspace.Term{future}) }
			},
		},
		{
			name: "Call type arguments future Static node",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				value := c.String(span, body, "callee")
				values := c.Values(span, body, []keyspace.Term{value}, 0)
				call := c.DeclareCall(span, body, value, 0, values, "")
				future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
				return c, func(c *Collector) { c.SetCallTypeArgs(call, []keyspace.Term{future}) }
			},
		},
		{
			name: "TypeValue future Static node",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				body := newBody(c)
				future := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
				return c, func(c *Collector) { c.TypeValue(span, body, future) }
			},
		},
		{
			name: "Function formal foreign local",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner, foreign := newBody(c), newBody(c)
				cell := c.Cell(span, foreign, "")
				function := c.DeclareFunction(span, owner)
				return c, func(c *Collector) { c.FillFunction(function, owner, []keyspace.Term{cell}, 0, nil) }
			},
		},
		{
			name: "Bind foreign local",
			build: func() (*Collector, func(*Collector)) {
				c := New(name, 0, bind.GlobalCensus{})
				owner, foreign := newBody(c), newBody(c)
				value := c.Bool(span, owner, true)
				values := c.Values(span, owner, []keyspace.Term{value}, 0)
				cell := c.Cell(span, foreign, "")
				return c, func(c *Collector) { c.Bind(span, owner, []keyspace.Term{cell}, values) }
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
			if cause == nil || !c.terminal {
				t.Fatalf("copied-term rejection was not terminal: err=%v terminal=%v", cause, c.terminal)
			}
			if c.Body(span) != 0 || c.err != cause {
				t.Fatalf("terminal copied-term collector reopened or replaced cause: %v", c.err)
			}
		})
	}
}

func TestNewFailureStopsConstructionAndPublishRetainsCause(t *testing.T) {
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
			if cause == nil || !c.terminal {
				t.Fatalf("New failure did not terminalize: err=%v terminal=%v", cause, c.terminal)
			}
			if c.Body(source.Span{}) != 0 || c.err != cause {
				t.Fatalf("post-New mutation changed terminal cause: %v", c.err)
			}
			published, err := c.Publish()
			if published != nil || err != cause {
				t.Fatalf("Publish after New failure = %#v/%v, want exact cause", published, err)
			}
		})
	}
}

// Module admission is part of the Collector lifecycle: reserved Import rows
// are owned by the module vertical, and an invalid alias must terminalize the
// same Collector that owns the Source, Flow, and Static rows.
func TestModuleRootRejectsFutureCellAliasAtOwnerBoundary(t *testing.T) {
	c := New("module-future-cell.lua", 1, bind.GlobalCensus{})
	future := keyspace.MakeTerm(keyspace.FamilyCell, 2)
	if c.SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 1), future) {
		t.Fatal("future Cell alias unexpectedly accepted")
	}
	if c.Body(source.Span{}) != 0 {
		t.Fatal("future alias rejection did not terminalize Collector")
	}
}

func TestModuleRootReservedImportCannotPopulateAnotherOrdinal(t *testing.T) {
	c := New("module-reserved.lua", 2, bind.GlobalCensus{})
	if c.SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 2), keyspace.MakeTerm(keyspace.FamilyCell, 1)) {
		t.Fatal("unfilled reserved Import accepted an alias")
	}
	if c.Body(source.Span{}) != 0 {
		t.Fatal("reserved Import rejection did not terminalize Collector")
	}
	outside := New("module-outside.lua", 1, bind.GlobalCensus{})
	if outside.SetImportAlias(keyspace.MakeTerm(keyspace.FamilyImport, 2), keyspace.MakeTerm(keyspace.FamilyCell, 1)) {
		t.Fatal("Import beyond census accepted an alias")
	}
}

func TestCollectorModuleObservationFillsReservedSlotsOutOfOrder(t *testing.T) {
	c := New("module.lua", 3, bind.GlobalCensus{})
	span := func(line uint32) source.Span {
		return source.Span{File: "module.lua", StartLine: line, StartCol: 1, EndLine: line, EndCol: 8}
	}
	body := c.Body(span(1))
	makeCall := func(line uint32) keyspace.Term {
		request := c.String(span(line), body, "pkg")
		values := c.Values(span(line), body, []keyspace.Term{request}, 0)
		return c.DeclareCall(span(line), body, request, 0, values, "")
	}
	call1, call2, call3 := makeCall(10), makeCall(20), makeCall(30)
	if c.Import(2, span(30), call3) != keyspace.MakeTerm(keyspace.FamilyImport, 3) ||
		c.Import(0, span(10), call1) != keyspace.MakeTerm(keyspace.FamilyImport, 1) ||
		c.Import(1, span(20), call2) != keyspace.MakeTerm(keyspace.FamilyImport, 2) {
		t.Fatal("reserved Import slots did not retain census order")
	}
	if c.Import(0, span(11), call1) != 0 {
		t.Fatal("duplicate reserved Import was accepted")
	}
}

func TestModuleRootRejectsEmptyStringRequestBeforeExactAdmission(t *testing.T) {
	const name = "module-empty-request.lua"
	c := New(name, 1, bind.GlobalCensus{})
	span := source.Span{File: name, StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 8}
	body := c.Body(span)
	request := c.String(span, body, "")
	values := c.Values(span, body, []keyspace.Term{request}, 0)
	call := c.DeclareCall(span, body, request, 0, values, "")
	if body == 0 || request == 0 || values == 0 || call == 0 {
		t.Fatal("empty request construction failed")
	}
	if got := c.Import(0, span, call); got != 0 {
		t.Fatalf("empty Module Import = %v, want rejection", got)
	}
	if c.Body(span) != 0 {
		t.Fatal("empty Module request rejection did not terminalize")
	}
}
