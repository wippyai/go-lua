package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func applyRootAssignmentFact(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.RootAssignment,
	closedDynamicAllValues []ClosedDynamicAllValueInvariant,
	typeValues *typevalue.Cache,
) (state.State, bool) {
	declared, hasDeclared := fact.DeclaredValue()
	out, targetPath, sourceValue, hasSourceValue, applied := applyRootAssignment(ctx, resolver, facts, sources, read, in, out, fact.TargetSymbol(), fact.TargetPathRef(), fact.Source(), declared, hasDeclared, fact.DeclaredValueContracts(), fact.DeclaredValueOverlays())
	if !applied {
		if evidenceTarget, ok := rootAssignmentEvidenceTargetPath(fact.TargetSymbol(), fact.TargetPathRef()); ok {
			out = addPathKeyMembershipsFromDynamicIndexSource(ctx, resolver, facts, out, evidenceTarget, fact.Source())
		}
		return out, false
	}
	// The source path-equality proof is suppressed inside the shared helper for a
	// covariant record exposure of this source: the alias is typed wider than its
	// source, so the equality would couple them through reference-equality member
	// congruence and let a write through the wide alias reset the narrow source to
	// Top. The eager source widen (the covariant exposure applied at the end of the
	// node transfer) establishes the sound widened source field type instead. An
	// array exposure keeps the equality for its read-back diagnostics.
	out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, targetPath, fact.Source())
	out = addPathEqualityProofFromDynamicIndexSource(ctx, resolver, facts, sources, read, in, out, targetPath, fact.Source())
	out = addPathKeyMembershipsFromDynamicIndexSource(ctx, resolver, facts, out, targetPath, fact.Source())
	out = applyRootAssignmentNumFloor(ctx, resolver, facts, in, out, targetPath, fact.Source())
	out = applyRootAssignmentNumCeil(ctx, resolver, facts, in, out, targetPath, fact.Source())
	out = applyUserLatticeAssignment(ctx, resolver, facts, in, out, targetPath, fact.Source())
	out = applyRootAssignmentLenFloorFromValue(ctx, resolver, typeValues, out, targetPath, sourceValue, hasSourceValue)
	out = applyObjectLiteralEntriesWithKnownSourceValue(ctx, resolver, facts, sources, read, in, out, targetPath, fact.Source(), sourceValue, hasSourceValue, typeValues)
	out = applyClosedDynamicAllValueRootAssignment(ctx, resolver, facts, sources, read, in, out, targetPath, fact.Source(), closedDynamicAllValues)
	return out, applied
}

func rootAssignmentEvidenceTargetPath(target symbol.ID, targetPath pathdom.Path) (pathdom.Path, bool) {
	root, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return pathdom.Path{}, false
	}
	return rootAssignmentPath(root, targetPath), true
}

func applyRootAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	target symbol.ID,
	targetPath pathdom.Path,
	source factflow.ValueSource,
	declared product.Value,
	hasDeclared bool,
	declaredContracts bool,
	declaredOverlays bool,
) (state.State, pathdom.Path, product.Value, bool, bool) {
	root, ok := rootAssignmentTarget(target, targetPath)
	if !ok {
		return out, pathdom.Path{}, product.Value{}, false, false
	}
	if hasDeclared && declaredContracts {
		declared = declaredContractWithSourceRuntimeIdentity(ctx, facts, sources, read, in, out, source, declared)
		targetPath = rootAssignmentPath(root, targetPath)
		return writeRootSymbol(ctx, resolver, out, root, targetPath, declared), targetPath, declared, false, true
	}
	var value product.Value
	hasSourceValue := false
	if sourceValue, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, out)); ok && !product.Equal(ctx.Registry, sourceValue, product.Bottom(ctx.Registry)) {
		value = sourceValue
		hasSourceValue = true
		value = refineRootAssignmentDynamicIndexValue(ctx, resolver, facts, sources, read, in, value, source)
		value = refineRootAssignmentPathPresenceValue(ctx, resolver, facts, in, value, source)
		if hasDeclared && declaredOverlays {
			value = valueref.MergeDeclaredContract(ctx.Registry, value, declared)
			if declaredClaim := product.Get(ctx.Registry, declared, assertion.Key); !declaredClaim.IsBottom() && !declaredClaim.IsTop() {
				currentClaim := product.Get(ctx.Registry, value, assertion.Key)
				value = product.Set(ctx.Registry, value, assertion.Key, assertion.Combine(currentClaim, declaredClaim))
			}
			if declaredPresence := product.PresenceOf(declared); !declaredPresence.IsBottom() && !declaredPresence.IsTop() {
				value = product.WithPresence(ctx.Registry, value, declaredPresence)
			}
		}
	} else {
		if !hasDeclared {
			return out, pathdom.Path{}, product.Value{}, false, false
		}
		value = declared
	}
	targetPath = rootAssignmentPath(root, targetPath)
	return writeRootSymbol(ctx, resolver, out, root, targetPath, value), targetPath, value, hasSourceValue, true
}

