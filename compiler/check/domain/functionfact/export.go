package functionfact

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/exportkey"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ProjectExportType replaces exported function fields with their canonical
// public function fact type. Body/entry parameter facts remain available to
// callee analysis, but module exports expose caller contracts from Signature and
// Params only.
func ProjectExportType(export typ.Type, rootName string, facts api.FunctionFacts, graph *cfg.Graph) typ.Type {
	if export == nil || graph == nil || len(facts) == 0 {
		return export
	}
	return projectExportTypeForNames(export, rootName, facts, graph)
}

type exportFieldTypes map[exportkey.MemberPathKey]exportFieldType

type exportFieldType struct {
	path exportkey.MemberPath
	typ  typ.Type
}

func projectExportTypeForNames(export typ.Type, rootName string, facts api.FunctionFacts, source exportkey.SymbolSource) typ.Type {
	if export == nil || source == nil || len(facts) == 0 {
		return export
	}
	fieldTypes := make(exportFieldTypes)
	for _, sym := range cfg.SortedSymbolIDs(facts) {
		projected := ExportTypeProjection(facts, sym, api.PhaseNarrowing)
		if projected == nil || unwrap.Function(projected) == nil {
			continue
		}
		path, ok := exportkey.MemberPathFromGraphSymbol(rootName, source, sym)
		if !ok {
			continue
		}
		fieldTypes[path.Key()] = exportFieldType{path: path, typ: projected}
	}
	if len(fieldTypes) == 0 {
		return export
	}

	return projectExportType(export, fieldTypes, 0)
}

func projectExportType(t typ.Type, fieldTypes exportFieldTypes, depth int) typ.Type {
	if t == nil || len(fieldTypes) == 0 || typ.DepthExceeded(depth) {
		return t
	}
	if projected, ok := projectDirectFunctionExport(t, fieldTypes, depth+1); ok {
		return projected
	}
	out := t
	for _, fieldType := range sortedExportFieldTypes(fieldTypes) {
		out = projectExportPath(out, fieldType.path.Segments(), fieldType.typ, fieldTypes, depth+1)
	}
	return out
}

func projectDirectFunctionExport(t typ.Type, fieldTypes exportFieldTypes, depth int) (typ.Type, bool) {
	if unwrap.Function(t) == nil || len(fieldTypes) != 1 {
		return nil, false
	}
	fieldType := sortedExportFieldTypes(fieldTypes)[0]
	if len(fieldType.path.Segments()) != 1 {
		return nil, false
	}
	projected := publicExportFunctionType(t, fieldType.typ, fieldTypes, depth+1)
	if projected == nil {
		return nil, false
	}
	return projected, true
}

func sortedExportFieldTypes(fieldTypes exportFieldTypes) []exportFieldType {
	keys := make([]exportkey.MemberPathKey, 0, len(fieldTypes))
	for key := range fieldTypes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return string(keys[i]) < string(keys[j])
	})
	out := make([]exportFieldType, 0, len(keys))
	for _, key := range keys {
		out = append(out, fieldTypes[key])
	}
	return out
}

func projectExportPath(t typ.Type, path []fieldkey.Key, public typ.Type, fieldTypes exportFieldTypes, depth int) typ.Type {
	if t == nil || len(path) == 0 || typ.DepthExceeded(depth) {
		return t
	}
	base := unwrap.Alias(t)
	var out typ.Type
	switch v := base.(type) {
	case *typ.Record:
		out = projectRecordExportPath(v, path, public, fieldTypes, depth+1)
	case *typ.Interface:
		out = projectInterfaceExportPath(v, path, public, fieldTypes, depth+1)
	case *typ.Optional:
		inner := projectExportPath(v.Inner, path, public, fieldTypes, depth+1)
		if typ.TypeEquals(inner, v.Inner) {
			out = base
		} else {
			out = typ.NewOptional(inner)
		}
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		for i, member := range v.Members {
			members[i] = projectExportPath(member, path, public, fieldTypes, depth+1)
			changed = changed || !typ.TypeEquals(members[i], member)
		}
		if !changed {
			out = base
		} else {
			out = typ.NewUnion(members...)
		}
	default:
		return t
	}
	if out == base {
		return t
	}
	if alias, ok := t.(*typ.Alias); ok {
		return typ.NewAlias(alias.Name, out)
	}
	return out
}

