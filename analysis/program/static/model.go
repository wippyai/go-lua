// Package static owns authored static syntax and sidecars attached to
// canonical Program Terms, including authored type-reference resolution. It
// owns no inferred, domain, Link, or Target facts. The Types vertical itself
// deliberately owns no TypeRef rows; References owns that exact relation.
package static

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// poolRange selects an owner-local interval in an immutable pool.
type poolRange struct{ Start, End uint32 }

// PrimitiveKind is the complete parser-authored primitive type vocabulary.
// User-defined spellings are owned by the References vertical, not open
// primitive names.
type PrimitiveKind uint8

const (
	PrimitiveNil PrimitiveKind = iota + 1
	PrimitiveBoolean
	PrimitiveNumber
	PrimitiveInteger
	PrimitiveString
	PrimitiveFunction
	PrimitiveAny
	PrimitiveUnknown
	PrimitiveNever
	PrimitiveSelf
)

func (kind PrimitiveKind) valid() bool { return kind >= PrimitiveNil && kind <= PrimitiveSelf }

// RuntimeLoadable is the exact primitive subset represented by a runtime
// type singleton. Function and Self are static-only forms.
func (kind PrimitiveKind) RuntimeLoadable() bool {
	switch kind {
	case PrimitiveNil, PrimitiveBoolean, PrimitiveNumber, PrimitiveInteger,
		PrimitiveString, PrimitiveAny, PrimitiveUnknown, PrimitiveNever:
		return true
	default:
		return false
	}
}

// PrimitiveKindForName is the only spelling-to-primitive conversion. It is
// intentionally closed so parser changes make this boundary fail closed.
func PrimitiveKindForName(name string) (PrimitiveKind, bool) {
	switch name {
	case "nil":
		return PrimitiveNil, true
	case "boolean":
		return PrimitiveBoolean, true
	case "number":
		return PrimitiveNumber, true
	case "integer":
		return PrimitiveInteger, true
	case "string":
		return PrimitiveString, true
	case "function":
		return PrimitiveFunction, true
	case "any":
		return PrimitiveAny, true
	case "unknown":
		return PrimitiveUnknown, true
	case "never":
		return PrimitiveNever, true
	case "self":
		return PrimitiveSelf, true
	default:
		return 0, false
	}
}

// Primitive, Literal, Optional, Union, Intersection, Generic, Array, Map,
// Record, and Field are separate typed relations. There is deliberately
// no universal node kind or generic child edge.
type Primitive struct{ Kind PrimitiveKind }

// Literal carries no duplicate atom payload. Bool, Integer, and String use a
// Source/keyspace-owned Exact handle; Float retains its authored IEEE payload.
type Literal struct {
	Kind      keyspace.LiteralKind
	Exact     keyspace.Key
	FloatBits uint64
}
type Optional struct{ Inner keyspace.Term }
type Union struct{ Members []keyspace.Term }
type Intersection struct{ Members []keyspace.Term }

// Generic has a TypeRef base owned by the References vertical; its
// arguments are source-ordered authored type occurrences.
type Generic struct {
	Base keyspace.Term
	Args []keyspace.Term
}

// TypeRefResolution preserves the authored binder result independently from
// its source spelling. It is not an inferred resolution result.
type TypeRefResolution uint8

const (
	TypeRefUnresolved TypeRefResolution = iota + 1
	TypeRefDeclaration
	TypeRefCanonicalPath
)

// TypeRef retains the complete authored spelling and its binder disposition.
// A declaration target and a canonical path are mutually exclusive.
type TypeRef struct {
	Resolution TypeRefResolution
	Target     keyspace.Term
	Root       keyspace.Term
	Source     []keyspace.Key
	Canonical  []keyspace.Key
}

type Array struct {
	Element  keyspace.Term
	ReadOnly bool
}

type Map struct {
	Key, Value keyspace.Term
	ReadOnly   bool
}

// Field is later claimed exactly once by a Record or an Interface member.
// This local Types vertical records neither cross-vertical ownership choice.
type Field struct {
	Key      keyspace.Key
	Type     keyspace.Term
	Optional bool
}

