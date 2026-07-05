package projection

import (
	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// StaticMemberValue is a proven rootless member value that should contribute to
// the structural witness for a root value.
type StaticMemberValue struct {
	Suffix []segment.Segment
	Value  product.Value
}

// ValueType resolves the projected type for a member value.
type ValueType func(product.Value) (typ.Type, bool)

// WithStaticMemberWitness overlays a root value with a structural witness built
// from proven rootless static members. This package owns the value-to-type
// projection boundary so summary projection and expression reading do not grow
// independent witness-construction paths.
func WithStaticMemberWitness(reg *axis.Registry, value product.Value, members []StaticMemberValue, typeOf ValueType) product.Value {
	if reg == nil || typeOf == nil || len(members) == 0 {
		return value
	}
	builder := staticmemberwitness.NewBuilder()
	for _, member := range members {
		if len(member.Suffix) == 0 || product.Equal(reg, member.Value, product.Bottom(reg)) {
			continue
		}
		memberType, ok := typeOf(member.Value)
		if !ok || memberType == nil {
			continue
		}
		builder.Add(member.Suffix, memberType)
	}
	witness, ok := builder.Build()
	if !ok {
		return value
	}
	if existing, ok := typevalue.TypeOf(reg, value); ok && existing != nil {
		merged, mergedOK := typetable.OverlayRecordMembers(existing, witness)
		if !mergedOK {
			return value
		}
		witness = merged
	}
	return typevalue.WithWitness(reg, value, witness)
}

// WithDeclaredContract projects a value through a declared contract and adopts
// the declared value's presence. Use this when the declared contract is the
// authoritative return slot.
func WithDeclaredContract(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	return product.WithPresence(reg, refinement.MergeDeclaredContract(reg, value, declared), product.PresenceOf(declared))
}

// WithDeclaredContractPreservingPresence projects a value through a declared
// contract while preserving the original value presence. Use this when a source
// expression supplied the slot and the declaration should refine only type facts.
func WithDeclaredContractPreservingPresence(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	return product.WithPresence(reg, WithDeclaredContract(reg, value, declared), product.PresenceOf(value))
}
