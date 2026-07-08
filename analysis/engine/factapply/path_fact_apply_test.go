package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFactsNodeTransferKeepsStaticMemberWritesDistinctFromPathAssignments(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(401)
	target := symbol.ID(401)
	targetPath := pathdom.NewPath(target, "table").Field("field")
	targetKey := pathdom.PathKey("sym401@1.field")
	assignmentSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(401), HasExpr: true}
	staticSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(402), HasExpr: true}
	assigned := presentValue(reg)
	proofValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			assignmentSource: assigned,
			staticSource:     proofValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	assignedState := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, assignmentSource),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, ks, assignedState, targetKey, assigned)
	if got, ok := assignedState.ReadPathStaticMember(ks, targetKey); ok {
		t.Fatalf("path assignment wrote static-member proof %s, want none", formatValue(reg, got))
	}

	staticState := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(targetPath, staticSource),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, ks, staticState, targetKey, product.Bottom(reg))
	gotProof, ok := staticState.ReadPathStaticMember(ks, targetKey)
	if !ok || !product.Equal(reg, gotProof, proofValue) {
		t.Fatalf("static-member proof = %s/%v, want %s/true", formatValue(reg, gotProof), ok, formatValue(reg, proofValue))
	}
}

func TestFactsNodeTransferKeepsSamePointStaticMemberWriteWithPathAssignment(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4011)
	target := symbol.ID(4011)
	targetPath := pathdom.NewPath(target, "provider").IndexInt(1)
	targetKey := pathdom.PathKey("sym4011@1[1]")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4011), HasExpr: true}
	value := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: value},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "provider")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(targetPath, source),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	assertPathValue(t, reg, ks, got, targetKey, value)
	gotProof, ok := got.ReadPathStaticMember(ks, targetKey)
	if !ok || !product.Equal(reg, gotProof, value) {
		t.Fatalf("same-point static-member proof = %s/%v, want %s/true", formatValue(reg, gotProof), ok, formatValue(reg, value))
	}
}

func TestFactsNodeTransferStaticMemberWriteUpdatesHeapTableIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4012)
	target := symbol.ID(4012)
	targetPath := pathdom.NewPath(target, "table").Field("result")
	tableID := identity.ID{Kind: "test.table", Site: "static-member-write", Index: 1}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4012), HasExpr: true}
	value := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{valueSource: value},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(targetPath, valueSource),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(target), tableValue).
		WriteHeapTableObject(reg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableValue})))

	memberKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("result").Segments)
	if !ok {
		t.Fatal("missing result suffix key")
	}
	member, ok := got.ReadHeapTableObject(reg, tableID).StaticMember(memberKey)
	if !ok || !product.Equal(reg, member, value) {
		t.Fatalf("heap static member = %s/%v, want %s/true", formatValue(reg, member), ok, formatValue(reg, value))
	}
}

func TestFactsNodeTransferStaticMemberWriteRefinesContainersPresent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4013)
	target := symbol.ID(4013)
	targetPath := pathdom.NewPath(target, "state").Field("nested").Field("handler")
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4013), HasExpr: true}
	value := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{valueSource: value},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "state")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathStaticMemberWrites: map[cfg.Point]factflow.PathStaticMemberWrite{
				point: factflow.NewPathStaticMemberWrite(targetPath, valueSource),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top()))

	root := got.ReadValue(reg, key.SymbolValue(target))
	if gotPresence := product.PresenceOf(root); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("root presence = %s in %s, want present", gotPresence, formatValue(reg, root))
	}
	assertPathPresence(t, reg, ks, got, pathdom.PathKey("sym4013@1.nested"), presence.Present())
}

func TestFactsNodeTransferAppliesDynamicIndexWriteKeyValueAdmission(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(402)
	table := symbol.ID(402)
	tablePath := pathdom.NewPath(table, "table").Field("items")
	tableKey := pathdom.PathKey("sym402@1.items")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(403), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(404), HasExpr: true}
	keyValue := presentValue(reg)
	writeValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   keyValue,
			valueSource: writeValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	gotFact := got.ReadDynamicIndexFact(reg, dynamicindex.Key{Table: mustStateKey(t, resolver.KeySpace(), tableKey), Site: dynamicindex.SiteForPoint(int(point))})
	if !presence.Equal(gotFact.KeyPresence, presence.Present()) ||
		!product.Equal(reg, gotFact.KeyValue, keyValue) ||
		!product.Equal(reg, gotFact.Value, writeValue) ||
		gotFact.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic-index fact = %#v, want key/value/admitted mapping", gotFact)
	}
	if len(sources.calls) != 2 || sources.calls[0].source != keySource || sources.calls[1].source != valueSource {
		t.Fatalf("dynamic-index source calls = %#v, want key then value", sources.calls)
	}
}

func TestFactsNodeTransferDynamicIndexWriteProvesKeyMembership(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(40201)
	table := symbol.ID(40201)
	keySymbol := symbol.ID(40202)
	tablePath := pathdom.NewPath(table, "suites")
	keyPath := pathdom.NewPath(keySymbol, "suite")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(40201), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(40202), HasExpr: true}
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
			valueSource: presentValue(reg),
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "suites")
	visibilityBuilder.Define(point, keySymbol, "suite")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	tableStateKey, tableOK := visibility.RootOrVisibleStateKeyAt(resolver, point, tablePath)
	keyStateKey, keyOK := resolver.StateKeyAt(point, keyPath)
	if !tableOK || !keyOK {
		t.Fatal("missing state keys")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				keySource.ExprRef: keyPath,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if !got.HasPathKeyMembership(keyStateKey, tableStateKey) {
		t.Fatalf("key memberships = %#v, want %s known as key of %s", got.KeyMembershipsSnapshot(), keyStateKey, tableStateKey)
	}
}

func TestFactsNodeTransferNilDynamicIndexWriteDoesNotProveKeyMembership(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(402011)
	table := symbol.ID(402011)
	keySymbol := symbol.ID(402012)
	tablePath := pathdom.NewPath(table, "registered")
	keyPath := pathdom.NewPath(keySymbol, "id")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(402011), HasExpr: true}
	nilSource := factflow.NewNilValueSource(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "registered")
	visibilityBuilder.Define(point, keySymbol, "id")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	tableStateKey, tableOK := resolver.StateKeyAt(point, tablePath)
	keyStateKey, keyOK := resolver.StateKeyAt(point, keyPath)
	if !tableOK || !keyOK {
		t.Fatal("missing state keys")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					nilSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				keySource.ExprRef: keyPath,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{
				keySource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
				nilSource: typevalue.Nil(reg),
			},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if got.HasPathKeyMembership(keyStateKey, tableStateKey) {
		t.Fatalf("key memberships = %#v, want nil write not to prove %s present in %s", got.KeyMembershipsSnapshot(), keyStateKey, tableStateKey)
	}
}

