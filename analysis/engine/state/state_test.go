package state

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestBottomReadsProductBottom(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	var s State

	if got := s.ReadValue(reg, key.SymbolValue(1)); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("absent value slot = %s, want product bottom", formatValue(reg, got))
	}
	if got := s.ReadPathKey(reg, pathdom.PathKey("sym1@1.field")); !valueDomain.Equal(got, valueDomain.Bottom()) {
		t.Fatalf("absent path key = %s, want product bottom", formatValue(reg, got))
	}
}

func TestWriteReadValueSlots(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	symSlot := key.SymbolValue(symbol.ID(10))
	retSlot := key.ReturnSlot(1)
	symValue := presentValue(reg)
	retValue := absentValue(reg)

	s := State{}.
		WriteValue(reg, symSlot, symValue).
		WriteValue(reg, retSlot, retValue)

	if got := s.ReadValue(reg, symSlot); !valueDomain.Equal(got, symValue) {
		t.Fatalf("symbol slot = %s, want %s", formatValue(reg, got), formatValue(reg, symValue))
	}
	if got := s.ReadSymbolValue(reg, symbol.ID(10)); !valueDomain.Equal(got, symValue) {
		t.Fatalf("symbol value = %s, want %s", formatValue(reg, got), formatValue(reg, symValue))
	}
	if got := s.ReadValue(reg, retSlot); !valueDomain.Equal(got, retValue) {
		t.Fatalf("return slot = %s, want %s", formatValue(reg, got), formatValue(reg, retValue))
	}
}

func TestWritesAreImmutable(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	slot := key.SymbolValue(symbol.ID(11))
	pathKey := pathdom.PathKey("sym11@1.field")
	present := presentValue(reg)
	absent := absentValue(reg)

	s1 := State{}.
		WriteValue(reg, slot, present).
		WritePathKey(reg, pathKey, present)
	s2 := s1.
		WriteValue(reg, slot, absent).
		WritePathKey(reg, pathKey, absent)

	if got := s1.ReadValue(reg, slot); !valueDomain.Equal(got, present) {
		t.Fatalf("original value slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("original path key changed to %s", formatValue(reg, got))
	}
	if got := s2.ReadValue(reg, slot); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated value slot = %s, want absent value", formatValue(reg, got))
	}
	if got := s2.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated path key = %s, want absent value", formatValue(reg, got))
	}
}

func TestUpdateHelpersReadCurrentAndCanonicalizeBottom(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	slot := key.SymbolValue(symbol.ID(12))
	retSlot := 0
	pathKey := pathdom.PathKey("sym12@1.field")
	present := presentValue(reg)
	absent := absentValue(reg)
	bottom := valueDomain.Bottom()

	s1 := State{}.
		WriteValue(reg, slot, present).
		WriteReturnSlot(reg, retSlot, present).
		WritePathKey(reg, pathKey, present)
	s2 := s1.
		UpdateValue(reg, slot, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdateValue read %s, want present", formatValue(reg, got))
			}
			return bottom
		}).
		UpdateReturnSlot(reg, retSlot, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdateReturnSlot read %s, want present", formatValue(reg, got))
			}
			return absent
		}).
		UpdatePathKey(reg, pathKey, func(got product.Value) product.Value {
			if !valueDomain.Equal(got, present) {
				t.Fatalf("UpdatePathKey read %s, want present", formatValue(reg, got))
			}
			return bottom
		})

	if got := s1.ReadValue(reg, slot); !valueDomain.Equal(got, present) {
		t.Fatalf("original value slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadReturnSlot(reg, retSlot); !valueDomain.Equal(got, present) {
		t.Fatalf("original return slot changed to %s", formatValue(reg, got))
	}
	if got := s1.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("original path key changed to %s", formatValue(reg, got))
	}
	if got := s2.ReadValue(reg, slot); !valueDomain.Equal(got, bottom) {
		t.Fatalf("updated value slot = %s, want bottom", formatValue(reg, got))
	}
	if got := s2.ReadReturnSlot(reg, retSlot); !valueDomain.Equal(got, absent) {
		t.Fatalf("updated return slot = %s, want absent", formatValue(reg, got))
	}
	if got := s2.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, bottom) {
		t.Fatalf("updated path key = %s, want bottom", formatValue(reg, got))
	}
	if _, ok := s2.values[slot]; ok {
		t.Fatalf("UpdateValue to bottom kept finite value entry")
	}
	if _, ok := s2.PathRefinementsSnapshot().Refinements[pathKey]; ok {
		t.Fatalf("UpdatePathKey to bottom kept finite path entry")
	}
	if !stateDomain.Equal(State{}.WriteReturnSlot(reg, retSlot, absent), State{}.WriteValue(reg, key.ReturnSlot(retSlot), absent)) {
		t.Fatalf("return-slot helper does not use key.ReturnSlot spelling")
	}
}

func TestDomainPointwiseOperations(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	valueSlot := key.SymbolValue(symbol.ID(21))
	retSlot := key.ReturnSlot(0)
	pathKey := pathdom.PathKey("sym21@2.field")
	otherPathKey := pathdom.PathKey("$0.item")

	a := State{}.
		WriteValue(reg, valueSlot, present).
		WritePathKey(reg, pathKey, present)
	b := State{}.
		WriteValue(reg, valueSlot, absent).
		WriteValue(reg, retSlot, present).
		WritePathKey(reg, pathKey, absent).
		WritePathKey(reg, otherPathKey, present)

	joined := stateDomain.Join(a, b)
	if got := joined.ReadValue(reg, valueSlot); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared value slot = %s, want top", formatValue(reg, got))
	}
	if got := joined.ReadValue(reg, retSlot); !valueDomain.Equal(got, present) {
		t.Fatalf("joined disjoint value slot = %s, want present", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined shared path key = %s, want top", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, otherPathKey); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("joined disjoint path key = %s, want bottom (dropped by must join)", formatValue(reg, got))
	}

	if widened := stateDomain.Widen(a, b); !stateDomain.Equal(widened, joined) {
		t.Fatalf("Widen differs from Join: got %s, want %s", formatState(reg, widened), formatState(reg, joined))
	}
	if !stateDomain.LessOrEq(a, joined) || !stateDomain.LessOrEq(b, joined) {
		t.Fatalf("Join is not an upper bound: a=%s b=%s joined=%s",
			formatState(reg, a), formatState(reg, b), formatState(reg, joined))
	}
	if stateDomain.LessOrEq(joined, a) {
		t.Fatalf("joined state unexpectedly <= left operand")
	}
	if stateDomain.Equal(a, b) {
		t.Fatalf("states with different pointwise lanes compare equal")
	}
	if !stateDomain.Equal(a, a.Snapshot()) {
		t.Fatalf("Snapshot should preserve state equality")
	}
}

func TestStateLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	d := Domain(reg)
	suite := latticelaws.LawSuite[State]{
		Name:   "state.State",
		Domain: d,
		Sample: stateLawSample(reg),
		Format: stateLawFormat(reg),
	}
	suite.Run(t)
}

