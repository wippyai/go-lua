package readexpr

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/access"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestProjectionScratchUsesStructuralPathIdentity(t *testing.T) {
	point := cfg.Point(1)
	first := path.NewPath(symbol.ID(101), "value")
	second := path.NewPath(symbol.ID(102), "value")
	ks := keyspace.New()
	firstID := projectionPathIdentity{Key: ks.FromPath(first)}
	secondID := projectionPathIdentity{Key: ks.FromPath(second)}

	var active projectionActive
	if !active.push(projectionFrame{point: point, path: firstID}) {
		t.Fatal("push returned false")
	}
	if active.contains(projectionFrame{point: point, path: secondID}) {
		t.Fatal("active projection stack conflated distinct symbols with the same display root")
	}
	active.pop(projectionFrame{point: point, path: secondID})
	if !active.contains(projectionFrame{point: point, path: firstID}) {
		t.Fatal("pop of different symbol removed active frame")
	}

	var memo projectionMemo
	firstKey := projectionMemoKey{point: point, path: firstID}
	secondKey := projectionMemoKey{point: point, path: secondID}
	memo.remember(firstKey, projectionResult{ok: true})
	if _, ok := memo.lookup(secondKey); ok {
		t.Fatal("projection memo conflated distinct symbols with the same display root")
	}

	for i := 0; i < projectionScratchInline; i++ {
		p := path.NewPath(symbol.ID(200+i), "value")
		id := projectionPathIdentity{Key: ks.FromPath(p)}
		active.push(projectionFrame{point: point, path: id})
		memo.remember(projectionMemoKey{point: point, path: id}, projectionResult{ok: true})
	}
	if active.entries == nil || memo.entries == nil {
		t.Fatal("test setup did not force overflow maps")
	}
	if active.contains(projectionFrame{point: point, path: secondID}) {
		t.Fatal("overflow active stack conflated distinct symbols with the same display root")
	}
	if _, ok := memo.lookup(secondKey); ok {
		t.Fatal("overflow projection memo conflated distinct symbols with the same display root")
	}
}

func TestProjectionScratchResetClearsAndRetainsSmallOverflowMaps(t *testing.T) {
	point := cfg.Point(1)
	ks := keyspace.New()
	var active projectionActive
	var memo projectionMemo

	first := projectionFrame{
		point: point,
		path:  projectionPathIdentity{Key: ks.FromPath(path.NewPath(symbol.ID(1000), "value"))},
	}
	for i := 0; i <= projectionScratchInline; i++ {
		frame := projectionFrame{
			point: point,
			path:  projectionPathIdentity{Key: ks.FromPath(path.NewPath(symbol.ID(1000+i), "value"))},
		}
		active.push(frame)
		memo.remember(projectionMemoKey{point: point, path: frame.path}, projectionResult{ok: true})
	}
	if active.entries == nil || memo.entries == nil {
		t.Fatal("test setup did not force overflow maps")
	}

	active.reset()
	memo.reset()
	if active.entries == nil || memo.entries == nil {
		t.Fatal("small overflow maps were discarded instead of retained for scratch reuse")
	}
	if len(active.entries) != 0 || len(memo.entries) != 0 {
		t.Fatalf("reset left stale entries: active=%d memo=%d", len(active.entries), len(memo.entries))
	}
	if active.contains(first) {
		t.Fatal("reset active stack still contains stale frame")
	}
	if _, ok := memo.lookup(projectionMemoKey{point: point, path: first.path}); ok {
		t.Fatal("reset memo still contains stale result")
	}
}

func TestProjectExactPresentDropsNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1)
	resolver := testResolver(point, symbol.ID(10), "t")
	readPath := path.NewPath(symbol.ID(10), "t").Field("name")
	childKey := resolver.KeyAt(point, readPath)
	childValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Nil)),
	)
	in := state.State{}.WritePathKey(reg, resolver.KeySpace(), childKey, childValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
}

func TestProjectDefiniteDynamicIndexWriteOverridesStaleExactAbsent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(21)
	tableSym := symbol.ID(31)
	resolver := testResolver(point, tableSym, "p")
	tablePath := path.NewPath(tableSym, "p")
	memberPath := tablePath.Field("send")
	memberKey := resolver.KeyAt(point, memberPath)
	tableStateKey, ok := resolver.StateKeyAt(point, tablePath)
	if !ok {
		t.Fatal("StateKeyAt(table) failed")
	}
	tableKey, ok := resolver.KeySpace().InternStateKey(tableStateKey)
	if !ok {
		t.Fatal("InternStateKey(table) failed")
	}
	fnType := typ.Func().Param("v", typ.String).Build()
	fnValue := typevalue.WithWitness(reg, typevalue.FromType(reg, fnType), fnType)
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("send")), typ.LiteralString("send"))
	in := state.State{}.
		WritePathKey(reg, resolver.KeySpace(), memberKey, product.Absent(reg)).
		WriteDynamicIndexFact(reg, dynamicindex.Key{
			Table: tableKey,
			Site:  dynamicindex.SiteForPoint(int(point)),
		}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    keyValue,
			Value:       fnValue,
			Admission:   dynamicindex.AdmissionAdmitted,
		})

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, memberPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatal("Project did not carry a type witness")
	}
	if !typ.TypeEquals(gotType, fnType) {
		t.Fatalf("projected type = %v, want %v", gotType, fnType)
	}
}

