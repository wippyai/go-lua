package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExtendsRecord_NilTypes(t *testing.T) {
	if ExtendsRecord(nil, typ.String) {
		t.Error("nil a should not extend")
	}
	if ExtendsRecord(typ.String, nil) {
		t.Error("nil b should not extend")
	}
}

func TestExtendsRecord_NotRecord(t *testing.T) {
	if ExtendsRecord(typ.String, typ.String) {
		t.Error("non-record should not extend")
	}
}

func TestSplitNilable_ProjectsUnionWithoutRehashingMembers(t *testing.T) {
	calls := 0
	left := &countingHashType{name: "left", hash: 10, calls: &calls}
	right := &countingHashType{name: "right", hash: 20, calls: &calls}
	union, ok := typ.NewUnion(typ.Nil, left, right).(*typ.Union)
	if !ok {
		t.Fatalf("expected union")
	}
	if calls != 2 {
		t.Fatalf("NewUnion Hash calls = %d, want 2", calls)
	}

	calls = 0
	inner, nilable := SplitNilable(union)
	if !nilable {
		t.Fatalf("SplitNilable did not report nilable")
	}
	gotUnion, ok := inner.(*typ.Union)
	if !ok || len(gotUnion.Members) != 2 {
		t.Fatalf("SplitNilable inner = %T %[1]v, want two-member union", inner)
	}
	if calls != 0 {
		t.Fatalf("SplitNilable Hash calls = %d, want projection without rehashing", calls)
	}
}

func TestElidesOptional_UsesNilableProjection(t *testing.T) {
	baseline := typ.NewUnion(typ.Nil, typ.String, typ.Number)
	if !ElidesOptional(typ.String, baseline) {
		t.Fatalf("expected string to elide nil from %v", baseline)
	}
	if ElidesOptional(typ.Boolean, baseline) {
		t.Fatalf("boolean should not elide nil from %v", baseline)
	}
}

func TestElidesOptional_RecursiveArrayEvidenceUsesPrecisionFamily(t *testing.T) {
	entry := typ.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewOptional(typ.NewMap(typ.String, typ.Any))).
		Build()
	baseline := typ.NewOptional(typ.NewArray(entry))
	candidate := typ.NewRecursive("Inferred", func(self typ.Type) typ.Type {
		return typ.NewArray(entry)
	})

	if !ElidesOptional(candidate, baseline) {
		t.Fatalf("recursive array evidence should elide nilable array baseline:\ncandidate=%v\nbaseline=%v", candidate, baseline)
	}
}

func TestElidesOptional_RecursiveEvidenceRequiresFiniteFamilyProof(t *testing.T) {
	baseline := typ.NewOptional(typ.NewRecord().
		Field("items", typ.NewArray(typ.String)).
		Build())
	candidate := typ.NewRecursive("Growing", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("items", typ.NewArray(self)).
			Build()
	})

	if ElidesOptional(candidate, baseline) {
		t.Fatalf("recursive evidence without a finite family proof must not fall through to structural subtype")
	}
}

func TestFoldSelfEmbeddingRejectsTupleElementShapeDrift(t *testing.T) {
	leafList := typ.NewTuple(typ.NewRecord().
		Field("text", typ.String).
		Build())
	containerList := typ.NewTuple(typ.NewRecord().
		Field("content", typ.NewRecord().
			Field("parts", leafList).
			Build()).
		Build())

	if got, ok := FoldSelfEmbedding(leafList, containerList); ok {
		t.Fatalf("tuple element shape drift folded into recursive family: %v", got)
	}
}

type countingHashType struct {
	name  string
	hash  uint64
	calls *int
}

func (t *countingHashType) Kind() kind.Kind { return kind.String }
func (t *countingHashType) String() string  { return t.name }
func (t *countingHashType) Hash() uint64 {
	(*t.calls)++
	return t.hash
}
func (t *countingHashType) Equals(other typ.Type) bool { return t == other }

func TestExtendsRecord_MapComponentConsistency(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Build()
	if ExtendsRecord(newRec, oldRec) {
		t.Error("record without map component should not extend record with map component")
	}
}

func TestCollapseTableTopEvidence_AbsorbsPreciseTableMembers(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	preciseRecord := typ.NewRecord().
		Field("name", typ.String).
		Field("tools", typ.NewArray(typ.String)).
		Build()
	preciseMap := typ.NewMap(typ.String, typ.Integer)
	evidence := typ.NewUnion(typ.NewOptional(tableTop), preciseRecord, preciseMap, typ.String)

	got := CollapseTableTopEvidence(evidence)
	want := typ.NewUnion(typ.NewOptional(tableTop), typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected table top to absorb precise table members as %v, got %v", want, got)
	}
}

