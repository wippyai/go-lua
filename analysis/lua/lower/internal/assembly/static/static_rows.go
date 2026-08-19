// Package static owns the mutable Static construction rows used by the Lua
// lowerer. It deliberately has no
// dependency on the old core Builder: rows are authored in the public
// program/static vocabulary and exact-key handles are resolved only after the
// Source preimage has been built.
package static

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// staticRawKey is a payload reference, not a provisional keyspace.Key.  The
// bool is necessary because false and the integer zero are valid exact atoms.
// A missing payload is never silently converted to Key(0) during freeze.
type staticRawKey struct {
	value   keyspace.LiteralValue
	present bool
}

type staticRawLiteral struct {
	kind      keyspace.LiteralKind
	exact     staticRawKey
	floatBits uint64
}

type staticRawAlias struct {
	owner      keyspace.Term
	target     keyspace.Term
	targetSet  bool
	name       staticRawKey
	coordinate source.Coordinate
	params     []keyspace.Term
	paramsSet  bool
}

type staticRawParam struct {
	owner      keyspace.Term
	name       staticRawKey
	constraint keyspace.Term
	filled     bool
}

type staticRawInterface struct {
	owner      keyspace.Term
	name       staticRawKey
	coordinate source.Coordinate
	extends    []keyspace.Term
	extendsSet bool
	membersRaw []staticRawInterfaceMember
	membersSet bool
}

type staticRawInterfaceMember struct {
	kind       staticdecl.InterfaceMemberKind
	field      keyspace.Term
	name       staticRawKey
	coordinate source.Coordinate
	signature  keyspace.Term
}

type staticRawTypeRef struct {
	resolution staticrefs.Resolution
	target     keyspace.Term
	root       keyspace.Term
	source     []staticRawKey
	canonical  []staticRawKey
}

type staticRawGeneric struct {
	base keyspace.Term
	args []keyspace.Term
}

type staticRawTypeFunction struct {
	scope              keyspace.Term
	typeParams         []keyspace.Term
	typeParamsSet      bool
	parameters         []staticRawParameter
	parametersSet      bool
	variadic           keyspace.Term
	variadicCoordinate source.Coordinate
	variadicSet        bool
	returnsKnown       bool
	returns            []keyspace.Term
	returnsSet         bool
}

type staticRawParameter struct {
	name       staticRawKey
	coordinate source.Coordinate
	typ        keyspace.Term
}

type staticRawAssertion struct {
	name       staticRawKey
	coordinate source.Coordinate
	bound      bool
	param      uint32
	narrow     keyspace.Term
	narrowSet  bool
}

type staticRawAnnotation struct {
	scope  keyspace.Term
	target keyspace.Term
	name   staticRawKey
	values keyspace.Term
	filled bool
}

type staticRawFunctionContract struct {
	typeParams    []keyspace.Term
	typeParamsSet bool
	returnsKnown  bool
	returns       []keyspace.Term
	returnsSet    bool
}

type staticRawCallContract struct {
	arguments []keyspace.Term
	filled    bool
}

// staticTypeRows owns the concrete authored type-expression relations.
type staticTypeRows struct {
	primitive    []statictypes.Primitive
	literal      []staticRawLiteral
	optional     []statictypes.Optional
	union        []statictypes.Union
	intersection []statictypes.Intersection
	generic      []staticRawGeneric
	array        []statictypes.Array
	mapType      []statictypes.Map
	record       []statictypes.Record
	field        []staticRawField
}

// staticReferenceRows owns authored TypeRef spelling and binder disposition.
type staticReferenceRows struct {
	references []staticRawTypeRef
}

// staticDeclarationRows owns aliases, interfaces, parameters, and declared
// Cell-type rows.  It never owns a generic declaration node.
type staticDeclarationRows struct {
	aliases    []staticRawAlias
	params     []staticRawParam
	interfaces []staticRawInterface
	declared   []staticdecl.DeclaredType
}

// staticSignatureRows owns source-only TypeFunction and assertion relations.
type staticSignatureRows struct {
	typeFunctions []staticRawTypeFunction
	assertions    []staticRawAssertion
}