func TestDynamicIndexProviderUsesExactLiteralKeyFactWhenPathMembershipVersionDiffers(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	point := cfg.Point(221)
	tableSym := symbol.ID(321)
	keySym := symbol.ID(322)
	tablePath := path.NewPath(tableSym, "suites")
	keyPath := path.NewPath(keySym, "suite")
	builder := visibility.NewBuilder()
	builder.Define(point, tableSym, "suites")
	builder.Define(point, keySym, "suite")
	resolver := visibility.NewResolver(builder.Build())
	tableKey, ok := visibility.AddressAt(resolver, point, tablePath).RootOrVisibleKeyspaceKey()
	if !ok {
		t.Fatal("missing table key")
	}
	keyStateKey, ok := resolver.StateKeyAt(point, keyPath)
	if !ok {
		t.Fatal("missing key state")
	}
	keyType := typ.LiteralString("alpha")
	keyValue := typevalue.WithWitness(reg, typeValues.FromType(reg, keyType), keyType)
	arrayType := typ.NewArray(typ.Any)
	arrayValue := typeValues.FromTypeWithWitness(reg, arrayType)
	tableValue := typeValues.FromTypeWithWitness(reg, typetable.NewMap(typ.String, arrayType))
	keyExpr := factflow.ExprRef(22101)
	readExpr := factflow.ExprRef(22102)
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	dyn, ok := factflow.NewDynamicIndexExpression(tablePath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{readExpr: dyn},
		ExpressionPaths:         map[factflow.ExprRef]path.Path{keyExpr: keyPath},
	})
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(tableSym), tableValue).
		WriteValue(reg, key.SymbolValue(keySym), keyValue).
		WritePathKey(reg, resolver.KeySpace(), keyStateKey.PathKey(), keyValue).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: tableKey, Site: dynamicindex.SiteForPoint(int(point))}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue:    keyValue,
			HasKeyValue: true,
			Value:       arrayValue,
			HasValue:    true,
			Admission:   dynamicindex.AdmissionUnknown,
		}))
	if gotKey, ok := dynamicIndexExpressionKeyValue(Config{Registry: reg, Facts: facts, Visibility: resolver, TypeValues: typeValues}, point, keySource, in); !ok {
		t.Fatal("key value did not resolve")
	} else if name, ok := staticStringKey(reg, typeValues, gotKey); !ok || name != "alpha" {
		t.Fatalf("key value = %v/%v, want alpha", name, ok)
	}
	if !dynamicIndexFactDefinitelyPresent(reg, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: arrayValue, HasValue: true, Admission: dynamicindex.AdmissionUnknown})) {
		t.Fatalf("array value not classified present: %v", product.PresenceOf(arrayValue))
	}

	value, ok := Provider(Config{
		Registry:   reg,
		Facts:      facts,
		Visibility: resolver,
		TypeValues: typeValues,
	})(point, readExpr, factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: readExpr, HasExpr: true}, in)
	if !ok {
		t.Fatal("Provider returned false")
	}
	assertPresence(t, reg, value, presence.Present())
	gotType, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(gotType, arrayType) {
		t.Fatalf("value type = %v/%v, want %v", gotType, ok, arrayType)
	}
}

func TestProjectRootStaticMemberOverlayUsesCurrentChildPathValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(211)
	suiteSym := symbol.ID(311)
	resolver := testResolver(point, suiteSym, "suite")
	rootPath := path.NewPath(suiteSym, "suite")
	testsPath := rootPath.Field("tests")
	testsKey := resolver.KeyAt(point, testsPath)
	emptyRecordType := typetable.NewRecord().Build()
	arrayType := typ.NewArray(typ.String)
	rootType := typetable.NewRecord().
		Field("name", typ.String).
		Field("tests", emptyRecordType).
		Build()
	rootValue := product.Set(
		reg,
		typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.Table),
	)
	want := typetable.NewRecord().
		Field("name", typ.String).
		Field("tests", arrayType).
		Build()
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(suiteSym), rootValue).
		WritePathStaticMember(resolver.KeySpace(), testsKey, typevalue.WithWitness(reg, typevalue.FromType(reg, emptyRecordType), emptyRecordType)).
		WritePathKey(reg, resolver.KeySpace(), testsKey, typevalue.WithWitness(reg, typevalue.FromType(reg, arrayType), arrayType))

	rawChildType, ok := typevalue.TypeOf(reg, in.ReadPathKey(reg, resolver.KeySpace(), testsKey))
	if !ok || !typ.TypeEquals(rawChildType, arrayType) {
		t.Fatalf("raw child path-key type = %v/%v, want %v", rawChildType, ok, arrayType)
	}
	currentChild, ok := currentPathKeyValue(Config{Registry: reg, Visibility: resolver}, point, testsPath, in)
	if !ok {
		t.Fatal("currentPathKeyValue returned false")
	}
	currentChildType, ok := typevalue.TypeOf(reg, currentChild)
	if !ok || !typ.TypeEquals(currentChildType, arrayType) {
		t.Fatalf("current child path-key type = %v/%v, want %v", currentChildType, ok, arrayType)
	}
	currentIndexedChild, ok := currentPathKeyValue(Config{Registry: reg, Visibility: resolver}, point, rootPath.IndexStr("tests"), in)
	if !ok {
		t.Fatal("currentPathKeyValue for indexed child returned false")
	}
	currentIndexedChildType, ok := typevalue.TypeOf(reg, currentIndexedChild)
	if !ok || !typ.TypeEquals(currentIndexedChildType, arrayType) {
		t.Fatalf("current indexed child path-key type = %v/%v, want %v", currentIndexedChildType, ok, arrayType)
	}
	overlaid := overlayStaticMemberWitness(Config{Registry: reg, Visibility: resolver}, point, rootPath, in, rootValue)
	overlaidType, ok := typevalue.TypeOf(reg, overlaid)
	if !ok || !typ.TypeEquals(overlaidType, want) {
		t.Fatalf("direct overlay type = %v/%v, want %v", overlaidType, ok, want)
	}
	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("projected root type = %v/%v, want %v", gotType, ok, want)
	}
}

func TestProjectExactPresentMergesOptionalFieldTypeFromRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	profileSym := symbol.ID(19)
	resolver := testResolver(point, profileSym, "opt")
	rootPath := path.NewPath(profileSym, "opt")
	readPath := rootPath.Field("label")
	childKey := resolver.KeyAt(point, readPath)
	profileType := typ.NewAlias(
		"__test_Profile",
		typetable.NewRecord().OptField("label", typ.String).Build(),
	)
	rootValue := typevalue.WithWitness(reg, product.Top(), profileType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(profileSym), rootValue).
		WritePathKey(reg, resolver.KeySpace(), childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
}

func TestProjectExactFieldUsesNarrowedRootWitnessWhenCompatible(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(21)
	resSym := symbol.ID(31)
	resolver := testResolver(point, resSym, "r")
	rootPath := path.NewPath(resSym, "r")
	readPath := rootPath.Field("value")
	childKey := resolver.KeyAt(point, readPath)
	okRecord := typetable.NewRecord().
		Field("tag", typ.LiteralString("ok")).
		Field("value", typ.String).
		Build()
	errRecord := typetable.NewRecord().
		Field("tag", typ.LiteralString("err")).
		Field("value", typ.Number).
		Build()
	okCase := typ.NewAlias("__test_OK", okRecord)
	errCase := typ.NewAlias("__test_ERR", errRecord)
	union := typeexpr.Union(okCase, errCase)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), okCase)
	staleChild := typevalue.FromType(reg, typeexpr.Union(typ.Number, typ.String))
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(resSym), rootValue).
		WritePathKey(reg, resolver.KeySpace(), childKey, staleChild)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
	if gotType, ok := typevalue.TypeOf(reg, got); !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("projected type = %v/%v, want string", gotType, ok)
	}
}