func declaredContractWithSourceRuntimeIdentity(
	ctx transfer.NodeContext,
	_ factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	source factflow.ValueSource,
	declared product.Value,
) product.Value {
	if ctx.Registry == nil || sources == nil || !source.HasExpr {
		return declared
	}
	if id, ok := product.Get(ctx.Registry, declared, identity.Key).ID(); ok && id != (identity.ID{}) {
		return declared
	}
	if !declaredContractCanCarrySourceRuntimeIdentity(ctx.Registry, declared) {
		return declared
	}
	sourceValue, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return declared
	}
	id, ok := product.Get(ctx.Registry, sourceValue, identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		return declared
	}
	return product.Set(ctx.Registry, declared, identity.Key, identity.Singleton(id))
}

func declaredContractCanCarrySourceRuntimeIdentity(reg *axis.Registry, declared product.Value) bool {
	t, ok := typevalue.TypeOf(reg, declared)
	if !ok || t == nil {
		return false
	}
	switch t.(type) {
	case *typ.Record:
		return true
	default:
		return typ.IsBuiltinTableTopMarker(t)
	}
}

func refineRootAssignmentDynamicIndexValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	value product.Value,
	source factflow.ValueSource,
) product.Value {
	if resolver == nil || sources == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return value
	}
	dyn, ok := facts.DynamicIndexExpression(source.ExprRef)
	if !ok {
		return value
	}
	if !dynamicIndexModuloLengthKeyInRange(ctx, resolver, facts, sources, read, in, dyn) {
		return value
	}
	return sourcevalue.WithoutNilRuntimeKind(ctx.Registry, product.WithPresence(ctx.Registry, value, presence.Present()))
}

func refineRootAssignmentPathPresenceValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	value product.Value,
	source factflow.ValueSource,
) product.Value {
	if resolver == nil || resolver.KeySpace() == nil {
		return value
	}
	sourcePath, ok := sourcePathFromValueSource(resolver, facts, source)
	if !ok || sourcePath.IsEmpty() {
		return value
	}
	sourceKey, ok := visibility.AddressAt(resolver, ctx.Point, sourcePath).VisibleLocalKeyspaceKey()
	if !ok || sourceKey.Kind == keyspace.KindInvalid {
		return value
	}
	snapshot := in.BranchProofsSnapshot(resolver.KeySpace())
	if snapshot.Bottom || snapshot.Top || len(snapshot.Proofs) == 0 {
		return value
	}
	for _, proof := range snapshot.Proofs {
		if proof.Kind != pathevidence.BranchProofPathPresence ||
			!presence.Equal(proof.Presence, presence.Present()) {
			continue
		}
		if rootAssignmentPresenceProofMatchesKey(resolver.KeySpace(), proof.Path, sourceKey) {
			return sourcevalue.WithoutNilRuntimeKind(ctx.Registry, product.WithPresence(ctx.Registry, value, presence.Present()))
		}
	}
	return value
}

func rootAssignmentPresenceProofMatchesKey(ks *keyspace.KeySpace, proof, candidate keyspace.Key) bool {
	if proof == candidate {
		return true
	}
	if ks == nil ||
		proof.Kind != keyspace.KindResolverSym ||
		candidate.Kind != keyspace.KindResolverSym ||
		proof.Sym == 0 ||
		proof.Sym != candidate.Sym {
		return false
	}
	proofSegments, proofOK := ks.SegmentsView(proof)
	candidateSegments, candidateOK := ks.SegmentsView(candidate)
	if !proofOK || !candidateOK || len(proofSegments) != len(candidateSegments) {
		return false
	}
	return pathaddr.SegmentsHasPrefix(proofSegments, candidateSegments) &&
		pathaddr.SegmentsHasPrefix(candidateSegments, proofSegments)
}

