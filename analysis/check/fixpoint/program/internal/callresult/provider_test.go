package callresult

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestOutcomeProviderReadsSummaryReturnsByCalleeIdentity(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	first := product.Top()
	second := product.Absent(reg)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{first, second}},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want true for matched summary")
	}
	assertCallOutcomeResults(t, reg, got.Results, []product.Value{first, second})
}

func TestOutcomeProviderCachesPreparedExactSummary(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(19)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 20})
	reader := &countingSummaryReader{
		entries: map[summary.SummaryKey]summary.Summary{
			key: {Returns: []product.Value{providerValueForType(reg, typ.String)}},
		},
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries: reader,
		KeyFor:    ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})
	site := factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: callee}).View()

	first := provider(transfer.NodeContext{Registry: reg}, site, state.State{}, nil)
	second := provider(transfer.NodeContext{Registry: reg}, site, state.State{}, nil)

	assertCallOutcomeResultType(t, reg, first.Results, typ.String)
	assertCallOutcomeResultType(t, reg, second.Results, typ.String)
	if reader.reads != 1 {
		t.Fatalf("summary reads = %d, want one prepared-summary read", reader.reads)
	}
}

type countingSummaryReader struct {
	entries map[summary.SummaryKey]summary.Summary
	reads   int
}

func (r *countingSummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	r.reads++
	got, ok := r.entries[key]
	if !ok {
		return summary.Summary{}, false
	}
	return got.Clone(), true
}

func TestSummaryIndexOwnsInputMaps(t *testing.T) {
	reg := standard.Registry()
	calleePath := path.NewPath(symbol.ID(21), "module").Field("run")
	calleeKey := mustCalleePathKey(t, calleePath)
	numberKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 22})
	stringKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 23})
	pathKeys := map[factflow.CalleePathKey]summary.SummaryKey{calleeKey: numberKey}
	functionTypes := map[summary.SummaryKey]*typ.Function{
		numberKey: typ.Func().Returns(typ.Number).Build(),
	}
	index := NewSummaryIndex(SummaryIndexConfig{
		PathKeys:      pathKeys,
		FunctionTypes: functionTypes,
	})
	pathKeys[calleeKey] = stringKey
	functionTypes[numberKey] = typ.Func().Returns(typ.String).Build()

	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{
				Key:     numberKey,
				Summary: summary.Summary{Returns: []product.Value{providerValueForType(reg, typ.Number)}},
			},
			summary.EntrySummary{
				Key:     stringKey,
				Summary: summary.Summary{Returns: []product.Value{providerValueForType(reg, typ.String)}},
			},
		),
		Index: index,
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Number)
}

func TestJoinInstantiatedReturnValuePreservesComputedIdentity(t *testing.T) {
	reg := standard.Registry()
	retID := identity.ID{Kind: "test.return", Site: "provider", Index: 1}
	value := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	value = product.Set(reg, value, identity.Key, identity.Singleton(retID))
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)

	got := joinInstantiatedReturnValue(reg, value, declared, typ.Number)

	id, ok := product.Get(reg, got, identity.Key).ID()
	if !ok || id != retID {
		t.Fatalf("return identity = %v/%v, want %s", id, ok, retID)
	}
}

func TestJoinInstantiatedReturnValueDoesNotWidenComputedVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	msg := typetable.NewRecord().
		Field("kind", typ.LiteralString("msg")).
		Field("value", typ.String).
		Build()
	timer := typetable.NewRecord().
		Field("kind", typ.LiteralString("timer")).
		Field("value", typ.Number).
		Build()
	declaredType := typeexpr.Union(msg, timer)
	value := typevalue.FromType(reg, msg)
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredType), declaredType)

	got := joinInstantiatedReturnValue(reg, value, declared, declaredType)

	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, msg) {
		t.Fatalf("return type = %v/%v, want computed variant %v (not declared %v)", gotType, ok, msg, declaredType)
	}
}

func TestJoinInstantiatedReturnValueUsesDeclaredPresenceAtCallBoundary(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)

	got := joinInstantiatedReturnValue(reg, value, declared, typ.String)

	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("return presence = %s, want declared present contract", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("return type = %v/%v, want declared string", gotType, ok)
	}
}

func TestOutcomeProviderClampsDeclaredAnyReturnMembersAtCallBoundary(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(21)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 22})
	ks := keyspace.New()
	memberKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "last_activity"}})
	if !ok {
		t.Fatal("member suffix key failed")
	}
	rootID := identity.ID{Kind: "table", Site: "declared-any-boundary", Index: 1}
	implType := typetable.NewRecord().Field("last_activity", typ.String).Build()
	declaredType := typetable.NewRecord().Field("last_activity", typ.Any).Build()
	rootValue := product.Set(reg, providerValueForType(reg, implType), identity.Key, identity.Singleton(rootID))
	memberValue := providerValueForType(reg, typ.String)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns:      []product.Value{rootValue},
				HeapKeySpace: ks,
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root:          rootValue,
						StaticMembers: map[keyspace.Key]product.Value{memberKey: memberValue},
					}),
				},
			},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: typ.Func().Returns(declaredType).Build()},
		KeySpace:      ks,
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, declaredType)
	object, ok := got.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("heap objects = %#v, want returned table object", got.HeapTableObjects)
	}
	member, ok := object.StaticMember(memberKey)
	if !ok {
		t.Fatalf("static members = %#v, want last_activity", object.StaticMembers())
	}
	if gotType, ok := typevalue.TypeOf(reg, member); ok && typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("returned heap member type = %v, want declared any boundary to erase implementation string", gotType)
	}
	if gotEvidence := product.Get(reg, member, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("returned heap member evidence = %s, want explicit-top any boundary", gotEvidence)
	}
}

func TestOutcomeProviderRehydratesDeclaredReturnWhenSummarySlotIsTopLike(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(19)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 20})
	channel := typetable.NewRecord().
		Field("receive", typ.Func().Returns(typ.String).Build()).
		Build()
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{product.Top()}},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{
			key: typ.Func().Returns(channel).Build(),
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one declared return", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, channel) {
		t.Fatalf("result type = %v/%v, want %v", gotType, ok, channel)
	}
}

func TestOutcomeProviderRebuildsReturnRecordThroughTablePolicy(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(23)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 24})
	fieldValue := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	indexValue := typevalue.FromType(reg, typeexpr.Optional(typ.Number))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{product.Top()},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathRefinements: []callboundary.PathValueFact{
						{Path: path.NewPlaceholder(0).Field("name"), Value: fieldValue},
						{Path: path.NewPlaceholder(0).IndexInt(1), Value: indexValue},
					},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one reconstructed record return", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok {
		t.Fatalf("result has no type witness: %#v", got.Results[0].Value)
	}
	record, ok := gotType.(*typ.Record)
	if !ok {
		t.Fatalf("result type = %T/%v, want record", gotType, gotType)
	}
	field := record.GetField("name")
	if field == nil {
		t.Fatalf("record fields = %#v, want optional field name", record.Fields)
	}
	if !typ.TypeEquals(field.Type, typ.String) || !field.Optional {
		t.Fatalf("field name = type %v optional=%v, want string optional", field.Type, field.Optional)
	}
	member := record.GetStaticIntIndex(1)
	if member == nil {
		t.Fatalf("record static members = %#v, want optional integer member 1", record.StaticMembers)
	}
	if !typ.TypeEquals(member.Type, typ.Number) || !member.Optional {
		t.Fatalf("static member 1 = type %v optional=%v, want number optional", member.Type, member.Optional)
	}
}