func TestProjectOriginFieldUsesNarrowedRootWitnessWhenOriginIsBroader(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	resSym := symbol.ID(32)
	resolver := testResolver(point, resSym, "r")
	rootPath := path.NewPath(resSym, "r")
	readPath := rootPath.Field("value")
	okRecord := typetable.NewRecord().
		Field("tag", typ.LiteralString("ok")).
		Field("value", typ.String).
		Build()
	errRecord := typetable.NewRecord().
		Field("tag", typ.LiteralString("err")).
		Field("value", typ.Number).
		Build()
	okCase := typ.NewAlias("__test_OK2", okRecord)
	errCase := typ.NewAlias("__test_ERR2", errRecord)
	union := typeexpr.Union(okCase, errCase)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), okCase)
	in := state.State{}.WriteValue(reg, key.SymbolValue(resSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
	if gotType, ok := typevalue.TypeOf(reg, got); !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("projected type = %v/%v, want string", gotType, ok)
	}
}

func TestProjectStructuralFieldInheritsMaybeParentPresence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	resSym := symbol.ID(24)
	resolver := testResolver(point, resSym, "res")
	resPath := path.NewPath(resSym, "res")
	readPath := resPath.Field("answer")
	resType := typetable.NewRecord().Field("answer", typ.String).Build()
	resValue := typevalue.WithWitness(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()),
		resType,
	)
	in := state.State{}.WriteValue(reg, key.SymbolValue(resSym), resValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectStructuralLiteralFieldInheritsMaybeParentPresence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(16)
	resSym := symbol.ID(26)
	resolver := testResolver(point, resSym, "res")
	resPath := path.NewPath(resSym, "res")
	readPath := resPath.Field("answer")
	resType := typetable.NewRecord().Field("answer", typ.LiteralString("ok")).Build()
	resValue := typevalue.WithWitness(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Maybe()),
		resType,
	)
	in := state.State{}.WriteValue(reg, key.SymbolValue(resSym), resValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectStructuralFieldKeepsOptionalParentWitnessNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(15)
	resSym := symbol.ID(25)
	resolver := testResolver(point, resSym, "res")
	resPath := path.NewPath(resSym, "res")
	readPath := resPath.Field("answer")
	resType := typeexpr.Optional(typetable.NewRecord().Field("answer", typ.String).Build())
	resValue := typevalue.WithWitness(reg, typevalue.FromType(reg, resType), resType)
	in := state.State{}.WriteValue(reg, key.SymbolValue(resSym), resValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectStructuralLiteralFieldKeepsOptionalParentWitnessNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	resSym := symbol.ID(27)
	resolver := testResolver(point, resSym, "res")
	resPath := path.NewPath(resSym, "res")
	readPath := resPath.Field("answer")
	resType := typeexpr.Optional(typetable.NewRecord().Field("answer", typ.LiteralString("ok")).Build())
	resValue := typevalue.WithWitness(reg, typevalue.FromType(reg, resType), resType)
	assertPresence(t, reg, resValue, presence.Maybe())
	in := state.State{}.WriteValue(reg, key.SymbolValue(resSym), resValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectExactPresentChildInheritsExplicitTopEvidenceFromRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	rawSym := symbol.ID(20)
	resolver := testResolver(point, rawSym, "raw")
	rootPath := path.NewPath(rawSym, "raw")
	readPath := rootPath.Field("id")
	childKey := resolver.KeyAt(point, readPath)
	rootValue := typevalue.FromType(reg, typ.Any)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(rawSym), rootValue).
		WritePathKey(reg, resolver.KeySpace(), childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("raw.id evidence = %s, want %s", gotEvidence, evidence.ExplicitTop())
	}
}

func TestProjectExactAbsentReturnsNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2)
	resolver := testResolver(point, symbol.ID(11), "t")
	readPath := path.NewPath(symbol.ID(11), "t").IndexStr("missing")
	childKey := resolver.KeyAt(point, readPath)
	in := state.State{}.WritePathKey(reg, resolver.KeySpace(), childKey, product.Absent(reg))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Absent())
}

func TestProjectUsesHeapIdentityMemberForAliasedRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	sym := symbol.ID(21)
	resolver := testResolver(point, sym, "alias")
	rootPath := path.NewPath(sym, "alias")
	readPath := rootPath.Field("id")
	id := identity.LuaTableLiteral(7002, 211)
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	memberValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	ks := resolver.KeySpace()
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: heapStaticMembers(ks, segment.Segment{Kind: segment.SegmentField, Name: "id"}, memberValue),
		}))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectUsesHeapIdentityRootWitnessWhenStaticMemberLaneIsEmpty(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	sym := symbol.ID(211)
	resolver := testResolver(point, sym, "alias")
	rootPath := path.NewPath(sym, "alias")
	readPath := rootPath.Field("id")
	id := identity.LuaTableLiteral(7002, 215)
	symbolValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	rootValue := typevalue.WithWitness(reg, symbolValue, typetable.NewRecord().Field("id", typ.String).Build())
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), symbolValue).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue,
		}))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectHeapIdentitySuffixDistinguishesFieldAndStringIndex(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	sym := symbol.ID(22)
	resolver := testResolver(point, sym, "obj")
	rootPath := path.NewPath(sym, "obj")
	id := identity.LuaTableLiteral(7002, 212)
	rootValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	fieldValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	indexValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	ks := resolver.KeySpace()
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: rootValue,
			StaticMembers: mergeHeapStaticMembers(
				heapStaticMembers(ks, segment.Segment{Kind: segment.SegmentField, Name: "id"}, fieldValue),
				heapStaticMembers(ks, segment.Segment{Kind: segment.SegmentIndexString, Name: "id"}, indexValue),
			),
		}))

	fieldRead, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath.Field("id"), in)
	if !ok {
		t.Fatal("field Project returned false")
	}
	indexRead, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath.IndexStr("id"), in)
	if !ok {
		t.Fatal("index Project returned false")
	}
	assertRuntimeKind(t, reg, fieldRead, runtimekind.Singleton(runtimekind.String))
	assertRuntimeKind(t, reg, indexRead, runtimekind.Singleton(runtimekind.Number))
}

