package body

import (
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

const ClosureCaptureFactSchemaVersion = 3

type ClosureCapturePolicy uint8

const (
	ClosureCapturePolicyUnknown ClosureCapturePolicy = iota
	ClosureCapturePolicyFull
	ClosureCapturePolicyWriteInvariant
)

func (p ClosureCapturePolicy) String() string {
	switch p {
	case ClosureCapturePolicyFull:
		return "full"
	case ClosureCapturePolicyWriteInvariant:
		return "write-invariant"
	default:
		return "unknown"
	}
}

// ClosureCaptureFact is the solved closure-site export for one captured symbol.
// It records only facts that are valid at closure-body entry under Policy.
type ClosureCaptureFact struct {
	SchemaVersion int
	Point         cfg.Point
	ExpressionID  wir.ExpressionID
	Function      symbol.ID
	CaptureIndex  int
	Symbol        symbol.ID
	Name          string
	Path          pathdom.Path
	Policy        ClosureCapturePolicy

	Value product.Value

	Type    typ.Type
	HasType bool

	Shape       typ.Type
	HasShape    bool
	StableShape bool
	ShapeTier   StableShapeTier
	Fields      []StableShapeField

	Nilable         bool
	NilabilityKnown bool

	Placement    placement.Value
	HasPlacement bool
	Identity     identity.ID
	HasIdentity  bool
}

// ClosureCaptureFacts returns the capture facts attached to OpClosure
// instructions at point, in WIR instruction order and capture order.
func (r *Result) ClosureCaptureFacts(point cfg.Point) []ClosureCaptureFact {
	if r == nil || r.wir == nil || r.registry == nil {
		return nil
	}
	var out []ClosureCaptureFact
	closureIndex := 0
	for _, inst := range r.wir.PointInstructions(point) {
		if inst.Op != wir.OpClosure {
			continue
		}
		out = append(out, r.closureCaptureFactsForInstruction(point, closureIndex, inst)...)
		closureIndex++
	}
	return out
}

// ForEachClosureCaptureFact visits solved capture exports in deterministic RPO
// order. Returning false stops iteration.
func (r *Result) ForEachClosureCaptureFact(visit func(ClosureCaptureFact) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.Graph().RPO() {
		for _, fact := range r.ClosureCaptureFacts(point) {
			visited = true
			if !visit(fact) {
				return true
			}
		}
	}
	return visited
}

func (r *Result) closureCaptureFactsForInstruction(point cfg.Point, _ int, inst wir.Instruction) []ClosureCaptureFact {
	if inst.Func == 0 {
		return nil
	}
	proto := r.wir.Proto(inst.Func)
	if proto.Symbol == 0 {
		return nil
	}
	captures := r.wir.Operands(inst.List)
	out := make([]ClosureCaptureFact, 0, len(captures))
	for i, op := range captures {
		if op.Kind != wir.OperandPath {
			continue
		}
		p := r.wir.Path(wir.PathRef(op.Ref))
		if p.IsEmpty() || p.Symbol == 0 {
			continue
		}
		fact, ok := r.closureCaptureFact(point, inst, proto, i, p)
		if ok {
			out = append(out, fact)
		}
	}
	return out
}

func (r *Result) closureCaptureFact(point cfg.Point, inst wir.Instruction, proto wir.FuncProto, index int, p pathdom.Path) (ClosureCaptureFact, bool) {
	policy := r.closureCapturePolicy(p.Symbol)
	value, ok := r.closureCaptureValue(point, p.Symbol, policy)
	if !ok {
		value = product.Top()
	}
	fact := ClosureCaptureFact{
		SchemaVersion: ClosureCaptureFactSchemaVersion,
		Point:         point,
		ExpressionID:  inst.ExprID,
		Function:      symbol.ID(proto.Symbol),
		CaptureIndex:  index,
		Symbol:        p.Symbol,
		Name:          p.Root,
		Path:          p.Clone(),
		Policy:        policy,
		Value:         value,
	}
	fact.Type, fact.HasType = typevalue.TypeOf(r.registry, value)
	fact.Nilable, fact.NilabilityKnown = closureCaptureNilability(value, fact.Type, fact.HasType)
	fact.Shape, fact.HasShape, fact.StableShape, fact.ShapeTier, fact.Fields = r.closureCaptureShape(point, value)
	fact.Identity, fact.HasIdentity = identityvalue.ExactID(r.registry, value)
	if fact.HasIdentity {
		if st, ok := r.StateAtBoundary(point); ok {
			fact.Placement = st.ReadPlacement(fact.Identity)
			fact.HasPlacement = !fact.Placement.IsBottom()
		}
	}
	return fact, true
}

func (r *Result) closureCapturePolicy(id symbol.ID) ClosureCapturePolicy {
	if id == 0 {
		return ClosureCapturePolicyUnknown
	}
	if r.SymbolHasWrite(id) {
		return ClosureCapturePolicyWriteInvariant
	}
	return ClosureCapturePolicyFull
}

func (r *Result) closureCaptureValue(point cfg.Point, id symbol.ID, policy ClosureCapturePolicy) (product.Value, bool) {
	if r == nil || r.registry == nil || id == 0 {
		return product.Value{}, false
	}
	if policy == ClosureCapturePolicyWriteInvariant {
		if value, ok := r.writeInvariantCaptureValue(point, id); ok {
			return value, true
		}
		return product.Value{}, false
	}
	if value, ok := r.SymbolValueAtBoundary(point, id); ok {
		return value, true
	}
	if value, ok := r.UninitializedLocalDeclarationValueAtBoundary(point, id); ok {
		return value, true
	}
	st, ok := r.StateAtBoundary(point)
	if !ok {
		return product.Value{}, false
	}
	slot := statekey.SymbolValue(id)
	if slot == 0 {
		return product.Value{}, false
	}
	value := st.ReadValue(r.registry, slot)
	if product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func (r *Result) writeInvariantCaptureValue(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if r.SymbolHasWrite(id) {
		if t, ok := r.SymbolDeclaredType(id); ok && t != nil {
			return r.typeValues.FromTypeWithWitness(r.registry, t), true
		}
		return product.Value{}, false
	}
	if t, ok := r.SymbolStaticType(id); ok && t != nil {
		if value, ok := r.SymbolValueAtBoundary(point, id); ok {
			if shape, ok := r.StableShapeForValueAtBoundary(point, value); ok && shape.Shape != nil {
				return r.typeValues.FromTypeWithWitness(r.registry, shape.Shape), true
			}
		}
		return r.typeValues.FromTypeWithWitness(r.registry, t), true
	}
	if value, ok := r.SymbolValueAtBoundary(point, id); ok {
		if shape, ok := r.StableShapeForValueAtBoundary(point, value); ok && shape.Shape != nil {
			return r.typeValues.FromTypeWithWitness(r.registry, shape.Shape), true
		}
		if t, ok := typevalue.StructuralTypeOf(r.registry, r.typeValues, value, typevalue.StructuralTypeOptions{ApplyPresence: true, OptionalWhenMaybe: true}); ok && t != nil {
			return r.typeValues.FromTypeWithWitness(r.registry, t), true
		}
	}
	return product.Value{}, false
}

func closureCaptureNilability(value product.Value, t typ.Type, hasType bool) (bool, bool) {
	p := product.PresenceOf(value)
	switch {
	case presence.Equal(p, presence.Present()):
		return false, true
	case presence.Equal(p, presence.Absent()), presence.Equal(p, presence.Maybe()):
		return true, true
	case hasType:
		return typevalue.TypeIncludesNil(t), true
	default:
		return false, false
	}
}

func (r *Result) closureCaptureShape(point cfg.Point, value product.Value) (typ.Type, bool, bool, StableShapeTier, []StableShapeField) {
	if shape, ok := r.StableShapeForValueAtBoundary(point, value); ok {
		stable := shape.Tier != StableShapeTierPrefixStable
		return shape.Shape, true, stable, shape.Tier, append([]StableShapeField(nil), shape.Fields...)
	}
	t, ok := typevalue.StructuralTypeOf(r.registry, r.typeValues, value, typevalue.StructuralTypeOptions{ApplyPresence: true, OptionalWhenMaybe: true})
	if !ok || !closureCaptureShapeType(t) {
		return nil, false, false, StableShapeTierUnknown, nil
	}
	return t, true, false, StableShapeTierUnknown, nil
}

func closureCaptureShapeType(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record, *typ.Interface, *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Tuple:
		return true
	case *typ.Optional:
		return closureCaptureShapeType(v.Inner)
	case *typ.Union:
		for _, member := range v.Members {
			if closureCaptureShapeType(member) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
