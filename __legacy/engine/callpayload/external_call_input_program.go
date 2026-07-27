package callpayload

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ExternalCallInputWireLayout is one frozen transfer input. Values and lanes
// are dense and descriptor-ordered; provider code addresses them by ordinal,
// never by scanning State or a product inventory.
type ExternalCallInputWireLayout[K comparable] struct {
	point                    cfg.Point
	valueRoots               []K
	valueOrdinals            map[K]int
	lanes                    []state.ProductLane
	typestateResourceQueries []state.TypestateResourceQuery
	readsDiagnostics         bool
	readsReachable           bool
}

func (l ExternalCallInputWireLayout[K]) Point() cfg.Point { return l.point }
func (l ExternalCallInputWireLayout[K]) ValueRoots() []K  { return append([]K(nil), l.valueRoots...) }
func (l ExternalCallInputWireLayout[K]) Lanes() []state.ProductLane {
	return append([]state.ProductLane(nil), l.lanes...)
}
func (l ExternalCallInputWireLayout[K]) TypestateResourceQueries() []state.TypestateResourceQuery {
	return append([]state.TypestateResourceQuery(nil), l.typestateResourceQueries...)
}
func (l ExternalCallInputWireLayout[K]) ReadsDiagnostics() bool { return l.readsDiagnostics }
func (l ExternalCallInputWireLayout[K]) ReadsReachable() bool   { return l.readsReachable }
func (l ExternalCallInputWireLayout[K]) ValueOrdinal(root K) (int, bool) {
	ordinal, ok := l.valueOrdinals[root]
	return ordinal, ok
}

// ExternalCallInputWireOperands is the adapter-facing construction syntax for
// one wire. It has no State and contains exactly the layout-owned fibers.
type ExternalCallInputWireOperands struct {
	Values                        []product.Value
	ValuesTop                     bool
	Factors                       []state.LaneFactor
	TypestateResourceObservations []state.TypestateResourceObservation
	Diagnostics                   DiagnosticOutput
	Reachable                     bool
}

// ExternalCallInputWire is an immutable, sealed provider input. Its slices are
// private so a provider cannot widen its authority after binding.
type ExternalCallInputWire struct {
	values                        []product.Value
	valuesTop                     bool
	factors                       []state.LaneFactor
	factorByID                    map[state.LaneID]state.LaneFactor
	typestateResourceObservations []state.TypestateResourceObservation
	diagnostics                   DiagnosticOutput
	reachable                     bool
}

func (w ExternalCallInputWire) Value(ordinal int) (product.Value, bool) {
	if ordinal < 0 || ordinal >= len(w.values) {
		return product.Value{}, false
	}
	if w.valuesTop {
		return product.Top(), true
	}
	return w.values[ordinal], true
}
func (w ExternalCallInputWire) ValuesTop() bool { return w.valuesTop }
func (w ExternalCallInputWire) Factors() []state.LaneFactor {
	return append([]state.LaneFactor(nil), w.factors...)
}

// Factor returns the exact sealed component for one registered semantic lane.
// The lookup is prepared when the frame binds; provider execution never scans
// the product inventory to discover a lane.
func (w ExternalCallInputWire) Factor(id state.LaneID) (state.LaneFactor, bool) {
	factor, ok := w.factorByID[id]
	return factor, ok
}
func (w ExternalCallInputWire) TypestateResourceObservation(index int) (state.TypestateResourceObservation, bool) {
	if index < 0 || index >= len(w.typestateResourceObservations) {
		return state.TypestateResourceObservation{}, false
	}
	return w.typestateResourceObservations[index], true
}

// ObserveTypestateResource returns the typed result bound for the exact sealed
// query identity. Provider composition may reorder or deduplicate queries, so
// semantic evaluators must never address them by a local ordinal.
func (w ExternalCallInputWire) ObserveTypestateResource(query state.TypestateResourceQuery) (state.TypestateResourceObservation, bool) {
	for _, observation := range w.typestateResourceObservations {
		if observation.ValidFor(query) {
			return observation, true
		}
	}
	return state.TypestateResourceObservation{}, false
}
func (w ExternalCallInputWire) Diagnostics() DiagnosticOutput {
	return w.diagnostics.Clone()
}
func (w ExternalCallInputWire) Reachable() bool { return w.reachable }

