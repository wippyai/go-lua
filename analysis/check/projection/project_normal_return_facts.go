package projection

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func projectNormalReturnFacts(reg *axis.Registry, result ResultReader, exit state.State) (callboundary.NormalReturnFacts, error) {
	params := parameterValuePaths(result)
	exitFactParams := exitFactParameterValuePaths(result, params)
	exitFactReturns := exitFactReturnValuePaths(result)
	var captured map[symbol.ID]struct{}
	if captureReader, ok := result.(capturedSymbolReader); ok {
		captured = capturedSinkSymbols(captureReader)
	}
	ks := result.KeySpace()
	boundary := newBoundaryPathProjector(ks, exitFactParams, exitFactReturns, captured)
	out, err := projectStateOwnedNormalReturnFacts(reg, result, exit, params, exitFactParams, exitFactReturns, captured)
	if err != nil {
		return callboundary.NormalReturnFacts{}, err
	}
	projectCtx := normalReturnSupplementContext{
		reg:      reg,
		result:   result,
		exit:     exit,
		params:   params,
		ks:       ks,
		boundary: boundary,
	}
	for _, lane := range normalReturnProjectLanes {
		lane.project(projectCtx, &out)
	}
	return out, nil
}

func dynamicIndexKeyPlaceholderPath(result ResultReader, params []path.Path, write factflow.DynamicIndexWrite) (path.Path, bool) {
	if keyPath, ok := write.KeyPath(); ok {
		return parameterPlaceholderPath(keyPath, params)
	}
	return dynamicIndexValueSourcePlaceholderPath(result, params, write.KeySource())
}

func dynamicIndexValueSourcePlaceholderPath(
	result ResultReader,
	params []path.Path,
	source factflow.ValueSource,
) (path.Path, bool) {
	exprPathReader, ok := result.(expressionPathRefReader)
	if !ok {
		return path.Path{}, false
	}
	sourcePath, ok := valueSourcePath(result, exprPathReader, source)
	if !ok {
		return path.Path{}, false
	}
	return normalReturnFactPlaceholderPath(sourcePath.Key(), params)
}

func projectAssignmentDynamicIndexFacts(reg *axis.Registry, result ResultReader, params []path.Path) []callboundary.DynamicIndexFact {
	if reg == nil {
		return nil
	}
	writeReader, ok := result.(dynamicIndexWriteReader)
	if !ok {
		return nil
	}
	valueReader, ok := result.(returnSourceValueReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	var captured map[symbol.ID]struct{}
	var kindReader symbolKindReader
	if captureReader, ok := result.(capturedSymbolReader); ok {
		captured = capturedSinkSymbols(captureReader)
	}
	if reader, ok := result.(symbolKindReader); ok {
		kindReader = reader
	}
	domain := dynamicindex.Domain(reg)
	var out []callboundary.DynamicIndexFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		write, ok := writeReader.DynamicIndexWrite(point)
		if !ok {
			continue
		}
		table, ok := dynamicIndexBoundaryTablePath(write.TablePath(), params, kindReader, captured)
		if !ok {
			continue
		}
		keyValue, ok := valueReader.SourceValueAtBoundary(point, write.KeySource())
		if !ok {
			continue
		}
		value, ok := valueReader.SourceValueAtBoundary(point, write.Source())
		if !ok {
			continue
		}
		factValue := dynamicindex.Fact{
			KeyPresence: product.PresenceOf(keyValue),
			KeyValue:    portableBoundaryValue(reg, keyValue),
			Value:       portableBoundaryValue(reg, value),
			Admission:   write.Admission(),
		}
		if domain.Equal(factValue, dynamicindex.Bottom(reg)) || domain.Equal(factValue, dynamicindex.Top()) {
			continue
		}
		fact := callboundary.DynamicIndexFact{
			Table: table,
			Site:  dynamicindex.SiteForPoint(int(point)),
			Value: factValue,
		}
		if keyPath, ok := dynamicIndexKeyPlaceholderPath(result, params, write); ok {
			fact.KeyPath = keyPath
		}
		if valuePath, ok := dynamicIndexWriteValueBoundaryPath(result, params, write); ok {
			fact.ValuePath = valuePath
		}
		out = append(out, fact)
	}
	return out
}

