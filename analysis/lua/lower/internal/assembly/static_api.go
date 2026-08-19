package assembly

import (
	"errors"

	staticrows "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly/static"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

func (c *Collector) Primitive(span source.Span, kind statictypes.PrimitiveKind) keyspace.Term {
	return staticEmit(c, keyspace.FamilyTypePrimitive, span, func(term keyspace.Term) error { return c.static.Primitive(term, kind) })
}
func (c *Collector) LiteralBool(span source.Span, value bool) keyspace.Term {
	if !staticAdmitExact(c, keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeLiteral, span, func(term keyspace.Term) error { return c.static.LiteralBool(term, value) })
}
func (c *Collector) LiteralInteger(span source.Span, value int64) keyspace.Term {
	if !staticAdmitExact(c, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeLiteral, span, func(term keyspace.Term) error { return c.static.LiteralInteger(term, value) })
}
func (c *Collector) LiteralFloat(span source.Span, value float64) keyspace.Term {
	if !staticFloatBitsValid(c, mathFloatBits(value)) {
		return 0
	}
	return c.LiteralFloatBits(span, mathFloatBits(value))
}
func (c *Collector) LiteralFloatBits(span source.Span, bits uint64) keyspace.Term {
	if !staticFloatBitsValid(c, bits) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeLiteral, span, func(term keyspace.Term) error { return c.static.LiteralFloat(term, bits) })
}
func (c *Collector) LiteralString(span source.Span, value string) keyspace.Term {
	if !staticAdmitExact(c, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeLiteral, span, func(term keyspace.Term) error { return c.static.LiteralString(term, value) })
}
func (c *Collector) Optional(span source.Span, inner keyspace.Term) keyspace.Term {
	if !staticExistingNode(c, inner) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeOptional, span, func(term keyspace.Term) error { return c.static.Optional(term, inner) })
}
func (c *Collector) Union(span source.Span, members []keyspace.Term) keyspace.Term {
	if !staticExistingNodeTerms(c, members) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeUnion, span, func(term keyspace.Term) error { return c.static.Union(term, members) })
}
func (c *Collector) Intersection(span source.Span, members []keyspace.Term) keyspace.Term {
	if !staticExistingNodeTerms(c, members) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeIntersection, span, func(term keyspace.Term) error { return c.static.Intersection(term, members) })
}
func (c *Collector) Generic(span source.Span, base keyspace.Term, args []keyspace.Term) keyspace.Term {
	if !staticExistingFamily(c, base, keyspace.FamilyTypeRef) || !staticExistingNodeTerms(c, args) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeGeneric, span, func(term keyspace.Term) error { return c.static.Generic(term, base, args) })
}
func (c *Collector) Array(span source.Span, element keyspace.Term, readonly bool) keyspace.Term {
	if !staticExistingNode(c, element) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeArray, span, func(term keyspace.Term) error { return c.static.Array(term, element, readonly) })
}
func (c *Collector) Map(span source.Span, key, value keyspace.Term, readonly bool) keyspace.Term {
	if !staticExistingNodeTerms(c, []keyspace.Term{key, value}) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeMap, span, func(term keyspace.Term) error { return c.static.Map(term, key, value, readonly) })
}
func (c *Collector) Field(span source.Span, name string, typ keyspace.Term, optional bool) keyspace.Term {
	if !staticStringValid(c, name) || !staticExistingNode(c, typ) {
		return 0
	}
	if !staticAdmitString(c, name) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeField, span, func(term keyspace.Term) error { return c.static.Field(term, name, typ, optional) })
}
func (c *Collector) Record(span source.Span, fields []keyspace.Term, readonly bool) keyspace.Term {
	if !staticExistingFamilyTerms(c, fields, keyspace.FamilyTypeField) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeRecord, span, func(term keyspace.Term) error { return c.static.Record(term, fields, readonly) })
}

