package state

import (
	"context"
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func applyAllocationIdentityQuotientTupleForTest(
	t *testing.T,
	domain ProductDomain,
	keys *keyspace.KeySpace,
	authority *BoundaryAllocationAuthority,
	input State,
) State {
	t.Helper()
	input = domain.Normalize(input)
	residual, values := DecomposeValueLane(domain.Lattice(), input)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	values, factors, err = ApplyAllocationIdentityQuotientTuple(context.Background(), domain, keys, authority, values, factors)
	if err != nil {
		t.Fatal(err)
	}
	out, err := domain.ComposeFactorTuple(values, factors)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAllocationIdentityQuotientTupleMatchesWholeStateAndNoTemplateIdentity(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-tuple")))
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	input := domain.Lattice().Bottom().WriteValue(
		reg, keySymbolValueForAllocationQuotientTest(), identityvalue.PresentTerm(reg, identity.AllocationTerm(template)),
	)
	want, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input)
	if err != nil {
		t.Fatal(err)
	}
	if got := applyAllocationIdentityQuotientTupleForTest(t, domain, keys, authority, input); !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native allocation quotient diverged from the whole-State transaction")
	}

	stableID := identity.ID{Kind: "allocation-tuple-test", Site: "stable", Index: 1}
	stable := domain.Lattice().Bottom().WriteValue(reg, keySymbolValueForAllocationQuotientTest(), identityvalue.Present(reg, stableID))
	residual, values := DecomposeValueLane(domain.Lattice(), stable)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	nextValues, nextFactors, err := ApplyAllocationIdentityQuotientTuple(context.Background(), domain, keys, nil, values, factors)
	if err != nil {
		t.Fatal(err)
	}
	if !ValueFactorLattice[key.Value](reg).Same(values, nextValues) {
		t.Fatal("template-free tuple did not preserve the Values representation")
	}
	for index := range factors {
		equal, equalErr := domain.LaneEqual(factors[index], nextFactors[index])
		if equalErr != nil || !equal {
			t.Fatalf("template-free tuple changed lane %d: equal=%v err=%v", index, equal, equalErr)
		}
	}

	_, _, err = ApplyAllocationIdentityQuotientTuple(context.Background(), domain, keys, nil,
		func() ValueLaneFactor {
			_, templateValues := DecomposeValueLane(domain.Lattice(), domain.Normalize(input))
			return templateValues
		}(),
		func() []LaneFactor {
			templateResidual, _ := DecomposeValueLane(domain.Lattice(), domain.Normalize(input))
			templateFactors, factorErr := domain.DecomposeLanes(templateResidual, domain.NonValuesLaneInventory())
			if factorErr != nil {
				t.Fatal(factorErr)
			}
			return templateFactors
		}(),
	)
	if err == nil {
		t.Fatal("factor-native quotient accepted templates without allocation authority")
	}
}

func TestAllocationIdentityQuotientMustFiberRequiresQuotientBeforeLeafJoin(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := RegisteredProductDomain(reg)
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-quotient-before-leaf-join")))
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, exact := authority.RebaseAllocation(template)
	if !exact {
		t.Fatal("allocation image")
	}
	actualTerm := identity.ConcreteTerm(actual)
	slot := keySymbolValueForAllocationQuotientTest()
	leaf := func(term identity.Term) State {
		out := domain.Lattice().Bottom().WriteValue(reg, slot, identityvalue.PresentTerm(reg, term))
		out.frozenTables, _ = out.frozenTables.freezeTerm(term)
		return out
	}
	left, right := leaf(templateTerm), leaf(actualTerm)

	// Publication quotients each complete correlated leaf before it forgets
	// correlation through the componentwise existential join.
	want := domain.Lattice().Join(
		applyAllocationIdentityQuotientTupleForTest(t, domain, keys, authority, left),
		applyAllocationIdentityQuotientTupleForTest(t, domain, keys, authority, right),
	)
	if !want.IsTableFrozen(actual) {
		t.Fatal("per-leaf quotient lost a must proof held in every correlated world")
	}

	// Quotient is deliberately not a join homomorphism: joining the two source
	// must sets first loses both distinct preimages before their collision is
	// known. This regression locks the ordering used by fused publication.
	joinedFirst := applyAllocationIdentityQuotientTupleForTest(t, domain, keys, authority, domain.Lattice().Join(left, right))
	if joinedFirst.IsTableFrozen(actual) {
		t.Fatal("join-before-quotient unexpectedly retained the collision must proof")
	}
	if domain.Lattice().Equal(joinedFirst, want) {
		t.Fatal("allocation quotient incorrectly behaved as a join homomorphism")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	residual, values := DecomposeValueLane(domain.Lattice(), domain.Normalize(left))
	factors, factorErr := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if factorErr != nil {
		t.Fatal(factorErr)
	}
	_, _, err = ApplyAllocationIdentityQuotientTuple(ctx, domain, keys, authority, values, factors)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled factor quotient error = %v", err)
	}
}

