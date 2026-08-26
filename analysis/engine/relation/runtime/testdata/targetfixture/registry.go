package targetfixture

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type factoryRegistry struct {
	factories []binding.Factory
}

func (value factoryRegistry) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() {
		return nil, false
	}
	var result binding.Binding
	for _, factory := range value.factories {
		if factory == nil {
			return nil, false
		}
		bound, ok := factory.Bind(operation)
		if !ok {
			continue
		}
		if bound == nil || result != nil {
			return nil, false
		}
		result = bound
	}
	return result, result != nil
}

type valueRegistry struct {
	algebras   map[model.TypeID]binding.ValueAlgebra
	equalities map[model.TypeID]binding.ValueEquality
}

func newValueRegistry(value Registry) (valueRegistry, bool) {
	result := valueRegistry{
		algebras:   make(map[model.TypeID]binding.ValueAlgebra, len(value.Algebras)),
		equalities: make(map[model.TypeID]binding.ValueEquality, len(value.Equalities)),
	}
	for _, algebra := range value.Algebras {
		if algebra == nil || !algebra.Type().Available() {
			return valueRegistry{}, false
		}
		if _, duplicate := result.algebras[algebra.Type()]; duplicate {
			return valueRegistry{}, false
		}
		result.algebras[algebra.Type()] = algebra
	}
	for _, equality := range value.Equalities {
		if equality == nil || !equality.Type().Available() {
			return valueRegistry{}, false
		}
		if _, duplicate := result.equalities[equality.Type()]; duplicate {
			return valueRegistry{}, false
		}
		result.equalities[equality.Type()] = equality
	}
	return result, true
}

func (value valueRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	algebra, ok := value.algebras[typeID]
	return algebra, ok && algebra != nil && algebra.Type() == typeID
}

func (value valueRegistry) ResolveEquality(typeID model.TypeID) (binding.ValueEquality, bool) {
	equality, ok := value.equalities[typeID]
	return equality, ok && equality != nil && equality.Type() == typeID
}