type externalCallInputProgramSeal struct{}

// ExternalCallInputProgram is the closed provider read authority. The primary
// role and historical point groups are resolved once; execution receives no
// read(point) callback and cannot discover another input.
type ExternalCallInputProgram[K comparable] struct {
	domain     state.ProductDomain
	layouts    []ExternalCallInputWireLayout[K]
	primary    int
	historical map[cfg.Point][]int
	seal       *externalCallInputProgramSeal
}

// ExternalCallInputFrame is one factor-native invocation of a sealed program.
type ExternalCallInputFrame[K comparable] struct {
	program *ExternalCallInputProgram[K]
	wires   []ExternalCallInputWire
}

// CallOutcomeValueOperands is the canonical source-role tuple evaluated from
// the existing ValueTerm DAG before provider execution. Missing roles are
// represented by explicit presence flags because product Bottom is a valid
// semantic value, not absence.
type CallOutcomeValueOperands struct {
	Callee      product.Value
	HasCallee   bool
	Receiver    product.Value
	HasReceiver bool
	Arguments   []CallOutcomeArgumentOperand
}

type CallOutcomeArgumentOperand struct {
	Value   product.Value
	Present bool
}

// CallOutcomeInput is the complete State-free provider input. Source roles
// are already-evaluated ValueTerms; primary/historical wires contain only the
// exact registered factors declared by TransferAccess.
type CallOutcomeInput struct {
	reg        *axis.Registry
	domain     state.ProductDomain
	primary    ExternalCallInputWire
	historical map[cfg.Point][]ExternalCallInputWire
	operands   CallOutcomeValueOperands
}

func (i CallOutcomeInput) Callee() (product.Value, bool) {
	return i.operands.Callee, i.operands.HasCallee
}
func (i CallOutcomeInput) Receiver() (product.Value, bool) {
	return i.operands.Receiver, i.operands.HasReceiver
}
func (i CallOutcomeInput) Argument(index int) (product.Value, bool) {
	if index < 0 || index >= len(i.operands.Arguments) {
		return product.Value{}, false
	}
	argument := i.operands.Arguments[index]
	return argument.Value, argument.Present
}
func (i CallOutcomeInput) ArgumentCount() int { return len(i.operands.Arguments) }
func (i CallOutcomeInput) Domain() state.ProductDomain {
	if i.reg == nil {
		return state.ProductDomain{}
	}
	return i.domain
}
func (i CallOutcomeInput) Primary() ExternalCallInputWire {
	return i.primary
}
func (i CallOutcomeInput) Historical(point cfg.Point) []ExternalCallInputWire {
	return append([]ExternalCallInputWire(nil), i.historical[point]...)
}

// HistoricalFactor joins exactly the sealed predecessor components for point
// in their stable wire order. It is the factor-native replacement for the old
// hidden read(point) whole-State join.
func (i CallOutcomeInput) HistoricalFactor(point cfg.Point, lane state.LaneID) (state.LaneFactor, bool, error) {
	wires := i.historical[point]
	var out state.LaneFactor
	found := false
	for _, wire := range wires {
		factor, ok := wire.Factor(lane)
		if !ok {
			continue
		}
		if !found {
			out, found = factor, true
			continue
		}
		var err error
		out, err = i.domain.LaneJoin(out, factor)
		if err != nil {
			return state.LaneFactor{}, false, err
		}
	}
	return out, found, nil
}

