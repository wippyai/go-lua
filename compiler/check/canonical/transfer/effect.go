package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type DynamicWriteMode uint8

const (
	DynamicWriteForeign DynamicWriteMode = iota
	DynamicWriteSelfDerived
)

type MutatorKind uint8

const (
	MutatorAppendElement MutatorKind = iota + 1
	MutatorAppendMapElement
	MutatorContainerElementUnion
)

type referenceWriteMode uint8

const (
	referenceWritePreserve referenceWriteMode = iota
	referenceWriteFromSource
	referenceWriteExplicit
)

type functionRefsWrite struct {
	Mode referenceWriteMode
	Refs flow.FunctionRefs
}

type closureRefsWrite struct {
	Mode referenceWriteMode
	Refs flow.ClosureRefs
}

type RelationEffectKind uint8

const (
	RelationSeedSiblingNil RelationEffectKind = iota + 1
	RelationKillSymbols
	RelationSeedTargetLengthParam
	RelationSeedContainerLowerBound
	RelationKillLengthTargets
	RelationSeedGuardedType
)

// RelationEffect is the canonical reducer payload for point-local relation facts.
// Relations are must-facts in PointState.Rel, so transfer code seeds and kills
// them through this reducer rather than editing the relation axis directly.
type RelationEffect struct {
	Kind          RelationEffectKind
	ErrSym        cfg.SymbolID
	ValueSyms     []cfg.SymbolID
	Symbols       []cfg.SymbolID
	TargetRoot    cfg.SymbolID
	TargetKey     constraint.PathKey
	ParamIndex    int
	Lower         int64
	GuardSym      cfg.SymbolID
	TargetSym     cfg.SymbolID
	GuardOnTruthy bool
	TargetType    typ.Type
}

// ReturnSlotEffect is the transfer payload for one caller-visible return slot.
// Source records function/closure identity facts under the slot placeholder;
// Value records the product value for non-symbol expressions whose value would
// otherwise be lost before summary projection.
type ReturnSlotEffect struct {
	Index        int
	Source       ast.Expr
	Value        product.AbstractValue
	FunctionRefs flow.FunctionRefs
}

// ReturnEffect is the canonical reducer payload for a normal return boundary.
// It owns the PointState axes consumed by summary projection: return-slot values,
// return-slot function/closure identities, and return-slot relations.
type ReturnEffect struct {
	Relations flow.ReturnRelations
	Slots     []ReturnSlotEffect
}

// ReferenceEffect is the canonical reducer payload for function/closure identity
// facts. Source-mode effects derive identity facts from a source expression and
// clear stale facts at the target. Explicit-mode effects install an already
// rebased reference map. Place targets use exact static paths when available and
// fall back to root-subtree invalidation for dynamic places.
type ReferenceEffect struct {
	Place        Place
	Path         constraint.Path
	Source       ast.Expr
	FunctionRefs functionRefsWrite
	ClosureRefs  closureRefsWrite
}

// PrototypeSelfEffect records the runtime `self` value that flows into methods
// of one prototype table in Lua split-pattern OOP. Both setmetatable publishing
// sites and method-body self writes lower through this reducer.
type PrototypeSelfEffect struct {
	Prototype cfg.SymbolID
	Value     product.AbstractValue
}

func sourceFunctionRefsWrite() functionRefsWrite {
	return functionRefsWrite{Mode: referenceWriteFromSource}
}

func explicitFunctionRefsWrite(refs flow.FunctionRefs) functionRefsWrite {
	return functionRefsWrite{Mode: referenceWriteExplicit, Refs: refs}
}

func sourceClosureRefsWrite() closureRefsWrite {
	return closureRefsWrite{Mode: referenceWriteFromSource}
}

func explicitClosureRefsWrite(refs flow.ClosureRefs) closureRefsWrite {
	return closureRefsWrite{Mode: referenceWriteExplicit, Refs: refs}
}

// CellEffect is the canonical reducer payload for applying captured-cell effects
// produced by a call. Effects update the caller's live cell store when they touch
// cells owned by the current frame, are always composed into the caller-visible
// summary effect, and can also update a stored closure environment when the call
// target is a closure value.
type CellEffect struct {
	Effects     flow.CaptureEffects
	ClosurePath constraint.Path
}

func (t *Transfer) applyCellEffect(out *flow.PointState, effect CellEffect) bool {
	if out == nil || flow.CaptureEffectsDomain.Equal(effect.Effects, flow.CaptureEffectsDomain.Bottom()) {
		return false
	}
	if t.applyClosureCellEffect(out, effect) {
		t.applyCellStoreEffects(out, t.currentCellEffects(out, effect.Effects))
		t.recordCellEffects(out, effect.Effects)
		return true
	}
	t.applyCellStoreEffects(out, effect.Effects)
	t.recordCellEffects(out, effect.Effects)
	return true
}

func (t *Transfer) applyClosureCellEffect(out *flow.PointState, effect CellEffect) bool {
	if effect.ClosurePath.IsEmpty() {
		return false
	}
	addr, ok := flow.StableAddressOfPath(effect.ClosurePath)
	if !ok {
		return false
	}
	if _, ok := flow.ClosureRefAtAddress(out.ClosureRefs, addr); !ok {
		return false
	}
	return references.applyClosureCellEffects(out, addr, effect.Effects)
}

func (t *Transfer) currentCellEffects(out *flow.PointState, effects flow.CaptureEffects) flow.CaptureEffects {
	if flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return flow.CaptureEffectsDomain.Bottom()
	}
	if effects.IsTop() {
		if len(out.Cells.Entries()) > 0 {
			return effects
		}
		return flow.CaptureEffectsDomain.Bottom()
	}
	var entries []flow.CaptureEffect
	for _, entry := range effects.Entries() {
		if t.symbolStorage.hasCellEffectTarget(out, entry.Symbol) {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return flow.CaptureEffectsDomain.Bottom()
	}
	return flow.CaptureEffectsOf(entries)
}

