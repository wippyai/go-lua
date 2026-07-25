package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func joinTestGuard(name, edge string) equation.Guard {
	return equation.Guard{Body: equation.BodyID{1}, Encoding: []byte("front/branch/" + name + "/" + edge)}
}

func joinTestPartition(t *testing.T, guards []equation.Guard, facts ...equation.Fact) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(guards, equation.OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

// TestArmWriteReachesPostDominatorAsUnion is the headline reconvergence case:
// `local x = 0; if c then x = "s" end` leaves the point after the branch holding
// both values.  Reading the pre-branch write alone would state that x is 0 on a
// path where it is "s".
func TestArmWriteReachesPostDominatorAsUnion(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"s\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	value, found := latestValue([]byte("path/x"), partition)
	if !found {
		t.Fatal("post-join read has no value")
	}
	joined, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		t.Fatalf("post-join value %q is not a type witness", value)
	}
	if !typ.TypeEquals(joined, typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralString("s")})) {
		t.Fatalf("post-join value = %v, want 0 | \"s\"", joined)
	}
	// The union is the value a claim is checked against: a `string` target must
	// be refuted by the surviving number member.
	if relation := valueAgainstType(value, typ.String); relation != shapeRefuted {
		t.Fatalf("string claim against joined value = %v, want refuted", relation)
	}
	if relation := valueAgainstType(value, typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})); relation == shapeRefuted {
		t.Fatal("number | string claim against joined value was refuted")
	}
}

// TestArmValueStaysPrivateInsideItsOwnEdge keeps the join from erasing the
// precision it depends on: inside the arm the arm's own write is the value, and
// inside the complementary edge it is not visible at all.
func TestArmValueStaysPrivateInsideItsOwnEdge(t *testing.T) {
	facts := []equation.Fact{
		{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"s\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	}
	inside := joinTestPartition(t, []equation.Guard{joinTestGuard("op-00000002", "true")}, facts...)
	if value, found := latestValue([]byte("path/x"), inside); !found || string(value) != "scalar/string/\"s\"" {
		t.Fatalf("inside the true edge value = %q / %v, want the arm write", value, found)
	}
	outside := joinTestPartition(t, []equation.Guard{joinTestGuard("op-00000002", "false")}, facts...)
	if value, found := latestValue([]byte("path/x"), outside); !found || string(value) != "scalar/number/0" {
		t.Fatalf("inside the false edge value = %q / %v, want the pre-branch write", value, found)
	}
}

// TestParallelGuardsDoNotJoin is the precision guardrail from the ruling: two
// guards that are inactive for unrelated reasons are not alternatives.  The
// value after the second branch joins that branch's edges, and the first
// branch's arm write reaches it only through its own join -- never by being
// treated as the complement of the second decision.
func TestParallelGuardsDoNotJoin(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"a\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: "value/path/x/op-00000005", Value: []byte("scalar/bool/true"), Guards: []equation.Guard{joinTestGuard("op-00000004", "true")}},
	)
	value, found := latestValue([]byte("path/x"), partition)
	if !found {
		t.Fatal("post-join read has no value")
	}
	joined, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		t.Fatalf("post-join value %q is not a type witness", value)
	}
	want := typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralString("a"), typ.True})
	if !typ.TypeEquals(joined, want) {
		t.Fatalf("post-join value = %v, want %v", joined, want)
	}
	// Under the second decision's false edge the second arm write is gone, but
	// the first decision is still open and must still join on its own.
	inner := joinTestPartition(t, []equation.Guard{joinTestGuard("op-00000004", "false")},
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"a\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: "value/path/x/op-00000005", Value: []byte("scalar/bool/true"), Guards: []equation.Guard{joinTestGuard("op-00000004", "true")}},
	)
	value, found = latestValue([]byte("path/x"), inner)
	if !found {
		t.Fatal("false-edge read has no value")
	}
	joined, decoded = shapefact.DecodeTarget(value)
	if !decoded {
		t.Fatalf("false-edge value %q is not a type witness", value)
	}
	want = typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralString("a")})
	if !typ.TypeEquals(joined, want) {
		t.Fatalf("false-edge value = %v, want %v -- the parallel guard's write leaked", joined, want)
	}
}

