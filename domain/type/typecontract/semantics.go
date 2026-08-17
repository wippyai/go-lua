package typecontract

import (
	"context"
	"errors"
	"fmt"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// Semantics is the Lua implementation of the neutral schema type-contract
// algebra. It is stateless: every operation reconstructs only the cold formal
// authority needed for that operation and delegates type judgments to the
// canonical domain packages.
type Semantics struct{}

var _ schematype.Semantics = Semantics{}

// NewSemantics returns the explicit Lua adapter required by schema/target
// composition. There is intentionally no package-global default.
func NewSemantics() Semantics { return Semantics{} }

// Validate performs neutral-envelope validation and domain decode admission.
// A valid closed declaration may be checked under a nonempty outer scope; an
// open declaration must carry the exact external formal count.
func (Semantics) Validate(value schematype.Type, formals []schematype.Type) error {
	scope, err := newFormalScope(formals)
	if err != nil {
		return err
	}
	_, err = decodeScoped(value, scope)
	return err
}

// Assignable decodes both neutral declarations under one formal authority and
// applies the Lua gradual assignment law. Invalid operands return an error;
// false with nil is a valid but rejected relation.
func (Semantics) Assignable(source, destination schematype.Type, formals []schematype.Type) (bool, error) {
	scope, err := newFormalScope(formals)
	if err != nil {
		return false, err
	}
	left, err := decodeScoped(source, scope)
	if err != nil {
		return false, err
	}
	right, err := decodeScoped(destination, scope)
	if err != nil {
		return false, err
	}
	return Assignable(left, right), nil
}

// Callable applies one neutral callable-admission request.
func (Semantics) Callable(value schematype.Type, admission schematype.CallableAdmission, formals []schematype.Type) (bool, error) {
	var requested Admission
	switch admission {
	case schematype.CallableAdmissionDirectFunction:
		requested = DirectFunction
	case schematype.CallableAdmissionOrdinary:
		requested = OrdinaryCallable
	default:
		return false, fmt.Errorf("typecontract: invalid callable admission %d", admission)
	}
	scope, err := newFormalScope(formals)
	if err != nil {
		return false, err
	}
	decoded, err := decodeScoped(value, scope)
	if err != nil {
		return false, err
	}
	return Admits(decoded, requested), nil
}

// Fresh applies one neutral runtime/fresh-class request.
func (Semantics) Fresh(value schematype.Type, class schematype.FreshClass, formals []schematype.Type) (bool, error) {
	requested, ok := freshKind(class)
	if !ok {
		return false, fmt.Errorf("typecontract: invalid fresh class %d", class)
	}
	scope, err := newFormalScope(formals)
	if err != nil {
		return false, err
	}
	decoded, err := decodeScoped(value, scope)
	if err != nil {
		return false, err
	}
	return FreshCompatible(decoded, requested), nil
}

func freshKind(class schematype.FreshClass) (FreshKind, bool) {
	switch class {
	case schematype.FreshClassTable:
		return FreshTable, true
	case schematype.FreshClassFunction:
		return FreshFunction, true
	case schematype.FreshClassThread:
		return FreshThread, true
	case schematype.FreshClassUserdata:
		return FreshUserdata, true
	case schematype.FreshClassError:
		return FreshError, true
	case schematype.FreshClassReflection:
		return FreshReflection, true
	default:
		return FreshInvalid, false
	}
}

type formalScope struct {
	formals []*typ.TypeParam
}

// newFormalScope reconstructs the operation-local formal authority from
// neutral constraint declarations. A constraint with zero external formals
// is closed and decodes independently; an open constraint must refer to the
// complete operation-local ordinal scope. The placeholders are installed
// before decoding so references retain ordinal identity.
func newFormalScope(declarations []schematype.Type) (formalScope, error) {
	formals := make([]*typ.TypeParam, len(declarations))
	for index := range formals {
		formals[index] = typ.NewTypeParam(fmt.Sprintf("$formal%d", index), nil)
	}
	for index, declaration := range declarations {
		if !declaration.Available() {
			continue
		}
		if declaration.ExternalFormals() != 0 && declaration.ExternalFormals() != uint32(len(formals)) {
			return formalScope{}, fmt.Errorf("typecontract: formal %d constraint scope %d, want %d", index, declaration.ExternalFormals(), len(formals))
		}
		constraint, err := decodeScoped(declaration, formalScope{formals: formals})
		if err != nil {
			return formalScope{}, fmt.Errorf("typecontract: formal %d constraint: %w", index, err)
		}
		formals[index].Constraint = constraint
	}
	return formalScope{formals: formals}, nil
}

func decodeScoped(value schematype.Type, scope formalScope) (typ.Type, error) {
	if !value.Available() {
		return nil, errors.New("typecontract: unavailable declaration")
	}
	return Decode(context.Background(), value, scope.formals)
}