func TestSelectTableUpperBound_AbsorbsTableUnion(t *testing.T) {
	tableTop := typ.NewOptional(typ.NewInterface("table", nil))
	strategySpec := typ.NewRecord().
		Field("kind", typ.LiteralString("strategy")).
		Field("tools", typ.NewTuple(typ.String, typ.String, typ.String)).
		Build()
	contextSpec := typ.NewRecord().
		Field("kind", typ.LiteralString("context")).
		Field("scope", typ.String).
		Build()
	nextHint := typ.NewUnion(strategySpec, contextSpec)

	got, ok := SelectTableUpperBound(tableTop, nextHint)
	if !ok || !typ.TypeEquals(got, tableTop) {
		t.Fatalf("expected table top upper bound %v, got %v ok=%v", tableTop, got, ok)
	}
}

func TestJoinMapRecordShape_PureMapComponentBecomesMap(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	canonical := typ.NewMap(typ.String, typ.NewArray(entry))
	recordView := typ.NewRecord().
		MapComponent(typ.NewUnion(typ.String, typ.False), typ.NewArray(entry)).
		SetOpen(true).
		Build()
	join := func(a, b typ.Type) typ.Type {
		if IsTruthyRefinement(a, b) {
			return a
		}
		if IsTruthyRefinement(b, a) {
			return b
		}
		return typ.JoinPreferNonSoft(a, b)
	}

	got, ok := JoinMapRecordShape(canonical, recordView, join)
	if !ok || !typ.TypeEquals(got, canonical) {
		t.Fatalf("expected canonical map %v, got %v ok=%v", canonical, got, ok)
	}
}

func TestRecordSlotPreserves_RecursiveSlotsUseShallowEvidenceFamily(t *testing.T) {
	base := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	withProc := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("proc", typ.Any).
			Build()
	})

	baseline := typ.NewUnion(base, withProc)
	candidate := typ.NewUnion(withProc, base)
	if !recordSlotPreserves(candidate, baseline) {
		t.Fatalf("recursive union slots with the same shallow product family should preserve each other")
	}
}

func TestJoinMapShape_PreservesStringMapKeyAgainstAny(t *testing.T) {
	left := typ.NewMap(typ.String, typ.Any)
	right := typ.NewMap(typ.Any, typ.Any)

	got, ok := JoinMapShape(left, right, typ.JoinPreferNonSoft)
	if !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("JoinMapShape() = %v ok=%v, want %v", got, ok, left)
	}
}

func TestJoinMapRecordShape_PreservesStringMapKeyAgainstAnyMapComponent(t *testing.T) {
	left := typ.NewMap(typ.String, typ.Any)
	right := typ.NewRecord().MapComponent(typ.Any, typ.Any).Build()

	got, ok := JoinMapRecordShape(left, right, typ.JoinPreferNonSoft)
	if !ok || !typ.TypeEquals(got, left) {
		t.Fatalf("JoinMapRecordShape() = %v ok=%v, want %v", got, ok, left)
	}
}

func TestJoinMapRecordShape_PlainRecordBecomesRecordWithMapComponent(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	canonical := typ.NewMap(typ.String, entry)
	recordView := typ.NewRecord().
		Field("container", typ.NewRecord().Build()).
		SetOpen(true).
		Build()

	got, ok := JoinMapRecordShape(canonical, recordView, typ.JoinPreferNonSoft)
	want := typ.NewRecord().
		OptField("container", typ.JoinPreferNonSoft(typ.NewRecord().Build(), entry)).
		MapComponent(typ.String, entry).
		SetOpen(true).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("expected plain record plus map to become %v, got %v ok=%v", want, got, ok)
	}
}

func TestJoinMapShape_FoldsSelfEmbeddingMapGrowth(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	stable := typ.NewMap(typ.String, entry)
	growing := typ.NewMap(typ.String,
		typ.NewRecord().Field("child", stable).Build(),
	)

	got, ok := JoinMapShape(stable, growing, typ.JoinPreferNonSoft)
	rec, okRec := got.(*typ.Recursive)
	if !ok || !okRec {
		t.Fatalf("expected self-embedding map join to produce recursive type, got %T %v ok=%v", got, got, ok)
	}
	body, ok := rec.Body.(*typ.Map)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want map", rec.Body)
	}
	childRecord, ok := body.Value.(*typ.Record)
	if !ok {
		t.Fatalf("recursive map value = %T %[1]v, want record", body.Value)
	}
	child := childRecord.GetField("child")
	if child == nil || !typ.IsRecursiveRef(child.Type, rec) {
		t.Fatalf("recursive map child = %v, want recursive self reference %v", child, rec)
	}
}

