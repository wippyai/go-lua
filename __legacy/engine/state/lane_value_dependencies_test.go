package state

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
)

func TestLaneCatalogRequiresExplicitValueDependencyPolicy(t *testing.T) {
	tests := []struct {
		name string
		edit func(*laneSpec)
		want string
	}{
		{
			name: "missing declaration",
			edit: func(spec *laneSpec) { spec.valueDependencies = laneValueDependencyPolicy{} },
			want: "has no Values dependency declaration",
		},
		{
			name: "independent with enumerator",
			edit: func(spec *laneSpec) {
				spec.valueDependencies = laneValueDependencyPolicy{
					kind:  laneValueDependenciesIndependent,
					visit: func(State, *keyspace.KeySpace, func(statekey.ValueDependency)) {},
				}
			},
			want: "Values-independent lane",
		},
		{
			name: "enumerated without enumerator",
			edit: func(spec *laneSpec) {
				spec.valueDependencies = laneValueDependencyPolicy{kind: laneValueDependenciesEnumerated}
			},
			want: "Values-dependent lane",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := valuesLaneSpec
			spec.id = LaneID("test." + test.name)
			test.edit(&spec)
			defer func() {
				got := recover()
				if got == nil || !strings.Contains(fmt.Sprint(got), test.want) {
					t.Fatalf("newLaneCatalog panic = %v, want %q", got, test.want)
				}
			}()
			_ = newLaneCatalog([]laneSpec{spec})
		})
	}
}

func TestDefaultLaneCatalogDeclaresValueDependenciesForEveryLane(t *testing.T) {
	want := map[LaneID]laneValueDependencyKind{
		LaneValues:            laneValueDependenciesIndependent,
		LanePathEvidence:      laneValueDependenciesEnumerated,
		LaneDynamicIndex:      laneValueDependenciesIndependent,
		LaneHeapTableIdentity: laneValueDependenciesIndependent,
		LaneFrozenTables:      laneValueDependenciesIndependent,
		LaneEffectDeltas:      laneValueDependenciesIndependent,
		LaneEscapeEvents:      laneValueDependenciesIndependent,
		LaneChannelSelect:     laneValueDependenciesIndependent,
		LaneStoreRelations:    laneValueDependenciesIndependent,
		LaneKeyMemberships:    laneValueDependenciesIndependent,
		LaneTypestates:        laneValueDependenciesIndependent,
		LanePlacement:         laneValueDependenciesIndependent,
		LaneLenFloors:         laneValueDependenciesIndependent,
		LaneNumFloors:         laneValueDependenciesIndependent,
		LaneNumCeils:          laneValueDependenciesIndependent,
		LaneDiffRelations:     laneValueDependenciesIndependent,
		LaneUserLattices:      laneValueDependenciesIndependent,
	}
	if got := len(defaultLaneCatalog.specs); got != len(want) {
		t.Fatalf("default lane inventory has %d lanes, want %d", got, len(want))
	}
	for _, spec := range defaultLaneCatalog.specs {
		kind, ok := want[spec.id]
		if !ok {
			t.Errorf("lane %q is absent from Values dependency inventory", spec.id)
			continue
		}
		if spec.valueDependencies.kind != kind {
			t.Errorf("lane %q Values dependency kind = %d, want %d", spec.id, spec.valueDependencies.kind, kind)
		}
		delete(want, spec.id)
	}
	for id := range want {
		t.Errorf("Values dependency inventory names unregistered lane %q", id)
	}
}
