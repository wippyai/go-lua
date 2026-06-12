package summary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFromResultProjectsNormalReturnFactsFromExitSnapshots(t *testing.T) {
	reg := standard.Registry()
	param0 := symbol.ID(901)
	param1 := symbol.ID(902)
	value0 := presentProduct(reg)
	value1 := absentProduct(reg)

	refineKey := normalReturnFactProjectTestKey(param0, ".refined")
	staticKey := normalReturnFactProjectTestKey(param0, ".member")
	dynAdmittedKey := normalReturnFactProjectTestKey(param0, `.items["admitted"]`)
	dynRejectedKey := normalReturnFactProjectTestKey(param0, `.items["rejected"]`)
	dynUnknownKey := normalReturnFactProjectTestKey(param1, ".items")
	branchPresenceKey := normalReturnFactProjectTestKey(param0, ".ready")
	branchEqualLeftKey := normalReturnFactProjectTestKey(param0, ".left")
	branchEqualRightKey := normalReturnFactProjectTestKey(param1, ".right")
	branchNotEqualLeftKey := normalReturnFactProjectTestKey(param0, ".a")
	branchNotEqualRightKey := normalReturnFactProjectTestKey(param1, ".b")
	selectResultKey := normalReturnFactProjectTestKey(param0, ".selectResult")
	receiveResultKey := normalReturnFactProjectTestKey(param0, ".receiveResult")
	receiveCaseKey := normalReturnFactProjectTestKey(param1, ".receiveCase")
	casePathKey := normalReturnFactProjectTestKey(param1, ".casePath")
	mutationKey := normalReturnFactProjectTestKey(param0, ".mutation")
	escapeKey := normalReturnFactProjectTestKey(param0, ".escape")
	callKey := normalReturnFactProjectTestKey(param1, ".call")

	exit := state.State{}.
		WritePathKey(reg, refineKey, value0).
		WritePathStaticMember(staticKey, product.Top()).
		WriteDynamicIndexFact(reg, state.DynamicIndexKey{Table: dynAdmittedKey, Site: "dyn-admitted"}, state.DynamicIndexFact{
			KeyPresence: presence.Present(),
			KeyValue:    value0,
			Value:       value1,
			Admission:   state.DynamicIndexAdmissionAdmitted,
		}).
		WriteDynamicIndexFact(reg, state.DynamicIndexKey{Table: dynRejectedKey, Site: "dyn-rejected"}, state.DynamicIndexFact{
			KeyPresence: presence.Absent(),
			KeyValue:    value1,
			Value:       value0,
			Admission:   state.DynamicIndexAdmissionRejected,
		}).
		WriteDynamicIndexFact(reg, state.DynamicIndexKey{Table: dynUnknownKey, Site: "dyn-unknown"}, state.DynamicIndexFact{
			KeyPresence: presence.Maybe(),
			KeyValue:    value0,
			Value:       value1,
			Admission:   state.DynamicIndexAdmissionUnknown,
		}).
		AddBranchProof(state.BranchProof{
			Kind:     state.BranchProofPathPresence,
			Path:     branchPresenceKey,
			Presence: presence.Present(),
		}).
		AddBranchProof(state.BranchProof{
			Kind:  state.BranchProofPathEqual,
			Path:  branchEqualLeftKey,
			Other: branchEqualRightKey,
		}).
		AddBranchProof(state.BranchProof{
			Kind:  state.BranchProofPathNotEqual,
			Path:  branchNotEqualLeftKey,
			Other: branchNotEqualRightKey,
		}).
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: "select-kind",
			Kind:   state.ChannelSelectFactSelect,
			Result: selectResultKey,
			Index:  0,
		}).
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: "receive-kind",
			Kind:   state.ChannelSelectFactReceive,
			Result: receiveResultKey,
			Case:   receiveCaseKey,
			Index:  1,
		}).
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: "case-kind",
			Kind:   state.ChannelSelectFactCase,
			Case:   casePathKey,
			Index:  2,
		}).
		WriteEffectDelta(reg, state.EffectDeltaKey{
			Target: mutationKey,
			Site:   "effect-mutation",
			Kind:   state.EffectDeltaMutation,
		}, state.EffectDelta{
			Before: value0,
			After:  value1,
			Change: state.EffectDeltaChangeChanged,
		}).
		WriteEffectDelta(reg, state.EffectDeltaKey{
			Target: escapeKey,
			Site:   "effect-escape",
			Kind:   state.EffectDeltaEscape,
		}, state.EffectDelta{
			Before: value0,
			After:  value0,
			Change: state.EffectDeltaChangeNone,
		}).
		WriteEffectDelta(reg, state.EffectDeltaKey{
			Target: callKey,
			Site:   "effect-call",
			Kind:   state.EffectDeltaCall,
		}, state.EffectDelta{
			Before: value1,
			After:  value0,
			Change: state.EffectDeltaChangeUnknown,
		})

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param0, param1)).NormalReturnFacts

	if len(got.PathRefinements) != 1 ||
		!got.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0).Field("refined")) ||
		!product.Equal(reg, got.PathRefinements[0].Value, value0) {
		t.Fatalf("PathRefinements = %#v, want $0.refined", got.PathRefinements)
	}
	if len(got.PathStaticMembers) != 1 ||
		!got.PathStaticMembers[0].Path.Equal(pathdom.NewPlaceholder(0).Field("member")) ||
		!product.Equal(reg, got.PathStaticMembers[0].Value, product.Top()) {
		t.Fatalf("PathStaticMembers = %#v, want top $0.member fact", got.PathStaticMembers)
	}

	assertDynamicAdmission(t, got.DynamicIndexFacts, "dyn-admitted", pathdom.NewPlaceholder(0).Field("items").IndexStr("admitted"), DynamicIndexAdmissionAdmitted)
	assertDynamicAdmission(t, got.DynamicIndexFacts, "dyn-rejected", pathdom.NewPlaceholder(0).Field("items").IndexStr("rejected"), DynamicIndexAdmissionRejected)
	assertDynamicAdmission(t, got.DynamicIndexFacts, "dyn-unknown", pathdom.NewPlaceholder(1).Field("items"), DynamicIndexAdmissionUnknown)

	assertBranchProof(t, got.BranchProofs, BranchProofPathPresence, pathdom.NewPlaceholder(0).Field("ready"), pathdom.Path{}, presence.Present())
	assertBranchProof(t, got.BranchProofs, BranchProofPathEqual, pathdom.NewPlaceholder(0).Field("left"), pathdom.NewPlaceholder(1).Field("right"), presence.Bottom())
	assertBranchProof(t, got.BranchProofs, BranchProofPathNotEqual, pathdom.NewPlaceholder(0).Field("a"), pathdom.NewPlaceholder(1).Field("b"), presence.Bottom())

	assertChannelSelect(t, got.ChannelSelects, "select-kind", ChannelSelectFactSelect, pathdom.NewPlaceholder(0).Field("selectResult"), pathdom.Path{})
	assertChannelSelect(t, got.ChannelSelects, "receive-kind", ChannelSelectFactReceive, pathdom.NewPlaceholder(0).Field("receiveResult"), pathdom.NewPlaceholder(1).Field("receiveCase"))
	assertChannelSelect(t, got.ChannelSelects, "case-kind", ChannelSelectFactCase, pathdom.Path{}, pathdom.NewPlaceholder(1).Field("casePath"))

	assertEffectDelta(t, got.EffectDeltas, "effect-mutation", pathdom.NewPlaceholder(0).Field("mutation"), EffectDeltaMutation, EffectDeltaChangeChanged)
	assertEffectDelta(t, got.EffectDeltas, "effect-escape", pathdom.NewPlaceholder(0).Field("escape"), EffectDeltaEscape, EffectDeltaChangeNone)
	assertEffectDelta(t, got.EffectDeltas, "effect-call", pathdom.NewPlaceholder(1).Field("call"), EffectDeltaCall, EffectDeltaChangeUnknown)
}