func TestFactsNodeTransferKnownNilDynamicIndexWritePublishesStaticMember(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(402012)
	table := symbol.ID(402013)
	keySymbol := symbol.ID(402014)
	tablePath := pathdom.NewPath(table, "box")
	keyPath := pathdom.NewPath(keySymbol, "key")
	memberPath := tablePath.IndexStr("value")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(402014), HasExpr: true}
	nilSource := factflow.NewNilValueSource(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "box")
	visibilityBuilder.Define(point, keySymbol, "key")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	memberKey := resolver.KeyAt(point, memberPath)
	if memberKey == "" {
		t.Fatal("missing member key")
	}
	tableStateKey, tableOK := resolver.StateKeyAt(point, tablePath)
	keyStateKey, keyOK := resolver.StateKeyAt(point, keyPath)
	if !tableOK || !keyOK {
		t.Fatal("missing state keys")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					nilSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				keySource.ExprRef: keyPath,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{
				keySource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("value")), typ.LiteralString("value")),
				nilSource: typevalue.Nil(reg),
			},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	gotMember, ok := got.ReadPathStaticMember(resolver.KeySpace(), memberKey)
	if !ok || !product.Equal(reg, gotMember, typevalue.Nil(reg)) {
		t.Fatalf("static member = %v/%v, want exact nil for known dynamic write", gotMember, ok)
	}
	if got.HasPathKeyMembership(keyStateKey, tableStateKey) {
		t.Fatalf("key memberships = %#v, want nil write not to prove key present", got.KeyMembershipsSnapshot())
	}
}

func TestFactsNodeTransferDynamicIndexReadCarriesAllValueKeyMembership(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(40202)
	target := symbol.ID(41202)
	ids := symbol.ID(41203)
	registered := symbol.ID(41204)
	keySym := symbol.ID(41205)
	targetPath := pathdom.NewPath(target, "id")
	idsPath := pathdom.NewPath(ids, "ids")
	registeredPath := pathdom.NewPath(registered, "registered")
	sourceExpr := factflow.ExprRef(41202)
	keyExpr := factflow.ExprRef(41205)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "id")
	visibilityBuilder.Define(point, ids, "ids")
	visibilityBuilder.Define(point, registered, "registered")
	visibilityBuilder.Define(point, keySym, "key")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	idsStateKey, ok := resolver.StateKeyAt(point, idsPath)
	if !ok {
		t.Fatal("missing ids state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatal("missing registered state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		t.Fatal("missing target state key")
	}
	idsKey, ok := resolver.KeySpace().InternStateKey(idsStateKey)
	if !ok {
		t.Fatal("missing ids key")
	}
	dyn, ok := factflow.NewDynamicIndexExpression(idsPath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source),
			},
			DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
				sourceExpr: dyn,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{source: presentValue(reg)},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.AddDynamicIndexAllValuesKeyMembership(idsKey, registeredStateKey))

	if !got.HasPathKeyMembership(targetStateKey, registeredStateKey) {
		t.Fatalf("key memberships = %#v, want all-value invariant to prove %s key of %s", got.KeyMembershipsSnapshot(), targetStateKey, registeredStateKey)
	}
}

func TestFactsNodeTransferDynamicIndexReadCarriesValueKeyMembership(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(40203)
	target := symbol.ID(40203)
	channelToID := symbol.ID(40204)
	registered := symbol.ID(40205)
	resultChannel := symbol.ID(40206)
	targetPath := pathdom.NewPath(target, "channel_id")
	channelToIDPath := pathdom.NewPath(channelToID, "channel_to_id")
	registeredPath := pathdom.NewPath(registered, "registered_channels")
	sourceExpr := factflow.ExprRef(40203)
	keyExpr := factflow.ExprRef(40206)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	value := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			source: value,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "channel_id")
	visibilityBuilder.Define(point, channelToID, "channel_to_id")
	visibilityBuilder.Define(point, registered, "registered_channels")
	visibilityBuilder.Define(point, resultChannel, "result_channel")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	channelToIDStateKey, ok := resolver.StateKeyAt(point, channelToIDPath)
	if !ok {
		t.Fatalf("missing channel_to_id state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatalf("missing registered_channels state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		t.Fatalf("missing target state key")
	}
	channelToIDKey, ok := resolver.KeySpace().InternStateKey(channelToIDStateKey)
	if !ok {
		t.Fatalf("missing channel_to_id key")
	}
	site := dynamicindex.Site("actor.register_channel")
	in := state.State{}.
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: channelToIDKey, Site: site}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			Value:     value,
			HasValue:  true,
			Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddDynamicIndexValueKeyMembership(channelToIDKey, site, registeredStateKey)
	dyn, ok := factflow.NewDynamicIndexExpression(channelToIDPath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source),
			},
			DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
				sourceExpr: dyn,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	if !got.HasPathKeyMembership(targetStateKey, registeredStateKey) {
		t.Fatalf("key memberships = %#v, want %s from channel_to_id read known as key of %s", got.KeyMembershipsSnapshot(), targetStateKey, registeredStateKey)
	}
}

func TestFactsNodeTransferDynamicIndexReadCarriesValueKeyMembershipWithoutConcreteReadValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(402031)
	target := symbol.ID(402031)
	channelToID := symbol.ID(402041)
	registered := symbol.ID(402051)
	resultChannel := symbol.ID(402061)
	targetPath := pathdom.NewPath(target, "channel_id")
	channelToIDPath := pathdom.NewPath(channelToID, "channel_to_id")
	registeredPath := pathdom.NewPath(registered, "registered_channels")
	resultChannelPath := pathdom.NewPath(resultChannel, "result_channel")
	sourceExpr := factflow.ExprRef(402031)
	keyExpr := factflow.ExprRef(402061)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	value := presentValue(reg)

	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "channel_id")
	visibilityBuilder.Define(point, channelToID, "channel_to_id")
	visibilityBuilder.Define(point, registered, "registered_channels")
	visibilityBuilder.Define(point, resultChannel, "result_channel")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	channelToIDStateKey, ok := resolver.StateKeyAt(point, channelToIDPath)
	if !ok {
		t.Fatalf("missing channel_to_id state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatalf("missing registered_channels state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		t.Fatalf("missing target state key")
	}
	resultChannelStateKey, ok := resolver.StateKeyAt(point, resultChannelPath)
	if !ok {
		t.Fatalf("missing result_channel state key")
	}
	channelToIDKey, ok := resolver.KeySpace().InternStateKey(channelToIDStateKey)
	if !ok {
		t.Fatalf("missing channel_to_id key")
	}
	site := dynamicindex.Site("actor.register_channel")
	in := state.State{}.
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: channelToIDKey, Site: site}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			Value:     value,
			HasValue:  true,
			Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddDynamicIndexValueKeyMembership(channelToIDKey, site, registeredStateKey)
	dyn, ok := factflow.NewDynamicIndexExpression(channelToIDPath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source),
			},
			DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
				sourceExpr: dyn,
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				keyExpr: resultChannelPath,
			},
		}),
		Sources:    &recordingSourceValues{},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	if !got.HasPathKeyMembership(targetStateKey, registeredStateKey) {
		t.Fatalf("key memberships = %#v, want %s from channel_to_id read known as key of %s even when the read value is unknown", got.KeyMembershipsSnapshot(), targetStateKey, registeredStateKey)
	}
	origins := got.DynamicIndexReadOriginsForValue(targetStateKey)
	foundOrigin := false
	for _, origin := range origins {
		if origin.Container == channelToIDKey && origin.Key == resultChannelStateKey {
			foundOrigin = true
			break
		}
	}
	if !foundOrigin {
		t.Fatalf("dynamic read origins = %#v, want channel_to_id[result_channel] origin", origins)
	}
}