func (t *Transfer) applyCellStoreEffects(out *flow.PointState, effects flow.CaptureEffects) {
	if flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return
	}
	out.Cells = effects.Apply(out.Cells)
}

func (t *Transfer) recordCellEffects(out *flow.PointState, effects flow.CaptureEffects) {
	if flow.CaptureEffectsDomain.Equal(effects, flow.CaptureEffectsDomain.Bottom()) {
		return
	}
	out.CellEffects = out.CellEffects.Then(effects)
}

func (t *Transfer) recordReceiverEffect(
	out *flow.PointState,
	slot int,
	value product.AbstractValue,
	mutations ...flow.ReceiverMutation,
) bool {
	if out == nil || slot < 0 || value.IsZero() {
		return false
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(flow.ReceiverMustWriteWithMutations(slot, value, mutations))
	return !flow.ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}

func (t *Transfer) recordReceiverMutationEffect(
	out *flow.PointState,
	sym cfg.SymbolID,
	mutations ...flow.ReceiverMutation,
) bool {
	if out == nil || sym == 0 || len(mutations) == 0 {
		return false
	}
	slot, ok := t.paramBySym[sym]
	if !ok {
		if t.prototypeSelfSymbol == 0 || sym != t.prototypeSelfSymbol || t.prototypeSelfSlot < 0 {
			return false
		}
		slot = t.prototypeSelfSlot
	}
	before := out.ReceiverEffects
	out.ReceiverEffects = out.ReceiverEffects.Then(flow.ReceiverMutations(slot, mutations))
	return !flow.ReceiverEffectsDomain.Equal(before, out.ReceiverEffects)
}

func receiverMutationForPlace(place Place, presentElementWrite bool) (flow.ReceiverMutation, bool) {
	path, ok := place.StaticPrefixPath()
	if !ok || path.Symbol == 0 {
		return flow.ReceiverMutation{}, false
	}
	return flow.ReceiverMutation{
		Segments:            append([]constraint.Segment(nil), path.Segments...),
		PresentElementWrite: presentElementWrite,
	}, true
}

func (t *Transfer) applyRelationEffect(out *flow.PointState, effect RelationEffect) bool {
	if out == nil {
		return false
	}
	before := out.Rel
	switch effect.Kind {
	case RelationSeedSiblingNil:
		out.Rel = out.Rel.WithSiblingNil(effect.ErrSym, effect.ValueSyms)
	case RelationKillSymbols:
		out.Rel = out.Rel.KillSymbols(effect.Symbols...)
	case RelationSeedTargetLengthParam:
		out.Rel = out.Rel.WithTargetLengthParam(effect.TargetRoot, effect.TargetKey, effect.ParamIndex)
	case RelationSeedContainerLowerBound:
		out.Rel = out.Rel.WithContainerLowerBound(effect.TargetRoot, effect.TargetKey, effect.Lower)
	case RelationKillLengthTargets:
		out.Rel = out.Rel.KillLengthTargets(effect.Symbols...)
	case RelationSeedGuardedType:
		out.Rel = out.Rel.WithGuardedType(effect.GuardSym, effect.TargetSym, effect.GuardOnTruthy, effect.TargetType)
	default:
		return false
	}
	return !flow.PointRelationsDomain.Equal(before, out.Rel)
}

func (t *Transfer) applyReturnEffect(out *flow.PointState, effect ReturnEffect) bool {
	if out == nil {
		return false
	}
	changed := false
	if !flow.ReturnRelationsDomain.Equal(out.ReturnRel, effect.Relations) {
		out.ReturnRel = effect.Relations
		changed = true
	}
	writer := flow.NewPointWriter(out)
	for _, slot := range effect.Slots {
		if slot.Index < 0 {
			continue
		}
		key := ReturnSlotKey(slot.Index)
		changed = writer.DeleteValueKey(key) || changed
		if slot.Source != nil {
			changed = t.applyReferenceEffect(out, ReferenceEffect{
				Path:         constraint.NewPlaceholder(slot.Index),
				Source:       slot.Source,
				FunctionRefs: sourceFunctionRefsWrite(),
				ClosureRefs:  sourceClosureRefsWrite(),
			}) || changed
		}
		if !flow.FunctionRefsDomain.Equal(slot.FunctionRefs, flow.FunctionRefsDomain.Bottom()) {
			changed = t.applyReferenceEffect(out, ReferenceEffect{
				Path:         constraint.NewPlaceholder(slot.Index),
				FunctionRefs: explicitFunctionRefsWrite(slot.FunctionRefs),
			}) || changed
		}
		if slot.Value.IsZero() {
			continue
		}
		changed = writer.WriteValueKey(key, slot.Value, false) || changed
	}
	return changed
}

func (t *Transfer) applyReferenceEffect(out *flow.PointState, effect ReferenceEffect) bool {
	if out == nil {
		return false
	}
	path, exact, ok := effect.referenceTarget()
	if !ok {
		return false
	}
	changed := t.applyFunctionReferenceEffect(out, path, exact, effect.Source, effect.FunctionRefs)
	changed = t.applyClosureReferenceEffect(out, path, exact, effect.Source, effect.ClosureRefs) || changed
	return changed
}

func (effect ReferenceEffect) referenceTarget() (constraint.Path, bool, bool) {
	if !effect.Path.IsEmpty() {
		return effect.Path, true, true
	}
	if effect.Place.Root == 0 {
		return constraint.Path{}, false, false
	}
	if path, ok := effect.Place.StaticPath(); ok && !path.IsEmpty() {
		return path, true, true
	}
	return constraint.NewPath(effect.Place.Root, effect.Place.RootName), false, true
}

func (t *Transfer) applyFunctionReferenceEffect(
	out *flow.PointState,
	path constraint.Path,
	exact bool,
	src ast.Expr,
	write functionRefsWrite,
) bool {
	before := out.FunctionRefs
	switch write.Mode {
	case referenceWritePreserve:
		return false
	case referenceWriteFromSource:
		if !exact {
			if addr, ok := flow.StableAddressOfPath(path); ok {
				references.clearFunctionSubtree(out, addr)
			}
			return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
		}
		t.recordFunctionRefAt(out, path, src)
	case referenceWriteExplicit:
		if addr, ok := flow.StableAddressOfPath(path); ok {
			references.clearFunctionSubtree(out, addr)
		}
		references.joinFunctions(out, write.Refs)
	default:
		return false
	}
	return !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs)
}

