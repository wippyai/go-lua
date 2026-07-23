package factapply

import (
	"math"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyDynamicIndexWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
) state.State {
	request, ok := freezeResolvedDynamicIndexWrite(ctx, resolver, facts, sources, read, in, out, fact)
	if !ok {
		return out
	}
	written, ok := ApplyResolvedDynamicIndexWrite(ctx.Registry, resolver.KeySpace(), out, request)
	if !ok {
		return out
	}
	return written
}

func freezeResolvedDynamicIndexWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
) (ResolvedDynamicIndexWrite, bool) {
	tablePath := fact.TablePathRef()
	table, err := FreezeResolvedPathAddress(resolver, ctx.Point, tablePath)
	if err != nil {
		return ResolvedDynamicIndexWrite{}, false
	}
	value := dynamicIndexFact(ctx, sources, read, in, out, fact)
	tableStateKeys := visibility.AddressAt(resolver, ctx.Point, tablePath).StateKeys(
		visibility.StateKeyVisible, visibility.StateKeyRootOrVisible, visibility.StateKeyStructural,
	)
	resolved := &resolvedDynamicIndexWriteData{
		Table:               table,
		Key:                 dynamicindex.Key{Table: table.rootOrVisibleLocal, Site: dynamicindex.SiteForPoint(int(ctx.Point))},
		Fact:                value,
		TableStateKeys:      append([]pathaddr.StateKey(nil), tableStateKeys...),
		TableSymbol:         tablePath.Symbol,
		DefinitelyPresent:   dynamicIndexFactDefinitelyPresent(ctx.Registry, value),
		DefinitelyAbsent:    dynamicIndexFactDefinitelyAbsent(ctx.Registry, value),
		MayBeAbsent:         !dynamicIndexFactDefinitelyPresent(ctx.Registry, value),
		TableOwnerPlacement: placement.Bottom,
		Direct:              len(fact.TargetSuffixRef()) == 0,
	}
	var keyStateKey pathaddr.StateKey
	if resolvedKey, ok := dynamicIndexWriteKeyStateKeyAt(resolver, ctx.Point, facts, fact); ok {
		keyStateKey = resolvedKey
		resolved.KeyStateKey, resolved.HasKeyStateKey = resolvedKey, true
		path, interned := resolver.KeySpace().InternStateKey(resolvedKey)
		if !interned {
			return ResolvedDynamicIndexWrite{}, false
		}
		equivalent, observed := observeIndexMutationEquivalentKeys(ctx.Registry, resolver.KeySpace(), out, path)
		if !observed {
			return ResolvedDynamicIndexWrite{}, false
		}
		resolved.RestoreKeys = append([]pathaddr.StateKey{resolvedKey}, equivalent...)
	}
	var sourceStateKeys []pathaddr.StateKey
	if sourcePath, ok := dynamicIndexWriteSourcePath(resolver, facts, fact); ok {
		sourceStateKeys = pathMembershipSourceStateKeysAt(resolver, ctx.Point, sourcePath)
	}
	evidence, ok := observeIndexMutationEvidence(ctx.Registry, in, out, state.DynamicIndexMembershipEvidenceQuery{
		Container: table.rootOrVisibleLocal, KeyStateKey: keyStateKey,
		SourceStateKeys: sourceStateKeys, TableStateKeys: tableStateKeys,
	})
	if !ok {
		return ResolvedDynamicIndexWrite{}, false
	}
	resolved.AllValueTables = evidence.AllValueTables
	resolved.SourceMemberships = evidence.SourceMemberships
	if !resolved.DefinitelyPresent {
		resolved.PendingRestores = evidence.PendingRestores
	}
	if suffix := fact.TargetSuffixRef(); len(suffix) != 0 {
		if name, ok := staticStringKey(ctx.Registry, value.KeyValue); ok {
			targetPath := tablePath.IndexStr(name).AppendSegments(suffix)
			if !product.Equal(ctx.Registry, value.Value, product.Bottom(ctx.Registry)) {
				if target, err := FreezeResolvedPathAddress(resolver, ctx.Point, targetPath); err == nil {
					resolved.StaticTarget, resolved.HasStaticTarget = target, true
				}
			}
			if sourcePath, ok := sourcePathFromValueSource(resolver, facts, fact.Source()); ok &&
				!covariantExposureSuppressesPathProof(facts, resolver, ctx.Point, fact.Source()) {
				target, targetErr := FreezeResolvedPathAddress(resolver, ctx.Point, targetPath)
				source, sourceErr := FreezeResolvedPathAddress(resolver, ctx.Point, sourcePath)
				if targetErr == nil && sourceErr == nil {
					resolved.EqualityTarget, resolved.EqualitySource, resolved.HasEquality = target, source, true
				}
			}
		}
	}
	if resolved.Direct && resolved.DefinitelyPresent && dynamicIndexWriteKeyIsLengthAppend(ctx.Registry, resolver, facts, fact.KeySource(), tablePath) {
		if tableKey, ok := visibility.AddressAt(resolver, ctx.Point, tablePath).VisibleStateKey(); ok {
			path, interned := resolver.KeySpace().InternStateKey(tableKey)
			if !interned {
				return ResolvedDynamicIndexWrite{}, false
			}
			if base, observed := observeIndexMutationLengthFloor(ctx.Registry, resolver.KeySpace(), in, path); observed && base < math.MaxInt64 {
				resolved.AppendStateKey, resolved.AppendFloor, resolved.HasAppend = tableKey, base+1, true
			}
		}
	}
	if tableValue, ok := ResolvePathAddressValue(ctx.Registry, resolver.KeySpace(), out, table); ok {
		if tableID, ok := product.Get(ctx.Registry, tableValue, identity.Key).ID(); ok {
			resolved.TableID, resolved.HasTableID = tableID, true
			owner, observed := observeIndexMutationPlacement(ctx.Registry, out, identity.ConcreteTerm(tableID))
			if !observed {
				return ResolvedDynamicIndexWrite{}, false
			}
			resolved.TableOwnerPlacement = owner
		}
	}
	return ResolvedDynamicIndexWrite{data: resolved}, true
}

