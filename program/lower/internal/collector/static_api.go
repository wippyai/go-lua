package collector

import (
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
	staticrole "github.com/wippyai/go-lua/program/static/role"
)

func (w StaticRoot) Types() StaticTypes               { return StaticTypes{writer: w} }
func (w StaticRoot) References() StaticReferences     { return StaticReferences{writer: w} }
func (w StaticRoot) Declarations() StaticDeclarations { return StaticDeclarations{writer: w} }
func (w StaticRoot) Signatures() StaticSignatures     { return StaticSignatures{writer: w} }
func (w StaticRoot) Operators() StaticOperators       { return StaticOperators{writer: w} }
func (w StaticRoot) Operands() StaticOperands         { return StaticOperands{writer: w} }
func (w StaticRoot) Publications() StaticPublications { return StaticPublications{writer: w} }

// StaticTypes owns the concrete authored type-expression families.
type StaticTypes struct{ writer StaticRoot }

func (v StaticTypes) Primitive(span source.Span, kind programstatic.PrimitiveKind) Term {
	return staticEmit(v.writer, keyspace.FamilyTypePrimitive, span, func(term Term) error { return v.writer.collector.static.Primitive(term, kind) })
}
func (v StaticTypes) LiteralBool(span source.Span, value bool) Term {
	if !staticAdmitExact(v.writer, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeLiteral, span, func(term Term) error { return v.writer.collector.static.LiteralBool(term, value) })
}
func (v StaticTypes) LiteralInteger(span source.Span, value int64) Term {
	if !staticAdmitExact(v.writer, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeLiteral, span, func(term Term) error { return v.writer.collector.static.LiteralInteger(term, value) })
}
func (v StaticTypes) LiteralFloat(span source.Span, value float64) Term {
	if !staticFloatBitsValid(v.writer, mathFloatBits(value)) {
		return 0
	}
	return v.LiteralFloatBits(span, mathFloatBits(value))
}
func (v StaticTypes) LiteralFloatBits(span source.Span, bits uint64) Term {
	if !staticFloatBitsValid(v.writer, bits) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeLiteral, span, func(term Term) error { return v.writer.collector.static.LiteralFloat(term, bits) })
}
func (v StaticTypes) LiteralString(span source.Span, value string) Term {
	if !staticAdmitExact(v.writer, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeLiteral, span, func(term Term) error { return v.writer.collector.static.LiteralString(term, value) })
}
func (v StaticTypes) Optional(span source.Span, inner Term) Term {
	if !staticExistingNode(v.writer, inner) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeOptional, span, func(term Term) error { return v.writer.collector.static.Optional(term, inner) })
}
func (v StaticTypes) Union(span source.Span, members []Term) Term {
	if !staticExistingNodeTerms(v.writer, members) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeUnion, span, func(term Term) error { return v.writer.collector.static.Union(term, members) })
}
func (v StaticTypes) Intersection(span source.Span, members []Term) Term {
	if !staticExistingNodeTerms(v.writer, members) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeIntersection, span, func(term Term) error { return v.writer.collector.static.Intersection(term, members) })
}
func (v StaticTypes) Generic(span source.Span, base Term, args []Term) Term {
	if !staticExistingFamily(v.writer, base, keyspace.FamilyTypeRef) || !staticExistingNodeTerms(v.writer, args) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeGeneric, span, func(term Term) error { return v.writer.collector.static.Generic(term, base, args) })
}
func (v StaticTypes) Array(span source.Span, element Term, readonly bool) Term {
	if !staticExistingNode(v.writer, element) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeArray, span, func(term Term) error { return v.writer.collector.static.Array(term, element, readonly) })
}
func (v StaticTypes) Map(span source.Span, key, value Term, readonly bool) Term {
	if !staticExistingNodeTerms(v.writer, []Term{key, value}) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeMap, span, func(term Term) error { return v.writer.collector.static.Map(term, key, value, readonly) })
}
func (v StaticTypes) Field(span source.Span, name string, typ Term, optional bool) Term {
	if !staticStringValid(v.writer, name) || !staticExistingNode(v.writer, typ) {
		return 0
	}
	if !staticAdmitString(v.writer, name) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeField, span, func(term Term) error { return v.writer.collector.static.Field(term, name, typ, optional) })
}
func (v StaticTypes) Record(span source.Span, fields []Term, readonly bool) Term {
	if !staticExistingFamilyTerms(v.writer, fields, keyspace.FamilyTypeField) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeRecord, span, func(term Term) error { return v.writer.collector.static.Record(term, fields, readonly) })
}