func TestStateOrderConsistencyAndJoinMonotonicity(t *testing.T) {
	reg := standard.Registry()
	d := Domain(reg)
	sample := stateLawSample(reg)

	for _, a := range sample {
		for _, b := range sample {
			eq := d.Equal(a, b)
			le := d.LessOrEq(a, b)
			ge := d.LessOrEq(b, a)
			if eq != (le && ge) {
				t.Fatalf("equality/order mismatch: a=%s b=%s equal=%v less-or-eq=%v reverse=%v",
					stateLawFormat(reg)(a), stateLawFormat(reg)(b), eq, le, ge)
			}
		}
	}

	for _, a := range sample {
		for _, b := range sample {
			if !d.LessOrEq(a, b) {
				continue
			}
			for _, c := range sample {
				left := d.Join(a, c)
				right := d.Join(b, c)
				if !d.LessOrEq(left, right) {
					t.Fatalf("join monotonicity failed: %s ⊑ %s but Join(%s,%s)=%s ⊑ Join(%s,%s)=%s does not hold",
						stateLawFormat(reg)(a), stateLawFormat(reg)(b),
						stateLawFormat(reg)(a), stateLawFormat(reg)(c), stateLawFormat(reg)(left),
						stateLawFormat(reg)(b), stateLawFormat(reg)(c), stateLawFormat(reg)(right))
				}
				left = d.Join(c, a)
				right = d.Join(c, b)
				if !d.LessOrEq(left, right) {
					t.Fatalf("join monotonicity failed on left argument: %s ⊑ %s but Join(%s,%s)=%s ⊑ Join(%s,%s)=%s does not hold",
						stateLawFormat(reg)(a), stateLawFormat(reg)(b),
						stateLawFormat(reg)(c), stateLawFormat(reg)(a), stateLawFormat(reg)(left),
						stateLawFormat(reg)(c), stateLawFormat(reg)(b), stateLawFormat(reg)(right))
				}
			}
		}
	}
}

func TestStateCloneIndependenceAcrossLanes(t *testing.T) {
	reg := standard.Registry()
	fx := stateLawFixtureFor(reg)

	original := State{}.
		WriteValue(reg, fx.valueSlot, fx.present).
		WritePathKey(reg, fx.pathKey, fx.present).
		WritePathStaticMember(fx.staticKey, fx.present).
		WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact).
		WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: fx.present,
			StaticMembers: map[pathdom.PathKey]product.Value{
				fx.staticKey: fx.present,
			},
		})).
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack).
		AddChannelSelectFact(fx.channelFact).
		AddBranchProof(fx.proof)

	cloneOnlyProof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathNotEqual,
		Path:  fx.pathKey,
		Other: pathdom.PathKey("sym201@1.other"),
	}
	clone := original.Snapshot()
	clone = clone.WriteValue(reg, fx.valueSlot, fx.absent)
	clone = clone.WritePathKey(reg, fx.pathKey, fx.absent)
	clone = clone.WritePathStaticMember(fx.staticKey, fx.absent)
	clone = clone.AddBranchProof(cloneOnlyProof)
	clone = clone.WriteDynamicIndexFact(reg, fx.dynamicKey, dynamicindex.Bottom(reg))
	clone = clone.WriteHeapTableObject(reg, fx.heapID, heapidentity.BottomObject(reg))
	clone = clone.WriteEffectDelta(fx.effectKey, effectdelta.Bottom(reg))
	clone = clone.WritePlacement(fx.escapeID, placement.Unknown)
	clone = clone.AddChannelSelectFact(channelselectfact.Fact{
		Select: "clone-only",
		Kind:   channelselectfact.FactCase,
		Case:   fx.pathKey,
		Index:  7,
	})

	if got := original.ReadValue(reg, fx.valueSlot); !product.Equal(reg, got, fx.present) {
		t.Fatalf("original value slot mutated through clone: %s", formatValue(reg, got))
	}
	if got := original.ReadPathKey(reg, fx.pathKey); !product.Equal(reg, got, fx.present) {
		t.Fatalf("original path key mutated through clone: %s", formatValue(reg, got))
	}
	if got, ok := original.ReadPathStaticMember(fx.staticKey); !ok || !product.Equal(reg, got, fx.present) {
		t.Fatalf("original static member mutated through clone: %s ok=%v", formatValue(reg, got), ok)
	}
	if got := original.ReadDynamicIndexFact(reg, fx.dynamicKey); !dynamicindex.Domain(reg).Equal(got, fx.dynamicFact) {
		t.Fatalf("original dynamic index mutated through clone: %#v", got)
	}
	if got := original.ReadHeapTableObject(reg, fx.heapID); !product.Equal(reg, got.Root(), fx.present) ||
		!staticMemberEqual(reg, got, fx.staticKey, fx.present) {
		t.Fatalf("original heap object mutated through clone: %#v", got)
	}
	if got := original.ReadEffectDelta(fx.effectKey); !effectdelta.Domain(reg).Equal(got, fx.effectDelta) {
		t.Fatalf("original effect delta mutated through clone: %#v", got)
	}
	if got := original.ReadPlacement(fx.escapeID); got != placement.Stack {
		t.Fatalf("original placement mutated through clone: %v", got)
	}
	if !original.HasChannelSelectFact(fx.channelFact) {
		t.Fatalf("original channel-select fact mutated through clone")
	}
	if !original.HasBranchProof(fx.proof) {
		t.Fatalf("original branch proof mutated through clone")
	}

	if got := clone.ReadValue(reg, fx.valueSlot); !product.Equal(reg, got, fx.absent) {
		t.Fatalf("clone value slot = %s, want absent", formatValue(reg, got))
	}
	if got := clone.ReadPathKey(reg, fx.pathKey); !product.Equal(reg, got, fx.absent) {
		t.Fatalf("clone path key = %s, want absent", formatValue(reg, got))
	}
	if got, ok := clone.ReadPathStaticMember(fx.staticKey); !ok || !product.Equal(reg, got, fx.absent) {
		t.Fatalf("clone static member = %s ok=%v, want absent", formatValue(reg, got), ok)
	}
	if got := clone.ReadDynamicIndexFact(reg, fx.dynamicKey); dynamicindex.Domain(reg).Equal(got, fx.dynamicFact) {
		t.Fatalf("clone dynamic index did not change")
	}
	if got := clone.ReadHeapTableObject(reg, fx.heapID); product.Equal(reg, got.Root(), fx.present) {
		t.Fatalf("clone heap object root did not change")
	}
	if got := clone.ReadEffectDelta(fx.effectKey); effectdelta.Domain(reg).Equal(got, fx.effectDelta) {
		t.Fatalf("clone effect delta did not change")
	}
	if got := clone.ReadPlacement(fx.escapeID); got != placement.Unknown {
		t.Fatalf("clone placement = %v, want unknown", got)
	}
	if !clone.HasChannelSelectFact(fx.channelFact) || !clone.HasChannelSelectFact(channelselectfact.Fact{
		Select: "clone-only",
		Kind:   channelselectfact.FactCase,
		Case:   fx.pathKey,
		Index:  7,
	}) {
		t.Fatalf("clone channel-select update did not stick")
	}
	if !clone.HasBranchProof(fx.proof) || !clone.HasBranchProof(cloneOnlyProof) {
		t.Fatalf("clone branch proof updates did not stick")
	}
}