func TestProjectNoExactProofKeepsRuntimeIndexOptionality(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(3)
	resolver := testResolver(point, symbol.ID(12), "t")
	readPath := path.NewPath(symbol.ID(12), "t").IndexInt(1)
	parentValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(12)), parentValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Top())
}

func TestProjectInRangeStructuralArrayIndexDropsNil(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	sym := symbol.ID(23)
	resolver := testResolver(point, sym, "arr")
	parentPath := path.NewPath(sym, "arr")
	readPath := parentPath.IndexInt(2)
	rootValue := typevalue.WithWitness(reg, product.Top(), typ.NewArray(typ.String))
	parentKey, parentKeyOK := resolver.StateKeyAt(point, parentPath)
	if !parentKeyOK {
		t.Fatal("StateKeyAt(parent) failed")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteLenFloor(resolver.KeySpace(), parentKey, 2)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectLenFloorPrunesImpossibleEmptyUnionArmForArrayIndex(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	sym := symbol.ID(27)
	resolver := testResolver(point, sym, "parts")
	parentPath := path.NewPath(sym, "parts")
	readPath := parentPath.IndexInt(2)
	parentType := typeexpr.Union(
		typetable.NewRecord().Build(),
		typ.NewArray(typ.String),
	)
	if !definitelyInBoundsIndexContainerTypeAtFloor(parentType, 2, 2, 0) {
		t.Fatalf("type predicate rejected %s", parentType.String())
	}
	rootValue := typevalue.WithWitness(reg, product.Top(), parentType)
	parentKey, parentKeyOK := resolver.StateKeyAt(point, parentPath)
	if !parentKeyOK {
		t.Fatal("StateKeyAt(parent) failed")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteLenFloor(resolver.KeySpace(), parentKey, 2)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
}

func TestProjectLenFloorDoesNotDropNilForIntegerMapIndex(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	sym := symbol.ID(28)
	resolver := testResolver(point, sym, "lookup")
	parentPath := path.NewPath(sym, "lookup")
	readPath := parentPath.IndexInt(2)
	rootValue := typevalue.WithWitness(reg, product.Top(), typetable.NewMap(typ.Integer, typ.String))
	parentKey, parentKeyOK := resolver.StateKeyAt(point, parentPath)
	if !parentKeyOK {
		t.Fatal("StateKeyAt(parent) failed")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteLenFloor(resolver.KeySpace(), parentKey, 2)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectLenFloorKeepsNilForOutOfRangeArrayIndex(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	sym := symbol.ID(29)
	resolver := testResolver(point, sym, "arr")
	parentPath := path.NewPath(sym, "arr")
	readPath := parentPath.IndexInt(3)
	rootValue := typevalue.WithWitness(reg, product.Top(), typ.NewArray(typ.String))
	parentKey, parentKeyOK := resolver.StateKeyAt(point, parentPath)
	if !parentKeyOK {
		t.Fatal("StateKeyAt(parent) failed")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteLenFloor(resolver.KeySpace(), parentKey, 2)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectLenFloorKeepsNilForZeroArrayIndex(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	sym := symbol.ID(30)
	resolver := testResolver(point, sym, "arr")
	parentPath := path.NewPath(sym, "arr")
	readPath := parentPath.IndexInt(0)
	rootValue := typevalue.WithWitness(reg, product.Top(), typ.NewArray(typ.String))
	parentKey, parentKeyOK := resolver.StateKeyAt(point, parentPath)
	if !parentKeyOK {
		t.Fatal("StateKeyAt(parent) failed")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(sym), rootValue).
		WriteLenFloor(resolver.KeySpace(), parentKey, 2)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectNoExactProofUsesNarrowedParentOrigin(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	result := symbol.ID(17)
	resolver := testResolver(point, result, "result")
	readPath := path.NewPath(result, "result").Field("value")
	intCase := typetable.NewRecord().
		Field("channel", typ.NewAlias("__test_ChanInt", typetable.NewRecord().Field("__tag", typ.LiteralString("int")).Build())).
		Field("value", typ.Number).
		Build()
	chanStr := typ.NewAlias("__test_ChanStr", typetable.NewRecord().Field("__tag", typ.LiteralString("str")).Build())
	strCase := typetable.NewRecord().
		Field("channel", chanStr).
		Field("value", typ.String).
		Build()
	union := typeexpr.Union(intCase, strCase)
	rootFamily, rootCases, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("missing root origin")
	}
	constraintFamily, constraintCases, ok := variant.OriginOfType(chanStr)
	if !ok {
		t.Fatal("missing channel origin")
	}
	strCases, ok := variant.NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}, constraintFamily, constraintCases, true)
	if !ok {
		t.Fatal("failed to narrow root origin")
	}
	rootValue := product.Set(reg, typevalue.FromType(reg, union), variantorigin.Key, variantorigin.Of(rootFamily, strCases))
	in := state.State{}.WriteValue(reg, key.SymbolValue(result), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectNestedPathUsesNarrowedRootOrigin(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	result := symbol.ID(18)
	resolver := testResolver(point, result, "result")
	readPath := path.NewPath(result, "result").Field("value").Field("error")
	errChan := typ.NewAlias("__test_ChanErr", typetable.NewRecord().Field("__tag", typ.LiteralString("err")).Build())
	okChan := typ.NewAlias("__test_ChanOK", typetable.NewRecord().Field("__tag", typ.LiteralString("ok")).Build())
	errCase := typetable.NewRecord().
		Field("channel", errChan).
		Field("value", typetable.NewRecord().Field("error", typ.String).Build()).
		Build()
	okCase := typetable.NewRecord().
		Field("channel", okChan).
		Field("value", typetable.NewRecord().Field("data", typ.Number).Build()).
		Build()
	union := typeexpr.Union(okCase, errCase)
	rootFamily, rootCases, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("missing root origin")
	}
	errFamily, errCases, ok := variant.OriginOfType(errChan)
	if !ok {
		t.Fatal("missing channel origin")
	}
	narrowedCases, ok := variant.NarrowOriginByPath(rootFamily, rootCases, []segment.Segment{{Kind: segment.SegmentField, Name: "channel"}}, errFamily, errCases, true)
	if !ok {
		t.Fatal("failed to narrow root origin")
	}
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	rootValue = product.Set(reg, rootValue, variantorigin.Key, variantorigin.Of(rootFamily, narrowedCases))
	in := state.State{}.WriteValue(reg, key.SymbolValue(result), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectUsesOriginTypeWhenWitnessFamilyDoesNotReplay(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	petSym := symbol.ID(22)
	resolver := testResolver(point, petSym, "pet")
	readPath := path.NewPath(petSym, "pet").Field("bark")
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.String).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.String).
		Build()
	union := typeexpr.Union(dog, cat)
	dogFamily, dogCases, ok := variant.OriginOfType(dog)
	if !ok {
		t.Fatal("missing dog origin")
	}
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	rootValue = product.Set(reg, rootValue, variantorigin.Key, variantorigin.Of(dogFamily, dogCases))
	in := state.State{}.WriteValue(reg, key.SymbolValue(petSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectExactPresentChildUsesGenericVariantRootArm(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	resultSym := symbol.ID(23)
	resolver := testResolver(point, resultSym, "result")
	resultPath := path.NewPath(resultSym, "result")
	readPath := resultPath.Field("value")
	childKey := resolver.KeyAt(point, readPath)
	tp := typ.NewTypeParam("T", nil)
	ep := typ.NewTypeParam("E", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp, ep}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", tp).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", ep).
			Build(),
	))
	concrete := typ.Instantiate(result, typ.LiteralInt(41), typ.String)
	okFamily, okCases, ok := variant.OriginByPathLiteral(concrete, []segment.Segment{{Kind: segment.SegmentField, Name: "ok"}}, typ.LiteralBool(true))
	if !ok {
		t.Fatal("missing concrete Result ok-arm origin")
	}
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, concrete), concrete)
	rootValue = product.Set(reg, rootValue, variantorigin.Key, variantorigin.Of(okFamily, okCases))
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(resultSym), rootValue).
		WritePathKey(reg, resolver.KeySpace(), childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.LiteralInt(41)) {
		t.Fatalf("Project type = %v/%v, want 41", gotType, ok)
	}
}

func TestProjectPresentRootWitnessMakesNestedPathNonOptional(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	responseSym := symbol.ID(20)
	resolver := testResolver(point, responseSym, "response")
	responsePath := path.NewPath(responseSym, "response")
	readPath := responsePath.Field("metadata").Field("response_id")
	responseType := typetable.NewRecord().
		Field("metadata", typetable.NewRecord().
			Field("response_id", typ.String).
			Build()).
		Build()
	rootType := typeexpr.Optional(responseType)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType)
	rootValue = product.WithPresence(reg, rootValue, presence.Present())
	in := state.State{}.WriteValue(reg, key.SymbolValue(responseSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectPresentIntermediateWitnessMakesRequiredChildNonOptional(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	ctxSym := symbol.ID(22)
	resolver := testResolver(point, ctxSym, "ctx")
	ctxPath := path.NewPath(ctxSym, "ctx")
	sessionPath := ctxPath.Field("session")
	readPath := sessionPath.Field("user_id")
	sessionKey := resolver.KeyAt(point, sessionPath)
	sessionType := typetable.NewRecord().
		Field("id", typ.String).
		Field("user_id", typ.String).
		Field("scopes", typetable.NewMap(typ.String, typ.Boolean)).
		OptField("last_seen", typetable.NewRecord().
			Field("unix", typ.Func().Returns(typ.Integer).Build()).
			Build()).
		OptField("attributes", typetable.NewMap(typ.String, typ.String)).
		Build()
	ctxType := typetable.NewRecord().
		Field("request", typetable.NewRecord().
			Field("kind", typ.LiteralString("http")).
			Field("method", typeexpr.Union(typ.LiteralString("GET"), typ.LiteralString("POST"))).
			Field("path", typ.String).
			Field("headers", typetable.NewMap(typ.String, typ.String)).
			OptField("params", typetable.NewMap(typ.String, typ.String)).
			OptField("body", typ.String).
			Field("meta", typetable.NewRecord().
				Field("trace_id", typ.String).
				OptField("tags", typetable.NewMap(typ.String, typ.String)).
				Build()).
			Build()).
		Field("params", typetable.NewMap(typ.String, typ.String)).
		Field("locals", typetable.NewMap(typ.String, typ.String)).
		OptField("session", sessionType).
		Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, ctxType), ctxType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(ctxSym), rootValue).
		WritePathKey(reg, resolver.KeySpace(), sessionKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectBranchPresenceProofMakesOptionalFieldPresent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	pageSym := symbol.ID(41)
	resolver := testResolver(point, pageSym, "page")
	pagePath := path.NewPath(pageSym, "page")
	readPath := pagePath.Field("data_func")
	readKey := resolver.KeyAt(point, readPath)
	stateKey, ok := resolver.KeySpace().FromPathKey(readKey)
	if !ok {
		t.Fatalf("FromPathKey(%q) failed", readKey)
	}
	pageType := typetable.NewRecord().
		OptField("data_func", typ.String).
		Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, pageType), pageType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(pageSym), rootValue).
		AddBranchProof(pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     stateKey,
			Presence: presence.Present(),
		})

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectExactParentMissingFieldReturnsNilForExclusivePlacement(t *testing.T) {
	cases := []struct {
		name    string
		place   placement.Value
		wantNil bool
	}{
		{name: "stack", place: placement.Stack, wantNil: true},
		{name: "owned-heap", place: placement.OwnedHeap, wantNil: true},
		{name: "shared-heap", place: placement.SharedHeap, wantNil: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := standard.Registry()
			point := cfg.Point(120)
			dogsSym := symbol.ID(120)
			resolver := testResolver(point, dogsSym, "dogs")
			rootPath := path.NewPath(dogsSym, "dogs")
			parentPath := rootPath.IndexInt(1)
			readPath := parentPath.Field("breed")
			parentKey := resolver.KeyAt(point, parentPath)
			dogType := typetable.NewRecord().
				Field("name", typ.String).
				Field("breed", typ.String).
				Build()
			catType := typetable.NewRecord().
				Field("name", typ.String).
				Build()
			catID := identity.LuaTableLiteral(120, 1)
			rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.NewArray(dogType)), typ.NewArray(dogType))
			parentValue := typevalue.WithWitness(reg, typevalue.FromType(reg, catType), catType)
			parentValue = product.Set(reg, parentValue, identity.Key, identity.Singleton(catID))
			in := state.State{}.
				WriteValue(reg, key.SymbolValue(dogsSym), rootValue).
				WritePathKey(reg, resolver.KeySpace(), parentKey, parentValue).
				WritePlacement(catID, tc.place)

			got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
			if !ok {
				t.Fatal("Project returned false")
			}
			gotType, ok := typevalue.TypeOf(reg, got)
			isNil := ok && typ.TypeEquals(gotType, typ.Nil)
			if tc.wantNil && !isNil {
				t.Fatalf("projected type = %v/%v, want nil", gotType, ok)
			}
			if !tc.wantNil && isNil {
				t.Fatalf("projected type = nil for %s; shared placement must not prove an absent slot", tc.place)
			}
		})
	}
}

