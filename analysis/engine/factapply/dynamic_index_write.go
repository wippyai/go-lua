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
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
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
		AllValueTables:      append([]pathaddr.StateKey(nil), out.DynamicIndexAllValuesKeyMembershipTables(table.rootOrVisibleLocal)...),
		TableSymbol:         tablePath.Symbol,
		DefinitelyPresent:   dynamicIndexFactDefinitelyPresent(ctx.Registry, value),
		DefinitelyAbsent:    dynamicIndexFactDefinitelyAbsent(ctx.Registry, value),
		MayBeAbsent:         !dynamicIndexFactDefinitelyPresent(ctx.Registry, value),
		TableOwnerPlacement: placement.Bottom,
	}
	resolved.PendingRestores = append(resolved.PendingRestores, pendingDynamicAllValueRestoresFromPrimaryDelete(ctx, resolver, facts, out, fact, value)...)
	if keyStateKey, ok := dynamicIndexWriteKeyStateKeyAt(resolver, ctx.Point, facts, fact); ok {
		resolved.KeyStateKey, resolved.HasKeyStateKey = keyStateKey, true
	}
	if sourcePath, ok := dynamicIndexWriteSourcePath(resolver, facts, fact); ok {
		resolved.SourceMemberships = append(resolved.SourceMemberships, pathMembershipSourceTablesAt(in, resolver, ctx.Point, sourcePath)...)
	}
	if name, ok := staticStringKey(ctx.Registry, value.KeyValue); ok {
		targetPath := tablePath.IndexStr(name)
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
	if resolved.DefinitelyPresent && dynamicIndexWriteKeyIsLengthAppend(ctx.Registry, resolver, facts, fact.KeySource(), tablePath) {
		if tableKey, ok := visibility.AddressAt(resolver, ctx.Point, tablePath).VisibleStateKey(); ok {
			if base, ok := in.ReadLenFloor(resolver.KeySpace(), tableKey); ok && base < math.MaxInt64 {
				resolved.AppendStateKey, resolved.AppendFloor, resolved.HasAppend = tableKey, base+1, true
			}
		}
	}
	if tableValue, ok := ResolvePathAddressValue(ctx.Registry, resolver.KeySpace(), out, table); ok {
		if tableID, ok := product.Get(ctx.Registry, tableValue, identity.Key).ID(); ok {
			resolved.TableID, resolved.HasTableID = tableID, true
			resolved.TableOwnerPlacement = out.ReadPlacement(tableID)
		}
	}
	return ResolvedDynamicIndexWrite{data: resolved}, true
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

func pendingDynamicAllValueRestoresFromPrimaryDelete(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
) []state.PendingDynamicAllValueRestore {
	if resolver == nil || dynamicIndexFactDefinitelyPresent(ctx.Registry, value) {
		return nil
	}
	keyStateKey, ok := dynamicIndexWriteKeyStateKeyAt(resolver, ctx.Point, facts, fact)
	if !ok {
		return nil
	}
	origins := in.DynamicIndexReadOriginsForValue(keyStateKey)
	if len(origins) == 0 {
		return nil
	}
	var out []state.PendingDynamicAllValueRestore
	tablePath := fact.TablePathRef()
	forEachDynamicWriteTableStateKeyAt(resolver, ctx.Point, tablePath, func(tableStateKey pathaddr.StateKey) bool {
		for _, container := range in.DynamicIndexAllValuesKeyMembershipContainers(tableStateKey) {
			for _, origin := range origins {
				if origin.Container == container {
					out = append(out, state.PendingDynamicAllValueRestore{
						Container: container,
						Table:     tableStateKey,
						Key:       origin.Key,
					})
				}
			}
		}
		return true
	})
	return out
}

func forEachDynamicWriteTableStateKeyAt(resolver *visibility.Resolver, point cfg.Point, tablePath pathdom.Path, fn func(pathaddr.StateKey) bool) bool {
	if resolver == nil || tablePath.IsEmpty() || tablePath.Symbol == 0 {
		return true
	}
	return visibility.AddressAt(resolver, point, tablePath).ForEachStateKey(fn,
		visibility.StateKeyVisible,
		visibility.StateKeyRootOrVisible,
		visibility.StateKeyStructural,
	)
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

func dynamicIndexFactDefinitelyPresent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	if reg == nil || product.Equal(reg, fact.Value, product.Bottom(reg)) {
		return false
	}
	if typevalue.HasOnlyNilType(reg, fact.Value) {
		return false
	}
	return presence.Equal(product.PresenceOf(fact.Value), presence.Present())
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

func writeHeapTableDynamicIndexFact(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	tablePath pathdom.Path,
	key dynamicindex.Key,
	value dynamicindex.Fact,
) state.State {
	table, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, tablePath, nil)
	if !ok {
		return out
	}
	id, ok := product.Get(ctx.Registry, table.value, identity.Key).ID()
	if !ok {
		return out
	}
	object := out.ReadHeapTableObject(ctx.Registry, id)
	if heapidentity.ObjectDomain(ctx.Registry).Equal(object, heapidentity.BottomObject(ctx.Registry)) {
		return out
	}
	dynamic := object.DynamicIndexFacts()
	if dynamic == nil {
		dynamic = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
	}
	if existing, ok := dynamic[key]; ok {
		dynamic[key] = dynamicindex.Domain(ctx.Registry).Join(existing, value)
	} else {
		dynamic[key] = value
	}
	return out.WriteHeapTableObject(ctx.Registry, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              object.Root(),
		StaticMembers:     object.StaticMembers(),
		DynamicIndexFacts: dynamic,
	}))
}

func applyStoredDynamicIndexPlacement(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	tablePath pathdom.Path,
	value product.Value,
) state.State {
	if resolver == nil || tablePath.IsEmpty() {
		return out
	}
	table, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, tablePath, nil)
	if !ok {
		return out
	}
	tableID, ok := product.Get(ctx.Registry, table.value, identity.Key).ID()
	if !ok {
		return out
	}
	ownerPlacement := out.ReadPlacement(tableID)
	switch ownerPlacement {
	case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return markReachableHeapValuePlacement(ctx.Registry, out, value, ownerPlacement, map[identity.ID]struct{}{})
	default:
		return out
	}
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
