package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (component *Component) View() View { return View{component: component} }

func (view View) Types() Types { return Types(view) }
func (view View) References() References {
	return References(view)
}
func (view View) Declarations() Declarations {
	return Declarations(view)
}
func (view View) Publications() Publications {
	return Publications(view)
}

func (view Declarations) Aliases() Aliases {
	return Aliases(view)
}
func (view Declarations) TypeParams() TypeParams {
	return TypeParams(view)
}
func (view Declarations) Interfaces() Interfaces {
	return Interfaces(view)
}

func (view Types) Primitives() Primitives {
	return Primitives(view)
}
func (view Types) Literals() Literals { return Literals(view) }
func (view Types) Optionals() Optionals {
	return Optionals(view)
}
func (view Types) Unions() Unions { return Unions(view) }
func (view Types) Intersections() Intersections {
	return Intersections(view)
}
func (view Types) Generics() Generics { return Generics(view) }
func (view Types) Arrays() Arrays     { return Arrays(view) }
func (view Types) Maps() Maps         { return Maps(view) }
func (view Types) Records() Records   { return Records(view) }
func (view Types) Fields() Fields     { return Fields(view) }

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
	return row.source.len(), ok
}
func (view References) SourceAt(term keyspace.Term, index int) (keyspace.Key, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	if !ok {
		return 0, false
	}
	return poolAt(component.references.source, row.source, index)
}
func (view References) CanonicalCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	return row.canonical.len(), ok
}
func (view References) CanonicalAt(term keyspace.Term, index int) (keyspace.Key, bool) {
	component := view.componentOf()
	row, ok := typeRefRowAt(component, term)
	if !ok {
		return 0, false
	}
	return poolAt(component.references.canonical, row.canonical, index)
}

func (view Aliases) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Key, source.Coordinate, bool) {
	component := view.componentOf()
	row, ok := aliasRowAt(component, term)
	return row.owner, row.target, row.name, row.coordinate, ok
}
func (view Aliases) ParamCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := aliasRowAt(component, term)
	return row.params.len(), ok
}
func (view Aliases) ParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := aliasRowAt(component, term)
	if !ok {
		return 0, false
	}
	return poolAt(component.declarations.aliasParams, row.params, index)
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
	return row.extends.len(), ok
}
func (view Interfaces) ExtendAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	if !ok {
		return 0, false
	}
	return poolAt(component.declarations.interfaceRefs, row.extends, index)
}
func (view Interfaces) MemberCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	return row.members.len(), ok
}
func (view Interfaces) MemberAt(term keyspace.Term, index int) (InterfaceMember, bool) {
	component := view.componentOf()
	row, ok := interfaceRowAt(component, term)
	if !ok {
		return InterfaceMember{}, false
	}
	member, found := poolAt(component.declarations.members, row.members, index)
	if !found {
		return InterfaceMember{}, false
	}
	return InterfaceMember{Kind: member.kind, Field: member.field, Name: member.name, NameCoordinate: member.coordinate, Signature: member.signature}, true
}

func (view Unions) MemberCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeUnion, len(component.types.union)) {
		return 0, false
	}
	row := component.types.union[keyspace.TermOrdinal(term)-1]
	return row.len(), true
}
func (view Intersections) MemberCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeIntersection, len(component.types.intersection)) {
		return 0, false
	}
	row := component.types.intersection[keyspace.TermOrdinal(term)-1]
	return row.len(), true
}
func (view Unions) MemberAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeUnion, len(component.types.union)) || index < 0 {
		return 0, false
	}
	row := component.types.union[keyspace.TermOrdinal(term)-1]
	return poolAt(component.types.terms, row, index)
}
func (view Intersections) MemberAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeIntersection, len(component.types.intersection)) || index < 0 {
		return 0, false
	}
	row := component.types.intersection[keyspace.TermOrdinal(term)-1]
	return poolAt(component.types.terms, row, index)
}
func (view Generics) Get(term keyspace.Term) (keyspace.Term, int, bool) {
	component := view.componentOf()
	row, ok := genericRowAt(component, term)
	return row.base, row.args.len(), ok
}
func (view Generics) ArgAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := genericRowAt(component, term)
	if !ok {
		return 0, false
	}
	return poolAt(component.types.terms, row.args, index)
}
func (view Records) Get(term keyspace.Term) (bool, int, bool) {
	component := view.componentOf()
	row, ok := recordRowAt(component, term)
	return row.readOnly, row.fields.len(), ok
}
func (view Records) FieldAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := recordRowAt(component, term)
	if !ok {
		return 0, false
	}
	return poolAt(component.types.fields, row.fields, index)
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