func TestProjectMaybeRootWitnessKeepsChildOptional(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	responseSym := symbol.ID(21)
	resolver := testResolver(point, responseSym, "response")
	responsePath := path.NewPath(responseSym, "response")
	readPath := responsePath.Field("answer")
	responseType := typetable.NewRecord().Field("answer", typ.String).Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, responseType), responseType)
	rootValue = product.WithPresence(reg, rootValue, presence.Maybe())
	in := state.State{}.WriteValue(reg, key.SymbolValue(responseSym), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Maybe())
	assertRuntimeKind(t, reg, got, runtimekind.Singleton(runtimekind.String))
}

func TestProjectRootIdentifierReadsSymbolState(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6)
	sym := symbol.ID(16)
	readPath := path.NewPath(sym, "x")
	want := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.String),
	)
	in := state.State{}.WriteValue(reg, key.SymbolValue(sym), want)

	got, ok := Project(Config{Registry: reg}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	if !product.Equal(reg, got, want) {
		t.Fatalf("Project root value = %v, want %v", got, want)
	}
}

func TestProjectMemberOfExplicitTopRootCarriesExplicitTopEvidence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	raw := symbol.ID(21)
	resolver := testResolver(point, raw, "raw")
	readPath := path.NewPath(raw, "raw").Field("id")
	rootValue := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	in := state.State{}.WriteValue(reg, key.SymbolValue(raw), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("projected evidence = %s, want explicit-top", gotEvidence)
	}
}