func (c *Collector) Unresolved(span source.Span, path []string, root keyspace.Term) keyspace.Term {
	if !staticPathValid(c, path) || !staticExistingOptional(c, root) {
		return 0
	}
	if !staticAdmitPaths(c, path) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeRef, span, func(term keyspace.Term) error { return c.static.TypeRefUnresolved(term, root, path) })
}
func (c *Collector) Declaration(span source.Span, path []string, root, target keyspace.Term) keyspace.Term {
	if !staticPathValid(c, path) || !staticExistingOptional(c, root) || !staticExistingReferenceTarget(c, target) {
		return 0
	}
	if !staticAdmitPaths(c, path) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeRef, span, func(term keyspace.Term) error { return c.static.TypeRefDeclaration(term, root, target, path) })
}
func (c *Collector) Canonical(span source.Span, path, canonical []string, root keyspace.Term) keyspace.Term {
	if !staticPathValid(c, path) || !staticPathValid(c, canonical) || !staticExistingOptional(c, root) {
		return 0
	}
	if !staticAdmitPaths(c, path, canonical) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeRef, span, func(term keyspace.Term) error { return c.static.TypeRefCanonical(term, root, path, canonical) })
}

// StaticInterfaceMember is the raw-name construction DTO for one interface
// member. Field members use Field and leave Name empty; method members use
// Name and Signature and leave Field zero.
type StaticInterfaceMember struct {
	Kind      staticdecl.InterfaceMemberKind
	Field     keyspace.Term
	Name      string
	Span      source.Span
	Signature keyspace.Term
}

func (c *Collector) Alias(span, nameSpan source.Span, owner keyspace.Term, name string) keyspace.Term {
	if !staticStringValid(c, name) || !staticExistingFamily(c, owner, keyspace.FamilyBody) {
		return 0
	}
	coordinate, ok := staticCoordinate(c, nameSpan)
	if !ok {
		return 0
	}
	if !staticAdmitString(c, name) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeAlias, span, func(term keyspace.Term) error { return c.static.AliasDeclare(term, owner, name, coordinate) })
}
func (c *Collector) AliasParams(alias keyspace.Term, params []keyspace.Term) bool {
	if !staticExistingFamily(c, alias, keyspace.FamilyTypeAlias) || !staticTypeParamsForOwner(c, alias, params) {
		return false
	}
	return staticFill(c, func() error { return c.static.AliasParams(alias, params) })
}
func (c *Collector) AliasTarget(alias, target keyspace.Term) bool {
	if !staticExistingFamily(c, alias, keyspace.FamilyTypeAlias) || !staticExistingNode(c, target) {
		return false
	}
	return staticFill(c, func() error { return c.static.AliasTarget(alias, target) })
}
func (c *Collector) TypeParam(span source.Span, owner keyspace.Term, name string) keyspace.Term {
	if !staticStringValid(c, name) || !staticExistingTypeParameterOwner(c, owner) {
		return 0
	}
	if !staticAdmitString(c, name) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeParam, span, func(term keyspace.Term) error { return c.static.TypeParamDeclare(term, owner, name) })
}
func (c *Collector) TypeParamConstraint(param, constraint keyspace.Term) bool {
	if !staticExistingFamily(c, param, keyspace.FamilyTypeParam) || !staticExistingOptionalNode(c, constraint) {
		return false
	}
	return staticFill(c, func() error { return c.static.TypeParamFill(param, constraint) })
}
func (c *Collector) DeclaredType(span source.Span, cell, target keyspace.Term) keyspace.Term {
	if !staticExistingFamily(c, cell, keyspace.FamilyCell) || !staticExistingNode(c, target) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyDeclaredType, span, func(term keyspace.Term) error { return c.static.DeclaredType(term, cell, target) })
}
func (c *Collector) Interface(span, nameSpan source.Span, owner keyspace.Term, name string) keyspace.Term {
	if !staticStringValid(c, name) || !staticExistingFamily(c, owner, keyspace.FamilyBody) {
		return 0
	}
	coordinate, ok := staticCoordinate(c, nameSpan)
	if !ok {
		return 0
	}
	if !staticAdmitString(c, name) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeInterface, span, func(term keyspace.Term) error {
		return c.static.InterfaceDeclare(term, owner, name, coordinate)
	})
}
func (c *Collector) InterfaceExtends(iface keyspace.Term, extends []keyspace.Term) bool {
	if !staticExistingFamily(c, iface, keyspace.FamilyTypeInterface) || !staticExistingFamilyTerms(c, extends, keyspace.FamilyTypeRef) {
		return false
	}
	return staticFill(c, func() error { return c.static.InterfaceExtends(iface, extends) })
}
func (c *Collector) InterfaceMembers(iface keyspace.Term, members []StaticInterfaceMember) bool {
	if !mutationReady(c) {
		return false
	}
	if !staticExistingFamily(c, iface, keyspace.FamilyTypeInterface) {
		return false
	}
	raw := make([]staticrows.InterfaceMember, len(members))
	memberNames := make([]keyspace.LiteralValue, 0, len(members))
	for index, member := range members {
		coordinate, ok := staticCoordinate(c, member.Span)
		if !ok {
			c.fail(errors.New("program/lower/collector: invalid interface member span"))
			return false
		}
		switch member.Kind {
		case staticdecl.InterfaceField:
			if member.Name != "" || member.Signature != 0 || !staticExistingFamily(c, member.Field, keyspace.FamilyTypeField) {
				return rejectMutationf(c, "program/lower/collector: invalid interface field member")
			}
		case staticdecl.InterfaceMethod:
			if member.Name == "" || member.Field != 0 || !staticTypeFunctionForScope(c, member.Signature, iface) {
				return rejectMutationf(c, "program/lower/collector: invalid interface method member")
			}
		default:
			return rejectMutationf(c, "program/lower/collector: invalid interface member kind")
		}
		if member.Name != "" {
			memberNames = append(memberNames, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: member.Name})
		}
		raw[index] = staticrows.InterfaceMember{Kind: member.Kind, Field: member.Field, Name: member.Name, Coordinate: coordinate, Signature: member.Signature}
	}
	for _, name := range memberNames {
		if !staticAdmitExact(c, name) {
			return false
		}
	}
	return staticFill(c, func() error { return c.static.InterfaceMembers(iface, raw) })
}