func dynamicIndexModuloLengthKeyInRange(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	dyn factflow.DynamicIndexExpression,
) bool {
	tablePath := dyn.TablePathRef()
	if !dynamicIndexTableProvenNonEmpty(ctx, resolver, in, tablePath) {
		return false
	}
	baseSource, ok := moduloLengthIndexBaseSource(ctx.Registry, facts, dyn.KeySource(), tablePath)
	if !ok {
		return false
	}
	baseValue, ok := sources.ValueOfSource(ctx.Point, baseSource, in, readWithCurrentPointState(ctx.Point, read, in))
	if !ok {
		return false
	}
	return typevalue.HasIntegerType(ctx.Registry, baseValue)
}

func moduloLengthIndexBaseSource(
	reg *axis.Registry,
	facts factflow.Facts,
	keySource factflow.ValueSource,
	tablePath pathdom.Path,
) (factflow.ValueSource, bool) {
	plus, ok := binaryExpressionOperation(facts, keySource, "+")
	if !ok {
		return factflow.ValueSource{}, false
	}
	var modSource factflow.ValueSource
	switch {
	case expressionSourceIsIntegerLiteral(reg, facts, plus.Right(), 1):
		modSource = plus.Left()
	case expressionSourceIsIntegerLiteral(reg, facts, plus.Left(), 1):
		modSource = plus.Right()
	default:
		return factflow.ValueSource{}, false
	}
	mod, ok := binaryExpressionOperation(facts, modSource, "%")
	if !ok || !expressionSourceIsLengthOfPath(facts, mod.Right(), tablePath) {
		return factflow.ValueSource{}, false
	}
	return mod.Left(), true
}

func binaryExpressionOperation(facts factflow.Facts, source factflow.ValueSource, op string) (factflow.ExpressionOperation, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ExpressionOperation{}, false
	}
	exprOp, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || exprOp.Kind() != factflow.ExpressionOperationBinary || exprOp.Op() != op {
		return factflow.ExpressionOperation{}, false
	}
	return exprOp, true
}

func expressionSourceIsIntegerLiteral(reg *axis.Registry, facts factflow.Facts, source factflow.ValueSource, want int64) bool {
	if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralInteger {
		return source.Int == want
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	value, ok := facts.ExpressionValue(source.ExprRef)
	if !ok {
		return false
	}
	got, ok := typevalue.IntegerLiteralValue(reg, value)
	return ok && got == want
}

func expressionSourceIsLengthOfPath(facts factflow.Facts, source factflow.ValueSource, p pathdom.Path) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationUnary || op.Op() != "#" {
		return false
	}
	operand := op.Left()
	if operand.Kind != factflow.ValueSourceExpression || !operand.HasExpr {
		return false
	}
	got, ok := facts.ExpressionPathRef(operand.ExprRef)
	return ok && got.Equal(p)
}

func dynamicIndexTableProvenNonEmpty(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	in state.State,
	tablePath pathdom.Path,
) bool {
	stateKey, ok := visibility.AddressAt(resolver, ctx.Point, tablePath).VisibleStateKey()
	if !ok {
		return false
	}
	if floor, ok := in.ReadLenFloor(resolver.KeySpace(), stateKey); ok && floor >= 1 {
		return true
	}
	if value, ok := in.ReadPathStaticMember(resolver.KeySpace(), stateKey.PathKey()); ok {
		if t, ok := typevalue.TypeOf(ctx.Registry, value); ok {
			return typevalue.DefinitelyNonEmptyIndexContainer(t)
		}
	}
	if value, ok := sourcevalue.ReadPathValue(ctx.Registry, resolver, ctx.Point, tablePath, in); ok {
		if t, ok := typevalue.TypeOf(ctx.Registry, value); ok {
			return typevalue.DefinitelyNonEmptyIndexContainer(t)
		}
	}
	return false
}

func rootAssignmentTarget(target symbol.ID, targetPath pathdom.Path) (symbol.ID, bool) {
	if len(targetPath.Segments) != 0 {
		return 0, false
	}
	if target != 0 {
		return target, true
	}
	if targetPath.Symbol != 0 {
		return targetPath.Symbol, true
	}
	return 0, false
}

func rootAssignmentPath(target symbol.ID, targetPath pathdom.Path) pathdom.Path {
	out := targetPath
	if out.Symbol == 0 {
		out.Symbol = target
	}
	return out
}