// freezeResolvedDynamicIndexWriteAt is the structural-boundary entry point.
// The nil form is exactly the ordinary visibility-owned transaction.
func freezeResolvedDynamicIndexWriteAt(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
	boundaryTable *ResolvedPathAddress,
) (ResolvedDynamicIndexWrite, bool) {
	if boundaryTable == nil {
		return freezeResolvedDynamicIndexWrite(ctx, resolver, facts, sources, read, in, out, fact)
	}
	return freezeResolvedDynamicIndexWriteWithTable(ctx, resolver, facts, sources, read, in, out, fact, *boundaryTable)
}

func freezeResolvedDynamicIndexWriteWithTable(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
	table ResolvedPathAddress,
) (ResolvedDynamicIndexWrite, bool) {
	tablePath := fact.TablePathRef()
	if resolver == nil || !table.belongsTo(resolver.KeySpace()) || !table.path.Equal(tablePath) {
		return ResolvedDynamicIndexWrite{}, false
	}
	value := dynamicIndexFact(ctx, sources, read, in, out, fact)
	tableStateKeys := uniqueStateKeys(table.stateKey, table.rootOrVisible, table.structural)
	resolved := &resolvedDynamicIndexWriteData{
		Table: table, Key: dynamicindex.Key{Table: table.rootOrVisibleLocal, Site: dynamicindex.SiteForPoint(int(ctx.Point))}, Fact: value,
		TableStateKeys: tableStateKeys,
		TableSymbol:    tablePath.Symbol, DefinitelyPresent: dynamicIndexFactDefinitelyPresent(ctx.Registry, value),
		DefinitelyAbsent: dynamicIndexFactDefinitelyAbsent(ctx.Registry, value), MayBeAbsent: !dynamicIndexFactDefinitelyPresent(ctx.Registry, value),
		TableOwnerPlacement: placement.Bottom,
		Direct:              len(fact.TargetSuffixRef()) == 0,
	}
	var keyStateKey pathaddr.StateKey
	if resolvedKey, ok := dynamicIndexWriteKeyStateKeyAt(resolver, ctx.Point, facts, fact); ok {
		keyStateKey = resolvedKey
		resolved.KeyStateKey, resolved.HasKeyStateKey = resolvedKey, true
		path, interned := resolver.KeySpace().InternStateKey(resolvedKey)
		if !interned {
			return ResolvedDynamicIndexWrite{}, false
		}
		equivalent, observed := observeIndexMutationEquivalentKeys(ctx.Registry, resolver.KeySpace(), out, path)
		if !observed {
			return ResolvedDynamicIndexWrite{}, false
		}
		resolved.RestoreKeys = append([]pathaddr.StateKey{resolvedKey}, equivalent...)
	}
	var sourceStateKeys []pathaddr.StateKey
	if sourcePath, ok := dynamicIndexWriteSourcePath(resolver, facts, fact); ok {
		sourceStateKeys = pathMembershipSourceStateKeysAt(resolver, ctx.Point, sourcePath)
	}
	evidence, ok := observeIndexMutationEvidence(ctx.Registry, in, out, state.DynamicIndexMembershipEvidenceQuery{
		Container: table.rootOrVisibleLocal, KeyStateKey: keyStateKey,
		SourceStateKeys: sourceStateKeys, TableStateKeys: tableStateKeys,
	})
	if !ok {
		return ResolvedDynamicIndexWrite{}, false
	}
	resolved.AllValueTables = evidence.AllValueTables
	resolved.SourceMemberships = evidence.SourceMemberships
	if !resolved.DefinitelyPresent {
		resolved.PendingRestores = evidence.PendingRestores
	}
	if suffix := fact.TargetSuffixRef(); len(suffix) != 0 {
		if name, ok := staticStringKey(ctx.Registry, value.KeyValue); ok {
			targetPath := tablePath.IndexStr(name).AppendSegments(suffix)
			root := table.owner.FromPath(tablePath.RootOnly())
			if target, err := FreezeBoundaryPathAddress(table.owner, root, targetPath); err == nil {
				if !product.Equal(ctx.Registry, value.Value, product.Bottom(ctx.Registry)) {
					resolved.StaticTarget, resolved.HasStaticTarget = target, true
				}
				if sourcePath, ok := sourcePathFromValueSource(resolver, facts, fact.Source()); ok &&
					!covariantExposureSuppressesPathProof(facts, resolver, ctx.Point, fact.Source()) {
					if source, sourceErr := FreezeResolvedPathAddress(resolver, ctx.Point, sourcePath); sourceErr == nil {
						resolved.EqualityTarget, resolved.EqualitySource, resolved.HasEquality = target, source, true
					}
				}
			}
		}
	}
	if tableValue, ok := ResolvePathAddressValue(ctx.Registry, resolver.KeySpace(), out, table); ok {
		if tableID, ok := product.Get(ctx.Registry, tableValue, identity.Key).ID(); ok {
			resolved.TableID, resolved.HasTableID = tableID, true
			owner, observed := observeIndexMutationPlacement(ctx.Registry, out, identity.ConcreteTerm(tableID))
			if !observed {
				return ResolvedDynamicIndexWrite{}, false
			}
			resolved.TableOwnerPlacement = owner
		}
	}
	return ResolvedDynamicIndexWrite{data: resolved}, true
}