func TestOutcomeProviderMapsSummaryReturnsAndNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(37)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 38})
	ret := product.Absent(reg)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.Absent(reg)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{ret},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathRefinements: []callboundary.PathValueFact{
						{Path: path.NewPlaceholder(0).Field("field"), Value: absent},
					},
					PathStaticMembers: []callboundary.PathStaticMemberFact{
						{Path: path.NewPlaceholder(0).Field("static"), Value: present},
					},
					PathInvalidations: []callboundary.PathInvalidationFact{
						{Path: path.NewPlaceholder(0).Field("invalidated")},
					},
					DynamicIndexFacts: []callboundary.DynamicIndexFact{
						{
							Table: path.NewPlaceholder(0).Field("items"),
							Site:  "summary.dynamic",
							Value: dynamicindex.Fact{
								KeyPresence: presence.Present(),
								KeyValue:    present,
								Value:       absent,
								Admission:   dynamicindex.AdmissionAdmitted,
							},
						},
					},
					BranchProofs: []callboundary.BranchProof{
						{
							Kind:  pathevidence.BranchProofPathEqual,
							Path:  path.NewPlaceholder(0).Field("left"),
							Other: path.NewPlaceholder(1).Field("right"),
						},
					},
					ChannelSelects: []callboundary.ChannelSelectFact{
						{
							Select: channelselectfact.ID("summary.select"),
							Kind:   channelselectfact.FactReceive,
							Result: path.NewPlaceholder(0).Field("result"),
							Case:   path.NewPlaceholder(1).Field("case"),
							Index:  2,
						},
					},
					FrozenTables: []callboundary.FrozenTableFact{
						{Target: path.NewPlaceholder(0).Field("frozen")},
					},
					EffectDeltas: []callboundary.EffectDelta{
						{
							Target: path.NewPlaceholder(0).Field("items"),
							Site:   "summary.effect",
							Kind:   effectdelta.Mutation,
							Value:  effectdelta.Value{Before: present, After: absent, Change: effectdelta.ChangeChanged},
						},
					},
					EscapeEvents: []callboundary.EscapeEventFact{
						{
							Target:    path.NewPlaceholder(0).Field("sent"),
							Kind:      callboundary.EscapeEventSend,
							Recursive: true,
						},
					},
					LifecycleFacts: []callboundary.LifecycleFact{
						{
							Target:   path.NewPlaceholder(0).Field("tx"),
							Kind:     callboundary.LifecycleTransition,
							Protocol: typestate.Protocol("transaction"),
							From:     typestate.State("active"),
							To:       typestate.State("finished"),
						},
					},
					NumFloors: []callboundary.NumFloorFact{
						{Path: path.NewPlaceholder(0).Field("index"), Floor: 1},
					},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{ret})
	if len(got.NormalReturnFacts.PathRefinements) != 1 ||
		!got.NormalReturnFacts.PathRefinements[0].Path.Equal(path.NewPlaceholder(0).Field("field")) ||
		!product.Equal(reg, got.NormalReturnFacts.PathRefinements[0].Value, absent) {
		t.Fatalf("path refinements = %#v, want mapped summary path refinement", got.NormalReturnFacts.PathRefinements)
	}
	if len(got.NormalReturnFacts.PathStaticMembers) != 1 ||
		!got.NormalReturnFacts.PathStaticMembers[0].Path.Equal(path.NewPlaceholder(0).Field("static")) ||
		!product.Equal(reg, got.NormalReturnFacts.PathStaticMembers[0].Value, present) {
		t.Fatalf("path static members = %#v, want mapped summary static member", got.NormalReturnFacts.PathStaticMembers)
	}
	if len(got.NormalReturnFacts.PathInvalidations) != 1 ||
		!got.NormalReturnFacts.PathInvalidations[0].Path.Equal(path.NewPlaceholder(0).Field("invalidated")) {
		t.Fatalf("path invalidations = %#v, want mapped summary invalidation", got.NormalReturnFacts.PathInvalidations)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 ||
		!got.NormalReturnFacts.DynamicIndexFacts[0].Table.Equal(path.NewPlaceholder(0).Field("items")) ||
		got.NormalReturnFacts.DynamicIndexFacts[0].Site != "summary.dynamic" ||
		!presence.Equal(got.NormalReturnFacts.DynamicIndexFacts[0].Value.KeyPresence, presence.Present()) ||
		!product.Equal(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.KeyValue, present) ||
		!product.Equal(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value, absent) ||
		got.NormalReturnFacts.DynamicIndexFacts[0].Value.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic-index facts = %#v, want mapped summary dynamic fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if len(got.NormalReturnFacts.BranchProofs) != 1 ||
		got.NormalReturnFacts.BranchProofs[0].Kind != pathevidence.BranchProofPathEqual ||
		!got.NormalReturnFacts.BranchProofs[0].Path.Equal(path.NewPlaceholder(0).Field("left")) ||
		!got.NormalReturnFacts.BranchProofs[0].Other.Equal(path.NewPlaceholder(1).Field("right")) {
		t.Fatalf("branch proofs = %#v, want mapped summary branch proof", got.NormalReturnFacts.BranchProofs)
	}
	if len(got.NormalReturnFacts.ChannelSelects) != 1 ||
		got.NormalReturnFacts.ChannelSelects[0].Kind != channelselectfact.FactReceive ||
		got.NormalReturnFacts.ChannelSelects[0].Select != channelselectfact.ID("summary.select") ||
		!got.NormalReturnFacts.ChannelSelects[0].Result.Equal(path.NewPlaceholder(0).Field("result")) ||
		!got.NormalReturnFacts.ChannelSelects[0].Case.Equal(path.NewPlaceholder(1).Field("case")) ||
		got.NormalReturnFacts.ChannelSelects[0].Index != 2 {
		t.Fatalf("channel selects = %#v, want mapped summary channel select", got.NormalReturnFacts.ChannelSelects)
	}
	if len(got.NormalReturnFacts.FrozenTables) != 1 ||
		!got.NormalReturnFacts.FrozenTables[0].Target.Equal(path.NewPlaceholder(0).Field("frozen")) {
		t.Fatalf("frozen tables = %#v, want mapped summary frozen-table fact", got.NormalReturnFacts.FrozenTables)
	}
	if len(got.NormalReturnFacts.EffectDeltas) != 1 ||
		got.NormalReturnFacts.EffectDeltas[0].Kind != effectdelta.Mutation ||
		got.NormalReturnFacts.EffectDeltas[0].Site != "summary.effect" ||
		!got.NormalReturnFacts.EffectDeltas[0].Target.Equal(path.NewPlaceholder(0).Field("items")) ||
		!product.Equal(reg, got.NormalReturnFacts.EffectDeltas[0].Value.Before, present) ||
		!product.Equal(reg, got.NormalReturnFacts.EffectDeltas[0].Value.After, absent) ||
		got.NormalReturnFacts.EffectDeltas[0].Value.Change != effectdelta.ChangeChanged {
		t.Fatalf("effect deltas = %#v, want mapped summary effect delta", got.NormalReturnFacts.EffectDeltas)
	}
	if len(got.NormalReturnFacts.EscapeEvents) != 1 ||
		!got.NormalReturnFacts.EscapeEvents[0].Target.Equal(path.NewPlaceholder(0).Field("sent")) ||
		got.NormalReturnFacts.EscapeEvents[0].Kind != callboundary.EscapeEventSend ||
		!got.NormalReturnFacts.EscapeEvents[0].Recursive {
		t.Fatalf("escape events = %#v, want mapped summary escape event", got.NormalReturnFacts.EscapeEvents)
	}
	if len(got.NormalReturnFacts.LifecycleFacts) != 1 ||
		!got.NormalReturnFacts.LifecycleFacts[0].Target.Equal(path.NewPlaceholder(0).Field("tx")) ||
		got.NormalReturnFacts.LifecycleFacts[0].Kind != callboundary.LifecycleTransition ||
		got.NormalReturnFacts.LifecycleFacts[0].Protocol != typestate.Protocol("transaction") {
		t.Fatalf("lifecycle facts = %#v, want mapped summary lifecycle fact", got.NormalReturnFacts.LifecycleFacts)
	}
	if len(got.NormalReturnFacts.NumFloors) != 1 ||
		!got.NormalReturnFacts.NumFloors[0].Path.Equal(path.NewPlaceholder(0).Field("index")) ||
		got.NormalReturnFacts.NumFloors[0].Floor != 1 {
		t.Fatalf("num floors = %#v, want mapped summary numeric floor", got.NormalReturnFacts.NumFloors)
	}
}

