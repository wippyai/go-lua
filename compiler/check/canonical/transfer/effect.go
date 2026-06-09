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
	referenceWriteTree
)

type referenceWrite struct {
	FunctionMode referenceWriteMode
	FunctionTree flow.FunctionRefTree
	ClosureMode  referenceWriteMode
	ClosureTree  flow.ClosureRefTree
}

// ReturnSlotEffect is the transfer payload for one caller-visible return slot.
// Source records function/closure identity facts under the slot placeholder;
// FunctionRefTree installs normalized relative identity surfaces such as
// prototype methods under the same placeholder;
// Value records the product value for non-symbol expressions whose value would
// otherwise be lost before summary projection.
type ReturnSlotEffect struct {
	Index              int
	Source             ast.Expr
	Value              product.AbstractValue
	FunctionRefTree    flow.FunctionRefTree
	HasFunctionRefTree bool
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
// clear stale facts at the target. Tree-mode effects install a normalized
// relative identity surface. Place targets use exact static paths when available
// and fall back to root-subtree invalidation for dynamic places.
type ReferenceEffect struct {
	Place      Place
	Path       constraint.Path
	Source     ast.Expr
	References referenceWrite
}

func sourceReferenceWrite() referenceWrite {
	return referenceWrite{
		FunctionMode: referenceWriteFromSource,
		ClosureMode:  referenceWriteFromSource,
	}
}

func (write referenceWrite) WithFunctionTree(tree flow.FunctionRefTree) referenceWrite {
	write.FunctionMode = referenceWriteTree
	write.FunctionTree = tree
	return write
}

func (write referenceWrite) WithClosureTree(tree flow.ClosureRefTree) referenceWrite {
	write.ClosureMode = referenceWriteTree
	write.ClosureTree = tree
	return write
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
		if out.Cells.HasFiniteEntries() {
			return effects
		}
		return flow.CaptureEffectsDomain.Bottom()
	}
	filtered := effects.FilterSymbols(func(sym cfg.SymbolID) bool {
		return t.symbolStorage.hasCellEffectTarget(out, sym)
	})
	if flow.CaptureEffectsDomain.Equal(filtered, flow.CaptureEffectsIdentity()) {
		return flow.CaptureEffectsDomain.Bottom()
	}
	return filtered
}

func (t *Transfer) applyCellStoreEffects(out *flow.PointState, effects flow.CaptureEffects) {
	flow.ApplyCaptureEffectsToCellStore(out, effects)
}

func (t *Transfer) recordCellEffects(out *flow.PointState, effects flow.CaptureEffects) {
	flow.RecordCaptureEffects(out, effects)
}