func TestStateBottomWritesRemoveExplicitBottomEntries(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	fx := stateLawFixtureFor(reg)

	state := State{}.
		WriteValue(reg, fx.valueSlot, fx.present).
		WritePathKey(reg, fx.pathKey, fx.present).
		WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact).
		WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: fx.present,
			StaticMembers: map[pathdom.PathKey]product.Value{
				fx.staticKey: fx.present,
			},
		})).
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack)

	state = state.WriteValue(reg, fx.valueSlot, valueDomain.Bottom())
	state = state.WritePathKey(reg, fx.pathKey, valueDomain.Bottom())
	state = state.WriteDynamicIndexFact(reg, fx.dynamicKey, dynamicindex.Bottom(reg))
	state = state.WriteHeapTableObject(reg, fx.heapID, heapidentity.BottomObject(reg))
	state = state.WriteEffectDelta(fx.effectKey, effectdelta.Bottom(reg))
	state = state.WritePlacement(fx.escapeID, placement.Bottom)

	if _, ok := state.values[fx.valueSlot]; ok {
		t.Fatalf("value slot kept explicit bottom entry")
	}
	if _, ok := state.PathRefinementsSnapshot().Refinements[fx.pathKey]; ok {
		t.Fatalf("path refinement kept explicit bottom entry")
	}
	if _, ok := state.dynamicIndex[fx.dynamicKey]; ok {
		t.Fatalf("dynamic index kept explicit bottom entry")
	}
	if _, ok := state.heapTableIdentity[fx.heapID]; ok {
		t.Fatalf("heap identity kept explicit bottom entry")
	}
	if state.effectDeltas.hasFinite(fx.effectKey) {
		t.Fatalf("effect delta kept explicit bottom entry")
	}
	if state.placement.hasFinite(fx.escapeID) {
		t.Fatalf("placement kept explicit bottom entry")
	}
}

func TestExplicitBottomEntriesCanonicalizeToAbsence(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	bottom := valueDomain.Bottom()
	explicit := State{
		values: map[key.Value]product.Value{
			key.ReturnSlot(0): bottom,
		},
	}

	if !stateDomain.Equal(explicit, State{}) {
		t.Fatalf("explicit bottom entries should equal absence")
	}
	joined := stateDomain.Join(explicit, State{})
	if !stateDomain.Equal(joined, State{}) {
		t.Fatalf("Join should canonicalize bottom entries away, got %s", formatState(reg, joined))
	}
	if len(joined.values) != 0 {
		t.Fatalf("Join kept bottom entries: values=%d", len(joined.values))
	}

	withValue := State{}.WriteValue(reg, key.ReturnSlot(0), presentValue(reg))
	withoutValue := withValue.WriteValue(reg, key.ReturnSlot(0), bottom)
	if !stateDomain.Equal(withoutValue, State{}) {
		t.Fatalf("writing bottom should delete the value entry, got %s", formatState(reg, withoutValue))
	}
}

func TestWritesFromStateBottomProduceReachableState(t *testing.T) {
	reg := standard.Registry()
	stateDomain := Domain(reg)
	bottom := stateDomain.Bottom()
	empty := State{}
	if stateDomain.Equal(bottom, empty) {
		t.Fatalf("lattice bottom must stay distinct from reachable empty for must-fact lanes")
	}

	slot := key.SymbolValue(symbol.ID(65))
	pathKey := pathdom.PathKey("sym65@1.member")
	dynamicKey := dynamicindex.Key{Table: pathdom.PathKey("sym65@1.table"), Site: "dyn"}
	heapID := identity.ID{Kind: "table", Site: "bottom-write", Index: 1}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: pathKey, Presence: presence.Present()}
	implication := pathevidence.PathPresenceImplication{
		Trigger:         pathKey,
		TriggerPresence: presence.Absent(),
		Target:          pathdom.PathKey("sym65@1.value"),
		TargetPresence:  presence.Present(),
	}
	effectKey := effectdelta.Key{Target: pathdom.PathKey("sym65@1.table"), Site: "effect", Kind: effectdelta.Mutation}
	channel := channelselectfact.Fact{Select: "select-bottom", Kind: channelselectfact.FactSelect, Result: pathKey}
	escapeID := identity.ID{Kind: "table", Site: "escape-bottom", Index: 1}
	present := presentValue(reg)

	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}

	cases := []struct {
		name  string
		state State
	}{
		{"value", bottom.WriteValue(reg, slot, present)},
		{"path", bottom.WritePathKey(reg, pathKey, present)},
		{"static-member", bottom.WritePathStaticMember(pathKey, present)},
		{"dynamic-index", bottom.WriteDynamicIndexFact(reg, dynamicKey, dynamicFact)},
		{"heap-table", bottom.WriteHeapTableObject(reg, heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present}))},
		{"branch-proof", bottom.AddBranchProof(proof)},
		{"path-presence-implication", bottom.AddPathPresenceImplication(implication)},
		{"effect-delta", bottom.WriteEffectDelta(effectKey, effectDelta)},
		{"channel-select", bottom.AddChannelSelectFact(channel)},
		{"placement", bottom.WritePlacement(escapeID, placement.Stack)},
	}
	for _, tc := range cases {
		if tc.state.pathEvidence.StaticMembersBottom() ||
			tc.state.pathEvidence.ProofsBottom() ||
			tc.state.pathEvidence.PathPresenceImplicationsBottom() ||
			tc.state.ChannelSelectFactsSnapshot().Bottom {
			t.Fatalf("%s write left partial must-lane bottom: %#v", tc.name, tc.state)
		}
		if !stateDomain.LessOrEq(bottom, tc.state) {
			t.Fatalf("%s write did not move upward from lattice bottom", tc.name)
		}
	}
}

func TestPathPresenceImplicationsUseMustJoinAndInvalidate(t *testing.T) {
	reg := standard.Registry()
	stateDomain := Domain(reg)
	common := pathevidence.PathPresenceImplication{
		Trigger:         pathdom.PathKey("sym101@1.err"),
		TriggerPresence: presence.Absent(),
		Target:          pathdom.PathKey("sym101@1.value"),
		TargetPresence:  presence.Present(),
	}
	leftOnly := pathevidence.PathPresenceImplication{
		Trigger:         pathdom.PathKey("sym101@1.err"),
		TriggerPresence: presence.Present(),
		Target:          pathdom.PathKey("sym101@1.value"),
		TargetPresence:  presence.Absent(),
	}
	rightOnly := pathevidence.PathPresenceImplication{
		Trigger:         pathdom.PathKey("sym101@1.value"),
		TriggerPresence: presence.Absent(),
		Target:          pathdom.PathKey("sym101@1.err"),
		TargetPresence:  presence.Present(),
	}
	left := State{}.AddPathPresenceImplication(common).AddPathPresenceImplication(leftOnly)
	right := State{}.AddPathPresenceImplication(common).AddPathPresenceImplication(rightOnly)

	if !left.HasPathPresenceImplication(common) {
		t.Fatalf("missing inserted path-presence implication")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for path-presence implications")
	}
	joined := stateDomain.Join(left, right)
	if !joined.HasPathPresenceImplication(common) {
		t.Fatalf("common path-presence implication was dropped")
	}
	if joined.HasPathPresenceImplication(leftOnly) || joined.HasPathPresenceImplication(rightOnly) {
		t.Fatalf("disjoint path-presence implication survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("path-presence implication widen differs from join")
	}
	clone := left.Snapshot().AddPathPresenceImplication(rightOnly)
	if left.HasPathPresenceImplication(rightOnly) || !clone.HasPathPresenceImplication(rightOnly) {
		t.Fatalf("path-presence implication clone write mutated original or missed clone")
	}

	out, ok := left.InvalidatePathKeySubtree(pathdom.PathKey("sym101@1.err"))
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected trigger path")
	}
	if out.HasPathPresenceImplication(common) || out.HasPathPresenceImplication(leftOnly) {
		t.Fatalf("trigger path-presence implication survived trigger invalidation")
	}

	out, ok = left.InvalidatePathKeySubtree(pathdom.PathKey("sym101@1.value"))
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected target path")
	}
	if out.HasPathPresenceImplication(common) || out.HasPathPresenceImplication(leftOnly) {
		t.Fatalf("target path-presence implication survived target invalidation")
	}
}

