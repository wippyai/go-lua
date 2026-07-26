package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
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

func joinTestHeapIdentity(term, operation, identity string, guards ...equation.Guard) equation.Fact {
	fact := heapIdentityFact(term, operation, []byte(identity))
	fact.Guards = guards
	return fact
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

// treeNodeArms is the discriminated union the joined-surface tests reconverge:
// one arm a literal-tagged text node, the other a literal-tagged group node.
func treeNodeArms() (typ.Type, typ.Type) {
	text := typetable.NewRecord().Field("kind", typ.LiteralString("text")).Field("value", typ.String).Build()
	group := typetable.NewRecord().Field("kind", typ.LiteralString("group")).Field("count", typ.Number).Build()
	return text, group
}

// TestJoinedSurfaceKeepsWhatEachEdgeProved states the rule a local written on
// both edges of a branch depends on. One edge assigned a literal and holds its
// type directly; the other assigned a call result and holds an honest unknown
// value beside the summary that call proved. The joined point is the union of
// the two witnesses, so a discriminant read past the branch still has a surface
// to select an arm from.
func TestJoinedSurfaceKeepsWhatEachEdgeProved(t *testing.T) {
	text, group := treeNodeArms()
	literal, ok := shapefact.EncodeTarget(text)
	if !ok {
		t.Fatal("encode arm literal witness")
	}
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/tree/op-00000001", Value: []byte("scalar/nil")},
		equation.Fact{Key: "value/path/tree/op-00000003", Value: literal, Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: "value/path/tree/op-00000004", Value: []byte("scalar/top"), Guards: []equation.Guard{joinTestGuard("op-00000002", "false")}},
		equation.Fact{Key: summaryTypePrefix + "path/tree/op-00000004", Value: mustCanonicalType(group), Guards: []equation.Guard{joinTestGuard("op-00000002", "false")}},
	)
	// The value lane alone still reports the honest unknown: one edge published
	// no value witness at all.
	if value, found := latestValue([]byte("path/tree"), partition); !found || !isUnknownScalar(value) {
		t.Fatalf("value lane at the join = %q / %v, want the unknown scalar", value, found)
	}
	root, suffix, source, resolved := typedAncestor([]byte("path/tree.kind"), partition)
	if !resolved {
		t.Fatal("the joined point published no member surface")
	}
	if string(root) != "path/tree" || len(suffix) != 1 {
		t.Fatalf("ancestor = %s with %d segments, want path/tree with one", root, len(suffix))
	}
	if !typ.TypeEquals(source, normalize.UnionForEvidence(text, group)) {
		t.Fatalf("joined surface = %v, want the union of both edge witnesses", source)
	}
}

// TestJoinedSurfaceWithholdsWhenAnEdgeProvedNothing is the falsifiable half: an
// edge that published neither a value witness nor a summary states nothing
// about the joined point, so the other edge must not speak for it.
func TestJoinedSurfaceWithholdsWhenAnEdgeProvedNothing(t *testing.T) {
	text, _ := treeNodeArms()
	literal, ok := shapefact.EncodeTarget(text)
	if !ok {
		t.Fatal("encode arm literal witness")
	}
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/tree/op-00000001", Value: []byte("scalar/nil")},
		equation.Fact{Key: "value/path/tree/op-00000003", Value: literal, Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: "value/path/tree/op-00000004", Value: []byte("scalar/top"), Guards: []equation.Guard{joinTestGuard("op-00000002", "false")}},
	)
	if value, found := reconvergedSurfaceValue([]byte("path/tree"), partition); !found || !isUnknownScalar(value) {
		t.Fatalf("joined surface = %q / %v, want the unknown scalar", value, found)
	}
	if _, _, _, resolved := typedAncestor([]byte("path/tree.kind"), partition); resolved {
		t.Fatal("an edge that proved nothing still produced a member surface")
	}
}

