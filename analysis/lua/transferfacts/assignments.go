package transferfacts

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) localAssignment(point cfg.Point, fact semantics.LocalAssignmentFact) (factflow.RootAssignment, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 {
		return factflow.RootAssignment{}, false
	}
	target := path.NewPath(fact.Symbol, fact.Name)
	source := l.assignmentSource(point, fact.Source)
	if l.declaredValueApplies(fact) {
		if declared, ok := l.declaredValue(fact.Type); ok {
			return factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source, declared), true
		}
	}
	if declared, ok := l.declaredReturnLocalContract(fact); ok {
		return factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source, l.valueFromTypeWithWitness(declared)), true
	}
	if declared, ok := l.returnLocalObjectLiteralContract(fact); ok && recordWithCallableField(declared) {
		return factflow.NewRootAssignmentWithDeclaredContractValue(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source, l.valueFromTypeWithWitness(declared)), true
	}
	if declared, ok := l.localCastContract(fact.Expr); ok {
		return factflow.NewRootAssignmentWithDeclaredOverlayValue(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source, declared), true
	}
	return factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, fact.Symbol, target, source), true
}

// addLocalAliasExposure records a covariant exposure for a local declaration that
// aliases an existing array/record variable (or a sub-object of one) with a
// strictly wider element/field type. The alias and its source name the same
// runtime object, so a mutable alias declared with a wider type lets writes
// through the alias store wider values; the source object's element/field type
// is widened to this contract at the exposure point to keep aliasing sound.
func (l *lowerer) addLocalAliasExposure(input *factflow.FactsInput, point cfg.Point, fact semantics.LocalAssignmentFact) {
	if fact.Type == nil || fact.Source.Kind != sourceprovenance.SourceExpression {
		return
	}
	sourcePath, sourceType, ok := l.aliasSource(fact.Expr)
	if !ok {
		return
	}
	targetType, ok := l.resolveType(fact.Type)
	if !ok || typ.IsAny(targetType) || typ.IsUnknown(targetType) {
		return
	}
	if !aliasStrictlyWidens(sourceType, targetType) {
		return
	}
	l.addCovariantExposure(input, point, sourcePath, fact.Type)
}

// addCovariantExposure appends a covariant-exposure fact for sourcePath toward
// the annotated contract type, carrying the contract's witness-bearing value.
func (l *lowerer) addCovariantExposure(input *factflow.FactsInput, point cfg.Point, sourcePath path.Path, contract ast.TypeExpr) {
	resolved, ok := l.resolveType(contract)
	if !ok {
		return
	}
	l.addCovariantExposureType(input, point, sourcePath, resolved)
}

// addCovariantExposureType appends a covariant-exposure fact for sourcePath
// toward a resolved contract type, when the contract is a mutable container view
// (array or record). A non-container contract is not a covariant exposure and is
// skipped.
func (l *lowerer) addCovariantExposureType(input *factflow.FactsInput, point cfg.Point, sourcePath path.Path, contract typ.Type) {
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return
	}
	kind, ok := covariantExposureKind(contract)
	if !ok {
		return
	}
	wide := l.valueFromTypeWithWitness(contract)
	if input.CovariantExposures == nil {
		input.CovariantExposures = make(map[cfg.Point][]factflow.CovariantExposure)
	}
	input.CovariantExposures[point] = append(input.CovariantExposures[point], factflow.NewCovariantExposure(sourcePath, wide, kind))
}

// covariantExposureKind selects the widening kind for a mutable container
// contract: an opaque-array element widen for an array, a record field rebuild
// for a record. Any other shape is not a mutable container view and emits no
// exposure. The call-boundary twin callresult.covariantExposureKind must
// classify identically; the layered architecture keeps factflow type-independent,
// so the two cannot share one helper.
func covariantExposureKind(contract typ.Type) (factflow.CovariantExposureKind, bool) {
	switch unwrap.Alias(contract).(type) {
	case *typ.Array:
		return factflow.CovariantExposureArray, true
	case *typ.Record:
		return factflow.CovariantExposureRecord, true
	default:
		return 0, false
	}
}

