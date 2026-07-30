package gen

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/wippyai/go-lua/analysis/semantic/inventory"
)

type Bindings struct {
	StateLanes []StateLaneBinding `json:"state_lanes"`
	ValueAxes  []ValueAxisBinding `json:"value_axes"`
}

type StateLaneBinding struct {
	ID         string `json:"id"`
	IDSymbol   string `json:"id_symbol"`
	SpecSymbol string `json:"spec_symbol"`
	BitSymbol  string `json:"bit_symbol"`
}
type ValueAxisBinding struct {
	ID         string `json:"id"`
	Alias      string `json:"alias"`
	ImportPath string `json:"import_path"`
	SpecSymbol string `json:"spec_symbol"`
}

func DecodeBindings(r io.Reader) (Bindings, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var out Bindings
	if err := dec.Decode(&out); err != nil {
		return Bindings{}, err
	}
	return out, nil
}

func orderedBindings(in inventory.Inventory, bindings Bindings) ([]StateLaneBinding, []ValueAxisBinding, error) {
	stateByID := make(map[string]StateLaneBinding, len(bindings.StateLanes))
	for _, binding := range bindings.StateLanes {
		stateByID[binding.ID] = binding
	}
	states := make([]StateLaneBinding, 0, len(stateByID))
	for _, lane := range in.StateLanes() {
		binding, ok := stateByID[lane.ID]
		if !ok || binding.IDSymbol == "" || binding.SpecSymbol == "" || binding.BitSymbol == "" {
			return nil, nil, fmt.Errorf("state lane %q has no complete generator binding", lane.ID)
		}
		states = append(states, binding)
		delete(stateByID, lane.ID)
	}
	if len(stateByID) != 0 {
		return nil, nil, fmt.Errorf("generator bindings contain unknown state lanes")
	}
	axisByID := make(map[string]ValueAxisBinding, len(bindings.ValueAxes))
	for _, binding := range bindings.ValueAxes {
		axisByID[binding.ID] = binding
	}
	axes := make([]ValueAxisBinding, 0, len(axisByID))
	for _, axis := range in.ValueAxes() {
		binding, ok := axisByID[axis.ID]
		if !ok || binding.Alias == "" || binding.ImportPath == "" || binding.SpecSymbol == "" {
			return nil, nil, fmt.Errorf("value axis %q has no complete generator binding", axis.ID)
		}
		axes = append(axes, binding)
		delete(axisByID, axis.ID)
	}
	if len(axisByID) != 0 {
		return nil, nil, fmt.Errorf("generator bindings contain unknown value axes")
	}
	return states, axes, nil
}