func TestPathStaticMembersAndRefinementsUseMustJoin(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	common := pathdom.PathKey("sym70@1.shared")
	leftOnly := pathdom.PathKey("sym70@1.left")
	rightOnly := pathdom.PathKey("sym70@1.right")
	present := presentValue(reg)
	absent := absentValue(reg)

	left := State{}.
		WritePathKey(reg, leftOnly, present).
		WritePathStaticMember(common, present).
		WritePathStaticMember(leftOnly, present)
	right := State{}.
		WritePathKey(reg, rightOnly, present).
		WritePathStaticMember(common, absent).
		WritePathStaticMember(rightOnly, present)

	joined := stateDomain.Join(left, right)
	if got := joined.ReadPathKey(reg, leftOnly); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("left-only refinement survived must join: %s", formatValue(reg, got))
	}
	if got := joined.ReadPathKey(reg, rightOnly); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("right-only refinement survived must join: %s", formatValue(reg, got))
	}
	if got, ok := joined.ReadPathStaticMember(common); !ok || !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("joined static member = %s ok=%v, want top common fact", formatValue(reg, got), ok)
	}
	if _, ok := joined.ReadPathStaticMember(leftOnly); ok {
		t.Fatalf("left-only static member survived must join")
	}
	if _, ok := joined.ReadPathStaticMember(rightOnly); ok {
		t.Fatalf("right-only static member survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("static member widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("static member order should move toward fewer/weaker must facts")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for static members")
	}

	clone := left.Snapshot().WritePathStaticMember(common, absent)
	if got, _ := left.ReadPathStaticMember(common); !valueDomain.Equal(got, present) {
		t.Fatalf("static member clone write mutated original: %s", formatValue(reg, got))
	}
	if got, _ := clone.ReadPathStaticMember(common); !valueDomain.Equal(got, absent) {
		t.Fatalf("static member clone write = %s, want absent", formatValue(reg, got))
	}
}

func TestDynamicIndexKeysPointwiseFacts(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	common := dynamicindex.Key{Table: pathdom.PathKey("sym80@1.table"), Site: "common"}
	leftOnly := dynamicindex.Key{Table: pathdom.PathKey("sym80@1.table"), Site: "left"}
	presentFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    presentValue(reg),
		Value:       presentValue(reg),
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	absentFact := dynamicindex.Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absentValue(reg),
		Value:       absentValue(reg),
		Admission:   dynamicindex.AdmissionRejected,
	}

	if got := (State{}).ReadDynamicIndexFact(reg, common); !dynamicindex.Domain(reg).Equal(got, dynamicindex.Bottom(reg)) {
		t.Fatalf("empty dynamic index fact = %#v, want bottom", got)
	}

	left := State{}.
		WriteDynamicIndexFact(reg, common, presentFact).
		WriteDynamicIndexFact(reg, leftOnly, presentFact)
	right := State{}.WriteDynamicIndexFact(reg, common, absentFact)
	same := State{}.WriteDynamicIndexFact(reg, common, presentFact)
	if !stateDomain.Equal(State{}.WriteDynamicIndexFact(reg, common, presentFact), same) {
		t.Fatalf("equal dynamic index facts compare different")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for dynamic index facts")
	}

	joined := stateDomain.Join(left, right)
	got := joined.ReadDynamicIndexFact(reg, common)
	if !presence.Equal(got.KeyPresence, presence.Maybe()) ||
		!valueDomain.Equal(got.KeyValue, product.Top()) ||
		!valueDomain.Equal(got.Value, product.Top()) ||
		got.Admission != dynamicindex.AdmissionUnknown {
		t.Fatalf("joined dynamic fact = %#v, want joined key/value/admission atoms", got)
	}
	if got := joined.ReadDynamicIndexFact(reg, leftOnly); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("disjoint dynamic index fact did not survive pointwise join: %#v", got)
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("dynamic index widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("dynamic index order should be pointwise")
	}

	clone := left.Snapshot().WriteDynamicIndexFact(reg, common, absentFact)
	if got := left.ReadDynamicIndexFact(reg, common); !dynamicindex.Domain(reg).Equal(got, presentFact) {
		t.Fatalf("dynamic index clone write mutated original: %#v", got)
	}
	if got := clone.ReadDynamicIndexFact(reg, common); !dynamicindex.Domain(reg).Equal(got, absentFact) {
		t.Fatalf("dynamic index clone write = %#v, want absent fact", got)
	}
}

func TestHeapTableIdentityFacadeReadWriteAndCopy(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	stateDomain := Domain(reg)
	id := identity.ID{Kind: "table", Site: "alloc", Index: 1}
	staticCommon := pathdom.PathKey("sym90@1.table.name")
	dynCommon := dynamicindex.Key{Table: pathdom.PathKey("sym90@1.table"), Site: "dyn"}
	present := presentValue(reg)
	absent := absentValue(reg)

	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: present,
		StaticMembers: map[pathdom.PathKey]product.Value{
			staticCommon: present,
		},
		DynamicIndexFacts: map[dynamicindex.Key]dynamicindex.Fact{
			dynCommon: {
				KeyPresence: presence.Present(),
				KeyValue:    present,
				Value:       present,
				Admission:   dynamicindex.AdmissionAdmitted,
			},
		},
	})

	if got := (State{}).ReadHeapTableObject(reg, id); !heapidentity.ObjectDomain(reg).Equal(got, heapidentity.BottomObject(reg)) {
		t.Fatalf("empty heap object = %#v, want bottom", got)
	}

	written := State{}.WriteHeapTableObject(reg, id, object)
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), written), written) {
		t.Fatalf("state bottom should be join identity for heap table identity")
	}
	if got := written.ReadHeapTableObject(reg, id); !valueDomain.Equal(got.Root(), present) {
		t.Fatalf("heap object root = %s, want present", formatValue(reg, got.Root()))
	}
	read := written.ReadHeapTableObject(reg, id)
	readStatic := read.StaticMembers()
	readDynamic := read.DynamicIndexFacts()
	readStatic[staticCommon] = absent
	readDynamic[dynCommon] = dynamicindex.Fact{Admission: dynamicindex.AdmissionRejected}
	again := written.ReadHeapTableObject(reg, id)
	if got, ok := again.StaticMember(staticCommon); !ok || !valueDomain.Equal(got, present) {
		t.Fatalf("heap object read exposed mutable static members")
	}
	if got, ok := again.DynamicIndexFact(dynCommon); !ok || got.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("heap object read exposed mutable dynamic facts")
	}
}