func uniqueStateKeys(keys ...pathaddr.StateKey) []pathaddr.StateKey {
	out := make([]pathaddr.StateKey, 0, len(keys))
	for _, key := range keys {
		if key != "" && !stateKeyIn(out, key) {
			out = append(out, key)
		}
	}
	return out
}

func dynamicIndexWriteKeyIsLengthAppend(reg *axis.Registry, resolver *visibility.Resolver, facts factflow.Facts, source factflow.ValueSource, tablePath pathdom.Path) bool {
	plus, ok := binaryExpressionOperation(facts, source, "+")
	if !ok {
		return false
	}
	return dynamicIndexWriteSourceIsLengthOfPath(resolver, facts, plus.Left(), tablePath) &&
		expressionSourceIsIntegerLiteral(reg, facts, plus.Right(), 1) ||
		expressionSourceIsIntegerLiteral(reg, facts, plus.Left(), 1) &&
			dynamicIndexWriteSourceIsLengthOfPath(resolver, facts, plus.Right(), tablePath)
}

func dynamicIndexWriteSourceIsLengthOfPath(resolver *visibility.Resolver, facts factflow.Facts, source factflow.ValueSource, tablePath pathdom.Path) bool {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return false
	}
	op, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationUnary || op.Op() != "#" {
		return false
	}
	operand := op.Left()
	if operand.Kind == factflow.ValueSourceExpression && operand.HasExpr {
		got, ok := facts.ExpressionPathRef(operand.ExprRef)
		return ok && got.Equal(tablePath)
	}
	got, ok := callSourcePath(facts, resolver, operand)
	return ok && got.Equal(tablePath)
}