func TestJoinMapRecordShape_FoldsSelfEmbeddingMapRecordGrowth(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	stable := typ.NewMap(typ.String, entry)
	growing := typ.NewRecord().
		Field("container", stable).
		MapComponent(typ.String, entry).
		SetOpen(true).
		Build()

	got, ok := JoinMapRecordShape(stable, growing, typ.JoinPreferNonSoft)
	rec, okRec := got.(*typ.Recursive)
	if !ok || !okRec {
		t.Fatalf("expected self-embedding map-record join to produce recursive type, got %T %v ok=%v", got, got, ok)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T %[1]v, want record", rec.Body)
	}
	container := body.GetField("container")
	if container == nil || !typ.IsRecursiveRef(container.Type, rec) {
		t.Fatalf("recursive record container = %v, want recursive self reference %v", container, rec)
	}
}

func TestJoinRecordShape_PlainRecordAndMapRecordShareMapComponent(t *testing.T) {
	entry := typ.NewRecord().OptField("proc", typ.Any).Build()
	plain := typ.NewRecord().
		Field("container", typ.NewRecord().Build()).
		SetOpen(true).
		Build()
	withMap := typ.NewRecord().
		MapComponent(typ.String, entry).
		SetOpen(true).
		Build()

	got, ok := JoinRecordShape(plain, withMap, typ.JoinPreferNonSoft)
	want := typ.NewRecord().
		OptField("container", typ.JoinPreferNonSoft(typ.NewRecord().Build(), entry)).
		MapComponent(typ.String, entry).
		SetOpen(true).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("expected record/map-record join %v, got %v ok=%v", want, got, ok)
	}
}

func TestJoinRecordShape_DirectFieldPresentOnBothSidesStaysRequiredWithMapComponent(t *testing.T) {
	plain := typ.NewRecord().
		Field("chars_per_token", typ.Integer).
		Field("prompt_buffer_tokens", typ.Integer).
		Build()
	withMap := typ.NewRecord().
		Field("chars_per_token", typ.Integer).
		Field("prompt_buffer_tokens", typ.Integer).
		MapComponent(typ.String, typ.Integer).
		Build()

	got, ok := JoinRecordShape(plain, withMap, typ.JoinPreferNonSoft)
	if !ok {
		t.Fatalf("JoinRecordShape() ok=false")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("JoinRecordShape() = %T %[1]v, want record", got)
	}
	for _, name := range []string{"chars_per_token", "prompt_buffer_tokens"} {
		field := rec.GetField(name)
		if field == nil {
			t.Fatalf("field %s missing from %v", name, rec)
		}
		if field.Optional {
			t.Fatalf("field %s became optional in %v", name, rec)
		}
		if !typ.TypeEquals(field.Type, typ.Integer) {
			t.Fatalf("field %s type = %v, want integer", name, field.Type)
		}
	}
}

func TestJoinRecordShape_StaticBracketMembersStaySeparateFromDotFields(t *testing.T) {
	left := typ.NewRecord().
		Field("name", typ.String).
		StaticStringIndex("raw-key", typ.Number).
		Build()
	right := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Boolean).
		Build()

	got, ok := JoinRecordShape(left, right, typ.JoinPreferNonSoft)
	if !ok {
		t.Fatal("JoinRecordShape() ok=false")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("JoinRecordShape() = %T %[1]v, want record", got)
	}
	field := rec.GetField("name")
	if field == nil || field.Optional || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("dot field name = %#v, want required string", field)
	}
	member := rec.GetStaticStringIndex("raw-key")
	if member == nil {
		t.Fatalf("static member [\"raw-key\"] missing from %v", rec)
	}
	if !member.Optional {
		t.Fatalf("static member [\"raw-key\"] = %#v, want optional after map branch", member)
	}
	want := typ.JoinPreferNonSoft(typ.Number, typ.Boolean)
	if !typ.TypeEquals(member.Type, want) {
		t.Fatalf("static member [\"raw-key\"] type = %v, want %v", member.Type, want)
	}
}

func TestJoinRecordShape_DisjointPartialRecordsBecomeOptionalFields(t *testing.T) {
	left := typ.NewRecord().Field("promptTokenCount", typ.Integer).Build()
	right := typ.NewRecord().Field("candidatesTokenCount", typ.Integer).Build()

	got, ok := JoinRecordShape(left, right, typ.JoinPreferNonSoft)
	want := typ.NewRecord().
		OptField("candidatesTokenCount", typ.Integer).
		OptField("promptTokenCount", typ.Integer).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("expected partial records to join as %v, got %v ok=%v", want, got, ok)
	}
}

func TestRecordWidthDiffer_IgnoresIdenticalRecord(t *testing.T) {
	rec := typ.NewRecord().
		Field("payload", typ.NewRecord().Field("id", typ.String).Build()).
		Build()
	if RecordWidthDiffer(rec, rec) {
		t.Fatalf("RecordWidthDiffer(same record) = true, want false")
	}
}