func TestProjectRejectsKnownNonTableParent(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4)
	resolver := testResolver(point, symbol.ID(13), "t")
	readPath := path.NewPath(symbol.ID(13), "t").Field("name")
	parentValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(13)), parentValue)

	if got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, readPath, in); ok {
		t.Fatalf("Project = %v/true, want false", got)
	}
}

func TestProjectChildProofDoesNotProveParentAggregate(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(5)
	resolver := testResolver(point, symbol.ID(14), "t")
	parentPath := path.NewPath(symbol.ID(14), "t")
	childPath := parentPath.Field("ready")
	childKey := resolver.KeyAt(point, childPath)
	parentKey := key.SymbolValue(symbol.ID(14))
	parentValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.
		WriteValue(reg, parentKey, parentValue).
		WritePathKey(reg, resolver.KeySpace(), childKey, product.NewWithPresence(reg, product.ShapeTop, presence.Present()))

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, childPath, in)
	if !ok {
		t.Fatalf("Project returned false")
	}
	assertPresence(t, reg, got, presence.Present())
	assertRuntimeKind(t, reg, in.ReadValue(reg, parentKey), runtimekind.Singleton(runtimekind.String))
	root, ok := Project(Config{Registry: reg, Visibility: resolver}, point, parentPath, in)
	if !ok {
		t.Fatalf("root aggregate read returned false")
	}
	assertRuntimeKind(t, reg, root, runtimekind.Singleton(runtimekind.String))
}

func TestProjectRootOverlaysCurrentStaticMemberWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	provider := symbol.ID(24)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	memberPath := rootPath.IndexInt(1)
	memberKey := resolver.KeyAt(point, memberPath)
	memberType := typ.Func().Param("payload", typ.Number).Build()
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, memberType), memberType)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typetable.NewRecord().Build()), typetable.NewRecord().Build())
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), memberKey, memberValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath, in)
	if !ok {
		t.Fatal("Project root returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatal("Project root did not carry a type witness")
	}
	callable, _, ok := typecall.IndexedMemberCallable(gotType, typ.LiteralInt(1))
	if !ok || callable == nil || len(callable.Params) != 1 || !typ.TypeEquals(callable.Params[0].Type, typ.Number) {
		t.Fatalf("root static member callable = %#v/%v, want one number parameter", callable, ok)
	}
}

func TestProjectWithoutRootStaticMemberOverlayKeepsBaseWitness(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(141)
	provider := symbol.ID(241)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	memberPath := rootPath.Field("run")
	memberKey := resolver.KeyAt(point, memberPath)
	memberType := typ.Func().Param("payload", typ.Number).Build()
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, memberType), memberType)
	baseType := typetable.NewRecord().Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, baseType), baseType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), memberKey, memberValue)

	got, ok := ProjectWithoutRootStaticMemberOverlay(Config{Registry: reg, Visibility: resolver}, point, rootPath, in)
	if !ok {
		t.Fatal("ProjectWithoutRootStaticMemberOverlay returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, baseType) {
		t.Fatalf("root type = %v/%v, want base witness %v without static-member overlay", gotType, ok, baseType)
	}
	if callable, _, ok := typecall.MemberCallable(gotType, "run"); ok || callable != nil {
		t.Fatalf("no-overlay root unexpectedly resolved run member: %#v", callable)
	}
}

func TestProjectStaticMemberReadsOneMemberWithoutRootOverlay(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(142)
	provider := symbol.ID(242)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	member := segment.Segment{Kind: segment.SegmentField, Name: "run"}
	memberKey := resolver.KeyAt(point, rootPath.AppendSegments([]segment.Segment{member}))
	memberType := typ.Func().Param("payload", typ.Number).Build()
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, memberType), memberType)
	baseType := typetable.NewRecord().Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, baseType), baseType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), memberKey, memberValue)

	got, ok := ProjectStaticMember(Config{Registry: reg, Visibility: resolver}, point, rootPath, member, in)
	if !ok {
		t.Fatal("ProjectStaticMember returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, memberType) {
		t.Fatalf("static member type = %v/%v, want %v", gotType, ok, memberType)
	}
	rootGot, ok := ProjectWithoutRootStaticMemberOverlay(Config{Registry: reg, Visibility: resolver}, point, rootPath, in)
	if !ok {
		t.Fatal("ProjectWithoutRootStaticMemberOverlay returned false")
	}
	rootType, ok := typevalue.TypeOf(reg, rootGot)
	if !ok || !typ.TypeEquals(rootType, baseType) {
		t.Fatalf("root type = %v/%v, want base witness untouched by static-member lookup", rootType, ok)
	}
}

