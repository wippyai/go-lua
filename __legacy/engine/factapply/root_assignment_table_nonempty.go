package factapply

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// RootAssignmentTableNonEmptyQuery is the frozen factor-native proof demand
// for the table operand of a modulo-length dynamic read.
type RootAssignmentTableNonEmptyQuery struct {
	domain       state.ProductDomain
	lenFloor     state.CoordinateSlot
	refinement   state.CoordinateSlot
	staticMember state.CoordinateSlot
	rootValue    statekey.Value
	hasRootValue bool
	sealed       bool
}

type RootAssignmentTableNonEmptyInputs struct {
	LenFloor     state.CoordinateScalarFactor
	Refinement   state.CoordinateScalarFactor
	StaticMember state.CoordinateScalarFactor
	RootValue    product.Value
	HasRootValue bool
}

// TableNonEmptyQuery returns the exact coordinate and Values-slot inventory
// needed to prove the modulo table non-empty. It is absent for non-modulo
// dynamic reads.
func (p RootAssignmentDynamicSourcePlan) TableNonEmptyQuery() (RootAssignmentTableNonEmptyQuery, bool, error) {
	if !p.sealed || !p.domain.Valid() || p.resolver == nil || !p.hasModulo {
		return RootAssignmentTableNonEmptyQuery{}, false, nil
	}
	address := visibility.AddressAt(p.resolver, p.point, p.dynamic.TablePathRef())
	path, ok := address.VisibleKeyspaceKey()
	if !ok || path.Kind == keyspace.KindInvalid {
		return RootAssignmentTableNonEmptyQuery{}, false, fmt.Errorf("factapply: modulo table has no exact key")
	}
	keys := p.resolver.KeySpace()
	length, err := p.domain.LenFloorCoordinateSlot(keys, path)
	if err != nil {
		return RootAssignmentTableNonEmptyQuery{}, false, err
	}
	refinement, err := p.domain.PathRefinementCoordinateSlot(keys, path)
	if err != nil {
		return RootAssignmentTableNonEmptyQuery{}, false, err
	}
	staticMember, err := p.domain.PathStaticMemberCoordinateSlot(keys, path)
	if err != nil {
		return RootAssignmentTableNonEmptyQuery{}, false, err
	}
	if p.isFormal {
		length, err = p.domain.RekeyCoordinateSlotFormal(p.formalRekey, length)
		if err != nil {
			return RootAssignmentTableNonEmptyQuery{}, false, err
		}
		refinement, err = p.domain.RekeyCoordinateSlotFormal(p.formalRekey, refinement)
		if err != nil {
			return RootAssignmentTableNonEmptyQuery{}, false, err
		}
		staticMember, err = p.domain.RekeyCoordinateSlotFormal(p.formalRekey, staticMember)
		if err != nil {
			return RootAssignmentTableNonEmptyQuery{}, false, err
		}
	}
	query := RootAssignmentTableNonEmptyQuery{domain: p.domain, lenFloor: length, refinement: refinement, staticMember: staticMember, sealed: true}
	table := p.dynamic.TablePathRef()
	if len(table.Segments) == 0 && table.Symbol != 0 {
		query.rootValue, query.hasRootValue = statekey.SymbolValue(table.Symbol), true
	}
	return query, true, nil
}

func (q RootAssignmentTableNonEmptyQuery) LenFloorSlot() (state.CoordinateSlot, bool) {
	return q.lenFloor, q.sealed
}
func (q RootAssignmentTableNonEmptyQuery) RefinementSlot() (state.CoordinateSlot, bool) {
	return q.refinement, q.sealed
}
func (q RootAssignmentTableNonEmptyQuery) StaticMemberSlot() (state.CoordinateSlot, bool) {
	return q.staticMember, q.sealed
}
func (q RootAssignmentTableNonEmptyQuery) RootValueSlot() (statekey.Value, bool) {
	return q.rootValue, q.sealed && q.hasRootValue
}

// DefinitelyNonEmpty evaluates only the sealed exact factors. It never
// materializes State or falls back to a lane snapshot.
func (q RootAssignmentTableNonEmptyQuery) DefinitelyNonEmpty(typeValues *typevalue.Cache, inputs RootAssignmentTableNonEmptyInputs) (bool, error) {
	if !q.sealed || !q.domain.Valid() || typeValues == nil {
		return false, fmt.Errorf("factapply: invalid table-nonempty query")
	}
	for _, pair := range [][2]state.CoordinateSlot{{q.lenFloor, inputs.LenFloor.Slot()}, {q.refinement, inputs.Refinement.Slot()}, {q.staticMember, inputs.StaticMember.Slot()}} {
		equal, err := q.domain.CoordinateSlotEqual(pair[0], pair[1])
		if err != nil || !equal {
			return false, fmt.Errorf("factapply: table-nonempty factor mismatch")
		}
	}
	if inputs.HasRootValue != q.hasRootValue {
		return false, fmt.Errorf("factapply: table-nonempty root Values operand mismatch")
	}
	if q.hasRootValue && !product.BelongsToRegistry(q.domain.Registry(), inputs.RootValue) {
		return false, fmt.Errorf("factapply: foreign table root value")
	}
	if floor, present, err := q.domain.LenFloorCoordinateValue(inputs.LenFloor); err != nil {
		return false, err
	} else if present && floor >= 1 {
		return true, nil
	}
	for _, scalar := range []state.CoordinateScalarFactor{inputs.StaticMember, inputs.Refinement} {
		value, present, err := q.domain.PathEvidenceCoordinateValue(scalar)
		if err != nil {
			return false, err
		}
		if present && definitelyNonEmptyValue(q.domain, typeValues, value) {
			return true, nil
		}
	}
	if q.hasRootValue {
		if definitelyNonEmptyValue(q.domain, typeValues, inputs.RootValue) {
			return true, nil
		}
	}
	return false, nil
}

func definitelyNonEmptyValue(domain state.ProductDomain, typeValues *typevalue.Cache, value product.Value) bool {
	t, ok := typeValues.TypeOf(domain.Registry(), value)
	return ok && typevalue.DefinitelyNonEmptyIndexContainer(t)
}
