package state

import (
	"errors"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCoordinateFormalRekeyQuotientsResolverVersionThroughLexicalRoot(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	const id = symbol.ID(704)
	lexical := from.FromPath(pathdom.NewPath(id, ""))
	versioned := from.FromPath(pathdom.Path{Symbol: id, Version: 9, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}})
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	want := formal.NewRoot(owner, 1, formal.Middle)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: lexical, Target: want, ResolverVersions: true}})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := domain.RekeyStructuralKeyFormal(plan, versioned)
	if err != nil {
		t.Fatal(err)
	}
	root, ok := to.StructuralRoot(mapped)
	got, exact := to.DescribeFormalRoot(root)
	if !ok || !exact || got != want || to.FormatReadOnly(mapped) == "" {
		t.Fatalf("mapped resolver version = %q root=%#v/%t/%t", to.FormatReadOnly(mapped), got, ok, exact)
	}
}

func TestFormalPublicationInverseSelectsExactResolverEnvironment(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	concrete, formalKeys := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	lexical := concrete.FromPath(pathdom.NewPath(symbol.ID(715), ""))
	inputRoot := formal.NewRoot(owner, 1, formal.Input)
	middleRoot := formal.NewRoot(owner, 1, formal.Middle)
	forward, err := domain.SealCoordinateFormalRootRekey(owner, concrete, formalKeys, []CoordinateFormalRootBinding{
		{Source: lexical, Target: inputRoot},
		{Source: lexical, Target: middleRoot, ResolverVersions: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	versioned := concrete.FromPath(pathdom.Path{Symbol: symbol.ID(715), Version: 11})
	publication, err := domain.SealCoordinateFormalPublicationInverse(forward, []CoordinateFormalInverseRootBinding{{Source: middleRoot, Target: versioned}})
	if err != nil {
		t.Fatal(err)
	}
	concreteMember := concrete.FromPath(pathdom.NewPath(symbol.ID(715), "").Field("member"))
	inputMember, ok := formalKeys.WithFormalRoot(concrete, concreteMember, inputRoot)
	if !ok {
		t.Fatal("formal input member")
	}
	formalMember, ok := formalKeys.WithFormalRoot(concrete, concreteMember, middleRoot)
	if !ok {
		t.Fatal("formal member")
	}
	forwardInput, err := domain.RekeyStructuralKeyFormal(forward, concreteMember)
	if err != nil || forwardInput != inputMember {
		t.Fatalf("structural forward = %q, %v", formalKeys.FormatReadOnly(forwardInput), err)
	}
	forwardMiddle, err := domain.RekeyStructuralKeyFormal(forward, concrete.FromPath(pathdom.Path{Symbol: symbol.ID(715), Version: 11, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}}))
	if err != nil || forwardMiddle != formalMember {
		t.Fatalf("resolver forward = %q, %v", formalKeys.FormatReadOnly(forwardMiddle), err)
	}
	inputBack, err := domain.RekeyStructuralKeyFormal(publication, inputMember)
	if err != nil || inputBack != concreteMember {
		t.Fatalf("structural inverse = %q, %v", concrete.FormatReadOnly(inputBack), err)
	}
	got, err := domain.RekeyStructuralKeyFormal(publication, formalMember)
	if err != nil || concrete.FormatReadOnly(got) != "sym715@11.member" {
		t.Fatalf("publication inverse = %q, %v", concrete.FormatReadOnly(got), err)
	}
	if _, err := domain.SealCoordinateFormalPublicationInverse(forward, nil); err == nil {
		t.Fatal("incomplete publication environment was accepted")
	}
}

func TestFormalPublicationRekeyRoundTripsEveryRegisteredAxis(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	concrete, formalKeys := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	lexical := concrete.FromPath(pathdom.NewPath(symbol.ID(201), ""))
	versioned := concrete.FromPath(pathdom.Path{Symbol: symbol.ID(201), Version: 1})
	bindings := []CoordinateFormalRootBinding{
		{Source: lexical, Target: formal.NewRoot(owner, 1, formal.Input)},
		{Source: lexical, Target: formal.NewRoot(owner, 1, formal.Middle), ResolverVersions: true},
	}
	for index, raw := range []pathdom.PathKey{"state-law-source", "state-law-target"} {
		root, ok := concrete.FromStateKey(raw)
		if !ok {
			t.Fatalf("state-law root %q is invalid", raw)
		}
		bindings = append(bindings, CoordinateFormalRootBinding{Source: root, Target: formal.NewRoot(owner, uint64(index+2), formal.Middle)})
	}
	forward, err := domain.SealCoordinateFormalRootRekey(owner, concrete, formalKeys, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.OwnsCoordinateFormalRootRekey(forward) {
		t.Fatalf("forward formal rekey is not owned: roots=%d index=%d", len(forward.roots), len(forward.rootIndex))
	}
	publicationBindings := []CoordinateFormalInverseRootBinding{{Source: bindings[1].Target, Target: versioned}}
	inverse, err := domain.SealCoordinateFormalPublicationInverse(forward, publicationBindings)
	if err != nil || !domain.OwnsCoordinateFormalRootRekey(inverse) {
		t.Fatalf("publication formal rekey = owned:%t err:%v", domain.OwnsCoordinateFormalRootRekey(inverse), err)
	}

	full := Reachable(domain.Lattice().Bottom())
	for _, sample := range stateLawLaneSamples(reg, concrete) {
		full = domain.Lattice().Join(full, sample.state)
	}
	// Exercise the structural Input and resolver Middle images of the same
	// lexical source in one complete product, not merely as scalar keys.
	full = full.WritePathKey(reg, concrete, pathdom.PathKey("sym201.structural"), presentValue(reg))
	residual, values := DecomposeValueLane(domain.Lattice(), full)
	factors, err := domain.DecomposeLanes(residual, domain.NonValuesLaneInventory())
	if err != nil {
		t.Fatal(err)
	}
	roundTrip := make([]LaneFactor, len(factors))
	for index, factor := range factors {
		families, familyErr := domain.CoordinateFamilies(factor.Lane())
		if familyErr != nil {
			t.Fatal(familyErr)
		}
		var mapped LaneFactor
		var forwardErr error
		if len(families) == 0 {
			mapped, forwardErr = domain.RekeyOrdinaryLaneFactorFormal(forward, factor)
			if forwardErr == nil {
				roundTrip[index], err = domain.RekeyOrdinaryLaneFactorFormal(inverse, mapped)
			}
		} else {
			mapped, forwardErr = domain.RekeyCoordinateLaneFactorFormal(forward, factor)
			if forwardErr == nil {
				var selected []CoordinateSlot
				families, familiesErr := domain.CoordinateFamilies(mapped.Lane())
				if familiesErr != nil {
					t.Fatalf("lane %q coordinate families: %v", factor.Lane().ID(), familiesErr)
				}
				for _, family := range families {
					_, familyScalars, familyErr := domain.DecomposeCoordinateFamily(mapped, family, formalKeys)
					if familyErr != nil {
						t.Fatalf("lane %q formal family: %v", factor.Lane().ID(), familyErr)
					}
					for _, scalar := range familyScalars {
						selected = append(selected, scalar.Slot())
					}
				}
				inventory, inventoryErr := domain.SealCoordinateFactorInventory(formalKeys, selected)
				if inventoryErr != nil {
					t.Fatalf("lane %q formal inventory: %v", factor.Lane().ID(), inventoryErr)
				}
				projection, projectionErr := domain.SealCoordinateFormalPublicationProjection(forward, inventory, publicationBindings)
				if projectionErr != nil {
					t.Fatalf("lane %q publication projection: %v", factor.Lane().ID(), projectionErr)
				}
				want, wantErr := domain.RekeyCoordinateLaneFactorFormalPublication(projection, mapped)
				if wantErr != nil {
					t.Fatalf("lane %q legacy fused oracle: %v", factor.Lane().ID(), wantErr)
				}
				fused, fusedErr := domain.SealCoordinateFormalBoundaryFactorPlan(projection, mapped.Lane())
				if fusedErr != nil {
					t.Fatalf("lane %q fused plan: %v", factor.Lane().ID(), fusedErr)
				}
				layouts := fused.FamilyLayouts()
				inputs := make([]CoordinateFormalBoundaryFamilyOperands, len(layouts))
				for familyIndex, layout := range layouts {
					skeleton, explicit, familyErr := domain.DecomposeCoordinateFamily(mapped, layout.Family(), formalKeys)
					if familyErr != nil {
						t.Fatalf("lane %q fused family: %v", factor.Lane().ID(), familyErr)
					}
					inputs[familyIndex].Skeleton = skeleton
					inputs[familyIndex].Scalars = make([]CoordinateScalarFactor, len(layout.Slots()))
					for slotIndex, slot := range layout.Slots() {
						for _, scalar := range explicit {
							equal, equalErr := domain.CoordinateSlotEqual(slot, scalar.Slot())
							if equalErr != nil {
								t.Fatal(equalErr)
							}
							if equal {
								inputs[familyIndex].Scalars[slotIndex] = scalar
								break
							}
						}
					}
				}
				got, gotErr := domain.ApplyCoordinateFormalBoundaryFactorPlan(fused, inputs)
				if gotErr != nil {
					t.Fatalf("lane %q fused apply: %v", factor.Lane().ID(), gotErr)
				}
				if equal, equalErr := domain.LaneCanonicalRepresentationEqual(got, want); equalErr != nil || !equal {
					t.Fatalf("lane %q fused publication parity = %t, err=%v", factor.Lane().ID(), equal, equalErr)
				}
				roundTrip[index], err = domain.RekeyCoordinateLaneFactorFormal(inverse, mapped)
			}
		}
		if forwardErr != nil {
			t.Fatalf("lane %q forward: %v", factor.Lane().ID(), forwardErr)
		}
		if err != nil {
			t.Fatalf("lane %q round trip: %v", factor.Lane().ID(), err)
		}
		equal, equalErr := domain.LaneEqual(factor, roundTrip[index])
		if equalErr != nil || !equal {
			t.Fatalf("lane %q inverse differs: equal=%t err=%v", factor.Lane().ID(), equal, equalErr)
		}
	}
	got, err := domain.ComposeFactorTuple(values, roundTrip)
	if err != nil || !domain.Lattice().Equal(got, domain.Normalize(full)) {
		t.Fatalf("17-axis inverse publication differs: equal=%t err=%v", domain.Lattice().Equal(got, domain.Normalize(full)), err)
	}
}

func TestOrdinaryFormalRekeyUsesSameResolverVersionQuotient(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneKeyMemberships})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	const tableID, keyID = symbol.ID(705), symbol.ID(706)
	tableRoot := from.FromPath(pathdom.NewPath(tableID, ""))
	keyRoot := from.FromPath(pathdom.NewPath(keyID, ""))
	tableVersion := from.FromPath(pathdom.Path{Symbol: tableID, Version: 4})
	keyVersion := from.FromPath(pathdom.Path{Symbol: keyID, Version: 7})
	tableState, tableOK := pathaddr.StateKeyFromPathKey(from.FormatReadOnly(tableVersion))
	keyState, keyOK := pathaddr.StateKeyFromPathKey(from.FormatReadOnly(keyVersion))
	if !tableOK || !keyOK {
		t.Fatal("versioned StateKey fixture")
	}
	source := State{}.AddPathKeyMembership(keyState, tableState)
	lane, _ := domain.ProductLane(LaneKeyMemberships)
	factors, err := domain.DecomposeLanes(source, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose=%d err=%v", len(factors), err)
	}
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	tableFormal := formal.NewRoot(owner, 1, formal.Middle)
	keyFormal := formal.NewRoot(owner, 2, formal.Middle)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{
		{Source: tableRoot, Target: tableFormal, ResolverVersions: true},
		{Source: keyRoot, Target: keyFormal, ResolverVersions: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := domain.RekeyOrdinaryLaneFactorFormal(plan, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.ComposeSparse([]LaneFactor{mapped})
	if err != nil {
		t.Fatal(err)
	}
	memberships := result.KeyMembershipsSnapshot().Memberships
	if len(memberships) != 1 {
		t.Fatalf("memberships=%#v", memberships)
	}
	for label, raw := range map[string]pathaddr.StateKey{"key": memberships[0].Key, "table": memberships[0].Table} {
		mappedKey, ok := to.FromStateKey(raw.PathKey())
		if !ok {
			t.Fatalf("%s mapped key=%q", label, raw)
		}
		root, rooted := to.StructuralRoot(mappedKey)
		_, exact := to.DescribeFormalRoot(root)
		if !rooted || !exact {
			t.Fatalf("%s has no formal root: %q", label, raw)
		}
	}
}

func TestOrdinaryFormalRekeyUsesRegisteredStateKeyBoundaryLaw(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneChannelSelect})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	left := from.FromPath(pathdom.Path{Symbol: 71, Version: 1})
	right := from.FromPath(pathdom.Path{Symbol: 72, Version: 1})
	leftState, leftOK := pathaddr.StateKeyFromPathKey(from.FormatReadOnly(left))
	rightState, rightOK := pathaddr.StateKeyFromPathKey(from.FormatReadOnly(right))
	if !leftOK || !rightOK {
		t.Fatal("source StateKey fixture")
	}
	source := State{}.AddChannelSelectFact(channelselectfact.Fact{
		Select: "formal-rekey", Kind: channelselectfact.FactCase, Result: leftState, Case: rightState,
	})
	lane, _ := domain.ProductLane(LaneChannelSelect)
	factors, err := domain.DecomposeLanes(source, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose = %d, %v", len(factors), err)
	}
	owner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())), 1)
	leftRoot := formal.NewRoot(owner, 1, formal.Input)
	rightRoot := formal.NewRoot(owner, 2, formal.Input)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{
		{Source: left, Target: leftRoot}, {Source: right, Target: rightRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := domain.RekeyOrdinaryLaneFactorFormal(plan, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.ComposeSparse([]LaneFactor{mapped})
	if err != nil {
		t.Fatal(err)
	}
	facts := result.ChannelSelectFactsSnapshot().Facts
	if len(facts) != 1 {
		t.Fatalf("mapped facts = %d", len(facts))
	}
	for index, raw := range []pathaddr.StateKey{facts[0].Result, facts[0].Case} {
		key, ok := to.FromStateKey(pathdom.PathKey(raw.String()))
		want := leftRoot
		if index == 1 {
			want = rightRoot
		}
		got, exact := to.DescribeFormalRoot(key)
		if !ok || !exact || got != want {
			t.Fatalf("mapped StateKey %d = %q, descriptor %#v/%t/%t", index, raw, got, ok, exact)
		}
	}
}

func TestOrdinaryFormalRekeyPreservesOptionalChannelSelectEndpoints(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneChannelSelect})
	if err != nil {
		t.Fatal(err)
	}
	from, to := keyspace.New(), keyspace.New()
	resultKey := from.FromPath(pathdom.Path{Symbol: 73, Version: 1})
	caseKey := from.FromPath(pathdom.Path{Symbol: 74, Version: 1})
	resultState, resultOK := pathaddr.StateKeyFromPathKey(from.FormatReadOnly(resultKey))
	caseState, caseOK := pathaddr.StateKeyFromPathKey(from.FormatReadOnly(caseKey))
	if !resultOK || !caseOK {
		t.Fatal("source StateKey fixture")
	}
	source := State{}.
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "formal-rekey-select", Kind: channelselectfact.FactSelect, Result: resultState,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "formal-rekey-case", Kind: channelselectfact.FactCase, Case: caseState,
		})
	lane, _ := domain.ProductLane(LaneChannelSelect)
	factors, err := domain.DecomposeLanes(source, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose = %d, %v", len(factors), err)
	}
	owner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())), 1)
	resultRoot := formal.NewRoot(owner, 1, formal.Input)
	caseRoot := formal.NewRoot(owner, 2, formal.Input)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{
		{Source: resultKey, Target: resultRoot}, {Source: caseKey, Target: caseRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := domain.RekeyOrdinaryLaneFactorFormal(plan, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.ComposeSparse([]LaneFactor{mapped})
	if err != nil {
		t.Fatal(err)
	}
	facts := result.ChannelSelectFactsSnapshot().Facts
	if len(facts) != 2 {
		t.Fatalf("mapped optional facts = %#v", facts)
	}
	for _, fact := range facts {
		var raw pathaddr.StateKey
		var want formal.Root
		switch fact.Kind {
		case channelselectfact.FactSelect:
			if fact.Case != "" {
				t.Fatalf("select case endpoint = %q", fact.Case)
			}
			raw, want = fact.Result, resultRoot
		case channelselectfact.FactCase:
			if fact.Result != "" {
				t.Fatalf("case result endpoint = %q", fact.Result)
			}
			raw, want = fact.Case, caseRoot
		default:
			t.Fatalf("unexpected channel-select fact: %#v", fact)
		}
		key, ok := to.FromStateKey(pathdom.PathKey(raw.String()))
		got, exact := to.DescribeFormalRoot(key)
		if !ok || !exact || got != want {
			t.Fatalf("mapped optional StateKey = %q, descriptor %#v/%t/%t", raw, got, ok, exact)
		}
	}
}

func TestOrdinaryFormalPublicationProjectsUnselectedStructuralFactsBeforeRekey(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneChannelSelect})
	if err != nil {
		t.Fatal(err)
	}
	concrete, formalKeys := keyspace.New(), keyspace.New()
	lexical := concrete.FromPath(pathdom.NewPath(symbol.ID(719), ""))
	versioned := concrete.FromPath(pathdom.Path{Symbol: symbol.ID(719), Version: 1})
	owner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())), 1)
	middle := formal.NewRoot(owner, 1, formal.Middle)
	forward, err := domain.SealCoordinateFormalRootRekey(owner, concrete, formalKeys, []CoordinateFormalRootBinding{
		{Source: lexical, Target: middle, ResolverVersions: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	formalVersion, err := domain.RekeyStructuralKeyFormal(forward, versioned)
	if err != nil {
		t.Fatal(err)
	}
	formalState, ok := pathaddr.StateKeyFromPathKey(formalKeys.FormatReadOnly(formalVersion))
	if !ok {
		t.Fatal("formal StateKey")
	}
	source := State{}.AddChannelSelectFact(channelselectfact.Fact{
		Select: "publication-projection", Kind: channelselectfact.FactCase,
		Result: formalState, Case: formalState,
	})
	lane, _ := domain.ProductLane(LaneChannelSelect)
	factors, err := domain.DecomposeLanes(source, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose = %d, %v", len(factors), err)
	}
	empty, err := domain.SealCoordinateFactorInventory(formalKeys, nil)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domain.SealCoordinateFormalPublicationProjection(forward, empty, nil)
	if err != nil {
		t.Fatal(err)
	}
	published, err := domain.RekeyOrdinaryLaneFactorFormalPublication(projection, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.ComposeSparse([]LaneFactor{published})
	if err != nil {
		t.Fatal(err)
	}
	if facts := result.ChannelSelectFactsSnapshot().Facts; len(facts) != 0 {
		t.Fatalf("unselected formal facts survived publication: %v", facts)
	}
}

func TestOrdinaryFormalPublicationPreservesSelectedStructuralFactsBeforeRekey(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneChannelSelect})
	if err != nil {
		t.Fatal(err)
	}
	concrete, formalKeys := keyspace.New(), keyspace.New()
	lexical := concrete.FromPath(pathdom.NewPath(symbol.ID(720), ""))
	owner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())), 1)
	input := formal.NewRoot(owner, 1, formal.Input)
	forward, err := domain.SealCoordinateFormalRootRekey(owner, concrete, formalKeys, []CoordinateFormalRootBinding{
		{Source: lexical, Target: input},
	})
	if err != nil {
		t.Fatal(err)
	}
	formalPath, err := domain.RekeyStructuralKeyFormal(forward, lexical)
	if err != nil {
		t.Fatal(err)
	}
	formalState, ok := pathaddr.StateKeyFromPathKey(formalKeys.FormatReadOnly(formalPath))
	if !ok {
		t.Fatal("formal StateKey")
	}
	source := State{}.AddChannelSelectFact(channelselectfact.Fact{
		Select: "publication-projection", Kind: channelselectfact.FactCase,
		Result: formalState, Case: formalState,
	})
	lane, _ := domain.ProductLane(LaneChannelSelect)
	factors, err := domain.DecomposeLanes(source, []ProductLane{lane})
	if err != nil || len(factors) != 1 {
		t.Fatalf("decompose = %d, %v", len(factors), err)
	}
	empty, err := domain.SealCoordinateFactorInventory(formalKeys, nil)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := domain.SealCoordinateFormalPublicationProjection(forward, empty, nil)
	if err != nil {
		t.Fatal(err)
	}
	published, err := domain.RekeyOrdinaryLaneFactorFormalPublication(projection, factors[0])
	if err != nil {
		t.Fatal(err)
	}
	result, err := domain.ComposeSparse([]LaneFactor{published})
	if err != nil {
		t.Fatal(err)
	}
	facts := result.ChannelSelectFactsSnapshot().Facts
	if len(facts) != 1 || facts[0].Result != pathaddr.StateKey(concrete.FormatReadOnly(lexical)) ||
		facts[0].Case != pathaddr.StateKey(concrete.FormatReadOnly(lexical)) {
		t.Fatalf("selected formal fact = %#v", facts)
	}
}