func TestFromResultDropsNonParameterNormalReturnFactPaths(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(911)
	value := presentProduct(reg)
	validParamKey := normalReturnFactProjectTestKey(param, ".kept")
	validPlaceholderKey := pathdom.PathKey("$0.already")
	invalidKeys := []pathdom.PathKey{
		normalReturnFactProjectTestKey(symbol.ID(912), ".local"),
		pathdom.PathKey(key.SymbolRoot(param) + ".versionless"),
		pathdom.PathKey("s911.stable"),
		pathdom.PathKey("ret[0].value"),
		pathdom.PathKey("global.value"),
		pathdom.PathKey("$1.outOfRange"),
		pathdom.PathKey(".unresolved"),
	}

	exit := state.State{}.
		WritePathKey(reg, validParamKey, value).
		WritePathKey(reg, validPlaceholderKey, value)
	for _, pathKey := range invalidKeys {
		exit = exit.WritePathKey(reg, pathKey, value)
	}
	exit = exit.
		AddBranchProof(state.BranchProof{
			Kind:  state.BranchProofPathEqual,
			Path:  validParamKey,
			Other: invalidKeys[0],
		}).
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: "invalid-result",
			Kind:   state.ChannelSelectFactReceive,
			Result: invalidKeys[1],
			Index:  0,
		})

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param)).NormalReturnFacts

	if len(got.PathRefinements) != 2 {
		t.Fatalf("PathRefinements = %#v, want only valid parameter and placeholder paths", got.PathRefinements)
	}
	if findPathRefinement(got.PathRefinements, pathdom.NewPlaceholder(0).Field("already")) == nil {
		t.Fatalf("PathRefinements = %#v, want already-placeholder key accepted", got.PathRefinements)
	}
	if findPathRefinement(got.PathRefinements, pathdom.NewPlaceholder(0).Field("kept")) == nil {
		t.Fatalf("PathRefinements = %#v, want parameter resolver key rebased", got.PathRefinements)
	}
	if len(got.BranchProofs) != 0 {
		t.Fatalf("BranchProofs = %#v, want proof with non-parameter other path dropped", got.BranchProofs)
	}
	if len(got.ChannelSelects) != 0 {
		t.Fatalf("ChannelSelects = %#v, want fact with non-parameter result path dropped", got.ChannelSelects)
	}
}