func TestJoinRecordShape_RecursiveAlternativesStayUnionable(t *testing.T) {
	inner := typ.NewRecord().Field("routes", typ.NewRecord().SetOpen(true).Build()).SetOpen(true).Build()
	outer := typ.NewRecord().Field("api", inner).SetOpen(true).Build()

	if got, ok := JoinRecordShape(outer, inner, typ.JoinPreferNonSoft); ok {
		t.Fatalf("expected recursive alternatives to stay distinct, got %v", got)
	}
}

func TestJoinRecordShape_AsymmetricLiteralTagStaysUnionable(t *testing.T) {
	user := typ.NewRecord().
		Field("kind", typ.LiteralString("user")).
		Field("content", typ.String).
		Build()
	tool := typ.NewRecord().
		Field("tool", typ.String).
		Field("arguments", typ.NewMap(typ.String, typ.Any)).
		Build()

	if got, ok := JoinRecordShape(user, tool, typ.JoinPreferNonSoft); ok {
		t.Fatalf("expected asymmetric literal tag alternatives to stay distinct, got %v", got)
	}
}

func TestJoinRecordShape_NonDiscriminantLiteralUpdatesCollapse(t *testing.T) {
	temperature := typ.NewRecord().
		Field("default_temperature", typ.LiteralNumber(0.8)).
		Build()
	unknown := typ.NewRecord().
		Field("unknown_key", typ.LiteralString("value")).
		Build()

	got, ok := JoinRecordShape(temperature, unknown, typ.JoinPreferNonSoft)
	if !ok {
		t.Fatalf("expected non-discriminant literal update records to collapse")
	}
	want := typ.NewRecord().
		OptField("default_temperature", typ.LiteralNumber(0.8)).
		OptField("unknown_key", typ.LiteralString("value")).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected collapsed update shape %v, got %v", want, got)
	}
}

func TestScanVisitsSharedStructuralNodeOnce(t *testing.T) {
	payload := typ.NewRecord().
		Field("value", typ.Integer).
		Build()
	root := typ.NewRecord().
		Field("left", payload).
		Field("right", payload).
		Build()

	visits := 0
	Scan(root, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if node == payload {
			visits++
		}
		return false, true
	})
	if visits != 1 {
		t.Fatalf("expected shared payload to be scanned once, got %d visits", visits)
	}
}

func TestScanVisitsEquivalentStructuralNodeOnce(t *testing.T) {
	left := typ.NewRecord().
		Field("value", typ.Integer).
		Build()
	right := typ.NewRecord().
		Field("value", typ.Integer).
		Build()
	root := typ.NewRecord().
		Field("left", left).
		Field("right", right).
		Build()

	visits := 0
	Scan(root, typ.NewGuard(), func(node typ.Type) (bool, bool) {
		if typ.TypeEquals(node, left) {
			visits++
		}
		return false, true
	})
	if visits != 1 {
		t.Fatalf("expected equivalent payload shapes to be scanned once, got %d visits", visits)
	}
}

func TestContainsEquivalentFindsNestedRecordByShape(t *testing.T) {
	target := typ.NewRecord().
		Field("name", typ.String).
		Build()
	root := typ.NewRecord().
		Field("wrapper", typ.NewUnion(typ.Number, target)).
		Build()

	if !ContainsEquivalent(root, target) {
		t.Fatalf("expected root to contain equivalent target record")
	}
	if ContainsEquivalent(root, typ.NewRecord().Field("missing", typ.String).Build()) {
		t.Fatalf("unexpected equivalent record found in %v", root)
	}
}

func TestRecursiveUpperBoundCoversLaterSameShapeObservation(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Module")
	body := typ.NewRecord().
		Field("method", typ.Func().
			Param("self", rec).
			Returns(rec).
			Build()).
		Build()
	rec.SetBody(body)
	observation := typ.NewRecord().
		Field("method", typ.Func().
			Param("self", body).
			Returns(body).
			Build()).
		Build()

	if !recursiveUpperBoundCovers(rec, observation) {
		t.Fatalf("recursive upper bound should absorb later observation with same module shape")
	}
}

func TestRecursiveEvidenceCoverBudgetDeclinesConservatively(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Module")
	body := typ.NewRecord().
		Field("method", typ.Func().
			Param("self", rec).
			Returns(rec).
			Build()).
		Build()
	rec.SetBody(body)
	observation := typ.NewRecord().
		Field("method", typ.Func().
			Param("self", body).
			Returns(body).
			Build()).
		Build()

	if !recursiveUpperBoundCovers(rec, observation) {
		t.Fatalf("default recursive cover should admit the same-shape observation")
	}
	cover := recursiveEvidenceCover{
		seen:   make(recursiveCoverSeen),
		budget: 1,
	}
	if cover.covers(rec, observation) {
		t.Fatalf("tiny recursive cover budget must decline instead of admitting without proof")
	}
	if !cover.exhausted {
		t.Fatalf("tiny recursive cover budget should report exhaustion")
	}
}