// TestNestedArmWriteJoinsOneDecisionAtATime checks that an inner arm write under
// an outer arm reaches the outermost point, and that the inner join happens
// under the outer edge rather than being flattened into one four-way merge of
// unrelated cubes.
func TestNestedArmWriteJoinsOneDecisionAtATime(t *testing.T) {
	outer, inner := joinTestGuard("op-00000002", "true"), joinTestGuard("op-00000003", "true")
	facts := []equation.Fact{
		{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		{Key: "value/path/x/op-00000004", Value: []byte("scalar/string/\"s\""), Guards: []equation.Guard{outer, inner}},
	}
	underOuter := joinTestPartition(t, []equation.Guard{outer}, facts...)
	value, found := latestValue([]byte("path/x"), underOuter)
	if !found {
		t.Fatal("inner join has no value")
	}
	joined, decoded := shapefact.DecodeTarget(value)
	if !decoded || !typ.TypeEquals(joined, typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralString("s")})) {
		t.Fatalf("inner join value = %q, want 0 | \"s\"", value)
	}
	full := joinTestPartition(t, nil, facts...)
	value, found = latestValue([]byte("path/x"), full)
	if !found {
		t.Fatal("outer join has no value")
	}
	joined, decoded = shapefact.DecodeTarget(value)
	if !decoded || !typ.TypeEquals(joined, typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.LiteralString("s")})) {
		t.Fatalf("outer join value = %q, want 0 | \"s\"", value)
	}
}

// TestProvenBranchEdgeReplacesTheJoin keeps a decided branch precise: once the
// front has published the branch proof, only the feasible edge contributes and
// no union is formed.
func TestProvenBranchEdgeReplacesTheJoin(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"s\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: "branch-proof/0100000000000000000000000000000000000000000000000000000000000000/op-00000002/true", Value: []byte("proven")},
	)
	value, found := latestValue([]byte("path/x"), partition)
	if !found || string(value) != "scalar/string/\"s\"" {
		t.Fatalf("proven-edge value = %q / %v, want the sole feasible arm write", value, found)
	}
}

// TestRevokedEdgeContributionIsNotResurrected covers the epoch rule: an arm
// write that a later write inside the same arm supersedes contributes only the
// surviving row.  The dead row must not reappear as that edge's join member.
func TestRevokedEdgeContributionIsNotResurrected(t *testing.T) {
	guard := joinTestGuard("op-00000002", "true")
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"dead\""), Guards: []equation.Guard{guard}},
		equation.Fact{Key: "value/path/x/op-00000004", Value: []byte("scalar/bool/true"), Guards: []equation.Guard{guard}},
	)
	value, found := latestValue([]byte("path/x"), partition)
	if !found {
		t.Fatal("post-join read has no value")
	}
	joined, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		t.Fatalf("post-join value %q is not a type witness", value)
	}
	want := typ.MaterializeUnion([]typ.Type{typ.LiteralInt(0), typ.True})
	if !typ.TypeEquals(joined, want) {
		t.Fatalf("post-join value = %v, want %v -- a superseded arm row was joined", joined, want)
	}
}

// TestReadBoundaryAppliesInsideEveryEdge keeps the join flow-sensitive: a read
// placed before the branch must not see the arm write through the join.
func TestReadBoundaryAppliesInsideEveryEdge(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/number/0")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"s\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	value, err := resolveValue([]byte("path/x"), []byte("front/read-before/op-00000002"), []byte("front/absence/error"), partition)
	if err != nil {
		t.Fatalf("bounded read: %v", err)
	}
	if string(value) != "scalar/number/0" {
		t.Fatalf("bounded read = %q, want the pre-branch write", value)
	}
}

