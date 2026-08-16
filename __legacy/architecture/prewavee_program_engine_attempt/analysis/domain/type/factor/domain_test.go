package typefactor_test

import (
	"context"
	"testing"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	typefactor "github.com/wippyai/go-lua/analysis/domain/type/factor"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
	programlower "github.com/wippyai/go-lua/analysis/program/lower"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestTypeFactorAssemblesSequentialValuesFromRetainedSubjects(t *testing.T) {
	project, shard, p := project(t, "values.lua", `return 7, "value", true`)
	values := returnValues(t, p)

	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := typefactor.Install(solver, project)
	if !ok {
		t.Fatal("install Type Factor")
	}
	query, ok := domain.Query(shard, values, values)
	if !ok {
		t.Fatal("declare Values query")
	}
	if !solver.Seal() {
		t.Fatal("seal Type Factor composition")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok || state == nil {
		t.Fatal("solve Type Factor composition")
	}
	pack, present := query.Read(state)
	if !present {
		t.Fatal("Values Pack absent")
	}
	assertClosed(t, domain.Table(), pack,
		typ.LiteralInt(7), typ.LiteralString("value"), typ.LiteralBool(true))
	assertOriginCount(t, pack, 0)
}

func TestTypeFactorKeepsSameRawTermsIsolatedAcrossShards(t *testing.T) {
	left := lower(t, "left.lua", `return 11`)
	right := lower(t, "right.lua", `return 22`)
	project := linked(t, []link.Module{{Name: "left", Program: left}, {Name: "right", Program: right}})
	leftShard := shardFor(t, project, left)
	rightShard := shardFor(t, project, right)
	leftValue, _ := left.IntegerAt(0)
	rightValue, _ := right.IntegerAt(0)
	if leftValue != rightValue {
		t.Fatal("fixture did not reuse the same raw Program Term across shards")
	}

	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := typefactor.Install(solver, project)
	if !ok {
		t.Fatal("install Type Factor")
	}
	leftQuery, ok := domain.Query(leftShard, leftValue, leftValue)
	if !ok {
		t.Fatal("left query")
	}
	rightQuery, ok := domain.Query(rightShard, rightValue, rightValue)
	if !ok {
		t.Fatal("right query")
	}
	if !solver.Seal() {
		t.Fatal("seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok {
		t.Fatal("solve")
	}
	leftPack, leftPresent := leftQuery.Read(state)
	rightPack, rightPresent := rightQuery.Read(state)
	if !leftPresent || !rightPresent {
		t.Fatalf("literal facts present=%v/%v", leftPresent, rightPresent)
	}
	assertClosed(t, domain.Table(), leftPack, typ.LiteralInt(11))
	assertClosed(t, domain.Table(), rightPack, typ.LiteralInt(22))
	assertOriginCount(t, leftPack, 0)
	assertOriginCount(t, rightPack, 0)
}

func TestTypeFactorSeparatesRepeatedProgramAcrossShards(t *testing.T) {
	// Reusing the exact Program instance makes the literal Rule semantic key
	// identical in both shards. Seal therefore proves that the shard anchor,
	// rather than a Program pointer or an incidental declaration order, keeps
	// the two equations and the two dense Type subjects distinct.
	p := lower(t, "shared.lua", `return 17, "shared"`)
	project := linked(t, []link.Module{
		{Name: "left", Program: p},
		{Name: "right", Program: p},
	})
	left, leftOK := project.ShardAt(0)
	right, rightOK := project.ShardAt(1)
	if !leftOK || !rightOK || left == right {
		t.Fatal("missing distinct Link shards")
	}
	values := returnValues(t, p)

	solver, err := engine.New(project)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := typefactor.Install(solver, project)
	if !ok {
		t.Fatal("install Type Factor")
	}
	leftQuery, ok := domain.Query(left, values, values)
	if !ok {
		t.Fatal("left Values query")
	}
	rightQuery, ok := domain.Query(right, values, values)
	if !ok {
		t.Fatal("right Values query")
	}
	if !solver.Seal() {
		t.Fatal("same semantic Program in two shards rejected at seal")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok {
		t.Fatal("solve")
	}
	leftPack, leftPresent := leftQuery.Read(state)
	rightPack, rightPresent := rightQuery.Read(state)
	if !leftPresent || !rightPresent {
		t.Fatalf("Values facts present=%v/%v", leftPresent, rightPresent)
	}
	assertClosed(t, domain.Table(), leftPack, typ.LiteralInt(17), typ.LiteralString("shared"))
	assertClosed(t, domain.Table(), rightPack, typ.LiteralInt(17), typ.LiteralString("shared"))
	assertOriginCount(t, leftPack, 0)
	assertOriginCount(t, rightPack, 0)
}

func TestTypeFactorAcceptsEquivalentLinkAuthorityWithoutPointerCoupling(t *testing.T) {
	// Solver ownership is semantic Link identity, not Go allocation identity.
	// This is essential for a persisted/redecoded Program artifact: the type
	// domain may be assembled from an equivalent Link but must never accept a
	// different project merely because dense terms coincide.
	ownedProgram := lower(t, "equivalent.lua", `return 0.0, 1.5, "ok"`)
	equivalentProgram := lower(t, "equivalent.lua", `return 0.0, 1.5, "ok"`)
	owned := linked(t, []link.Module{{Name: "main", Program: ownedProgram}})
	equivalent := linked(t, []link.Module{{Name: "main", Program: equivalentProgram}})
	if owned == equivalent || owned.ContentID() != equivalent.ContentID() {
		t.Fatal("fixture did not construct distinct equivalent Links")
	}
	shard, ok := equivalent.ShardAt(0)
	if !ok {
		t.Fatal("equivalent shard")
	}
	values := returnValues(t, equivalentProgram)
	firstLiteral, ok := equivalentProgram.FloatAt(0)
	if !ok {
		t.Fatal("equivalent first literal")
	}

	solver, err := engine.New(owned)
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := typefactor.Install(solver, equivalent)
	if !ok {
		t.Fatal("install equivalent Type Factor")
	}
	query, ok := domain.Query(shard, values, values)
	if !ok {
		t.Fatal("equivalent Values query")
	}
	literalQuery, ok := domain.Query(shard, firstLiteral, firstLiteral)
	if !ok {
		t.Fatal("equivalent literal query")
	}
	if !solver.Seal() {
		t.Fatal("seal equivalent Link Type Factor")
	}
	state, ok := solver.Solve(context.Background(), nil)
	if !ok {
		t.Fatal("solve equivalent Link Type Factor")
	}
	pack, present := query.Read(state)
	if !present {
		t.Fatal("equivalent Values Pack absent")
	}
	literalPack, literalPresent := literalQuery.Read(state)
	if !literalPresent {
		t.Fatal("equivalent literal Pack absent")
	}
	assertClosed(t, domain.Table(), literalPack, typ.LiteralNumber(0.0))
	assertClosed(t, domain.Table(), pack,
		typ.LiteralNumber(0.0), typ.LiteralNumber(1.5), typ.LiteralString("ok"))
	assertOriginCount(t, literalPack, 0)
	assertOriginCount(t, pack, 0)
}

func assertOriginCount(t testing.TB, value carrier.Value, want int) {
	t.Helper()
	origins, ok := value.Origins()
	if !ok || origins.Count() != want {
		t.Fatalf("carrier origins=%d/%v, want %d", origins.Count(), ok, want)
	}
}

func TestTypeFactorRejectsForeignSolverAuthorityAndNonSubjects(t *testing.T) {
	owned, shard, p := project(t, "owned.lua", `return 1`)
	foreign, _, _ := project(t, "foreign.lua", `return 2`)
	solver, err := engine.New(owned)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := typefactor.Install(solver, foreign); ok {
		t.Fatal("installed Type Factor from a foreign Link")
	}
	domain, ok := typefactor.Install(solver, owned)
	if !ok {
		t.Fatal("install owned Type Factor")
	}
	body, _ := p.BodyAt(0)
	if _, ok := domain.Query(shard, body, body); ok {
		t.Fatal("non-subject Body acquired a Type query")
	}
}

func assertClosed(t testing.TB, table *typedomain.Table, value carrier.Value, want ...typ.Type) {
	t.Helper()
	pack, ok := value.Data()
	if !ok {
		t.Fatal("Type Factor exposed no finite carrier data")
	}
	modes := pack.Modes()
	if len(modes) != 1 || modes[0].Kind() != typedomain.ModeClosed || modes[0].ClosedLen() != len(want) {
		t.Fatalf("Pack modes=%#v, want one closed pack of width %d", modes, len(want))
	}
	for index, expected := range want {
		handle, ok := modes[0].ClosedAt(index)
		if !ok {
			t.Fatalf("Pack[%d] absent", index)
		}
		got, err := table.Project(handle)
		if err != nil || !typ.TypeEquals(got, expected) {
			t.Fatalf("Pack[%d]=%v/%v, want %v", index, got, err, expected)
		}
	}
}

func project(t testing.TB, name, source string) (*link.Link, link.Shard, *program.Program) {
	t.Helper()
	p := lower(t, name, source)
	owner := linked(t, []link.Module{{Name: name, Program: p}})
	return owner, shardFor(t, owner, p), p
}

func lower(t testing.TB, name, source string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func linked(t testing.TB, modules []link.Module) *link.Link {
	t.Helper()
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := link.Seal(&link.Spec{Target: contract, Modules: modules})
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func shardFor(t testing.TB, owner *link.Link, p *program.Program) link.Shard {
	t.Helper()
	for index := 0; index < owner.ShardCount(); index++ {
		shard, ok := owner.ShardAt(index)
		if !ok {
			t.Fatal("missing shard")
		}
		candidate, ok := owner.Program(shard)
		if ok && candidate == p {
			return shard
		}
	}
	t.Fatal("Program not linked")
	return 0
}

func returnValues(t testing.TB, p *program.Program) program.Term {
	t.Helper()
	ret, ok := p.ReturnAt(0)
	if !ok {
		t.Fatal("missing Return")
	}
	values, ok := p.ReturnValues(ret)
	if !ok {
		t.Fatal("missing Return Values")
	}
	return values
}