func TestBranchProofsUseMustJoin(t *testing.T) {
	stateDomain := Domain(standard.Registry())
	common := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: pathdom.PathKey("sym100@1.err"), Presence: presence.Present()}
	leftOnly := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: pathdom.PathKey("sym100@1.a"), Other: pathdom.PathKey("sym100@1.b")}
	rightOnly := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: pathdom.PathKey("sym100@1.a"), Other: pathdom.PathKey("sym100@1.c")}
	left := State{}.AddBranchProof(common).AddBranchProof(leftOnly)
	right := State{}.AddBranchProof(common).AddBranchProof(rightOnly)

	if !left.HasBranchProof(common) || !stateDomain.Equal(State{}.AddBranchProof(common), State{}.AddBranchProof(common)) {
		t.Fatalf("branch proof empty/equality behavior failed")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for branch proofs")
	}
	joined := stateDomain.Join(left, right)
	if !joined.HasBranchProof(common) {
		t.Fatalf("common branch proof was dropped")
	}
	if joined.HasBranchProof(leftOnly) || joined.HasBranchProof(rightOnly) {
		t.Fatalf("disjoint branch proof survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("branch proof widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("branch proof order should move toward fewer must proofs")
	}
	clone := left.Snapshot().AddBranchProof(rightOnly)
	if left.HasBranchProof(rightOnly) || !clone.HasBranchProof(rightOnly) {
		t.Fatalf("branch proof clone write mutated original or missed clone")
	}
}

func TestEquivalentPathKeysFollowEqualityProofs(t *testing.T) {
	s := State{}.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  pathdom.PathKey("sym10@1"),
			Other: pathdom.PathKey("sym20@1"),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  pathdom.PathKey("sym20@1.child"),
			Other: pathdom.PathKey("sym30@1.leaf"),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  pathdom.PathKey("sym10@1"),
			Other: pathdom.PathKey("sym40@1"),
		})

	got := s.EquivalentPathKeys(pathdom.PathKey("sym10@1.child.name"))
	want := []pathdom.PathKey{
		pathdom.PathKey("sym20@1.child.name"),
		pathdom.PathKey("sym30@1.leaf.name"),
	}
	if len(got) != len(want) {
		t.Fatalf("EquivalentPathKeys len = %d (%#v), want %d (%#v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EquivalentPathKeys[%d] = %s, want %s (all %#v)", i, got[i], want[i], got)
		}
	}

	if got := s.EquivalentPathKeys(pathdom.PathKey("sym99@1.child")); len(got) != 0 {
		t.Fatalf("unrelated EquivalentPathKeys = %#v, want empty", got)
	}
}

func TestEffectDeltasPointwiseJoin(t *testing.T) {
	reg := standard.Registry()
	stateDomain := Domain(reg)
	common := effectdelta.Key{Target: pathdom.PathKey("sym110@1.table"), Site: "call", Kind: effectdelta.Mutation}
	leftOnly := effectdelta.Key{Target: pathdom.PathKey("sym110@1.left"), Site: "call", Kind: effectdelta.Mutation}
	presentDelta := effectdelta.Value{Before: presentValue(reg), After: presentValue(reg), Change: effectdelta.ChangeChanged}
	absentDelta := effectdelta.Value{Before: absentValue(reg), After: absentValue(reg), Change: effectdelta.ChangeNone}

	if got := (State{}).ReadEffectDelta(common); !effectdelta.Domain(reg).Equal(got, effectdelta.Bottom(reg)) {
		t.Fatalf("empty effect delta = %#v, want bottom", got)
	}
	if !stateDomain.Equal(State{}.WriteEffectDelta(common, presentDelta), State{}.WriteEffectDelta(common, presentDelta)) {
		t.Fatalf("equal effect deltas compare different")
	}

	left := State{}.WriteEffectDelta(common, presentDelta).WriteEffectDelta(leftOnly, presentDelta)
	right := State{}.WriteEffectDelta(common, absentDelta)
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for effect deltas")
	}
	joined := stateDomain.Join(left, right)
	got := joined.ReadEffectDelta(common)
	if !effectdelta.Domain(reg).Equal(got, effectdelta.Top()) {
		t.Fatalf("joined effect delta = %#v, want joined values and unknown change", got)
	}
	if got := joined.ReadEffectDelta(leftOnly); !effectdelta.Domain(reg).Equal(got, presentDelta) {
		t.Fatalf("disjoint effect delta did not survive pointwise join: %#v", got)
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("effect delta widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("effect delta order should be pointwise")
	}
	clone := left.Snapshot().WriteEffectDelta(common, absentDelta)
	if got := left.ReadEffectDelta(common); !effectdelta.Domain(reg).Equal(got, presentDelta) {
		t.Fatalf("effect delta clone write mutated original: %#v", got)
	}
	if got := clone.ReadEffectDelta(common); !effectdelta.Domain(reg).Equal(got, absentDelta) {
		t.Fatalf("effect delta clone write = %#v, want absent delta", got)
	}
}

func TestChannelSelectFactsUseMustJoin(t *testing.T) {
	stateDomain := Domain(standard.Registry())
	common := channelselectfact.Fact{Select: "select-1", Kind: channelselectfact.FactSelect, Result: pathdom.PathKey("sym120@1.result")}
	leftOnly := channelselectfact.Fact{Select: "select-1", Kind: channelselectfact.FactReceive, Case: pathdom.PathKey("sym120@1.left"), Index: 0}
	rightOnly := channelselectfact.Fact{Select: "select-1", Kind: channelselectfact.FactCase, Case: pathdom.PathKey("sym120@1.right"), Index: 1}
	left := State{}.AddChannelSelectFact(common).AddChannelSelectFact(leftOnly)
	right := State{}.AddChannelSelectFact(common).AddChannelSelectFact(rightOnly)

	if !left.HasChannelSelectFact(common) || !stateDomain.Equal(State{}.AddChannelSelectFact(common), State{}.AddChannelSelectFact(common)) {
		t.Fatalf("channel select empty/equality behavior failed")
	}
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for channel select facts")
	}
	joined := stateDomain.Join(left, right)
	if !joined.HasChannelSelectFact(common) {
		t.Fatalf("common channel select fact was dropped")
	}
	if joined.HasChannelSelectFact(leftOnly) || joined.HasChannelSelectFact(rightOnly) {
		t.Fatalf("disjoint channel select fact survived must join")
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("channel select widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("channel select order should move toward fewer must facts")
	}
	clone := left.Snapshot().AddChannelSelectFact(rightOnly)
	if left.HasChannelSelectFact(rightOnly) || !clone.HasChannelSelectFact(rightOnly) {
		t.Fatalf("channel select clone write mutated original or missed clone")
	}
}