func TestAllocationIdentityQuotientPreservesCompleteLaneProduct(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := Domain(reg)

	// The state-law inventory has exactly one non-bottom representative for
	// every registered lane. Keeping that inventory as the fixture makes this
	// test grow with the product instead of maintaining another axis list here.
	full := Reachable(State{})
	samples := stateLawLaneSamples(reg, keys)
	if len(samples) != len(DefaultLanes()) {
		t.Fatalf("state-law inventory = %d, enabled lanes = %d", len(samples), len(DefaultLanes()))
	}
	for _, sample := range samples {
		full = domain.Join(full, sample.state)
	}

	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-all-lanes"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok || !identity.IsRootBoundaryAllocation(actual) {
		t.Fatalf("root allocation image = %#v/%v", actual, ok)
	}
	slot := keySymbolValueForAllocationQuotientTest()
	input := full.WriteValue(reg, slot, identityvalue.PresentTerm(reg, templateTerm))
	want := full.WriteValue(reg, slot, identityvalue.Present(reg, actual))

	got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(got, want) {
		t.Fatal("allocation quotient changed a non-allocation lane or failed to alpha-rename the value")
	}
	factors, err := RegisteredProductDomain(reg).Decompose(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, factor := range factors {
		contains, err := RegisteredProductDomain(reg).LaneContainsAllocationTemplate(factor)
		if err != nil {
			t.Fatal(err)
		}
		if contains {
			t.Fatalf("quotient retained a lexical template in lane %q", factor.Lane().ID())
		}
	}

	stable, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, got)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Equal(stable, got) {
		t.Fatal("same-route allocation quotient did not stabilize")
	}
}

func TestAllocationIdentityQuotientApplyRefsAreFreshAndDeterministic(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-apply-refs"))
	caller := lexicalidentity.RootBody(namespace)
	callee := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	input := State{}.WriteValue(reg, keySymbolValueForAllocationQuotientTest(), identityvalue.PresentTerm(reg, identity.AllocationTerm(template)))

	apply := func(occurrence uint32) (State, identity.ID) {
		t.Helper()
		authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 19, occurrence), []identity.AllocationTemplate{template})
		if err != nil {
			t.Fatal(err)
		}
		actual, ok := authority.RebaseAllocation(template)
		if !ok || !identity.IsBoundaryAllocation(actual) {
			t.Fatalf("Apply allocation image = %#v/%v", actual, ok)
		}
		got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input)
		if err != nil {
			t.Fatal(err)
		}
		return got, actual
	}
	first, firstID := apply(1)
	second, secondID := apply(2)
	if firstID == secondID || Domain(reg).Equal(first, second) {
		t.Fatal("two ApplyRefs shared one allocation identity")
	}
	firstAgain, repeatedID := apply(1)
	if repeatedID != firstID || !Domain(reg).Equal(firstAgain, first) {
		t.Fatal("one ApplyRef did not derive a deterministic route identity")
	}
}