func TestProjectStaticMemberUsesExactCurrentPathValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(143)
	provider := symbol.ID(243)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	member := segment.Segment{Kind: segment.SegmentField, Name: "run"}
	memberPath := rootPath.AppendSegments([]segment.Segment{member})
	memberKey := resolver.KeyAt(point, memberPath)
	staticType := typ.Func().Param("payload", typ.Number).Build()
	staticValue := typevalue.WithWitness(reg, typevalue.FromType(reg, staticType), staticType)
	currentType := typ.Func().Param("payload", typ.String).Build()
	currentValue := typevalue.WithWitness(reg, typevalue.FromType(reg, currentType), currentType)
	rootType := typetable.NewRecord().Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), memberKey, staticValue).
		WritePathKey(reg, resolver.KeySpace(), memberKey, currentValue)

	got, ok := ProjectStaticMember(Config{Registry: reg, Visibility: resolver}, point, rootPath, member, in)
	if !ok {
		t.Fatal("ProjectStaticMember returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, currentType) {
		t.Fatalf("static member type = %v/%v, want current path type %v", gotType, ok, currentType)
	}
}

func TestProjectStaticMemberKeepsFallbackWhenCurrentPathOnlyHasIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(144)
	provider := symbol.ID(244)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	member := segment.Segment{Kind: segment.SegmentField, Name: "run"}
	memberPath := rootPath.AppendSegments([]segment.Segment{member})
	memberKey := resolver.KeyAt(point, memberPath)
	staticType := typ.Func().Param("payload", typ.Number).Build()
	staticValue := typevalue.WithWitness(reg, typevalue.FromType(reg, staticType), staticType)
	currentValue := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		identity.Key,
		identity.Singleton(identity.ID{Kind: "path", Site: "current", Index: 1}),
	)
	rootType := typetable.NewRecord().Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), memberKey, staticValue).
		WritePathKey(reg, resolver.KeySpace(), memberKey, currentValue)

	got, ok := ProjectStaticMember(Config{Registry: reg, Visibility: resolver}, point, rootPath, member, in)
	if !ok {
		t.Fatal("ProjectStaticMember returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, staticType) {
		t.Fatalf("static member type = %v/%v, want fallback type %v", gotType, ok, staticType)
	}
}

func TestProjectMemberReadsStaticMemberWhenRootWitnessIsDataOnly(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(145)
	provider := symbol.ID(245)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	memberPath := rootPath.Field("run")
	memberKey := resolver.KeyAt(point, memberPath)
	memberType := typ.Func().Param("payload", typ.Number).Build()
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, memberType), memberType)
	rootType := typetable.NewRecord().
		Field("status", typ.String).
		Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), memberKey, memberValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, memberPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, memberType) {
		t.Fatalf("projected member type = %v/%v, want static member %v", gotType, ok, memberType)
	}
}

func TestProjectPrefersReassignedParentStaticMemberOverStaleRootMember(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1451)
	container := symbol.ID(2451)
	resolver := testResolver(point, container, "container")
	containerPath := path.NewPath(container, "container")
	clientPath := containerPath.Field("client")
	metaPath := clientPath.Field("meta")
	containerID := identity.ID{Kind: "test.table", Site: "container", Index: 1}
	replacementID := identity.ID{Kind: "test.table", Site: "replacement", Index: 1}
	containerValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(containerID))
	replacementValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(replacementID))
	staleType := typ.Func().Returns(typ.String).Build()
	freshType := typ.Func().Returns(typ.Number).Build()
	staleValue := typevalue.WithWitness(reg, typevalue.FromType(reg, staleType), staleType)
	freshValue := typevalue.WithWitness(reg, typevalue.FromType(reg, freshType), freshType)
	staleKey, ok := heapidentity.StaticMemberSuffixKey(resolver.KeySpace(), []segment.Segment{
		{Kind: segment.SegmentField, Name: "client"},
		{Kind: segment.SegmentField, Name: "meta"},
	})
	if !ok {
		t.Fatal("missing stale suffix key")
	}
	freshKey, ok := heapidentity.StaticMemberSuffixKey(resolver.KeySpace(), []segment.Segment{{Kind: segment.SegmentField, Name: "meta"}})
	if !ok {
		t.Fatal("missing fresh suffix key")
	}
	clientKey := resolver.KeyAt(point, clientPath)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(container), containerValue).
		WritePathKey(reg, resolver.KeySpace(), clientKey, replacementValue).
		WriteHeapTableObject(reg, containerID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          containerValue,
			StaticMembers: map[keyspace.Key]product.Value{staleKey: staleValue},
		})).
		WriteHeapTableObject(reg, replacementID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          replacementValue,
			StaticMembers: map[keyspace.Key]product.Value{freshKey: freshValue},
		}))

	config := Config{Registry: reg, Visibility: resolver, TypeValues: typevalue.NewCache()}
	parentGot, parentOK := Project(config, point, clientPath, in)
	if !parentOK {
		t.Fatal("parent Project returned false")
	}
	if parentID, ok := product.Get(reg, parentGot, identity.Key).ID(); !ok || parentID != replacementID {
		t.Fatalf("parent identity = %s/%v, want replacement %s", parentID, ok, replacementID)
	}
	got, ok := Project(config, point, metaPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, freshType) {
		t.Fatalf("projected type = %v/%v, want reassigned parent member %v", gotType, ok, freshType)
	}
}

func TestProjectMemberReadsStableStaticMemberAcrossVisibleVersions(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(146)
	provider := symbol.ID(246)
	resolver := testResolver(point, provider, "provider")
	rootPath := path.NewPath(provider, "provider")
	memberPath := rootPath.Field("run")
	memberType := typ.Func().Param("payload", typ.Number).Build()
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, memberType), memberType)
	stableKey, ok := resolver.KeySpace().FromStableSymbol(provider, memberPath.Segments)
	if !ok {
		t.Fatal("stable key failed")
	}
	rootType := typetable.NewRecord().
		Field("status", typ.String).
		Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, rootType), rootType)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WriteLocalPathStaticMember(stableKey, memberValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, memberPath, in)
	if !ok {
		t.Fatal("Project returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, memberType) {
		t.Fatalf("projected member type = %v/%v, want stable static member %v", gotType, ok, memberType)
	}
}

