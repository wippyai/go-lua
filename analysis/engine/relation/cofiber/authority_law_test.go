package cofiber_test

import (
	"bytes"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

type lawInventory struct {
	fence    address.Fence
	relation model.RelationID
	column   model.ColumnID
	key      model.KeyID
	first    model.ScopeID
	second   model.ScopeID
}

func (inventory lawInventory) Fence() address.Fence { return inventory.fence }
func (inventory lawInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == inventory.relation
}
func (inventory lawInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	return 2, id == inventory.column
}
func (inventory lawInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	return 3, id == inventory.key
}
func (inventory lawInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	switch id {
	case inventory.first:
		return 4, true
	case inventory.second:
		return 5, true
	default:
		return 0, false
	}
}
func (inventory lawInventory) ResolveExpression(model.ExpressionID) (uint64, bool) { return 0, false }
func (inventory lawInventory) ResolveDependency(model.DependencyID) (uint64, bool) { return 0, false }
func (inventory lawInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	return arrangement.NewHandle(inventory.fence, 1)
}
func (inventory lawInventory) ResolveDenominator(model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	return witness.DenominatorEvidence{}, false
}
func (inventory lawInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

type lawAlgebra struct{ typeID model.TypeID }

func (algebra lawAlgebra) Type() model.TypeID { return algebra.typeID }
func (algebra lawAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	return right, true
}
func (algebra lawAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}
func (algebra lawAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	return left.Type() == algebra.typeID && right.Type() == algebra.typeID
}

type lawAlgebras struct{ algebra lawAlgebra }

func (registry lawAlgebras) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

type fixture struct {
	mounted witness.Mounted
	first   witness.Scope
	second  witness.Scope
	manager *guard.Manager
	masks   [4]support.Mask
	regions map[identity.ContentID]support.Mask
}

func lawContent(t testing.TB, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/engine/relation/cofiber/law/v1", []byte(label))
	if !ok {
		t.Fatalf("content %q", label)
	}
	return value
}

func issue[T any](t testing.TB, label string, factory func(identity.ContentID) (T, bool)) T {
	t.Helper()
	value, ok := factory(lawContent(t, label))
	if !ok {
		t.Fatalf("issue %s", label)
	}
	return value
}

func newFixture(t testing.TB, generation identity.Generation) fixture {
	t.Helper()
	owner := issue(t, "owner", model.IssueOwnerID)
	schemaID := issue(t, "schema", func(value identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, value) })
	relation := issue(t, "relation", func(value identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, value) })
	column := issue(t, "column", func(value identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, value) })
	key := issue(t, "key", func(value identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(relation, value) })
	firstID := issue(t, "scope/first", func(value identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, value) })
	secondID := issue(t, "scope/second", func(value identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, value) })
	typeID := issue(t, "type", func(value identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, value) })
	firstRegion := scopeRegion(t, "first")

	builder := plan.NewBuilder(schemaID)
	typeCapability, capabilityOK := model.NewDecodeOnlyCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(typeCapability) {
		t.Fatal("type capability")
	}
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, firstID)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(firstID, nil, firstRegion)) ||
		!builder.AddScope(model.DefineScopeSchema(secondID, nil, region.True())) {
		t.Fatal("schema declarations")
	}
	schema, schemaOK := builder.Build()
	if !schemaOK {
		t.Fatal("schema")
	}
	certificateValue, refusal := certificate.Check(schema)
	if refusal != nil || !certificateValue.Available() {
		t.Fatalf("certificate: %v", refusal)
	}
	storeID, storeOK := identity.IssueStore()
	if !storeOK {
		t.Fatal("store")
	}
	fence, fenceOK := address.NewFence(schemaID, certificateValue.Digest(), storeID, identity.MountID{0xC0}, generation)
	if !fenceOK {
		t.Fatal("fence")
	}
	inventory := lawInventory{fence: fence, relation: relation, column: column, key: key, first: firstID, second: secondID}
	lineageFactory, lineageOK := lineage.NewFactory(owner)
	if !lineageOK {
		t.Fatal("lineage")
	}
	mounted, mountedOK := witness.Specialize(certificateValue, inventory, nil, lawAlgebras{algebra: lawAlgebra{typeID: typeID}}, lineageFactory)
	if !mountedOK || !mounted.Available() {
		t.Fatal("mounted")
	}
	first, firstOK := mounted.Scope(firstID)
	second, secondOK := mounted.Scope(secondID)
	if !firstOK || !secondOK {
		t.Fatal("mounted scopes")
	}
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	positive, positiveOK := work.Literal(1, true)
	negative, negativeOK := work.Not(positive)
	if !positiveOK || !negativeOK || !work.Seal() {
		t.Fatal("support formulas")
	}
	full, fullOK := support.True(manager)
	if !fullOK {
		t.Fatal("full")
	}
	emptySplit, emptySplitOK := support.Three(positive, positive)
	if !emptySplitOK {
		t.Fatal("empty")
	}
	masks := [4]support.Mask{emptySplit.LeftOnly(), negative, positive, full}
	return fixture{mounted: mounted, first: first, second: second, manager: manager, masks: masks, regions: map[identity.ContentID]support.Mask{firstRegion.Identity(): masks[2], region.True().Identity(): masks[3]}}
}