func TestFromResultSkipsTopSnapshotsAndTopNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(921)
	topFacts := FromResult(
		normalReturnFactProjectTestResult(reg, state.Domain(reg).Top(), param),
	).NormalReturnFacts
	if !normalReturnFactsEmpty(topFacts) {
		t.Fatalf("NormalReturnFacts from top state = %#v, want empty", topFacts)
	}

	paramKey := normalReturnFactProjectTestKey(param, ".value")
	topDynamic := state.DynamicIndexFact{
		KeyPresence: presence.Top(),
		KeyValue:    product.Top(),
		Value:       product.Top(),
		Admission:   state.DynamicIndexAdmissionUnknown,
	}
	topEffect := state.EffectDelta{
		Before: product.Top(),
		After:  product.Top(),
		Change: state.EffectDeltaChangeUnknown,
	}
	exit := state.State{}.
		WritePathKey(reg, paramKey, product.Top()).
		WritePathStaticMember(normalReturnFactProjectTestKey(param, ".member"), product.Bottom(reg)).
		WriteDynamicIndexFact(reg, state.DynamicIndexKey{
			Table: normalReturnFactProjectTestKey(param, ".table"),
			Site:  "dynamic-top",
		}, topDynamic).
		AddBranchProof(state.BranchProof{
			Kind:     state.BranchProofPathPresence,
			Path:     normalReturnFactProjectTestKey(param, ".proof"),
			Presence: presence.Top(),
		}).
		AddChannelSelectFact(state.ChannelSelectFact{
			Select: "invalid-kind",
			Kind:   0,
			Result: normalReturnFactProjectTestKey(param, ".result"),
			Index:  0,
		}).
		WriteEffectDelta(reg, state.EffectDeltaKey{
			Target: normalReturnFactProjectTestKey(param, ".effect"),
			Site:   "effect-top",
			Kind:   state.EffectDeltaMutation,
		}, topEffect)

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param)).NormalReturnFacts
	if !normalReturnFactsEmpty(got) {
		t.Fatalf("NormalReturnFacts = %#v, want top/no-op facts skipped", got)
	}
}

