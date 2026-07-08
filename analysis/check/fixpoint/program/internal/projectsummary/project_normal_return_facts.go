package projectsummary

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func projectNormalReturnFacts(reg *axis.Registry, result ResultReader, exit state.State) callboundary.NormalReturnFacts {
	params := parameterValuePaths(result)
	exitFactParams := exitFactParameterValuePaths(result, params)
	exitFactReturns := exitFactReturnValuePaths(result)
	var captured map[symbol.ID]struct{}
	if captureReader, ok := result.(functionCaptureReader); ok {
		captured = capturedSinkSymbols(captureReader)
	}
	ks := result.KeySpace()
	boundary := newBoundaryPathProjector(ks, exitFactParams, exitFactReturns, captured)
	projectCtx := normalReturnProjectContext{
		reg:      reg,
		result:   result,
		exit:     exit,
		params:   params,
		ks:       ks,
		boundary: boundary,
	}
	var out callboundary.NormalReturnFacts
	for _, lane := range normalReturnProjectLanes {
		lane.project(projectCtx, &out)
	}
	return out
}

func dynamicIndexSourcePlaceholderPath(
	result ResultReader,
	params []path.Path,
	site dynamicindex.Site,
	sourceOf func(factflow.DynamicIndexWrite) factflow.ValueSource,
) (path.Path, bool) {
	writeReader, ok := result.(dynamicIndexWriteReader)
	if !ok {
		return path.Path{}, false
	}
	rawPoint, ok := dynamicindex.PointFromSite(site)
	if !ok {
		return path.Path{}, false
	}
	write, ok := writeReader.DynamicIndexWrite(cfg.Point(rawPoint))
	if !ok {
		return path.Path{}, false
	}
	return dynamicIndexValueSourcePlaceholderPath(result, params, sourceOf(write))
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
	if captureReader, ok := result.(functionCaptureReader); ok {
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

func projectAssignmentDynamicValueKeyMemberships(
	result ResultReader,
	params []path.Path,
	boundary boundaryPathProjector,
) []callboundary.DynamicValueKeyMembershipFact {
	writeReader, ok := result.(dynamicIndexWriteReader)
	if !ok {
		return nil
	}
	pathReader, ok := result.(expressionPathRefReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	ks := result.KeySpace()
	if graph == nil || ks == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	var kindReader symbolKindReader
	if reader, ok := result.(symbolKindReader); ok {
		kindReader = reader
	}
	captured := boundary.captured
	stateReader, _ := result.(stateAtReader)
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	var out []callboundary.DynamicValueKeyMembershipFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		write, ok := writeReader.DynamicIndexWrite(point)
		if !ok {
			continue
		}
		container, ok := dynamicIndexBoundaryTablePath(write.TablePath(), params, kindReader, captured)
		if !ok {
			continue
		}
		sourcePath, ok := dynamicIndexWriteValuePath(result, pathReader, write)
		if !ok {
			continue
		}
		out = append(out, dominatingDynamicWriteKeyMemberships(result, params, captured, kindReader, pathReader, dom, point, write, container)...)
		if stateReader == nil {
			continue
		}
		st, ok := stateReader.StateAt(point)
		if !ok {
			continue
		}
		snapshot := st.KeyMembershipsSnapshot()
		if snapshot.Bottom || snapshot.Top {
			continue
		}
		site := dynamicindex.SiteForPoint(int(point))
		for _, membership := range snapshot.Memberships {
			if membership.Kind != state.KeyMembershipPath ||
				!stateKeyMatchesPath(ks, membership.Key, sourcePath) {
				continue
			}
			table, ok := boundary.StatePath(membership.Table)
			if !ok {
				continue
			}
			out = append(out, callboundary.DynamicValueKeyMembershipFact{
				Container: container,
				Site:      site,
				Table:     table,
			})
		}
	}
	return out
}

func dominatingDynamicWriteKeyMemberships(
	result ResultReader,
	params []path.Path,
	captured map[symbol.ID]struct{},
	kindReader symbolKindReader,
	pathReader expressionPathRefReader,
	dom *dominance.ImmediateDominators,
	point cfg.Point,
	write factflow.DynamicIndexWrite,
	container path.Path,
) []callboundary.DynamicValueKeyMembershipFact {
	if dom == nil {
		return nil
	}
	graph := result.Graph()
	writeReader, ok := result.(dynamicIndexWriteReader)
	if graph == nil || !ok {
		return nil
	}
	sourcePath, ok := dynamicIndexWriteValuePath(result, pathReader, write)
	if !ok {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	site := dynamicindex.SiteForPoint(int(point))
	var out []callboundary.DynamicValueKeyMembershipFact
	for _, candidate := range graph.RPO() {
		if candidate == point {
			break
		}
		if !dom.StrictlyDominates(candidate, point) {
			continue
		}
		if noNormal != nil && noNormal.NoNormalReturn(candidate) {
			continue
		}
		prior, ok := writeReader.DynamicIndexWrite(candidate)
		if !ok {
			continue
		}
		keyPath, ok := dynamicIndexWriteKeyPath(result, pathReader, prior)
		if !ok || !keyPath.Equal(sourcePath) {
			continue
		}
		table, ok := dynamicIndexBoundaryTablePath(prior.TablePath(), params, kindReader, captured)
		if !ok {
			continue
		}
		out = append(out, callboundary.DynamicValueKeyMembershipFact{
			Container: container,
			Site:      site,
			Table:     table,
		})
	}
	return out
}

func dynamicIndexWriteKeyPath(result ResultReader, pathReader expressionPathRefReader, write factflow.DynamicIndexWrite) (path.Path, bool) {
	if keyPath, ok := write.KeyPath(); ok && keyPath.Symbol != 0 {
		return keyPath, true
	}
	return dynamicIndexSourcePath(result, pathReader, write.KeySource())
}

func dynamicIndexWriteValuePath(result ResultReader, pathReader expressionPathRefReader, write factflow.DynamicIndexWrite) (path.Path, bool) {
	if valuePath, ok := write.ValuePath(); ok && valuePath.Symbol != 0 {
		return valuePath, true
	}
	return dynamicIndexSourcePath(result, pathReader, write.Source())
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

func dynamicIndexSourcePath(result ResultReader, pathReader expressionPathRefReader, source factflow.ValueSource) (path.Path, bool) {
	sourcePath, ok := valueSourcePath(result, pathReader, source)
	if !ok || sourcePath.Symbol == 0 {
		return path.Path{}, false
	}
	return sourcePath, true
}

func stateKeyMatchesPath(ks *keyspace.KeySpace, stateKey pathaddr.StateKey, target path.Path) bool {
	if ks == nil || stateKey == "" || target.Symbol == 0 {
		return false
	}
	key, ok := ks.InternStateKey(stateKey)
	if !ok || key.Sym != target.Symbol {
		return false
	}
	segments, ok := ks.SegmentsView(key)
	if !ok {
		return false
	}
	return segmentsEqual(segments, target.Segments)
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

func projectAssignmentPathInvalidations(result ResultReader, params []path.Path) []callboundary.PathInvalidationFact {
	rootReader, hasRootAssignments := result.(rootAssignmentReader)
	_, hasPathAssignments := result.(pathAssignmentReader)
	_, hasPathInvalidations := result.(pathDescendantInvalidationReader)
	if !hasOrdinaryAssignmentFactReader(result) && !hasRootAssignments && !hasPathAssignments && !hasPathInvalidations {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var captured map[symbol.ID]struct{}
	var kindReader symbolKindReader
	if captureReader, ok := result.(functionCaptureReader); ok {
		captured = capturedSinkSymbols(captureReader)
	}
	if reader, ok := result.(symbolKindReader); ok {
		kindReader = reader
	}
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
		projected, ok := normalReturnFactInvalidationPath(target, params)
		if !ok {
			continue
		}
		out = append(out, callboundary.PathInvalidationFact{
			Path:                      projected,
			PreserveStructuralWitness: preserveStructuralWitness,
			ClearTarget:               clearTarget,
		})
	}
	return out
}

func projectAssignmentPersistentPathWrites(reg *axis.Registry, result ResultReader, exit state.State) []callboundary.PathValueFact {
	rootReader, hasRootAssignments := result.(rootAssignmentReader)
	if reg == nil || (!hasOrdinaryAssignmentFactReader(result) && !hasRootAssignments) {
		return nil
	}
	valueReader, _ := result.(expressionValueBeforeReader)
	sourceValueReader, _ := result.(returnSourceValueReader)
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	var captured map[symbol.ID]struct{}
	var kindReader symbolKindReader
	if captureReader, ok := result.(functionCaptureReader); ok {
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
		target, value, ok := assignmentPersistentWriteAt(reg, result, rootReader, valueReader, sourceValueReader, exit, point, kindReader, captured)
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
	if fact, ok := ordinaryAssignmentFactAt(result, point); ok {
		return assignmentInvalidationPath(fact, kindReader, captured)
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
	result ResultReader,
	rootReader rootAssignmentReader,
	valueReader expressionValueBeforeReader,
	sourceValueReader returnSourceValueReader,
	exit state.State,
	point cfg.Point,
	kindReader symbolKindReader,
	captured map[symbol.ID]struct{},
) (path.Path, product.Value, bool) {
	if fact, ok := ordinaryAssignmentFactAt(result, point); ok {
		target, ok := assignmentPersistentValuePath(fact, kindReader, captured)
		if !ok {
			return path.Path{}, product.Value{}, false
		}
		value, ok := persistentAssignmentSourceValue(reg, valueReader, point, fact)
		if !ok {
			value = exit.ReadValue(reg, key.SymbolValue(target.Symbol))
		}
		return target, value, true
	}
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

func persistentAssignmentSourceValue(reg *axis.Registry, reader expressionValueBeforeReader, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (product.Value, bool) {
	if reader == nil || fact.Value == nil {
		return product.Value{}, false
	}
	value, ok := reader.ExpressionValueBeforeBoundary(point, fact.Value)
	if !ok {
		return product.Value{}, false
	}
	if kind, ok := valueexpr.RuntimeKind(fact.Value); ok {
		if kind.Contains(runtimekind.Nil) {
			if len(kind.Tags()) == 1 {
				return assignmentValueWithPresence(reg, value, presence.Absent()), true
			}
			return value, true
		}
		return assignmentValueWithPresence(reg, value, presence.Present()), true
	}
	return value, true
}

func assignmentValueWithPresence(reg *axis.Registry, value product.Value, p presence.Value) product.Value {
	out := product.WithPresence(reg, value, p)
	if witness, ok := typevalue.WitnessOf(reg, value); ok {
		out = typevalue.WithWitness(reg, out, typevalue.TypeWithPresence(witness, p))
	}
	return out
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
	_, hasPathAssignments := result.(pathAssignmentReader)
	if !hasOrdinaryAssignmentFactReader(result) && !hasPathAssignments {
		return nil
	}
	pathReader, ok := result.(expressionPathReader)
	refPathReader, hasRefPathReader := result.(expressionPathRefReader)
	if !ok && !hasRefPathReader {
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
				if source, ok := assignmentValueSourceParameterPlaceholder(assignment.Source(), refPathReader, params); ok {
					out = append(out, callboundary.StoreRelationFact{Source: source, Into: into})
					continue
				}
			}
		}
		fact, ok := ordinaryAssignmentFactAt(result, point)
		if !ok || !fact.HasPath || fact.Path.Symbol == 0 || len(fact.Path.Segments) == 0 {
			continue
		}
		into, ok := parameterPlaceholderPath(fact.Path, params)
		if !ok || len(into.Segments) == 0 {
			continue
		}
		source, ok := assignmentSourceParameterPlaceholder(fact, pathReader, params)
		if !ok {
			continue
		}
		out = append(out, callboundary.StoreRelationFact{Source: source, Into: into})
	}
	return out
}

func assignmentValueSourceParameterPlaceholder(
	source factflow.ValueSource,
	pathReader expressionPathRefReader,
	params []path.Path,
) (path.Path, bool) {
	if pathReader == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return path.Path{}, false
	}
	sourcePath, ok := pathReader.ExpressionPathRef(source.ExprRef)
	if !ok || sourcePath.Symbol == 0 || len(sourcePath.Segments) != 0 {
		return path.Path{}, false
	}
	placeholder, ok := parameterPlaceholderPath(sourcePath, params)
	if !ok || len(placeholder.Segments) != 0 {
		return path.Path{}, false
	}
	return placeholder, true
}

// assignmentSourceParameterPlaceholder resolves an ordinary assignment's source
// expression to a bare parameter placeholder ($i with no member segments). A
// member-path source is not a whole-object alias and is not lowered.
func assignmentSourceParameterPlaceholder(
	fact semantics.OrdinaryAssignmentFact,
	pathReader expressionPathReader,
	params []path.Path,
) (path.Path, bool) {
	if fact.Source.Kind != sourceprovenance.SourceExpression || fact.Source.Expr == nil {
		return path.Path{}, false
	}
	sourcePath, ok := pathReader.ExpressionPath(fact.Source.Expr)
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

func assignmentInvalidationPath(
	fact semantics.OrdinaryAssignmentFact,
	kindReader symbolKindReader,
	captured map[symbol.ID]struct{},
) (path.Path, bool, bool, bool) {
	if fact.HasPath && len(fact.Path.Segments) > 0 {
		return fact.Path, true, true, true
	}
	if fact.HasContainerPath && !fact.ContainerPath.IsEmpty() {
		return fact.ContainerPath, true, false, true
	}
	if fact.HasSymbol && fact.Symbol != 0 && persistentSinkSymbol(kindReader, captured, fact.Symbol) {
		return path.NewPath(fact.Symbol, ""), false, false, true
	}
	return path.Path{}, false, false, false
}

func assignmentPersistentValuePath(
	fact semantics.OrdinaryAssignmentFact,
	kindReader symbolKindReader,
	captured map[symbol.ID]struct{},
) (path.Path, bool) {
	if !fact.HasSymbol || fact.Symbol == 0 || !persistentSinkSymbol(kindReader, captured, fact.Symbol) {
		return path.Path{}, false
	}
	return path.NewPath(fact.Symbol, ""), true
}

func normalReturnFactInvalidationPath(target path.Path, params []path.Path) (path.Path, bool) {
	if target.IsEmpty() {
		return path.Path{}, false
	}
	if placeholder, ok := parameterPlaceholderPath(target, params); ok {
		return placeholder, true
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

func frozenTablePlaceholderPaths(reg *axis.Registry, ks *keyspace.KeySpace, exit state.State, params []path.Path) map[identity.ID][]path.Path {
	out := make(map[identity.ID][]path.Path)
	boundary := newBoundaryPathProjector(ks, params, nil, nil)
	var queue []frozenTablePathCandidate
	for i, param := range params {
		if param.Symbol == 0 {
			continue
		}
		value := exit.ReadValue(reg, key.SymbolValue(param.Symbol))
		id, ok := productIdentityID(reg, value)
		if !ok {
			continue
		}
		if addFrozenTablePlaceholderPath(out, id, path.NewPlaceholder(i)) {
			queue = append(queue, newFrozenTablePathCandidate(id, path.NewPlaceholder(i), nil))
		}
	}
	exit.ForEachPathRefinement(func(pathKey keyspace.Key, value product.Value) bool {
		id, ok := productIdentityID(reg, value)
		if !ok {
			return true
		}
		target, ok := boundary.KeyspacePlaceholderPath(pathKey)
		if !ok {
			return true
		}
		if addFrozenTablePlaceholderPath(out, id, target) {
			queue = append(queue, newFrozenTablePathCandidate(id, target, nil))
		}
		return true
	})
	exit.ForEachPathStaticMember(func(pathKey keyspace.Key, value product.Value) bool {
		id, ok := productIdentityID(reg, value)
		if !ok {
			return true
		}
		target, ok := boundary.KeyspacePlaceholderPath(pathKey)
		if !ok {
			return true
		}
		if addFrozenTablePlaceholderPath(out, id, target) {
			queue = append(queue, newFrozenTablePathCandidate(id, target, nil))
		}
		return true
	})
	heap := exit.HeapTableObjectsSnapshot()
	if heap.Top || len(heap.Objects) == 0 {
		return out
	}
	for len(queue) != 0 {
		candidate := queue[0]
		queue = queue[1:]
		object, ok := heap.Objects[candidate.id]
		if !ok {
			continue
		}
		for suffix, value := range object.StaticMembers() {
			childID, ok := productIdentityID(reg, value)
			if !ok || candidate.hasSeen(childID) {
				continue
			}
			segments, ok := ks.SuffixSegmentsView(suffix)
			if !ok {
				continue
			}
			childPath := candidate.path.AppendSegments(segments)
			if addFrozenTablePlaceholderPath(out, childID, childPath) {
				queue = append(queue, newFrozenTablePathCandidate(childID, childPath, candidate.seen))
			}
		}
	}
	return out
}

type frozenTablePathCandidate struct {
	id   identity.ID
	path path.Path
	seen map[identity.ID]struct{}
}

func newFrozenTablePathCandidate(id identity.ID, target path.Path, seen map[identity.ID]struct{}) frozenTablePathCandidate {
	nextSeen := make(map[identity.ID]struct{}, len(seen)+1)
	for seenID := range seen {
		nextSeen[seenID] = struct{}{}
	}
	nextSeen[id] = struct{}{}
	return frozenTablePathCandidate{id: id, path: target, seen: nextSeen}
}

func (c frozenTablePathCandidate) hasSeen(id identity.ID) bool {
	_, ok := c.seen[id]
	return ok
}

func productIdentityID(reg *axis.Registry, value product.Value) (identity.ID, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		return identity.ID{}, false
	}
	return id, true
}

func addFrozenTablePlaceholderPath(paths map[identity.ID][]path.Path, id identity.ID, target path.Path) bool {
	if id == (identity.ID{}) || target.IsEmpty() {
		return false
	}
	key := target.Key()
	for _, existing := range paths[id] {
		if existing.Key() == key {
			return false
		}
	}
	paths[id] = append(paths[id], target)
	return true
}

func portableBoundaryValue(reg *axis.Registry, value product.Value) product.Value {
	return product.Set(reg, value, evidence.Key, evidence.Top())
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
		if !localPath.HasPrefix(candidate.source) {
			continue
		}
		return candidate.target.AppendSegments(localPath.Segments[len(candidate.source.Segments):]), true
	}
	for _, candidate := range returns {
		if localPath.Symbol == 0 ||
			candidate.source.Symbol == 0 ||
			localPath.Symbol != candidate.source.Symbol ||
			len(candidate.source.Segments) > len(localPath.Segments) ||
			!segmentsEqual(localPath.Segments[:len(candidate.source.Segments)], candidate.source.Segments) {
			continue
		}
		return candidate.target.AppendSegments(localPath.Segments[len(candidate.source.Segments):]), true
	}
	return path.Path{}, false
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

func projectChannelSelectKind(kind channelselectfact.Kind) (channelselectfact.Kind, bool) {
	switch kind {
	case channelselectfact.FactSelect:
		return channelselectfact.FactSelect, true
	case channelselectfact.FactReceive:
		return channelselectfact.FactReceive, true
	case channelselectfact.FactCase:
		return channelselectfact.FactCase, true
	default:
		return 0, false
	}
}
