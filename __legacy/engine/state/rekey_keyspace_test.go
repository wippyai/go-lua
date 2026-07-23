package state

import (
	"errors"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestRekeyKeySpaceCoversEveryStructuralStateLane(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	container := from.FromPath(pathdom.NewPath(symbol.ID(7), "container").Field("member"))
	wantContainer, ok := to.ImportKey(from, container)
	if !ok {
		t.Fatal("target key import failed")
	}

	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	dynamicKey := dynamicindex.Key{Table: container, Site: "write"}
	dynamicFact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
	})
	effectKey := effectdelta.Key{Target: container, Site: "effect", Kind: effectdelta.Mutation}
	effectValue := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}
	table := pathaddr.StateKey("sym9@1")

	state := Domain(reg).Bottom().
		WriteDynamicIndexFact(reg, dynamicKey, dynamicFact).
		WriteEffectDelta(effectKey, effectValue).
		AddDynamicIndexAllValuesKeyMembership(container, table)
	if _, err := SemanticFingerprint(FingerprintConfig{Registry: reg, KeySpace: to}, state); !errors.Is(err, ErrFingerprintKeySpace) {
		t.Fatalf("foreign state fingerprint error = %v, want ErrFingerprintKeySpace", err)
	}

	rekeyed, err := state.RekeyKeySpace(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SemanticFingerprint(FingerprintConfig{Registry: reg, KeySpace: to}, rekeyed); err != nil {
		t.Fatalf("rekeyed state fingerprint failed: %v", err)
	}
	dynamic := rekeyed.DynamicIndexFactsSnapshot()
	if _, ok := dynamic.Facts[dynamicindex.Key{Table: wantContainer, Site: "write"}]; !ok {
		t.Fatalf("dynamic-index table key was not rebound: %#v", dynamic.Facts)
	}
	effects := rekeyed.EffectDeltasSnapshot()
	if _, ok := effects.Deltas[effectdelta.Key{Target: wantContainer, Site: "effect", Kind: effectdelta.Mutation}]; !ok {
		t.Fatalf("effect-delta target key was not rebound: %#v", effects.Deltas)
	}
	memberships := rekeyed.KeyMembershipsSnapshot().Memberships
	if len(memberships) != 1 || memberships[0].Container != wantContainer {
		t.Fatalf("key-membership container was not rebound: %#v", memberships)
	}
}

func TestRekeyKeySpaceAllowsNilOnlyForKeyFreeState(t *testing.T) {
	reg := standard.Registry()
	valueOnly := Domain(reg).Bottom().WriteValue(reg, statekey.SymbolValue(symbol.ID(31)), product.Top())
	got, err := valueOnly.RekeyKeySpace(nil, nil)
	if err != nil {
		t.Fatalf("key-free nil rekey failed: %v", err)
	}
	if !Domain(reg).Equal(got, valueOnly) {
		t.Fatal("key-free nil rekey changed state")
	}

	valid := keyspace.New()
	copyValue := *valid
	invalid := &copyValue
	got, err = valueOnly.RekeyKeySpace(invalid, invalid)
	if err == nil {
		t.Fatal("key-free state accepted invalid nonnil authority")
	}
	if !Domain(reg).Equal(got, valueOnly) {
		t.Fatal("failed invalid-authority rekey changed state")
	}
}

func TestRekeyKeySpaceValidatesConcreteKeysWithinSameSpace(t *testing.T) {
	reg := standard.Registry()
	claimed, foreign := keyspace.New(), keyspace.New()
	foreignKey := foreign.FromPath(pathdom.NewPath(symbol.ID(32), "foreign").Field("member"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	original := Domain(reg).Bottom().WriteEffectDelta(
		effectdelta.Key{Target: foreignKey, Site: "same-space", Kind: effectdelta.Mutation},
		effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged},
	)
	got, err := original.RekeyKeySpace(claimed, claimed)
	if err == nil {
		t.Fatal("same-space validation accepted a foreign concrete key")
	}
	if !Domain(reg).Equal(got, original) {
		t.Fatal("failed same-space validation changed state")
	}
}

func TestRekeyKeySpacePreservesMustFactsBesideDynamicTop(t *testing.T) {
	reg := standard.Registry()
	from, to := keyspace.New(), keyspace.New()
	container := from.FromPath(pathdom.NewPath(symbol.ID(21), "container"))
	wantContainer, ok := to.ImportKey(from, container)
	if !ok {
		t.Fatal("target key import failed")
	}
	table := pathaddr.StateKey("sym22@1")

	state := Domain(reg).Top().AddDynamicIndexAllValuesKeyMembership(container, table)
	if !state.keyMemberships.dynamicTop {
		t.Fatal("test setup lost dynamic-top marker")
	}
	rekeyed, err := state.RekeyKeySpace(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !rekeyed.keyMemberships.dynamicTop {
		t.Fatal("rekey lost dynamic-top marker")
	}
	if got := rekeyed.DynamicIndexAllValuesKeyMembershipTables(wantContainer); len(got) != 1 || got[0] != table {
		t.Fatalf("rekeyed dynamic-all must fact = %#v, want [%q]", got, table)
	}
	if got := rekeyed.DynamicIndexAllValuesKeyMembershipTables(container); len(got) != 0 {
		t.Fatalf("source-owned dynamic-all fact survived rekey: %#v", got)
	}
}

func TestLaneCatalogRequiresExplicitKeySpaceOwnership(t *testing.T) {
	tests := []struct {
		name string
		spec laneSpec
	}{
		{
			name: "missing policy",
			spec: laneSpec{id: "test.missing-keyspace-policy"},
		},
		{
			name: "owned without rekey",
			spec: laneSpec{id: "test.owned-without-rekey", keySpaceMode: laneKeySpaceOwned},
		},
		{
			name: "free with rekey",
			spec: laneSpec{
				id:           "test.free-with-rekey",
				keySpaceMode: laneKeySpaceFree,
				rekey:        func(s State, _, _ *keyspace.KeySpace) (State, bool) { return s, true },
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.spec.fingerprint = func(*fingerprintWriter, State) {}
			defer func() {
				if recover() == nil {
					t.Fatalf("invalid lane registration was accepted: %#v", test.spec)
				}
			}()
			_ = newLaneCatalog([]laneSpec{test.spec})
		})
	}
}
