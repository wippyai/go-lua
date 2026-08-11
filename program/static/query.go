package static

import (
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func (component *Component) View() View { return View{component: component} }

func (view View) Types() Types { return Types{component: view.component, state: view.state} }
func (view View) References() References {
	return References{component: view.component, state: view.state}
}
func (view View) Declarations() Declarations {
	return Declarations{component: view.component, state: view.state}
}
func (view View) Publications() Publications {
	return Publications{component: view.component, state: view.state}
}

func (view Declarations) Aliases() Aliases {
	return Aliases{component: view.component, state: view.state}
}
func (view Declarations) TypeParams() TypeParams {
	return TypeParams{component: view.component, state: view.state}
}
func (view Declarations) Interfaces() Interfaces {
	return Interfaces{component: view.component, state: view.state}
}

func (view Types) Primitives() Primitives {
	return Primitives{component: view.component, state: view.state}
}
func (view Types) Literals() Literals { return Literals{component: view.component, state: view.state} }
func (view Types) Optionals() Optionals {
	return Optionals{component: view.component, state: view.state}
}
func (view Types) Unions() Unions { return Unions{component: view.component, state: view.state} }
func (view Types) Intersections() Intersections {
	return Intersections{component: view.component, state: view.state}
}
func (view Types) Generics() Generics { return Generics{component: view.component, state: view.state} }
func (view Types) Arrays() Arrays     { return Arrays{component: view.component, state: view.state} }
func (view Types) Maps() Maps         { return Maps{component: view.component, state: view.state} }
func (view Types) Records() Records   { return Records{component: view.component, state: view.state} }
func (view Types) Fields() Fields     { return Fields{component: view.component, state: view.state} }

func (view Primitives) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.primitive)
}
func (view Literals) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.literal)
}
func (view Optionals) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.optional)
}
func (view Unions) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.union)
}
func (view Intersections) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.intersection)
}
func (view Generics) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.generic)
}
func (view Arrays) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.array)
}
func (view Maps) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.mapType)
}
func (view Records) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.record)
}
func (view Fields) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.types.field)
}
func (view References) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.references.rows)
}
func (view Aliases) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.declarations.aliases)
}
func (view TypeParams) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.declarations.params)
}
func (view Interfaces) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.declarations.interfaces)
}

func (view Primitives) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypePrimitive, index, view.Count())
}
func (view Literals) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeLiteral, index, view.Count())
}
func (view Optionals) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeOptional, index, view.Count())
}
func (view Unions) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeUnion, index, view.Count())
}
func (view Intersections) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeIntersection, index, view.Count())
}
func (view Generics) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeGeneric, index, view.Count())
}
func (view Arrays) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeArray, index, view.Count())
}
func (view Maps) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeMap, index, view.Count())
}
func (view Records) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeRecord, index, view.Count())
}
func (view Fields) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeField, index, view.Count())
}
func (view References) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeRef, index, view.Count())
}
func (view Aliases) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeAlias, index, view.Count())
}
func (view TypeParams) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeParam, index, view.Count())
}
func (view Interfaces) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeInterface, index, view.Count())
}

func at(family keyspace.Family, index, length int) (keyspace.Term, bool) {
	if index < 0 || index >= length {
		return 0, false
	}
	return keyspace.MakeTerm(family, uint32(index+1)), true
}

func (view Primitives) Get(term keyspace.Term) (PrimitiveKind, bool) {
	component := view.componentOf()
	row, ok := primitiveRow(component, term)
	return row.Kind, ok
}
func (view Literals) Get(term keyspace.Term) (keyspace.LiteralKind, keyspace.Key, uint64, bool) {
	component := view.componentOf()
	row, ok := literalRow(component, term)
	return row.Kind, row.Exact, row.FloatBits, ok
}
func (view Optionals) Get(term keyspace.Term) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := optionalRow(component, term)
	return row.Inner, ok
}
func (view Arrays) Get(term keyspace.Term) (keyspace.Term, bool, bool) {
	component := view.componentOf()
	row, ok := arrayRow(component, term)
	return row.Element, row.ReadOnly, ok
}
func (view Maps) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool, bool) {
	component := view.componentOf()
	row, ok := mapRow(component, term)
	return row.Key, row.Value, row.ReadOnly, ok
}
func (view Fields) Get(term keyspace.Term) (keyspace.Key, keyspace.Term, bool, bool) {
	component := view.componentOf()
	row, ok := fieldRow(component, term)
	return row.Key, row.Type, row.Optional, ok
}
func (view References) Get(term keyspace.Term) (TypeRefResolution, keyspace.Term, keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	return row.resolution, row.target, row.root, ok
}
func (view References) SourceCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	return int(row.source.End - row.source.Start), ok
}
func (view References) SourceAt(term keyspace.Term, index int) (keyspace.Key, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.source.End-row.source.Start {
		return 0, false
	}
	return component.references.source[row.source.Start+uint32(index)], true
}
func (view References) CanonicalCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	return int(row.canonical.End - row.canonical.Start), ok
}
func (view References) CanonicalAt(term keyspace.Term, index int) (keyspace.Key, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.canonical.End-row.canonical.Start {
		return 0, false
	}
	return component.references.canonical[row.canonical.Start+uint32(index)], true
}

func (view Aliases) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Key, source.Coordinate, bool) {
	component := view.componentOf()
	row, ok := aliasRowAt(component, term)
	return row.owner, row.target, row.name, row.coordinate, ok
}
func (view Aliases) ParamCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := aliasRowAt(component, term)
	return int(row.params.End - row.params.Start), ok
}
func (view Aliases) ParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := aliasRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.params.End-row.params.Start {
		return 0, false
	}
	return component.declarations.aliasParams[row.params.Start+uint32(index)], true
}