// BindCallOutcomeInput closes evaluated source roles and the exact factor
// wires into one provider invocation. It accepts any root vocabulary because
// physical root identities have already disappeared behind dense wire slots.
func (f ExternalCallInputFrame[K]) BindCallOutcomeInput(operands CallOutcomeValueOperands) (CallOutcomeInput, error) {
	if f.program == nil || !f.program.Valid() || len(f.wires) != len(f.program.layouts) {
		return CallOutcomeInput{}, fmt.Errorf("callpayload: invalid call-outcome input frame")
	}
	reg := f.program.domain.Registry()
	if operands.HasCallee && !product.BelongsToRegistry(reg, operands.Callee) ||
		operands.HasReceiver && !product.BelongsToRegistry(reg, operands.Receiver) {
		return CallOutcomeInput{}, fmt.Errorf("callpayload: foreign call-outcome source operand")
	}
	for index, argument := range operands.Arguments {
		if argument.Present && !product.BelongsToRegistry(reg, argument.Value) {
			return CallOutcomeInput{}, fmt.Errorf("callpayload: foreign call-outcome argument %d", index)
		}
	}
	operands.Arguments = append([]CallOutcomeArgumentOperand(nil), operands.Arguments...)
	out := CallOutcomeInput{
		reg: reg, domain: f.program.domain, primary: f.wires[f.program.primary], operands: operands,
		historical: make(map[cfg.Point][]ExternalCallInputWire, len(f.program.historical)),
	}
	for point, ordinals := range f.program.historical {
		wires := make([]ExternalCallInputWire, len(ordinals))
		for index, ordinal := range ordinals {
			wires[index] = f.wires[ordinal]
		}
		out.historical[point] = wires
	}
	return out, nil
}

// PrepareExternalCallInputProgram binds canonical TransferAccess to an
// arbitrary collision-free root vocabulary. Inventory traversal occurs once
// here; BindFrame and provider reads use only sealed dense operands.
func PrepareExternalCallInputProgram[K comparable](
	domain state.ProductDomain,
	access state.TransferAccess,
	inputPoints []cfg.Point,
	primary int,
	bind func(statekey.Value) (K, bool),
) (ExternalCallInputProgram[K], error) {
	if !domain.Valid() || !access.Valid() || access.ProviderInputCount() != len(inputPoints) ||
		primary < 0 || primary >= len(inputPoints) || bind == nil {
		return ExternalCallInputProgram[K]{}, fmt.Errorf("factapply: invalid external-call input program")
	}
	program := ExternalCallInputProgram[K]{
		domain: domain, layouts: make([]ExternalCallInputWireLayout[K], len(inputPoints)),
		primary: primary, historical: make(map[cfg.Point][]int), seal: &externalCallInputProgramSeal{},
	}
	inventory := domain.LaneInventory()
	for wire := range inputPoints {
		input, ok := access.ProviderInput(wire)
		if !ok {
			return ExternalCallInputProgram[K]{}, fmt.Errorf("factapply: missing external-call input role %d", wire)
		}
		layout := ExternalCallInputWireLayout[K]{
			point: inputPoints[wire], valueRoots: make([]K, 0, len(input.Values)),
			valueOrdinals:    make(map[K]int, len(input.Values)),
			readsDiagnostics: input.Diagnostics, readsReachable: input.Reachable,
		}
		for _, slot := range input.Values {
			root, bound := bind(slot)
			if !bound {
				return ExternalCallInputProgram[K]{}, fmt.Errorf("factapply: unresolved external-call input Values root")
			}
			if _, duplicate := layout.valueOrdinals[root]; duplicate {
				return ExternalCallInputProgram[K]{}, fmt.Errorf("factapply: external-call input roots are not injective")
			}
			layout.valueOrdinals[root] = len(layout.valueRoots)
			layout.valueRoots = append(layout.valueRoots, root)
		}
		for _, lane := range inventory {
			if input.Lanes.Has(lane.ID()) {
				layout.lanes = append(layout.lanes, lane)
			}
		}
		if wire != primary && len(input.TypestateResourceQueries) != 0 {
			return ExternalCallInputProgram[K]{}, fmt.Errorf("factapply: keyed typestate queries are primary-input only")
		}
		layout.typestateResourceQueries = append([]state.TypestateResourceQuery(nil), input.TypestateResourceQueries...)
		program.layouts[wire] = layout
		if wire != primary {
			program.historical[inputPoints[wire]] = append(program.historical[inputPoints[wire]], wire)
		}
	}
	return program, nil
}