func TestSelfEmbeddingUpperBound_SameNodeFastPath(t *testing.T) {
	fn := typ.Func().Param("x", typ.Any).Returns(typ.Any).Build()
	got, ok := SelfEmbeddingUpperBound(fn, fn)
	if !ok || got != fn {
		t.Fatalf("same type node should return directly, got %v ok=%v", got, ok)
	}
}

func TestSelfEmbeddingUpperBound_DoesNotNarrowBroadMapObservation(t *testing.T) {
	baseMeta := typ.NewMap(typ.String, typ.Any)
	flow := typ.NewRecursive("SuiteFlow", func(self typ.Type) typ.Type {
		entry := typ.NewRecord().
			Field("id", typ.String).
			OptField("meta", self).
			Field("name", typ.String).
			Build()
		return typ.NewMap(typ.String, typ.NewArray(entry))
	})

	if upper, ok := SelfEmbeddingUpperBound(flow, baseMeta); ok {
		t.Fatalf("recursive product must not be selected as upper bound for broader map: %v", upper)
	}
	if !subtype.IsSubtype(flow, baseMeta) {
		t.Fatalf("recursive suite flow should still be admissible through the broad map bound")
	}
	if subtype.IsSubtype(baseMeta, flow) {
		t.Fatalf("broad map observation is not a suite-flow product")
	}

	recursiveEntry := typ.NewRecord().
		Field("id", typ.String).
		OptField("meta", flow).
		Field("name", typ.String).
		Build()
	broadEntry := typ.NewRecord().
		Field("id", typ.String).
		OptField("meta", baseMeta).
		Field("name", typ.String).
		Build()
	if upper, ok := SelfEmbeddingUpperBound(recursiveEntry, broadEntry); ok {
		t.Fatalf("recursive entry must not be selected as upper bound for broader entry: %v", upper)
	}
	if subtype.IsSubtype(broadEntry, recursiveEntry) {
		t.Fatalf("broad entry observation is not a suite-flow entry")
	}

	recursiveEntries := typ.NewArray(recursiveEntry)
	broadEntries := typ.NewArray(broadEntry)
	if upper, ok := SelfEmbeddingUpperBound(recursiveEntries, broadEntries); ok {
		t.Fatalf("recursive entry array must not be selected as upper bound for broader array: %v", upper)
	}
}

func TestJoinStructuralUnionShape_FoldsCompatibleRecordUnion(t *testing.T) {
	prompt := typ.NewRecord().
		OptField("candidatesTokenCount", typ.Nil).
		Field("promptTokenCount", typ.Integer).
		Build()
	total := typ.NewRecord().
		OptField("candidatesTokenCount", typ.Nil).
		OptField("promptTokenCount", typ.Nil).
		Field("totalTokenCount", typ.Integer).
		Build()
	next := typ.NewRecord().
		Field("candidatesTokenCount", typ.Integer).
		Build()

	got, ok := JoinStructuralUnionShape(typ.NewUnion(prompt, total), next, typ.JoinPreferNonSoft)
	want := typ.NewRecord().
		OptField("candidatesTokenCount", typ.Integer).
		OptField("promptTokenCount", typ.Integer).
		OptField("totalTokenCount", typ.Integer).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("expected compatible record union to fold as %v, got %v ok=%v", want, got, ok)
	}
}

func TestJoinStructuralUnionShape_KeepsDiscriminatedRecordUnion(t *testing.T) {
	dev := typ.NewRecord().Field("role", typ.LiteralString("developer")).Build()
	user := typ.NewRecord().Field("role", typ.LiteralString("user")).Build()
	tool := typ.NewRecord().Field("role", typ.LiteralString("tool")).Build()

	if got, ok := JoinStructuralUnionShape(typ.NewUnion(dev, user), tool, typ.JoinPreferNonSoft); ok {
		t.Fatalf("expected discriminated record union to stay distinct, got %v", got)
	}
}

func TestSameEvidenceFamily_MatchesRecursiveProductFamily(t *testing.T) {
	left := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	right := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})

	if !SameEvidenceFamily(left, right) {
		t.Fatalf("same recursive product family should match")
	}
}

func TestRecursiveEvidenceCoverUsesFamilyMemoForGrowingProduct(t *testing.T) {
	upper := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	observation := typ.Type(upper)
	for i := 0; i < typ.DefaultRecursionDepth+16; i++ {
		observation = typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(observation)).
			Build()
	}

	if !recursiveEvidenceUpperBoundCovers(upper, observation) {
		t.Fatalf("recursive upper bound should cover same-family growing observation")
	}
}