func writeRootSymbol(ctx transfer.NodeContext, resolver *visibility.Resolver, out state.State, target symbol.ID, targetPath pathdom.Path, value product.Value) state.State {
	if target == 0 {
		return out
	}
	if resolver != nil {
		preserved := rootAssignmentPreservedStableTargetImplications(ctx.Registry, resolver, out, target, value)
		if invalidated, ok := invalidatePathSubtreeAt(out, resolver, ctx.Point, targetPath); ok {
			out = invalidated
		}
		out = out.InvalidateStableSymbolPathEvidence(target)
		for _, implication := range preserved {
			out = out.AddPathPresenceImplication(implication)
		}
	}
	return out.WriteValue(ctx.Registry, key.SymbolValue(target), value)
}

func rootAssignmentPreservedStableTargetImplications(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	out state.State,
	target symbol.ID,
	value product.Value,
) []pathevidence.PathPresenceImplication {
	if reg == nil || resolver == nil || target == 0 {
		return nil
	}
	snapshot := out.PathPresenceImplicationsSnapshot(resolver.KeySpace())
	if snapshot.Bottom || len(snapshot.Implications) == 0 {
		return nil
	}
	var preserved []pathevidence.PathPresenceImplication
	for _, implication := range snapshot.Implications {
		if stableRootImplicationEndpointMatchesSymbol(implication.Trigger, target) {
			continue
		}
		if !stableRootImplicationEndpointMatchesSymbol(implication.Target, target) {
			continue
		}
		if !rootAssignmentValueSatisfiesImplicationTarget(reg, value, implication) {
			continue
		}
		preserved = append(preserved, implication)
	}
	return preserved
}

func stableRootImplicationEndpointMatchesSymbol(candidate keyspace.Key, target symbol.ID) bool {
	if candidate.Sym != target || candidate.Segs != 0 {
		return false
	}
	switch candidate.Kind {
	case keyspace.KindUnversionedSym, keyspace.KindStableSym:
		return true
	default:
		return false
	}
}

func rootAssignmentValueSatisfiesImplicationTarget(
	reg *axis.Registry,
	value product.Value,
	implication pathevidence.PathPresenceImplication,
) bool {
	if implication.HasTargetValue {
		return product.Domain(reg).LessOrEq(value, implication.TargetValue)
	}
	if !presence.Equal(implication.TargetPresence, presence.Present()) && !presence.Equal(implication.TargetPresence, presence.Absent()) {
		return false
	}
	constraint := product.NewWithPresence(reg, product.ShapeTop, implication.TargetPresence)
	return product.Domain(reg).LessOrEq(value, constraint)
}

func applyClosedDynamicAllValueRootAssignment(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
	invariants []ClosedDynamicAllValueInvariant,
) state.State {
	if resolver == nil || len(invariants) == 0 || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return out
	}
	targetRoot := pathdom.Path{Symbol: targetPath.Symbol}
	freshEmpty := rootAssignmentIsFreshEmptyTable(ctx, resolver, facts, sources, read, in, out, targetRoot, source)
	for _, invariant := range invariants {
		if invariant.Container.Equal(targetRoot) && freshEmpty {
			out = addClosedDynamicAllValueInvariantAt(resolver, out, invariant)
			continue
		}
		if invariant.Table.Equal(targetRoot) && rootPathHasFreshEmptyTable(ctx.Registry, out, invariant.Container) {
			out = addClosedDynamicAllValueInvariantAt(resolver, out, invariant)
		}
	}
	return out
}

func rootAssignmentIsFreshEmptyTable(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	root pathdom.Path,
	source factflow.ValueSource,
) bool {
	if rootPathHasFreshEmptyTable(ctx.Registry, out, root) {
		return true
	}
	if sources == nil || !source.HasExpr {
		return false
	}
	literal, ok := facts.ObjectLiteralView(source.ExprRef)
	if !ok || literal.EntryCount() != 0 {
		return false
	}
	sourceValue, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return false
	}
	_, ok = product.Get(ctx.Registry, sourceValue, identity.Key).ID()
	return ok
}