// StaticReferences owns TypeRef spelling and binder disposition.
type StaticReferences struct{ writer StaticRoot }

func (v StaticReferences) Unresolved(span source.Span, path []string, root Term) Term {
	if !staticPathValid(v.writer, path) || !staticExistingOptional(v.writer, root) {
		return 0
	}
	if !staticAdmitPaths(v.writer, path) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeRef, span, func(term Term) error { return v.writer.collector.static.TypeRefUnresolved(term, root, path) })
}
func (v StaticReferences) Declaration(span source.Span, path []string, root, target Term) Term {
	if !staticPathValid(v.writer, path) || !staticExistingOptional(v.writer, root) || !staticExistingReferenceTarget(v.writer, target) {
		return 0
	}
	if !staticAdmitPaths(v.writer, path) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeRef, span, func(term Term) error { return v.writer.collector.static.TypeRefDeclaration(term, root, target, path) })
}
func (v StaticReferences) Canonical(span source.Span, path, canonical []string, root Term) Term {
	if !staticPathValid(v.writer, path) || !staticPathValid(v.writer, canonical) || !staticExistingOptional(v.writer, root) {
		return 0
	}
	if !staticAdmitPaths(v.writer, path, canonical) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeRef, span, func(term Term) error { return v.writer.collector.static.TypeRefCanonical(term, root, path, canonical) })
}

// StaticInterfaceMember is the raw-name construction DTO for one interface
// member. Field members use Field and leave Name empty; method members use
// Name and Signature and leave Field zero.
type StaticInterfaceMember struct {
	Kind      programstatic.InterfaceMemberKind
	Field     Term
	Name      string
	Span      source.Span
	Signature Term
}

// StaticDeclarations owns aliases, parameters, interfaces, and Cell type
// declarations. Each fill operation is terminal for its exact row.
type StaticDeclarations struct{ writer StaticRoot }