func TestFoldSelfEmbeddingRejectsRootlessOptionalSelf(t *testing.T) {
	anchor := typ.NewRecord().Field("full_path", typ.String).Build()
	if got, ok := FoldSelfEmbedding(anchor, typ.NewOptional(anchor)); ok {
		t.Fatalf("rootless optional self fold = %v, want no recursive product", got)
	}
}

func TestFoldSelfEmbeddingKeepsProductiveRecordRoot(t *testing.T) {
	anchor := typ.NewRecord().Field("full_path", typ.String).Build()
	got, ok := FoldSelfEmbedding(anchor, typ.NewRecord().Field("parent", anchor).Field("full_path", typ.String).Build())
	if !ok {
		t.Fatal("expected productive record self-embedding fold")
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("folded type = %T, want recursive product", got)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok || body.GetField("full_path") == nil || body.GetField("parent") == nil {
		t.Fatalf("recursive body lost productive fields: %v", rec.Body)
	}
}

func TestFoldSelfEmbeddingRejectsNonRecursiveNestedRecord(t *testing.T) {
	// {candidates:[{content:{parts:[{text:string}]}}]} is a finite, non-recursive
	// nested record. Two distinct records on this path share a shallow shape but
	// carry different deep content; the fold must not treat them as one recursive
	// self-embedding anchor.
	leaf := typ.NewRecord().Field("text", typ.String).Build()
	parts := typ.NewArray(leaf)
	content := typ.NewRecord().Field("parts", parts).Build()
	candidate := typ.NewRecord().Field("content", content).Build()
	root := typ.NewRecord().Field("candidates", typ.NewArray(candidate)).Build()

	if got, ok := SelfEmbeddingUpperBound(root, root); !ok || got != root {
		t.Fatalf("non-recursive record self-bound = %v ok=%v, want same node", got, ok)
	}

	for _, anchor := range []typ.Type{leaf, content, candidate, root} {
		if got, ok := FoldSelfEmbedding(anchor, root); ok {
			t.Fatalf("non-recursive nested record folded into recursive placeholder via anchor %v: %v", anchor, got)
		}
	}
	if typ.ContainsRecursive(root) {
		t.Fatalf("non-recursive nested record reported as recursive: %v", root)
	}

	// Self-fold of a finite record whose nested field shares only a shallow field
	// name with an ancestor must not invent a recursive product. The inner
	// {content:{text}} and outer {content:{content:{text}}} are distinct finite
	// records, not a growing self-embedding tower.
	innerContent := typ.NewRecord().Field("content", leaf).Build()
	outerContent := typ.NewRecord().Field("content", innerContent).Build()
	if got, ok := FoldSelfEmbedding(outerContent, outerContent); ok {
		t.Fatalf("shallow field-name collision self-folded into recursive: %v", got)
	}
	if typ.ContainsRecursive(WidenForConvergence(outerContent)) {
		t.Fatalf("widening a finite nested record produced a recursive product: %v", outerContent)
	}
}

func TestFoldSelfEmbeddingFoldsGenuineMuSelfEmbedding(t *testing.T) {
	// mu X.{next:X}: the observation {next:{next:X}} embeds the anchor {next:X}
	// as a genuine back-edge and must still fold so convergence stays bounded.
	anchor := typ.NewRecord().Field("next", typ.String).Build()
	nested := typ.NewRecord().Field("next", typ.String).Build()
	observation := typ.NewRecord().Field("next", nested).Build()

	got, ok := FoldSelfEmbedding(anchor, observation)
	if !ok {
		t.Fatal("genuine self-embedding tower must still fold")
	}
	rec, ok := got.(*typ.Recursive)
	if !ok {
		t.Fatalf("folded type = %T, want recursive product", got)
	}
	if !ContainsRecursiveRef(rec.Body, rec) {
		t.Fatalf("folded body lost recursive back-edge: %v", rec.Body)
	}
}

func TestSameEvidenceFamily_RejectsDiscriminantMismatch(t *testing.T) {
	dev := typ.NewRecord().Field("role", typ.LiteralString("developer")).Build()
	user := typ.NewRecord().Field("role", typ.LiteralString("user")).Build()

	if SameEvidenceFamily(dev, user) {
		t.Fatalf("different literal discriminants should not be the same evidence family")
	}
}

func TestSameEvidenceFamily_MatchesUnionMembersByFamily(t *testing.T) {
	devA := typ.NewRecord().Field("role", typ.LiteralString("developer")).Build()
	userA := typ.NewRecord().Field("role", typ.LiteralString("user")).Build()
	userB := typ.NewRecord().Field("role", typ.LiteralString("user")).Build()
	devB := typ.NewRecord().Field("role", typ.LiteralString("developer")).Build()

	if !SameEvidenceFamily(typ.NewUnion(devA, userA), typ.NewUnion(userB, devB)) {
		t.Fatalf("same union evidence families should match independent of order")
	}
}

func TestRecordSuperset_UsesRecursivePrecisionWithoutSubtypeWalk(t *testing.T) {
	oldSuite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	newSuite := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("full_path", typ.String).
			Build()
	})
	oldRec := typ.NewRecord().
		Field("suite", oldSuite).
		Field("count", typ.Number).
		Build()
	newRec := typ.NewRecord().
		Field("suite", newSuite).
		Field("count", typ.Integer).
		Field("ok", typ.Boolean).
		Build()

	if !RecordSuperset(newRec, oldRec) {
		t.Fatal("record superset should accept recursive and primitive precision")
	}
}