func TestAllocationIdentityQuotientRenamesEveryIdentityBearingLaneLeaf(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	fx := stateLawFixtureFor(reg, keys)
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-bearing-leaves"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation image")
	}

	build := func(term identity.Term) State {
		value := identityvalue.PresentTerm(reg, term)
		dynamic := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: value, HasKeyValue: true, Value: value, HasValue: true,
			Admission: dynamicindex.AdmissionAdmitted,
		})
		object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: value, StaticMembers: map[keyspace.Key]product.Value{fx.staticHeapKey: value},
			DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{fx.dynamicKey: dynamic},
		})
		out := Domain(reg).Bottom().
			WriteValue(reg, keySymbolValueForAllocationQuotientTest(), value).
			WritePathKey(reg, keys, fx.pathKey, value).
			WritePathStaticMember(keys, fx.staticKey, value).
			WriteDynamicIndexFact(reg, fx.dynamicKey, dynamic).
			WriteEffectDelta(fx.effectKey, effectdelta.Value{Before: value, After: value, Change: effectdelta.ChangeChanged}).
			AddChannelSelectFact(channelselectfact.Fact{
				Select: "allocation-identity", Kind: channelselectfact.FactCase,
				Result: mustTestStateKey(fx.pathKey), Case: mustTestStateKey(fx.staticKey),
				Payload: value, HasPayload: true,
			})
		out.heapTableIdentity = out.heapTableIdentity.withTerm(term, object)
		out.frozenTables, _ = out.frozenTables.freezeTerm(term)
		out.placement = out.placement.withTerm(term, placement.Stack)
		return out
	}
	got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, build(templateTerm))
	if err != nil {
		t.Fatal(err)
	}
	if !Domain(reg).Equal(got, build(identity.ConcreteTerm(actual))) {
		t.Fatal("allocation quotient missed an identity-bearing registered lane leaf")
	}
}

func TestAllocationIdentityQuotientDiscoversEveryIdentityBearingLaneInIsolation(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	fx := stateLawFixtureFor(reg, keys)
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-isolated-lanes"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation image")
	}
	value := func(term identity.Term) product.Value { return identityvalue.PresentTerm(reg, term) }
	dynamic := func(term identity.Term) dynamicindex.Fact {
		return dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: value(term), HasKeyValue: true, Value: value(term), HasValue: true,
			Admission: dynamicindex.AdmissionAdmitted,
		})
	}
	tests := []struct {
		name  string
		build func(identity.Term) State
	}{
		{name: "values", build: func(term identity.Term) State {
			return Domain(reg).Bottom().WriteValue(reg, keySymbolValueForAllocationQuotientTest(), value(term))
		}},
		{name: "path-refinement", build: func(term identity.Term) State {
			return Domain(reg).Bottom().WritePathKey(reg, keys, fx.pathKey, value(term))
		}},
		{name: "path-static-member", build: func(term identity.Term) State {
			return Domain(reg).Bottom().WritePathStaticMember(keys, fx.staticKey, value(term))
		}},
		{name: "dynamic-index", build: func(term identity.Term) State {
			return Domain(reg).Bottom().WriteDynamicIndexFact(reg, fx.dynamicKey, dynamic(term))
		}},
		{name: "heap-key-and-products", build: func(term identity.Term) State {
			object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root: value(term), StaticMembers: map[keyspace.Key]product.Value{fx.staticHeapKey: value(term)},
				DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{fx.dynamicKey: dynamic(term)},
			})
			out := Domain(reg).Bottom()
			out.heapTableIdentity = out.heapTableIdentity.withTerm(term, object)
			return out
		}},
		{name: "frozen-tables", build: func(term identity.Term) State {
			out := Domain(reg).Bottom()
			out.frozenTables, _ = out.frozenTables.freezeTerm(term)
			return out
		}},
		{name: "effect-deltas", build: func(term identity.Term) State {
			return Domain(reg).Bottom().WriteEffectDelta(fx.effectKey, effectdelta.Value{
				Before: value(term), After: value(term), Change: effectdelta.ChangeChanged,
			})
		}},
		{name: "channel-select", build: func(term identity.Term) State {
			return Domain(reg).Bottom().AddChannelSelectFact(channelselectfact.Fact{
				Select: "isolated-identity", Kind: channelselectfact.FactCase,
				Result: mustTestStateKey(fx.pathKey), Case: mustTestStateKey(fx.staticKey),
				Payload: value(term), HasPayload: true,
			})
		}},
		{name: "placement", build: func(term identity.Term) State {
			out := Domain(reg).Bottom()
			out.placement = out.placement.withTerm(term, placement.Stack)
			return out
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, test.build(templateTerm))
			if err != nil {
				t.Fatal(err)
			}
			if !Domain(reg).Equal(got, test.build(identity.ConcreteTerm(actual))) {
				t.Fatalf("identity-bearing lane %q was not independently discovered and rebased", test.name)
			}
		})
	}
}