func dynamicIndexBoundaryTablePath(
	table path.Path,
	params []path.Path,
	kindReader symbolKindReader,
	captured map[symbol.ID]struct{},
) (path.Path, bool) {
	if placeholder, ok := parameterPlaceholderPath(table, params); ok {
		return placeholder, true
	}
	if table.Symbol != 0 && persistentSinkSymbol(kindReader, captured, table.Symbol) {
		return table, true
	}
	return path.Path{}, false
}

func dynamicIndexWriteValueBoundaryPath(
	result ResultReader,
	params []path.Path,
	write factflow.DynamicIndexWrite,
) (path.Path, bool) {
	if valuePath, ok := write.ValuePath(); ok {
		return parameterPlaceholderPath(valuePath, params)
	}
	return dynamicIndexValueSourcePlaceholderPath(result, params, write.Source())
}

func segmentsEqual(a, b []segment.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func projectAssignmentPathInvalidations(result ResultReader, boundary boundaryPathProjector) []callboundary.PathInvalidationFact {
	rootReader, hasRootAssignments := result.(rootAssignmentReader)
	_, hasPathAssignments := result.(pathAssignmentReader)
	_, hasPathInvalidations := result.(pathDescendantInvalidationReader)
	if !hasRootAssignments && !hasPathAssignments && !hasPathInvalidations {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var captured map[symbol.ID]struct{}
	var kindReader symbolKindReader
	if captureReader, ok := result.(capturedSymbolReader); ok {
		captured = capturedSinkSymbols(captureReader)
	}
	if reader, ok := result.(symbolKindReader); ok {
		kindReader = reader
	}
	boundary = boundary.withSymbolKinds(kindReader)
	noNormal, _ := result.(noNormalReturnReader)
	var out []callboundary.PathInvalidationFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		target, preserveStructuralWitness, clearTarget, ok := assignmentInvalidationPathAt(result, rootReader, point, kindReader, captured)
		if !ok {
			continue
		}
		for _, projected := range boundary.ProjectLocalPaths(target) {
			out = append(out, callboundary.PathInvalidationFact{
				Path:                      projected,
				PreserveStructuralWitness: preserveStructuralWitness,
				ClearTarget:               clearTarget,
			})
		}
	}
	return out
}

func projectAssignmentPathStaticMemberDeltas(
	reg *axis.Registry,
	result ResultReader,
	exit state.State,
	params []path.Path,
	boundary boundaryPathProjector,
) []callboundary.PathStaticMemberDeltaFact {
	if reg == nil || len(params) == 0 {
		return nil
	}
	pathReader, hasPathAssignments := result.(pathAssignmentReader)
	valueReader, hasValues := result.(returnSourceValueReader)
	dynamicReader, hasDynamicWrites := result.(dynamicIndexWriteReader)
	if !hasPathAssignments && !hasDynamicWrites {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	unsafeParam := make(map[int]struct{})
	deltas := make(map[path.PathKey]callboundary.PathStaticMemberDeltaFact)
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		if hasDynamicWrites {
			if write, ok := dynamicReader.DynamicIndexWrite(point); ok {
				if placeholder, ok := parameterPlaceholderPath(write.TablePathRef(), params); ok {
					unsafeParam[placeholder.PlaceholderIndex()] = struct{}{}
				}
			}
		}
		if !hasPathAssignments {
			continue
		}
		assignment, ok := pathReader.PathAssignment(point)
		if !ok {
			continue
		}
		placeholder, ok := parameterPlaceholderPath(assignment.TargetPathRef(), params)
		if !ok {
			continue
		}
		param := placeholder.PlaceholderIndex()
		if param < 0 || len(placeholder.Segments) != 1 {
			unsafeParam[param] = struct{}{}
			continue
		}
		value := product.Top()
		if hasValues {
			if sourceValue, ok := valueReader.SourceValueAtBoundary(point, assignment.Source()); ok {
				value = sourceValue
			}
		}
		if typevalue.HasOnlyNilType(reg, value) {
			unsafeParam[param] = struct{}{}
			continue
		}
		key := placeholder.Key()
		if existing, ok := deltas[key]; ok {
			if !pathStaticMemberDeltaSameType(reg, existing.Value, value) {
				unsafeParam[param] = struct{}{}
				continue
			}
			existing.Value = product.Join(reg, existing.Value, value)
			deltas[key] = existing
			continue
		}
		deltas[key] = callboundary.PathStaticMemberDeltaFact{
			Path:  placeholder,
			Value: portableBoundaryValue(reg, value),
		}
	}
	if len(deltas) == 0 {
		return nil
	}
	required := requiredPathStaticMemberPaths(reg, exit, boundary)
	out := make([]callboundary.PathStaticMemberDeltaFact, 0, len(deltas))
	for _, delta := range deltas {
		if _, unsafe := unsafeParam[delta.Path.PlaceholderIndex()]; unsafe {
			continue
		}
		delta.Required = required[delta.Path.Key()]
		out = append(out, delta)
	}
	return out
}

func pathStaticMemberDeltaSameType(reg *axis.Registry, a, b product.Value) bool {
	at, aok := typevalue.TypeOf(reg, a)
	bt, bok := typevalue.TypeOf(reg, b)
	if aok && bok {
		return typ.TypeEquals(at, bt)
	}
	return product.Equal(reg, a, b)
}

func requiredPathStaticMemberPaths(reg *axis.Registry, exit state.State, boundary boundaryPathProjector) map[path.PathKey]bool {
	out := make(map[path.PathKey]bool)
	bottom := product.Bottom(reg)
	exit.ForEachPathStaticMember(boundary.ks, func(pathKey keyspace.Key, value product.Value) bool {
		if product.Equal(reg, value, bottom) {
			return true
		}
		target, ok := boundary.KeyspaceStatePath(pathKey)
		if ok {
			out[target.Key()] = true
		}
		return true
	})
	return out
}

func projectAssignmentPersistentPathWrites(reg *axis.Registry, result ResultReader, exit state.State) []callboundary.PathValueFact {
	rootReader, hasRootAssignments := result.(rootAssignmentReader)
	if reg == nil || !hasRootAssignments {
		return nil
	}
	sourceValueReader, _ := result.(returnSourceValueReader)
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var captured map[symbol.ID]struct{}
	var kindReader symbolKindReader
	if captureReader, ok := result.(capturedSymbolReader); ok {
		captured = capturedSinkSymbols(captureReader)
	}
	if reader, ok := result.(symbolKindReader); ok {
		kindReader = reader
	}
	noNormal, _ := result.(noNormalReturnReader)
	bottom := product.Bottom(reg)
	var out []callboundary.PathValueFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		target, value, ok := assignmentPersistentWriteAt(reg, rootReader, sourceValueReader, exit, point, kindReader, captured)
		if !ok {
			continue
		}
		if product.Equal(reg, value, bottom) {
			continue
		}
		out = append(out, callboundary.PathValueFact{
			Path:  target,
			Value: portableBoundaryValue(reg, value),
		})
	}
	return out
}

