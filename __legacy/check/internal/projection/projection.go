package projection

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/check/internal/staticmemberwitness"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/domain/type/access"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// StaticMemberValue is a proven rootless member value that should contribute to
// the structural witness for a root value.
type StaticMemberValue struct {
	Suffix []segment.Segment
	Value  product.Value
}

// ValueType resolves the projected type for a member value.
type ValueType func(product.Value) (typ.Type, bool)

// LiteralPathProof reports whether a literal type is proven at a path.
type LiteralPathProof func(pathdom.Path, typ.Type) bool

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
		if typ.SameNodeOrRecursiveIdentityEqual(existing, witness) {
			return value
		}
	}
	return typevalue.WithWitness(reg, value, witness)
}

// WithDeclaredContract projects a value through a declared contract and adopts
// the declared value's presence. Use this when the declared contract is the
// authoritative return slot.
func WithDeclaredContract(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	if refinement.DeclaredContractAlreadySatisfied(reg, value, declared) {
		return product.WithPresence(reg, value, product.PresenceOf(declared))
	}
	return product.WithPresence(reg, refinement.MergeDeclaredContract(reg, value, declared), product.PresenceOf(declared))
}

// WithDeclaredContractPreservingPresence projects a value through a declared
// contract while preserving the original value presence. Use this when a source
// expression supplied the slot and the declaration should refine only type facts.
func WithDeclaredContractPreservingPresence(reg *axis.Registry, value product.Value, declared product.Value) product.Value {
	if refinement.DeclaredContractAlreadySatisfiedPreservingPresence(reg, value, declared) {
		return value
	}
	return product.WithPresence(reg, WithDeclaredContract(reg, value, declared), product.PresenceOf(value))
}

// DeclaredPathType projects a declared root type through the path's segments
// using declaration semantics. Declaration projection is intentionally separate
// from runtime member projection: annotations follow the Lua type-projection
// package, not missing-field-as-nil runtime reads.
func DeclaredPathType(root typ.Type, p pathdom.Path) (typ.Type, bool) {
	if root == nil || p.Symbol == 0 {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return root, true
	}
	return luatypeprojection.ApplySegments(root, p.Segments)
}

// Field projects a field read from a type.
func Field(t typ.Type, name string) (typ.Type, bool) {
	return access.Field(t, name)
}

// RuntimeIndex projects a runtime-index read from a container/key type pair.
func RuntimeIndex(container typ.Type, key typ.Type) (typ.Type, bool) {
	return access.RuntimeIndex(container, key)
}

// MissingFieldReadsNil reports whether an absent field read projects to nil.
func MissingFieldReadsNil(t typ.Type) bool {
	return access.MissingFieldReadsNil(t)
}

// TypeAtSegment projects a runtime receiver type through one path segment.
func TypeAtSegment(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if field, ok := Field(t, seg.Name); ok {
			return field, true
		}
		if MissingFieldReadsNil(t) {
			return typ.Nil, true
		}
		return nil, false
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		key, ok := luatypeprojection.SegmentKeyType(seg)
		if !ok {
			return nil, false
		}
		return RuntimeIndex(t, key)
	default:
		return nil, false
	}
}

// TypeAtPath projects a runtime receiver type through a path's segments.
func TypeAtPath(root typ.Type, p pathdom.Path) (typ.Type, bool) {
	if root == nil || p.Symbol == 0 {
		return nil, false
	}
	current := root
	for _, seg := range p.Segments {
		next, ok := TypeAtSegment(current, seg)
		if !ok || next == nil {
			return nil, false
		}
		current = next
	}
	return current, true
}

// DiscriminantProvenMemberType returns the unique member type selected by a
// proven discriminant literal on receiver. Callers own solved-state proof
// queries; this package owns variant/type traversal.
func DiscriminantProvenMemberType(receiverType typ.Type, receiver pathdom.Path, member string, provesLiteral LiteralPathProof) (typ.Type, bool) {
	if receiverType == nil || receiver.IsEmpty() || receiver.Symbol == 0 || member == "" || provesLiteral == nil {
		return nil, false
	}
	_, cases, ok := variant.OriginCasesOfType(receiverType)
	if !ok || len(cases) < 2 {
		return nil, false
	}
	requiredIndex := -1
	var requiredType typ.Type
	for _, c := range cases {
		field, fieldOK := Field(c.Type, member)
		if !fieldOK {
			continue
		}
		if requiredIndex >= 0 {
			return nil, false
		}
		requiredIndex = c.Index
		requiredType = field
	}
	if requiredIndex < 0 || requiredType == nil {
		return nil, false
	}
	domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
	if !ok {
		return nil, false
	}
	for _, domain := range domains {
		discriminant := receiver.AppendSegments(domain.Suffix)
		for _, c := range cases {
			if c.Index != requiredIndex {
				continue
			}
			lit, litOK := variant.FieldAtPath(c.Type, domain.Suffix)
			if litOK && lit != nil && provesLiteral(discriminant, lit) {
				return requiredType, true
			}
		}
	}
	return nil, false
}