// TestIncompleteEdgeCoverageWithholdsTheJoin is the fail-closed rule: an arm
// write whose complementary edge carries no value at all is not a globally
// valid value, and the join must withhold rather than publish the single arm.
func TestIncompleteEdgeCoverageWithholdsTheJoin(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/string/\"s\""), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	if value, found := latestValue([]byte("path/x"), partition); found {
		t.Fatalf("single-edge value %q escaped its guard", value)
	}
}

// TestJoinedNilArmComposesWithOrSemantics is the short-circuit case.  An arm
// that writes nil makes the point optional, and `x or default` then resolves
// against that joined value under Lua's truthiness rule: the nil member cannot
// survive the left side of an `or`, and the right operand joins in.  Reading the
// pre-branch value alone would type the expression as the left operand alone.
func TestJoinedNilArmComposesWithOrSemantics(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/x/op-00000001", Value: []byte("scalar/string/\"a\"")},
		equation.Fact{Key: "value/path/x/op-00000003", Value: []byte("scalar/nil"), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	left, found := latestValue([]byte("path/x"), partition)
	if !found {
		t.Fatal("post-join read has no value")
	}
	joined, decoded := shapefact.DecodeTarget(left)
	if !decoded || !typ.TypeEquals(joined, normalize.UnionForEvidence(typ.Nil, typ.LiteralString("a"))) {
		t.Fatalf("post-join left operand = %q, want \"a\" | nil", left)
	}
	result := undecidedLogicalValue(left, []byte("scalar/string/\"d\""), wir.LogOr)
	value, decoded := shapefact.DecodeTarget(result)
	if !decoded {
		t.Fatalf("or result %q is not a type witness", result)
	}
	want := normalize.UnionForEvidence(typ.LiteralString("a"), typ.LiteralString("d"))
	if !typ.TypeEquals(value, want) {
		t.Fatalf("or result = %v, want %v", value, want)
	}
	// The nil that entered through the guarded arm is consumed by `or`, not
	// carried past it: the whole point of joining it in is that the expression
	// can then eliminate it soundly.
	if valueAgainstType(result, typ.String) == shapeRefuted {
		t.Fatal("or result was refuted against string")
	}
}

// TestSealedArmTablesJoinStructurally keeps the join from discarding evidence:
// two literal tables written on complementary edges reconverge as the union of
// their recorded structures, not as an unknown scalar.
func TestSealedArmTablesJoinStructurally(t *testing.T) {
	before, ok := shapefact.EncodeTable(shapefact.Table{Closed: true, Members: []shapefact.Member{{Suffix: ".answer", Present: true, Value: "scalar/nil"}}})
	if !ok {
		t.Fatal("encode pre-branch table")
	}
	arm, ok := shapefact.EncodeTable(shapefact.Table{Closed: true, Members: []shapefact.Member{{Suffix: ".answer", Present: true, Value: "scalar/string/\"ok\""}}})
	if !ok {
		t.Fatal("encode arm table")
	}
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/m/op-00000001", Value: before},
		equation.Fact{Key: "value/path/m/op-00000003", Value: arm, Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	value, found := latestValue([]byte("path/m"), partition)
	if !found {
		t.Fatal("post-join table read has no value")
	}
	if isUnknownScalar(value) {
		t.Fatal("post-join table collapsed to the unknown scalar")
	}
	joined, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		t.Fatalf("post-join table value %q is not a type witness", value)
	}
	want := normalize.UnionForEvidence(
		typetable.NewRecord().Field("answer", typ.Nil).Build(),
		typetable.NewRecord().Field("answer", typ.LiteralString("ok")).Build(),
	)
	if !typ.TypeEquals(joined, want) {
		t.Fatalf("joined table = %v, want %v", joined, want)
	}
}