func TestCoordinateFormalLaneFactorCrossOwnerIsInjectiveAndRoundTrips(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	concrete, callerKeys, calleeKeys := keyspace.New(), keyspace.New(), keyspace.New()
	leftRoot := concrete.FromPath(pathdom.NewPath(symbol.ID(501), ""))
	rightRoot := concrete.FromPath(pathdom.NewPath(symbol.ID(502), ""))
	left := concrete.FromPath(pathdom.NewPath(symbol.ID(501), "").Append(segment.Segment{Kind: segment.SegmentField, Name: "left"}))
	right := concrete.FromPath(pathdom.NewPath(symbol.ID(502), "").Append(segment.Segment{Kind: segment.SegmentField, Name: "right"}))
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	sourceState := domain.Lattice().Bottom().
		WriteLocalPathKey(reg, left, value).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: left, Other: right})
	sourceFactors, err := domain.Decompose(sourceState)
	if err != nil || len(sourceFactors) != 1 {
		t.Fatalf("source factors=%d err=%v", len(sourceFactors), err)
	}

	callerOwner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name() + "/caller")))
	calleeOwner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name() + "/callee")))
	callerLeft := formal.NewRoot(callerOwner, 1, formal.Input)
	callerRight := formal.NewRoot(callerOwner, 2, formal.Input)
	// Deliberately reverse destination ordinals: the lane primitive must use
	// registered destination key order rather than retaining source slice order.
	calleeLeft := formal.NewRoot(calleeOwner, 2, formal.Input)
	calleeRight := formal.NewRoot(calleeOwner, 1, formal.Input)
	concreteToCaller, err := domain.SealCoordinateFormalRootRekey(callerOwner, concrete, callerKeys, []CoordinateFormalRootBinding{
		{Source: leftRoot, Target: callerLeft}, {Source: rightRoot, Target: callerRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	callerFactor, err := domain.RekeyCoordinateLaneFactorFormal(concreteToCaller, sourceFactors[0])
	if err != nil {
		t.Fatal(err)
	}
	callerLeftKey, ok := callerKeys.WithFormalRoot(concrete, leftRoot, callerLeft)
	if !ok {
		t.Fatal("caller left formal root")
	}
	callerRightKey, ok := callerKeys.WithFormalRoot(concrete, rightRoot, callerRight)
	if !ok {
		t.Fatal("caller right formal root")
	}
	callerToCallee, err := domain.SealCoordinateFormalRootRekey(calleeOwner, callerKeys, calleeKeys, []CoordinateFormalRootBinding{
		{Source: callerLeftKey, Target: calleeLeft}, {Source: callerRightKey, Target: calleeRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignDomain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LanePathEvidence, LaneLenFloors})
	if err != nil {
		t.Fatal(err)
	}
	if got, foreignErr := foreignDomain.RekeyCoordinateLaneFactorFormal(callerToCallee, callerFactor); !errors.Is(foreignErr, ErrInvalidLaneFactor) || got.payload != nil {
		t.Fatalf("foreign ProductDomain result=%#v err=%v", got, foreignErr)
	}
	calleeFactor, err := domain.RekeyCoordinateLaneFactorFormal(callerToCallee, callerFactor)
	if err != nil {
		t.Fatal(err)
	}
	callerProof, err := domain.PathBranchProofCoordinateSlot(callerKeys, pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual,
		Path: mustFormalDescendant(t, callerKeys, concrete, left, callerLeft), Other: mustFormalDescendant(t, callerKeys, concrete, right, callerRight)})
	if err != nil {
		t.Fatal(err)
	}
	calleeProof, err := domain.RekeyCoordinateSlotFormal(callerToCallee, callerProof)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := pathevidence.DescribeCoordinate(pathEvidenceCoordinateKey(calleeProof.key))
	if !ok {
		t.Fatal("mapped proof descriptor")
	}
	for name, path := range map[string]keyspace.Key{"left": descriptor.Proof.Path, "right": descriptor.Proof.Other} {
		root, rootOK := calleeKeys.StructuralRoot(path)
		formalRoot, exact := calleeKeys.DescribeFormalRoot(root)
		if !rootOK || !exact || formalRoot.Owner() != calleeOwner || formalRoot.Owner() == callerOwner {
			t.Fatalf("%s root=%#v structural=%t exact=%t", name, formalRoot, rootOK, exact)
		}
	}

	calleeLeftKey := mustFormalDescendant(t, calleeKeys, callerKeys, callerLeftKey, calleeLeft)
	calleeRightKey := mustFormalDescendant(t, calleeKeys, callerKeys, callerRightKey, calleeRight)
	back, err := domain.SealCoordinateFormalRootRekey(callerOwner, calleeKeys, callerKeys, []CoordinateFormalRootBinding{
		{Source: calleeLeftKey, Target: callerLeft}, {Source: calleeRightKey, Target: callerRight},
	})
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := domain.RekeyCoordinateLaneFactorFormal(back, calleeFactor)
	if err != nil {
		t.Fatal(err)
	}
	if equal, equalErr := domain.LaneEqual(callerFactor, roundTrip); equalErr != nil || !equal {
		t.Fatalf("cross-owner roundtrip equal=%t err=%v", equal, equalErr)
	}

	if _, err := domain.SealCoordinateFormalRootRekey(calleeOwner, callerKeys, calleeKeys, []CoordinateFormalRootBinding{
		{Source: callerLeftKey, Target: calleeLeft}, {Source: callerRightKey, Target: calleeLeft},
	}); !errors.Is(err, ErrInvalidLaneFactor) {
		t.Fatalf("non-injective mapping error=%v", err)
	}
	missing, err := domain.SealCoordinateFormalRootRekey(calleeOwner, callerKeys, calleeKeys, []CoordinateFormalRootBinding{{Source: callerLeftKey, Target: calleeLeft}})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := domain.RekeyCoordinateLaneFactorFormal(missing, callerFactor); !errors.Is(err, ErrInvalidLaneFactor) || got.payload != nil {
		t.Fatalf("unmapped root result=%#v err=%v", got, err)
	}
}

func TestCoordinateFormalLaneFactorCarriesHeapDynamicNestedKeysAndExtraFamily(t *testing.T) {
	reg := standard.Registry()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))

	heapDomain := heapCoordinateTestDomain(t)
	from, to := keyspace.New(), keyspace.New()
	nested, ok := from.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "outer"}, {Kind: segment.SegmentField, Name: "inner"}})
	if !ok {
		t.Fatal("nested rootless key")
	}
	id := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	fact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: product.Top(), HasValue: true, Admission: dynamicindex.AdmissionAdmitted})
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.Absent(reg), StaticMembers: map[keyspace.Key]product.Value{nested: product.Top()},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{{Table: nested, Site: "nested"}: fact}, StableShape: true,
	})
	heap := onlyHeapTableIdentityFactor(t, heapDomain, heapDomain.Lattice().Bottom().WriteHeapTableObject(reg, id, object))
	plan, err := heapDomain.SealCoordinateFormalRootRekey(owner, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	mappedHeap, err := heapDomain.RekeyCoordinateLaneFactorFormal(plan, heap)
	if err != nil {
		t.Fatal(err)
	}
	heapLane, _ := heapDomain.ProductLane(LaneHeapTableIdentity)
	heapFamilies, _ := heapDomain.CoordinateFamilies(heapLane)
	mappedSkeleton, mappedScalars, err := heapDomain.DecomposeCoordinateFamily(mappedHeap, heapFamilies[0], to)
	if err != nil || len(mappedScalars) != 2 {
		t.Fatalf("mapped heap scalars=%d err=%v", len(mappedScalars), err)
	}
	mappedObject := heapCoordinateSkeletonValue(mappedSkeleton.payload).objects[identity.ConcreteTerm(id)]
	if len(mappedObject.staticKeys) != 1 || len(mappedObject.dynamicIndexFacts) != 1 {
		t.Fatalf("nested heap keys static=%d dynamic=%d", len(mappedObject.staticKeys), len(mappedObject.dynamicIndexFacts))
	}
	if got := string(to.FormatReadOnly(mappedObject.staticKeys[0])); !strings.Contains(got, "outer.inner") {
		t.Fatalf("mapped nested heap key=%q", got)
	}

	// LenFloor is an independently registered structural family. The generic
	// lane primitive picks it up through registration without a family switch.
	extraDomain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneLenFloors})
	if err != nil {
		t.Fatal(err)
	}
	extraFrom, extraTo := keyspace.New(), keyspace.New()
	extraPath := pathaddr.StateKey("sym777@1.items")
	extraKey, ok := extraFrom.InternStateKey(extraPath)
	if !ok {
		t.Fatal("extra registered family path")
	}
	extraRoot, ok := extraFrom.StructuralRoot(extraKey)
	if !ok {
		t.Fatal("extra registered family root")
	}
	extraState := extraDomain.Lattice().Bottom().WriteLenFloor(extraFrom, extraPath, 7)
	extraFactors, err := extraDomain.Decompose(extraState)
	if err != nil || len(extraFactors) != 1 {
		t.Fatalf("extra family factors=%d err=%v", len(extraFactors), err)
	}
	extraPlan, err := extraDomain.SealCoordinateFormalRootRekey(owner, extraFrom, extraTo, []CoordinateFormalRootBinding{{Source: extraRoot, Target: formal.NewRoot(owner, 7, formal.Middle)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extraDomain.RekeyCoordinateLaneFactorFormal(extraPlan, extraFactors[0]); err != nil {
		t.Fatalf("extra registered family was not transported: %v", err)
	}
}

func mustFormalDescendant(t *testing.T, to, from *keyspace.KeySpace, source keyspace.Key, target formal.Root) keyspace.Key {
	t.Helper()
	mapped, ok := to.WithFormalRoot(from, source, target)
	if !ok {
		t.Fatalf("formal descendant rekey failed for %#v", target)
	}
	return mapped
}

func TestCoordinateFormalRekeyMapsPresenceImplicationByRegisteredFamilyLaw(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	triggerRoot := from.FromPath(pathdom.NewPath(symbol.ID(101), ""))
	targetRoot := from.FromPath(pathdom.NewPath(symbol.ID(102), ""))
	trigger := from.FromPath(pathdom.NewPath(symbol.ID(101), "").Append(segment.Segment{Kind: segment.SegmentField, Name: "ready"}))
	target := from.FromPath(pathdom.NewPath(symbol.ID(102), "").Append(segment.Segment{Kind: segment.SegmentField, Name: "value"}))
	row := pathevidence.NewPathPresenceImplication(trigger, presence.Present(), target, presence.Present())
	slot, err := domain.PresenceImplicationCoordinateSlot(from, row)
	if err != nil {
		t.Fatal(err)
	}
	triggerFormal := formal.NewRoot(owner, 1, formal.Input)
	targetFormal := formal.NewRoot(owner, 1, formal.Middle)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: triggerRoot, Target: triggerFormal}, {Source: targetRoot, Target: targetFormal}})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := domain.RekeyCoordinateSlotFormal(plan, slot)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := domain.PresenceImplicationShapes(to, []CoordinateSlot{mapped})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
	if root, ok := to.DescribeFormalRoot(rows[0].Trigger); !ok || root != triggerFormal {
		t.Fatalf("trigger root=%#v exact=%t", root, ok)
	}
	if root, ok := to.DescribeFormalRoot(rows[0].Target); !ok || root != targetFormal {
		t.Fatalf("target root=%#v exact=%t", root, ok)
	}
	if got := string(to.FormatReadOnly(rows[0].Trigger)); !strings.HasSuffix(got, ".ready") {
		t.Fatalf("trigger suffix=%q", got)
	}

	skeleton, err := domain.CoordinateSkeletonTop(slot.Family(), from)
	if err != nil {
		t.Fatal(err)
	}
	scalar, err := domain.CoordinateDefault(skeleton, slot)
	if err != nil {
		t.Fatal(err)
	}
	mappedSkeleton, err := domain.RekeyCoordinateSkeletonFormal(plan, skeleton)
	if err != nil {
		t.Fatal(err)
	}
	mappedScalar, err := domain.RekeyCoordinateScalarFormal(plan, scalar)
	if err != nil {
		t.Fatal(err)
	}
	left, err := domain.CoordinateScalarJoin(scalar, scalar)
	if err != nil {
		t.Fatal(err)
	}
	mappedLeft, err := domain.RekeyCoordinateScalarFormal(plan, left)
	if err != nil {
		t.Fatal(err)
	}
	right, err := domain.CoordinateScalarJoin(mappedScalar, mappedScalar)
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.CoordinateScalarEqual(mappedLeft, right); err != nil || !equal {
		t.Fatalf("registered scalar operation changed: equal=%t err=%v", equal, err)
	}
	if support, err := domain.CoordinateScalarSupport(mappedSkeleton, mapped); err != nil || support == CoordinateScalarForbidden {
		t.Fatalf("mapped joint family support=%v err=%v", support, err)
	}
}