func (t *Transfer) recordReceiverMutationEffect(
	out *flow.PointState,
	sym cfg.SymbolID,
	mutations ...flow.ReceiverMutation,
) bool {
	if out == nil || sym == 0 || len(mutations) == 0 {
		return false
	}
	param, ok := t.params.Lookup(sym)
	slot := param.Index
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
				Path:       constraint.NewPlaceholder(slot.Index),
				Source:     slot.Source,
				References: sourceReferenceWrite(),
			}) || changed
		}
		if slot.HasFunctionRefTree {
			changed = flow.ReplaceFunctionRefTreePath(out, constraint.NewPlaceholder(slot.Index), slot.FunctionRefTree) || changed
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
	return t.applyReferenceWrite(out, path, exact, effect.Source, effect.References)
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

func (t *Transfer) applyReferenceWrite(
	out *flow.PointState,
	path constraint.Path,
	exact bool,
	src ast.Expr,
	write referenceWrite,
) bool {
	changed := false
	switch write.FunctionMode {
	case referenceWritePreserve:
		// No function identity change for this write.
	case referenceWriteFromSource:
		if !exact {
			changed = flow.ClearFunctionRefSubtreePath(out, path) || changed
		} else {
			before := out.FunctionRefs
			t.recordFunctionRefAt(out, path, src)
			changed = !flow.FunctionRefsDomain.Equal(before, out.FunctionRefs) || changed
		}
	case referenceWriteTree:
		if !exact {
			changed = flow.ClearFunctionRefSubtreePath(out, path) || changed
		} else {
			changed = flow.ReplaceFunctionRefTreePath(out, path, write.FunctionTree) || changed
		}
	}
	switch write.ClosureMode {
	case referenceWritePreserve:
		// No closure identity change for this write.
	case referenceWriteFromSource:
		if !exact {
			changed = flow.ClearClosureRefSubtreePath(out, path) || changed
		} else {
			before := out.ClosureRefs
			t.recordClosureRefAt(out, path, src)
			changed = !flow.ClosureRefsDomain.Equal(before, out.ClosureRefs) || changed
		}
	case referenceWriteTree:
		if !exact {
			changed = flow.ClearClosureRefSubtreePath(out, path) || changed
		} else {
			changed = flow.ReplaceClosureRefTreePath(out, path, write.ClosureTree) || changed
		}
	}
	return changed
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
		Place:      effect.Place,
		Source:     effect.Source,
		References: effect.References,
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

	References referenceWrite

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
	LengthRef       flow.ContainerRef
	LengthIncrement int64
}

func (t *Transfer) applyMutatorEffect(out *flow.PointState, effect MutatorEffect) bool {
	if out == nil {
		return false
	}
	changed := false
	if effect.Kind == MutatorAppendElement {
		changed = t.applyAppendElementMutationFacts(out, effect) || changed
	} else {
		changed = t.applyPlaceMutationEffect(out, PlaceMutationEffect{
			Place:         effect.Place,
			StaticMembers: true,
			Conditions:    true,
			KeyFacts:      true,
		}) || changed
	}
	if effect.LengthRef.IsValid() && effect.LengthIncrement > 0 {
		changed = t.incrementLenBound(out, effect.LengthRef, effect.LengthIncrement) || changed
	}
	mutation, mutationOK := receiverMutationForPlace(effect.Place, false)
	if mutationOK {
		changed = t.recordReceiverMutationEffect(out, effect.Place.Root, mutation) || changed
	}
	if effect.Place.Root == 0 || effect.Element.IsZero() {
		return changed
	}
	var updatedPlace product.AbstractValue
	updated, ok := t.placeWriter().Update(out, effect.Place, func(base product.AbstractValue) (product.AbstractValue, bool) {
		var next product.AbstractValue
		var ok bool
		switch effect.Kind {
		case MutatorAppendElement:
			next, ok = product.AppendElement(base, effect.Element), true
		case MutatorAppendMapElement:
			if effect.Key.IsZero() {
				return product.AbstractValue{}, false
			}
			next, ok = product.AppendMapElement(base, effect.Key, effect.Element), true
		case MutatorContainerElementUnion:
			next, ok = product.ContainerElementUnion(base, effect.Element), true
		default:
			return product.AbstractValue{}, false
		}
		if ok {
			updatedPlace = next
		}
		return next, ok
	})
	if !ok {
		return changed
	}
	if path, pathOK := effect.Place.StaticPath(); pathOK && len(path.Segments) > 0 && !updatedPlace.IsZero() {
		changed = flow.SetStaticMemberPath(out, path, updatedPlace) || changed
	}
	if mutationOK {
		changed = t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, true, mutation) || changed
	} else {
		changed = t.recordPrototypeSelfWrite(out, effect.Place.Root, updated, true) || changed
	}
	return true
}

func (t *Transfer) applyAppendElementMutationFacts(out *flow.PointState, effect MutatorEffect) bool {
	if out == nil {
		return false
	}
	arrayPath, ok := effect.Place.StaticPath()
	if !ok || arrayPath.IsEmpty() {
		return t.applyPlaceMutationEffect(out, PlaceMutationEffect{
			Place:         effect.Place,
			StaticMembers: true,
			Conditions:    true,
			KeyFacts:      true,
		})
	}
	footprint, ok := effect.Place.WriteFootprint(false, product.AbstractValue{})
	if !ok {
		return t.applyPlaceMutationEffect(out, PlaceMutationEffect{
			Place:         effect.Place,
			StaticMembers: true,
			Conditions:    true,
			KeyFacts:      true,
		})
	}
	return flow.ApplyAppendElementMutationPathTransaction(out, flow.AppendElementMutationPathTransaction{
		Footprint:    footprint,
		ArrayPath:    arrayPath,
		ElementPath:  effect.ElementPath,
		ElementValue: effect.Element,
		FieldSources: t.appendElementFieldOriginSources(effect.ElementExpr),
	})
}