func assignmentInvalidationPathAt(
	result ResultReader,
	rootReader rootAssignmentReader,
	point cfg.Point,
	kindReader symbolKindReader,
	captured map[symbol.ID]struct{},
) (path.Path, bool, bool, bool) {
	if assignment, ok := pathAssignmentAt(result, point); ok {
		target := assignment.TargetPath()
		if !target.IsEmpty() && target.Symbol != 0 && len(target.Segments) != 0 {
			return target, true, true, true
		}
	}
	if invalidation, ok := pathDescendantInvalidationAt(result, point); ok {
		target := invalidation.ContainerPath()
		if !target.IsEmpty() {
			return target, true, false, true
		}
	}
	if rootReader == nil {
		return path.Path{}, false, false, false
	}
	assignment, ok := rootReader.RootAssignment(point)
	if !ok || assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
		return path.Path{}, false, false, false
	}
	target := assignment.TargetPath()
	if target.Symbol == 0 || len(target.Segments) != 0 || !persistentSinkSymbol(kindReader, captured, target.Symbol) {
		return path.Path{}, false, false, false
	}
	return path.NewPath(target.Symbol, ""), false, false, true
}

func assignmentPersistentWriteAt(
	reg *axis.Registry,
	rootReader rootAssignmentReader,
	sourceValueReader returnSourceValueReader,
	exit state.State,
	point cfg.Point,
	kindReader symbolKindReader,
	captured map[symbol.ID]struct{},
) (path.Path, product.Value, bool) {
	if rootReader == nil {
		return path.Path{}, product.Value{}, false
	}
	assignment, ok := rootReader.RootAssignment(point)
	if !ok || assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite {
		return path.Path{}, product.Value{}, false
	}
	target := assignment.TargetPath()
	if target.Symbol == 0 || len(target.Segments) != 0 || !persistentSinkSymbol(kindReader, captured, target.Symbol) {
		return path.Path{}, product.Value{}, false
	}
	value := product.Value{}
	if sourceValueReader != nil {
		value, ok = sourceValueReader.SourceValueAtBoundary(point, assignment.Source())
	}
	if !ok {
		value = exit.ReadValue(reg, key.SymbolValue(target.Symbol))
	}
	return path.NewPath(target.Symbol, ""), value, true
}