func TestProjectRootStaticMemberOverlayDoesNotEraseUnionRoot(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	result := symbol.ID(29)
	resolver := testResolver(point, result, "result")
	rootPath := path.NewPath(result, "result")
	errorPath := rootPath.Field("error")
	errorKey := resolver.KeyAt(point, errorPath)
	okCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", typ.String).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	union := typeexpr.Union(okCase, errorCase)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	errorValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(result), rootValue).
		WritePathStaticMember(resolver.KeySpace(), errorKey, errorValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath, in)
	if !ok {
		t.Fatal("Project root returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, union) {
		t.Fatalf("root type = %v/%v, want original union", gotType, ok)
	}
}

func TestProjectUnionBooleanDiscriminantAdmitsBothEdges(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	result := symbol.ID(30)
	resolver := testResolver(point, result, "result")
	rootPath := path.NewPath(result, "result")
	okPath := rootPath.Field("ok")
	okCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", typ.String).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	union := typeexpr.Union(okCase, errorCase)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	in := state.State{}.WriteValue(reg, key.SymbolValue(result), rootValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, okPath, in)
	if !ok {
		t.Fatal("Project ok returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typeexpr.Union(typ.True, typ.False)) {
		t.Fatalf("ok type = %v/%v, want true|false", gotType, ok)
	}
}

func TestProjectRootStaticMemberWitnessRecursivelyOverlaysDeclaredRecord(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	actor := symbol.ID(27)
	resolver := testResolver(point, actor, "actor")
	rootPath := path.NewPath(actor, "actor")
	lastIDPath := rootPath.Field("state").Field("last_id")
	lastIDKey := resolver.KeyAt(point, lastIDPath)
	stateType := typetable.NewRecord().
		Field("processed", typetable.NewMap(typ.String, typ.String)).
		Field("counters", typetable.NewMap(typ.String, typ.Number)).
		OptField("last_id", typ.String).
		Build()
	actorType := typetable.NewRecord().
		Field("state", stateType).
		Field("id", typ.String).
		Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, actorType), actorType)
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(actor), rootValue).
		WritePathStaticMember(resolver.KeySpace(), lastIDKey, memberValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, rootPath, in)
	if !ok {
		t.Fatal("Project root returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatal("Project root did not carry a type witness")
	}
	projectedState, ok := access.Field(gotType, "state")
	if !ok {
		t.Fatalf("projected actor has no state field: %v", gotType)
	}
	if _, ok := access.Field(projectedState, "processed"); !ok {
		t.Fatalf("projected state lost processed sibling: %v", projectedState)
	}
	if _, ok := access.Field(projectedState, "counters"); !ok {
		t.Fatalf("projected state lost counters sibling: %v", projectedState)
	}
	lastID, ok := access.Field(projectedState, "last_id")
	if !ok || !typ.TypeEquals(lastID, typ.String) {
		t.Fatalf("projected state last_id = %v/%v, want string", lastID, ok)
	}
}

func TestProjectNestedAggregateStaticMemberWitnessPreservesDeclaredSiblings(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	actor := symbol.ID(28)
	resolver := testResolver(point, actor, "actor")
	rootPath := path.NewPath(actor, "actor")
	statePath := rootPath.Field("state")
	lastIDPath := statePath.Field("last_id")
	lastIDKey := resolver.KeyAt(point, lastIDPath)
	stateType := typetable.NewRecord().
		Field("processed", typetable.NewMap(typ.String, typ.String)).
		Field("counters", typetable.NewMap(typ.String, typ.Number)).
		OptField("last_id", typ.String).
		Build()
	actorType := typetable.NewRecord().
		Field("state", stateType).
		Field("id", typ.String).
		Build()
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, actorType), actorType)
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(actor), rootValue).
		WritePathStaticMember(resolver.KeySpace(), lastIDKey, memberValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, point, statePath, in)
	if !ok {
		t.Fatal("Project nested aggregate returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatal("Project nested aggregate did not carry a type witness")
	}
	if _, ok := access.Field(gotType, "processed"); !ok {
		t.Fatalf("projected state lost processed sibling: %v", gotType)
	}
	if _, ok := access.Field(gotType, "counters"); !ok {
		t.Fatalf("projected state lost counters sibling: %v", gotType)
	}
	lastID, ok := access.Field(gotType, "last_id")
	if !ok || !typ.TypeEquals(lastID, typ.String) {
		t.Fatalf("projected state last_id = %v/%v, want string", lastID, ok)
	}
}

func TestProjectRootStaticMemberWitnessRequiresCurrentVisibleVersion(t *testing.T) {
	reg := standard.Registry()
	oldPoint := cfg.Point(15)
	currentPoint := cfg.Point(16)
	provider := symbol.ID(25)
	builder := visibility.NewBuilder()
	builder.Define(oldPoint, provider, "provider")
	builder.Define(currentPoint, provider, "provider")
	resolver := visibility.NewResolver(builder.Build())
	rootPath := path.NewPath(provider, "provider")
	staleMemberKey := resolver.KeyAt(oldPoint, rootPath.IndexInt(1))
	memberType := typ.Func().Param("payload", typ.Number).Build()
	memberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, memberType), memberType)
	rootValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typetable.NewRecord().Build()), typetable.NewRecord().Build())
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(provider), rootValue).
		WritePathStaticMember(resolver.KeySpace(), staleMemberKey, memberValue)

	got, ok := Project(Config{Registry: reg, Visibility: resolver}, currentPoint, rootPath, in)
	if !ok {
		t.Fatal("Project root returned false")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatal("Project root did not carry its original type witness")
	}
	if callable, _, ok := typecall.IndexedMemberCallable(gotType, typ.LiteralInt(1)); ok || callable != nil {
		t.Fatalf("stale version static member leaked into current root: %#v", callable)
	}
}

func testResolver(point cfg.Point, sym symbol.ID, root string) *visibility.Resolver {
	builder := visibility.NewBuilder()
	builder.Define(point, sym, root)
	return visibility.NewResolver(builder.Build())
}

func assertPresence(t *testing.T, reg *axis.Registry, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, want) {
		t.Fatalf("runtimekind = %s, want %s", gotKind, want)
	}
}

func heapStaticMembers(ks *keyspace.KeySpace, suffix segment.Segment, value product.Value) map[keyspace.Key]product.Value {
	key, ok := ks.FromRootlessSuffix([]segment.Segment{suffix})
	if !ok {
		panic("heapStaticMembers: failed to build key")
	}
	return map[keyspace.Key]product.Value{key: value}
}

func mergeHeapStaticMembers(maps ...map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	out := make(map[keyspace.Key]product.Value)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