type Record struct {
	Fields   []keyspace.Term
	ReadOnly bool
}

// TypesInput is the full authored Types denominator. Counts allocates and
// validates canonical Term identities but is never retained after Build.
type TypesInput struct {
	Primitive    []Primitive
	Literal      []Literal
	Optional     []Optional
	Union        []Union
	Intersection []Intersection
	Generic      []Generic
	Array        []Array
	Map          []Map
	Record       []Record
	Field        []Field
}

// ReferencesInput is the complete authored TypeRef denominator. Source and
// canonical paths retain key handles only; Source/keyspace membership is a
// later joint-seal obligation.
type ReferencesInput struct{ TypeRef []TypeRef }

// TypeAlias, TypeParam, Interface, and InterfaceMember are distinct authored
// declaration relations. They deliberately do not form a generic declaration
// node/edge vocabulary: every ownership and ordering law remains visible here.
type TypeAlias struct {
	Owner          keyspace.Term
	Target         keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Params         []keyspace.Term
}

// TypeParam has no secondary coordinate: the owning Term span is its name.
// Its owner may be a TypeAlias, a later Signature TypeFunction, or Flow's
// authored Function family.
type TypeParam struct {
	Owner      keyspace.Term
	Name       keyspace.Key
	Constraint keyspace.Term
}

type Interface struct {
	Owner          keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Extends        []keyspace.Term
	Members        []InterfaceMember
}

type InterfaceMemberKind uint8

const (
	InterfaceField InterfaceMemberKind = iota + 1
	InterfaceMethod
)

// InterfaceMember is exact-xor: Field members populate Field only; Method
// members populate Name, NameCoordinate, and Signature only.
type InterfaceMember struct {
	Kind           InterfaceMemberKind
	Field          keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Signature      keyspace.Term
}

type DeclarationsInput struct {
	Alias        []TypeAlias
	TypeParam    []TypeParam
	Interface    []Interface
	DeclaredType []DeclaredType
}

// DeclaredType is one authored binding from a canonical lexical Cell to its
// declared static type. Cell identity remains owned by Flow/Source; this
// relation owns neither a name nor a coordinate and must not reconstruct one.
type DeclaredType struct {
	Cell   keyspace.Term
	Target keyspace.Term
}

// Parameter is one authored fixed parameter of a TypeFunction. An absent
// source name has both Name and NameCoordinate zero; a named parameter has
// both present. The parameter's Type is a concrete static type child.
type Parameter struct {
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Type           keyspace.Term
}

// TypeFunction is one source-only static callable. Scope is the existing
// static-scope handle; its eventual lexical/body containment is sealed jointly
// with the owner that owns that geometry. Params, Parameters, and Returns are
// source ordered. ReturnsKnown distinguishes an omitted clause from `-> ()`.
type TypeFunction struct {
	Scope              keyspace.Term
	TypeParams         []keyspace.Term
	Parameters         []Parameter
	Variadic           keyspace.Term
	VariadicCoordinate source.Coordinate
	ReturnsKnown       bool
	Returns            []keyspace.Term
}

// TypeAsserts retains the authored asserted parameter and its immediate
// binder disposition without an overloaded negative sentinel. Narrow zero is
// the authored truthy/non-nil form.
type TypeAsserts struct {
	Name            keyspace.Key
	ParamCoordinate source.Coordinate
	Bound           bool
	Param           uint32
	Narrow          keyspace.Term
}

type SignaturesInput struct {
	TypeFunction []TypeFunction
	TypeAsserts  []TypeAsserts
}

// Input is the standalone authored Static boundary. Later verticals extend
// this package's private component rather than create a parallel static IR.
type Input struct {
	Counts       [keyspace.FamilyCount]uint32
	Types        TypesInput
	References   ReferencesInput
	Declarations DeclarationsInput
	Signatures   SignaturesInput
	Contracts    ContractsInput
	Operators    OperatorsInput
	Operands     OperandsInput
	Publications PublicationsInput
}

type typeStore struct {
	primitive    []Primitive
	literal      []Literal
	optional     []Optional
	union        []poolRange
	intersection []poolRange
	generic      []genericRow
	array        []Array
	mapType      []Map
	record       []recordRow
	field        []Field
	terms        []keyspace.Term
	fields       []keyspace.Term
}