// projectAssignmentStoreRelations lowers each normal-return-reachable
// param-to-param member store (dst.member = src, where dst is a parameter and src
// resolves to another parameter) into a StoreRelationFact over placeholder paths.
// The callee aliases the source parameter object into the destination parameter's
// member slot, so the caller must eager-widen the source argument toward the
// destination slot's type to keep a later narrow read of the source sound. The
// Source placeholder is the bare source parameter; the Into placeholder carries
// the destination parameter's member segments so the call-boundary lowering can
// project the destination slot type.
func projectAssignmentStoreRelations(result ResultReader, params []path.Path) []callboundary.StoreRelationFact {
	if _, ok := result.(pathAssignmentReader); !ok {
		return nil
	}
	refPathReader, ok := result.(expressionPathRefReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	var out []callboundary.StoreRelationFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		if assignment, ok := pathAssignmentAt(result, point); ok {
			into, ok := parameterPlaceholderPath(assignment.TargetPath(), params)
			if ok && len(into.Segments) != 0 {
				if source, ok := assignmentValueSourceParameterPlaceholder(result, assignment.Source(), refPathReader, params); ok {
					out = append(out, callboundary.StoreRelationFact{Source: source, Into: into})
					continue
				}
			}
		}
	}
	return out
}

func assignmentValueSourceParameterPlaceholder(
	result ResultReader,
	source factflow.ValueSource,
	pathReader expressionPathRefReader,
	params []path.Path,
) (path.Path, bool) {
	sourcePath, ok := valueSourcePath(result, pathReader, source)
	if !ok || sourcePath.Symbol == 0 || len(sourcePath.Segments) != 0 {
		return path.Path{}, false
	}
	placeholder, ok := parameterPlaceholderPath(sourcePath, params)
	if !ok || len(placeholder.Segments) != 0 {
		return path.Path{}, false
	}
	return placeholder, true
}

func projectCallOutcomeLifecycleFacts(result ResultReader, params []path.Path) []callboundary.LifecycleFact {
	if !hasCallSiteView(result) {
		return nil
	}
	outcomeReader, ok := result.(callOutcomeAtReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	edgeReachability, _ := result.(normalEdgeReachabilityReader)
	var out []callboundary.LifecycleFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		site, ok := callSiteViewAt(result, point)
		if !ok {
			continue
		}
		outcome, ok := outcomeReader.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		if !pointOnEveryNormalReturnPath(graph, point, noNormal, edgeReachability) {
			continue
		}
		bindings := callSiteBindings(result, site)
		for _, fact := range outcome.NormalReturnFacts.LifecycleFacts {
			target, ok := fact.Target.Substitute(bindings)
			if !ok {
				continue
			}
			projected, ok := normalReturnLifecycleFactPath(target, params)
			if !ok {
				continue
			}
			fact.Target = projected
			out = append(out, fact)
		}
	}
	return out
}