// StaticParameter is the raw-name construction DTO for one TypeFunction
// fixed parameter. An empty Name is the explicit unnamed-parameter state.
type StaticParameter struct {
	Name string
	Span source.Span
	Type keyspace.Term
}

func (c *Collector) TypeFunction(span source.Span, scope keyspace.Term) keyspace.Term {
	if !staticExistingScope(c, scope) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeFunction, span, func(term keyspace.Term) error { return c.static.TypeFunctionDeclare(term, scope) })
}
func (c *Collector) TypeFunctionGenerics(function keyspace.Term, params []keyspace.Term) bool {
	if !staticExistingFamily(c, function, keyspace.FamilyTypeFunction) || !staticTypeParamsForOwner(c, function, params) {
		return false
	}
	return staticFill(c, func() error { return c.static.TypeFunctionGenerics(function, params) })
}
func (c *Collector) TypeFunctionParameters(function keyspace.Term, params []StaticParameter) bool {
	if !mutationReady(c) {
		return false
	}
	if !staticExistingFamily(c, function, keyspace.FamilyTypeFunction) {
		return false
	}
	raw := make([]staticrows.Parameter, len(params))
	parameterNames := make([]keyspace.LiteralValue, 0, len(params))
	for index, parameter := range params {
		if !staticExistingNode(c, parameter.Type) {
			return false
		}
		coordinate, ok := staticCoordinate(c, parameter.Span)
		if !ok {
			c.fail(errors.New("program/lower/collector: invalid TypeFunction parameter span"))
			return false
		}
		if parameter.Name != "" {
			parameterNames = append(parameterNames, keyspace.LiteralValue{Kind: keyspace.LiteralString, String: parameter.Name})
		}
		raw[index] = staticrows.Parameter{Name: parameter.Name, Coordinate: coordinate, Type: parameter.Type}
	}
	for _, name := range parameterNames {
		if !staticAdmitExact(c, name) {
			return false
		}
	}
	return staticFill(c, func() error { return c.static.TypeFunctionParameters(function, raw) })
}
func (c *Collector) TypeFunctionVariadic(function keyspace.Term, span source.Span, typ keyspace.Term) bool {
	if !staticExistingFamily(c, function, keyspace.FamilyTypeFunction) || !staticExistingOptionalNode(c, typ) {
		return false
	}
	coordinate, ok := staticCoordinate(c, span)
	if !ok {
		return false
	}
	return staticFill(c, func() error { return c.static.TypeFunctionVariadic(function, typ, coordinate) })
}
func (c *Collector) TypeFunctionReturns(function keyspace.Term, known bool, returns []keyspace.Term) bool {
	if !staticExistingFamily(c, function, keyspace.FamilyTypeFunction) || !staticExistingNodeTerms(c, returns) {
		return false
	}
	return staticFill(c, func() error { return c.static.TypeFunctionReturns(function, known, returns) })
}
func (c *Collector) TypeAsserts(span, paramSpan source.Span, name string, bound bool, param uint32, narrow keyspace.Term) keyspace.Term {
	if !staticStringValid(c, name) || !staticExistingOptionalNode(c, narrow) {
		return 0
	}
	coordinate, ok := staticCoordinate(c, paramSpan)
	if !ok {
		return 0
	}
	if !staticAdmitString(c, name) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeAsserts, span, func(term keyspace.Term) error {
		return c.static.TypeAsserts(term, name, coordinate, bound, param, narrow)
	})
}