func (p ExternalCallInputProgram[K]) Valid() bool {
	return p.seal != nil && p.domain.Valid() && len(p.layouts) != 0 && p.primary >= 0 && p.primary < len(p.layouts)
}
func (p ExternalCallInputProgram[K]) InputCount() int { return len(p.layouts) }
func (p ExternalCallInputProgram[K]) Layout(index int) (ExternalCallInputWireLayout[K], bool) {
	if !p.Valid() || index < 0 || index >= len(p.layouts) {
		return ExternalCallInputWireLayout[K]{}, false
	}
	layout := p.layouts[index]
	layout.valueRoots = append([]K(nil), layout.valueRoots...)
	layout.valueOrdinals = make(map[K]int, len(p.layouts[index].valueOrdinals))
	for root, ordinal := range p.layouts[index].valueOrdinals {
		layout.valueOrdinals[root] = ordinal
	}
	layout.lanes = append([]state.ProductLane(nil), layout.lanes...)
	layout.typestateResourceQueries = append([]state.TypestateResourceQuery(nil), layout.typestateResourceQueries...)
	return layout, true
}

// BindFrame validates exact widths, descriptor order, registry ownership and
// declared diagnostic/reachability access, then detaches every operand.
func (p *ExternalCallInputProgram[K]) BindFrame(operands []ExternalCallInputWireOperands) (ExternalCallInputFrame[K], error) {
	if p == nil || !p.Valid() || len(operands) != len(p.layouts) {
		return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: invalid external-call input frame")
	}
	frame := ExternalCallInputFrame[K]{program: p, wires: make([]ExternalCallInputWire, len(operands))}
	reg := p.domain.Registry()
	for wire, input := range operands {
		layout := p.layouts[wire]
		if input.ValuesTop {
			if len(input.Values) != 0 {
				return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: external-call Values Top has finite operands")
			}
		} else if len(input.Values) != len(layout.valueRoots) {
			return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: external-call input Values width mismatch")
		}
		for ordinal, value := range input.Values {
			if !product.BelongsToRegistry(reg, value) {
				return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: foreign external-call input value %d/%d", wire, ordinal)
			}
		}
		if len(input.Factors) != len(layout.lanes) {
			return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: external-call input factor width mismatch")
		}
		for ordinal, factor := range input.Factors {
			if factor.Lane() != layout.lanes[ordinal] {
				return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: reordered external-call input factor %d/%d", wire, ordinal)
			}
		}
		if len(input.TypestateResourceObservations) != len(layout.typestateResourceQueries) {
			return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: external-call keyed typestate observation width mismatch")
		}
		for ordinal, observation := range input.TypestateResourceObservations {
			if !observation.ValidFor(layout.typestateResourceQueries[ordinal]) {
				return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: foreign external-call keyed typestate observation %d/%d", wire, ordinal)
			}
		}
		if !layout.readsDiagnostics && !input.Diagnostics.Empty() {
			return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: undeclared external-call diagnostic input")
		}
		if layout.readsDiagnostics && !input.Diagnostics.Valid(reg) {
			return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: invalid external-call diagnostic input")
		}
		if !layout.readsReachable && input.Reachable {
			return ExternalCallInputFrame[K]{}, fmt.Errorf("factapply: undeclared external-call reachability input")
		}
		factorByID := make(map[state.LaneID]state.LaneFactor, len(input.Factors))
		for _, factor := range input.Factors {
			factorByID[factor.Lane().ID()] = factor
		}
		frame.wires[wire] = ExternalCallInputWire{
			values: append([]product.Value(nil), input.Values...), valuesTop: input.ValuesTop,
			factors: append([]state.LaneFactor(nil), input.Factors...), factorByID: factorByID,
			typestateResourceObservations: append([]state.TypestateResourceObservation(nil), input.TypestateResourceObservations...),
			diagnostics:                   input.Diagnostics.Clone(), reachable: input.Reachable,
		}
	}
	return frame, nil
}