// aliasStrictlyWidens reports whether the target array element or record field(s)
// strictly supertype the source's, which is the covariant alias that needs an
// eager source widen.
func aliasStrictlyWidens(sourceType, targetType typ.Type) bool {
	if sourceElement, ok := arrayElementType(sourceType); ok {
		if targetElement, ok := arrayElementType(targetType); ok {
			return strictlyWidens(sourceElement, targetElement)
		}
		return false
	}
	sourceRecord, ok := unwrap.Alias(sourceType).(*typ.Record)
	if !ok || sourceRecord == nil {
		return false
	}
	targetRecord, ok := unwrap.Alias(targetType).(*typ.Record)
	if !ok || targetRecord == nil {
		return false
	}
	return recordHasStrictlyWiderField(sourceRecord, targetRecord, make(map[[2]typ.Type]bool))
}

// recordHasStrictlyWiderField reports whether any shared field of target
// strictly widens the same-named source field, recursing into nested records.
func recordHasStrictlyWiderField(source, target *typ.Record, visited map[[2]typ.Type]bool) bool {
	key := [2]typ.Type{source, target}
	if visited[key] {
		return false
	}
	visited[key] = true
	for i := range target.Fields {
		tf := target.Fields[i]
		sf, ok := recordField(source, tf.Name)
		if !ok {
			continue
		}
		if sf.Type == nil || tf.Type == nil {
			continue
		}
		if typ.IsAny(sf.Type) || typ.IsUnknown(sf.Type) || typ.IsAny(tf.Type) || typ.IsUnknown(tf.Type) {
			continue
		}
		if strictlyWidens(sf.Type, tf.Type) {
			return true
		}
		if sr, ok := unwrap.Alias(sf.Type).(*typ.Record); ok && sr != nil {
			if tr, ok := unwrap.Alias(tf.Type).(*typ.Record); ok && tr != nil {
				if recordHasStrictlyWiderField(sr, tr, visited) {
					return true
				}
			}
		}
	}
	return false
}

func recordField(r *typ.Record, name string) (typ.Field, bool) {
	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return r.Fields[i], true
		}
	}
	return typ.Field{}, false
}

func strictlyWidens(narrow, wide typ.Type) bool {
	return subtype.IsSubtype(narrow, wide) && !subtype.IsSubtype(wide, narrow)
}

// exposureContractElement strips a container-slot contract's outer optionality
// (array element / optional-field presence) to the element record when the
// stored object cannot itself be nil. The widen rebuilds the stored object's
// record structure, so the element record is the contract to widen toward.
func exposureContractElement(sourceType, contract typ.Type) typ.Type {
	if _, ok := unwrap.Alias(sourceType).(*typ.Record); !ok {
		return contract
	}
	inner := unwrap.Optional(contract)
	if inner == nil {
		return contract
	}
	if _, ok := unwrap.Alias(inner).(*typ.Record); ok {
		return inner
	}
	return contract
}

// aliasSource resolves the source path and declared type of an alias source: a
// bare identifier (narrow) or a member access of one (narrow.inner). A sub-path
// source carries the symbol's structural field type projected through the
// segments, so the exposure can repair the ancestor symbol's witness.
func (l *lowerer) aliasSource(expr ast.Expr) (path.Path, typ.Type, bool) {
	sourcePath, ok := pathexpr.ResolveAlias(expr, l.bindings)
	if !ok || sourcePath.Symbol == 0 {
		return path.Path{}, nil, false
	}
	sourceType, ok := l.aliasPathType(sourcePath)
	if !ok {
		return path.Path{}, nil, false
	}
	return sourcePath, sourceType, true
}

func arrayElementType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	array, ok := unwrap.Alias(t).(*typ.Array)
	if !ok || array == nil || array.Element == nil {
		return nil, false
	}
	return array.Element, true
}