type normalReturnFactProjectResultStub struct {
	reg   *axis.Registry
	graph cfg.Graph
	exit  state.State
	slots []key.Value
}

func normalReturnFactProjectTestResult(
	reg *axis.Registry,
	exit state.State,
	params ...symbol.ID,
) normalReturnFactProjectResultStub {
	slots := make([]key.Value, len(params))
	for i, param := range params {
		slots[i] = key.SymbolValue(param)
	}
	return normalReturnFactProjectResultStub{
		reg:   reg,
		graph: cfg.New(),
		exit:  exit,
		slots: slots,
	}
}

func (r normalReturnFactProjectResultStub) Registry() *axis.Registry { return r.reg }

func (r normalReturnFactProjectResultStub) Graph() cfg.Graph { return r.graph }

func (r normalReturnFactProjectResultStub) ExitState() (state.State, bool) { return r.exit, true }

func (r normalReturnFactProjectResultStub) ReturnPoints() []cfg.Point { return nil }

func (r normalReturnFactProjectResultStub) ReturnArity(cfg.Point) (int, bool) { return 0, false }

func (r normalReturnFactProjectResultStub) ParameterValueSlots() []key.Value {
	if len(r.slots) == 0 {
		return nil
	}
	out := make([]key.Value, len(r.slots))
	copy(out, r.slots)
	return out
}

func normalReturnFactProjectTestKey(sym symbol.ID, suffix string) pathdom.PathKey {
	return pathdom.PathKey(key.SymbolVersionRoot(sym, 1) + suffix)
}

func assertDynamicAdmission(
	t *testing.T,
	facts []DynamicIndexFact,
	site string,
	table pathdom.Path,
	admission DynamicIndexAdmission,
) {
	t.Helper()
	fact := findDynamicIndexFact(facts, site)
	if fact == nil || !fact.Table.Equal(table) || fact.Admission != admission {
		t.Fatalf("dynamic index %q = %#v, want table %s admission %d", site, fact, table, admission)
	}
}

func assertBranchProof(
	t *testing.T,
	proofs []BranchProof,
	kind BranchProofKind,
	path pathdom.Path,
	other pathdom.Path,
	wantPresence presence.Value,
) {
	t.Helper()
	for _, proof := range proofs {
		if proof.Kind != kind || !proof.Path.Equal(path) {
			continue
		}
		switch kind {
		case BranchProofPathPresence:
			if presence.Equal(proof.Presence, wantPresence) {
				return
			}
		case BranchProofPathEqual, BranchProofPathNotEqual:
			if proof.Other.Equal(other) {
				return
			}
		}
	}
	t.Fatalf("BranchProofs = %#v, want kind %d path %s other %s", proofs, kind, path, other)
}

func assertChannelSelect(
	t *testing.T,
	facts []ChannelSelectFact,
	selectID string,
	kind ChannelSelectFactKind,
	result pathdom.Path,
	casePath pathdom.Path,
) {
	t.Helper()
	for _, fact := range facts {
		if fact.Select == selectID &&
			fact.Kind == kind &&
			fact.Result.Equal(result) &&
			fact.Case.Equal(casePath) {
			return
		}
	}
	t.Fatalf("ChannelSelects = %#v, want %q kind %d", facts, selectID, kind)
}

func assertEffectDelta(
	t *testing.T,
	deltas []EffectDelta,
	site string,
	target pathdom.Path,
	kind EffectDeltaKind,
	change EffectDeltaChange,
) {
	t.Helper()
	delta := findEffectDelta(deltas, site)
	if delta == nil || !delta.Target.Equal(target) || delta.Kind != kind || delta.Change != change {
		t.Fatalf("effect delta %q = %#v, want target %s kind %d change %d", site, delta, target, kind, change)
	}
}