func projectCallOutcomePersistentPathWrites(result ResultReader, params []path.Path) []callboundary.PathValueFact {
	if !hasCallSiteView(result) {
		return nil
	}
	outcomeReader, ok := result.(callOutcomeAtReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	edgeReachability, _ := result.(normalEdgeReachabilityReader)
	var out []callboundary.PathValueFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		site, ok := callSiteViewAt(result, point)
		if !ok {
			continue
		}
		outcome, ok := outcomeReader.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.PersistentPathWrites) == 0 {
			continue
		}
		if !pointOnEveryNormalReturnPath(graph, point, noNormal, edgeReachability) {
			continue
		}
		bindings := callSiteBindings(result, site)
		for _, fact := range outcome.NormalReturnFacts.PersistentPathWrites {
			target, ok := fact.Path.Substitute(bindings)
			if !ok || target.IsEmpty() || target.IsPlaceholder() || target.Symbol == 0 {
				continue
			}
			fact.Path = target
			out = append(out, fact)
		}
	}
	return out
}

func pointOnEveryNormalReturnPath(
	graph cfg.Graph,
	point cfg.Point,
	noNormal noNormalReturnReader,
	edgeReachability normalEdgeReachabilityReader,
) bool {
	if graph == nil || point == 0 {
		return false
	}
	if point == graph.Entry() {
		return true
	}
	exit := graph.Exit()
	seen := map[cfg.Point]struct{}{graph.Entry(): {}}
	stack := []cfg.Point{graph.Entry()}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == point {
			continue
		}
		if current == exit {
			return false
		}
		if noNormal != nil && noNormal.NoNormalReturn(current) {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, current) {
			if edgeReachability != nil && !edgeReachability.EdgeCanCompleteNormally(current, succ) {
				continue
			}
			if _, ok := seen[succ]; ok {
				continue
			}
			seen[succ] = struct{}{}
			stack = append(stack, succ)
		}
	}
	return true
}

func normalReturnLifecycleFactPath(target path.Path, params []path.Path) (path.Path, bool) {
	if target.IsEmpty() {
		return path.Path{}, false
	}
	if projected, ok := parameterPlaceholderPath(target, params); ok {
		return projected, true
	}
	if target.Symbol != 0 {
		return target, true
	}
	return path.Path{}, false
}

func parameterPlaceholderPath(target path.Path, params []path.Path) (path.Path, bool) {
	if target.IsPlaceholder() {
		index := target.PlaceholderIndex()
		if index < 0 || index >= len(params) || params[index].IsEmpty() {
			return path.Path{}, false
		}
		return target, true
	}
	if target.Symbol == 0 {
		return path.Path{}, false
	}
	for i, param := range params {
		if param.Symbol == 0 || target.Symbol != param.Symbol {
			continue
		}
		return path.NewPlaceholder(i).AppendSegments(target.Segments), true
	}
	return path.Path{}, false
}

func exitFactParameterValuePaths(result ResultReader, params []path.Path) []path.Path {
	if len(params) == 0 {
		return nil
	}
	reassignedReader, ok := result.(reassignedParameterValueSlotReader)
	if !ok {
		return params
	}
	reassigned := reassignedReader.ReassignedParameterValueSlots()
	if len(reassigned) == 0 {
		return params
	}
	out := make([]path.Path, len(params))
	copy(out, params)
	for i, param := range out {
		slot := key.SymbolValue(param.Symbol)
		if slot == 0 {
			out[i] = path.Path{}
			continue
		}
		if _, ok := reassigned[slot]; ok {
			out[i] = path.Path{}
		}
	}
	return out
}