func scopeRegion(t testing.TB, label string) region.Region {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/engine/relation/cofiber/law-region/v2", []byte(label))
	if !ok {
		t.Fatal("region identity")
	}
	atom, ok := region.NewAtom(id)
	if !ok {
		t.Fatal("region atom")
	}
	value, ok := region.FromAtom(atom)
	if !ok {
		t.Fatal("region")
	}
	return value
}

func (fixture fixture) translate(value region.Region) (support.Mask, bool) {
	mask, ok := fixture.regions[value.Identity()]
	return mask, ok
}

func newAuthority(t testing.TB, value fixture) cofiber.Authority {
	t.Helper()
	authority, ok := cofiber.New(value.mounted, value.manager, value.translate)
	if !ok || !authority.Available() {
		t.Fatal("cofiber authority")
	}
	return authority
}

func TestAuthoritySealsDeterministicHomomorphicTranslation(t *testing.T) {
	value := newFixture(t, 1)
	calls := 0
	authority, ok := cofiber.New(value.mounted, value.manager, func(neutral region.Region) (support.Mask, bool) {
		calls++
		return value.translate(neutral)
	})
	if !ok || !authority.Available() {
		t.Fatal("authority")
	}
	sealedCalls := calls
	if sealedCalls == 0 {
		t.Fatal("translator was not used at bootstrap")
	}
	for _, scope := range []witness.Scope{value.first, value.second} {
		mask, maskOK := authority.Mask(scope)
		normalized, normalizedOK := authority.Normalize(mask)
		roundTrip, roundTripOK := authority.Mask(normalized)
		if !maskOK || !normalizedOK || !roundTripOK || !roundTrip.Equal(mask) {
			t.Fatal("declared scope did not round-trip through physical normalization")
		}
	}
	if !authority.Entails(value.first, value.second) || authority.Entails(value.second, value.first) {
		t.Fatal("sealed physical entailment diverged from the declared scope order")
	}
	if calls != sealedCalls {
		t.Fatal("runtime retained or re-invoked the bootstrap translator")
	}
}

func TestAuthorityRedeemsOnlyItsExactMountedArtifact(t *testing.T) {
	value := newFixture(t, 1)
	foreign := newFixture(t, 2)
	authority := newAuthority(t, value)
	if !authority.ValidFor(value.mounted) {
		t.Fatal("authority rejected its exact mounted artifact")
	}
	if authority.ValidFor(foreign.mounted) {
		t.Fatal("authority accepted a foreign mounted artifact")
	}
	if (cofiber.Authority{}).ValidFor(value.mounted) {
		t.Fatal("unavailable authority redeemed a mounted artifact")
	}
}

