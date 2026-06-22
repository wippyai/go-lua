package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

func projectNormalReturnFacts(reg *axis.Registry, result ResultReader, exit state.State) callboundary.NormalReturnFacts {
	params := parameterValuePaths(result)
	exitFactParams := exitFactParameterValuePaths(result, params)
	projectPath := func(pathKey path.PathKey) (path.Path, bool) {
		return normalReturnFactPlaceholderPath(pathKey, exitFactParams)
	}
	projectStatePath := func(pathKey path.PathKey) (path.Path, bool) {
		return normalReturnFactStatePlaceholderPath(result.KeySpace(), pathKey, exitFactParams)
	}
	out := callboundary.NormalReturnFacts{}
	out.PathInvalidations = append(out.PathInvalidations, projectAssignmentPathInvalidations(result, params)...)
	out.DynamicIndexFacts = append(out.DynamicIndexFacts, projectAssignmentDynamicIndexFacts(reg, result, params)...)
	out.LifecycleFacts = append(out.LifecycleFacts, projectCallOutcomeLifecycleFacts(result, params)...)
	out.StoreRelations = append(out.StoreRelations, projectAssignmentStoreRelations(result, params)...)

	ks := result.KeySpace()
	if snapshot := exit.PathRefinementsSnapshot(ks); !snapshot.Top {
		bottom := product.Bottom(reg)
		top := product.Top()
		for pathKey, value := range snapshot.Refinements {
			if product.Equal(reg, value, bottom) || product.Equal(reg, value, top) {
				continue
			}
			target, ok := projectPath(pathKey)
			if !ok {
				continue
			}
			out.PathRefinements = append(out.PathRefinements, callboundary.PathValueFact{
				Path:  target,
				Value: portableBoundaryValue(reg, value),
			})
		}
	}

	if snapshot := exit.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		bottom := product.Bottom(reg)
		for pathKey, value := range snapshot.Members {
			if product.Equal(reg, value, bottom) {
				continue
			}
			target, ok := projectPath(pathKey)
			if !ok {
				continue
			}
			out.PathStaticMembers = append(out.PathStaticMembers, callboundary.PathStaticMemberFact{
				Path:  target,
				Value: value,
			})
		}
	}

	if snapshot := exit.DynamicIndexFactsSnapshot(); !snapshot.Top {
		for stateKey, stateFact := range snapshot.Facts {
			table, ok := projectPath(ks.Format(stateKey.Table))
			if !ok {
				continue
			}
			domain := dynamicindex.Domain(reg)
			if domain.Equal(stateFact, dynamicindex.Bottom(reg)) {
				continue
			}
			out.PathInvalidations = append(out.PathInvalidations, callboundary.PathInvalidationFact{
				Path: table,
			})
			if domain.Equal(stateFact, dynamicindex.Top()) {
				continue
			}
			fact := callboundary.DynamicIndexFact{
				Table: table,
				Site:  stateKey.Site,
				Value: stateFact,
			}
			if keyPath, ok := dynamicIndexSourcePlaceholderPath(result, params, stateKey.Site, func(write factflow.DynamicIndexWrite) factflow.ValueSource {
				return write.KeySource()
			}); ok {
				fact.KeyPath = keyPath
			}
			if valuePath, ok := dynamicIndexSourcePlaceholderPath(result, params, stateKey.Site, func(write factflow.DynamicIndexWrite) factflow.ValueSource {
				return write.Source()
			}); ok {
				fact.ValuePath = valuePath
			}
			out.DynamicIndexFacts = append(out.DynamicIndexFacts, fact)
		}
	}

	if snapshot := exit.BranchProofsSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for _, stateProof := range snapshot.Proofs {
			target, ok := projectPath(ks.Format(stateProof.Path))
			if !ok {
				continue
			}
			kind, ok := projectBranchProofKind(stateProof.Kind)
			if !ok {
				continue
			}
			proof := callboundary.BranchProof{
				Kind: kind,
				Path: target,
			}
			switch kind {
			case pathevidence.BranchProofPathPresence:
				if stateProof.Presence.IsBottom() || stateProof.Presence.IsTop() {
					continue
				}
				proof.Presence = stateProof.Presence
			case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual, pathevidence.BranchProofIndexInRange:
				other, ok := projectPath(ks.Format(stateProof.Other))
				if !ok {
					continue
				}
				proof.Other = other
			default:
				continue
			}
			out.BranchProofs = append(out.BranchProofs, proof)
		}
	}

	if snapshot := exit.NumFloorsSnapshot(ks); !snapshot.Bottom {
		for pathKey, floor := range snapshot.Floors {
			target, ok := projectStatePath(pathKey)
			if !ok {
				continue
			}
			out.NumFloors = append(out.NumFloors, callboundary.NumFloorFact{
				Path:  target,
				Floor: floor,
			})
		}
	}

	if snapshot := exit.RelConstraints(); !snapshot.Bottom && !snapshot.Top {
		for _, constraint := range snapshot.Constraints {
			projected, ok := projectRelConstraintFact(projectStatePath, constraint)
			if !ok {
				continue
			}
			out.RelConstraints = append(out.RelConstraints, projected)
		}
	}

	if snapshot := exit.ChannelSelectFactsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, stateFact := range snapshot.Facts {
			kind, ok := projectChannelSelectKind(stateFact.Kind)
			if !ok {
				continue
			}
			fact := callboundary.ChannelSelectFact{
				Select:     channelselectfact.ID(stateFact.Select),
				Kind:       kind,
				Index:      stateFact.Index,
				HasDefault: stateFact.HasDefault,
			}
			if stateFact.Result != "" {
				resultPath, ok := projectPath(stateFact.Result.PathKey())
				if !ok {
					continue
				}
				fact.Result = resultPath
			}
			if stateFact.Case != "" {
				casePath, ok := projectPath(stateFact.Case.PathKey())
				if !ok {
					continue
				}
				fact.Case = casePath
			}
			out.ChannelSelects = append(out.ChannelSelects, fact)
		}
	}

	if snapshot := exit.FrozenTablesSnapshot(); !snapshot.Bottom && !snapshot.Top {
		frozenPaths := frozenTablePlaceholderPaths(reg, ks, exit, exitFactParams)
		for _, id := range snapshot.Tables {
			for _, target := range frozenPaths[id] {
				out.FrozenTables = append(out.FrozenTables, callboundary.FrozenTableFact{
					Target: target,
				})
			}
		}
	}

	if snapshot := exit.EffectDeltasSnapshot(); !snapshot.Top {
		for stateKey, stateDelta := range snapshot.Deltas {
			target, ok := projectPath(ks.Format(stateKey.Target))
			if !ok {
				continue
			}
			if stateKey.Kind == effectdelta.Freeze && callboundary.IsFrozenTableEffectSite(stateKey.Site) {
				out.FrozenTables = append(out.FrozenTables, callboundary.FrozenTableFact{
					Target: target,
				})
				continue
			}
			if stateKey.Kind == effectdelta.Mutation && callboundary.IsPathInvalidationEffectSite(stateKey.Site) {
				out.PathInvalidations = append(out.PathInvalidations, callboundary.PathInvalidationFact{
					Path: target,
				})
				continue
			}
			delta := callboundary.EffectDelta{
				Target: target,
				Site:   stateKey.Site,
				Kind:   stateKey.Kind,
				Value:  stateDelta,
			}
			domain := effectdelta.Domain(reg)
			if domain.Equal(delta.Value, domain.Bottom()) || domain.Equal(delta.Value, effectdelta.Top()) {
				continue
			}
			out.EffectDeltas = append(out.EffectDeltas, delta)
		}
	}

	if snapshot := exit.EscapeEventsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, event := range snapshot.Facts {
			target, ok := projectPath(event.Target.PathKey())
			if !ok {
				continue
			}
			out.EscapeEvents = append(out.EscapeEvents, callboundary.EscapeEventFact{
				Target:    target,
				Kind:      event.Kind,
				Recursive: event.Recursive,
			})
		}
	}

	if snapshot := exit.StoreRelationsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, relation := range snapshot.Relations {
			source, ok := projectPath(relation.Source.PathKey())
			if !ok {
				continue
			}
			into, ok := projectPath(relation.Into.PathKey())
			if !ok {
				continue
			}
			out.StoreRelations = append(out.StoreRelations, callboundary.StoreRelationFact{
				Source: source,
				Into:   into,
			})
		}
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
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return path.Path{}, false
	}
	sourcePath, ok := exprPathReader.ExpressionPathRef(source.ExprRef)
	if !ok {
		return path.Path{}, false
	}
	return normalReturnFactPlaceholderPath(sourcePath.Key(), params)
}