func TestParamDirectMutationExposureFromPathInvalidation(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	wide := typetable.NewRecord().Field("x", typeexpr.Union(typ.Number, typ.String)).Build()
	got := summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: path.NewPlaceholder(0).Field("x")},
				{Path: path.NewPlaceholder(1).Field("x")},
				{Path: path.NewPlaceholder(0)},
			},
		},
	}
	fn := typ.Func().Param("w", wide).Param("unpassed", wide).Build()

	exposures := paramDirectMutationExposures(reg, typeValues, 1, got, fn)

	if len(exposures) != 1 {
		t.Fatalf("exposures = %#v, want one direct mutation exposure", exposures)
	}
	if !exposures[0].Source.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("source = %s, want $0", exposures[0].Source.String())
	}
	if exposures[0].Kind != factflow.CovariantExposureRecord {
		t.Fatalf("kind = %v, want record exposure", exposures[0].Kind)
	}
	contract, ok := typevalue.TypeOf(reg, exposures[0].Contract)
	if !ok || !typ.TypeEquals(contract, wide) {
		t.Fatalf("contract = %v ok=%v, want %s", contract, ok, wide.String())
	}
}

func TestOutcomeProviderCarriesNestedSummaryEscapeEvents(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(39)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 40})
	nested := providerNestedTableType()
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{providerValueForType(reg, nested)},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					EscapeEvents: []callboundary.EscapeEventFact{
						{
							Target: path.NewPlaceholder(0),
							Kind:   callboundary.EscapeEventSend,
						},
						{
							Target:    path.NewPlaceholder(0).Field("child"),
							Kind:      callboundary.EscapeEventStore,
							Recursive: true,
						},
					},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one nested-table return", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, nested) {
		t.Fatalf("result type = %v/%v, want %v", gotType, ok, nested)
	}
	if len(got.NormalReturnFacts.EscapeEvents) != 2 {
		t.Fatalf("escape events = %#v, want root and child facts", got.NormalReturnFacts.EscapeEvents)
	}
	if got.NormalReturnFacts.EscapeEvents[0].Kind != callboundary.EscapeEventSend ||
		!got.NormalReturnFacts.EscapeEvents[0].Target.Equal(path.NewPlaceholder(0)) ||
		got.NormalReturnFacts.EscapeEvents[0].Recursive {
		t.Fatalf("root escape event = %#v, want non-recursive send on $0", got.NormalReturnFacts.EscapeEvents[0])
	}
	if got.NormalReturnFacts.EscapeEvents[1].Kind != callboundary.EscapeEventStore ||
		!got.NormalReturnFacts.EscapeEvents[1].Target.Equal(path.NewPlaceholder(0).Field("child")) ||
		!got.NormalReturnFacts.EscapeEvents[1].Recursive {
		t.Fatalf("child escape event = %#v, want recursive store on $0.child", got.NormalReturnFacts.EscapeEvents[1])
	}
}

func TestOutcomeProviderCopiesSummaryHeapTableObjects(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(41)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 42})
	tableID := identity.ID{Kind: "table", Site: "provider", Index: 1}
	ks := keyspace.New()
	memberKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "field"}})
	if !ok {
		t.Fatal("member suffix key failed")
	}
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          product.Absent(reg),
		StaticMembers: map[keyspace.Key]product.Value{memberKey: product.Top()},
	})
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				HeapKeySpace: ks,
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					tableID: object,
				},
			},
		}),
		KeyFor:   ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		KeySpace: ks,
	})

	first := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)
	projected, ok := first.HeapTableObjects[tableID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want %v", first.HeapTableObjects, tableID)
	}
	if !product.Equal(reg, projected.Root(), product.Absent(reg)) {
		t.Fatalf("projected heap root = %#v, want absent", projected.Root())
	}

	mutatedMembers := projected.StaticMembers()
	mutatedMembers[memberKey] = product.Absent(reg)
	first.HeapTableObjects[tableID] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          product.Top(),
		StaticMembers: mutatedMembers,
	})

	second := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)
	again, ok := second.HeapTableObjects[tableID]
	if !ok || !product.Equal(reg, again.Root(), product.Absent(reg)) {
		t.Fatalf("provider exposed mutable heap object state: %#v/%v", again, ok)
	}
}

func TestOutcomeProviderPreservesReturnedNestedHeapIdentityClosure(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(43)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 44})
	rootID := identity.ID{Kind: "table", Site: "provider-closure", Index: 1}
	childID := identity.ID{Kind: "table", Site: "provider-closure", Index: 2}
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(childID))
	ks := keyspace.New()
	childKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("child suffix key failed")
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns:      []product.Value{rootValue},
				HeapKeySpace: ks,
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root:          rootValue,
						StaticMembers: map[keyspace.Key]product.Value{childKey: childValue},
					}),
					childID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root: childValue,
					}),
				},
			},
		}),
		KeyFor:   ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		KeySpace: ks,
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{rootValue})
	rootObject, ok := got.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want root object %v", got.HeapTableObjects, rootID)
	}
	if member, ok := rootObject.StaticMember(childKey); !ok || !product.Equal(reg, member, childValue) {
		t.Fatalf("root child member = %#v/%v, want %#v", member, ok, childValue)
	}
	childObject, ok := got.HeapTableObjects[childID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want child object %v", got.HeapTableObjects, childID)
	}
	if !product.Equal(reg, childObject.Root(), childValue) {
		t.Fatalf("child object root = %#v, want %#v", childObject.Root(), childValue)
	}
}

func TestOutcomeProviderMaterializesReturnParamPathAliases(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(45)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 46})
	rootID := identity.ID{Kind: "table", Site: "provider-alias", Index: 1}
	registryID := identity.ID{Kind: "table", Site: "provider-alias", Index: 2}
	clientID := identity.ID{Kind: "table", Site: "provider-alias", Index: 3}
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(rootID))
	registryValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(registryID))
	clientValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(clientID))
	source := providerExpressionSource(t, factflow.ExprRef(45), 0)
	ks := keyspace.New()
	apiKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "api"}})
	if !ok {
		t.Fatal("api suffix key failed")
	}
	apiBackupKey, ok := ks.FromRootlessSuffix([]segment.Segment{
		{Kind: segment.SegmentField, Name: "api"},
		{Kind: segment.SegmentField, Name: "backup"},
	})
	if !ok {
		t.Fatal("api.backup suffix key failed")
	}
	backupKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "backup"}})
	if !ok {
		t.Fatal("backup suffix key failed")
	}
	aliasSource, ok := pathaddr.PlaceholderKeyFromPathKey(path.PathKey("$0.backup"))
	if !ok {
		t.Fatal("alias source key failed")
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns:      []product.Value{rootValue},
				HeapKeySpace: ks,
				HeapTableObjects: map[identity.ID]heapidentity.TableObject{
					rootID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root:          rootValue,
						StaticMembers: map[keyspace.Key]product.Value{apiKey: registryValue},
					}),
					registryID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
						Root: registryValue,
					}),
				},
				ReturnParamPathAliases: []summary.ReturnParamPathAlias{
					{ReturnIndex: 0, Member: ".api.backup", Source: aliasSource},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				source.ExprRef: registryValue,
			},
		}),
		KeySpace: ks,
	})
	in := state.State{}.WriteHeapTableObject(reg, registryID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          registryValue,
		StaticMembers: map[keyspace.Key]product.Value{backupKey: clientValue},
	}))

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol:    callee,
		ArgumentSources: []factflow.ValueSource{source},
	}).View(), in, nil)

	rootObject, ok := got.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want root object %v", got.HeapTableObjects, rootID)
	}
	if member, ok := rootObject.StaticMember(apiBackupKey); !ok || !product.Equal(reg, member, clientValue) {
		t.Fatalf("root api.backup member = %#v/%v, want %#v", member, ok, clientValue)
	}
	registryObject, ok := got.HeapTableObjects[registryID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want registry object %v", got.HeapTableObjects, registryID)
	}
	if member, ok := registryObject.StaticMember(backupKey); !ok || !product.Equal(reg, member, clientValue) {
		t.Fatalf("registry backup member = %#v/%v, want %#v", member, ok, clientValue)
	}

	again := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol:    callee,
		ArgumentSources: []factflow.ValueSource{source},
	}).View(), state.State{}, nil)
	againRoot, ok := again.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("second HeapTableObjects = %#v, want root object %v", again.HeapTableObjects, rootID)
	}
	if member, ok := againRoot.StaticMember(apiBackupKey); ok {
		t.Fatalf("provider materialization mutated summary snapshot: api.backup = %#v", member)
	}
}