func addClosedDynamicAllValueInvariantAt(resolver *visibility.Resolver, out state.State, invariant ClosedDynamicAllValueInvariant) state.State {
	containerKey := resolver.KeySpace().FromPath(invariant.Container)
	if containerKey.Kind == keyspace.KindInvalid {
		return out
	}
	tableKey := resolver.KeySpace().FromPath(invariant.Table)
	if tableKey.Kind == keyspace.KindInvalid {
		return out
	}
	tableStateKey, ok := pathaddr.StateKeyFromPathKey(resolver.KeySpace().Format(tableKey))
	if !ok {
		return out
	}
	return out.AddDynamicIndexAllValuesKeyMembership(containerKey, tableStateKey)
}

func rootPathHasFreshEmptyTable(reg *axis.Registry, st state.State, root pathdom.Path) bool {
	if reg == nil || root.Symbol == 0 || len(root.Segments) != 0 {
		return false
	}
	value := st.ReadSymbolValue(reg, root.Symbol)
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return false
	}
	object := st.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return false
	}
	return len(object.StaticMembers()) == 0 && len(object.DynamicIndexFacts()) == 0
}

func rootPathDynamicValueKeyMembershipTables(reg *axis.Registry, st state.State, root pathdom.Path, container keyspace.Key) []pathaddr.StateKey {
	if reg == nil || root.Symbol == 0 || len(root.Segments) != 0 {
		return nil
	}
	value := st.ReadSymbolValue(reg, root.Symbol)
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return nil
	}
	object := st.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return nil
	}
	if !product.Equal(reg, object.Root(), value) || len(object.StaticMembers()) != 0 {
		return nil
	}
	return dynamicIndexValueCommonKeyMembershipTablesFromFacts(reg, st, container, object.DynamicIndexFacts())
}

func addPathKeyMembershipsFromDynamicIndexSource(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return out
	}
	dyn, ok := facts.DynamicIndexExpression(source.ExprRef)
	if !ok {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).VisibleStateKey()
	if !ok {
		return out
	}
	var readKey pathaddr.StateKey
	if keyPath, keyOK := dynamicIndexExpressionKeyPath(resolver, facts, dyn); keyOK {
		readKey, _ = visibility.AddressAt(resolver, ctx.Point, keyPath).VisibleStateKey()
	}
	forEachDynamicIndexTableKeyAt(resolver, ctx.Point, dyn.TablePathRef(), func(containerKey keyspace.Key) bool {
		if readKey != "" {
			out = out.AddDynamicIndexReadOrigin(targetKey, containerKey, readKey)
		}
		for _, table := range dynamicIndexValueCommonKeyMembershipTables(ctx.Registry, out, containerKey) {
			out = out.AddPathKeyMembership(targetKey, table)
		}
		return true
	})
	return out
}

func dynamicIndexExpressionKeyPath(resolver *visibility.Resolver, facts factflow.Facts, dyn factflow.DynamicIndexExpression) (pathdom.Path, bool) {
	p, ok := sourcePathFromValueSource(resolver, facts, dyn.KeySource())
	if !ok || p.IsEmpty() || p.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return p, true
}

func forEachDynamicIndexTableKeyAt(resolver *visibility.Resolver, point cfg.Point, tablePath pathdom.Path, fn func(keyspace.Key) bool) bool {
	if resolver == nil {
		return true
	}
	return visibility.AddressAt(resolver, point, tablePath).ForEachKeyspaceKey(fn,
		visibility.StateKeyVisible,
		visibility.StateKeyRootOrVisible,
	)
}

func dynamicIndexValueCommonKeyMembershipTables(reg *axis.Registry, st state.State, container keyspace.Key) []pathaddr.StateKey {
	if tables := st.DynamicIndexAllValuesKeyMembershipTables(container); len(tables) != 0 {
		return tables
	}
	snapshot := st.DynamicIndexFactsSnapshot()
	if snapshot.Top || len(snapshot.Facts) == 0 {
		return nil
	}
	return dynamicIndexValueCommonKeyMembershipTablesFromFacts(reg, st, container, snapshot.Facts)
}

