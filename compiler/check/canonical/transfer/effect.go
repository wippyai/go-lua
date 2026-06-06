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
	return flow.ApplyClosureCellEffectsToRefsPath(out, effect.ClosurePath, effect.Effects)
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
	flow.ApplyCaptureEffectsToCellStore(out, effects)
}

func (t *Transfer) recordCellEffects(out *flow.PointState, effects flow.CaptureEffects) {
	flow.RecordCaptureEffects(out, effects)
}

func (t *Transfer) recordReceiverEffect(
	out *flow.PointState,
	slot int,
	value product.AbstractValue,
	mutations ...flow.ReceiverMutation,
) bool {
	return flow.RecordReceiverWrite(out, slot, value, mutations...)
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
	return flow.RecordReceiverMutation(out, slot, mutations...)
}

func receiverMutationForPlace(place Place, presentElementWrite bool) (flow.ReceiverMutation, bool) {
	footprint, ok := place.WriteFootprint(presentElementWrite, product.AbstractValue{})
	if !ok {
		return flow.ReceiverMutation{}, false
	}
	return flow.ReceiverMutationFromAccessFootprint(footprint)
}

func (t *Transfer) applyReturnEffect(out *flow.PointState, effect ReturnEffect) bool {
	if out == nil {
		return false
	}
	changed := flow.SetReturnRelations(out, effect.Relations)
	for _, slot := range effect.Slots {
		if slot.Index < 0 {
			continue
		}
		changed = flow.ClearReturnSlotValue(out, slot.Index) || changed
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
		changed = flow.WriteReturnSlotValue(out, slot.Index, slot.Value) || changed
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
			return flow.ClearFunctionRefSubtreePath(out, path)
		}
		t.recordFunctionRefAt(out, path, src)
	case referenceWriteExplicit:
		return flow.ReplaceFunctionRefSubtreePath(out, path, write.Refs)
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
			return flow.ClearClosureRefSubtreePath(out, path)
		}
		t.recordClosureRefAt(out, path, src)
	case referenceWriteExplicit:
		return flow.ReplaceClosureRefSubtreePath(out, path, write.Refs)
	default:
		return false
	}
	return !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs)
}

func (t *Transfer) applyPrototypeSelfEffect(out *flow.PointState, effect PrototypeSelfEffect) bool {
	return flow.RecordPrototypeSelf(out, effect.Prototype, effect.Value)
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
	keyArraySelection := t.keyArraySelectionPreservedByAppend(out, effect)
	appendHistoryArray := constraint.Path{}
	preserveAppendHistoryBase := false
	if effect.Kind == MutatorAppendElement {
		if path, ok := effect.Place.StaticPath(); ok && !path.IsEmpty() {
			appendHistoryArray = path
			preserveAppendHistoryBase = flow.PointFactsOf(*out).HasAppendHistoryBase(path)
		}
	}
	appendDestinations := flow.AppendOriginDestinationsPath(*out, appendHistoryArray, nil)
	changed := false
	changed = t.applyPlaceMutationEffect(out, PlaceMutationEffect{
		Place:         effect.Place,
		StaticMembers: true,
		Conditions:    true,
		KeyFacts:      true,
	}) || changed
	if preserveAppendHistoryBase {
		changed = flow.ApplyAppendHistoryBasePathProof(out, appendHistoryArray) || changed
	}
	changed = t.recordAppendKeyFact(out, effect.Place, effect.Kind, effect.ElementPath) || changed
	changed = t.recordAppendElementFieldOrigins(out, effect.Place, effect.Kind, effect.ElementExpr, appendDestinations) || changed
	if effect.LengthKey != "" && effect.LengthIncrement > 0 {
		changed = t.incrementLenBound(out, effect.LengthKey, effect.LengthIncrement) || changed
	}
	if len(keyArraySelection.Tables) > 0 || len(keyArraySelection.Pending) > 0 {
		changed = t.applyAppendKeyArrayFacts(out, effect.Place, keyArraySelection, effect.ElementPath, effect.Element) || changed
	}
	mutation, mutationOK := receiverMutationForPlace(effect.Place, false)
	if mutationOK {
		changed = t.recordReceiverMutationEffect(out, effect.Place.Root, mutation) || changed
	}
	if effect.Place.Root == 0 || effect.Element.IsZero() {
		return changed
	}
	updated, ok := t.placeWriter().Update(out, effect.Place, func(base product.AbstractValue) (product.AbstractValue, bool) {
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
	if mutationOK {
		changed = t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, true, mutation) || changed
	} else {
		changed = t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, true) || changed
	}
	return true
}