func TestOutcomeProviderUsesOwnedSummaryReadWithoutMutatingStoredSummary(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(125)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 126})
	memberFact := callboundary.PathStaticMemberFact{
		Path:  path.NewPlaceholder(0).Field("name"),
		Value: providerValueForType(reg, typ.String),
	}
	reader := &countingOwnedSummaryReader{
		snapshot: summary.NewSnapshotOwnedNormalized(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.NormalizeOwned(reg, summary.Summary{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathStaticMembers: []callboundary.PathStaticMemberFact{memberFact},
				},
			}),
		}),
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries: reader,
		KeyFor:    ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)

	wantType := typetable.NewRecord().Field("name", typ.String).Build()
	assertCallOutcomeResultType(t, reg, got.Results, wantType)
	if reader.ownedReads == 0 || reader.publicReads != 0 {
		t.Fatalf("summary reads = owned:%d public:%d, want owned-only", reader.ownedReads, reader.publicReads)
	}
	stored, ok := reader.snapshot.ReadOwnedNormalized(key)
	if !ok {
		t.Fatal("stored summary missing after provider call")
	}
	if len(stored.Returns) != 0 {
		t.Fatalf("provider mutated owned summary returns = %#v, want none", stored.Returns)
	}
	if len(stored.NormalReturnFacts.PathStaticMembers) != 1 ||
		!stored.NormalReturnFacts.PathStaticMembers[0].Path.Equal(memberFact.Path) {
		t.Fatalf("stored normal-return facts = %#v, want original member fact", stored.NormalReturnFacts)
	}
}

func TestOutcomeProviderJoinMaterializesTableEntryShapeFromNilableFactRecord(t *testing.T) {
	reg := standard.Registry()
	calleePath := path.NewPath(symbol.ID(127), "module").Field("make")
	leftKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 128})
	rightKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 129})
	optionalString := typ.MaterializeOptional(typ.String)
	optionalInteger := typ.MaterializeOptional(typ.Integer)
	facts := callboundary.NormalReturnFacts{
		PathRefinements: []callboundary.PathValueFact{
			{Path: path.NewPlaceholder(0).Field("value"), Value: providerValueForType(reg, optionalString)},
		},
		PathStaticMembers: []callboundary.PathStaticMemberFact{
			{Path: path.NewPlaceholder(0).IndexInt(1), Value: providerValueForType(reg, optionalInteger)},
		},
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{
				Key:     leftKey,
				Summary: summary.Summary{NormalReturnFacts: facts},
			},
			summary.EntrySummary{
				Key:     rightKey,
				Summary: summary.Summary{NormalReturnFacts: facts},
			},
		),
		PathMultiKeys: map[factflow.CalleePathKey][]summary.SummaryKey{
			mustCalleePathKey(t, calleePath): {leftKey, rightKey},
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(),

		state.State{}, nil)

	if len(got.Results) != 1 || got.Results[0].Index != 0 {
		t.Fatalf("results = %#v, want synthesized return slot 0", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok {
		t.Fatalf("result value has no type witness: %#v", got.Results[0].Value)
	}
	record, ok := gotType.(*typ.Record)
	if !ok {
		t.Fatalf("result type = %T %[1]v, want record", gotType)
	}
	if record.HasMapComponent() {
		t.Fatalf("result record map component = [%v]: %v, want none", record.MapKey, record.MapValue)
	}
	field := record.GetField("value")
	if field == nil || !field.Optional || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("field value = %#v, want optional string payload", field)
	}
	member := record.GetStaticIntIndex(1)
	if member == nil || !member.Optional || !typ.TypeEquals(member.Type, typ.Integer) {
		t.Fatalf("static member [1] = %#v, want optional integer payload", member)
	}
}

func TestOutcomeProviderJoinDropsDescendantEscapeEventsBelowMaybeAbsentReturn(t *testing.T) {
	reg := standard.Registry()
	calleePath := path.NewPath(symbol.ID(131), "module").Field("maybe")
	leftKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 132})
	rightKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 133})
	returnValue := providerValueForType(reg, providerNestedTableType())
	rootEvent := callboundary.EscapeEventFact{
		Target: path.NewPlaceholder(0),
		Kind:   callboundary.EscapeEventSend,
	}
	childEvent := callboundary.EscapeEventFact{
		Target:    path.NewPlaceholder(0).Field("child"),
		Kind:      callboundary.EscapeEventStore,
		Recursive: true,
	}
	rootRelation := callboundary.StoreRelationFact{
		Source: path.NewPlaceholder(0),
		Into:   path.NewPlaceholder(1),
	}
	childRelation := callboundary.StoreRelationFact{
		Source: path.NewPlaceholder(0).Field("child"),
		Into:   path.NewPlaceholder(1),
	}
	rootFloor := callboundary.NumFloorFact{
		Path:  path.NewPlaceholder(0),
		Floor: 1,
	}
	childFloor := callboundary.NumFloorFact{
		Path:  path.NewPlaceholder(0).Field("child"),
		Floor: 1,
	}
	rootConstraint := callboundary.RelConstraintFact{
		CoA: 1,
		A:   callboundary.RelOperand{Path: path.NewPlaceholder(0)},
		C:   callboundary.RelOperand{Path: path.NewPlaceholder(1)},
	}
	childConstraint := callboundary.RelConstraintFact{
		CoA: 1,
		A:   callboundary.RelOperand{Path: path.NewPlaceholder(0).Field("child")},
		C:   callboundary.RelOperand{Path: path.NewPlaceholder(1)},
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{
				Key: leftKey,
				Summary: summary.Summary{
					Returns: []product.Value{returnValue},
					NormalReturnFacts: callboundary.NormalReturnFacts{
						EscapeEvents:   []callboundary.EscapeEventFact{rootEvent, childEvent},
						StoreRelations: []callboundary.StoreRelationFact{rootRelation, childRelation},
						NumFloors:      []callboundary.NumFloorFact{rootFloor, childFloor},
						RelConstraints: []callboundary.RelConstraintFact{rootConstraint, childConstraint},
					},
				},
			},
			summary.EntrySummary{
				Key: rightKey,
				Summary: summary.Summary{
					Returns: []product.Value{product.Absent(reg)},
					NormalReturnFacts: callboundary.NormalReturnFacts{
						StoreRelations: []callboundary.StoreRelationFact{rootRelation, childRelation},
						NumFloors:      []callboundary.NumFloorFact{rootFloor, childFloor},
						RelConstraints: []callboundary.RelConstraintFact{rootConstraint, childConstraint},
					},
				},
			},
		),
		PathMultiKeys: map[factflow.CalleePathKey][]summary.SummaryKey{
			mustCalleePathKey(t, calleePath): {leftKey, rightKey},
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(),

		state.State{}, nil)

	if len(got.Results) != 1 || product.DefinitelyPresent(got.Results[0].Value) {
		t.Fatalf("results = %#v, want maybe-absent joined return", got.Results)
	}
	if len(got.NormalReturnFacts.EscapeEvents) != 1 ||
		!got.NormalReturnFacts.EscapeEvents[0].Target.Equal(rootEvent.Target) ||
		got.NormalReturnFacts.EscapeEvents[0].Kind != rootEvent.Kind ||
		got.NormalReturnFacts.EscapeEvents[0].Recursive != rootEvent.Recursive {
		t.Fatalf("escape events = %#v, want only root event", got.NormalReturnFacts.EscapeEvents)
	}
	if len(got.NormalReturnFacts.StoreRelations) != 1 ||
		!got.NormalReturnFacts.StoreRelations[0].Source.Equal(rootRelation.Source) ||
		!got.NormalReturnFacts.StoreRelations[0].Into.Equal(rootRelation.Into) {
		t.Fatalf("store relations = %#v, want only root relation", got.NormalReturnFacts.StoreRelations)
	}
	if len(got.NormalReturnFacts.NumFloors) != 1 ||
		!got.NormalReturnFacts.NumFloors[0].Path.Equal(rootFloor.Path) ||
		got.NormalReturnFacts.NumFloors[0].Floor != rootFloor.Floor {
		t.Fatalf("numeric floors = %#v, want only root floor", got.NormalReturnFacts.NumFloors)
	}
	if len(got.NormalReturnFacts.RelConstraints) != 1 ||
		!got.NormalReturnFacts.RelConstraints[0].A.Path.Equal(rootConstraint.A.Path) ||
		!got.NormalReturnFacts.RelConstraints[0].C.Path.Equal(rootConstraint.C.Path) {
		t.Fatalf("rel constraints = %#v, want only root constraint", got.NormalReturnFacts.RelConstraints)
	}
}

func TestOutcomeProviderMapsNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(137)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 138})
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				NormalReturnParams: []product.Value{
					present,
					product.Top(),
					product.Bottom(reg),
				},
				NormalReturnParamConditions: []summary.ParamCondition{
					summary.ParamConditionTruthy,
					summary.ParamConditionFalsy,
					summary.ParamConditionTop,
				},
				NormalReturnParamEqualities: []summary.ParamEquality{
					{Left: 0, Right: 1},
				},
				ReturnConditionParamRefinements: []summary.ReturnConditionParamRefinement{
					{
						ReturnIndex: 0,
						ReturnValue: true,
						Target:      path.NewPlaceholder(0).Field("value"),
						Value:       present,
					},
				},
				ReturnPresenceRelations: []summary.ReturnPresenceRelation{
					{
						TriggerIndex:    1,
						TriggerPresence: presence.Present(),
						TargetIndex:     0,
						TargetPresence:  presence.Absent(),
					},
				},
				ReturnConditionSlotRefinements: []summary.ReturnConditionSlotRefinement{
					{
						ReturnIndex: 0,
						ReturnValue: false,
						TargetIndex: 1,
						Value:       present,
					},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 ||
		!got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) ||
		!product.Equal(reg, got.ParamPathRefinements[0].Value, present) {
		t.Fatalf("param path refinements = %#v, want only useful normal-return param", got.ParamPathRefinements)
	}
	if len(got.ParamConditions) != 2 ||
		got.ParamConditions[0] != (callpayload.CallParamCondition{ParamIndex: 0, Value: true}) ||
		got.ParamConditions[1] != (callpayload.CallParamCondition{ParamIndex: 1, Value: false}) {
		t.Fatalf("param conditions = %#v, want truthy/falsy conditions only", got.ParamConditions)
	}
	if len(got.ParamPathRelations) != 1 ||
		got.ParamPathRelations[0].Kind != callpayload.CallPathRelationEqual ||
		!got.ParamPathRelations[0].Left.Equal(path.NewPlaceholder(0)) ||
		!got.ParamPathRelations[0].Right.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("param path relations = %#v, want mapped equality", got.ParamPathRelations)
	}
	if len(got.ReturnConditionRefinements) != 1 ||
		got.ReturnConditionRefinements[0].ReturnIndex != 0 ||
		!got.ReturnConditionRefinements[0].ReturnValue ||
		!got.ReturnConditionRefinements[0].Target.Equal(path.NewPlaceholder(0).Field("value")) ||
		!product.Equal(reg, got.ReturnConditionRefinements[0].Value, present) {
		t.Fatalf("return condition refinements = %#v, want mapped refinement", got.ReturnConditionRefinements)
	}
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v, want mapped relation", got.ReturnPresenceRelations)
	}
	if len(got.ReturnConditionSlots) != 1 ||
		got.ReturnConditionSlots[0].ReturnIndex != 0 ||
		got.ReturnConditionSlots[0].ReturnValue ||
		got.ReturnConditionSlots[0].TargetIndex != 1 ||
		!product.Equal(reg, got.ReturnConditionSlots[0].Value, present) {
		t.Fatalf("return condition slots = %#v, want mapped slot refinement", got.ReturnConditionSlots)
	}
}

func TestOutcomeProviderMapsParamObligationsDistinctFromNormalReturnRefinements(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(147)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 148})
	obligation := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				ParamObligations: []product.Value{obligation},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(),

		state.State{}, nil)

	if len(got.ParamObligations) != 1 ||
		got.ParamObligations[0].ParamIndex != 0 ||
		!product.Equal(reg, got.ParamObligations[0].Value, obligation) {
		t.Fatalf("param obligations = %#v, want mapped summary obligation", got.ParamObligations)
	}
	if len(got.ParamPathRefinements) != 0 {
		t.Fatalf("param path refinements = %#v, want none for pre-call obligation", got.ParamPathRefinements)
	}
}

func TestOutcomeFromSummaryNormalReturnAnyPresenceDoesNotTaintCallerType(t *testing.T) {
	reg := standard.Registry()
	anyPresent := product.WithPresence(reg, typevalue.FromType(reg, typ.Any), presence.Present())
	anyPresent = product.Set(reg, anyPresent, assertion.Key, assertion.Any())

	got := outcomeFromSummary(reg, summary.Summary{
		NormalReturnParams: []product.Value{anyPresent},
	}, 0, nil, func(int) bool { return true })

	if len(got.ParamPathRefinements) != 1 {
		t.Fatalf("param path refinements = %#v, want one presence refinement", got.ParamPathRefinements)
	}
	value := got.ParamPathRefinements[0].Value
	if !presence.Equal(product.PresenceOf(value), presence.Present()) {
		t.Fatalf("presence = %s, want present", product.PresenceOf(value))
	}
	if claim := product.Get(reg, value, assertion.Key); !claim.IsTop() {
		t.Fatalf("assertion claim = %s, want top/no any claim", claim)
	}
	if tpe, ok := typevalue.TypeOf(reg, value); ok && typ.IsAny(tpe) {
		t.Fatalf("refinement type = %s, want presence-only without any type", tpe)
	}
}

func TestOutcomeProviderAddsFunctionTypeParamObligations(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(157)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 158})
	fn := typ.Func().
		Param("url", typ.String).
		Param("options", typetable.NewMap(typ.Any, typ.Any)).
		Returns(typ.Any).
		Build()
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}).View(),

		state.State{}, nil)

	if len(got.ParamObligations) != 2 ||
		got.ParamObligations[0].ParamIndex != 0 ||
		got.ParamObligations[1].ParamIndex != 1 {
		t.Fatalf("param obligations = %#v, want explicit concrete params", got.ParamObligations)
	}
	if gotType, ok := typevalue.TypeOf(reg, got.ParamObligations[0].Value); !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("param obligation type = %v/%v, want string", gotType, ok)
	}
	if len(got.ParamPathRefinements) != 0 {
		t.Fatalf("param path refinements = %#v, want none for function-type pre-call obligation", got.ParamPathRefinements)
	}
}