func TestPlacementOrderAndCopy(t *testing.T) {
	stateDomain := Domain(standard.Registry())
	id := identity.ID{Kind: "table", Site: "escape", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "escape", Index: 2}

	if got := (State{}).ReadPlacement(id); got != placement.Bottom {
		t.Fatalf("empty placement = %v, want bottom", got)
	}
	if !stateDomain.Equal(State{}.WritePlacement(id, placement.Stack), State{}.WritePlacement(id, placement.Stack)) {
		t.Fatalf("equal placements compare different")
	}

	left := State{}.
		WritePlacement(id, placement.Stack).
		WritePlacement(otherID, placement.OwnedHeap)
	right := State{}.WritePlacement(id, placement.SharedHeap)
	if !stateDomain.Equal(stateDomain.Join(stateDomain.Bottom(), left), left) {
		t.Fatalf("state bottom should be join identity for placement")
	}
	joined := stateDomain.Join(left, right)
	if got := joined.ReadPlacement(id); got != placement.SharedHeap {
		t.Fatalf("joined placement = %v, want shared heap", got)
	}
	if got := joined.ReadPlacement(otherID); got != placement.OwnedHeap {
		t.Fatalf("disjoint placement did not survive pointwise join: %v", got)
	}
	if widened := stateDomain.Widen(left, right); !stateDomain.Equal(widened, joined) {
		t.Fatalf("placement widen differs from join")
	}
	if !stateDomain.LessOrEq(left, joined) || stateDomain.LessOrEq(joined, left) {
		t.Fatalf("placement order should move toward shared/unknown")
	}
	clone := left.Snapshot().WritePlacement(id, placement.Unknown)
	if got := left.ReadPlacement(id); got != placement.Stack {
		t.Fatalf("placement clone write mutated original: %v", got)
	}
	if got := clone.ReadPlacement(id); got != placement.Unknown {
		t.Fatalf("placement clone write = %v, want unknown", got)
	}
}

func TestPlacementCanBeReadThroughValueIdentity(t *testing.T) {
	reg := standard.Registry()
	id := identity.ID{Kind: "table", Site: "literal", Index: 7}
	value := product.Set(reg, product.Top(), identity.Key, identity.Singleton(id))
	state := State{}.WritePlacement(id, placement.OwnedHeap)

	if got := placementOfValue(reg, state, value); got != placement.OwnedHeap {
		t.Fatalf("placement through identity = %v, want owned heap", got)
	}
	if got := placementOfValue(reg, state, product.Top()); got != placement.Bottom {
		t.Fatalf("placement without identity = %v, want bottom", got)
	}
}

func TestInvalidatePathKeySubtreeRemovesStructuredDescendants(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	root := pathdom.PathKey("sym40@3")
	prefix := pathdom.PathKey("sym40@3.field")
	child := pathdom.PathKey("sym40@3.field.deep")
	siblingPrefixCollision := pathdom.PathKey("sym40@3.fieldish")
	localVersionless := pathdom.PathKey("sym40.field.deep")
	otherVersion := pathdom.PathKey("sym40@4.field.deep")
	otherSymbol := pathdom.PathKey("sym41@3.field.deep")
	placeholderPrefix := pathdom.PathKey("$0.field")
	placeholderChild := pathdom.PathKey("$0.field.deep")
	placeholderSibling := pathdom.PathKey("$0.fieldish")
	prefixProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: prefix, Presence: presence.Present()}
	childProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: child, Other: otherSymbol}
	otherProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: otherSymbol, Other: otherVersion}

	s := State{}.
		WritePathKey(reg, root, present).
		WritePathKey(reg, prefix, present).
		WritePathKey(reg, child, present).
		WritePathKey(reg, siblingPrefixCollision, present).
		WritePathKey(reg, localVersionless, present).
		WritePathKey(reg, otherVersion, present).
		WritePathKey(reg, otherSymbol, present).
		WritePathKey(reg, placeholderPrefix, present).
		WritePathKey(reg, placeholderChild, present).
		WritePathKey(reg, placeholderSibling, present).
		WritePathStaticMember(prefix, present).
		WritePathStaticMember(child, present).
		WritePathStaticMember(siblingPrefixCollision, present).
		WritePathStaticMember(otherVersion, present).
		AddBranchProof(prefixProof).
		AddBranchProof(childProof).
		AddBranchProof(otherProof)

	invalidPrefix, ok := s.InvalidatePathKeySubtree(pathdom.PathKey(".field"))
	if ok {
		t.Fatal("InvalidatePathKeySubtree accepted invalid path key")
	}
	if !Domain(reg).Equal(invalidPrefix, s) {
		t.Fatal("invalid path-key prefix changed state")
	}

	out, ok := s.InvalidatePathKeySubtree(prefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected versioned prefix")
	}
	for _, removed := range []pathdom.PathKey{prefix, child} {
		if got := out.ReadPathKey(reg, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{root, siblingPrefixCollision, localVersionless, otherVersion, otherSymbol, placeholderPrefix, placeholderChild, placeholderSibling} {
		if got := out.ReadPathKey(reg, kept); !valueDomain.Equal(got, present) {
			t.Fatalf("%s = %s, want present", kept, formatValue(reg, got))
		}
	}
	for _, removed := range []pathdom.PathKey{prefix, child} {
		if got, ok := out.ReadPathStaticMember(removed); ok {
			t.Fatalf("static member %s = %s, want removed", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{siblingPrefixCollision, otherVersion} {
		if got, ok := out.ReadPathStaticMember(kept); !ok || !valueDomain.Equal(got, present) {
			t.Fatalf("static member %s = %s ok=%v, want present", kept, formatValue(reg, got), ok)
		}
	}
	if out.HasBranchProof(prefixProof) || out.HasBranchProof(childProof) {
		t.Fatalf("subtree branch proof survived path invalidation")
	}
	if !out.HasBranchProof(otherProof) {
		t.Fatalf("unrelated branch proof was removed")
	}
	if got := s.ReadPathKey(reg, child); !valueDomain.Equal(got, present) {
		t.Fatalf("original child changed to %s", formatValue(reg, got))
	}
	if got, ok := s.ReadPathStaticMember(child); !ok || !valueDomain.Equal(got, present) {
		t.Fatalf("original static member changed to %s ok=%v", formatValue(reg, got), ok)
	}
	if !s.HasBranchProof(childProof) {
		t.Fatalf("original branch proof changed")
	}

	out, ok = out.InvalidatePathKeySubtree(placeholderPrefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected placeholder prefix")
	}
	for _, removed := range []pathdom.PathKey{placeholderPrefix, placeholderChild} {
		if got := out.ReadPathKey(reg, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	if got := out.ReadPathKey(reg, placeholderSibling); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", placeholderSibling, formatValue(reg, got))
	}
}

func placementOfValue(reg *axis.Registry, state State, value product.Value) placement.Value {
	idValue := product.Get(reg, value, identity.Key)
	id, ok := idValue.ID()
	if !ok {
		return placement.Bottom
	}
	return state.ReadPlacement(id)
}

func TestInvalidatePathKeySubtreeRemovesBranchProofsWithOtherUnderSubtree(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	prefix := pathdom.PathKey("sym70@2.field")
	otherInside := pathdom.PathKey("sym70@2.field.deep")
	outside := pathdom.PathKey("sym72@2.field")
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: outside, Other: otherInside}

	s := State{}.
		WritePathKey(reg, prefix, present).
		WritePathKey(reg, otherInside, present).
		WritePathKey(reg, outside, present).
		WritePathStaticMember(otherInside, present).
		AddBranchProof(proof)

	out, ok := s.InvalidatePathKeySubtree(prefix)
	if !ok {
		t.Fatal("InvalidatePathKeySubtree rejected versioned prefix")
	}
	if got := out.ReadPathKey(reg, prefix); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom", prefix, formatValue(reg, got))
	}
	if got := out.ReadPathKey(reg, otherInside); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom", otherInside, formatValue(reg, got))
	}
	if got := out.ReadPathKey(reg, outside); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", outside, formatValue(reg, got))
	}
	if got, ok := out.ReadPathStaticMember(otherInside); ok {
		t.Fatalf("static member %s = %s, want removed", otherInside, formatValue(reg, got))
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with Other under invalidated subtree survived")
	}
}

func TestInvalidatePathKeyDescendantsKeepsContainerAndUnrelatedPaths(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	container := pathdom.PathKey("sym60@2.item")
	child := pathdom.PathKey("sym60@2.item.count")
	deepChild := pathdom.PathKey("sym60@2.item.name.first")
	siblingPrefixCollision := pathdom.PathKey("sym60@2.itemized.count")
	root := pathdom.PathKey("sym60@2")
	otherVersion := pathdom.PathKey("sym60@3.item.count")
	otherSymbol := pathdom.PathKey("sym61@2.item.count")
	placeholderContainer := pathdom.PathKey("$0.item")
	placeholderChild := pathdom.PathKey("$0.item.count")
	containerProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: container, Presence: presence.Present()}
	childProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: child, Other: otherSymbol}

	s := State{}.
		WritePathKey(reg, container, present).
		WritePathKey(reg, child, present).
		WritePathKey(reg, deepChild, present).
		WritePathKey(reg, siblingPrefixCollision, present).
		WritePathKey(reg, root, present).
		WritePathKey(reg, otherVersion, present).
		WritePathKey(reg, otherSymbol, present).
		WritePathKey(reg, placeholderContainer, present).
		WritePathKey(reg, placeholderChild, present).
		WritePathStaticMember(container, present).
		WritePathStaticMember(child, present).
		WritePathStaticMember(deepChild, present).
		WritePathStaticMember(otherVersion, present).
		AddBranchProof(containerProof).
		AddBranchProof(childProof)

	invalidPrefix, ok := s.InvalidatePathKeyDescendants(pathdom.PathKey(".item"))
	if ok {
		t.Fatal("InvalidatePathKeyDescendants accepted invalid path key")
	}
	if !Domain(reg).Equal(invalidPrefix, s) {
		t.Fatal("invalid path-key prefix changed state")
	}

	out, ok := s.InvalidatePathKeyDescendants(container)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected versioned prefix")
	}
	for _, removed := range []pathdom.PathKey{child, deepChild} {
		if got := out.ReadPathKey(reg, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{container, siblingPrefixCollision, root, otherVersion, otherSymbol, placeholderContainer, placeholderChild} {
		if got := out.ReadPathKey(reg, kept); !valueDomain.Equal(got, present) {
			t.Fatalf("%s = %s, want present", kept, formatValue(reg, got))
		}
	}
	for _, removed := range []pathdom.PathKey{child, deepChild} {
		if got, ok := out.ReadPathStaticMember(removed); ok {
			t.Fatalf("static member %s = %s, want removed", removed, formatValue(reg, got))
		}
	}
	for _, kept := range []pathdom.PathKey{container, otherVersion} {
		if got, ok := out.ReadPathStaticMember(kept); !ok || !valueDomain.Equal(got, present) {
			t.Fatalf("static member %s = %s ok=%v, want present", kept, formatValue(reg, got), ok)
		}
	}
	if !out.HasBranchProof(containerProof) {
		t.Fatalf("container branch proof was removed by descendant invalidation")
	}
	if out.HasBranchProof(childProof) {
		t.Fatalf("descendant branch proof survived descendant invalidation")
	}
	if got := s.ReadPathKey(reg, child); !valueDomain.Equal(got, present) {
		t.Fatalf("original child changed to %s", formatValue(reg, got))
	}

	out, ok = out.InvalidatePathKeyDescendants(placeholderContainer)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected placeholder prefix")
	}
	if got := out.ReadPathKey(reg, placeholderContainer); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", placeholderContainer, formatValue(reg, got))
	}
	if got := out.ReadPathKey(reg, placeholderChild); !valueDomain.Equal(got, bottom) {
		t.Fatalf("%s = %s, want bottom", placeholderChild, formatValue(reg, got))
	}
}