func TestCollapseStructuralUnionShape_FoldsCompatibleRecordMembers(t *testing.T) {
	prompt := typ.NewRecord().
		Field("promptTokenCount", typ.Integer).
		OptField("totalTokenCount", typ.Nil).
		Build()
	total := typ.NewRecord().
		OptField("promptTokenCount", typ.Nil).
		Field("totalTokenCount", typ.Integer).
		Build()

	got := CollapseStructuralUnionShape(typ.NewUnion(prompt, total), typ.JoinPreferNonSoft)
	want := typ.NewRecord().
		OptField("promptTokenCount", typ.Integer).
		OptField("totalTokenCount", typ.Integer).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected compatible record union to collapse as %v, got %v", want, got)
	}
}

func TestCollapseStructuralUnionShape_KeepsDiscriminatedRecordMembers(t *testing.T) {
	dev := typ.NewRecord().Field("role", typ.LiteralString("developer")).Build()
	user := typ.NewRecord().Field("role", typ.LiteralString("user")).Build()
	union := typ.NewUnion(dev, user)

	got := CollapseStructuralUnionShape(union, typ.JoinPreferNonSoft)
	if !typ.TypeEquals(got, union) {
		t.Fatalf("expected discriminated union to remain %v, got %v", union, got)
	}
}

func TestCollapseStructuralUnionShape_NoChangeReturnsOriginalUnion(t *testing.T) {
	event := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()
	union := typ.NewUnion(event, typ.String)

	got := CollapseStructuralUnionShape(union, typ.JoinPreferNonSoft)
	if got != union {
		t.Fatalf("expected unchanged structural union to keep original identity")
	}
}

func TestCollapseStructuralUnionShape_UnchangedRecursiveUnionKeepsIdentity(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("kind", typ.LiteralString("node")).
			Field("next", typ.NewOptional(self)).
			Build()
	})
	leaf := typ.NewRecord().Field("kind", typ.LiteralString("leaf")).Build()
	union := typ.NewUnion(node, leaf)

	got := CollapseStructuralUnionShape(union, typ.JoinPreferNonSoft)
	if got != union {
		t.Fatalf("expected unchanged recursive/discriminated union to keep original identity")
	}
}

func TestRefineStructuralAnnotation_MapValueFromRecordEvidence(t *testing.T) {
	annotation := typ.NewMap(typ.String, typ.Any)
	evidence := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, changed := RefineStructuralAnnotation(annotation, evidence, typ.JoinPreferNonSoft)
	want := typ.NewMap(typ.String, typ.JoinPreferNonSoft(typ.String, typ.Integer))
	if !changed || !typ.TypeEquals(got, want) {
		t.Fatalf("expected refined map annotation %v, got %v changed=%v", want, got, changed)
	}
}

func TestRefineStructuralAnnotation_ArrayElementFromIntersectionEvidence(t *testing.T) {
	id := typ.NewRecord().Field("id", typ.String).Build()
	named := typ.NewRecord().Field("name", typ.String).Build()
	annotation := typ.NewArray(typ.Any)
	evidence := typ.NewIntersection(typ.NewArray(id), typ.NewArray(named))

	got, changed := RefineStructuralAnnotation(annotation, evidence, typ.JoinPreferNonSoft)
	want := typ.NewArray(subtype.NormalizeIntersection(id, named))
	if !changed || !typ.TypeEquals(got, want) {
		t.Fatalf("expected intersection evidence to refine array annotation as %v, got %v changed=%v", want, got, changed)
	}
}

func TestRefineStructuralAnnotation_ArrayElementFromRecursiveEvidence(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	annotation := typ.NewArray(typ.Any)
	evidence := typ.NewRecursive("Entries", func(self typ.Type) typ.Type {
		return typ.NewArray(entry)
	})

	got, changed := RefineStructuralAnnotation(annotation, evidence, typ.JoinPreferNonSoft)
	want := typ.NewArray(entry)
	if !changed || !typ.TypeEquals(got, want) {
		t.Fatalf("expected recursive array evidence to refine annotation as %v, got %v changed=%v", want, got, changed)
	}
}