func (l *lowerer) declaredValueApplies(fact semantics.LocalAssignmentFact) bool {
	if fact.Type == nil || fact.Source.Kind != sourceprovenance.SourceExpression {
		return false
	}
	t, ok := l.resolveType(fact.Type)
	if !ok {
		return false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	if literal, ok := valueexpr.LiteralType(fact.Expr); ok {
		if typ.TypeEquals(literal, typ.Nil) {
			return false
		}
		return true
	}
	if !tableConstructorExpr(fact.Expr) {
		return false
	}
	// A table constructor declared as a method-bearing record (the class/builder
	// instance pattern) carries the declared contract value. Such literals wire
	// method fields to callables defined later in the module or across modules,
	// which resolve to nothing at construction and drop out of the inferred
	// record; the declared annotation is the checked contract for the local.
	if recordWithCallableField(t) {
		return true
	}
	// A table constructor declared as an array carries the declared array
	// contract value. The literal is still verified element-wise against the
	// annotation at the assignment site, while dynamic-index facts keep precise
	// evidence for later writes such as table.insert.
	return reachesArray(t)
}

func reachesArray(t typ.Type) bool {
	return reachesArrayDepth(t, 0)
}

func reachesArrayDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Array:
		return true
	case *typ.Alias:
		return reachesArrayDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return reachesArrayDepth(v.Body, depth+1)
	default:
		return false
	}
}

func tableConstructorExpr(expr ast.Expr) bool {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return false
	}
	_, ok = inner.(*ast.TableExpr)
	return ok
}

func emptyTableConstructorExpr(expr ast.Expr) bool {
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return false
	}
	table, ok := inner.(*ast.TableExpr)
	return ok && len(table.Fields) == 0
}

func (l *lowerer) declaredReturnLocalContract(fact semantics.LocalAssignmentFact) (typ.Type, bool) {
	if !returnLocalInitializerCandidate(fact) {
		return nil, false
	}
	t, ok := l.declaredReturnLocalTypes[fact.Symbol]
	if !ok || !declaredReturnLocalContractType(t) {
		return nil, false
	}
	return t, true
}

func (l *lowerer) returnLocalObjectLiteralContract(fact semantics.LocalAssignmentFact) (typ.Type, bool) {
	if !returnLocalInitializerCandidate(fact) {
		return nil, false
	}
	t, ok := l.returnLocalObjectLiteralTypes[fact.Symbol]
	if !ok || !declaredReturnLocalContractType(t) {
		return nil, false
	}
	return t, true
}

func recordWithCallableField(t typ.Type) bool {
	return recordWithCallableFieldDepth(t, 0)
}

func recordWithCallableFieldDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return recordWithCallableFieldDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return recordWithCallableFieldDepth(v.Inner, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return recordWithCallableFieldDepth(v.Body, depth+1)
	case *typ.Record:
		for i := range v.Fields {
			if _, ok := typecall.Callable(v.Fields[i].Type); ok {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (l *lowerer) ordinaryAssignment(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (factflow.RootAssignment, bool) {
	source := l.valueSource(fact.Source)
	if !fact.HasSymbol || fact.Symbol == 0 {
		if targetSymbol, targetPath, ok := l.globalTableFieldRootTarget(fact); ok {
			return factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, targetSymbol, targetPath, source), true
		}
		return factflow.RootAssignment{}, false
	}
	target := fact.Path
	if !fact.HasPath {
		target = path.NewPath(fact.Symbol, "")
	}
	if len(target.Segments) != 0 {
		return factflow.RootAssignment{}, false
	}
	return factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, fact.Symbol, target, source), true
}

func (l *lowerer) globalTableFieldRootTarget(fact semantics.OrdinaryAssignmentFact) (symbol.ID, path.Path, bool) {
	if l.bindings == nil || !fact.HasPath || fact.Path.Symbol == 0 {
		return 0, path.Path{}, false
	}
	if l.bindings.Name(fact.Path.Symbol) != "_G" {
		return 0, path.Path{}, false
	}
	kind, ok := l.bindings.Kind(fact.Path.Symbol)
	if !ok || kind != symbol.Global {
		return 0, path.Path{}, false
	}
	name, ok := fact.Path.DirectFieldName()
	if !ok {
		return 0, path.Path{}, false
	}
	target, ok := l.bindings.GlobalSymbol(name)
	if !ok || target == 0 {
		return 0, path.Path{}, false
	}
	return target, path.NewPath(target, name), true
}

// addReassignExposure records a covariant exposure for an ordinary root
// reassignment (wide = narrow) where the target's declared type strictly widens
// the source object. The two name the same runtime object after the write, so a
// later write through the wider target view can launder a wide value back; the
// source object is widened to the target's declared contract.
func (l *lowerer) addReassignExposure(input *factflow.FactsInput, point cfg.Point, fact semantics.OrdinaryAssignmentFact) {
	if !fact.HasSymbol || fact.Symbol == 0 || (fact.HasPath && len(fact.Path.Segments) != 0) {
		return
	}
	targetType, ok := l.symbolTypes[fact.Symbol]
	if !ok {
		return
	}
	l.addAliasExposureToContractType(input, point, fact.Source, targetType)
}

// addStoreExposure records a covariant exposure for a field or index store into a
// declared container slot (holder.ref = narrow, sink[1] = narrow) whose declared
// slot type strictly widens the stored object. The container retains the object
// at the slot, so a later write through the wider slot view can launder a wide
// value back into the source object; the source object is widened to the slot's
// declared contract.
func (l *lowerer) addStoreExposure(input *factflow.FactsInput, point cfg.Point, fact semantics.OrdinaryAssignmentFact) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return
	}
	containerType, ok := l.symbolTypes[fact.Path.Symbol]
	if !ok {
		return
	}
	slotType, ok := luatypeprojection.ApplySegments(containerType, fact.Path.Segments)
	if !ok || slotType == nil {
		return
	}
	l.addAliasExposureToContractType(input, point, fact.Source, slotType)
}

// addAliasExposureToContractType emits a covariant exposure for an alias source
// (a bare identifier or member access) whose declared object type is strictly
// widened by the contract type. It is the shared body of the reassignment and
// container-store exposure sites.
func (l *lowerer) addAliasExposureToContractType(input *factflow.FactsInput, point cfg.Point, source sourceprovenance.ASTSource, contract typ.Type) {
	if source.Kind != sourceprovenance.SourceExpression {
		return
	}
	sourcePath, sourceType, ok := l.aliasSource(source.Expr)
	if !ok {
		return
	}
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) {
		return
	}
	// A container-slot contract (array element, optional field) may wrap the element
	// record in an Optional reflecting slot presence, not element covariance. The
	// exposure widens the stored object's structure, so the element record is the
	// contract; strip the slot's outer optionality before comparing and widening.
	contract = exposureContractElement(sourceType, contract)
	if !aliasStrictlyWidens(sourceType, contract) {
		return
	}
	l.addCovariantExposureType(input, point, sourcePath, contract)
}

func (l *lowerer) pathAssignment(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (factflow.PathAssignment, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return factflow.PathAssignment{}, false
	}
	return factflow.NewPathAssignment(fact.Path, l.assignmentSource(point, fact.Source)), true
}

func (l *lowerer) pathStaticMemberWrite(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (factflow.PathStaticMemberWrite, bool) {
	if !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
		return factflow.PathStaticMemberWrite{}, false
	}
	return factflow.NewPathStaticMemberWrite(fact.Path, l.assignmentSource(point, fact.Source)), true
}