func TestInvalidatePathKeyDescendantsFromRootRemovesStaticMembersAndBranchProofs(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	bottom := valueDomain.Bottom()
	root := pathdom.PathKey("sym80@1")
	child := pathdom.PathKey("sym80@1.field")
	descendant := pathdom.PathKey("sym80@1.field.deep")
	outside := pathdom.PathKey("sym81@1.field")
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: outside, Other: descendant}

	s := State{}.
		WritePathKey(reg, root, present).
		WritePathKey(reg, child, present).
		WritePathKey(reg, descendant, present).
		WritePathKey(reg, outside, present).
		WritePathStaticMember(child, present).
		WritePathStaticMember(descendant, present).
		AddBranchProof(proof)

	out, ok := s.InvalidatePathKeyDescendants(root)
	if !ok {
		t.Fatal("InvalidatePathKeyDescendants rejected versioned root")
	}
	if got := out.ReadPathKey(reg, root); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", root, formatValue(reg, got))
	}
	for _, removed := range []pathdom.PathKey{child, descendant} {
		if got := out.ReadPathKey(reg, removed); !valueDomain.Equal(got, bottom) {
			t.Fatalf("%s = %s, want bottom", removed, formatValue(reg, got))
		}
		if got, ok := out.ReadPathStaticMember(removed); ok {
			t.Fatalf("static member %s = %s, want removed", removed, formatValue(reg, got))
		}
	}
	if got := out.ReadPathKey(reg, outside); !valueDomain.Equal(got, present) {
		t.Fatalf("%s = %s, want present", outside, formatValue(reg, got))
	}
	if out.HasBranchProof(proof) {
		t.Fatalf("branch proof with Other under root descendant survived descendant invalidation")
	}
}

func TestTopLanesReadTopAndRejectFiniteUpdates(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	top := Domain(reg).Top()
	slot := key.SymbolValue(symbol.ID(50))
	pathKey := pathdom.PathKey("sym50@1.field")
	dynamicKey := dynamicindex.Key{Table: pathdom.PathKey("sym50@1.table"), Site: "dyn"}
	heapID := identity.ID{Kind: "table", Site: "top", Index: 1}
	effectKey := effectdelta.Key{Target: pathdom.PathKey("sym50@1.table"), Site: "effect", Kind: effectdelta.Mutation}
	escapeID := identity.ID{Kind: "table", Site: "escape-top", Index: 1}
	present := presentValue(reg)
	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}

	if got := top.ReadValue(reg, slot); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top value read = %s, want top", formatValue(reg, got))
	}
	if got := top.ReadReturnSlot(reg, 0); !valueDomain.Equal(got, product.Top()) {
		t.Fatalf("top return read = %s, want top", formatValue(reg, got))
	}
	if got := top.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, product.Bottom(reg)) {
		t.Fatalf("top path read = %s, want bottom absence", formatValue(reg, got))
	}
	if got := top.ReadDynamicIndexFact(reg, dynamicKey); !dynamicindex.Domain(reg).Equal(got, dynamicindex.Top()) {
		t.Fatalf("top dynamic-index read = %#v, want top", got)
	}
	if got := top.ReadHeapTableObject(reg, heapID); !heapidentity.ObjectDomain(reg).Equal(got, heapidentity.TopObject()) {
		t.Fatalf("top heap-object read = %#v, want top", got)
	}
	if got := top.ReadEffectDelta(effectKey); !effectdelta.Domain(reg).Equal(got, effectdelta.Top()) {
		t.Fatalf("top effect-delta read = %#v, want top", got)
	}
	if got := top.ReadPlacement(escapeID); got != placement.Unknown {
		t.Fatalf("top placement read = %v, want unknown", got)
	}
	if _, ok := top.ReadPathStaticMember(pathKey); ok {
		t.Fatalf("top static-member lane should read as unknown absence")
	}

	requirePanic(t, func() {
		top.WriteValue(reg, slot, present)
	})
	requirePanic(t, func() {
		top.UpdateValue(reg, slot, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdateValue on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.WriteReturnSlot(reg, 0, present)
	})
	requirePanic(t, func() {
		top.UpdateReturnSlot(reg, 0, func(v product.Value) product.Value {
			if !valueDomain.Equal(v, product.Top()) {
				t.Fatalf("UpdateReturnSlot on top read %s, want top", formatValue(reg, v))
			}
			return present
		})
	})
	requirePanic(t, func() {
		top.WriteDynamicIndexFact(reg, dynamicKey, dynamicFact)
	})
	requirePanic(t, func() {
		top.WriteHeapTableObject(reg, heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present}))
	})
	requirePanic(t, func() {
		top.WriteEffectDelta(effectKey, effectDelta)
	})
	requirePanic(t, func() {
		top.WritePlacement(escapeID, placement.Stack)
	})
}