func TestDynamicIndexValueMembershipFromRootlessPathUsesVisibleOrRootOrVisibleSource(t *testing.T) {
	point := cfg.Point(402031)
	valueSym := symbol.ID(402032)
	containerSym := symbol.ID(402033)
	tableSym := symbol.ID(402034)

	builder := visibility.NewBuilder()
	builder.Define(point, valueSym, "value")
	builder.Define(point, containerSym, "ids")
	builder.Define(point, tableSym, "registered")
	resolver := visibility.NewResolver(builder.Build())

	valuePath := pathdom.Path{Symbol: valueSym}
	containerKey, ok := visibility.AddressAt(resolver, point, pathdom.Path{Symbol: containerSym}).RootOrVisibleKeyspaceKey()
	if !ok {
		t.Fatal("container key missing")
	}
	tableKey, ok := visibility.AddressAt(resolver, point, pathdom.Path{Symbol: tableSym}).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("table key missing")
	}
	valueRootKey, ok := visibility.AddressAt(resolver, point, valuePath).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("value root-or-visible key missing")
	}
	valueVisibleKey, ok := visibility.AddressAt(resolver, point, valuePath).VisibleStateKey()
	if !ok {
		t.Fatal("value visible key missing")
	}

	site := dynamicindex.Site("signature.table_mutator:0:-1")

	for _, tc := range []struct {
		name     string
		valueKey pathaddr.StateKey
	}{
		{name: "visible", valueKey: valueVisibleKey},
		{name: "root-or-visible", valueKey: valueRootKey},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := state.State{}.AddPathKeyMembership(tc.valueKey, tableKey)

			got := addDynamicIndexValueKeyMembershipsFromPath(
				transfer.NodeContext{Point: point, Registry: standard.Registry()},
				resolver,
				in,
				valuePath,
				containerKey,
				site,
			)
			tables := got.DynamicIndexValueKeyMembershipTables(containerKey, site)
			if len(tables) != 1 || tables[0] != tableKey {
				t.Fatalf("dynamic value key memberships = %#v, want table %s", tables, tableKey)
			}
		})
	}
}

func TestFactsNodeTransferDynamicIndexReadRequiresCommonValueKeyMembership(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(40204)
	target := symbol.ID(40207)
	ids := symbol.ID(40208)
	registered := symbol.ID(40209)
	keySym := symbol.ID(40210)
	targetPath := pathdom.NewPath(target, "id")
	idsPath := pathdom.NewPath(ids, "ids")
	registeredPath := pathdom.NewPath(registered, "registered")
	sourceExpr := factflow.ExprRef(40207)
	keyExpr := factflow.ExprRef(40210)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceExpr, HasExpr: true}
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	value := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "id")
	visibilityBuilder.Define(point, ids, "ids")
	visibilityBuilder.Define(point, registered, "registered")
	visibilityBuilder.Define(point, keySym, "key")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	idsStateKey, ok := resolver.StateKeyAt(point, idsPath)
	if !ok {
		t.Fatalf("missing ids state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatalf("missing registered state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		t.Fatalf("missing target state key")
	}
	idsKey, ok := resolver.KeySpace().InternStateKey(idsStateKey)
	if !ok {
		t.Fatalf("missing ids key")
	}
	pairedSite := dynamicindex.Site("paired")
	unpairedSite := dynamicindex.Site("unpaired")
	in := state.State{}.
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: idsKey, Site: pairedSite}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			Value:     value,
			HasValue:  true,
			Admission: dynamicindex.AdmissionAdmitted,
		})).
		AddDynamicIndexValueKeyMembership(idsKey, pairedSite, registeredStateKey).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: idsKey, Site: unpairedSite}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			Value:     value,
			HasValue:  true,
			Admission: dynamicindex.AdmissionAdmitted,
		}))
	dyn, ok := factflow.NewDynamicIndexExpression(idsPath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source),
			},
			DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
				sourceExpr: dyn,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{source: value},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, in)

	if got.HasPathKeyMembership(targetStateKey, registeredStateKey) {
		t.Fatalf("key memberships = %#v, want no proof when another value-producing site lacks membership", got.KeyMembershipsSnapshot())
	}
}

func TestFactsNodeTransferDynamicIndexWritePreservesAllValueMembershipOnlyForSafeWrites(t *testing.T) {
	reg := standard.Registry()
	table := symbol.ID(42201)
	valueSym := symbol.ID(42202)
	registered := symbol.ID(42203)
	tablePath := pathdom.NewPath(table, "ids")
	valuePath := pathdom.NewPath(valueSym, "id")
	registeredPath := pathdom.NewPath(registered, "registered")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(42201), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(42202), HasExpr: true}
	nilSource := factflow.NewNilValueSource(0)

	for _, tc := range []struct {
		name             string
		point            cfg.Point
		source           factflow.ValueSource
		values           map[factflow.ValueSource]product.Value
		sourcePath       bool
		sourceMembership bool
		wantInvariant    bool
	}{
		{
			name:             "proven value preserves",
			point:            cfg.Point(42201),
			source:           valueSource,
			values:           map[factflow.ValueSource]product.Value{keySource: presentValue(reg), valueSource: presentValue(reg)},
			sourcePath:       true,
			sourceMembership: true,
			wantInvariant:    true,
		},
		{
			name:          "unknown present value clears",
			point:         cfg.Point(42202),
			source:        valueSource,
			values:        map[factflow.ValueSource]product.Value{keySource: presentValue(reg), valueSource: presentValue(reg)},
			sourcePath:    true,
			wantInvariant: false,
		},
		{
			name:          "nil delete preserves",
			point:         cfg.Point(42203),
			source:        nilSource,
			values:        map[factflow.ValueSource]product.Value{keySource: presentValue(reg), nilSource: typevalue.Nil(reg)},
			wantInvariant: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			visibilityBuilder := visibility.NewBuilder()
			visibilityBuilder.Define(tc.point, table, "ids")
			visibilityBuilder.Define(tc.point, valueSym, "id")
			visibilityBuilder.Define(tc.point, registered, "registered")
			resolver := visibility.NewResolver(visibilityBuilder.Build())
			tableStateKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, tc.point, tablePath)
			if !ok {
				t.Fatal("missing table state key")
			}
			valueStateKey, ok := resolver.StateKeyAt(tc.point, valuePath)
			if !ok {
				t.Fatal("missing value state key")
			}
			registeredStateKey, ok := resolver.StateKeyAt(tc.point, registeredPath)
			if !ok {
				t.Fatal("missing registered state key")
			}
			tableKey, ok := resolver.KeySpace().InternStateKey(tableStateKey)
			if !ok {
				t.Fatal("missing table key")
			}
			input := factflow.FactsInput{
				DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
					tc.point: factflow.NewDynamicIndexWrite(
						tablePath,
						keySource,
						tc.source,
						dynamicindex.AdmissionAdmitted,
						factflow.DynamicIndexReadbackKeyAndValue,
					),
				},
			}
			if tc.sourcePath {
				input.ExpressionPaths = map[factflow.ExprRef]pathdom.Path{
					valueSource.ExprRef: valuePath,
				}
			}
			in := state.State{}.AddDynamicIndexAllValuesKeyMembership(tableKey, registeredStateKey)
			if tc.sourceMembership {
				in = in.AddPathKeyMembership(valueStateKey, registeredStateKey)
			}

			got := NewFactsNodeTransfer(FactsNodeTransferConfig{
				Facts:      factflow.NewFacts(input),
				Sources:    &recordingSourceValues{values: tc.values},
				Visibility: resolver,
			})(transfer.NodeContext{
				Registry: reg,
				Point:    tc.point,
			}, in)

			tables := got.DynamicIndexAllValuesKeyMembershipTables(tableKey)
			gotInvariant := len(tables) == 1 && tables[0] == registeredStateKey
			if gotInvariant != tc.wantInvariant {
				t.Fatalf("all-value invariant = %#v, want present=%v", tables, tc.wantInvariant)
			}
		})
	}
}