func (f ExternalCallInputFrame[K]) Primary() (ExternalCallInputWire, ExternalCallInputWireLayout[K], bool) {
	if f.program == nil || !f.program.Valid() || len(f.wires) != len(f.program.layouts) {
		return ExternalCallInputWire{}, ExternalCallInputWireLayout[K]{}, false
	}
	layout, _ := f.program.Layout(f.program.primary)
	return f.wires[f.program.primary], layout, true
}

func (f ExternalCallInputFrame[K]) Domain() state.ProductDomain {
	if f.program == nil || !f.program.Valid() {
		return state.ProductDomain{}
	}
	return f.program.domain
}

func (f ExternalCallInputFrame[K]) Input(index int) (ExternalCallInputWire, ExternalCallInputWireLayout[K], bool) {
	if f.program == nil || !f.program.Valid() || len(f.wires) != len(f.program.layouts) || index < 0 || index >= len(f.wires) {
		return ExternalCallInputWire{}, ExternalCallInputWireLayout[K]{}, false
	}
	layout, _ := f.program.Layout(index)
	return f.wires[index], layout, true
}

// Historical returns every sealed predecessor wire for point. Multiple wires
// remain explicit: the provider must use its declared lattice join rather than
// receiving a hidden read(point) State merge.
func (f ExternalCallInputFrame[K]) Historical(point cfg.Point) ([]ExternalCallInputWire, []ExternalCallInputWireLayout[K]) {
	if f.program == nil || !f.program.Valid() || len(f.wires) != len(f.program.layouts) {
		return nil, nil
	}
	ordinals := f.program.historical[point]
	wires := make([]ExternalCallInputWire, len(ordinals))
	layouts := make([]ExternalCallInputWireLayout[K], len(ordinals))
	for index, ordinal := range ordinals {
		wires[index] = f.wires[ordinal]
		layouts[index], _ = f.program.Layout(ordinal)
	}
	return wires, layouts
}

// JoinHistorical joins the exact predecessor wires for point in their stable
// sealed order. Layout equality is checked before any lattice operation, so a
// caller cannot accidentally merge different physical root vocabularies.
func (f ExternalCallInputFrame[K]) JoinHistorical(point cfg.Point) (ExternalCallInputWire, ExternalCallInputWireLayout[K], bool, error) {
	wires, layouts := f.Historical(point)
	if len(wires) == 0 {
		return ExternalCallInputWire{}, ExternalCallInputWireLayout[K]{}, false, nil
	}
	layout := layouts[0]
	out := wires[0]
	for index := 1; index < len(wires); index++ {
		if !externalCallWireLayoutsEqual(layout, layouts[index]) {
			return ExternalCallInputWire{}, ExternalCallInputWireLayout[K]{}, false, fmt.Errorf("callpayload: historical external-call wire layout drift")
		}
		var err error
		out, err = joinExternalCallInputWire(f.program.domain, out, wires[index])
		if err != nil {
			return ExternalCallInputWire{}, ExternalCallInputWireLayout[K]{}, false, err
		}
	}
	return out, layout, true, nil
}

func externalCallWireLayoutsEqual[K comparable](left, right ExternalCallInputWireLayout[K]) bool {
	if left.point != right.point || left.readsDiagnostics != right.readsDiagnostics || left.readsReachable != right.readsReachable ||
		len(left.valueRoots) != len(right.valueRoots) || len(left.lanes) != len(right.lanes) ||
		len(left.typestateResourceQueries) != len(right.typestateResourceQueries) {
		return false
	}
	for index := range left.valueRoots {
		if left.valueRoots[index] != right.valueRoots[index] {
			return false
		}
	}
	for index := range left.lanes {
		if left.lanes[index] != right.lanes[index] {
			return false
		}
	}
	for index := range left.typestateResourceQueries {
		if !left.typestateResourceQueries[index].Equal(right.typestateResourceQueries[index]) {
			return false
		}
	}
	return true
}

