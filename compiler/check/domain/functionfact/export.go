package functionfact

import (
	"strings"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
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

type exportNameSource interface {
	NameOf(cfg.SymbolID) string
}

func projectExportTypeForNames(export typ.Type, rootName string, facts api.FunctionFacts, names exportNameSource) typ.Type {
	if export == nil || names == nil || len(facts) == 0 {
		return export
	}
	fieldTypes := make(map[string]typ.Type)
	for _, sym := range cfg.SortedSymbolIDs(facts) {
		projected := ExportTypeProjection(facts, sym, api.PhaseNarrowing)
		if projected == nil || unwrap.Function(projected) == nil {
			continue
		}
		field, ok := exportFieldName(rootName, names.NameOf(sym))
		if !ok {
			continue
		}
		fieldTypes[field] = projected
	}
	if len(fieldTypes) == 0 {
		return export
	}

	return projectExportType(export, fieldTypes, 0)
}

func exportFieldName(rootName, symbolName string) (string, bool) {
	if symbolName == "" {
		return "", false
	}
	if rootName != "" {
		prefix := rootName + "."
		if !strings.HasPrefix(symbolName, prefix) {
			return "", false
		}
		field := strings.TrimPrefix(symbolName, prefix)
		return field, field != "" && !strings.Contains(field, ".")
	}
	if !strings.Contains(symbolName, ".") {
		return symbolName, true
	}
	parts := strings.Split(symbolName, ".")
	if len(parts) == 2 && parts[1] != "" {
		return parts[1], true
	}
	return "", false
}

func projectExportType(t typ.Type, fieldTypes map[string]typ.Type, depth int) typ.Type {
	if t == nil || len(fieldTypes) == 0 || typ.DepthExceeded(depth) {
		return t
	}
	switch v := t.(type) {
	case *typ.Record:
		return projectRecordExport(v, fieldTypes, depth+1)
	case *typ.Interface:
		return projectInterfaceExport(v, fieldTypes, depth+1)
	case *typ.Function:
		if projected := publicExportFunctionType(v, nil, fieldTypes, depth+1); projected != nil {
			return projected
		}
		return t
	case *typ.Optional:
		inner := projectExportType(v.Inner, fieldTypes, depth+1)
		if typ.TypeEquals(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		for i, member := range v.Members {
			members[i] = projectExportType(member, fieldTypes, depth+1)
			changed = changed || !typ.TypeEquals(members[i], member)
		}
		if !changed {
			return t
		}
		return typ.NewUnion(members...)
	case *typ.Array:
		elem := projectExportType(v.Element, fieldTypes, depth+1)
		if typ.TypeEquals(elem, v.Element) {
			return t
		}
		return typ.NewArray(elem)
	case *typ.Map:
		value := projectExportType(v.Value, fieldTypes, depth+1)
		if typ.TypeEquals(value, v.Value) {
			return t
		}
		return typ.NewMap(v.Key, value)
	case *typ.Tuple:
		elems := make([]typ.Type, len(v.Elements))
		changed := false
		for i, elem := range v.Elements {
			elems[i] = projectExportType(elem, fieldTypes, depth+1)
			changed = changed || !typ.TypeEquals(elems[i], elem)
		}
		if !changed {
			return t
		}
		return typ.NewTuple(elems...)
	default:
		return t
	}
}

func projectRecordExport(rec *typ.Record, fieldTypes map[string]typ.Type, depth int) typ.Type {
	if rec == nil || len(fieldTypes) == 0 {
		return rec
	}
	changed := false
	fields := make([]typ.Field, len(rec.Fields))
	for i, field := range rec.Fields {
		fields[i] = field
		fieldType := projectExportType(field.Type, fieldTypes, depth+1)
		if publicType := publicExportFunctionType(fieldType, fieldTypes[field.Name], fieldTypes, depth+1); publicType != nil {
			fieldType = publicType
		}
		if typ.TypeEquals(field.Type, fieldType) {
			continue
		}
		fields[i].Type = fieldType
		changed = true
	}
	meta := projectExportType(rec.Metatable, fieldTypes, depth+1)
	if !typ.TypeEquals(meta, rec.Metatable) {
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
	if meta != nil {
		builder.Metatable(meta)
	}
	for _, field := range fields {
		addRecordField(builder, field)
	}
	return builder.Build()
}

func projectInterfaceExport(iface *typ.Interface, fieldTypes map[string]typ.Type, depth int) typ.Type {
	if iface == nil || len(fieldTypes) == 0 {
		return iface
	}
	changed := false
	methods := make([]typ.Method, len(iface.Methods))
	for i, method := range iface.Methods {
		methods[i] = method
		methodType := projectExportType(method.Type, fieldTypes, depth+1)
		publicFn := unwrap.Function(publicExportFunctionType(methodType, fieldTypes[method.Name], fieldTypes, depth+1))
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

func publicExportFunctionType(current, public typ.Type, fieldTypes map[string]typ.Type, depth int) typ.Type {
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

func projectFunctionReturns(fn *typ.Function, fieldTypes map[string]typ.Type, depth int, preserve *typ.Function) typ.Type {
	if fn == nil {
		return nil
	}
	builder := typ.Func().ReserveParams(len(fn.Params))
	for _, tp := range fn.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
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