func TestRefinesFalsyMapKey(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Number)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	ok, changed := RefinesFalsyMapKey(candidate, baseline)
	if !ok || !changed {
		t.Fatalf("expected truthy key refinement, got ok=%v changed=%v", ok, changed)
	}
}

func TestRefinesTableKeyByTruthiness_Map(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Number)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	if !RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("expected map key truthiness refinement")
	}
}

func TestRefinesTableKeyByTruthiness_RecordMapComponent(t *testing.T) {
	candidate := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	baseline := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.NewUnion(typ.String, typ.False), typ.Number).
		Build()

	if !RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("expected record map-key truthiness refinement")
	}
}

func TestRefinesTableKeyByTruthiness_RejectsValueChange(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Integer)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	if RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("value changes are not table-key truthiness refinements")
	}
}

func TestRefinesTableKeyByTruthiness_SplitsNilableUnion(t *testing.T) {
	candidate := typ.NewUnion(typ.Nil, typ.NewMap(typ.String, typ.Number))
	baseline := typ.NewUnion(typ.Nil, typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number))

	if !RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("expected nilable map key truthiness refinement")
	}
}

func TestNestedNilOnlyRegression(t *testing.T) {
	candidate := typ.NewRecord().Field("value", typ.Nil).Build()
	baseline := typ.NewRecord().OptField("value", typ.String).Build()

	if !NestedNilOnlyRegression(candidate, baseline) {
		t.Fatalf("expected nested nil-only regression")
	}
}

func TestContainsNestedStructuralShape(t *testing.T) {
	shape := typ.NewMap(typ.String, typ.Any)
	growing := typ.NewMap(typ.String, typ.NewMap(typ.String, typ.Nil))

	if !ContainsNestedStructuralShape(growing, shape) {
		t.Fatalf("expected nested structural shape")
	}
}

func TestContainsNestedStructuralShape_TupleUsesArityAsShallowShape(t *testing.T) {
	shape := typ.NewTuple(typ.Any)
	growing := typ.NewTuple(typ.NewTuple(typ.Nil))

	if !ShallowStructuralShapeEquals(growing, shape) {
		t.Fatalf("expected tuple shallow shape to compare by arity")
	}
	if !ContainsNestedStructuralShape(growing, shape) {
		t.Fatalf("expected nested tuple structural shape")
	}
}

func TestRefinesSoftContainer_RecordFieldsReplaceDynamicEvidence(t *testing.T) {
	baseline := typ.NewRecord().
		Field("max_tokens", typ.Any).
		Field("output_tokens", typ.Unknown).
		Build()
	candidate := typ.NewRecord().
		Field("max_tokens", typ.LiteralInt(0)).
		Field("output_tokens", typ.Number).
		Build()

	refines, changed := RefinesSoftContainer(candidate, baseline)
	if !refines || !changed {
		t.Fatalf("RefinesSoftContainer() = (%v, %v), want field-level dynamic refinement", refines, changed)
	}
}

func TestRefinesSoftContainer_RecordUnionMembersReplaceDynamicEvidence(t *testing.T) {
	baseline := typ.NewOptional(typ.NewRecord().
		Field("max_tokens", typ.Any).
		Field("output_tokens", typ.Any).
		Build())
	candidate := typ.NewUnion(
		typ.NewRecord().
			Field("max_tokens", typ.Integer).
			Field("output_tokens", typ.Integer).
			Build(),
		typ.NewRecord().
			Field("max_tokens", typ.LiteralInt(0)).
			Field("output_tokens", typ.LiteralInt(0)).
			Build(),
	)

	refines, changed := RefinesSoftContainer(candidate, baseline)
	if !refines || !changed {
		t.Fatalf("RefinesSoftContainer(union) = (%v, %v), want field-level dynamic refinement", refines, changed)
	}
}

func TestRefinesSoftContainer_RecursiveFamilyUsesProductSeenKey(t *testing.T) {
	baseline := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("payload", typ.NewRecord().
				Field("owner", self).
				Field("value", typ.Any).
				Build()).
			Build()
	})
	candidate := typ.NewRecursive("Suite", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("name", typ.String).
			Field("children", typ.NewArray(self)).
			Field("payload", typ.NewRecord().
				Field("owner", self).
				Field("value", typ.String).
				Build()).
			Build()
	})

	refines, changed := RefinesSoftContainer(candidate, baseline)
	if !refines || !changed {
		t.Fatalf("RefinesSoftContainer(recursive) = (%v, %v), want recursive soft-slot refinement", refines, changed)
	}
}