func TestAllocationIdentityQuotientFindsTemplatesOnlyInPathImplicationProducts(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	fx := stateLawFixtureFor(reg, keys)
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-implication"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation image")
	}
	build := func(term identity.Term) State {
		value := identityvalue.PresentTerm(reg, term)
		return Domain(reg).Bottom().AddPathPresenceImplication(
			pathevidence.NewPathValueRefinementImplication(fx.dynamicKey.Table, value, fx.staticHeapKey, value),
		)
	}
	got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, build(templateTerm))
	if err != nil {
		t.Fatal(err)
	}
	if !Domain(reg).Equal(got, build(identity.ConcreteTerm(actual))) {
		t.Fatal("allocation quotient missed implication trigger/target product identities")
	}

	foreign := identity.ManifestAllocationTemplate(body, 2, 1)
	foreignAuthority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{foreign})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, foreignAuthority, build(templateTerm)); err == nil {
		t.Fatal("allocation quotient accepted an implication template absent from its authority")
	}
}

func TestAllocationIdentityQuotientFindsTemplatesOnlyInChannelSelectPayload(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	fx := stateLawFixtureFor(reg, keys)
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-channel-select"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation image")
	}
	build := func(term identity.Term) State {
		return Domain(reg).Bottom().AddChannelSelectFact(channelselectfact.Fact{
			Select: "identity-only", Kind: channelselectfact.FactCase,
			Result: mustTestStateKey(fx.pathKey), Case: mustTestStateKey(fx.staticKey),
			Payload: identityvalue.PresentTerm(reg, term), HasPayload: true,
		})
	}
	got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, build(templateTerm))
	if err != nil {
		t.Fatal(err)
	}
	if !Domain(reg).Equal(got, build(identity.ConcreteTerm(actual))) {
		t.Fatal("allocation quotient missed channel-select payload identity")
	}

	foreign := identity.ManifestAllocationTemplate(body, 2, 1)
	foreignAuthority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{foreign})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, foreignAuthority, build(templateTerm)); err == nil {
		t.Fatal("allocation quotient accepted a channel-select template absent from its authority")
	}
}

func TestAllocationIdentityQuotientUsesUniversalMustFiber(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-must"))
	caller := lexicalidentity.RootBody(namespace)
	callee := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(callee, 1, 1)
	templateTerm := identity.AllocationTerm(template)
	authority, err := NewBoundaryAllocationAuthority(ApplyBoundaryAllocationRoute(callee, caller, 23, 0), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	actual, ok := authority.RebaseAllocation(template)
	if !ok {
		t.Fatal("allocation image")
	}

	// Template and an already-instantiated route site coalesce. Frozen-table is
	// a must lane, so the target is frozen only when every active preimage is.
	templateRoot := product.Set(reg, typevalue.LiteralInt(reg, 7), identity.Key, identity.SingletonTerm(templateTerm))
	actualRoot := product.Set(reg, typevalue.LiteralString(reg, "prior"), identity.Key, identity.Singleton(actual))
	input := State{}.WriteValue(reg, keySymbolValueForAllocationQuotientTest(), identityvalue.PresentTerm(reg, templateTerm))
	input.heapTableIdentity = input.heapTableIdentity.
		withTerm(templateTerm, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: templateRoot})).
		with(actual, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: actualRoot}))
	input.frozenTables, _ = input.frozenTables.freezeTerm(templateTerm)
	// A placement entry makes the concrete target an active identity preimage
	// without manufacturing a second heap payload.
	input = input.WritePlacement(actual, placement.Stack)
	got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.IsTableFrozen(actual) {
		t.Fatal("must-frozen proof survived an unfrozen active preimage")
	}
	wantRoot := product.Join(reg,
		product.Set(reg, templateRoot, identity.Key, identity.Singleton(actual)),
		actualRoot,
	)
	if object := got.ReadHeapTableObject(reg, actual); !product.Equal(reg, object.Root(), wantRoot) {
		t.Fatalf("may heap payload did not join across the quotient fiber: got %v, want %v", object.Root(), wantRoot)
	}

	all := input.FreezeTable(actual)
	got, err = ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, all)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsTableFrozen(actual) {
		t.Fatal("must-frozen proof established by the complete fiber was dropped")
	}
}