func TestCoordinateFormalRekeyRejectsForeignOwnerAndUnboundRoot(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("owner")))
	foreign := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("foreign")))
	root := from.FromPath(pathdom.NewPath(symbol.ID(201), ""))
	if _, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: root, Target: formal.NewRoot(foreign, 1, formal.Middle)}}); err == nil {
		t.Fatal("foreign formal owner admitted")
	}
	slot, err := domain.PathRefinementCoordinateSlot(from, root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.RekeyCoordinateSlotFormal(plan, slot); err == nil {
		t.Fatal("unbound concrete root admitted")
	}
}

func TestCoordinateFormalRekeyClosesLinkedCallResultEquality(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	from, to := keyspace.New(), keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	targetRoot := from.FromPath(pathdom.NewPath(symbol.ID(301), ""))
	resultBase := from.FromPath(pathdom.Path{Root: "ret[0]"})
	resultRoot, ok := from.ImportExistential(from, resultBase, keyspace.ExistentialNamespace{OwnerHi: 1, Point: 17})
	if !ok {
		t.Fatal("call-result existential")
	}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: targetRoot, Other: resultRoot}
	slot, err := domain.PathBranchProofCoordinateSlot(from, proof)
	if err != nil {
		t.Fatal(err)
	}
	targetFormal := formal.NewRoot(owner, 1, formal.Middle)
	resultFormal := formal.NewRoot(owner, 2, formal.Middle)
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, []CoordinateFormalRootBinding{{Source: targetRoot, Target: targetFormal}, {Source: resultRoot, Target: resultFormal}})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := domain.RekeyCoordinateSlotFormal(plan, slot)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok := pathevidence.DescribeCoordinate(pathEvidenceCoordinateKey(mapped.key))
	if !ok || descriptor.Kind != pathevidence.CoordinateDescriptorBranchProof {
		t.Fatal("mapped equality lost branch-proof identity")
	}
	if root, exact := to.DescribeFormalRoot(descriptor.Proof.Path); !exact || root != targetFormal {
		t.Fatalf("target=%#v exact=%t", root, exact)
	}
	if root, exact := to.DescribeFormalRoot(descriptor.Proof.Other); !exact || root != resultFormal {
		t.Fatalf("result=%#v exact=%t", root, exact)
	}
}