func (t *Transfer) applyClosureReferenceEffect(
	out *flow.PointState,
	path constraint.Path,
	exact bool,
	src ast.Expr,
	write closureRefsWrite,
) bool {
	before := out.ClosureRefs
	switch write.Mode {
	case referenceWritePreserve:
		return false
	case referenceWriteFromSource:
		if !exact {
			if addr, ok := flow.StableAddressOfPath(path); ok {
				references.clearClosureSubtree(out, addr)
			}
			return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
		}
		t.recordClosureRefAt(out, path, src)
	case referenceWriteExplicit:
		if addr, ok := flow.StableAddressOfPath(path); ok {
			references.clearClosureSubtree(out, addr)
		}
		references.joinClosures(out, write.Refs)
	default:
		return false
	}
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (t *Transfer) applyPrototypeSelfEffect(out *flow.PointState, effect PrototypeSelfEffect) bool {
	if out == nil || effect.Prototype == 0 || effect.Value.IsZero() {
		return false
	}
	before := out.PrototypeSelf
	out.PrototypeSelf = out.PrototypeSelf.JoinValue(effect.Prototype, effect.Value)
	return !flow.PrototypeSelfDomain.Equal(before, out.PrototypeSelf)
}

func (t *Transfer) applyPrototypeSelfWriteEffect(out *flow.PointState, effect WriteEffect, updated product.AbstractValue) bool {
	if !effect.RecordProto {
		return false
	}
	mutation, _ := receiverMutationForPlace(
		effect.Place,
		presentDynamicElementWritePreservesKeyPresence(effect.Place, effect.Value),
	)
	return t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, len(effect.Place.Steps) > 0, mutation)
}

func referenceEffectForWrite(effect WriteEffect) ReferenceEffect {
	return ReferenceEffect{
		Place:        effect.Place,
		Source:       effect.Source,
		FunctionRefs: effect.FunctionRefs,
		ClosureRefs:  effect.ClosureRefs,
	}
}

// WriteEffect is the canonical product-state effect of writing a value to a
// Place. It owns every PointState axis that must move with that write, so callers
// do not hand-maintain parallel Env/Cells/static-member/ref/provenance updates.
type WriteEffect struct {
	Place         Place
	Value         product.AbstractValue
	ClearValue    bool
	JoinExisting  bool
	Source        ast.Expr
	IndexTarget   cfg.AssignTarget
	DynamicMode   DynamicWriteMode
	LengthTarget  cfg.AssignTarget
	LengthBase    flow.ValueKey
	KeyArrayTable constraint.Path

	FunctionRefs functionRefsWrite
	ClosureRefs  closureRefsWrite

	KillRelations bool
	RecordProto   bool
	RecordStatic  bool
}

// MutatorEffect is the canonical product-state effect of mutating the value at a
// Place without replacing the place itself. This covers Lua table mutators such
// as `table.insert(arr, v)` and `table.insert(m[k], v)`: the reducer owns the
// value-domain mutation plus the side axes that are invalidated by that semantic
// mutation.
type MutatorEffect struct {
	Place           Place
	Kind            MutatorKind
	Element         product.AbstractValue
	ElementExpr     ast.Expr
	ElementPath     constraint.Path
	Key             product.AbstractValue
	LengthKey       constraint.PathKey
	LengthIncrement int64
}

func (t *Transfer) applyMutatorEffect(out *flow.PointState, effect MutatorEffect) bool {
	if out == nil {
		return false
	}
	keyArrayTables, pendingKeyArrayTables := t.keyArrayTablesPreservedByAppend(out, effect)
	appendHistoryArray := constraint.Path{}
	preserveAppendHistoryBase := false
	if effect.Kind == MutatorAppendElement {
		if path, ok := effect.Place.StaticPath(); ok && !path.IsEmpty() {
			appendHistoryArray = path
			if addr, ok := flow.StableAddressOfPath(path); ok {
				preserveAppendHistoryBase = out.KeyPresence.HasAppendHistoryBase(addr.Key())
			}
		}
	}
	appendDestinations := appendOriginDestinations(out, appendHistoryArray, nil)
	changed := false
	changed = t.applyPlaceMutationEffect(out, PlaceMutationEffect{
		Place:         effect.Place,
		StaticMembers: true,
		Conditions:    true,
		KeyFacts:      true,
	}) || changed
	if preserveAppendHistoryBase {
		if addr, ok := flow.StableAddressOfPath(appendHistoryArray); ok {
			changed = flow.ApplyAppendHistoryBaseProof(out, flow.AppendHistoryBaseProof{Array: addr}) || changed
		}
	}
	changed = t.recordAppendKeyFact(out, effect.Place, effect.Kind, effect.ElementPath) || changed
	changed = t.recordAppendElementFieldOrigins(out, effect.Place, effect.Kind, effect.ElementExpr, appendDestinations) || changed
	if effect.LengthKey != "" && effect.LengthIncrement > 0 {
		changed = t.incrementLenBound(out, effect.LengthKey, effect.LengthIncrement) || changed
	}
	if len(keyArrayTables) > 0 || len(pendingKeyArrayTables) > 0 {
		changed = t.applyAppendKeyArrayFacts(out, effect.Place, keyArrayTables, pendingKeyArrayTables, effect.ElementPath, effect.Element) || changed
	}
	mutation, mutationOK := receiverMutationForPlace(effect.Place, false)
	if mutationOK {
		changed = t.recordReceiverMutationEffect(out, effect.Place.Root, mutation) || changed
	}
	if effect.Place.Root == 0 || effect.Element.IsZero() {
		return changed
	}
	updated, _, ok := t.updatePlace(out, effect.Place, func(base product.AbstractValue) (product.AbstractValue, bool) {
		switch effect.Kind {
		case MutatorAppendElement:
			return product.AppendElement(base, effect.Element), true
		case MutatorAppendMapElement:
			if effect.Key.IsZero() {
				return product.AbstractValue{}, false
			}
			return product.AppendMapElement(base, effect.Key, effect.Element), true
		case MutatorContainerElementUnion:
			return product.ContainerElementUnion(base, effect.Element), true
		default:
			return product.AbstractValue{}, false
		}
	})
	if !ok {
		return changed
	}
	t.writeRootContainer(out, effect.Place.Root, updated)
	if mutationOK {
		changed = t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, true, mutation) || changed
	} else {
		changed = t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, true) || changed
	}
	return true
}

