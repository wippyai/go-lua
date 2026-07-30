package state

import "fmt"

// CallBoundaryFactorLanes returns the exact registered residual carriers whose
// value changes merely by crossing an opaque call boundary.
func (d ProductDomain) CallBoundaryFactorLanes() []ProductLane {
	if !d.Valid() {
		return nil
	}
	out := make([]ProductLane, 0, 1)
	for index := range d.factorLanes {
		runtime := &d.factorLanes[index]
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticCallBoundary)
		if declared && law.participates {
			out = append(out, runtime.lane)
		}
	}
	return out
}

func (d ProductDomain) ApplyCallBoundaryFactor(current LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateFactor(current)
	if err != nil {
		return LaneFactor{}, err
	}
	law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticCallBoundary)
	if !declared || !law.participates {
		return LaneFactor{}, fmt.Errorf("%w: lane %q does not participate in call boundary", ErrInvalidLaneFactor, runtime.lane.id)
	}
	next, changed, valid := law.applyFactor(current.payload, callBoundaryRequest{reg: d.reg})
	if !valid {
		return LaneFactor{}, fmt.Errorf("state: lane %q rejected call boundary", runtime.lane.id)
	}
	if !changed {
		return current, nil
	}
	return LaneFactor{lane: runtime.lane, payload: next}, nil
}

// ApplyCallBoundary is the concrete adapter for the factor law. It patches the
// input only after every registered participant succeeds.
func (d ProductDomain) ApplyCallBoundary(input State) (State, error) {
	if !d.Valid() {
		return input, fmt.Errorf("state: invalid call-boundary product domain")
	}
	lanes := d.CallBoundaryFactorLanes()
	factors, err := d.DecomposeLanes(input, lanes)
	if err != nil {
		return input, err
	}
	for index := range factors {
		factors[index], err = d.ApplyCallBoundaryFactor(factors[index])
		if err != nil {
			return input, err
		}
	}
	return d.PatchLaneFactors(input, factors)
}