func projectAssignmentDynamicIndexFacts(reg *axis.Registry, result ResultReader, params []path.Path) []callboundary.DynamicIndexFact {
	if reg == nil || len(params) == 0 {
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
		table, ok := parameterPlaceholderPath(write.TablePath(), params)
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
		if valuePath, ok := dynamicIndexValueSourcePlaceholderPath(result, params, write.Source()); ok {
			fact.ValuePath = valuePath
		}
		out = append(out, fact)
	}
	return out
}

func projectAssignmentPathInvalidations(result ResultReader, params []path.Path) []callboundary.PathInvalidationFact {
	reader, ok := result.(ordinaryAssignmentReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	noNormal, _ := result.(noNormalReturnReader)
	var out []callboundary.PathInvalidationFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		fact, ok := reader.OrdinaryAssignment(point)
		if !ok {
			continue
		}
		target, ok := assignmentInvalidationPath(fact)
		if !ok {
			continue
		}
		projected, ok := normalReturnFactInvalidationPath(target, params)
		if !ok {
			continue
		}
		out = append(out, callboundary.PathInvalidationFact{Path: projected})
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
	reader, ok := result.(ordinaryAssignmentReader)
	if !ok {
		return nil
	}
	pathReader, ok := result.(expressionPathReader)
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
		fact, ok := reader.OrdinaryAssignment(point)
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
	callResult, ok := result.(callReader)
	if !ok {
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
	var out []callboundary.LifecycleFact
	for _, point := range graph.RPO() {
		if noNormal != nil && noNormal.NoNormalReturn(point) {
			continue
		}
		site, ok := callResult.CallSite(point)
		if !ok {
			continue
		}
		outcome, ok := outcomeReader.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.LifecycleFacts) == 0 {
			continue
		}
		if !pointOnEveryNormalReturnPath(graph, point, noNormal) {
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

func pointOnEveryNormalReturnPath(graph cfg.Graph, point cfg.Point, noNormal noNormalReturnReader) bool {
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
		for _, succ := range graph.Successors(current) {
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

func assignmentInvalidationPath(fact semantics.OrdinaryAssignmentFact) (path.Path, bool) {
	if fact.HasPath && len(fact.Path.Segments) > 0 {
		return fact.Path, true
	}
	if fact.HasContainerPath && !fact.ContainerPath.IsEmpty() {
		return fact.ContainerPath, true
	}
	return path.Path{}, false
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

func frozenTablePlaceholderPaths(reg *axis.Registry, ks *keyspace.KeySpace, exit state.State, params []path.Path) map[identity.ID][]path.Path {
	out := make(map[identity.ID][]path.Path)
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
	if snapshot := exit.PathRefinementsSnapshot(ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			id, ok := productIdentityID(reg, value)
			if !ok {
				continue
			}
			target, ok := normalReturnFactPlaceholderPath(pathKey, params)
			if !ok {
				continue
			}
			if addFrozenTablePlaceholderPath(out, id, target) {
				queue = append(queue, newFrozenTablePathCandidate(id, target, nil))
			}
		}
	}
	if snapshot := exit.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			id, ok := productIdentityID(reg, value)
			if !ok {
				continue
			}
			target, ok := normalReturnFactPlaceholderPath(pathKey, params)
			if !ok {
				continue
			}
			if addFrozenTablePlaceholderPath(out, id, target) {
				queue = append(queue, newFrozenTablePathCandidate(id, target, nil))
			}
		}
	}
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
			segments, ok := ks.SuffixSegments(suffix)
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

func projectRelConstraintFact(
	projectStatePath func(path.PathKey) (path.Path, bool),
	constraint state.RelConstraint,
) (callboundary.RelConstraintFact, bool) {
	a, ok := projectRelConstraintOperand(projectStatePath, constraint.A)
	if !ok {
		return callboundary.RelConstraintFact{}, false
	}
	c, ok := projectRelConstraintOperand(projectStatePath, constraint.C)
	if !ok {
		return callboundary.RelConstraintFact{}, false
	}
	out := callboundary.RelConstraintFact{
		CoA: constraint.CoA,
		A:   a,
		C:   c,
		K:   constraint.K,
	}
	if constraint.B.IsValid() && constraint.CoB != 0 {
		b, ok := projectRelConstraintOperand(projectStatePath, constraint.B)
		if !ok {
			return callboundary.RelConstraintFact{}, false
		}
		out.CoB = constraint.CoB
		out.B = b
	}
	return out, true
}

func projectRelConstraintOperand(
	projectStatePath func(path.PathKey) (path.Path, bool),
	operand state.RelOperand,
) (callboundary.RelOperand, bool) {
	target, ok := projectStatePath(operand.StateKey().PathKey())
	if !ok {
		return callboundary.RelOperand{}, false
	}
	return callboundary.RelOperand{Path: target, IsLength: operand.IsLength()}, true
}

func normalReturnFactStatePlaceholderPath(ks *keyspace.KeySpace, pathKey path.PathKey, params []path.Path) (path.Path, bool) {
	if pathKey == "" || ks == nil || len(params) == 0 {
		return path.Path{}, false
	}
	if placeholder, ok := pathaddr.PlaceholderPathFromKey(pathKey); ok {
		index := placeholder.PlaceholderIndex()
		if index < 0 || index >= len(params) || params[index].IsEmpty() {
			return path.Path{}, false
		}
		return placeholder, true
	}
	k, ok := ks.FromStateKey(pathKey)
	if !ok {
		return path.Path{}, false
	}
	switch k.Kind {
	case keyspace.KindUnversionedSym:
		if k.Segs != 0 {
			return path.Path{}, false
		}
		return placeholderForParameterPath(params, path.NewPath(k.Sym, ""))
	case keyspace.KindResolverSym:
		return placeholderForParameterPath(params, path.NewPath(k.Sym, "").AppendSegments(ks.Segments(k)))
	case keyspace.KindPlaceholder:
		index := int(k.Root)
		if index < 0 || index >= len(params) || params[index].IsEmpty() {
			return path.Path{}, false
		}
		return path.NewPlaceholder(index).AppendSegments(ks.Segments(k)), true
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