func TestStatePackageDoesNotImportLuaPackages(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/__old",
		"github.com/wippyai/go-lua/analysis/engine/visibility",
		"github.com/wippyai/go-lua/analysis/ir/cfg",
		"github.com/wippyai/go-lua/analysis/lua",
		"github.com/wippyai/go-lua/compiler",
		"github.com/wippyai/go-lua/compiler/ast",
		"go/ast",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("state package imports forbidden dependency %q", dep)
			}
		}
	}
}

type stateLawFixture struct {
	valueSlot   key.Value
	returnSlot  int
	pathKey     pathdom.PathKey
	staticKey   pathdom.PathKey
	dynamicKey  dynamicindex.Key
	heapID      identity.ID
	effectKey   effectdelta.Key
	escapeID    identity.ID
	channelFact channelselectfact.Fact
	proof       pathevidence.BranchProof
	present     product.Value
	absent      product.Value
	dynamicFact dynamicindex.Fact
	effectDelta effectdelta.Value
}

func stateLawFixtureFor(reg *axis.Registry) stateLawFixture {
	present := presentValue(reg)
	absent := absentValue(reg)
	pathKey := pathdom.PathKey("sym201@1.field")
	staticKey := pathdom.PathKey("sym201@1.shared")
	tableKey := pathdom.PathKey("sym201@1.table")
	valueSlot := key.SymbolValue(symbol.ID(201))
	returnSlot := 3
	dynamicKey := dynamicindex.Key{Table: tableKey, Site: "dyn"}
	heapID := identity.ID{Kind: "table", Site: "state-law", Index: 1}
	effectKey := effectdelta.Key{Target: tableKey, Site: "effect", Kind: effectdelta.Mutation}
	escapeID := identity.ID{Kind: "table", Site: "escape-law", Index: 1}
	channelFact := channelselectfact.Fact{Select: "select-law", Kind: channelselectfact.FactSelect, Result: pathKey}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: pathKey, Presence: presence.Present()}
	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}

	return stateLawFixture{
		valueSlot:   valueSlot,
		returnSlot:  returnSlot,
		pathKey:     pathKey,
		staticKey:   staticKey,
		dynamicKey:  dynamicKey,
		heapID:      heapID,
		effectKey:   effectKey,
		escapeID:    escapeID,
		channelFact: channelFact,
		proof:       proof,
		present:     present,
		absent:      absent,
		dynamicFact: dynamicFact,
		effectDelta: effectDelta,
	}
}

func stateLawSample(reg *axis.Registry) []State {
	fx := stateLawFixtureFor(reg)
	bottom := Domain(reg).Bottom()
	top := Domain(reg).Top()

	valueState := State{}.
		WriteValue(reg, fx.valueSlot, fx.present).
		WriteReturnSlot(reg, fx.returnSlot, fx.absent)
	pathState := State{}.
		WritePathKey(reg, fx.pathKey, fx.present).
		WritePathStaticMember(fx.staticKey, fx.present).
		AddBranchProof(fx.proof)
	dynamicState := State{}.WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact)
	heapState := State{}.WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: fx.present,
		StaticMembers: map[pathdom.PathKey]product.Value{
			fx.staticKey: fx.present,
		},
	}))
	effectState := State{}.
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack)
	channelState := State{}.AddChannelSelectFact(fx.channelFact)
	fullState := valueState.
		WritePathKey(reg, fx.pathKey, fx.present).
		WritePathStaticMember(fx.staticKey, fx.present).
		WriteDynamicIndexFact(reg, fx.dynamicKey, fx.dynamicFact).
		WriteHeapTableObject(reg, fx.heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: fx.present,
			StaticMembers: map[pathdom.PathKey]product.Value{
				fx.staticKey: fx.present,
			},
		})).
		WriteEffectDelta(fx.effectKey, fx.effectDelta).
		WritePlacement(fx.escapeID, placement.Stack).
		AddChannelSelectFact(fx.channelFact).
		AddBranchProof(fx.proof)

	return []State{bottom, top, valueState, pathState, dynamicState, heapState, effectState, channelState, fullState}
}

func stateLawFormat(reg *axis.Registry) func(State) string {
	fx := stateLawFixtureFor(reg)
	return func(s State) string {
		static := "absent"
		if got, ok := s.ReadPathStaticMember(fx.staticKey); ok {
			static = formatValue(reg, got)
		}
		return fmt.Sprintf(
			"v=%s ret=%s path=%s static=%s dyn=%#v heap-root=%s effect=%#v placement=%v chan=%v proof=%v",
			formatValue(reg, s.ReadValue(reg, fx.valueSlot)),
			formatValue(reg, s.ReadReturnSlot(reg, fx.returnSlot)),
			formatValue(reg, s.ReadPathKey(reg, fx.pathKey)),
			static,
			s.ReadDynamicIndexFact(reg, fx.dynamicKey),
			formatValue(reg, s.ReadHeapTableObject(reg, fx.heapID).Root()),
			s.ReadEffectDelta(fx.effectKey),
			s.ReadPlacement(fx.escapeID),
			s.HasChannelSelectFact(fx.channelFact),
			s.HasBranchProof(fx.proof),
		)
	}
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func staticMemberEqual(reg *axis.Registry, object heapidentity.TableObject, key pathdom.PathKey, want product.Value) bool {
	got, ok := object.StaticMember(key)
	return ok && product.Equal(reg, got, want)
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	case presence.Equal(product.PresenceOf(v), presence.Present()):
		return "present"
	case presence.Equal(product.PresenceOf(v), presence.Absent()):
		return "absent"
	default:
		return product.PresenceOf(v).String()
	}
}

func formatState(reg *axis.Registry, s State) string {
	return "value-slot=" + formatValue(reg, s.ReadValue(reg, key.SymbolValue(21))) +
		" return-slot=" + formatValue(reg, s.ReadValue(reg, key.ReturnSlot(0))) +
		" path=" + formatValue(reg, s.ReadPathKey(reg, pathdom.PathKey("sym21@2.field"))) +
		" other-path=" + formatValue(reg, s.ReadPathKey(reg, pathdom.PathKey("$0.item")))
}