func TestHeapCoordinateFormalRekeyMatchesRegisteredConcreteImport(t *testing.T) {
	reg := standard.Registry()
	domain := heapCoordinateTestDomain(t)
	from, to := keyspace.New(), keyspace.New()
	field, _ := from.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "value"}})
	id := identity.ID{Kind: "table", Site: t.Name(), Index: 1}
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg), StaticMembers: map[keyspace.Key]product.Value{field: product.Top()}, StableShape: true})
	factor := onlyHeapTableIdentityFactor(t, domain, domain.Lattice().Bottom().WriteHeapTableObject(reg, id, object))
	lane, _ := domain.ProductLane(LaneHeapTableIdentity)
	families, _ := domain.CoordinateFamilies(lane)
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factor, families[0], from)
	if err != nil {
		t.Fatal(err)
	}
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	plan, err := domain.SealCoordinateFormalRootRekey(owner, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	mappedSkeleton, err := domain.RekeyCoordinateSkeletonFormal(plan, skeleton)
	if err != nil {
		t.Fatal(err)
	}
	mappedScalars := make([]CoordinateScalarFactor, len(scalars))
	for index, scalar := range scalars {
		mappedScalars[index], err = domain.RekeyCoordinateScalarFormal(plan, scalar)
		if err != nil {
			t.Fatal(err)
		}
	}
	mapped, err := domain.ComposeCoordinateFamilies(lane, to, []CoordinateFamilySkeleton{mappedSkeleton}, [][]CoordinateScalarFactor{mappedScalars})
	if err != nil {
		t.Fatal(err)
	}
	want, ok := typedLaneFactorValue[heapTableIdentityLane](factor.payload).rekey(from, to)
	if !ok {
		t.Fatal("canonical heap import failed")
	}
	got := typedLaneFactorValue[heapTableIdentityLane](mapped.payload)
	mapDomain := heapTermMapDomain(reg)
	if !mapDomain.Equal(want.asMap(mapDomain), got.asMap(mapDomain)) {
		t.Fatal("formal heap family rekey differs from canonical registered import")
	}
}