func TestOutcomeProviderSpecializesGenericReturnFromArgument(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(217)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 218})
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("value", fnParam).
		Returns(typ.Instantiate(result, fnParam)).
		Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParam))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{rawReturn},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathRefinements: []callboundary.PathValueFact{
						{Path: path.NewPlaceholder(0).Field("ok"), Value: present},
					},
				},
			},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, profile),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
		},
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, profile))
	if len(got.NormalReturnFacts.PathRefinements) != 1 ||
		!got.NormalReturnFacts.PathRefinements[0].Path.Equal(path.NewPlaceholder(0).Field("ok")) ||
		!product.Equal(reg, got.NormalReturnFacts.PathRefinements[0].Value, present) {
		t.Fatalf("path refinements = %#v, want preserved summary fact", got.NormalReturnFacts.PathRefinements)
	}
}

func TestOutcomeProviderPreparedSummaryCacheKeepsGenericCallsContextual(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(219)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 220})
	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("value", fnParam).
		Returns(fnParam).
		Build()
	reader := &countingSummaryReader{
		entries: map[summary.SummaryKey]summary.Summary{
			key: {Returns: []product.Value{providerValueForType(reg, fnParam)}},
		},
	}
	provider := OutcomeProvider(ProviderConfig{
		Summaries:     reader,
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.Number),
			2: providerValueForType(reg, typ.String),
		}),
	})

	numberCall := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
		},
	}).View(), state.State{}, nil)
	stringCall := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 2, 0),
		},
	}).View(), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, numberCall.Results, typ.Number)
	assertCallOutcomeResultType(t, reg, stringCall.Results, typ.String)
	if reader.reads != 1 {
		t.Fatalf("summary reads = %d, want one cached base summary read", reader.reads)
	}
}

func TestOutcomeProviderSpecializesGenericTupleReturnAcrossSlots(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(227)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 228})
	fnParamA := typ.NewTypeParam("A", nil)
	fnParamB := typ.NewTypeParam("B", nil)
	fn := typ.Func().
		TypeParamRef(fnParamA).
		TypeParamRef(fnParamB).
		Param("a", fnParamA).
		Param("b", fnParamB).
		Returns(typ.NewTuple(fnParamA, fnParamB)).
		Build()
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{Returns: []product.Value{
				providerValueForType(reg, fnParamA),
				providerValueForType(reg, fnParamB),
			}},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.LiteralInt(42)),
			2: providerValueForType(reg, typ.LiteralString("hello")),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResultsTypes(t, reg, got.Results, []typ.Type{
		typ.LiteralInt(42),
		typ.LiteralString("hello"),
	})
}

func TestOutcomeProviderSpecializesGenericReturnFromCallbackReturn(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(237)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 238})
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(fnParamU).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()
	callback := typ.Func().Param("item", profile).Returns(typ.String).Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParamU))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{rawReturn}},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.Instantiate(result, profile)),
			2: providerValueForType(reg, callback),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, typ.String))
}

func TestOutcomeProviderSpecializesGenericReturnFromSolvedCallbackSummary(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(247)
	callbackSym := symbol.ID(248)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 249})
	callbackKey := summary.DefaultSummaryKey(ref.FromSymbol(callbackSym))
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(fnParamU).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()
	callback := typ.Func().Param("item", profile).Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParamU))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{
				Key:     key,
				Summary: summary.Summary{Returns: []product.Value{rawReturn}},
			},
			summary.EntrySummary{
				Key:     callbackKey,
				Summary: summary.Summary{Returns: []product.Value{providerValueForType(reg, typ.Number)}},
			},
		),
		KeyFor:       ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		Facts:        factflow.NewFacts(factflow.FactsInput{ExpressionFunctions: map[factflow.ExprRef]symbol.ID{2: callbackSym}}),
		FunctionKeys: map[symbol.ID]summary.SummaryKey{callbackSym: callbackKey},
		FunctionTypes: map[summary.SummaryKey]*typ.Function{
			key:         fn,
			callbackKey: callback,
		},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.Instantiate(result, profile)),
			2: providerValueForType(reg, callback),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, typ.Number))
}

func TestOutcomeProviderSpecializesGenericReturnFromCallbackResultReturn(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(257)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 258})
	result := providerResultGeneric()
	profile := providerProfileType()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(typ.Instantiate(result, fnParamU)).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()
	callback := typ.Func().Param("item", profile).Returns(typ.Instantiate(result, typ.Number)).Build()
	rawReturn := providerValueForType(reg, typ.Instantiate(result, fnParamU))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{rawReturn}},
		}),
		KeyFor:        ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		FunctionTypes: map[summary.SummaryKey]*typ.Function{key: fn},
		Sources: providerSourceValues(reg, map[factflow.ExprRef]product.Value{
			1: providerValueForType(reg, typ.Instantiate(result, profile)),
			2: providerValueForType(reg, callback),
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
		ArgumentSources: []factflow.ValueSource{
			providerExpressionSource(t, 1, 0),
			providerExpressionSource(t, 2, 1),
		},
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Instantiate(result, typ.Number))
}

func TestOutcomeProviderMissingSummaryYieldsZeroOutcomeAndEmptySummaryHasAuthority(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(17)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 18})
	missingKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 19})
	snap := summary.NewSnapshot(reg, summary.EntrySummary{
		Key:     key,
		Summary: summary.Summary{Returns: []product.Value{product.Top()}},
	})
	site := factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: callee})
	ctx := transfer.NodeContext{Registry: reg}

	tests := []struct {
		name     string
		provider callpayload.CallOutcomeProvider
	}{
		{
			name:     "nil reader",
			provider: OutcomeProvider(ProviderConfig{KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key})}),
		},
		{
			name:     "nil key func",
			provider: OutcomeProvider(ProviderConfig{Summaries: snap}),
		},
		{
			name:     "missing key",
			provider: OutcomeProvider(ProviderConfig{Summaries: snap, KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{})}),
		},
		{
			name:     "missing summary",
			provider: OutcomeProvider(ProviderConfig{Summaries: snap, KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: missingKey})}),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertEmptyOutcome(t, tc.provider(ctx, site.View(), state.State{}, nil))
		})
	}

	emptySummaryProvider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: summary.Summary{Returns: nil}}),
		KeyFor:    ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})
	if got := emptySummaryProvider(ctx, site.View(), state.State{}, nil); got.PostReturnAuthority || len(got.Results) != 0 || got.HasPostReturnEvidence() {
		t.Fatalf("empty matched summary outcome = %#v, want no authority or evidence", got)
	}
}

func TestOutcomeProviderWeakMatchedSummaryIsNotPostReturnAuthority(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(754)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 754})
	weak := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     key,
			Summary: summary.Summary{Returns: []product.Value{weak}},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)

	if got.PostReturnAuthority {
		t.Fatalf("weak matched summary PostReturnAuthority = true, want false")
	}
	assertCallOutcomeResults(t, reg, got.Results, []product.Value{weak})
}

func TestOutcomeProviderRefinesMatchedSummaryResultFromCurrentCallableType(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(755)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 755})
	optionalNumber := product.WithPresence(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number), presence.Maybe())
	callableType := typ.Func().Returns(typ.Number).Build()
	callableValue := typevalue.WithWitness(reg, typevalue.FromType(reg, callableType), callableType)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key: key,
			Summary: summary.Summary{
				Returns: []product.Value{optionalNumber},
				ReturnConditionSlotRefinements: []summary.ReturnConditionSlotRefinement{
					{ReturnIndex: 0, ReturnValue: true, TargetIndex: 0, Value: optionalNumber},
				},
			},
		}),
		KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			return callableValue, true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: callee,
	}).View(), state.State{}, nil)

	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if len(got.Results) != 1 || !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("matched summary result = %#v type %v/%v, want exact number", got.Results, gotType, ok)
	}
	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want summary return-condition authority preserved")
	}
}

func TestOutcomeProviderUnresolvedFunctionFallbackIsNotPostReturnAuthority(t *testing.T) {
	reg := standard.Registry()
	target := symbol.ID(753)
	functionValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg),
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			return functionValue, true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "result")),
		},
	}).View(),

		state.State{}, nil)

	if got.PostReturnAuthority {
		t.Fatalf("unresolved function fallback PostReturnAuthority = true, want false")
	}
	if len(got.Results) != 1 || got.Results[0].Index != 0 {
		t.Fatalf("fallback results = %#v, want one unknown result slot", got.Results)
	}
}