func (v StaticDeclarations) Alias(span, nameSpan source.Span, owner Term, name string) Term {
	if !staticStringValid(v.writer, name) || !staticExistingFamily(v.writer, owner, keyspace.FamilyBody) {
		return 0
	}
	coordinate, ok := staticCoordinate(v.writer, nameSpan)
	if !ok {
		return 0
	}
	if !staticAdmitString(v.writer, name) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeAlias, span, func(term Term) error { return v.writer.collector.static.AliasDeclare(term, owner, name, coordinate) })
}
func (v StaticDeclarations) AliasParams(alias Term, params []Term) bool {
	if !staticExistingFamily(v.writer, alias, keyspace.FamilyTypeAlias) || !staticTypeParamsForOwner(v.writer, alias, params) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.AliasParams(alias, params) })
}
func (v StaticDeclarations) AliasTarget(alias, target Term) bool {
	if !staticExistingFamily(v.writer, alias, keyspace.FamilyTypeAlias) || !staticExistingNode(v.writer, target) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.AliasTarget(alias, target) })
}
func (v StaticDeclarations) TypeParam(span source.Span, owner Term, name string) Term {
	if !staticStringValid(v.writer, name) || !staticExistingTypeParameterOwner(v.writer, owner) {
		return 0
	}
	if !staticAdmitString(v.writer, name) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeParam, span, func(term Term) error { return v.writer.collector.static.TypeParamDeclare(term, owner, name) })
}
func (v StaticDeclarations) TypeParamConstraint(param, constraint Term) bool {
	if !staticExistingFamily(v.writer, param, keyspace.FamilyTypeParam) || !staticExistingOptionalNode(v.writer, constraint) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.TypeParamFill(param, constraint) })
}
func (v StaticDeclarations) DeclaredType(span source.Span, cell, target Term) Term {
	if !staticExistingFamily(v.writer, cell, keyspace.FamilyCell) || !staticExistingNode(v.writer, target) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyDeclaredType, span, func(term Term) error { return v.writer.collector.static.DeclaredType(term, cell, target) })
}
func (v StaticDeclarations) Interface(span, nameSpan source.Span, owner Term, name string) Term {
	if !staticStringValid(v.writer, name) || !staticExistingFamily(v.writer, owner, keyspace.FamilyBody) {
		return 0
	}
	coordinate, ok := staticCoordinate(v.writer, nameSpan)
	if !ok {
		return 0
	}
	if !staticAdmitString(v.writer, name) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeInterface, span, func(term Term) error {
		return v.writer.collector.static.InterfaceDeclare(term, owner, name, coordinate)
	})
}
func (v StaticDeclarations) InterfaceExtends(iface Term, extends []Term) bool {
	if !staticExistingFamily(v.writer, iface, keyspace.FamilyTypeInterface) || !staticExistingFamilyTerms(v.writer, extends, keyspace.FamilyTypeRef) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.InterfaceExtends(iface, extends) })
}
func (v StaticDeclarations) InterfaceMembers(iface Term, members []StaticInterfaceMember) bool {
	if !mutationReady(v.writer.collector) {
		return false
	}
	if !staticExistingFamily(v.writer, iface, keyspace.FamilyTypeInterface) {
		return false
	}
	raw := make([]staticRawInterfaceMember, len(members))
	memberNames := make([]keyspace.LiteralValue, 0, len(members))
	for index, member := range members {
		coordinate, ok := staticCoordinate(v.writer, member.Span)
		if !ok {
			v.writer.collector.fail(errors.New("program/lower/collector: invalid interface member span"))
			return false
		}
		var name staticRawKey
		if member.Name != "" {
			payload, err := rawString(member.Name)
			if err != nil {
				v.writer.collector.fail(err)
				return false
			}
			name = payload
		}
		switch member.Kind {
		case programstatic.InterfaceField:
			if member.Name != "" || member.Signature != 0 || !staticExistingFamily(v.writer, member.Field, keyspace.FamilyTypeField) {
				return rejectMutationf(v.writer.collector, "program/lower/collector: invalid interface field member")
			}
		case programstatic.InterfaceMethod:
			if member.Name == "" || member.Field != 0 || !staticTypeFunctionForScope(v.writer, member.Signature, iface) {
				return rejectMutationf(v.writer.collector, "program/lower/collector: invalid interface method member")
			}
		default:
			return rejectMutationf(v.writer.collector, "program/lower/collector: invalid interface member kind")
		}
		if name.present {
			memberNames = append(memberNames, name.value)
		}
		raw[index] = staticRawInterfaceMember{kind: member.Kind, field: member.Field, name: name, coordinate: coordinate, signature: member.Signature}
	}
	for _, name := range memberNames {
		if !staticAdmitExact(v.writer, name) {
			return false
		}
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.InterfaceMembersRaw(iface, raw) })
}

// StaticParameter is the raw-name construction DTO for one TypeFunction
// fixed parameter. An empty Name is the explicit unnamed-parameter state.
type StaticParameter struct {
	Name string
	Span source.Span
	Type Term
}

// StaticSignatures owns source-only TypeFunction and TypeAsserts rows.
type StaticSignatures struct{ writer StaticRoot }