func TestAuthorityNormalizesExactBooleanPartitionsCanonically(t *testing.T) {
	value := newFixture(t, 1)
	authority := newAuthority(t, value)
	work := support.New(value.manager)
	if work == nil {
		t.Fatal("support work")
	}
	leftMask, leftMaskOK := work.Literal(1, true)
	rightMask, rightMaskOK := work.Literal(2, true)
	if !leftMaskOK || !rightMaskOK || !work.Seal() {
		t.Fatal("partition inputs")
	}
	split, splitOK := support.Three(leftMask, rightMask)
	if !splitOK {
		t.Fatal("partition")
	}
	for _, candidate := range []support.Mask{split.Left(), split.Right(), split.LeftOnly(), split.RightOnly(), split.Overlap(), split.Union()} {
		scope, scopeOK := authority.Normalize(candidate)
		repeated, repeatedOK := authority.Normalize(candidate)
		roundTrip, roundTripOK := authority.Mask(scope)
		if !scopeOK || !repeatedOK || !repeated.Same(scope) || !roundTripOK || !roundTrip.Equal(candidate) {
			t.Fatal("Boolean partition did not recover one exact canonical runtime scope")
		}
	}

	positive, positiveOK := authority.Normalize(leftMask)
	negative, negativeOK := authority.Normalize(value.masks[1])
	if !positiveOK || !negativeOK {
		t.Fatal("non-empty scopes")
	}
	contradictionMask, contradictionMaskOK := support.Intersect(leftMask, value.masks[1])
	if !contradictionMaskOK || !support.Empty(contradictionMask) {
		t.Fatal("test supports were not complementary")
	}
	if contradictory, contradictoryOK := authority.Conjoin(positive, negative); contradictoryOK || contradictory.Available() {
		t.Fatal("contradiction received an executable scope")
	}
	region, regionOK := value.mounted.RegionForScope(positive)
	logicalID := region.Identity()
	physicalID, physicalOK := leftMask.Identity()
	if !regionOK || !logicalID.Available() || !physicalOK || bytes.Equal(logicalID[:], physicalID[:]) {
		t.Fatal("logical region identity was conflated with physical formula identity")
	}
}

func TestAuthorityRejectsForeignAndBadTranslation(t *testing.T) {
	value := newFixture(t, 1)
	foreignManager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	foreign, foreignOK := support.True(foreignManager)
	if !foreignOK {
		t.Fatal("foreign support")
	}
	if authority, ok := cofiber.New(value.mounted, value.manager, func(region.Region) (support.Mask, bool) { return foreign, true }); ok || authority.Available() {
		t.Fatal("foreign-manager translator admitted")
	}

	flip := false
	if authority, ok := cofiber.New(value.mounted, value.manager, func(region.Region) (support.Mask, bool) {
		flip = !flip
		if flip {
			return value.masks[3], true
		}
		return value.masks[2], true
	}); ok || authority.Available() {
		t.Fatal("non-deterministic translator admitted")
	}

	if authority, ok := cofiber.New(value.mounted, value.manager, func(region.Region) (support.Mask, bool) {
		return value.masks[3], true // violates strict-subset entailment
	}); ok || authority.Available() {
		t.Fatal("non-homomorphic translator admitted")
	}

	authority := newAuthority(t, value)
	foreignFixture := newFixture(t, 2)
	if _, ok := authority.Mask(foreignFixture.first); ok {
		t.Fatal("foreign mount scope crossed fence")
	}
	if _, ok := authority.Normalize(foreign); ok {
		t.Fatal("foreign support crossed manager fence")
	}
	injected, injectedOK := value.mounted.AdmitRuntimeRegion(scopeRegion(t, "injected"))
	if !injectedOK || !injected.Available() {
		t.Fatal("trusted test admission")
	}
	if _, ok := authority.Mask(injected); ok {
		t.Fatal("foreign runtime region became an authority scope")
	}
}

func TestAuthorityConcurrentAndWarmNormalization(t *testing.T) {
	value := newFixture(t, 1)
	authority := newAuthority(t, value)
	mask := value.masks[3]
	warm, warmOK := authority.Normalize(mask)
	if !warmOK {
		t.Fatal("warm normalization")
	}
	const workers = 48
	values := make([]witness.Scope, workers)
	okays := make([]bool, workers)
	var group sync.WaitGroup
	for index := range values {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			values[index], okays[index] = authority.Normalize(mask)
		}(index)
	}
	group.Wait()
	for index, scope := range values {
		if !okays[index] || !scope.Same(warm) {
			t.Fatalf("concurrent normalization %d was not canonical", index)
		}
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		if !authority.Available() {
			t.Fatal("authority unavailable")
		}
		scope, scopeOK := authority.Normalize(mask)
		resolved, resolvedOK := authority.Mask(scope)
		if !scopeOK || !resolvedOK || !scope.Same(warm) || !resolved.Equal(mask) {
			t.Fatal("warm path changed semantics")
		}
	}); allocations != 0 {
		t.Fatalf("warm scope normalization allocated %f times", allocations)
	}
}