func TestAllocationIdentityQuotientAbsentAuthorityIsRawIdentityOnlyWithoutTemplates(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	body := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-empty")))
	empty, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	input := State{}.WriteValue(reg, keySymbolValueForAllocationQuotientTest(), product.Top())
	for name, authority := range map[string]*BoundaryAllocationAuthority{"missing": nil, "empty": empty} {
		got, applyErr := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input)
		if applyErr != nil {
			t.Fatalf("%s authority rejected template-free product: %v", name, applyErr)
		}
		if got.canonical != input.canonical || got.laneMask != input.laneMask || !Domain(reg).Equal(got, input) {
			t.Fatalf("%s authority normalized or otherwise changed the raw state", name)
		}
	}
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	templateInput := input.WriteValue(reg, keySymbolValueForAllocationQuotientTest(), identityvalue.PresentTerm(reg, identity.AllocationTerm(template)))
	for name, authority := range map[string]*BoundaryAllocationAuthority{"missing": nil, "empty": empty} {
		if _, applyErr := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, templateInput); applyErr == nil {
			t.Fatalf("%s authority accepted a template-bearing product", name)
		}
	}
}

func TestAllocationIdentityQuotientNoTemplateFastPathIsRawAndDoesNotBuildSupport(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	fx := stateLawFixtureFor(reg, keys)
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("allocation-identity-quotient-no-template"))
	body := lexicalidentity.FunctionBody(namespace, 1)
	template := identity.ManifestAllocationTemplate(body, 1, 1)
	authority, err := NewBoundaryAllocationAuthority(RootBoundaryAllocationRoute(body), []identity.AllocationTemplate{template})
	if err != nil {
		t.Fatal(err)
	}
	stable := identity.ID{Kind: "lua.table", Site: "already-canonical", Index: 1}
	value := identityvalue.Present(reg, stable)
	dynamic := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue: value, HasKeyValue: true, Value: value, HasValue: true,
		Admission: dynamicindex.AdmissionAdmitted,
	})
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: value, StaticMembers: map[keyspace.Key]product.Value{fx.staticHeapKey: value},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{fx.dynamicKey: dynamic},
	})
	input := Domain(reg).Bottom().
		WriteValue(reg, keySymbolValueForAllocationQuotientTest(), value).
		WritePathKey(reg, keys, fx.pathKey, value).
		WritePathStaticMember(keys, fx.staticKey, value).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(
			fx.dynamicKey.Table, value, fx.staticHeapKey, value,
		)).
		WriteDynamicIndexFact(reg, fx.dynamicKey, dynamic).
		WriteHeapTableObject(reg, stable, object).
		FreezeTable(stable).
		WriteEffectDelta(fx.effectKey, effectdelta.Value{Before: value, After: value, Change: effectdelta.ChangeChanged}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "no-template", Kind: channelselectfact.FactCase,
			Result: mustTestStateKey(fx.pathKey), Case: mustTestStateKey(fx.staticKey),
			Payload: value, HasPayload: true,
		}).
		WritePlacement(stable, placement.Stack)

	got, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.canonical != input.canonical || got.laneMask != input.laneMask || !Domain(reg).Equal(got, input) {
		t.Fatal("no-template quotient path changed the raw State")
	}
	allocs := testing.AllocsPerRun(100, func() {
		if _, err := ApplyAllocationIdentityQuotient(context.Background(), reg, keys, authority, input); err != nil {
			panic(err)
		}
	})
	if allocs > 1 {
		t.Fatalf("no-template quotient allocations = %v, want at most one visitor closure and no support map", allocs)
	}
}

func keySymbolValueForAllocationQuotientTest() key.Value {
	return key.SymbolValue(symbol.ID(9001))
}