func (c *Collector) TypeOf(span source.Span, scope, operand keyspace.Term) keyspace.Term {
	if !staticExistingScope(c, scope) || !staticExistingTerm(c, operand) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeOf, span, func(term keyspace.Term) error { return c.static.TypeOf(term, scope, operand) })
}
func (c *Collector) KeyOf(span source.Span, inner keyspace.Term) keyspace.Term {
	if !staticExistingNode(c, inner) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeKeyOf, span, func(term keyspace.Term) error { return c.static.KeyOf(term, inner) })
}
func (c *Collector) IndexAccess(span source.Span, object, index keyspace.Term) keyspace.Term {
	if !staticExistingNodeTerms(c, []keyspace.Term{object, index}) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeIndexAccess, span, func(term keyspace.Term) error { return c.static.IndexAccess(term, object, index) })
}
func (c *Collector) Conditional(span source.Span, check, extends, then, otherwise keyspace.Term) keyspace.Term {
	if !staticExistingNodeTerms(c, []keyspace.Term{check, extends, then, otherwise}) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypeConditional, span, func(term keyspace.Term) error {
		return c.static.Conditional(term, check, extends, then, otherwise)
	})
}

func (c *Collector) Annotation(span source.Span, scope, target keyspace.Term, name string) keyspace.Term {
	if !staticStringValid(c, name) || !staticExistingScope(c, scope) || !staticExistingAnnotationTarget(c, target) {
		return 0
	}
	if !staticAdmitString(c, name) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyAnnotation, span, func(term keyspace.Term) error { return c.static.AnnotationDeclare(term, scope, target, name) })
}
func (c *Collector) AnnotationValues(annotation, values keyspace.Term) bool {
	if !staticExistingFamily(c, annotation, keyspace.FamilyAnnotation) || !staticExistingFamily(c, values, keyspace.FamilyValues) {
		return false
	}
	return staticFill(c, func() error { return c.static.AnnotationFill(annotation, values) })
}

func (c *Collector) Type(span source.Span, assign keyspace.Term, pair uint32, target keyspace.Term) keyspace.Term {
	if !staticPublicationAdmission(c, assign, pair, target) {
		return 0
	}
	return staticEmit(c, keyspace.FamilyTypePublication, span, func(term keyspace.Term) error { return c.static.TypePublication(term, assign, pair, target) })
}