func (l *lowerer) dynamicIndexWrite(point cfg.Point, fact semantics.OrdinaryAssignmentFact) (factflow.DynamicIndexWrite, bool) {
	if fact.HasPath {
		return factflow.DynamicIndexWrite{}, false
	}
	tablePath, ok := l.directDynamicIndexWriteTablePath(fact.Target)
	if !ok || tablePath.Symbol == 0 {
		return factflow.DynamicIndexWrite{}, false
	}
	keySource, readKey := l.dynamicIndexKeySource(point, fact.Target)
	source := l.assignmentSource(point, fact.Source)
	readValue := fact.Source.Kind != sourceprovenance.SourceUnknown
	write := factflow.NewDynamicIndexWrite(
		tablePath,
		keySource,
		source,
		dynamicindex.AdmissionUnknown,
		dynamicIndexReadbackIntent(readKey, readValue),
	)
	if keyPath, ok := l.dynamicIndexKeyPath(fact.Target); ok {
		write = write.WithKeyPath(keyPath)
	}
	if valuePath, ok := l.dynamicIndexValuePath(fact.Source); ok {
		write = write.WithValuePath(valuePath)
	}
	return write, true
}

func (l *lowerer) directDynamicIndexWriteTablePath(target ast.Expr) (path.Path, bool) {
	attr, ok := target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return path.Path{}, false
	}
	switch attr.Key.(type) {
	case *ast.StringExpr, *ast.NumberExpr:
		return path.Path{}, false
	}
	return pathexpr.Resolve(attr.Object, l.bindings)
}

func (l *lowerer) pathDescendantInvalidation(fact semantics.OrdinaryAssignmentFact) (factflow.PathDescendantInvalidation, bool) {
	if fact.HasPath || !fact.HasContainerPath || fact.ContainerPath.Symbol == 0 {
		return factflow.PathDescendantInvalidation{}, false
	}
	out := factflow.NewPathDescendantInvalidation(fact.ContainerPath)
	if tablePath, keySource, suffix, ok := l.dynamicInvalidationTarget(fact.Target); ok {
		out = out.WithDynamicTarget(tablePath, keySource, suffix)
	}
	return out, true
}

func (l *lowerer) dynamicInvalidationTarget(target ast.Expr) (path.Path, factflow.ValueSource, []segment.Segment, bool) {
	var suffix []segment.Segment
	for {
		attr, ok := target.(*ast.AttrGetExpr)
		if !ok {
			return path.Path{}, factflow.ValueSource{}, nil, false
		}
		if attr.KeySyntax == ast.AttrKeyIndex {
			switch attr.Key.(type) {
			case *ast.StringExpr, *ast.NumberExpr:
			default:
				tablePath, ok := pathexpr.Resolve(attr.Object, l.bindings)
				if !ok || tablePath.Symbol == 0 {
					return path.Path{}, factflow.ValueSource{}, nil, false
				}
				keySource, ok := l.dynamicIndexKeySourceFromAST(attr)
				if !ok {
					return path.Path{}, factflow.ValueSource{}, nil, false
				}
				return tablePath, keySource, suffix, true
			}
		}
		seg, ok := staticAttrSegment(attr)
		if !ok {
			return path.Path{}, factflow.ValueSource{}, nil, false
		}
		suffix = append([]segment.Segment{seg}, suffix...)
		target = attr.Object
	}
}

func staticAttrSegment(attr *ast.AttrGetExpr) (segment.Segment, bool) {
	if attr == nil || attr.Key == nil {
		return segment.Segment{}, false
	}
	switch key := attr.Key.(type) {
	case *ast.StringExpr:
		switch attr.KeySyntax {
		case ast.AttrKeyDot:
			if key.Value == "" {
				return segment.Segment{}, false
			}
			return segment.Segment{Kind: segment.SegmentField, Name: key.Value}, true
		case ast.AttrKeyIndex:
			return segment.Segment{Kind: segment.SegmentIndexString, Name: key.Value}, true
		default:
			return segment.Segment{Kind: segment.SegmentField, Name: key.Value}, key.Value != ""
		}
	case *ast.NumberExpr:
		index, ok := parseStaticIndex(key.Value)
		if !ok {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentIndexInt, Index: index}, true
	default:
		return segment.Segment{}, false
	}
}