func TestFactsNodeTransferPrimaryDeleteClearsDynamicAllValueMembershipsToTable(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(42204)
	ids := symbol.ID(42204)
	registered := symbol.ID(42205)
	keySym := symbol.ID(42206)
	idsPath := pathdom.NewPath(ids, "ids")
	registeredPath := pathdom.NewPath(registered, "registered")
	keyPath := pathdom.NewPath(keySym, "id")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(42204), HasExpr: true}
	nilSource := factflow.NewNilValueSource(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, ids, "ids")
	visibilityBuilder.Define(point, registered, "registered")
	visibilityBuilder.Define(point, keySym, "id")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	idsStateKey, ok := resolver.StateKeyAt(point, idsPath)
	if !ok {
		t.Fatal("missing ids state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatal("missing registered state key")
	}
	idsKey, ok := resolver.KeySpace().InternStateKey(idsStateKey)
	if !ok {
		t.Fatal("missing ids key")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					registeredPath,
					keySource,
					nilSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				keySource.ExprRef: keyPath,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{
				keySource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
				nilSource: typevalue.Nil(reg),
			},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.AddDynamicIndexAllValuesKeyMembership(idsKey, registeredStateKey))

	if tables := got.DynamicIndexAllValuesKeyMembershipTables(idsKey); len(tables) != 0 {
		t.Fatalf("all-value memberships = %#v, want primary delete to clear reverse-map proof", tables)
	}
}

func TestFactsNodeTransferExpressionNilDynamicIndexWriteClearsReverseProof(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(42216)
	ids := symbol.ID(42216)
	registered := symbol.ID(42217)
	keySym := symbol.ID(42218)
	idsPath := pathdom.NewPath(ids, "ids")
	registeredPath := pathdom.NewPath(registered, "registered")
	keyPath := pathdom.NewPath(keySym, "id")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(42216), HasExpr: true}
	nilSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(42217), HasExpr: true}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, ids, "ids")
	visibilityBuilder.Define(point, registered, "registered")
	visibilityBuilder.Define(point, keySym, "id")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	idsStateKey, ok := resolver.StateKeyAt(point, idsPath)
	if !ok {
		t.Fatal("missing ids state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatal("missing registered state key")
	}
	registeredRootStateKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, point, registeredPath)
	if !ok {
		t.Fatal("missing registered root state key")
	}
	registeredRootKey, ok := resolver.KeySpace().InternStateKey(registeredRootStateKey)
	if !ok {
		t.Fatal("missing registered root key")
	}
	keyStateKey, ok := resolver.StateKeyAt(point, keyPath)
	if !ok {
		t.Fatal("missing key state key")
	}
	idsKey, ok := resolver.KeySpace().InternStateKey(idsStateKey)
	if !ok {
		t.Fatal("missing ids key")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					registeredPath,
					keySource,
					nilSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				keySource.ExprRef: keyPath,
				nilSource.ExprRef: keyPath,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{
				keySource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
				nilSource: typevalue.Nil(reg),
			},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		AddDynamicIndexAllValuesKeyMembership(idsKey, registeredStateKey).
		AddPathKeyMembership(keyStateKey, registeredStateKey))

	if tables := got.DynamicIndexAllValuesKeyMembershipTables(idsKey); len(tables) != 0 {
		t.Fatalf("all-value memberships = %#v, want expression-nil primary delete to clear reverse-map proof", tables)
	}
	site := dynamicindex.SiteForPoint(int(point))
	if tables := got.DynamicIndexValueKeyMembershipTables(registeredRootKey, site); len(tables) != 0 {
		t.Fatalf("dynamic value memberships = %#v, want expression-nil write not to prove stored value is a key", tables)
	}
}

func TestFactsNodeTransferSamePointPrimaryDeleteBlocksReverseReadProof(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(42207)
	target := symbol.ID(42207)
	ids := symbol.ID(42208)
	registered := symbol.ID(42209)
	keySym := symbol.ID(42210)
	targetPath := pathdom.NewPath(target, "stale_id")
	idsPath := pathdom.NewPath(ids, "ids")
	registeredPath := pathdom.NewPath(registered, "registered")
	keyPath := pathdom.NewPath(keySym, "id")
	readExpr := factflow.ExprRef(42207)
	keyExpr := factflow.ExprRef(42208)
	deleteKeyExpr := factflow.ExprRef(42209)
	readSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: readExpr, HasExpr: true}
	readKeySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	deleteKeySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: deleteKeyExpr, HasExpr: true}
	nilSource := factflow.NewNilValueSource(0)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, target, "stale_id")
	visibilityBuilder.Define(point, ids, "ids")
	visibilityBuilder.Define(point, registered, "registered")
	visibilityBuilder.Define(point, keySym, "id")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	idsStateKey, ok := resolver.StateKeyAt(point, idsPath)
	if !ok {
		t.Fatal("missing ids state key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(point, registeredPath)
	if !ok {
		t.Fatal("missing registered state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(point, targetPath)
	if !ok {
		t.Fatal("missing target state key")
	}
	idsKey, ok := resolver.KeySpace().InternStateKey(idsStateKey)
	if !ok {
		t.Fatal("missing ids key")
	}
	dyn, ok := factflow.NewDynamicIndexExpression(idsPath, readKeySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					registeredPath,
					deleteKeySource,
					nilSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			RootAssignments: map[cfg.Point]factflow.RootAssignment{
				point: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, readSource),
			},
			DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
				readExpr: dyn,
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				deleteKeyExpr: keyPath,
			},
		}),
		Sources: &recordingSourceValues{
			values: map[factflow.ValueSource]product.Value{
				readSource:      presentValue(reg),
				deleteKeySource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
				nilSource:       typevalue.Nil(reg),
			},
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.AddDynamicIndexAllValuesKeyMembership(idsKey, registeredStateKey))

	if got.HasPathKeyMembership(targetStateKey, registeredStateKey) {
		t.Fatalf("key memberships = %#v, want primary delete to block same-point reverse read proof", got.KeyMembershipsSnapshot())
	}
}