func (v StaticSignatures) TypeFunction(span source.Span, scope Term) Term {
	if !staticExistingScope(v.writer, scope) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeFunction, span, func(term Term) error { return v.writer.collector.static.TypeFunctionDeclare(term, scope) })
}
func (v StaticSignatures) TypeFunctionGenerics(function Term, params []Term) bool {
	if !staticExistingFamily(v.writer, function, keyspace.FamilyTypeFunction) || !staticTypeParamsForOwner(v.writer, function, params) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.TypeFunctionGenerics(function, params) })
}
func (v StaticSignatures) TypeFunctionParameters(function Term, params []StaticParameter) bool {
	if !mutationReady(v.writer.collector) {
		return false
	}
	if !staticExistingFamily(v.writer, function, keyspace.FamilyTypeFunction) {
		return false
	}
	raw := make([]staticRawParameter, len(params))
	parameterNames := make([]keyspace.LiteralValue, 0, len(params))
	for index, parameter := range params {
		if !staticExistingNode(v.writer, parameter.Type) {
			return false
		}
		coordinate, ok := staticCoordinate(v.writer, parameter.Span)
		if !ok {
			v.writer.collector.fail(errors.New("program/lower/collector: invalid TypeFunction parameter span"))
			return false
		}
		var name staticRawKey
		if parameter.Name != "" {
			payload, err := rawString(parameter.Name)
			if err != nil {
				v.writer.collector.fail(err)
				return false
			}
			name = payload
			parameterNames = append(parameterNames, payload.value)
		}
		raw[index] = staticRawParameter{name: name, coordinate: coordinate, typ: parameter.Type}
	}
	for _, name := range parameterNames {
		if !staticAdmitExact(v.writer, name) {
			return false
		}
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.TypeFunctionParametersRaw(function, raw) })
}
func (v StaticSignatures) TypeFunctionVariadic(function Term, span source.Span, typ Term) bool {
	if !staticExistingFamily(v.writer, function, keyspace.FamilyTypeFunction) || !staticExistingOptionalNode(v.writer, typ) {
		return false
	}
	coordinate, ok := staticCoordinate(v.writer, span)
	if !ok {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.TypeFunctionVariadic(function, typ, coordinate) })
}
func (v StaticSignatures) TypeFunctionReturns(function Term, known bool, returns []Term) bool {
	if !staticExistingFamily(v.writer, function, keyspace.FamilyTypeFunction) || !staticExistingNodeTerms(v.writer, returns) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.TypeFunctionReturns(function, known, returns) })
}
func (v StaticSignatures) TypeAsserts(span, paramSpan source.Span, name string, bound bool, param uint32, narrow Term) Term {
	if !staticStringValid(v.writer, name) || !staticExistingOptionalNode(v.writer, narrow) {
		return 0
	}
	coordinate, ok := staticCoordinate(v.writer, paramSpan)
	if !ok {
		return 0
	}
	if !staticAdmitString(v.writer, name) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeAsserts, span, func(term Term) error {
		return v.writer.collector.static.TypeAsserts(term, name, coordinate, bound, param, narrow)
	})
}

// StaticOperators owns the four concrete authored static operators.
type StaticOperators struct{ writer StaticRoot }

func (v StaticOperators) TypeOf(span source.Span, scope, operand Term) Term {
	if !staticExistingScope(v.writer, scope) || !staticExistingTerm(v.writer, operand) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeOf, span, func(term Term) error { return v.writer.collector.static.TypeOf(term, scope, operand) })
}
func (v StaticOperators) KeyOf(span source.Span, inner Term) Term {
	if !staticExistingNode(v.writer, inner) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeKeyOf, span, func(term Term) error { return v.writer.collector.static.KeyOf(term, inner) })
}
func (v StaticOperators) IndexAccess(span source.Span, object, index Term) Term {
	if !staticExistingNodeTerms(v.writer, []Term{object, index}) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeIndexAccess, span, func(term Term) error { return v.writer.collector.static.IndexAccess(term, object, index) })
}
func (v StaticOperators) Conditional(span source.Span, check, extends, then, otherwise Term) Term {
	if !staticExistingNodeTerms(v.writer, []Term{check, extends, then, otherwise}) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypeConditional, span, func(term Term) error {
		return v.writer.collector.static.Conditional(term, check, extends, then, otherwise)
	})
}