func TestByCalleeIdentitySymbolKeyMapsAreCloned(t *testing.T) {
	callee := symbol.ID(21)
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 23})

	symbolMap := map[symbol.ID]summary.SummaryKey{callee: symbolKey}
	keyFor := ByCalleeIdentity(symbolMap)
	symbolMap[callee] = summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 25})

	got, ok := keyFor(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: callee}).View())
	if !ok || got != symbolKey {
		t.Fatalf("symbol key = %v, %v; want %v, true", got, ok, symbolKey)
	}
}

func TestOutcomeProviderUsesCurrentCalleeValueIdentityForPathSummary(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	symbolKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 31})
	identityKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 32})
	calleeSymbol := symbol.ID(33)
	calleePath := path.NewPath(symbol.ID(34), "module").Field("call")
	fnID := identity.LuaFunction(35)
	calleeSlot := path.PathKey("sym34@1.call")
	ret := product.Absent(reg)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{Key: symbolKey, Summary: summary.Summary{Returns: []product.Value{product.Top()}}},
			summary.EntrySummary{Key: identityKey, Summary: summary.Summary{Returns: []product.Value{ret}}},
		),
		KeyFor:      ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{calleeSymbol: symbolKey}),
		FunctionIDs: map[identity.ID]summary.SummaryKey{fnID: identityKey},
		CalleeValue: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
			if site.CalleePath().IsEmpty() {
				return product.Value{}, false
			}
			return in.ReadPathKey(ctx.Registry, ks, calleeSlot), true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: calleeSymbol,
		CalleePath:   calleePath,
	}).View(),

		state.State{}, nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{product.Top()})

	identityValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	identityValue = product.Set(reg, identityValue, identity.Key, identity.Singleton(fnID))
	got = provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(),

		state.State{}.WritePathKey(reg, ks, calleeSlot, identityValue), nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{ret})

	otherValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	otherValue = product.Set(reg, otherValue, identity.Key, identity.Singleton(identity.LuaFunction(36)))
	got = provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(),

		state.State{}.WritePathKey(reg, ks, calleeSlot, otherValue), nil)

	if len(got.Results) != 0 {
		t.Fatalf("different path identity results = %#v, want none", got.Results)
	}

	got = provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(),

		state.State{}, nil)

	if len(got.Results) != 0 {
		t.Fatalf("missing path identity results = %#v, want none", got.Results)
	}
}

func TestOutcomeProviderFallsBackToCurrentIdentityWhenContextKeyMissing(t *testing.T) {
	reg := standard.Registry()
	missingContextKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 41})
	identityKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 42})
	fnID := identity.LuaFunction(43)
	ret := product.Absent(reg)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     identityKey,
			Summary: summary.Summary{Returns: []product.Value{ret}},
		}),
		ContextKeyFor: func(transfer.NodeContext, factflow.CallSiteView) (summary.SummaryKey, bool) {
			return missingContextKey, true
		},
		FunctionIDs: map[identity.ID]summary.SummaryKey{fnID: identityKey},
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
			value = product.Set(reg, value, identity.Key, identity.Singleton(fnID))
			value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
			return value, true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		ExprRef:      1,
		HasExpr:      true,
		CalleeSymbol: symbol.ID(44),
	}).View(), state.State{}, nil)

	assertCallOutcomeResults(t, reg, got.Results, []product.Value{ret})
}

func TestOutcomeProviderContextSummaryInheritsSameIdentityNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	identityKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 46})
	contextKey := identityKey
	contextKey.Entry.Facts = 1
	fnID := identity.LuaFunction(47)
	captured := path.NewPath(symbol.ID(48), "captured")
	write := providerValueForType(reg, typ.Func().Build())
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{Key: contextKey, Summary: summary.Summary{}},
			summary.EntrySummary{
				Key: identityKey,
				Summary: summary.Summary{
					NormalReturnFacts: callboundary.NormalReturnFacts{
						PersistentPathWrites: []callboundary.PathValueFact{{Path: captured, Value: write}},
					},
				},
			},
		),
		ContextKeyFor: func(transfer.NodeContext, factflow.CallSiteView) (summary.SummaryKey, bool) {
			return contextKey, true
		},
		FunctionIDs: map[identity.ID]summary.SummaryKey{fnID: identityKey},
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
			value = product.Set(reg, value, identity.Key, identity.Singleton(fnID))
			value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
			return value, true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		ExprRef: 1,
		HasExpr: true,
	}).View(), state.State{}, nil)

	if len(got.NormalReturnFacts.PersistentPathWrites) != 1 ||
		!got.NormalReturnFacts.PersistentPathWrites[0].Path.Equal(captured) ||
		!product.Equal(reg, got.NormalReturnFacts.PersistentPathWrites[0].Value, write) {
		t.Fatalf("persistent writes = %#v, want inherited same-identity write", got.NormalReturnFacts.PersistentPathWrites)
	}
}

func TestOutcomeProviderCurrentIdentityRejectsStaleContextKey(t *testing.T) {
	reg := standard.Registry()
	contextKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 14501})
	identityKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 14502})
	fnID := identity.LuaFunction(14503)
	contextRet := providerValueForType(reg, typ.String)
	identityRet := providerValueForType(reg, typ.Integer)
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg,
			summary.EntrySummary{
				Key:     contextKey,
				Summary: summary.Summary{Returns: []product.Value{contextRet}},
			},
			summary.EntrySummary{
				Key:     identityKey,
				Summary: summary.Summary{Returns: []product.Value{identityRet}},
			},
		),
		ContextKeyFor: func(transfer.NodeContext, factflow.CallSiteView) (summary.SummaryKey, bool) {
			return contextKey, true
		},
		FunctionIDs: map[identity.ID]summary.SummaryKey{fnID: identityKey},
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
			value = product.Set(reg, value, identity.Key, identity.Singleton(fnID))
			value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))
			return value, true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		ExprRef: 1,
		HasExpr: true,
	}).View(), state.State{}, nil)

	assertCallOutcomeResultType(t, reg, got.Results, typ.Integer)
}

func TestOutcomeProviderRejectsStalePathSummaryForCurrentNonFunctionCallee(t *testing.T) {
	reg := standard.Registry()
	staleKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 39})
	calleePath := path.NewPath(symbol.ID(40), "module").Field("call")
	staleRet := product.Absent(reg)
	currentNumber := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	currentNumber = product.Set(reg, currentNumber, runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	provider := OutcomeProvider(ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
			Key:     staleKey,
			Summary: summary.Summary{Returns: []product.Value{staleRet}},
		}),
		PathKeys: map[factflow.CalleePathKey]summary.SummaryKey{
			mustCalleePathKey(t, calleePath): staleKey,
		},
		CalleeValue: func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
			if site.CalleePath().IsEmpty() {
				return product.Value{}, false
			}
			return currentNumber, true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleePath: calleePath,
	}).View(),

		state.State{}, nil)

	if got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = true, want false for stale non-function callee path")
	}
	if len(got.Results) != 0 {
		t.Fatalf("stale non-function callee results = %#v, want none", got.Results)
	}
}