func TestFactsNodeTransferReverseDeleteRestoresClosedAllValueInvariant(t *testing.T) {
	reg := standard.Registry()
	readPoint := cfg.Point(42220)
	primaryDeletePoint := cfg.Point(42221)
	reverseDeletePoint := cfg.Point(42222)
	target := symbol.ID(42220)
	ids := symbol.ID(42221)
	registered := symbol.ID(42222)
	chanSym := symbol.ID(42223)
	targetPath := pathdom.NewPath(target, "channel_id")
	idsPath := pathdom.NewPath(ids, "channel_to_id")
	registeredPath := pathdom.NewPath(registered, "registered_channels")
	chanPath := pathdom.NewPath(chanSym, "chan")
	readExpr := factflow.ExprRef(42220)
	chanExpr := factflow.ExprRef(42221)
	targetExpr := factflow.ExprRef(42222)
	readSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: readExpr, HasExpr: true}
	chanSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: chanExpr, HasExpr: true}
	targetSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: targetExpr, HasExpr: true}
	nilSource := factflow.NewNilValueSource(0)
	visibilityBuilder := visibility.NewBuilder()
	targetVersion := visibilityBuilder.Define(readPoint, target, "channel_id")
	idsVersion := visibilityBuilder.Define(readPoint, ids, "channel_to_id")
	registeredVersion := visibilityBuilder.Define(readPoint, registered, "registered_channels")
	chanVersion := visibilityBuilder.Define(readPoint, chanSym, "chan")
	for _, point := range []cfg.Point{readPoint, primaryDeletePoint, reverseDeletePoint} {
		visibilityBuilder.SetVisible(point, target, targetVersion)
		visibilityBuilder.SetVisible(point, ids, idsVersion)
		visibilityBuilder.SetVisible(point, registered, registeredVersion)
		visibilityBuilder.SetVisible(point, chanSym, chanVersion)
	}
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	idsStateKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, readPoint, idsPath)
	if !ok {
		t.Fatal("missing ids state key")
	}
	idsKey, ok := resolver.KeySpace().InternStateKey(idsStateKey)
	if !ok {
		t.Fatal("missing ids key")
	}
	registeredStateKey, ok := resolver.StateKeyAt(readPoint, registeredPath)
	if !ok {
		t.Fatal("missing registered state key")
	}
	targetStateKey, ok := resolver.StateKeyAt(readPoint, targetPath)
	if !ok {
		t.Fatal("missing target state key")
	}
	chanStateKey, ok := resolver.StateKeyAt(readPoint, chanPath)
	if !ok {
		t.Fatal("missing chan state key")
	}
	dyn, ok := factflow.NewDynamicIndexExpression(idsPath, chanSource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{
			readExpr: dyn,
		},
		RootAssignments: map[cfg.Point]factflow.RootAssignment{
			readPoint: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, readSource),
		},
		DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
			primaryDeletePoint: factflow.NewDynamicIndexWrite(
				registeredPath,
				targetSource,
				nilSource,
				dynamicindex.AdmissionAdmitted,
				factflow.DynamicIndexReadbackKeyAndValue,
			),
			reverseDeletePoint: factflow.NewDynamicIndexWrite(
				idsPath,
				chanSource,
				nilSource,
				dynamicindex.AdmissionAdmitted,
				factflow.DynamicIndexReadbackKeyAndValue,
			),
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
			chanExpr:   chanPath,
			targetExpr: targetPath,
		},
	})
	sources := &recordingSourceValues{values: map[factflow.ValueSource]product.Value{
		readSource:   presentValue(reg),
		chanSource:   presentValue(reg),
		targetSource: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
		nilSource:    nilSourceValue(reg),
	}}
	transferFn := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts:      facts,
		Sources:    sources,
		Visibility: resolver,
	})
	st := state.State{}.AddDynamicIndexAllValuesKeyMembership(idsKey, registeredStateKey)
	st = transferFn(transfer.NodeContext{Registry: reg, Point: readPoint}, st)
	if !st.HasPathKeyMembership(targetStateKey, registeredStateKey) {
		t.Fatalf("key memberships = %#v, want read value known as registered key", st.KeyMembershipsSnapshot())
	}

	st = transferFn(transfer.NodeContext{Registry: reg, Point: primaryDeletePoint}, st)
	if tables := st.DynamicIndexAllValuesKeyMembershipTables(idsKey); len(tables) != 0 {
		t.Fatalf("all-value memberships = %#v, want primary delete to suspend invariant", tables)
	}
	restores := st.PendingDynamicAllValueRestores(idsKey, chanStateKey)
	if len(restores) != 1 || restores[0].Table != registeredStateKey {
		t.Fatalf("pending restores = %#v, want one restore for registered", restores)
	}

	st = transferFn(transfer.NodeContext{Registry: reg, Point: reverseDeletePoint}, st)
	tables := st.DynamicIndexAllValuesKeyMembershipTables(idsKey)
	if len(tables) != 1 || tables[0] != registeredStateKey {
		t.Fatalf("all-value memberships = %#v, want matching reverse delete to restore invariant", tables)
	}
	if restores := st.PendingDynamicAllValueRestores(idsKey, chanStateKey); len(restores) != 0 {
		t.Fatalf("pending restores = %#v, want restore consumed", restores)
	}
}

func TestFactsNodeTransferKnownDynamicIndexWriteAddsPathEqualityProof(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4021)
	table := symbol.ID(4021)
	sourceSymbol := symbol.ID(4022)
	tablePath := pathdom.NewPath(table, "holder")
	sourcePath := pathdom.NewPath(sourceSymbol, "tx")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4021), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4022), HasExpr: true}
	keyType := typ.LiteralString("tx")
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, keyType), keyType)
	writeValue := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   keyValue,
			valueSource: writeValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "holder")
	visibilityBuilder.Define(point, sourceSymbol, "tx")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				valueSource.ExprRef: sourcePath,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey(`sym4021@1["tx"]`)),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym4022@1")),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("known dynamic-index write did not publish path equality proof")
	}
	equivalent := got.EquivalentPathKeys(ks, pathdom.PathKey("sym4021@1.tx"))
	if !pathKeysContain(equivalent, pathdom.PathKey("sym4022@1")) {
		t.Fatalf("field equivalent keys = %#v, want source path", equivalent)
	}
}