type exitFactReturnPath struct {
	source path.Path
	target path.Path
}

func exitFactReturnValuePaths(result ResultReader) []exitFactReturnPath {
	sourceReader, hasSources := result.(returnValueSourceReader)
	pathReader, hasPaths := result.(expressionPathRefReader)
	if !hasSources || !hasPaths {
		return nil
	}
	var out []exitFactReturnPath
	for _, returnPoint := range result.ReturnPoints() {
		sources, ok := sourceReader.ReturnValueSources(returnPoint)
		if !ok {
			continue
		}
		for returnIndex, source := range sources {
			if returnIndex < 0 {
				continue
			}
			sourcePath, ok := valueSourcePath(result, pathReader, source)
			if !ok || sourcePath.IsEmpty() {
				continue
			}
			target := returnSlotPath(returnIndex)
			if target.IsEmpty() {
				continue
			}
			out = append(out, exitFactReturnPath{source: sourcePath, target: target})
		}
	}
	return out
}

func returnSlotPath(index int) path.Path {
	if index < 0 {
		return path.Path{}
	}
	return path.Path{Root: "ret[" + strconv.Itoa(index) + "]"}
}

func portableBoundaryValue(reg *axis.Registry, value product.Value) product.Value {
	return product.ProjectBoundary(reg, value)
}

func normalReturnFactPlaceholderPath(pathKey path.PathKey, params []path.Path) (path.Path, bool) {
	if pathKey == "" || len(params) == 0 {
		return path.Path{}, false
	}
	if placeholder, ok := pathaddr.PlaceholderPathFromKey(pathKey); ok {
		index := placeholder.PlaceholderIndex()
		if index < 0 || index >= len(params) || params[index].IsEmpty() {
			return path.Path{}, false
		}
		return placeholder, true
	}
	localPath, ok := pathaddr.LocalPathFromKey(pathKey)
	if !ok {
		return path.Path{}, false
	}
	return placeholderForParameterPath(params, localPath)
}

func boundaryPathForReturnPath(returns []exitFactReturnPath, localPath path.Path) (path.Path, bool) {
	for _, candidate := range returns {
		if projected, ok := boundaryPathForOneReturnPath(candidate, localPath); ok {
			return projected, true
		}
	}
	return path.Path{}, false
}

func boundaryPathForOneReturnPath(candidate exitFactReturnPath, localPath path.Path) (path.Path, bool) {
	if localPath.HasPrefix(candidate.source) {
		return candidate.target.AppendSegments(localPath.Segments[len(candidate.source.Segments):]), true
	}
	if localPath.Symbol == 0 || candidate.source.Symbol == 0 ||
		localPath.Symbol != candidate.source.Symbol ||
		len(candidate.source.Segments) > len(localPath.Segments) ||
		!segmentsEqual(localPath.Segments[:len(candidate.source.Segments)], candidate.source.Segments) {
		return path.Path{}, false
	}
	return candidate.target.AppendSegments(localPath.Segments[len(candidate.source.Segments):]), true
}

func placeholderForParameterPath(params []path.Path, localPath path.Path) (path.Path, bool) {
	if localPath.Symbol == 0 {
		return path.Path{}, false
	}
	for i, param := range params {
		if param.Symbol == 0 || param.Symbol != localPath.Symbol {
			continue
		}
		return path.NewPlaceholder(i).AppendSegments(localPath.Segments), true
	}
	return path.Path{}, false
}

func projectBranchProofKind(kind pathevidence.BranchProofKind) (pathevidence.BranchProofKind, bool) {
	switch kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProofPathPresence, true
	case pathevidence.BranchProofPathEqual:
		return pathevidence.BranchProofPathEqual, true
	case pathevidence.BranchProofPathNotEqual:
		return pathevidence.BranchProofPathNotEqual, true
	case pathevidence.BranchProofIndexInRange:
		return pathevidence.BranchProofIndexInRange, true
	default:
		return 0, false
	}
}
