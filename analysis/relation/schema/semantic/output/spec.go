package output

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// ReducerKind is the only reducer admitted by this schema boundary.
//
// A Unique proof is not inferred from a dependency or expression identity:
// until the checker issues a real single-writer/injectivity witness, Unique
// is intentionally unavailable here.
type ReducerKind uint8

const (
	ReducerInvalid ReducerKind = iota
	Contributions
)

// Available reports whether kind is the currently admitted reducer.
func (kind ReducerKind) Available() bool {
	return kind == Contributions
}

// String returns the stable reducer vocabulary.
func (kind ReducerKind) String() string {
	if kind == Contributions {
		return "Contributions"
	}
	return "Invalid"
}

// OutputPort is the exact structural identity of one declared output:
// operation identity plus the owner-issued output ColumnID. It deliberately
// has no output ordinal; Signature.OutputFor is the declaration authority.
type OutputPort struct {
	Operation signature.Identity
	Column    model.ColumnID
}

// Available reports whether both halves of the structural port are present.
func (port OutputPort) Available() bool {
	return port.Operation.Available() && port.Column.Available()
}

// Spec is an unchecked contribution declaration. Signature is admission
// context and is not retained after Seal; the usable value carries only the
// structural port, signature-derived presence contract, value capability,
// reducer, and digest.
type Spec struct {
	Signature signature.Signature
	Port      OutputPort
	ValueType model.TypeID
	Algebra   model.TypeCapability
	Reducer   ReducerKind
}

// ContributionSpec is the minimal immutable, digest-covered semantic output
// declaration. InvocationAddress remains the runtime structural owner of
// concrete invocations; no schema projection, destination map, ordinal, or
// runtime receipt is restated here.
type ContributionSpec struct {
	port      OutputPort
	presence  signature.PresenceContract
	valueType model.TypeID
	algebra   model.TypeCapability
	reducer   ReducerKind
	digest    identity.ContentID
	sealed    bool
}

// Seal validates and freezes one contribution declaration. The supplied
// Signature proves that the exact structural port is a declared output and
// that ValueType is its declared type. Its Presence contract is copied from
// that exact output; callers cannot author a second presence field. Foreign
// identities and every reducer other than Contributions are rejected.
func Seal(spec Spec) (ContributionSpec, bool) {
	if !spec.Signature.Available() ||
		!spec.Port.Available() ||
		!spec.ValueType.Available() ||
		!spec.Algebra.Available() ||
		spec.Algebra.Type() != spec.ValueType ||
		spec.Reducer != Contributions ||
		spec.Signature.Identity() != spec.Port.Operation {
		return ContributionSpec{}, false
	}

	declared, declaredOK := spec.Signature.OutputFor(spec.Port.Column.Relation(), spec.Port.Column)
	if !declaredOK || !declared.Available() || declared.Type != spec.ValueType {
		return ContributionSpec{}, false
	}

	owner := spec.Port.Operation.Operation.Owner()
	fence := spec.Signature.Fence()
	if !fence.Available() || fence.Owner != owner || fence.Schema.Owner() != owner {
		return ContributionSpec{}, false
	}
	if spec.Port.Column.Owner() != owner ||
		spec.ValueType.Owner() != owner ||
		spec.Algebra.Type().Owner() != owner {
		return ContributionSpec{}, false
	}

	value := ContributionSpec{
		port:      spec.Port,
		presence:  declared.Presence,
		valueType: spec.ValueType,
		algebra:   spec.Algebra,
		reducer:   spec.Reducer,
		sealed:    true,
	}
	value.digest = digestContribution(value)
	if !value.digest.Available() {
		return ContributionSpec{}, false
	}
	return value, true
}

// Available reports whether the complete immutable declaration is present.
func (spec ContributionSpec) Available() bool {
	return spec.sealed && spec.digest.Available()
}

// Port returns the exact structural output port.
func (spec ContributionSpec) Port() OutputPort {
	if !spec.Available() {
		return OutputPort{}
	}
	return spec.port
}

// Column returns the output column carried by Port.
func (spec ContributionSpec) Column() model.ColumnID {
	if !spec.Available() {
		return model.ColumnID{}
	}
	return spec.port.Column
}

// Presence returns the exact output presence contract declared by the sealed
// Signature. An unavailable spec returns the invalid zero enum; no optional
// or absent interpretation is inferred here.
func (spec ContributionSpec) Presence() signature.PresenceContract {
	if !spec.Available() {
		return signature.PresenceContract(0)
	}
	return spec.presence
}

// ValueType returns the exact owner-issued output value type.
func (spec ContributionSpec) ValueType() model.TypeID {
	if !spec.Available() {
		return model.TypeID{}
	}
	return spec.valueType
}

// Algebra returns the exact schema-owned value capability.
func (spec ContributionSpec) Algebra() model.TypeCapability {
	if !spec.Available() {
		return model.TypeCapability{}
	}
	return spec.algebra
}

// Reducer returns the admitted reducer case.
func (spec ContributionSpec) Reducer() ReducerKind {
	if !spec.Available() {
		return ReducerInvalid
	}
	return spec.reducer
}

// Digest returns the deterministic identity of the complete declaration.
func (spec ContributionSpec) Digest() identity.ContentID {
	if !spec.Available() {
		return identity.ContentID{}
	}
	return spec.digest
}

// Equal compares complete sealed content.
func (spec ContributionSpec) Equal(other ContributionSpec) bool {
	return spec.Available() && other.Available() &&
		spec.digest == other.digest &&
		spec.port == other.port &&
		spec.presence == other.presence &&
		spec.valueType == other.valueType &&
		spec.algebra.Equal(other.algebra) &&
		spec.reducer == other.reducer
}

func digestContribution(spec ContributionSpec) identity.ContentID {
	operation := spec.port.Operation.Operation
	columnRelation := spec.port.Column.Relation()
	columnContent := spec.port.Column.Content()
	parts := [][]byte{
		nominalBytes(operation.Owner().Content(), operation.Content()),
		uint64Bytes(spec.port.Operation.Version),
		nominalBytes(columnRelation.Owner().Content(), columnRelation.Content()),
		contentBytes(columnContent),
		{byte(spec.presence)},
		typeBytes(spec.valueType),
		contentBytes(spec.algebra.Digest()),
		{byte(spec.reducer)},
	}
	value, ok := identity.DeriveContentID(
		"analysis/relation/schema/semantic/output/contribution/v3",
		parts...,
	)
	if !ok {
		return identity.ContentID{}
	}
	return value
}

func nominalBytes(owner, content identity.ContentID) []byte {
	value := make([]byte, 0, len(owner)+len(content))
	value = append(value, owner[:]...)
	return append(value, content[:]...)
}

func contentBytes(value identity.ContentID) []byte {
	return append([]byte(nil), value[:]...)
}

func typeBytes(value model.TypeID) []byte {
	return nominalBytes(value.Owner().Content(), value.Content())
}

func uint64Bytes(value uint64) []byte {
	result := make([]byte, 8)
	result[0] = byte(value >> 56)
	result[1] = byte(value >> 48)
	result[2] = byte(value >> 40)
	result[3] = byte(value >> 32)
	result[4] = byte(value >> 24)
	result[5] = byte(value >> 16)
	result[6] = byte(value >> 8)
	result[7] = byte(value)
	return result
}