func TestFactsNodeTransferBroadDynamicIndexWriteDoesNotAddPathEqualityProof(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4022)
	table := symbol.ID(4023)
	sourceSymbol := symbol.ID(4024)
	tablePath := pathdom.NewPath(table, "holder")
	sourcePath := pathdom.NewPath(sourceSymbol, "tx")
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4023), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(4024), HasExpr: true}
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	writeValue := presentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   keyValue,
			valueSource: writeValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "holder")
	visibilityBuilder.Define(point, sourceSymbol, "tx")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				valueSource.ExprRef: sourcePath,
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if equivalent := got.EquivalentPathKeys(ks, pathdom.PathKey("sym4023@1.tx")); len(equivalent) != 0 {
		t.Fatalf("broad dynamic-index key published equality: %#v", equivalent)
	}
}

func pathKeysContain(keys []pathdom.PathKey, want pathdom.PathKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func TestFactsNodeTransferDynamicIndexWritePublishesFirstHeapDynamicFact(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(403)
	table := symbol.ID(403)
	tablePath := pathdom.NewPath(table, "table")
	tableID := identity.ID{Kind: "test.table", Site: "dynamic", Index: 1}
	tableValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(tableID))
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(405), HasExpr: true}
	valueSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(406), HasExpr: true}
	keyValue := presentValue(reg)
	writeValue := absentValue(reg)
	sources := &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{
			keySource:   keyValue,
			valueSource: writeValue,
		},
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, table, "table")
	resolver := visibility.NewResolver(visibilityBuilder.Build())

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			DynamicIndexWrites: map[cfg.Point]factflow.DynamicIndexWrite{
				point: factflow.NewDynamicIndexWrite(
					tablePath,
					keySource,
					valueSource,
					dynamicindex.AdmissionAdmitted,
					factflow.DynamicIndexReadbackKeyAndValue,
				),
			},
		}),
		Sources:    sources,
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.
		WriteValue(reg, key.SymbolValue(table), tableValue).
		WriteHeapTableObject(reg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableValue})))

	dynamicKey := dynamicindex.Key{Table: mustStateKey(t, resolver.KeySpace(), pathdom.PathKey("sym403")), Site: dynamicindex.SiteForPoint(int(point))}
	object := got.ReadHeapTableObject(reg, tableID)
	heapFact, ok := object.DynamicIndexFact(dynamicKey)
	if !ok {
		t.Fatalf("heap dynamic fact missing for %v", dynamicKey)
	}
	if !presence.Equal(heapFact.KeyPresence, presence.Present()) ||
		!product.Equal(reg, heapFact.KeyValue, keyValue) ||
		!product.Equal(reg, heapFact.Value, writeValue) ||
		heapFact.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("heap dynamic-index fact = %#v, want key/value/admitted mapping", heapFact)
	}
}

func TestResolvePathValueReadsHeapDynamicFactAcrossPathKeyContexts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(404)
	root := symbol.ID(404)
	rootPath := pathdom.NewPath(root, "batch")
	itemsPath := rootPath.Field("items")
	itemPath := itemsPath.IndexStr("route-1")
	rootID := identity.ID{Kind: "test.table", Site: "root", Index: 1}
	itemsID := identity.ID{Kind: "test.table", Site: "items", Index: 1}
	itemID := identity.ID{Kind: "test.table", Site: "item", Index: 1}
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	itemsValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemsID))
	itemValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(itemID))
	routeKeyType := typ.LiteralString("route-1")
	routeKeyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, routeKeyType), routeKeyType)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, root, "batch")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	itemsKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("items").Segments)
	if !ok {
		t.Fatal("missing items suffix key")
	}
	oldDynamicKey := dynamicindex.Key{
		Table: mustStateKey(t, ks, pathdom.PathKey("callee.items")),
		Site:  dynamicindex.Site("callee.write"),
	}
	st := state.State{}.
		WriteValue(reg, key.SymbolValue(root), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{itemsKey: itemsValue},
		})).
		WriteHeapTableObject(reg, itemsID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: itemsValue,
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
				oldDynamicKey: {
					KeyPresence: presence.Present(),
					KeyValue:    routeKeyValue,
					Value:       itemValue,
					Admission:   dynamicindex.AdmissionAdmitted,
				},
			},
		}))

	got, ok := resolvePathValueAt(reg, resolver, point, st, itemPath, nil)
	if !ok {
		t.Fatalf("resolvePathValueAt(%s) returned false", itemPath)
	}
	if !product.Equal(reg, got.value, itemValue) {
		t.Fatalf("resolved value = %s, want item identity", formatValue(reg, got.value))
	}
}

func TestResolvePathValuePrefersReassignedParentStaticMemberOverStaleRootProjection(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4041)
	container := symbol.ID(4041)
	containerPath := pathdom.NewPath(container, "container")
	clientPath := containerPath.Field("client")
	metaPath := clientPath.Field("meta")
	containerID := identity.ID{Kind: "test.table", Site: "container", Index: 1}
	replacementID := identity.ID{Kind: "test.table", Site: "replacement", Index: 1}
	containerValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(containerID))
	replacementValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(replacementID))
	stale := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	fresh := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, container, "container")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	staleKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("client.meta").Segments)
	if !ok {
		t.Fatal("missing client.meta suffix key")
	}
	freshKey, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("meta").Segments)
	if !ok {
		t.Fatal("missing meta suffix key")
	}
	clientKey, ok := visibility.AddressAt(resolver, point, clientPath).VisibleLocalKeyspaceKey()
	if !ok {
		t.Fatal("missing container.client path key")
	}
	st := state.State{}.
		WriteValue(reg, key.SymbolValue(container), containerValue).
		WriteLocalPathKey(reg, clientKey, replacementValue).
		WriteHeapTableObject(reg, containerID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          containerValue,
			StaticMembers: map[keyspace.Key]product.Value{staleKey: stale},
		})).
		WriteHeapTableObject(reg, replacementID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          replacementValue,
			StaticMembers: map[keyspace.Key]product.Value{freshKey: fresh},
		}))

	got, ok := resolvePathValueAt(reg, resolver, point, st, metaPath, nil)
	if !ok {
		t.Fatalf("resolvePathValueAt(%s) returned false", metaPath)
	}
	if !product.Equal(reg, got.value, fresh) {
		t.Fatalf("resolved value = %s, want reassigned parent static member %s", formatValue(reg, got.value), formatValue(reg, fresh))
	}
}

func TestDynamicIndexStaticProjectionRequiresExactKey(t *testing.T) {
	reg := standard.Registry()
	exactType := typ.LiteralString("value")
	exact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    typevalue.WithWitness(reg, typevalue.FromType(reg, exactType), exactType),
		Value:       presentValue(reg),
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	broad := exact
	broad.KeyValue = typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	wrongType := typ.LiteralString("other")
	wrong := exact
	wrong.KeyValue = typevalue.WithWitness(reg, typevalue.FromType(reg, wrongType), wrongType)

	seg := fieldSuffix("value").Segments[0]
	if !dynamicIndexFactDefinitelyMatchesSegment(reg, exact, seg) {
		t.Fatalf("exact literal key did not prove static segment")
	}
	if dynamicIndexFactDefinitelyMatchesSegment(reg, broad, seg) {
		t.Fatalf("broad string key proved static segment")
	}
	if dynamicIndexFactDefinitelyMatchesSegment(reg, wrong, seg) {
		t.Fatalf("wrong literal key proved static segment")
	}
}