// StaticOperands owns authored Annotation rows. Flow's operand writer is the
// only public path for ClaimTarget and TypeValueTarget sidecars.
type StaticOperands struct{ writer StaticRoot }

func (v StaticOperands) Annotation(span source.Span, scope, target Term, name string) Term {
	if !staticStringValid(v.writer, name) || !staticExistingScope(v.writer, scope) || !staticExistingAnnotationTarget(v.writer, target) {
		return 0
	}
	if !staticAdmitString(v.writer, name) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyAnnotation, span, func(term Term) error { return v.writer.collector.static.AnnotationDeclare(term, scope, target, name) })
}
func (v StaticOperands) AnnotationValues(annotation, values Term) bool {
	if !staticExistingFamily(v.writer, annotation, keyspace.FamilyAnnotation) || !staticExistingFamily(v.writer, values, keyspace.FamilyValues) {
		return false
	}
	return staticFill(v.writer, func() error { return v.writer.collector.static.AnnotationFill(annotation, values) })
}

// StaticPublications owns the dense authored Assign-pair publication rows.
type StaticPublications struct{ writer StaticRoot }

func (v StaticPublications) Type(span source.Span, assign Term, pair uint32, target Term) Term {
	if !staticPublicationAdmission(v.writer, assign, pair, target) {
		return 0
	}
	return staticEmit(v.writer, keyspace.FamilyTypePublication, span, func(term Term) error { return v.writer.collector.static.TypePublication(term, assign, pair, target) })
}

func staticCoordinate(w StaticRoot, span source.Span) (source.Coordinate, bool) {
	if w.collector == nil || !mutationReady(w.collector) {
		return source.Coordinate{}, false
	}
	if !validSpan(w.collector, span) {
		rejectMutationf(w.collector, "program/lower/collector: invalid Static span")
		return source.Coordinate{}, false
	}
	coordinate, ok := source.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	if !ok {
		w.collector.fail(errors.New("program/lower/collector: invalid Static coordinate"))
		return source.Coordinate{}, false
	}
	return coordinate, true
}

func staticEmit(w StaticRoot, family keyspace.Family, span source.Span, accept func(Term) error) Term {
	if w.collector == nil {
		return 0
	}
	term := w.collector.mint(family, span)
	if term == 0 {
		return 0
	}
	if err := accept(term); err != nil {
		w.collector.fail(err)
		return 0
	}
	return term
}

func staticFill(w StaticRoot, apply func() error) bool {
	if !mutationReady(w.collector) {
		return false
	}
	if err := apply(); err != nil {
		w.collector.fail(err)
		return false
	}
	return true
}

// staticAdmitExact is the only Static-to-Source coordination point. Static
// rows retain the raw payload, but never retain a Source owner or call an
// admission callback. The concrete Collector operation below appends the
// payload to Source's single exact denominator before the row is minted.
func staticAdmitExact(w StaticRoot, value keyspace.LiteralValue) bool {
	if w.collector == nil || !mutationReady(w.collector) {
		return false
	}
	if !validRawExactCandidate(value) {
		if w.collector != nil && !w.collector.terminal {
			w.collector.fail(errors.New("program/lower/collector: invalid Static exact payload"))
		}
		return false
	}
	if w.collector.addExact(value) {
		return true
	}
	if w.collector.err == nil && !w.collector.terminal {
		w.collector.fail(errors.New("program/lower/collector: Source exact admission rejected payload"))
	}
	return false
}