func (t *Transfer) keyArrayTablesPreservedByAppend(out *flow.PointState, effect MutatorEffect) ([]constraint.PathKey, []constraint.PathKey) {
	if out == nil || effect.Kind != MutatorAppendElement || effect.ElementPath.IsEmpty() {
		return nil, nil
	}
	arrayPath, ok := effect.Place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return nil, nil
	}
	arrayKey := flow.KeyPresencePathKey(arrayPath)
	elementKey := flow.KeyPresencePathKey(effect.ElementPath)
	if arrayKey == "" || elementKey == "" {
		return nil, nil
	}
	existingTables := out.KeyPresence.KeyArrayTables(arrayKey)
	historyEmpty := out.KeyPresence.HasAppendHistoryBase(arrayKey)
	if historyEmpty {
		for _, event := range out.KeyPresence.AppendHistoryEventEntries() {
			if event.Array == arrayKey {
				historyEmpty = false
				break
			}
		}
	}
	canSeedFromEmpty := len(existingTables) == 0 &&
		(out.KeyPresence.HasEmptyKeyArray(arrayKey) || historyEmpty || t.arrayPathCurrentlyEmpty(out, arrayPath))
	var tables []constraint.PathKey
	var pending []constraint.PathKey
	if canSeedFromEmpty {
		pending = append(pending, "")
	}
	for _, table := range existingTables {
		if out.KeyPresence.Has(table, elementKey) {
			tables = append(tables, table)
			continue
		}
		pending = append(pending, table)
	}
	for _, fact := range out.KeyPresence.Entries() {
		if fact.Key != elementKey {
			continue
		}
		if _, ok := slicesFindPathKey(existingTables, fact.Table); ok || canSeedFromEmpty {
			tables = append(tables, fact.Table)
		}
	}
	return compactPathKeys(tables), compactPathKeys(pending)
}