// TestJoinedSurfaceRefusesAClaimPayload holds the trust boundary: a declaration
// recorded as a claim on the declaring write is user-asserted, not a checker
// proof, and therefore contributes no witness to the join.
func TestJoinedSurfaceRefusesAClaimPayload(t *testing.T) {
	text, _ := treeNodeArms()
	literal, ok := shapefact.EncodeTarget(text)
	if !ok {
		t.Fatal("encode arm literal witness")
	}
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: "value/path/tree/op-00000001", Value: []byte("scalar/nil")},
		equation.Fact{Key: "value/path/tree/op-00000002", Value: []byte("scalar/claim/claim-kind/3/claim-type/\"TreeNode\"")},
		equation.Fact{Key: "value/path/tree/op-00000004", Value: literal, Guards: []equation.Guard{joinTestGuard("op-00000003", "true")}},
	)
	if value, found := reconvergedSurfaceValue([]byte("path/tree"), partition); !found || !isUnknownScalar(value) {
		t.Fatalf("joined surface = %q / %v, want the unknown scalar", value, found)
	}
}

// TestJoinedSurfaceStaysPrivateInsideAnEdge keeps the join from erasing the
// precision it exists to preserve: inside one edge that edge's own publication
// is the surface, not the union with its complement.
func TestJoinedSurfaceStaysPrivateInsideAnEdge(t *testing.T) {
	text, group := treeNodeArms()
	literal, ok := shapefact.EncodeTarget(text)
	if !ok {
		t.Fatal("encode arm literal witness")
	}
	facts := []equation.Fact{
		{Key: "value/path/tree/op-00000001", Value: []byte("scalar/nil")},
		{Key: "value/path/tree/op-00000003", Value: literal, Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		{Key: "value/path/tree/op-00000004", Value: []byte("scalar/top"), Guards: []equation.Guard{joinTestGuard("op-00000002", "false")}},
		{Key: summaryTypePrefix + "path/tree/op-00000004", Value: mustCanonicalType(group), Guards: []equation.Guard{joinTestGuard("op-00000002", "false")}},
	}
	inside := joinTestPartition(t, []equation.Guard{joinTestGuard("op-00000002", "true")}, facts...)
	_, _, source, resolved := typedAncestor([]byte("path/tree.kind"), inside)
	if !resolved || !typ.TypeEquals(source, text) {
		t.Fatalf("inside the true edge surface = %v / %v, want the arm literal alone", source, resolved)
	}
}

// TestInheritedSummaryJoinsBothEdges is the rule a copy made past a branch
// depends on. A short-circuit chain writes its left operand before its own
// guard and its right operand on the taken edge, so the binding it feeds is
// reached through both edges. Reading a single visible row there returns the
// row the guard consumed -- the arm's overwrite is guarded and invisible -- and
// would state that the copy holds the left operand on a path where it holds the
// right one.
func TestInheritedSummaryJoinsBothEdges(t *testing.T) {
	left, right := typ.MaterializeOptional(typ.String), typ.MaterializeOptional(typ.Number)
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000001", Value: []byte("op-00000001")},
		equation.Fact{Key: summaryTypePrefix + "temp/1/op-00000001", Value: mustCanonicalType(left)},
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000003", Value: []byte("op-00000003"), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: summaryTypePrefix + "temp/1/op-00000003", Value: mustCanonicalType(right), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	encoded, found := reconvergedEpochFact(summaryTypePrefix, []byte("temp/1"), partition, joinSummaryTypes)
	if !found {
		t.Fatal("the joined point inherited no summary")
	}
	joined, err := decodeSummaryType(encoded)
	if err != nil {
		t.Fatalf("decode joined summary: %v", err)
	}
	if !typ.TypeEquals(joined, normalize.UnionForEvidence(left, right)) {
		t.Fatalf("joined summary = %v, want the union of both edges", joined)
	}
	// Inside the taken edge the copy still holds that edge's own summary: the
	// join belongs to the point both edges reach, not to either of them.
	inside := joinTestPartition(t, []equation.Guard{joinTestGuard("op-00000002", "true")},
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000001", Value: []byte("op-00000001")},
		equation.Fact{Key: summaryTypePrefix + "temp/1/op-00000001", Value: mustCanonicalType(left)},
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000003", Value: []byte("op-00000003"), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: summaryTypePrefix + "temp/1/op-00000003", Value: mustCanonicalType(right), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	armEncoded, armFound := reconvergedEpochFact(summaryTypePrefix, []byte("temp/1"), inside, joinSummaryTypes)
	if !armFound {
		t.Fatal("the taken edge inherited no summary")
	}
	arm, err := decodeSummaryType(armEncoded)
	if err != nil {
		t.Fatalf("decode edge summary: %v", err)
	}
	if !typ.TypeEquals(arm, right) {
		t.Fatalf("edge summary = %v, want the edge's own publication", arm)
	}
}

// TestInheritedSummaryWithholdsWhenAnEdgeProvedNothing is the falsifiable half.
// An edge whose current version publishes no summary states nothing about the
// joined point, so the other edge must not speak for it and the copy inherits
// no summary at all.
func TestInheritedSummaryWithholdsWhenAnEdgeProvedNothing(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000001", Value: []byte("op-00000001")},
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000003", Value: []byte("op-00000003"), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		equation.Fact{Key: summaryTypePrefix + "temp/1/op-00000003", Value: mustCanonicalType(typ.String), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
	)
	if encoded, found := reconvergedEpochFact(summaryTypePrefix, []byte("temp/1"), partition, joinSummaryTypes); found {
		t.Fatalf("an edge that published no summary still inherited %q", encoded)
	}
}

// TestInheritedSummaryKeepsEpochAuthority holds the version rule the join is
// built on. A later write that publishes no summary retires the earlier one:
// the row the copy could still find describes a value the write replaced.
func TestInheritedSummaryKeepsEpochAuthority(t *testing.T) {
	partition := joinTestPartition(t, nil,
		equation.Fact{Key: epochFactPrefix + "path/x/op-00000001", Value: []byte("op-00000001")},
		equation.Fact{Key: summaryTypePrefix + "path/x/op-00000001", Value: mustCanonicalType(typ.String)},
		equation.Fact{Key: epochFactPrefix + "path/x/op-00000002", Value: []byte("op-00000002")},
	)
	if encoded, found := reconvergedEpochFact(summaryTypePrefix, []byte("path/x"), partition, joinSummaryTypes); found {
		t.Fatalf("a retired summary was inherited as %q", encoded)
	}
}

// TestInheritedNameJoinsOnlyByAgreement separates the two lattices. A payload
// that names something -- a heap identity, a select origin -- has no widened
// form, so two edges that name different things state nothing joint and the
// copy inherits nothing.
func TestInheritedNameJoinsOnlyByAgreement(t *testing.T) {
	agreeing := joinTestPartition(t, nil,
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000001", Value: []byte("op-00000001")},
		joinTestHeapIdentity("temp/1", "op-00000001", "sealed-table/01/op-00000000"),
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000003", Value: []byte("op-00000003"), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		joinTestHeapIdentity("temp/1", "op-00000003", "sealed-table/01/op-00000000", joinTestGuard("op-00000002", "true")),
	)
	identity, found := reconvergedFamilyEpochFact(factkey.HeapTableIdentity, []byte("temp/1"), agreeing, joinAgreedValues)
	if !found || string(identity) != "sealed-table/01/op-00000000" {
		t.Fatalf("agreeing edges inherited %q / %v, want the common identity", identity, found)
	}
	disagreeing := joinTestPartition(t, nil,
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000001", Value: []byte("op-00000001")},
		joinTestHeapIdentity("temp/1", "op-00000001", "sealed-table/01/op-00000000"),
		equation.Fact{Key: epochFactPrefix + "temp/1/op-00000003", Value: []byte("op-00000003"), Guards: []equation.Guard{joinTestGuard("op-00000002", "true")}},
		joinTestHeapIdentity("temp/1", "op-00000003", "sealed-table/01/op-00000004", joinTestGuard("op-00000002", "true")),
	)
	if identity, found := reconvergedFamilyEpochFact(factkey.HeapTableIdentity, []byte("temp/1"), disagreeing, joinAgreedValues); found {
		t.Fatalf("edges naming different identities inherited %q", identity)
	}
}