func staticAdmitString(w StaticRoot, value string) bool {
	if !staticStringValid(w, value) {
		return false
	}
	key, err := rawString(value)
	if err != nil {
		if w.collector != nil {
			w.collector.fail(err)
		}
		return false
	}
	return staticAdmitExact(w, key.value)
}

// staticAdmitPaths validates every component of every path first, then
// submits all components through the concrete Source operation. This keeps a
// malformed later component from leaving an earlier path component admitted.
func staticAdmitPaths(w StaticRoot, paths ...[]string) bool {
	for _, path := range paths {
		if !staticPathValid(w, path) {
			return false
		}
	}
	for _, path := range paths {
		for _, part := range path {
			if !staticAdmitString(w, part) {
				return false
			}
		}
	}
	return true
}

func staticStringValid(w StaticRoot, value string) bool {
	if value != "" {
		return true
	}
	if w.collector != nil && !w.collector.terminal && w.collector.err == nil {
		w.collector.fail(errors.New("program/lower/collector: empty Static key"))
	}
	return false
}

func staticPathValid(w StaticRoot, path []string) bool {
	if len(path) == 0 {
		if w.collector != nil && !w.collector.terminal && w.collector.err == nil {
			w.collector.fail(errors.New("program/lower/collector: empty Static type path"))
		}
		return false
	}
	for _, part := range path {
		if !staticStringValid(w, part) {
			return false
		}
	}
	return true
}

func staticFloatBitsValid(w StaticRoot, bits uint64) bool {
	if !math.IsNaN(math.Float64frombits(bits)) {
		return true
	}
	if w.collector != nil && !w.collector.terminal && w.collector.err == nil {
		w.collector.fail(errors.New("program/lower/collector: NaN static float literal"))
	}
	return false
}

// existingTerm closes the construction-time child edge against the one
// Collector census. A Static relation may point to a predeclared term, but it
// may not smuggle a future ordinal that only becomes valid after a later mint.
func staticExistingTerm(w StaticRoot, term Term) bool {
	if w.collector == nil || !mutationReady(w.collector) {
		return false
	}
	if !validTermInCounts(w.collector, term) {
		if w.collector.err == nil && !w.collector.terminal {
			w.collector.fail(fmt.Errorf("program/lower/collector: Static child term %d is not already present", term))
		}
		return false
	}
	return true
}

func staticExistingNode(w StaticRoot, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.Node(w.collector.counts, term) {
		w.collector.fail(fmt.Errorf("program/lower/collector: Static term %d is not a type node", term))
		return false
	}
	return true
}

func staticExistingNodeTerms(w StaticRoot, terms []Term) bool {
	for _, term := range terms {
		if !staticExistingNode(w, term) {
			return false
		}
	}
	return true
}

func staticExistingOptionalNode(w StaticRoot, term Term) bool {
	return term == 0 || staticExistingNode(w, term)
}

func staticExistingScope(w StaticRoot, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.ScopeHandle(w.collector.counts, term) {
		w.collector.fail(fmt.Errorf("program/lower/collector: Static term %d is not a scope handle", term))
		return false
	}
	return true
}

func staticExistingReferenceTarget(w StaticRoot, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.TypeReferenceTarget(w.collector.counts, term) {
		w.collector.fail(fmt.Errorf("program/lower/collector: Static term %d is not a TypeRef target", term))
		return false
	}
	return true
}

func staticExistingTypeParameterOwner(w StaticRoot, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.TypeParameterOwner(w.collector.counts, term) {
		w.collector.fail(fmt.Errorf("program/lower/collector: Static term %d is not a TypeParam owner", term))
		return false
	}
	return true
}

func staticExistingAnnotationTarget(w StaticRoot, term Term) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if !staticrole.AnnotationTarget(w.collector.counts, term) {
		w.collector.fail(fmt.Errorf("program/lower/collector: Static term %d is not an Annotation target", term))
		return false
	}
	return true
}