type referenceStore struct {
	rows      []typeRefRow
	source    []keyspace.Key
	canonical []keyspace.Key
}

type declarationStore struct {
	aliases        []typeAliasRow
	params         []TypeParam
	interfaces     []interfaceRow
	declaredTypes  []declaredTypeRow
	declaredByCell []keyspace.Term
	aliasParams    []keyspace.Term
	interfaceRefs  []keyspace.Term
	members        []interfaceMemberRow
}

type declaredTypeRow struct {
	cell   keyspace.Term
	target keyspace.Term
}

type typeAliasRow struct {
	owner      keyspace.Term
	target     keyspace.Term
	name       keyspace.Key
	coordinate source.Coordinate
	params     poolRange
}

type interfaceRow struct {
	owner      keyspace.Term
	name       keyspace.Key
	coordinate source.Coordinate
	extends    poolRange
	members    poolRange
}

type interfaceMemberRow struct {
	kind       InterfaceMemberKind
	field      keyspace.Term
	name       keyspace.Key
	coordinate source.Coordinate
	signature  keyspace.Term
}

type typeRefRow struct {
	resolution TypeRefResolution
	target     keyspace.Term
	root       keyspace.Term
	source     poolRange
	canonical  poolRange
}

type genericRow struct {
	base keyspace.Term
	args poolRange
}

type recordRow struct {
	fields   poolRange
	readOnly bool
}

// Component is immutable authored Static syntax with no inferred/domain resolution, query index, or receipt.
// staticTypes is fixed-size prefix metadata over the authored typed families;
// it retains no duplicate Term stream or second graph.
type Component struct {
	contentID    identity.ContentID
	staticTypes  staticTypeIndex
	types        typeStore
	references   referenceStore
	declarations declarationStore
	signatures   signatureStore
	contracts    contractsStore
	operators    operatorsStore
	operands     operandsStore
	publications []publicationRow
}

type draftState struct {
	component        *Component
	localContainment *localContainmentProof
	phase            draftPhase
	mu               sync.Mutex
}

type draftPhase uint8

const (
	draftOpen draftPhase = iota + 1
	draftClaimed
	draftCommitted
	draftAborted
)

// Draft is a shared construction capability. Copies share state; publication
// is possible only by first claiming the owner-defined Finalizer.
type Draft struct{ state *draftState }

// Finalizer is an owner-defined one-shot publication capability. Copies share
// the Draft state, so exactly one terminal action (Commit or Abort) can win.
// The View is a read-only validation surface for the owner that coordinates
// finalization; it carries no construction state or sibling projection.
type Finalizer struct {
	state *draftState
}

// View partitions this vertical by exact typed relation.
type View struct {
	component *Component
	state     *draftState
}
type Types struct {
	component *Component
	state     *draftState
}
type Primitives struct {
	component *Component
	state     *draftState
}
type Literals struct {
	component *Component
	state     *draftState
}
type Optionals struct {
	component *Component
	state     *draftState
}
type Unions struct {
	component *Component
	state     *draftState
}
type Intersections struct {
	component *Component
	state     *draftState
}
type Generics struct {
	component *Component
	state     *draftState
}
type Arrays struct {
	component *Component
	state     *draftState
}
type Maps struct {
	component *Component
	state     *draftState
}
type Records struct {
	component *Component
	state     *draftState
}
type Fields struct {
	component *Component
	state     *draftState
}
type References struct {
	component *Component
	state     *draftState
}
type Declarations struct {
	component *Component
	state     *draftState
}
type Aliases struct {
	component *Component
	state     *draftState
}
type TypeParams struct {
	component *Component
	state     *draftState
}
type Interfaces struct {
	component *Component
	state     *draftState
}
type DeclaredTypes struct {
	component *Component
	state     *draftState
}
type Signatures struct {
	component *Component
	state     *draftState
}
type TypeFunctions struct {
	component *Component
	state     *draftState
}
type Assertions struct {
	component *Component
	state     *draftState
}