func (view TypeParams) Get(term keyspace.Term) (keyspace.Term, keyspace.Key, keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := typeParamRowAt(component, term)
	return row.Owner, row.Name, row.Constraint, ok
}

func (view Interfaces) Get(term keyspace.Term) (keyspace.Term, keyspace.Key, source.Coordinate, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	return row.owner, row.name, row.coordinate, ok
}
func (view Interfaces) ExtendCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	return int(row.extends.End - row.extends.Start), ok
}
func (view Interfaces) ExtendAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.extends.End-row.extends.Start {
		return 0, false
	}
	return component.declarations.interfaceRefs[row.extends.Start+uint32(index)], true
}
func (view Interfaces) MemberCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	return int(row.members.End - row.members.Start), ok
}
func (view Interfaces) MemberAt(term keyspace.Term, index int) (InterfaceMember, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.members.End-row.members.Start {
		return InterfaceMember{}, false
	}
	member := component.declarations.members[row.members.Start+uint32(index)]
	return InterfaceMember{Kind: member.kind, Field: member.field, Name: member.name, NameCoordinate: member.coordinate, Signature: member.signature}, true
}

func (view Unions) MemberCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeUnion, len(component.types.union)) {
		return 0, false
	}
	row := component.types.union[keyspace.TermOrdinal(term)-1]
	return int(row.End - row.Start), true
}
func (view Intersections) MemberCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeIntersection, len(component.types.intersection)) {
		return 0, false
	}
	row := component.types.intersection[keyspace.TermOrdinal(term)-1]
	return int(row.End - row.Start), true
}
func (view Unions) MemberAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeUnion, len(component.types.union)) || index < 0 {
		return 0, false
	}
	row := component.types.union[keyspace.TermOrdinal(term)-1]
	if uint32(index) >= row.End-row.Start {
		return 0, false
	}
	return component.types.terms[row.Start+uint32(index)], true
}
func (view Intersections) MemberAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeIntersection, len(component.types.intersection)) || index < 0 {
		return 0, false
	}
	row := component.types.intersection[keyspace.TermOrdinal(term)-1]
	if uint32(index) >= row.End-row.Start {
		return 0, false
	}
	return component.types.terms[row.Start+uint32(index)], true
}
func (view Generics) Get(term keyspace.Term) (keyspace.Term, int, bool) {
	component := view.componentOf()
	row, ok := genericRowAt(component, term)
	return row.base, int(row.args.End - row.args.Start), ok
}
func (view Generics) ArgAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := genericRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.args.End-row.args.Start {
		return 0, false
	}
	return component.types.terms[row.args.Start+uint32(index)], true
}
func (view Records) Get(term keyspace.Term) (bool, int, bool) {
	component := view.componentOf()
	row, ok := recordRowAt(component, term)
	return row.readOnly, int(row.fields.End - row.fields.Start), ok
}
func (view Records) FieldAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := recordRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.fields.End-row.fields.Start {
		return 0, false
	}
	return component.types.fields[row.fields.Start+uint32(index)], true
}

func primitiveRow(component *Component, term keyspace.Term) (Primitive, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypePrimitive, len(component.types.primitive)) {
		return Primitive{}, false
	}
	return component.types.primitive[keyspace.TermOrdinal(term)-1], true
}
func literalRow(component *Component, term keyspace.Term) (Literal, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeLiteral, len(component.types.literal)) {
		return Literal{}, false
	}
	return component.types.literal[keyspace.TermOrdinal(term)-1], true
}
func optionalRow(component *Component, term keyspace.Term) (Optional, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeOptional, len(component.types.optional)) {
		return Optional{}, false
	}
	return component.types.optional[keyspace.TermOrdinal(term)-1], true
}
func arrayRow(component *Component, term keyspace.Term) (Array, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeArray, len(component.types.array)) {
		return Array{}, false
	}
	return component.types.array[keyspace.TermOrdinal(term)-1], true
}
func mapRow(component *Component, term keyspace.Term) (Map, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeMap, len(component.types.mapType)) {
		return Map{}, false
	}
	return component.types.mapType[keyspace.TermOrdinal(term)-1], true
}
func fieldRow(component *Component, term keyspace.Term) (Field, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeField, len(component.types.field)) {
		return Field{}, false
	}
	return component.types.field[keyspace.TermOrdinal(term)-1], true
}
func genericRowAt(component *Component, term keyspace.Term) (genericRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeGeneric, len(component.types.generic)) {
		return genericRow{}, false
	}
	return component.types.generic[keyspace.TermOrdinal(term)-1], true
}
func recordRowAt(component *Component, term keyspace.Term) (recordRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeRecord, len(component.types.record)) {
		return recordRow{}, false
	}
	return component.types.record[keyspace.TermOrdinal(term)-1], true
}

func typeRefRowAt(component *Component, term keyspace.Term) (typeRefRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeRef, len(component.references.rows)) {
		return typeRefRow{}, false
	}
	return component.references.rows[keyspace.TermOrdinal(term)-1], true
}

func aliasRowAt(component *Component, term keyspace.Term) (typeAliasRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeAlias, len(component.declarations.aliases)) {
		return typeAliasRow{}, false
	}
	return component.declarations.aliases[keyspace.TermOrdinal(term)-1], true
}
func typeParamRowAt(component *Component, term keyspace.Term) (TypeParam, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeParam, len(component.declarations.params)) {
		return TypeParam{}, false
	}
	return component.declarations.params[keyspace.TermOrdinal(term)-1], true
}
func interfaceRowAt(component *Component, term keyspace.Term) (interfaceRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeInterface, len(component.declarations.interfaces)) {
		return interfaceRow{}, false
	}
	return component.declarations.interfaces[keyspace.TermOrdinal(term)-1], true
}