func (t *Transfer) recordAppendKeyFact(out *flow.PointState, place Place, kind MutatorKind, elementPath constraint.Path) bool {
	if out == nil || kind != MutatorAppendElement || elementPath.IsEmpty() {
		return false
	}
	arrayPath, ok := place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	before := out.KeyPresence
	arrayAddr, arrayOK := flow.StableAddressOfPath(arrayPath)
	elementAddr, elementOK := flow.StableAddressOfPath(elementPath)
	if arrayOK && elementOK {
		flow.ApplyAppendKeyProof(out, flow.AppendKeyProof{
			Array: arrayAddr,
			Key:   elementAddr,
		})
	}
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

func (t *Transfer) recordAppendElementFieldOrigins(out *flow.PointState, place Place, kind MutatorKind, elem ast.Expr, destinations []appendOriginDestination) bool {
	if out == nil || kind != MutatorAppendElement || elem == nil {
		return false
	}
	arrayPath, ok := place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	before := out.KeyPresence
	if len(destinations) == 0 {
		destinations = appendOriginDestinations(out, arrayPath, nil)
	}
	if table, ok := elem.(*ast.TableExpr); ok && table != nil {
		for _, field := range table.Fields {
			if field == nil || field.Value == nil {
				continue
			}
			seg, ok := fieldkey.FromTableField(field)
			if !ok {
				continue
			}
			source, ok := t.staticPathOfExpr(field.Value)
			if !ok || source.IsEmpty() {
				continue
			}
			sources := appendOriginSources(out, source)
			for _, dst := range destinations {
				field := append([]constraint.Segment(nil), dst.fieldPrefix...)
				field = append(field, seg)
				for _, src := range sources {
					arrayAddr, arrayOK := flow.StableAddressOfPath(dst.array)
					sourceAddr, sourceOK := flow.StableAddressOfPath(src.source)
					if !arrayOK || !sourceOK {
						continue
					}
					flow.ApplyAppendElementFieldOriginProof(out, flow.AppendElementFieldOriginProof{
						Array:       arrayAddr,
						Field:       field,
						Source:      sourceAddr,
						SourceField: src.sourceField,
					})
				}
			}
		}
	}
	elementPath, ok := t.staticPathOfExpr(elem)
	if !ok || elementPath.IsEmpty() {
		return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
	}
	for _, fact := range out.KeyPresence.AppendElementFieldOriginEntries() {
		field, ok := flow.AppendElementFieldSegments(fact.Field)
		if !ok || len(field) == 0 {
			continue
		}
		elementField := elementPath
		for _, seg := range field {
			elementField = elementField.Append(seg)
		}
		elementFieldAddr, ok := flow.StableAddressOfPath(elementField)
		if !ok {
			continue
		}
		for _, originUse := range out.ValueOrigins.OriginsCoveringAddress(elementFieldAddr) {
			recordAppendElementFieldOriginUse(out, destinations, field, originUse)
		}
		for _, aliasUse := range out.PathAliases.AliasesCoveringAddress(elementFieldAddr) {
			source, ok := pathFromKey(aliasUse.Alias.Source)
			if !ok {
				continue
			}
			for _, seg := range aliasUse.Remainder {
				source = source.Append(seg)
			}
			sourceAddr, ok := flow.StableAddressOfPath(source)
			if !ok {
				continue
			}
			for _, originUse := range out.ValueOrigins.OriginsCoveringAddress(sourceAddr) {
				recordAppendElementFieldOriginUse(out, destinations, field, originUse)
			}
		}
	}
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

type appendOriginDestination struct {
	array       constraint.Path
	fieldPrefix []constraint.Segment
}

type appendOriginSource struct {
	source      constraint.Path
	sourceField []constraint.Segment
}

func appendOriginDestinations(out *flow.PointState, arrayPath constraint.Path, fieldPrefix []constraint.Segment) []appendOriginDestination {
	if out == nil || arrayPath.IsEmpty() {
		return nil
	}
	seen := map[string]bool{}
	var destinations []appendOriginDestination
	var add func(constraint.Path, []constraint.Segment)
	add = func(array constraint.Path, prefix []constraint.Segment) {
		key := flow.KeyPresencePathKey(array)
		seenKey := string(key) + "/" + string(flow.AppendElementFieldPathKey(prefix))
		if key == "" || seen[seenKey] {
			return
		}
		seen[seenKey] = true
		destinations = append(destinations, appendOriginDestination{
			array:       array,
			fieldPrefix: append([]constraint.Segment(nil), prefix...),
		})
		arrayAddr, ok := flow.StableAddressOfPath(array)
		if !ok {
			return
		}
		for _, use := range out.ValueOrigins.OriginsCoveringAddress(arrayAddr) {
			if use.Origin.Kind != flow.ValueOriginIndexedIterator || use.Origin.VarIndex != 1 || len(use.Remainder) == 0 {
				continue
			}
			source, ok := pathFromKey(use.Origin.Source)
			if !ok {
				continue
			}
			nextPrefix := append([]constraint.Segment(nil), use.Remainder...)
			nextPrefix = append(nextPrefix, prefix...)
			add(source, nextPrefix)
		}
		for _, aliasUse := range out.PathAliases.AliasesCoveringAddress(arrayAddr) {
			source, ok := pathFromKey(aliasUse.Alias.Source)
			if !ok {
				continue
			}
			for _, seg := range aliasUse.Remainder {
				source = source.Append(seg)
			}
			add(source, prefix)
		}
	}
	add(arrayPath, fieldPrefix)
	return destinations
}

func appendOriginSources(out *flow.PointState, sourcePath constraint.Path) []appendOriginSource {
	if out == nil || sourcePath.IsEmpty() {
		return nil
	}
	var sources []appendOriginSource
	add := func(source constraint.Path, sourceField []constraint.Segment) {
		if source.IsEmpty() {
			return
		}
		sources = append(sources, appendOriginSource{
			source:      source,
			sourceField: append([]constraint.Segment(nil), sourceField...),
		})
	}
	add(sourcePath, nil)
	sourceAddr, ok := flow.StableAddressOfPath(sourcePath)
	if !ok {
		return sources
	}
	for _, use := range out.ValueOrigins.OriginsCoveringAddress(sourceAddr) {
		source, ok := pathFromKey(use.Origin.Source)
		if !ok {
			continue
		}
		switch use.Origin.Kind {
		case flow.ValueOriginIndexedIterator:
			if use.Origin.VarIndex == 1 && len(use.Remainder) > 0 {
				add(source, use.Remainder)
			}
		case flow.ValueOriginAssignmentAlias:
			for _, seg := range use.Remainder {
				source = source.Append(seg)
			}
			add(source, nil)
		}
	}
	for _, aliasUse := range out.PathAliases.AliasesCoveringAddress(sourceAddr) {
		source, ok := pathFromKey(aliasUse.Alias.Source)
		if !ok {
			continue
		}
		for _, seg := range aliasUse.Remainder {
			source = source.Append(seg)
		}
		add(source, nil)
	}
	return sources
}

func recordAppendElementFieldOriginUse(
	out *flow.PointState,
	destinations []appendOriginDestination,
	field []constraint.Segment,
	originUse flow.ValueOriginUse,
) {
	if out == nil {
		return
	}
	if originUse.Origin.Kind != flow.ValueOriginIndexedIterator || originUse.Origin.VarIndex != 1 || len(originUse.Remainder) == 0 {
		return
	}
	for _, sourceUse := range out.KeyPresence.AppendElementFieldSources(originUse.Origin.Source, originUse.Remainder) {
		source, ok := pathFromKey(sourceUse.Origin.Source)
		if !ok {
			continue
		}
		sourceField := append([]constraint.Segment(nil), sourceUse.SourceField...)
		if len(sourceField) > 0 {
			sourceField = append(sourceField, sourceUse.FieldRemainder...)
		} else {
			for _, seg := range sourceUse.FieldRemainder {
				source = source.Append(seg)
			}
		}
		for _, dst := range destinations {
			dstField := append([]constraint.Segment(nil), dst.fieldPrefix...)
			dstField = append(dstField, field...)
			arrayAddr, arrayOK := flow.StableAddressOfPath(dst.array)
			sourceAddr, sourceOK := flow.StableAddressOfPath(source)
			if !arrayOK || !sourceOK {
				continue
			}
			flow.ApplyAppendElementFieldOriginProof(out, flow.AppendElementFieldOriginProof{
				Array:       arrayAddr,
				Field:       dstField,
				Source:      sourceAddr,
				SourceField: sourceField,
			})
		}
	}
}

func pathFromKey(key constraint.PathKey) (constraint.Path, bool) {
	sym, segments, ok := flow.ParseSymbolPathKey(key)
	if !ok || sym == 0 {
		return constraint.Path{}, false
	}
	return constraint.Path{Symbol: sym, Segments: segments}, true
}

func (t *Transfer) arrayPathCurrentlyEmpty(out *flow.PointState, path constraint.Path) bool {
	if out == nil || path.IsEmpty() {
		return false
	}
	av, ok := flow.PointFactsOf(*out).PathValue(path)
	if !ok || av.IsZero() || !av.DefinitelyPresent() {
		return false
	}
	return productValueIsFreshEmptySequence(av)
}

func productValueIsFreshEmptySequence(av product.AbstractValue) bool {
	t := unwrap.Alias(av.ProjectValue())
	switch v := t.(type) {
	case *typ.Array:
		return v.Fresh || typ.IsNever(v.Element)
	case *typ.Record:
		return len(v.Fields) == 0 &&
			len(v.StaticMembers) == 0 &&
			!v.HasMapComponent() &&
			!v.Open &&
			v.Metatable == nil
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !productTypeIsFreshEmptySequence(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func productTypeIsFreshEmptySequence(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return v.Fresh || typ.IsNever(v.Element)
	case *typ.Record:
		return len(v.Fields) == 0 &&
			len(v.StaticMembers) == 0 &&
			!v.HasMapComponent() &&
			!v.Open &&
			v.Metatable == nil
	default:
		return false
	}
}

func (t *Transfer) applyAppendKeyArrayFacts(
	out *flow.PointState,
	place Place,
	tables []constraint.PathKey,
	pendingTables []constraint.PathKey,
	elementPath constraint.Path,
	element product.AbstractValue,
) bool {
	if out == nil || (len(tables) == 0 && len(pendingTables) == 0) {
		return false
	}
	arrayPath, ok := place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	arrayAddr, arrayOK := flow.StableAddressOfPath(arrayPath)
	if !arrayOK {
		return false
	}
	elementAddr, elementOK := flow.StableAddressOfPath(elementPath)
	before := out.KeyPresence
	for _, table := range tables {
		tableAddr, ok := flow.StableAddressFromKey(table)
		if !ok {
			continue
		}
		flow.ApplyKeyArrayProof(out, flow.KeyArrayProof{
			Array: arrayAddr,
			Table: tableAddr,
		})
		if elementPath.IsEmpty() {
			continue
		}
		tablePath, ok := indexWritePathFromKey(table)
		if !ok || tablePath.IsEmpty() {
			continue
		}
		keyType := product.ProjectValueOrUnknown(element)
		if keyType == nil {
			keyType = typ.Unknown
		}
		tableAddr, tableOK := flow.StableAddressOfPath(tablePath)
		keyAddr, keyOK := flow.StableAddressOfPath(elementPath)
		if !tableOK || !keyOK {
			continue
		}
		value, ok := out.IndexWrites.AdmissionAtAddress(flow.IndexWriteAddressQuery{
			Target:     tableAddr,
			KeyPath:    keyAddr,
			HasKeyPath: true,
			KeyValue:   product.FromType(keyType),
		})
		if !ok || value.IsZero() {
			continue
		}
		flow.ApplyKeyArrayValueProof(out, flow.KeyArrayValueProof{
			Array:        arrayAddr,
			Table:        tableAddr,
			Value:        value,
			AppendKey:    elementAddr,
			HasAppendKey: elementOK,
		})
	}
	for _, table := range pendingTables {
		proof := flow.PendingKeyArrayProof{
			Array: arrayAddr,
			Key:   elementAddr,
		}
		if table != "" {
			tableAddr, ok := flow.StableAddressFromKey(table)
			if !ok {
				continue
			}
			proof.Table = tableAddr
			proof.HasTable = true
		}
		flow.ApplyPendingKeyArrayProof(out, proof)
	}
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

func slicesFindPathKey(xs []constraint.PathKey, want constraint.PathKey) (int, bool) {
	for i, x := range xs {
		if x == want {
			return i, true
		}
	}
	return -1, false
}

func compactPathKeys(xs []constraint.PathKey) []constraint.PathKey {
	if len(xs) == 0 {
		return nil
	}
	out := xs[:0]
	for _, x := range xs {
		if _, ok := slicesFindPathKey(out, x); ok {
			continue
		}
		out = append(out, x)
	}
	return append([]constraint.PathKey(nil), out...)
}

func (t *Transfer) applyWriteEffect(out *flow.PointState, effect WriteEffect) bool {
	return t.applyWriteEffectWithAliasReplay(out, effect, true)
}

func (t *Transfer) applyWriteEffectWithAliasReplay(out *flow.PointState, effect WriteEffect, replayAliases bool) bool {
	if out == nil || effect.Place.Root == 0 {
		return false
	}
	aliasWrites := []WriteEffect(nil)
	if replayAliases {
		aliasWrites = t.aliasReplayWriteEffects(out, effect)
	}
	changed := false
	if effect.KillRelations && len(effect.Place.Steps) == 0 {
		changed = t.applyRelationEffect(out, RelationEffect{Kind: RelationKillSymbols, Symbols: []cfg.SymbolID{effect.Place.Root}}) || changed
	} else if len(effect.Place.Steps) > 0 {
		changed = t.applyRelationEffect(out, RelationEffect{Kind: RelationKillLengthTargets, Symbols: []cfg.SymbolID{effect.Place.Root}}) || changed
	}
	changed = t.applyPlaceMutationEffect(out, PlaceMutationEffect{
		Place:                  effect.Place,
		StaticMembers:          effect.RecordStatic,
		Conditions:             true,
		KeyFacts:               true,
		PresentElementKeyFacts: presentDynamicElementWritePreservesKeyPresence(effect.Place, effect.Value),
		PresentElementValue:    effect.Value,
	}) || changed
	if len(effect.Place.Steps) > 0 {
		if mutation, ok := receiverMutationForPlace(
			effect.Place,
			presentDynamicElementWritePreservesKeyPresence(effect.Place, effect.Value),
		); ok {
			changed = t.recordReceiverMutationEffect(out, effect.Place.Root, mutation) || changed
		}
	}
	if effect.RecordStatic && !effect.ClearValue {
		t.installStaticMemberWriteFactForPlace(out, effect.Place, effect.Value)
	}
	t.seedKeyArrayForWriteEffect(out, effect)
	changed = t.seedEmptyContainerKeyArraysForWriteEffect(out, effect) || changed
	if effect.LengthBase != "" {
		t.applyIndexWriteLength(out, effect.LengthTarget, effect.LengthBase)
	}
	if len(effect.Place.Steps) == 0 {
		changed = t.applyRootWriteEffect(out, effect) || changed
		return t.applyAliasReplayWriteEffects(out, aliasWrites) || changed
	}
	if effect.ClearValue || effect.Value.IsZero() {
		changed = t.applyReferenceEffect(out, referenceEffectForWrite(effect)) || changed
		return t.applyAliasReplayWriteEffects(out, aliasWrites) || changed
	}
	admittedIndexKey := product.AbstractValue{}
	admittedIndexValue := product.AbstractValue{}
	sealedIndexTarget := false
	if targetPath, ok := indexWriteTargetPath(effect.Place); ok {
		sealedIndexTarget = t.indexWriteTargetSealed(targetPath)
	}
	if !sealedIndexTarget && effect.IndexTarget.Kind == cfg.TargetIndex {
		symbolicChanged := t.applySymbolicDynamicIndexWriteProof(out, effect.IndexTarget, effect.Source, effect.Value)
		changed = symbolicChanged || changed
	}
	updated, _, ok := t.assignPlaceValue(out, effect.Place, effect.Value, func(base product.AbstractValue, step PlaceStep, val product.AbstractValue) (product.AbstractValue, bool) {
		if sealedIndexTarget {
			if !product.SealedIndexWriteAdmits(base, step.Key, val) {
				return product.AbstractValue{}, false
			}
			written := product.WriteIndex(base, step.Key, val)
			admittedIndexKey = step.Key
			admittedIndexValue = indexWriteReadBackValue(written, step.Key, val)
			return written, true
		}
		if effect.DynamicMode == DynamicWriteSelfDerived {
			written := product.WriteSelfDerivedIndex(base, step.Key, val)
			admittedIndexKey = step.Key
			admittedIndexValue = val
			return written, true
		}
		written := product.WriteIndexForeign(base, step.Key, val)
		if product.IndexWriteAdmits(base, step.Key, val) {
			admittedIndexKey = step.Key
			admittedIndexValue = val
		}
		return written, true
	})
	if !ok {
		return t.applyAliasReplayWriteEffects(out, aliasWrites) || changed
	}
	if sealedIndexTarget && effect.IndexTarget.Kind == cfg.TargetIndex {
		symbolicChanged := t.applySymbolicDynamicIndexWriteProof(out, effect.IndexTarget, effect.Source, effect.Value)
		changed = symbolicChanged || changed
	}
	if proof, ok := t.dynamicIndexWriteProofEffect(effect, admittedIndexKey, admittedIndexValue); ok {
		changed = t.applyDynamicIndexWriteProofEffect(out, proof) || changed
	}
	t.writeRootContainer(out, effect.Place.Root, updated)
	t.applyPrototypeSelfWriteEffect(out, effect, updated)
	t.applyReferenceEffect(out, referenceEffectForWrite(effect))
	changed = true
	return t.applyAliasReplayWriteEffects(out, aliasWrites) || changed
}

func (t *Transfer) aliasReplayWriteEffects(out *flow.PointState, effect WriteEffect) []WriteEffect {
	if out == nil || out.ValueOrigins.IsBottom() {
		return nil
	}
	path, ok := effect.Place.StaticPath()
	if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
		return nil
	}
	var outEffects []WriteEffect
	seen := map[constraint.PathKey]struct{}{}
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		return nil
	}
	for _, use := range out.ValueOrigins.OriginsCoveringAddress(addr) {
		if use.Origin.Kind != flow.ValueOriginAssignmentAlias || len(use.Remainder) == 0 {
			continue
		}
		source, ok := indexWritePathFromKey(use.Origin.Source)
		if !ok || source.IsEmpty() {
			continue
		}
		for _, seg := range use.Remainder {
			source = source.Append(seg)
		}
		if source.Equal(path) {
			continue
		}
		sourceKey := flow.SymbolPathKey(source.Symbol, source.Segments)
		if sourceKey == "" {
			continue
		}
		if _, ok := seen[sourceKey]; ok {
			continue
		}
		seen[sourceKey] = struct{}{}
		place, ok := staticPlace(source.Symbol, source.Segments)
		if !ok || place.Root == 0 || len(place.Steps) == 0 {
			continue
		}
		outEffects = append(outEffects, WriteEffect{
			Place:         place,
			Value:         effect.Value,
			ClearValue:    effect.ClearValue,
			JoinExisting:  effect.JoinExisting,
			Source:        effect.Source,
			FunctionRefs:  effect.FunctionRefs,
			ClosureRefs:   effect.ClosureRefs,
			KillRelations: effect.KillRelations,
			RecordProto:   effect.RecordProto,
			RecordStatic:  effect.RecordStatic,
		})
	}
	return outEffects
}

func (t *Transfer) applyAliasReplayWriteEffects(out *flow.PointState, effects []WriteEffect) bool {
	changed := false
	for _, effect := range effects {
		changed = t.applyWriteEffectWithAliasReplay(out, effect, false) || changed
	}
	return changed
}

func indexWriteReadBackValue(container, key, written product.AbstractValue) product.AbstractValue {
	if container.IsZero() || key.IsZero() {
		return written
	}
	read, ok := product.RuntimeIndexOf(container, key)
	if !ok || read.IsZero() {
		return written
	}
	if written.DefinitelyPresent() {
		present := product.NarrowPresent(read)
		if !present.IsZero() {
			return present
		}
	}
	return read
}

func (t *Transfer) applySymbolWriteEffect(
	out *flow.PointState,
	target cfg.AssignTarget,
	val product.AbstractValue,
	clear bool,
	joinExisting bool,
	src ast.Expr,
	keyArrayTable constraint.Path,
	functionRefs functionRefsWrite,
	closureRefs closureRefsWrite,
) bool {
	if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
		return false
	}
	changed := t.applyWriteEffect(out, WriteEffect{
		Place:         Place{Root: target.Symbol},
		Value:         val,
		ClearValue:    clear,
		JoinExisting:  joinExisting,
		Source:        src,
		KeyArrayTable: keyArrayTable,
		FunctionRefs:  functionRefs,
		ClosureRefs:   closureRefs,
		KillRelations: true,
		RecordProto:   true,
		RecordStatic:  true,
	})
	if t.applySetMetatableInstanceBinding(out, src, target.Symbol) {
		changed = true
	}
	return changed
}

func (t *Transfer) applyRootWriteEffect(out *flow.PointState, effect WriteEffect) bool {
	if effect.ClearValue {
		t.clearPrototypeInstance(out, effect.Place.Root)
		t.symbolStorage.clear(out, effect.Place.Root, effect.JoinExisting, true)
	} else {
		if effect.Value.IsZero() {
			return t.applyReferenceEffect(out, referenceEffectForWrite(effect))
		}
		if _, ok := t.setMetatablePrototypeFromSource(effect.Source); !ok {
			t.clearPrototypeInstance(out, effect.Place.Root)
		}
		t.writeSymbolValue(out, effect.Place.Root, effect.Value, effect.JoinExisting, true)
		t.applyPrototypeSelfWriteEffect(out, effect, effect.Value)
	}
	t.applyReferenceEffect(out, referenceEffectForWrite(effect))
	return true
}

func (t *Transfer) seedKeyArrayForWriteEffect(out *flow.PointState, effect WriteEffect) {
	if out == nil || effect.KeyArrayTable.IsEmpty() || len(effect.Place.Steps) != 0 {
		return
	}
	path, ok := effect.Place.StaticPath()
	if !ok {
		return
	}
	arrayAddr, arrayOK := flow.StableAddressOfPath(path)
	tableAddr, tableOK := flow.StableAddressOfPath(effect.KeyArrayTable)
	if arrayOK && tableOK {
		flow.ApplyKeyArrayProof(out, flow.KeyArrayProof{
			Array: arrayAddr,
			Table: tableAddr,
		})
	}
}

func (t *Transfer) seedEmptyContainerKeyArraysForWriteEffect(out *flow.PointState, effect WriteEffect) bool {
	if out == nil || effect.ClearValue || effect.Value.IsZero() || len(effect.Place.Steps) != 0 {
		return false
	}
	if !emptyContainerKeyArraySeedSource(effect.Source) {
		return false
	}
	rootPath, ok := effect.Place.StaticPath()
	if !ok || rootPath.Symbol == 0 {
		return false
	}
	rec := unwrap.Record(effect.Value.ProjectValue())
	if rec == nil {
		return false
	}
	arrays := emptyContainerKeyArraySeedFields(effect.Value, rec)
	if len(arrays) == 0 {
		return false
	}
	before := out.KeyPresence
	for _, array := range arrays {
		if addr, ok := flow.StableAddressOfPath(rootPath.Field(array)); ok {
			flow.ApplyEmptyKeyArrayProof(out, flow.EmptyKeyArrayProof{Array: addr})
		}
	}
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

func emptyContainerKeyArraySeedSource(src ast.Expr) bool {
	switch src.(type) {
	case *ast.TableExpr, *ast.FuncCallExpr:
		return true
	default:
		return false
	}
}

func emptyContainerKeyArraySeedFields(root product.AbstractValue, rec *typ.Record) []string {
	if root.IsZero() || rec == nil {
		return nil
	}
	var arrays []string
	for _, field := range rec.Fields {
		fieldValue, ok := product.FieldOf(root, field.Name)
		if !ok || fieldValue.IsZero() || !fieldValue.DefinitelyPresent() {
			continue
		}
		if productValueIsFreshEmptyArray(fieldValue) {
			arrays = append(arrays, field.Name)
		}
	}
	return arrays
}

func productValueIsFreshEmptyArray(av product.AbstractValue) bool {
	if av.IsZero() || !av.DefinitelyPresent() {
		return false
	}
	return productTypeIsFreshEmptyArray(av.ProjectValue())
}

func productTypeIsFreshEmptyArray(t typ.Type) bool {
	switch v := unwrap.Alias(t).(type) {
	case *typ.Array:
		return v.Fresh || typ.IsNever(v.Element)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return productTypeIsFreshEmptyArray(v.Body)
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, member := range v.Members {
			if !productTypeIsFreshEmptyArray(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