func parseStaticIndex(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	index, err := strconv.Atoi(raw)
	return index, err == nil && index >= 0
}

func (l *lowerer) dynamicIndexKeySource(point cfg.Point, target ast.Expr) (factflow.ValueSource, bool) {
	if source, ok := l.dynamicIndexKeySourceFromWIR(point); ok {
		return source, true
	}
	return l.dynamicIndexKeySourceFromAST(target)
}

func (l *lowerer) dynamicIndexKeySourceFromWIR(point cfg.Point) (factflow.ValueSource, bool) {
	if l == nil || l.wir == nil {
		return factflow.ValueSource{}, false
	}
	for _, inst := range l.wir.PointInstructions(point) {
		if inst.Op != wir.OpDynamicIndexWrite || inst.A.Kind == wir.OperandNone {
			continue
		}
		if source, ok := l.rootPathExpressionSourceFromWIR(
			"dynamic-index-key",
			point,
			inst.A,
			sourceprovenance.NoSourceIndex,
			sourceprovenance.NoSourceIndex,
			true,
			false,
			false,
			symbol.Local,
			symbol.Param,
		); ok {
			return source, true
		}
		return l.valueSourceFromWIROperand(
			inst.A,
			sourceprovenance.NoSourceIndex,
			sourceprovenance.NoSourceIndex,
			true,
			false,
			false,
			l.callResultValueSourcesByTempFromWIR(),
		)
	}
	return factflow.ValueSource{}, false
}

func (l *lowerer) dynamicIndexKeySourceFromAST(target ast.Expr) (factflow.ValueSource, bool) {
	attr, ok := target.(*ast.AttrGetExpr)
	if !ok || attr.Key == nil || sourceprovenance.CanProduceMultipleValues(attr.Key) {
		return factflow.NewUnknownValueSource(factflow.NoValueSourceIndex), false
	}
	shape, ok := sourceprovenance.NewSourceShape(true, false, false, false)
	if !ok {
		panic("transferfacts: invalid dynamic index key source shape")
	}
	source, ok := sourceprovenance.NewExpressionSource(
		attr.Key,
		sourceprovenance.NoSourceIndex,
		sourceprovenance.NoSourceIndex,
		0,
		shape,
	)
	if !ok {
		return factflow.NewUnknownValueSource(factflow.NoValueSourceIndex), false
	}
	return l.valueSource(source), true
}

func (l *lowerer) dynamicIndexKeyPath(target ast.Expr) (path.Path, bool) {
	attr, ok := target.(*ast.AttrGetExpr)
	if !ok || attr.Key == nil {
		return path.Path{}, false
	}
	return pathexpr.ResolveAlias(attr.Key, l.bindings)
}

func (l *lowerer) dynamicIndexValuePath(source sourceprovenance.ASTSource) (path.Path, bool) {
	if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
		return path.Path{}, false
	}
	return pathexpr.ResolveAlias(source.Expr, l.bindings)
}

func dynamicIndexReadbackIntent(readKey bool, readValue bool) factflow.DynamicIndexReadbackIntent {
	switch {
	case readKey && readValue:
		return factflow.DynamicIndexReadbackKeyAndValue
	case readKey:
		return factflow.DynamicIndexReadbackKey
	case readValue:
		return factflow.DynamicIndexReadbackValue
	default:
		return factflow.DynamicIndexReadbackNone
	}
}

func (l *lowerer) declaredValue(expr ast.TypeExpr) (product.Value, bool) {
	if expr == nil {
		return product.Value{}, false
	}
	t, ok := l.resolveType(expr)
	if !ok {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(t), true
}

func (l *lowerer) localCastContract(expr ast.Expr) (product.Value, bool) {
	cast, ok := expr.(*ast.CastExpr)
	if !ok || cast.Type == nil {
		return product.Value{}, false
	}
	t, ok := l.resolveType(cast.Type)
	if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(t), true
}