func projectRecordExportPath(rec *typ.Record, path []fieldkey.Key, public typ.Type, fieldTypes exportFieldTypes, depth int) typ.Type {
	if rec == nil || len(path) == 0 {
		return rec
	}
	changed := false
	fields := make([]typ.Field, len(rec.Fields))
	for i, field := range rec.Fields {
		fields[i] = field
		key, ok := fieldkey.FromName(field.Name)
		if !ok || key != path[0] {
			continue
		}
		fieldType := projectExportMemberType(field.Type, path[1:], public, fieldTypes, depth+1)
		if typ.TypeEquals(field.Type, fieldType) {
			continue
		}
		fields[i].Type = fieldType
		changed = true
	}
	staticMembers := make([]typ.StaticMember, len(rec.StaticMembers))
	for i, member := range rec.StaticMembers {
		staticMembers[i] = member
		key, ok := staticMemberKey(member)
		if !ok || key != path[0] {
			continue
		}
		memberType := projectExportMemberType(member.Type, path[1:], public, fieldTypes, depth+1)
		if typ.TypeEquals(member.Type, memberType) {
			continue
		}
		staticMembers[i].Type = memberType
		changed = true
	}
	if !changed {
		return rec
	}
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	for _, member := range staticMembers {
		builder.AddStaticMember(member)
	}
	for _, field := range fields {
		addRecordField(builder, field)
	}
	return builder.Build()
}

func projectExportMemberType(current typ.Type, rest []fieldkey.Key, public typ.Type, fieldTypes exportFieldTypes, depth int) typ.Type {
	if len(rest) == 0 {
		if projected := publicExportFunctionType(current, public, fieldTypes, depth+1); projected != nil {
			return projected
		}
		return current
	}
	return projectExportPath(current, rest, public, fieldTypes, depth+1)
}

func staticMemberKey(member typ.StaticMember) (fieldkey.Key, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return fieldkey.FromSegment(fieldkey.Key{Kind: constraint.SegmentIndexString, Name: member.Name})
	case typ.StaticMemberIntIndex:
		return fieldkey.FromSegment(fieldkey.Key{Kind: constraint.SegmentIndexInt, Index: int(member.Index)})
	default:
		return fieldkey.Key{}, false
	}
}

func projectInterfaceExportPath(iface *typ.Interface, path []fieldkey.Key, public typ.Type, fieldTypes exportFieldTypes, depth int) typ.Type {
	if iface == nil || len(path) != 1 {
		return iface
	}
	name, ok := fieldkey.StringKeyFromSegment(path[0])
	if !ok {
		return iface
	}
	changed := false
	methods := make([]typ.Method, len(iface.Methods))
	for i, method := range iface.Methods {
		methods[i] = method
		if method.Name != name {
			continue
		}
		publicFn := unwrap.Function(publicExportFunctionType(method.Type, public, fieldTypes, depth+1))
		if publicFn == nil || typ.TypeEquals(method.Type, publicFn) {
			continue
		}
		methods[i].Type = publicFn
		changed = true
	}
	if !changed {
		return iface
	}
	return typ.NewInterface(iface.Name, methods)
}

func publicExportFunctionType(current, public typ.Type, fieldTypes exportFieldTypes, depth int) typ.Type {
	currentFn := unwrap.Function(current)
	publicFn := unwrap.Function(public)
	if currentFn == nil && publicFn == nil {
		return nil
	}
	sourceFn := publicFn
	if sourceFn == nil {
		sourceFn = currentFn
	}
	if publicFn != nil && (publicFn.Refinement != nil || publicFn.Effects != nil || publicFn.Spec != nil) {
		return projectFunctionReturns(publicFn, fieldTypes, depth+1, nil)
	}
	return projectFunctionReturns(sourceFn, fieldTypes, depth+1, currentFn)
}

func projectFunctionReturns(fn *typ.Function, fieldTypes exportFieldTypes, depth int, preserve *typ.Function) typ.Type {
	if fn == nil {
		return nil
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder.TypeParamRef(tp)
	}
	for _, param := range fn.Params {
		if param.Optional {
			builder.OptParam(param.Name, param.Type)
		} else {
			builder.Param(param.Name, param.Type)
		}
	}
	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}
	if len(fn.Returns) > 0 {
		returns := make([]typ.Type, len(fn.Returns))
		for i, ret := range fn.Returns {
			returns[i] = projectExportType(ret, nil, depth+1)
		}
		builder.Returns(returns...)
	}
	if fn.Effects != nil {
		builder.Effects(fn.Effects)
	} else if preserve != nil && preserve.Effects != nil {
		builder.Effects(preserve.Effects)
	}
	if fn.Spec != nil {
		builder.Spec(fn.Spec)
	} else if preserve != nil && preserve.Spec != nil {
		builder.Spec(preserve.Spec)
	}
	if fn.Refinement != nil {
		builder.WithRefinement(fn.Refinement)
	} else if preserve != nil && preserve.Refinement != nil {
		builder.WithRefinement(preserve.Refinement)
	}
	return builder.Build()
}

func addRecordField(builder *typ.RecordBuilder, field typ.Field) {
	switch {
	case field.Optional && field.Readonly:
		builder.OptReadonlyField(field.Name, field.Type)
	case field.Optional:
		builder.OptField(field.Name, field.Type)
	case field.Readonly:
		builder.ReadonlyField(field.Name, field.Type)
	default:
		builder.Field(field.Name, field.Type)
	}
}