func TestOutcomeProviderIntegratesWithFactflowCallRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	calleeKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 26})
	callValue := product.Top()
	existingTargetValue := product.Absent(reg)
	target := symbol.ID(27)

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), existingTargetValue),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				CallSites: map[cfg.Point]factflow.CallSite{
					call: factflow.NewCallSite(factflow.CallSiteConfig{
						Context:      factflow.CallSiteContextAssignmentSource,
						CalleeSymbol: symbol.ID(28),
						ResultTargets: []factflow.CallResultTarget{
							factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "x")),
						},
					}),
				},
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, path.NewPath(target, "x"), factflow.ValueSource{
						Kind:         factflow.ValueSourceCall,
						CallPoint:    call,
						HasCallPoint: true,
						ResultIndex:  0,
					}),
				},
			}),
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: OutcomeProvider(ProviderConfig{
				Summaries: summary.NewSnapshot(reg, summary.EntrySummary{
					Key:     calleeKey,
					Summary: summary.Summary{Returns: []product.Value{callValue}},
				}),
				KeyFor: ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{symbol.ID(28): calleeKey}),
			}),
		}),
	})

	assertValue(t, reg, got[call], key.SymbolValue(target), existingTargetValue)
	assertValue(t, reg, got[graph.Exit()], key.SymbolValue(target), callValue)
}

func TestProductionImportsAreBounded(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list imports . error = %v", err)
	}
	allowed := map[string]bool{
		"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/memberaccess": true,
		"github.com/wippyai/go-lua/analysis/check/fixpoint/summary":                       true,
		"github.com/wippyai/go-lua/analysis/domain/path":                                  true,
		"github.com/wippyai/go-lua/analysis/domain/path/address":                          true,
		"github.com/wippyai/go-lua/analysis/domain/path/keyspace":                         true,
		"github.com/wippyai/go-lua/analysis/domain/path/segment":                          true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis":                            true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence":                   true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/identity":                   true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/presence":                   true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind":                true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness":                true,
		"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin":              true,
		"github.com/wippyai/go-lua/analysis/domain/value/product":                         true,
		"github.com/wippyai/go-lua/analysis/domain/value/proof":                           true,
		"github.com/wippyai/go-lua/analysis/domain/value/refinement":                      true,
		"github.com/wippyai/go-lua/analysis/domain/value/typevalue":                       true,
		"github.com/wippyai/go-lua/analysis/domain/value/variant":                         true,
		"github.com/wippyai/go-lua/analysis/engine/callboundary":                          true,
		"github.com/wippyai/go-lua/analysis/engine/calloutcome":                           true,
		"github.com/wippyai/go-lua/analysis/engine/callpayload":                           true,
		"github.com/wippyai/go-lua/analysis/engine/factflow":                              true,
		"github.com/wippyai/go-lua/analysis/engine/factquery":                             true,
		"github.com/wippyai/go-lua/analysis/engine/sourcevalue":                           true,
		"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact":               true,
		"github.com/wippyai/go-lua/analysis/engine/state/heapidentity":                    true,
		"github.com/wippyai/go-lua/analysis/engine/state/pathevidence":                    true,
		"github.com/wippyai/go-lua/analysis/engine/state":                                 true,
		"github.com/wippyai/go-lua/analysis/engine/transfer":                              true,
		"github.com/wippyai/go-lua/analysis/internal/mapedit":                             true,
		"github.com/wippyai/go-lua/analysis/ir/dominance":                                 true,
		"github.com/wippyai/go-lua/analysis/ir/cfg":                                       true,
		"github.com/wippyai/go-lua/analysis/type/typecall":                                true,
		"github.com/wippyai/go-lua/analysis/lua/typeprojection":                           true,
		"github.com/wippyai/go-lua/analysis/symbol":                                       true,
		"github.com/wippyai/go-lua/analysis/type/normalize":                               true,
		"github.com/wippyai/go-lua/analysis/type/refinement":                              true,
		"github.com/wippyai/go-lua/analysis/type/table":                                   true,
		"github.com/wippyai/go-lua/analysis/type/typ":                                     true,
	}
	for _, imp := range strings.Fields(string(out)) {
		if !allowed[imp] {
			t.Fatalf("unexpected production import %q", imp)
		}
	}
}

func TestFactapplyDoesNotImportSummary(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", "../../../../../engine/factapply").Output()
	if err != nil {
		t.Fatalf("go list imports factapply error = %v", err)
	}
	for _, imp := range strings.Fields(string(out)) {
		if imp == "github.com/wippyai/go-lua/analysis/check/fixpoint/summary" {
			t.Fatalf("factapply imports summary")
		}
	}
}

func providerResultGeneric() *typ.Generic {
	param := typ.NewTypeParam("T", nil)
	okRecord := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", param).
		Build()
	errRecord := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	return typ.NewGeneric("Result", []*typ.TypeParam{param}, typeexpr.Union(okRecord, errRecord))
}

func providerProfileType() typ.Type {
	return typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()
}

func providerNestedTableType() typ.Type {
	return typetable.NewRecord().
		Field("child", typetable.NewRecord().
			Field("leaf", typ.String).
			Build()).
		Build()
}

func providerValueForType(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}

type countingOwnedSummaryReader struct {
	snapshot    summary.Snapshot
	publicReads int
	ownedReads  int
}

func (r *countingOwnedSummaryReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	r.publicReads++
	return r.snapshot.Read(key)
}

func (r *countingOwnedSummaryReader) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	r.ownedReads++
	return r.snapshot.ReadOwnedNormalized(key)
}

func providerSourceValues(reg *axis.Registry, values map[factflow.ExprRef]product.Value) sourcevalue.SourceValues {
	return sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry:         reg,
		ExpressionValues: values,
	})
}

func providerExpressionSource(t *testing.T, ref factflow.ExprRef, index int) factflow.ValueSource {
	t.Helper()
	shape, ok := factflow.NewValueSourceShape(false, false, false, false)
	if !ok {
		t.Fatal("expression source shape invalid")
	}
	source, ok := factflow.NewExpressionValueSource(ref, index, index, factflow.NoValueSourceIndex, shape)
	if !ok {
		t.Fatalf("expression source for ref %d invalid", ref)
	}
	return source
}

func mustCalleePathKey(t *testing.T, p path.Path) factflow.CalleePathKey {
	t.Helper()
	key, ok := factflow.CalleePathKeyFromPath(p)
	if !ok {
		t.Fatalf("CalleePathKeyFromPath(%s) failed", p.String())
	}
	return key
}

func assertCallOutcomeResultType(t *testing.T, reg *axis.Registry, got []callpayload.CallResult, want typ.Type) {
	t.Helper()
	if len(got) != 1 || got[0].Index != 0 {
		t.Fatalf("results = %#v, want one result at index 0", got)
	}
	gotType, ok := typeFromValue(reg, got[0].Value)
	if !ok {
		t.Fatalf("result value has no structural type: %#v", got[0].Value)
	}
	gotType = subst.ExpandInstantiated(gotType)
	want = subst.ExpandInstantiated(want)
	if !typ.TypeEquals(gotType, want) {
		t.Fatalf("result type = %v, want %v", gotType, want)
	}
}

func assertCallOutcomeResultsTypes(t *testing.T, reg *axis.Registry, got []callpayload.CallResult, want []typ.Type) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %#v", len(got), len(want), got)
	}
	for i, wantType := range want {
		if got[i].Index != i {
			t.Fatalf("result %d index = %d, want %d", i, got[i].Index, i)
		}
		gotType, ok := typeFromValue(reg, got[i].Value)
		if !ok {
			t.Fatalf("result %d value has no structural type: %#v", i, got[i].Value)
		}
		gotType = subst.ExpandInstantiated(gotType)
		wantType = subst.ExpandInstantiated(wantType)
		if !typ.TypeEquals(gotType, wantType) {
			t.Fatalf("result %d type = %v, want %v", i, gotType, wantType)
		}
	}
}

func assertCallOutcomeResults(t *testing.T, reg *axis.Registry, got []callpayload.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %#v", len(got), len(want), got)
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("result %d index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("result %d value = %#v, want %#v", i, got[i].Value, value)
		}
	}
}

func assertEmptyOutcome(t *testing.T, got callpayload.CallOutcome) {
	t.Helper()
	if !got.Empty() {
		t.Fatalf("provider returned non-empty outcome: %#v", got)
	}
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	got := st.ReadValue(reg, slot)
	if !product.Equal(reg, got, want) {
		t.Fatalf("state[%v] = %#v, want %#v", slot, got, want)
	}
}