func dynamicIndexWriteKeyStateKeyAt(resolver *visibility.Resolver, point cfg.Point, facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathaddr.StateKey, bool) {
	keyPath, ok := dynamicIndexWriteKeyPath(resolver, facts, fact)
	if !ok {
		return "", false
	}
	return visibility.AddressAt(resolver, point, keyPath).VisibleStateKey()
}

func dynamicIndexWriteKeyPath(resolver *visibility.Resolver, facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if keyPath, ok := fact.KeyPathRef(); ok && keyPath.Symbol != 0 {
		return keyPath, true
	}
	sourcePath, ok := callSourcePath(facts, resolver, fact.KeySource())
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return sourcePath, true
}

func dynamicIndexFactDefinitelyAbsent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	if reg == nil || product.Equal(reg, fact.Value, product.Bottom(reg)) {
		return false
	}
	if presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
		return true
	}
	return typevalue.HasOnlyNilType(reg, fact.Value)
}

// DynamicIndexFactDefinitelyAbsent exposes the canonical write classification
// to the formal factor executor.
func DynamicIndexFactDefinitelyAbsent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	return dynamicIndexFactDefinitelyAbsent(reg, fact)
}

func dynamicIndexFactDefinitelyPresent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	if reg == nil || product.Equal(reg, fact.Value, product.Bottom(reg)) {
		return false
	}
	if typevalue.HasOnlyNilType(reg, fact.Value) {
		return false
	}
	return presence.Equal(product.PresenceOf(fact.Value), presence.Present())
}

// DynamicIndexFactDefinitelyPresent exposes the canonical write
// classification to the formal factor executor.
func DynamicIndexFactDefinitelyPresent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	return dynamicIndexFactDefinitelyPresent(reg, fact)
}

func stateKeyIn(keys []pathaddr.StateKey, want pathaddr.StateKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func dynamicIndexWriteSourcePath(resolver *visibility.Resolver, facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if valuePath, ok := fact.ValuePathRef(); ok && !valuePath.IsEmpty() && valuePath.Symbol != 0 {
		return valuePath, true
	}
	sourcePath, ok := callSourcePath(facts, resolver, fact.Source())
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return sourcePath, true
}

func dynamicIndexFact(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	current state.State,
	fact factflow.DynamicIndexWrite,
) dynamicindex.Fact {
	config := dynamicindex.FactConfig{Admission: fact.Admission()}
	readKey, readValue := dynamicIndexReadback(fact.ReadbackIntent())
	if readKey {
		keySource := fact.KeySource()
		if keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithCurrentPointState(ctx.Point, read, current)); ok {
			config.KeyValue = keyValue
			config.HasKeyValue = true
		}
	}
	if readValue {
		source := fact.Source()
		if value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, current)); ok {
			config.Value = value
			config.HasValue = true
		}
	}
	return dynamicindex.NewFact(ctx.Registry, config)
}

func dynamicIndexReadback(intent factflow.DynamicIndexReadbackIntent) (readKey bool, readValue bool) {
	switch intent {
	case factflow.DynamicIndexReadbackKey:
		return true, false
	case factflow.DynamicIndexReadbackValue:
		return false, true
	case factflow.DynamicIndexReadbackKeyAndValue:
		return true, true
	default:
		return false, false
	}
}
