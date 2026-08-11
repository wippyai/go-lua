package link

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
)

func expressionForStaticRoot(t testing.TB, source *Link, shard linkproject.Shard, rootTerm keyspace.Term) linkstatic.Expression {
	t.Helper()
	for index := 0; index < source.Static().Expressions().Count(); index++ {
		expression, ok := source.Static().Expressions().At(index)
		if !ok {
			continue
		}
		reference, ok := source.Static().Expressions().Reference(expression)
		expressionShard, shardOK := source.Static().Expressions().Shard(expression)
		if ok && shardOK && expressionShard == shard && reference.Term() == rootTerm {
			return expression
		}
	}
	t.Fatalf("missing static expression for root %d", rootTerm)
	return linkstatic.Expression{}
}

func TestStaticExpressionRefReplaysAcrossDecodedProgram(t *testing.T) {
	p := source(t, `type Subject = { value: string }`)
	sealed := linked(t, contract(t), linkproject.Module{Name: "main", Program: p})
	root, ok := p.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing static alias")
	}
	expression := expressionForStaticRoot(t, sealed, onlyProjectShardFor(t, sealed, p), root)
	ref, ok := sealed.Static().Expressions().Ref(expression)
	if !ok {
		t.Fatal("missing static expression ref")
	}
	replayed := artifactAssertProjectionRoundTrip(t, sealed, contract(t), p)
	rebound, ok := replayed.Static().Expressions().Find(ref)
	if !ok {
		t.Fatal("decoded Program did not replay static expression ref")
	}
	reference, ok := replayed.Static().Expressions().Reference(rebound)
	if !ok || reference.Term() != ref.Reference() {
		t.Fatal("decoded Program changed portable static expression reference")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _ = sealed.Static().Expressions().Reference(expression)
		_, _ = sealed.Static().Expressions().Resolver(expression)
		_, _ = sealed.Static().Expressions().Ref(expression)
	}); allocations != 0 {
		t.Fatalf("hot static expression projections allocate %.2f times", allocations)
	}
}

func TestStaticExpressionRefsFenceDuplicateProgramContentByShard(t *testing.T) {
	first := source(t, `type Subject = string`)
	second := source(t, `type Subject = string`)
	if first.ContentID() != second.ContentID() {
		t.Fatal("equivalent Programs did not retain equal content identity")
	}
	sealed, err := Seal(&Spec{Target: contract(t), Modules: []linkproject.Module{
		{Name: "first", Program: first}, {Name: "second", Program: second},
	}})
	if err != nil {
		t.Fatal(err)
	}
	root, ok := first.Static().Declarations().Aliases().At(0)
	if !ok {
		t.Fatal("missing static alias")
	}
	left := expressionForStaticRoot(t, sealed, onlyProjectShardFor(t, sealed, first), root)
	right := expressionForStaticRoot(t, sealed, onlyProjectShardFor(t, sealed, second), root)
	leftRef, leftOK := sealed.Static().Expressions().Ref(left)
	rightRef, rightOK := sealed.Static().Expressions().Ref(right)
	if !leftOK || !rightOK || leftRef.Reference() != rightRef.Reference() || leftRef.ShardOrdinal() == rightRef.ShardOrdinal() || leftRef == rightRef {
		t.Fatal("duplicate Program content did not retain shard-fenced expression identity")
	}
	if _, ok := sealed.Static().Expressions().Find(leftRef); !ok {
		t.Fatal("first-shard static expression ref did not replay")
	}
	if _, ok := sealed.Static().Expressions().Find(rightRef); !ok {
		t.Fatal("second-shard static expression ref did not replay")
	}
}
