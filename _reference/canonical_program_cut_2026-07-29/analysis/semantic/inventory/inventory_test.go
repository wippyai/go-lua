package inventory

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestBaseInventoryMatchesCurrentBuiltins(t *testing.T) {
	inventory, err := Base()
	if err != nil {
		t.Fatal(err)
	}
	wantLanes := []string{
		"values", "path-evidence", "dynamic-index", "heap-table-identity",
		"frozen-tables", "effect-deltas", "escape-events", "channel-select",
		"store-relations", "key-memberships", "typestates", "placement",
		"len-floors", "num-floors", "num-ceils", "diff-relations", "user-lattices",
	}
	wantAxes := []string{
		"variantorigin", "identity", "runtimekind", "typewitness",
		"escape", "evidence", "assertion",
	}
	if got := laneIDs(inventory.StateLanes()); !slices.Equal(got, wantLanes) {
		t.Fatalf("state lanes = %v, want %v", got, wantLanes)
	}
	if got := axisIDs(inventory.ValueAxes()); !slices.Equal(got, wantAxes) {
		t.Fatalf("value axes = %v, want %v", got, wantAxes)
	}
	wantBoundaries := []string{
		"portable-identity", "portable-identity", "portable-identity",
		"portable-identity", "portable-identity", "projected", "portable-identity",
	}
	if got := axisBoundaries(inventory.ValueAxes()); !slices.Equal(got, wantBoundaries) {
		t.Fatalf("axis boundaries = %v, want %v", got, wantBoundaries)
	}
	reducers := inventory.Reducers()
	if len(reducers) != 1 || reducers[0].ID != "typewitness.runtimekind-reduction" ||
		reducers[0].OwnerAxis != "typewitness" ||
		!slices.Equal(reducers[0].Reads, []string{"runtimekind", "typewitness"}) ||
		!slices.Equal(reducers[0].Writes, []string{"typewitness"}) {
		t.Fatalf("reducers = %#v", reducers)
	}
	if inventory.Digest() == ([32]byte{}) || len(inventory.CanonicalBytes()) == 0 {
		t.Fatal("base inventory was not sealed")
	}
	first := inventory.StateLanes()
	first[0] = StateLane{}
	if inventory.StateLanes()[0].ID == "" {
		t.Fatal("StateLanes aliases inventory storage")
	}
}

func TestInventoryRejectsDuplicateReducerStableID(t *testing.T) {
	source := validTestDocument()
	source.Reducers = append(source.Reducers, source.Reducers[0])
	_, err := Parse(mustJSON(t, source))
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `duplicate id "typewitness.runtimekind-reduction"`) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestInventoryRejectsDanglingReducerAxis(t *testing.T) {
	source := validTestDocument()
	source.Reducers[0].Reads = append(source.Reducers[0].Reads, "missing-axis")
	_, err := Parse(mustJSON(t, source))
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `dangling read axis "missing-axis"`) {
		t.Fatalf("dangling reducer error = %v", err)
	}
}

func TestInventoryInputPermutationDoesNotChangeCanonicalBytes(t *testing.T) {
	source := validTestDocument()
	source.StateLanes = append(source.StateLanes, StateLane{ID: "path-evidence", Order: 1})
	source.Reducers = append(source.Reducers, Reducer{
		ID: "runtimekind.identity-reduction", OwnerAxis: "runtimekind",
		Reads: []string{"runtimekind", "typewitness"}, Writes: []string{"runtimekind"},
	})
	base, err := Parse(mustJSON(t, source))
	if err != nil {
		t.Fatal(err)
	}
	var permuted document
	if err := json.Unmarshal(base.CanonicalBytes(), &permuted); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(permuted.StateLanes)
	slices.Reverse(permuted.ValueAxes)
	slices.Reverse(permuted.Reducers)
	for index := range permuted.Reducers {
		slices.Reverse(permuted.Reducers[index].Reads)
		slices.Reverse(permuted.Reducers[index].Writes)
	}
	reparsed, err := Parse(mustJSON(t, permuted))
	if err != nil {
		t.Fatal(err)
	}
	if !base.Equal(reparsed) || !reflect.DeepEqual(base.Digest(), reparsed.Digest()) {
		t.Fatalf("permutation changed inventory:\n%s\n%s", base.CanonicalBytes(), reparsed.CanonicalBytes())
	}
}

func validTestDocument() document {
	return document{
		Schema: Schema,
		StateLanes: []StateLane{
			{ID: "values", Order: 0},
		},
		ValueAxes: []ValueAxis{
			{ID: "runtimekind", Order: 0, Boundary: "portable-identity"},
			{ID: "typewitness", Order: 1, Boundary: "portable-identity"},
		},
		Reducers: []Reducer{
			{ID: "typewitness.runtimekind-reduction", OwnerAxis: "typewitness", Reads: []string{"typewitness", "runtimekind"}, Writes: []string{"typewitness"}},
		},
	}
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func laneIDs(lanes []StateLane) []string {
	out := make([]string, len(lanes))
	for index, lane := range lanes {
		out[index] = lane.ID
	}
	return out
}

func axisIDs(axes []ValueAxis) []string {
	out := make([]string, len(axes))
	for index, axis := range axes {
		out[index] = axis.ID
	}
	return out
}

func axisBoundaries(axes []ValueAxis) []string {
	out := make([]string, len(axes))
	for index, axis := range axes {
		out[index] = axis.Boundary
	}
	return out
}
