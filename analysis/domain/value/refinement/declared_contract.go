package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
)

// MergeDeclaredContract enriches a computed value with a declared contract
// without erasing concrete value evidence. A declared return type is authoritative
// when the body produced no value evidence, but once the body proves a concrete
// variant origin that origin remains the value's precision lane. The declared
// type witness still records the API contract, and type reconstruction can then
// narrow that witness through the preserved origin.
func MergeDeclaredContract(reg *axis.Registry, value, declared product.Value) product.Value {
	if product.Equal(reg, value, product.Top()) || product.Equal(reg, value, product.Bottom(reg)) {
		return declared
	}
	value = typevalue.MergeDeclaredTypeFacts(reg, value, declared)
	value = typevalue.TrustScalarRuntimeProofForDeclaredContract(reg, value, declared)
	ed := product.Edit(reg, value)
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	if !declaredWitness.IsBottom() && !declaredWitness.IsTop() {
		product.EditSet(&ed, typewitness.Key, declaredWitness)
	}
	if origin, ok := declaredOriginForValue(reg, value, declared); ok {
		product.EditSet(&ed, variantorigin.Key, origin)
	}
	return ed.Done()
}

// DeclaredContractAlreadySatisfied reports whether MergeDeclaredContract(value,
// declared) would leave value unchanged.
func DeclaredContractAlreadySatisfied(reg *axis.Registry, value, declared product.Value) bool {
	if !typevalue.DeclaredTypeFactsAlreadySatisfied(reg, value, declared) {
		return false
	}
	if origin, ok := declaredOriginForValue(reg, value, declared); ok &&
		!variantorigin.Equal(product.Get(reg, value, variantorigin.Key), origin) {
		return false
	}
	return true
}

func declaredOriginForValue(reg *axis.Registry, value, declared product.Value) (variantorigin.Value, bool) {
	declaredOrigin := product.Get(reg, declared, variantorigin.Key)
	if declaredOrigin.IsBottom() || declaredOrigin.IsTop() {
		return variantorigin.Value{}, false
	}
	valueOrigin := product.Get(reg, value, variantorigin.Key)
	if valueType, ok := typevalue.TypeOf(reg, value); ok {
		if selected, ok := typevalue.OriginCasesForType(declaredOrigin.Family(), declaredOrigin.CasesRef(), valueType); ok {
			return variantorigin.Of(declaredOrigin.Family(), selected), true
		}
	}
	if valueOrigin.IsBottom() || valueOrigin.IsTop() {
		return declaredOrigin, true
	}
	return variantorigin.Value{}, false
}
