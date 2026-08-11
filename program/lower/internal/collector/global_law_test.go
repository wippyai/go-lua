package collector

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func TestGlobalCensusReservesPrefixAndSeedsSource(t *testing.T) {
	const name = "global-prefix.lua"
	binding := bindFixture(t, name, "local first = alpha\nlocal second = beta")
	census := binding.GlobalCensus()
	if census.Len() != 2 {
		t.Fatalf("global census length = %d, want 2", census.Len())
	}
	c := New(name, 0, census)
	span := source.Span{File: name}
	body := c.Source().Order().Body(span)
	for index, want := range []string{"alpha", "beta"} {
		cell, ok := census.At(index)
		if !ok || cell.Slot() != uint32(index) || cell.Ordinal() != uint32(index+1) {
			t.Fatalf("census slot %d = %#v/%v", index, cell, ok)
		}
		if cell.Name() != want {
			t.Fatalf("census slot %d name = %q, want %q", index, cell.Name(), want)
		}
	}
	local := c.Flow().Storage().Cell(span, body)
	if want := keyspace.MakeTerm(keyspace.FamilyCell, 3); local != want {
		t.Fatalf("first local Cell = %v, want reserved-prefix successor %v", local, want)
	}
	value := c.Source().Literals().Bool(span, body, true)
	values := c.Flow().Values().Values(span, body, []keyspace.Term{value}, 0)
	bindTerm := c.Flow().Storage().Bind(span, body, []keyspace.Term{local}, values)
	if body == 0 || value == 0 || values == 0 || bindTerm == 0 || !c.Source().Order().SetBody(body, bindTerm) || !c.Source().Order().SetEntry(body) {
		t.Fatalf("Source body setup failed: %v", failure(c))
	}
	lease := prepareGlobalLaw(t, c)
	preimage := preparedSourcePreimage(t, lease.prepared)
	identity := preimage.Identity()
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
		if key, ok := preimage.Keys().Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: want}); !ok || key == 0 {
			t.Fatalf("Source exact census omitted global %q", want)
		}
	}
	if gotSpan, ok := identity.Span(local); !ok || gotSpan != span {
		t.Fatalf("local Cell span = %#v/%v, want %#v", gotSpan, ok, span)
	}
	lease.assemble(t)
	if preimage.Identity().Name() != "" {
		t.Fatal("Source preimage remained live after Assemble")
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
	if got := c.Flow().Storage().Global(identity); got != 0 {
		t.Fatalf("runtime type-only Global = %v, want rejection", got)
	}
	if prepared, err := c.Prepare(); err == nil || prepared.state != nil {
		t.Fatalf("Prepare after rejected runtime type-only Global = %#v/%v, want terminal failure", prepared, err)
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
		body := c.Source().Order().Body(span)
		if body == 0 || !c.Source().Order().SetBody(body) || !c.Source().Order().SetEntry(body) {
			t.Fatalf("Source setup failed: %v", failure(c))
		}
	}
	leftLease := prepareGlobalLaw(t, left)
	rightLease := prepareGlobalLaw(t, right)
	leftPreimage := preparedSourcePreimage(t, leftLease.prepared)
	rightPreimage := preparedSourcePreimage(t, rightLease.prepared)
	leftIdentity := leftPreimage.Identity()
	rightIdentity := rightPreimage.Identity()
	if leftIdentity.FamilyCount(keyspace.FamilyCell) != rightIdentity.FamilyCount(keyspace.FamilyCell) ||
		leftIdentity.ContentID() != rightIdentity.ContentID() {
		t.Fatalf("prepared Source identities differ: %v/%v", leftIdentity.ContentID(), rightIdentity.ContentID())
	}
	for index := 0; index < firstCensus.Len(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
		leftSpan, leftOK := leftIdentity.Span(term)
		rightSpan, rightOK := rightIdentity.Span(term)
		if !leftOK || !rightOK || leftSpan != rightSpan {
			t.Fatalf("prepared Cell span %d differs: %#v/%v and %#v/%v", index+1, leftSpan, leftOK, rightSpan, rightOK)
		}
	}
	if leftPreimage.Keys().ExactCount() != rightPreimage.Keys().ExactCount() {
		t.Fatalf("prepared exact counts differ: %d/%d", leftPreimage.Keys().ExactCount(), rightPreimage.Keys().ExactCount())
	}
	for index := 0; index < leftPreimage.Keys().ExactCount(); index++ {
		leftKey, leftAtom, leftOK := leftPreimage.Keys().ExactAt(index)
		rightKey, rightAtom, rightOK := rightPreimage.Keys().ExactAt(index)
		if !leftOK || !rightOK || leftKey != rightKey || leftAtom != rightAtom {
			t.Fatalf("prepared exact atom %d differs: %v/%#v/%v and %v/%#v/%v", index, leftKey, leftAtom, leftOK, rightKey, rightAtom, rightOK)
		}
	}
	leftLease.assemble(t)
	rightLease.assemble(t)
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
		got := c.Flow().Storage().Global(identity)
		want := keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1))
		if got != want {
			t.Fatalf("global %d = %v, want reserved %v", index, got, want)
		}
	}
	order := c.Source().Order()
	body := order.Body(source.Span{File: name})
	if body == 0 || !order.SetBody(body) || !order.SetEntry(body) {
		t.Fatalf("large Source setup failed: %v", failure(c))
	}
	lease := prepareGlobalLaw(t, c)
	preimage := preparedSourcePreimage(t, lease.prepared)
	if got := preimage.Identity().FamilyCount(keyspace.FamilyCell); got != count {
		t.Fatalf("prepared large Cell count = %d, want %d", got, count)
	}
	if got := preimage.Keys().ExactCount(); got != count {
		t.Fatalf("prepared large exact count = %d, want %d", got, count)
	}
	lease.assemble(t)
}

type globalLawLease struct {
	prepared Prepared
	consumed bool
}

func prepareGlobalLaw(t *testing.T, c *Collector) *globalLawLease {
	t.Helper()
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	lease := &globalLawLease{prepared: prepared}
	t.Cleanup(func() {
		if lease.consumed {
			return
		}
		if err := abortPreparedSourceForTest(t, lease.prepared); err != nil {
			t.Errorf("cleanup Source Abort: %v", err)
		}
		assembly, err := lease.prepared.Assemble()
		lease.consumed = true
		if err == nil || assembly != nil {
			t.Errorf("cleanup Assemble after Source Abort = %v/%v, want terminal failure", assembly, err)
		}
	})
	return lease
}

func (lease *globalLawLease) assemble(t *testing.T) {
	t.Helper()
	if lease == nil || lease.consumed {
		t.Fatal("prepared global law lease was absent or already consumed")
	}
	assembly, err := lease.prepared.Assemble()
	lease.consumed = true
	if err != nil || assembly == nil {
		t.Fatalf("Assemble = %v/%v", assembly, err)
	}
}

func bindFixture(t testing.TB, name, text string) *bind.Result {
	t.Helper()
	stmts, err := parse.ParseString(text, name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return bind.BindChunk(stmts)
}