func dynamicIndexValueCommonKeyMembershipTablesFromFacts(reg *axis.Registry, st state.State, container keyspace.Key, facts map[dynamicindex.Key]dynamicindex.Fact) []pathaddr.StateKey {
	if len(facts) == 0 {
		return nil
	}
	domain := product.Domain(reg)
	common := map[pathaddr.StateKey]struct{}{}
	foundValueSource := false
	for dynamicKey, fact := range facts {
		if dynamicKey.Table != container || fact.Admission == dynamicindex.AdmissionRejected ||
			domain.Equal(fact.Value, domain.Bottom()) ||
			presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
			continue
		}
		tables := st.DynamicIndexValueKeyMembershipTables(container, dynamicKey.Site)
		if len(tables) == 0 {
			return nil
		}
		if !foundValueSource {
			for _, table := range tables {
				common[table] = struct{}{}
			}
			foundValueSource = true
			continue
		}
		next := make(map[pathaddr.StateKey]struct{}, len(common))
		for _, table := range tables {
			if _, ok := common[table]; ok {
				next[table] = struct{}{}
			}
		}
		common = next
		if len(common) == 0 {
			return nil
		}
	}
	if !foundValueSource {
		return nil
	}
	out := make([]pathaddr.StateKey, 0, len(common))
	for table := range common {
		out = append(out, table)
	}
	return out
}

func applyRootAssignmentNumFloor(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).RootOrVisibleStateKey()
	if !ok {
		return out
	}
	// Reassigning the root invalidates every difference relation over its old
	// value (and, if it is an array, its old length).
	out = out.ClearDiffConstraintsFor(targetKey)
	out = out.ClearNumFloor(resolver.KeySpace(), targetKey)
	if floor, ok := sourcevalue.NumFloorForSource(ctx.Registry, resolver, ctx.Point, facts, in, source); ok {
		return out.WriteNumFloor(resolver.KeySpace(), targetKey, floor)
	}
	return out
}

func applyRootAssignmentNumCeil(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).RootOrVisibleStateKey()
	if !ok {
		return out
	}
	out = out.ClearNumCeil(resolver.KeySpace(), targetKey)
	if ceil, ok := sourcevalue.NumCeilForSource(ctx.Registry, resolver, ctx.Point, facts, in, source); ok {
		return out.WriteNumCeil(resolver.KeySpace(), targetKey, ceil)
	}
	return out
}

func applyRootAssignmentLenFloorFromValue(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	typeValues *typevalue.Cache,
	out state.State,
	targetPath pathdom.Path,
	sourceValue product.Value,
	hasSourceValue bool,
) state.State {
	if resolver == nil || typeValues == nil || !hasSourceValue || targetPath.Symbol == 0 || len(targetPath.Segments) != 0 {
		return out
	}
	targetKey, ok := visibility.AddressAt(resolver, ctx.Point, targetPath).VisibleStateKey()
	if !ok {
		return out
	}
	t, ok := typeValues.TypeOf(ctx.Registry, sourceValue)
	if !ok {
		return out
	}
	if floor := staticSequenceLengthFloor(t, 0); floor > 0 {
		return out.WriteLenFloor(resolver.KeySpace(), targetKey, floor)
	}
	return out
}

func staticSequenceLengthFloor(t typ.Type, depth int) int64 {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return 0
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return staticSequenceLengthFloor(tt.Inner, depth+1)
	case *typ.Alias:
		return staticSequenceLengthFloor(tt.UnaliasedTarget(), depth+1)
	case *typ.Tuple:
		return int64(len(tt.Elements))
	case *typ.Record:
		var floor int64
		for i := int64(1); ; i++ {
			member := tt.GetStaticIntIndex(i)
			if member == nil || member.Optional {
				return floor
			}
			floor = i
		}
	case *typ.Union:
		if len(tt.Members) == 0 {
			return 0
		}
		var min int64
		for _, member := range tt.Members {
			floor := staticSequenceLengthFloor(member, depth+1)
			if floor == 0 {
				return 0
			}
			if min == 0 || floor < min {
				min = floor
			}
		}
		return min
	default:
		return 0
	}
}

func staticSequenceExactLength(t typ.Type, depth int) (int64, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return 0, false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return staticSequenceExactLength(tt.Inner, depth+1)
	case *typ.Alias:
		return staticSequenceExactLength(tt.UnaliasedTarget(), depth+1)
	case *typ.Tuple:
		return int64(len(tt.Elements)), true
	case *typ.Union:
		if len(tt.Members) == 0 {
			return 0, false
		}
		var length int64
		for i, member := range tt.Members {
			memberLength, ok := staticSequenceExactLength(member, depth+1)
			if !ok || (i > 0 && memberLength != length) {
				return 0, false
			}
			length = memberLength
		}
		return length, true
	default:
		return 0, false
	}
}