func (t *Transfer) appendElementFieldOriginSources(elem ast.Expr) []flow.AppendElementFieldOriginPathSource {
	table, ok := elem.(*ast.TableExpr)
	if !ok || table == nil {
		return nil
	}
	var out []flow.AppendElementFieldOriginPathSource
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
		out = append(out, flow.AppendElementFieldOriginPathSource{
			Field:      []constraint.Segment{seg},
			SourcePath: source,
		})
	}
	return out
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
		if targetPath, ok := effect.Place.FinalDynamicIndexTargetPath(); ok {
			if tx, ok := t.dynamicIndexWritePathTransaction(
				out,
				effect.IndexTarget,
				effect.Source,
				targetPath,
				product.FromType(typ.Unknown),
				effect.Value,
				effect.Value,
			); ok && !tx.KeyPath.IsEmpty() {
				changed = flow.ApplyDynamicIndexWritePathTransaction(out, tx) || changed
			}
		}
	}
	updated, ok := t.placeWriter().Assign(out, effect.Place, effect.Value, func(base product.AbstractValue, step PlaceStep, val product.AbstractValue) (product.AbstractValue, bool) {
		if sealedIndexTarget {
			if !product.SealedIndexWriteAdmits(base, step.Key, val) {
				return product.AbstractValue{}, false
			}
			written := product.WriteIndex(base, step.Key, val)
			admittedIndexKey = step.Key
			admittedIndexValue = flow.DynamicIndexWriteReadbackValue(written, step.Key, val)
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
		if targetPath, ok := effect.Place.FinalDynamicIndexTargetPath(); ok {
			if tx, ok := t.dynamicIndexWritePathTransaction(
				out,
				effect.IndexTarget,
				effect.Source,
				targetPath,
				product.FromType(typ.Unknown),
				effect.Value,
				effect.Value,
			); ok && !tx.KeyPath.IsEmpty() {
				changed = flow.ApplyDynamicIndexWritePathTransaction(out, tx) || changed
			}
		}
	}
	if targetPath, ok := effect.Place.FinalDynamicIndexTargetPath(); ok {
		if tx, ok := t.dynamicIndexWritePathTransaction(
			out,
			effect.IndexTarget,
			effect.Source,
			targetPath,
			admittedIndexKey,
			effect.Value,
			admittedIndexValue,
		); ok {
			changed = flow.ApplyDynamicIndexWritePathTransaction(out, tx) || changed
		}
	}
	t.applyPrototypeSelfWriteEffect(out, effect, updated)
	t.applyReferenceEffect(out, referenceEffectForWrite(effect))
	changed = true
	return t.applyAliasReplayWriteEffects(out, aliasWrites) || changed
}

func (t *Transfer) aliasReplayWriteEffects(out *flow.PointState, effect WriteEffect) []WriteEffect {
	if flow.ValueOriginAxisIsBottom(out) {
		return nil
	}
	path, ok := effect.Place.StaticPath()
	if !ok || path.Symbol == 0 || len(path.Segments) == 0 {
		return nil
	}
	var outEffects []WriteEffect
	for _, route := range flow.PointFactsOfBorrowed(out).ProvenanceRoutesFor(flow.ProvenanceRouteQuery{
		Path:                path,
		IdentityAliases:     true,
		IdentityAliasPolicy: flow.IdentityAliasDescendantOriginPolicy,
	}) {
		source := route.Source
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
			References:    effect.References,
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

func (t *Transfer) applySymbolWriteEffect(
	out *flow.PointState,
	target cfg.AssignTarget,
	val product.AbstractValue,
	clear bool,
	joinExisting bool,
	src ast.Expr,
	keyArrayTable constraint.Path,
	references referenceWrite,
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
		References:    references,
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
		flow.ClearPrototypeInstance(out, effect.Place.Root)
		t.symbolStorage.clear(out, effect.Place.Root, effect.JoinExisting, true)
	} else {
		if effect.Value.IsZero() {
			return t.applyReferenceEffect(out, referenceEffectForWrite(effect))
		}
		if _, ok := t.setMetatablePrototypeFromSource(effect.Source); !ok {
			flow.ClearPrototypeInstance(out, effect.Place.Root)
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
	flow.ApplyKeyArraySeedPathTransaction(out, flow.KeyArraySeedPathTransaction{
		ArrayPath: path,
		TablePath: effect.KeyArrayTable,
		HasTable:  true,
	})
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
		flow.ApplyKeyArraySeedPathTransaction(out, flow.KeyArraySeedPathTransaction{
			ArrayPath: rootPath.Field(array),
			Empty:     true,
		})
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