func (t *Transfer) keyArraySelectionPreservedByAppend(out *flow.PointState, effect MutatorEffect) flow.AppendKeyArraySelection {
	if out == nil || effect.Kind != MutatorAppendElement || effect.ElementPath.IsEmpty() {
		return flow.AppendKeyArraySelection{}
	}
	arrayPath, ok := effect.Place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return flow.AppendKeyArraySelection{}
	}
	freshEmptySeed := flow.PointFactsOf(*out).HasEmptyKeyArray(arrayPath) ||
		flow.AppendHistoryBaseWithoutEventsPath(*out, arrayPath) ||
		t.arrayPathCurrentlyEmpty(out, arrayPath)
	return flow.AppendKeyArrayPathPreservation(*out, flow.AppendKeyArrayPathPreservationQuery{
		ArrayPath:      arrayPath,
		KeyPath:        effect.ElementPath,
		FreshEmptySeed: freshEmptySeed,
	})
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
	flow.ApplyAppendKeyPathProof(out, flow.AppendKeyPathProof{
		ArrayPath: arrayPath,
		KeyPath:   elementPath,
	})
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}

func (t *Transfer) recordAppendElementFieldOrigins(out *flow.PointState, place Place, kind MutatorKind, elem ast.Expr, destinations []flow.AppendOriginDestination) bool {
	if out == nil || kind != MutatorAppendElement || elem == nil {
		return false
	}
	arrayPath, ok := place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	before := out.KeyPresence
	if len(destinations) == 0 {
		destinations = flow.AppendOriginDestinationsPath(*out, arrayPath, nil)
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
			sources := flow.AppendOriginSourcesPath(*out, source)
			for _, dst := range destinations {
				field := append([]constraint.Segment(nil), dst.FieldPrefix...)
				field = append(field, seg)
				for _, src := range sources {
					flow.ApplyAppendElementFieldOriginProof(out, flow.AppendElementFieldOriginProof{
						Array:       dst.Array,
						Field:       field,
						Source:      src.Source,
						SourceField: src.SourceField,
					})
				}
			}
		}
	}
	elementPath, ok := t.staticPathOfExpr(elem)
	if !ok || elementPath.IsEmpty() {
		return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
	}
	for _, field := range flow.AppendElementFieldOriginFields(*out) {
		elementField := elementPath
		for _, seg := range field {
			elementField = elementField.Append(seg)
		}
		for _, originUse := range flow.AppendElementFieldOriginUsesPath(*out, elementField) {
			flow.ApplyAppendElementFieldOriginUse(out, destinations, field, originUse)
		}
	}
	return !flow.KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
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
	selection flow.AppendKeyArraySelection,
	elementPath constraint.Path,
	element product.AbstractValue,
) bool {
	if out == nil || (len(selection.Tables) == 0 && len(selection.Pending) == 0) {
		return false
	}
	arrayPath, ok := place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return false
	}
	return flow.ApplyAppendKeyArrayPathConsequences(out, flow.AppendKeyArrayPathConsequences{
		ArrayPath: arrayPath,
		KeyPath:   elementPath,
		HasKey:    !elementPath.IsEmpty(),
		KeyValue:  element,
		Tables:    selection.Tables,
		Pending:   selection.Pending,
	})
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
		changed = flow.ApplyRelationEffect(out, flow.RelationEffect{Kind: flow.RelationKillSymbols, Symbols: []cfg.SymbolID{effect.Place.Root}}) || changed
	} else if len(effect.Place.Steps) > 0 {
		changed = flow.ApplyRelationEffect(out, flow.RelationEffect{Kind: flow.RelationKillLengthTargets, Symbols: []cfg.SymbolID{effect.Place.Root}}) || changed
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
	t.applyIndexWriteLength(out, effect.LengthTarget)
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
	if targetPath, ok := effect.Place.FinalDynamicIndexTargetPath(); ok {
		sealedIndexTarget = t.indexWriteTargetSealed(targetPath)
	}
	if !sealedIndexTarget && effect.IndexTarget.Kind == cfg.TargetIndex {
		symbolicChanged := t.applySymbolicDynamicIndexWriteProof(out, effect.IndexTarget, effect.Source, effect.Value)
		changed = symbolicChanged || changed
	}
	updated, ok := t.placeWriter().Assign(out, effect.Place, effect.Value, func(base product.AbstractValue, step PlaceStep, val product.AbstractValue) (product.AbstractValue, bool) {
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
	if proof, ok := t.dynamicIndexWriteProof(effect, admittedIndexKey, admittedIndexValue); ok {
		changed = flow.ApplyMapWriteProof(out, proof) || changed
	}
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
	facts := flow.PointFactsOf(*out)
	for _, source := range facts.IdentityAliasSourcePaths(path, flow.IdentityAliasDescendantOriginPolicy) {
		if source.IsEmpty() || len(source.Segments) == 0 {
			continue
		}
		if source.Equal(path) {
			continue
		}
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
	flow.ApplyKeyArrayPathProof(out, path, effect.KeyArrayTable)
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
		flow.ApplyEmptyKeyArrayPathProof(out, rootPath.Field(array))
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