func TestFactsEdgeTransferAddsPointLevelBranchPathEvidenceOnBothBranchOutputs(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	err := symbol.ID(403)
	left := symbol.ID(404)
	right := symbol.ID(405)
	errPath := pathdom.NewPath(err, "err")
	leftPath := pathdom.NewPath(left, "left").Field("value")
	rightPath := pathdom.NewPath(right, "right").Field("value")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, err, "err")
	visibilityBuilder.Define(branch, left, "left")
	visibilityBuilder.Define(branch, right, "right")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	wantPresence := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     mustStateKey(t, ks, pathdom.PathKey("sym403@1")),
		Presence: presence.Present(),
	}
	wantEquality := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey("sym404@1.value")),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym405@1.value")),
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathPresenceEvidenceOnEdge(errPath, presence.Present(), true),
						factflow.NewBranchPathPresenceEvidenceOnEdge(errPath, presence.Present(), false),
						factflow.NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, true),
						factflow.NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	if !got[thenPoint].HasBranchProof(wantPresence) || !got[thenPoint].HasBranchProof(wantEquality) {
		t.Fatalf("true branch missing point-level branch proofs")
	}
	if !got[elsePoint].HasBranchProof(wantPresence) || !got[elsePoint].HasBranchProof(wantEquality) {
		t.Fatalf("false branch missing point-level branch proofs")
	}
}

func TestFactsEdgeTransferBranchPathEvidenceRespectEdgesAndJoinByIntersection(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, join, false)
	graph.AddEdge(elsePoint, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	err := symbol.ID(430)
	left := symbol.ID(431)
	right := symbol.ID(432)
	errPath := pathdom.NewPath(err, "err")
	leftPath := pathdom.NewPath(left, "left").Field("value")
	rightPath := pathdom.NewPath(right, "right").Field("value")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(branch, err, "err")
	visibilityBuilder.Define(branch, left, "left")
	visibilityBuilder.Define(branch, right, "right")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	oneSided := pathevidence.BranchProof{
		Kind:     pathevidence.BranchProofPathPresence,
		Path:     mustStateKey(t, ks, pathdom.PathKey("sym430@1")),
		Presence: presence.Present(),
	}
	twoSided := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey("sym431@1.value")),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym432@1.value")),
	}

	got := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				BranchPathEvidence: map[cfg.Point]factflow.BranchPathEvidenceSet{
					branch: factflow.NewBranchPathEvidenceSet(
						factflow.NewBranchPathPresenceEvidenceOnEdge(errPath, presence.Present(), true),
						factflow.NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, true),
						factflow.NewBranchPathEqualityEvidenceOnEdge(leftPath, rightPath, false),
					),
				},
			}),
			Visibility: resolver,
		}),
	})

	if !got[thenPoint].HasBranchProof(oneSided) || !got[thenPoint].HasBranchProof(twoSided) {
		t.Fatalf("true branch proofs missing one-sided or two-sided proof")
	}
	if got[elsePoint].HasBranchProof(oneSided) {
		t.Fatalf("false branch kept true-edge-only proof")
	}
	if !got[elsePoint].HasBranchProof(twoSided) {
		t.Fatalf("false branch dropped two-sided proof")
	}
	if got[join].HasBranchProof(oneSided) {
		t.Fatalf("one-sided proof survived join")
	}
	if !got[join].HasBranchProof(twoSided) {
		t.Fatalf("two-sided proof did not survive join")
	}
}

func TestFactsNodeTransferAppliesChannelSelectFactsWithPathKeys(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(403)
	result := symbol.ID(406)
	selectedCase := symbol.ID(407)
	resultPath := pathdom.NewPath(result, "select").Field("result")
	casePath := pathdom.NewPath(selectedCase, "select").Field("case")
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, result, "select")
	visibilityBuilder.Define(point, selectedCase, "select")
	want := channelselectfact.Fact{
		Select: channelselectfact.ID("select-1"),
		Kind:   channelselectfact.FactReceive,
		Result: testStateKey(t, pathdom.PathKey("sym406@1.result")),
		Case:   testStateKey(t, pathdom.PathKey("sym407@1.case")),
		Index:  2,
	}
	wantSelect := channelselectfact.Fact{
		Select:     channelselectfact.ID("select-1"),
		Kind:       channelselectfact.FactSelect,
		Result:     testStateKey(t, pathdom.PathKey("sym406@1.result")),
		Index:      0,
		HasDefault: true,
	}

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
				point: factflow.NewChannelSelectSet(
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      factflow.ChannelSelectID("select-1"),
						Kind:          factflow.ChannelSelectSelect,
						ResultPath:    resultPath,
						HasResultPath: true,
						HasDefault:    true,
						Index:         0,
					}),
					factflow.NewChannelSelect(factflow.ChannelSelectConfig{
						SelectID:      factflow.ChannelSelectID("select-1"),
						Kind:          factflow.ChannelSelectReceive,
						ResultPath:    resultPath,
						HasResultPath: true,
						CasePath:      casePath,
						HasCasePath:   true,
						Index:         2,
					}),
				),
			},
		}),
		Visibility: visibility.NewResolver(visibilityBuilder.Build()),
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if !got.HasChannelSelectFact(want) {
		t.Fatalf("channel-select fact missing: %#v", want)
	}
	if !got.HasChannelSelectFact(wantSelect) {
		t.Fatalf("channel-select default fact missing: %#v", wantSelect)
	}
}

func TestFactsNodeTransferCallOutcomeRebasesPathRefinement(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(501)
	arg := symbol.ID(501)
	argPath := pathdom.NewPath(arg, "arg")
	argFieldKey := pathdom.PathKey("sym501@1.field")
	placeholderKey := pathdom.NewPlaceholder(0).Field("field").Key()
	argExpr := factflow.ExprRef(501)
	refinement := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, arg, "arg")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathRefinements: []callboundary.PathValueFact{
						{Path: pathdom.NewPlaceholder(0).Field("field"), Value: refinement},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{}.WritePathKey(reg, ks, argFieldKey, product.Top()))

	assertPathValue(t, reg, ks, got, argFieldKey, refinement)
	assertPathValue(t, reg, ks, got, placeholderKey, product.Bottom(reg))
}