func joinExternalCallInputWire(domain state.ProductDomain, left, right ExternalCallInputWire) (ExternalCallInputWire, error) {
	if !domain.Valid() || len(left.values) != len(right.values) || len(left.factors) != len(right.factors) ||
		len(left.typestateResourceObservations) != 0 || len(right.typestateResourceObservations) != 0 {
		return ExternalCallInputWire{}, fmt.Errorf("callpayload: incompatible external-call input wires")
	}
	out := ExternalCallInputWire{valuesTop: left.valuesTop || right.valuesTop, reachable: left.reachable || right.reachable}
	if !out.valuesTop {
		out.values = make([]product.Value, len(left.values))
		for index := range left.values {
			out.values[index] = domain.ValueJoin(left.values[index], right.values[index])
		}
	}
	out.factors = make([]state.LaneFactor, len(left.factors))
	out.factorByID = make(map[state.LaneID]state.LaneFactor, len(left.factors))
	for index := range left.factors {
		factor, err := domain.LaneJoin(left.factors[index], right.factors[index])
		if err != nil {
			return ExternalCallInputWire{}, err
		}
		out.factors[index] = factor
		out.factorByID[factor.Lane().ID()] = factor
	}
	out.diagnostics = left.diagnostics.Join(domain.Registry(), right.diagnostics)
	return out, nil
}

// BindConcreteExternalCallInputFrame is the sole concrete State read adapter.
// It projects only the sealed roots and lanes and immediately discards State;
// providers receive the returned factor frame.
func BindConcreteExternalCallInputFrame(
	program *ExternalCallInputProgram[statekey.Value],
	inputs []state.State,
	diagnostics []DiagnosticOutput,
) (ExternalCallInputFrame[statekey.Value], error) {
	if program == nil || !program.Valid() || len(inputs) != len(program.layouts) ||
		len(diagnostics) != len(inputs) {
		return ExternalCallInputFrame[statekey.Value]{}, fmt.Errorf("factapply: invalid concrete external-call input frame")
	}
	operands := make([]ExternalCallInputWireOperands, len(inputs))
	reg := program.domain.Registry()
	bottom := program.domain.Lattice().Bottom()
	for wire, input := range inputs {
		layout := program.layouts[wire]
		_, valueFactor := state.DecomposeValueLane(program.domain.Lattice(), input)
		operands[wire].ValuesTop = valueFactor.Top
		if !valueFactor.Top {
			operands[wire].Values = make([]product.Value, len(layout.valueRoots))
			for ordinal, slot := range layout.valueRoots {
				if value, present := valueFactor.Values[slot]; present {
					operands[wire].Values[ordinal] = value
				} else {
					operands[wire].Values[ordinal] = product.Bottom(reg)
				}
			}
		}
		factors, err := program.domain.DecomposeLanes(input, layout.lanes)
		if err != nil {
			return ExternalCallInputFrame[statekey.Value]{}, err
		}
		operands[wire].Factors = factors
		operands[wire].TypestateResourceObservations = make([]state.TypestateResourceObservation, len(layout.typestateResourceQueries))
		for ordinal, query := range layout.typestateResourceQueries {
			observation, err := query.ObserveState(program.domain, input)
			if err != nil {
				return ExternalCallInputFrame[statekey.Value]{}, err
			}
			operands[wire].TypestateResourceObservations[ordinal] = observation
		}
		if layout.readsDiagnostics {
			operands[wire].Diagnostics = diagnostics[wire]
		}
		if layout.readsReachable {
			operands[wire].Reachable = !program.domain.Lattice().Equal(input, bottom)
		}
	}
	return program.BindFrame(operands)
}