func staticExistingFamily(w StaticRoot, term Term, family keyspace.Family) bool {
	if !staticExistingTerm(w, term) {
		return false
	}
	if keyspace.TermFamily(term) != family {
		rejectMutationf(w.collector, "program/lower/collector: Static term %d has wrong family", term)
		return false
	}
	return true
}

func staticExistingOptional(w StaticRoot, term Term) bool {
	return term == 0 || staticExistingTerm(w, term)
}

func staticExistingFamilyTerms(w StaticRoot, terms []Term, family keyspace.Family) bool {
	for _, term := range terms {
		if !staticExistingFamily(w, term, family) {
			return false
		}
	}
	return true
}

func staticTypeFunctionForScope(w StaticRoot, signature, scope Term) bool {
	if !staticExistingFamily(w, signature, keyspace.FamilyTypeFunction) {
		return false
	}
	ordinal := keyspace.TermOrdinal(signature)
	if ordinal == 0 || uint64(ordinal) > uint64(len(w.collector.static.typeFunctions)) {
		w.collector.fail(fmt.Errorf("program/lower/collector: TypeFunction %d row is absent", signature))
		return false
	}
	if w.collector.static.typeFunctions[ordinal-1].scope != scope {
		w.collector.fail(fmt.Errorf("program/lower/collector: TypeFunction %d has foreign interface scope", signature))
		return false
	}
	return true
}

func staticPublicationAdmission(w StaticRoot, assign Term, pair uint32, target Term) bool {
	if !staticExistingFamily(w, assign, keyspace.FamilyAssign) || !staticExistingFamily(w, target, keyspace.FamilyTypeRef) {
		return false
	}
	targetOrdinal := keyspace.TermOrdinal(target)
	if targetOrdinal == 0 || uint64(targetOrdinal) > uint64(len(w.collector.static.references)) {
		w.collector.fail(fmt.Errorf("program/lower/collector: publication TypeRef %d row is absent", target))
		return false
	}
	ref := w.collector.static.references[targetOrdinal-1]
	if ref.resolution != programstatic.TypeRefDeclaration && ref.resolution != programstatic.TypeRefCanonicalPath {
		w.collector.fail(fmt.Errorf("program/lower/collector: publication target %d is unresolved", target))
		return false
	}
	for _, publication := range w.collector.static.publications {
		if publication.Assign == assign && publication.Pair == pair {
			w.collector.fail(fmt.Errorf("program/lower/collector: duplicate publication Assign %d pair %d", assign, pair))
			return false
		}
	}
	return true
}

// staticTypeParamsForOwner closes a generic-child edge against the authored
// TypeParam declaration row. A family/count check alone permits a parameter
// belonging to another alias/signature/function and permits duplicate claims
// in one generic list; both would otherwise survive until Static freeze.
func staticTypeParamsForOwner(w StaticRoot, owner Term, params []Term) bool {
	if w.collector == nil {
		return false
	}
	seen := make(map[Term]struct{}, len(params))
	for _, param := range params {
		if !staticExistingFamily(w, param, keyspace.FamilyTypeParam) {
			return false
		}
		ordinal := keyspace.TermOrdinal(param)
		if ordinal == 0 || uint64(ordinal) > uint64(len(w.collector.static.params)) {
			w.collector.fail(fmt.Errorf("program/lower/collector: TypeParam %d row is absent", param))
			return false
		}
		if w.collector.static.params[ordinal-1].owner != owner {
			w.collector.fail(fmt.Errorf("program/lower/collector: TypeParam %d belongs to another owner", param))
			return false
		}
		if _, duplicate := seen[param]; duplicate {
			w.collector.fail(fmt.Errorf("program/lower/collector: TypeParam %d claimed more than once", param))
			return false
		}
		seen[param] = struct{}{}
	}
	return true
}

// Keep the conversion local to this construction API. The public method
// accepts float64, while Static's authored row retains exact IEEE bits.
func mathFloatBits(value float64) uint64 {
	return math.Float64bits(value)
}