func TestFactsNodeTransferStatementCallOutcomeDoesNotWriteReturnSlots(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(502)
	arg := symbol.ID(502)
	argPath := pathdom.NewPath(arg, "arg")
	argKey := pathdom.PathKey("sym502@1.side")
	argExpr := factflow.ExprRef(502)
	returnValue := absentValue(reg)
	sideValue := presentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, arg, "arg")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				Results: []callpayload.CallResult{{Index: 0, Value: returnValue}},
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathStaticMembers: []callboundary.PathStaticMemberFact{
						{Path: pathdom.NewPlaceholder(0).Field("side"), Value: sideValue},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue := got.ReadReturnSlot(reg, 0); !product.Equal(reg, gotValue, product.Bottom(reg)) {
		t.Fatalf("return slot 0 = %s, want bottom for statement call", formatValue(reg, gotValue))
	}
	if gotValue, ok := got.ReadPathStaticMember(ks, argKey); !ok || !product.Equal(reg, gotValue, sideValue) {
		t.Fatalf("statement side fact = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, sideValue))
	}
}

func TestFactsNodeTransferCallOutcomeBindsReceiverBeforeExplicitArgs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(503)
	receiver := symbol.ID(503)
	arg := symbol.ID(504)
	receiverPath := pathdom.NewPath(receiver, "receiver")
	argPath := pathdom.NewPath(arg, "arg")
	receiverKey := pathdom.PathKey("sym503@1.self")
	argKey := pathdom.PathKey("sym504@1.value")
	argExpr := factflow.ExprRef(503)
	receiverValue := presentValue(reg)
	argValue := absentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, receiver, "receiver")
	visibilityBuilder.Define(point, arg, "arg")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context:         factflow.CallSiteContextStatement,
					ReceiverPath:    receiverPath,
					HasReceiverPath: true,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				argExpr: argPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PathStaticMembers: []callboundary.PathStaticMemberFact{
						{Path: pathdom.NewPlaceholder(0).Field("self"), Value: receiverValue},
						{Path: pathdom.NewPlaceholder(1).Field("value"), Value: argValue},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if gotValue, ok := got.ReadPathStaticMember(ks, receiverKey); !ok || !product.Equal(reg, gotValue, receiverValue) {
		t.Fatalf("receiver static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, receiverValue))
	}
	if gotValue, ok := got.ReadPathStaticMember(ks, argKey); !ok || !product.Equal(reg, gotValue, argValue) {
		t.Fatalf("arg static member = %s/%v, want %s/true", formatValue(reg, gotValue), ok, formatValue(reg, argValue))
	}
}

func TestFactsNodeTransferCallOutcomeRebasesBoundaryFacts(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(504)
	first := symbol.ID(505)
	second := symbol.ID(506)
	firstPath := pathdom.NewPath(first, "first")
	secondPath := pathdom.NewPath(second, "second")
	firstExpr := factflow.ExprRef(504)
	secondExpr := factflow.ExprRef(505)
	present := presentValue(reg)
	absent := absentValue(reg)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, first, "first")
	visibilityBuilder.Define(point, second, "second")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			CallSites: map[cfg.Point]factflow.CallSite{
				point: factflow.NewCallSite(factflow.CallSiteConfig{
					Context: factflow.CallSiteContextStatement,
					ArgumentSources: []factflow.ValueSource{
						{Kind: factflow.ValueSourceExpression, ExprRef: firstExpr, HasExpr: true},
						{Kind: factflow.ValueSourceExpression, ExprRef: secondExpr, HasExpr: true},
					},
				}),
			},
			ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
				firstExpr:  firstPath,
				secondExpr: secondPath,
			},
		}),
		CallOutcome: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{
				NormalReturnFacts: callboundary.NormalReturnFacts{
					DynamicIndexFacts: []callboundary.DynamicIndexFact{
						{
							Table: pathdom.NewPlaceholder(0).Field("items"),
							Site:  "callee.dynamic",
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
							Path:  pathdom.NewPlaceholder(0).Field("left"),
							Other: pathdom.NewPlaceholder(1).Field("right"),
						},
					},
					ChannelSelects: []callboundary.ChannelSelectFact{
						{
							Select: channelselectfact.ID("callee.select"),
							Kind:   channelselectfact.FactReceive,
							Result: pathdom.NewPlaceholder(0).Field("result"),
							Case:   pathdom.NewPlaceholder(1).Field("case"),
							Index:  3,
						},
					},
					EffectDeltas: []callboundary.EffectDelta{
						{
							Target: pathdom.NewPlaceholder(0).Field("items"),
							Site:   "callee.effect",
							Kind:   effectdelta.Mutation,
							Value: effectdelta.Value{
								Before: present,
								After:  absent,
								Change: effectdelta.ChangeChanged,
							},
						},
					},
					EscapeEvents: []callboundary.EscapeEventFact{
						{
							Target:    pathdom.NewPlaceholder(0).Field("sent"),
							Kind:      callboundary.EscapeEventSend,
							Recursive: true,
						},
					},
				},
			}
		},
		Visibility: resolver,
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	dynamicKey := dynamicindex.Key{Table: mustStateKey(t, ks, pathdom.PathKey("sym505@1.items")), Site: dynamicindex.Site("callee.dynamic")}
	gotDynamic := got.ReadDynamicIndexFact(reg, dynamicKey)
	if !presence.Equal(gotDynamic.KeyPresence, presence.Present()) ||
		!product.Equal(reg, gotDynamic.KeyValue, present) ||
		!product.Equal(reg, gotDynamic.Value, absent) ||
		gotDynamic.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic-index fact = %#v, want rebased fact", gotDynamic)
	}

	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  mustStateKey(t, ks, pathdom.PathKey("sym505@1.left")),
		Other: mustStateKey(t, ks, pathdom.PathKey("sym506@1.right")),
	}
	if !got.HasBranchProof(proof) {
		t.Fatalf("branch proof missing: %#v", proof)
	}

	selectFact := channelselectfact.Fact{
		Select: channelselectfact.ID("callee.select"),
		Kind:   channelselectfact.FactReceive,
		Result: testStateKey(t, pathdom.PathKey("sym505@1.result")),
		Case:   testStateKey(t, pathdom.PathKey("sym506@1.case")),
		Index:  3,
	}
	if !got.HasChannelSelectFact(selectFact) {
		t.Fatalf("channel-select fact missing: %#v", selectFact)
	}

	effectKey := effectdelta.Key{
		Target: mustStateKey(t, ks, pathdom.PathKey("sym505@1.items")),
		Site:   "callee.effect",
		Kind:   effectdelta.Mutation,
	}
	gotEffect := got.ReadEffectDelta(effectKey)
	if !product.Equal(reg, gotEffect.Before, present) ||
		!product.Equal(reg, gotEffect.After, absent) ||
		gotEffect.Change != effectdelta.ChangeChanged {
		t.Fatalf("effect delta = %#v, want rebased delta", gotEffect)
	}

	escapeEvent := state.EscapeEvent{
		Target:    testStateKey(t, pathdom.PathKey("sym505@1.sent")),
		Kind:      callboundary.EscapeEventSend,
		Recursive: true,
	}
	if !got.HasEscapeEvent(escapeEvent) {
		t.Fatalf("escape event missing: %#v", escapeEvent)
	}
}