// staticContractRows owns dense Flow Function/Call static sidecars.
type staticContractRows struct {
	functionContracts []staticRawFunctionContract
	callContracts     []staticRawCallContract
}

// staticOperatorRows owns the four concrete authored static operators.
type staticOperatorRows struct {
	typeOf      []staticoperators.TypeOf
	keyOf       []staticoperators.KeyOf
	indexAccess []staticoperators.IndexAccess
	conditional []staticoperators.Conditional
}

// staticOperandRows owns the sparse/dense authored operand sidecars.
type staticOperandRows struct {
	claims      []staticoperands.ClaimTarget
	typeValues  []staticoperands.TypeValueTarget
	annotations []staticRawAnnotation
}

// staticPublicationRows owns the dense Assign-pair publication relation.
type staticPublicationRows struct {
	publications []staticpubs.Publication
}

// staticRows is the complete Static lowering denominator. It is a pure row
// store: terms and raw payloads are supplied by the typed construction API,
// while Collector/Source admission is coordinated at that API boundary.
// Static never mints identities or creates an exact-key interner.
type staticRows struct {
	staticTypeRows
	staticReferenceRows
	staticDeclarationRows
	staticSignatureRows
	staticContractRows
	staticOperatorRows
	staticOperandRows
	staticPublicationRows
}

// Rows is the Static-owned construction plane. The implementation stays
// private to this package; assembly core receives only this owner value and
// its explicit freeze/witness operations.
type Rows struct{ staticRows }

type staticRawField struct {
	key      staticRawKey
	typ      keyspace.Term
	optional bool
}

func validStaticRawLiteral(value keyspace.LiteralValue) bool {
	switch value.Kind {
	case keyspace.LiteralBool:
		return true
	case keyspace.LiteralInteger:
		return true
	case keyspace.LiteralFloat:
		return !math.IsNaN(math.Float64frombits(value.FloatBits))
	case keyspace.LiteralString:
		return true
	default:
		return false
	}
}

func rawString(value string) (staticRawKey, error) {
	if value == "" {
		return staticRawKey{}, errors.New("program/lower/collector: empty Static key")
	}
	return staticRawKey{value: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}, present: true}, nil
}

func rawLiteral(value keyspace.LiteralValue) (staticRawKey, error) {
	if !validStaticRawLiteral(value) {
		return staticRawKey{}, errors.New("program/lower/collector: invalid Static key payload")
	}
	return staticRawKey{value: value, present: true}, nil
}

func staticRawPath(path []string) ([]staticRawKey, error) {
	if len(path) == 0 {
		return nil, errors.New("program/lower/collector: empty Static type path")
	}
	result := make([]staticRawKey, len(path))
	for index, part := range path {
		key, err := rawString(part)
		if err != nil {
			return nil, err
		}
		result[index] = key
	}
	return result, nil
}

func validCoordinateOrZero(coordinate source.Coordinate) bool {
	startLine, startCol, endLine, endCol := coordinate.Parts()
	copy, ok := source.CoordinateFromParts(startLine, startCol, endLine, endCol)
	return ok && copy == coordinate
}

func requireFamily(term keyspace.Term, family keyspace.Family) error {
	if term == 0 || keyspace.TermFamily(term) != family {
		return fmt.Errorf("program/lower/collector: expected family %d term", family)
	}
	return nil
}

func denseOrdinal(term keyspace.Term, family keyspace.Family, length int) (int, error) {
	if err := requireFamily(term, family); err != nil {
		return 0, err
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) > uint64(length) {
		return 0, fmt.Errorf("program/lower/collector: family %d term is not an existing row", family)
	}
	return int(ordinal - 1), nil
}

func staticDenseAppendTerm(term keyspace.Term, family keyspace.Family, length int) error {
	if term == 0 || keyspace.TermFamily(term) != family || keyspace.TermOrdinal(term) != uint32(length+1) {
		return fmt.Errorf("program/lower/collector: noncanonical family %d row", family)
	}
	return nil
}
