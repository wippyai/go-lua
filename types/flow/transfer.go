// transfer.go implements the transfer functions for the flow analysis worklist algorithm.
//
// Transfer functions define how types change at each CFG point. This file handles:
//
//   - Phi node processing: Joining types from multiple incoming branches
//   - Assignment processing: Computing types for assignment targets
//   - Iterator derivation: Computing loop variable types from iterator sources
//   - Widening: Expanding types for table mutations and dynamic indexing
//
// The functions return changed keys to drive worklist propagation.
package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type phiJoinKey struct {
	point cfg.Point
	key   string
}

type phiJoinValue struct {
	operands []typ.Type
	joined   typ.Type
}

type mapMutationEvaluation struct {
	keyType      typ.Type
	valueType    typ.Type
	currentType  typ.Type
	existingType typ.Type
	pathKey      string
}

// processPointReturnChangedKeys processes all type-changing operations at a CFG point.
//
// This is the main transfer function called by the worklist algorithm. It processes:
//   - Phi nodes: Joining types from multiple control flow paths
//   - Assignments: Computing and storing target types
//
// Returns the list of canonical keys whose types changed, used to determine
// which dependent points need re-processing.
func (s *Solution) processPointReturnChangedKeys(p cfg.Point) []string {
	c := s.inputs.Graph
	node := c.Node(p)
	if node == nil {
		return nil
	}

	var changedKeys []string
	oldMutableState := s.beginPointMutableState(p)
	if !s.transferPointReachable(p) {
		return s.clearPointTransferState(p, oldMutableState)
	}

	// Process phi nodes at this point
	phiKeys := s.processJoinReturnChangedKeys(p)
	changedKeys = append(changedKeys, phiKeys...)

	assignKeys := s.processAssignmentReturnChangedKeys(p)
	changedKeys = append(changedKeys, assignKeys...)
	changedKeys = append(changedKeys, s.mutableStateChangedKeys(oldMutableState, p)...)

	return changedKeys
}

// processAssignmentReturnChangedKeys processes all assignments at a CFG point.
//
// For each assignment targeting this point, computes the assigned type by:
//  1. Evaluating the canonical AssignmentSource against current flow state
//  2. Reconciling that evidence with the statically extracted type
//
// Also handles special assignment types:
//   - MapMutatorAssignments: Lua map writes (t[k] = v) through indexed-write admission
//   - TableMutatorAssignments: table.insert-like array element widening
//   - ContainerMutatorAssignments: channel.send-like element type widening
//
// Returns keys that changed, enabling worklist-driven convergence.
func (s *Solution) processAssignmentReturnChangedKeys(p cfg.Point) []string {
	var changedKeys []string

	for _, assign := range s.assignmentsAt(p) {
		// Field/index writes create a new symbol version. Carry forward unchanged
		// base/suffix facts from predecessor versions so sibling fields remain stable.
		if len(assign.TargetPath.Segments) > 0 {
			changedKeys = append(changedKeys, s.carryForwardStructuredVersionFacts(p, assign.TargetPath)...)
		}

		assignedType := assignmentEvidenceType(assign.Type, s.assignmentSourceTypeAt(p, assign))

		targetKey := s.pkResolver.KeyAt(p, assign.TargetPath)
		if targetKey == "" {
			continue
		}

		// Track alias provenance for path assignments: t.cur = s
		// so writes through t.cur.* propagate to s.*.
		if len(assign.TargetPath.Segments) > 0 {
			targetKeyStr := string(targetKey)
			if assign.Source.Kind == AssignmentSourcePath && assign.Source.Path.HasSymbol() {
				sourceKey := s.pkResolver.KeyAt(p, assign.Source.Path)
				if sourceKey != "" && sourceKey != targetKey {
					if s.pathAliases == nil {
						s.pathAliases = make(map[string]string)
					}
					s.pathAliases[targetKeyStr] = string(sourceKey)
				} else {
					delete(s.pathAliases, targetKeyStr)
				}
			} else {
				delete(s.pathAliases, targetKeyStr)
			}
		}

		if assignedType != nil {
			targetKeyStr := string(targetKey)
			old := s.valueAtPoint(p, targetKeyStr)
			if len(assign.TargetPath.Segments) > 0 && assignedType.Kind() == kind.Nil {
				assignedType = s.normalizeNilFieldAssignmentType(p, assign.TargetPath, old)
			}
			if !sameFlowValue(old, assignedType) {
				if len(assign.TargetPath.Segments) > 0 {
					s.setMutableValue(p, targetKeyStr, assignedType)
				} else {
					s.setValue(targetKeyStr, assignedType)
					changedKeys = append(changedKeys, targetKeyStr)
				}
			}
			changedKeys = append(changedKeys, s.clearMutableDescendantsAtPoint(p, targetKeyStr)...)
			if len(assign.TargetPath.Segments) > 0 {
				changedKeys = append(changedKeys, s.propagateAliasedFieldWrite(p, assign.TargetPath, assignedType)...)
				changedKeys = append(changedKeys, s.propagateSourceFieldWriteToAliases(p, assign.TargetPath, assignedType)...)
			}
		}
	}

	// Process map writes through the canonical indexed-write domain law.
	for _, mm := range s.mapMutatorAssignmentsAt(p) {
		if key := s.processMapMutatorAssignmentReturnKey(p, mm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	// Process table mutator assignments (table.insert-like)
	for _, tm := range s.tableMutatorAssignmentsAt(p) {
		if key := s.processTableMutatorAssignmentReturnKey(p, tm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	// Process container mutator assignments (channel.send-like)
	for _, cm := range s.containerMutatorAssignmentsAt(p) {
		if key := s.processContainerMutatorAssignmentReturnKey(p, cm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	return changedKeys
}

func assignmentEvidenceType(staticType, sourceType typ.Type) typ.Type {
	if sourceType == nil {
		return staticType
	}
	if staticType == nil || typ.TypeEquals(sourceType, staticType) {
		return sourceType
	}
	if typ.MorePrecise(staticType, sourceType) {
		return staticType
	}
	if typ.ContainsTypeParam(sourceType) && !typ.ContainsTypeParam(staticType) {
		return staticType
	}
	if typ.ContainsTypeParam(staticType) && !typ.ContainsTypeParam(sourceType) {
		return sourceType
	}
	if sourceType.Kind().IsPlaceholder() && !staticType.Kind().IsPlaceholder() {
		return staticType
	}
	if staticType.Kind().IsPlaceholder() {
		return sourceType
	}
	return sourceType
}

func (s *Solution) mutationValueTypeAt(p cfg.Point, valuePath constraint.Path, staticType typ.Type, template ValueTemplate) typ.Type {
	valueType := staticType
	if valuePath.HasSymbol() {
		if resolved := s.NarrowedTypeAt(p, valuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if len(template.Slots) == 0 {
		return valueType
	}
	return s.applyValueTemplate(p, valueType, template)
}

// MutatorValueTypeAt evaluates the canonical mutator value algebra against the
// solved abstract state. Postflow evidence reducers use this instead of
// re-reading AST expressions after flow has already lowered map/table/container
// mutations.
func (s *Solution) MutatorValueTypeAt(p cfg.Point, valuePath constraint.Path, staticType typ.Type, template ValueTemplate) typ.Type {
	if s == nil {
		return staticType
	}
	return s.mutationValueTypeAt(p, valuePath, staticType, template)
}

// MutatorKeyTypeAt evaluates the canonical mutator key algebra against the
// solved abstract state. Postflow reducers use this to store the same widened
// key domain that transfer used when applying map/table mutator operators.
func (s *Solution) MutatorKeyTypeAt(p cfg.Point, keyPath constraint.Path, staticType typ.Type) typ.Type {
	if staticType == nil && !keyPath.HasSymbol() {
		return nil
	}
	if s == nil {
		return normalizeDynamicKeyType(staticType)
	}
	keyType := staticType
	if keyPath.HasSymbol() {
		if narrowed := s.NarrowedTypeAt(p, keyPath); !typ.IsAbsentOrUnknown(narrowed) {
			keyType = narrowed
		}
	}
	return normalizeDynamicKeyType(keyType)
}

func (s *Solution) applyValueTemplate(p cfg.Point, base typ.Type, template ValueTemplate) typ.Type {
	out := base
	for _, slot := range template.Slots {
		if len(slot.Segments) == 0 || slot.Source.Kind == AssignmentSourceStatic {
			continue
		}
		slotType := s.assignmentSourceTypeAt(p, UnifiedAssignment{
			Point:  p,
			Type:   nil,
			Source: slot.Source,
		})
		if typ.IsAbsentOrUnknown(slotType) {
			continue
		}
		out = setValueTemplateSlot(out, slot.Segments, slotType)
	}
	return out
}

func setValueTemplateSlot(base typ.Type, segments []constraint.Segment, valueType typ.Type) typ.Type {
	if len(segments) == 0 || valueType == nil {
		return base
	}
	seg := segments[0]
	switch v := typ.UnwrapAnnotated(base).(type) {
	case *typ.Alias:
		updated := setValueTemplateSlot(v.Target, segments, valueType)
		if updated == nil || typ.TypeEquals(updated, v.Target) {
			return base
		}
		return typ.NewAlias(v.Name, updated)
	case *typ.Optional:
		updated := setValueTemplateSlot(v.Inner, segments, valueType)
		if updated == nil || typ.TypeEquals(updated, v.Inner) {
			return base
		}
		return typ.NewOptional(updated)
	case *typ.Union:
		members := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			updated := setValueTemplateSlot(member, segments, valueType)
			if updated == nil {
				updated = member
			}
			if !sameFlowValue(member, updated) {
				changed = true
			}
			members = append(members, updated)
		}
		if !changed {
			return base
		}
		return typ.NewUnion(members...)
	case *typ.Record:
		field, ok := valueTemplateFieldSegment(seg)
		if !ok {
			return base
		}
		child := typ.Type(nil)
		optional := false
		if existing := v.GetField(field); existing != nil {
			child = existing.Type
			optional = existing.Optional
		} else if v.HasMapComponent() {
			child = v.MapValue
			optional = true
		}
		updated := setValueTemplateSlot(child, segments[1:], valueType)
		if updated == nil {
			return base
		}
		return rebuildValueTemplateRecord(v, field, updated, optional)
	case *typ.Tuple:
		if seg.Kind != constraint.SegmentIndexInt || seg.Index < 1 || int(seg.Index) > len(v.Elements) {
			return base
		}
		idx := int(seg.Index) - 1
		updated := setValueTemplateSlot(v.Elements[idx], segments[1:], valueType)
		if updated == nil || typ.TypeEquals(updated, v.Elements[idx]) {
			return base
		}
		elements := make([]typ.Type, len(v.Elements))
		copy(elements, v.Elements)
		elements[idx] = updated
		return typ.NewTuple(elements...)
	case *typ.Array:
		if seg.Kind != constraint.SegmentIndexInt {
			return base
		}
		updated := setValueTemplateSlot(v.Element, segments[1:], valueType)
		if updated == nil || typ.TypeEquals(updated, v.Element) {
			return base
		}
		return typ.NewArray(updated)
	default:
		return base
	}
}

func valueTemplateFieldSegment(seg constraint.Segment) (string, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func rebuildValueTemplateRecord(rec *typ.Record, field string, fieldType typ.Type, optional bool) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	added := false
	for _, f := range rec.Fields {
		if f.Name != field {
			addValueTemplateRecordField(builder, f.Name, f.Type, f.Optional, f.Readonly)
			continue
		}
		addValueTemplateRecordField(builder, f.Name, fieldType, optional || f.Optional, f.Readonly)
		added = true
	}
	if !added {
		addValueTemplateRecordField(builder, field, fieldType, optional, false)
	}
	return builder.Build()
}

func addValueTemplateRecordField(builder *typ.RecordBuilder, name string, fieldType typ.Type, optional, readonly bool) {
	switch {
	case optional && readonly:
		builder.OptReadonlyField(name, fieldType)
	case optional:
		builder.OptField(name, fieldType)
	case readonly:
		builder.ReadonlyField(name, fieldType)
	default:
		builder.Field(name, fieldType)
	}
}

func (s *Solution) clearPointTransferState(p cfg.Point, oldMutableState map[string]product.AbstractValue) []string {
	if s == nil || s.inputs == nil || s.pkResolver == nil {
		return nil
	}

	changed := s.clearPointScalarTransferState(p)
	if len(s.mutableValues[p]) > 0 {
		delete(s.mutableValues, p)
	}
	changed = append(changed, s.mutableStateChangedKeys(oldMutableState, p)...)
	if len(changed) <= 1 {
		return changed
	}
	sort.Strings(changed)
	out := changed[:0]
	for _, key := range changed {
		if len(out) == 0 || out[len(out)-1] != key {
			out = append(out, key)
		}
	}
	return out
}

func (s *Solution) clearPointScalarTransferState(p cfg.Point) []string {
	var changed []string
	for _, assign := range s.assignmentsAt(p) {
		targetRoot := assign.TargetPath
		targetRoot.Segments = nil
		if key := s.pkResolver.KeyAt(p, targetRoot); key != "" {
			if s.deleteValueKey(string(key)) {
				changed = append(changed, string(key))
			}
			s.deletePathAliasesWithPrefix(string(key))
		}
		if len(assign.TargetPath.Segments) > 0 {
			if key := s.pkResolver.KeyAt(p, assign.TargetPath); key != "" {
				delete(s.pathAliases, string(key))
			}
		}
	}
	for _, phi := range s.phisAt(p) {
		key := s.pkResolver.KeyAtVersion(phi.Target.Symbol, phi.Target.ID, nil)
		if key == "" {
			continue
		}
		if s.deleteValueKey(string(key)) {
			changed = append(changed, string(key))
		}
		s.deletePathAliasesWithPrefix(string(key))
	}
	return changed
}

func (s *Solution) deleteValueKey(key string) bool {
	if s == nil || key == "" || s.values == nil {
		return false
	}
	if _, ok := s.values[key]; !ok {
		return false
	}
	delete(s.values, key)
	if s.presence != nil {
		delete(s.presence, key)
	}
	s.removeValueSuffix(key)
	s.invalidateQueryCachesForWrite(key)
	return true
}

func (s *Solution) deletePathAliasesWithPrefix(prefix string) {
	if s == nil || prefix == "" || len(s.pathAliases) == 0 {
		return
	}
	for key := range s.pathAliases {
		if key == prefix || pathKeyHasPrefix(key, prefix) {
			delete(s.pathAliases, key)
		}
	}
}

func pathKeyHasPrefix(key, prefix string) bool {
	if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
		return false
	}
	switch key[len(prefix)] {
	case '.', '[':
		return true
	default:
		return false
	}
}

func mergeIteratorAssignedType(extracted, derived typ.Type) typ.Type {
	if extracted == nil {
		return derived
	}
	if derived == nil {
		return extracted
	}
	if derived.Kind().IsPlaceholder() {
		if !extracted.Kind().IsPlaceholder() {
			return extracted
		}
		return derived
	}
	if extracted.Kind().IsPlaceholder() {
		return derived
	}
	if subtype.IsSubtype(extracted, derived) {
		return extracted
	}
	if subtype.IsSubtype(derived, extracted) {
		return derived
	}
	return derived
}

func (s *Solution) assignmentSourceTypeAt(p cfg.Point, assign UnifiedAssignment) typ.Type {
	var sourceType typ.Type
	switch assign.Source.Kind {
	case AssignmentSourcePath:
		if !assign.Source.Path.HasSymbol() {
			return nil
		}
		// Lua evaluates RHS before writing assignment targets. For self-related
		// assignments, source reads must come from predecessor state.
		if s.assignmentNeedsPreState(assign) {
			if pre := s.preAssignmentNarrowedTypeAt(p, assign.Source.Path); pre != nil {
				sourceType = pre
				break
			}
		}
		sourceType = s.NarrowedTypeAt(p, assign.Source.Path)
	case AssignmentSourceIterator:
		if iterType := s.iterVarTypeAt(p, assign.Source); iterType != nil {
			sourceType = mergeIteratorAssignedType(assign.Type, iterType)
		}
	case AssignmentSourceContainerElement:
		sourceType = s.containerElementTypeAt(p, assign.Source)
	case AssignmentSourceMapElement:
		sourceType = s.mapElementTypeAt(p, assign.Source)
	case AssignmentSourceLengthIndex:
		sourceType = s.lengthIndexTypeAt(p, assign.Source, assign.Type)
	case AssignmentSourceCallReturn:
		sourceType = s.callReturnTypeAt(p, assign.Source)
	}
	return assignmentSourceProjectionType(sourceType, assign.Source)
}

func assignmentSourceProjectionType(sourceType typ.Type, source AssignmentSource) typ.Type {
	if source.ProjectionKind == AssignmentSourceProjectionNone {
		return sourceType
	}
	return value.SelectSourceProjection(sourceType, source.ProjectedType)
}

// AssignedValueTypeAt evaluates the canonical assignment-source algebra against
// the solved abstract state and reconciles it with the static extraction type.
// Postflow reducers use this instead of re-reading AST expressions.
func (s *Solution) AssignedValueTypeAt(p cfg.Point, target constraint.Path, static typ.Type, source AssignmentSource) typ.Type {
	if s == nil {
		return static
	}
	return assignmentEvidenceType(static, s.assignmentSourceTypeAt(p, UnifiedAssignment{
		Point:      p,
		TargetPath: target,
		Type:       static,
		Source:     source,
	}))
}

// AssignmentSourceValueAt evaluates source-owned assignment evidence without
// reconciling it against the target/static slot type. Diagnostics and postflow
// projection use this to consume the same canonical RHS algebra as transfer
// without treating target annotations as source facts.
func (s *Solution) AssignmentSourceValueAt(p cfg.Point, target constraint.Path, source AssignmentSource) typ.Type {
	if s == nil {
		return nil
	}
	return s.assignmentSourceTypeAt(p, UnifiedAssignment{
		Point:      p,
		TargetPath: target,
		Source:     source,
	})
}

func (s *Solution) assignmentNeedsPreState(assign UnifiedAssignment) bool {
	if assign.TargetPath.Symbol == 0 || assign.Source.Kind != AssignmentSourcePath || assign.Source.Path.Symbol == 0 {
		return false
	}
	if assign.TargetPath.Symbol != assign.Source.Path.Symbol {
		return false
	}
	return pathkey.PathRelated(assign.TargetPath, assign.Source.Path)
}

func (s *Solution) preAssignmentNarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || path.Symbol == 0 {
		return nil
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	var joined []typ.Type
	seenVersions := make(map[int]struct{}, len(preds))
	for _, pred := range preds {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if ver.ID == 0 {
			continue
		}
		if _, seen := seenVersions[ver.ID]; seen {
			continue
		}
		seenVersions[ver.ID] = struct{}{}

		predPath := path
		predPath.Version = ver.ID
		if t := s.NarrowedTypeAt(pred, predPath); t != nil {
			joined = append(joined, t)
		}
	}
	if len(joined) == 0 {
		return nil
	}
	return join.Types(joined...)
}

func (s *Solution) propagateAliasedFieldWrite(p cfg.Point, targetPath constraint.Path, assignedType typ.Type) []string {
	if s == nil || s.pkResolver == nil || assignedType == nil || targetPath.Symbol == 0 {
		return nil
	}
	if len(targetPath.Segments) == 0 {
		return nil
	}

	// Find the longest aliased prefix, e.g. for t.cur.hook use alias(t.cur) => s.
	for cut := len(targetPath.Segments) - 1; cut >= 1; cut-- {
		prefix := constraint.Path{
			Root:     targetPath.Root,
			Symbol:   targetPath.Symbol,
			Segments: targetPath.Segments[:cut],
		}
		sourceKey := s.aliasSourceKeyAt(p, prefix)
		if sourceKey == "" {
			continue
		}

		sourceSym, sourceVersion, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(sourceKey))
		if !ok || sourceSym == 0 || sourceVersion == 0 {
			continue
		}

		aliasSegs := pathkey.ParseSuffix(suffix)
		destSegs := append(append([]constraint.Segment{}, aliasSegs...), targetPath.Segments[cut:]...)
		destKey := s.pkResolver.KeyAtVersion(sourceSym, sourceVersion, destSegs)
		if destKey == "" {
			return nil
		}

		keyStr := string(destKey)
		old := s.valueAtPoint(p, keyStr)
		newType := assignedType
		if len(destSegs) > 0 && assignedType.Kind() == kind.Nil {
			newType = s.normalizeNilFieldAssignmentType(p, constraint.Path{
				Symbol:   sourceSym,
				Version:  sourceVersion,
				Segments: destSegs,
			}, old)
		}
		if sameFlowValue(old, newType) {
			return nil
		}
		s.setMutableValue(p, keyStr, newType)
		return nil
	}

	return nil
}

func (s *Solution) propagateSourceFieldWriteToAliases(p cfg.Point, sourcePath constraint.Path, assignedType typ.Type) []string {
	if s == nil || s.pkResolver == nil || assignedType == nil || sourcePath.Symbol == 0 || len(sourcePath.Segments) == 0 || len(s.pathAliases) == 0 {
		return nil
	}
	sourceKey := s.pkResolver.KeyAt(p, sourcePath)
	if sourceKey == "" {
		return nil
	}
	sourceSym, sourceVersion, sourceSuffix, ok := pathkey.ParseKeyUnchecked(sourceKey)
	if !ok || sourceSym == 0 || sourceVersion == 0 {
		return nil
	}
	sourceSegs := pathkey.ParseSuffix(sourceSuffix)
	for aliasKey, aliasSourceKey := range s.pathAliases {
		aliasSourceSym, aliasSourceVersion, aliasSourceSuffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(aliasSourceKey))
		if !ok || aliasSourceSym != sourceSym || aliasSourceVersion != sourceVersion {
			continue
		}
		aliasSourceSegs := pathkey.ParseSuffix(aliasSourceSuffix)
		if !segmentsPrefix(aliasSourceSegs, sourceSegs) {
			continue
		}
		aliasSym, aliasVersion, aliasSuffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(aliasKey))
		if !ok || aliasSym == 0 || aliasVersion == 0 {
			continue
		}
		aliasSegs := pathkey.ParseSuffix(aliasSuffix)
		destSegs := append(append([]constraint.Segment{}, aliasSegs...), sourceSegs[len(aliasSourceSegs):]...)
		destKey := s.pkResolver.KeyAtVersion(aliasSym, aliasVersion, destSegs)
		if destKey == "" {
			continue
		}
		keyStr := string(destKey)
		old := s.valueAtPoint(p, keyStr)
		newType := assignedType
		if len(destSegs) > 0 && assignedType.Kind() == kind.Nil {
			newType = s.normalizeNilFieldAssignmentType(p, constraint.Path{
				Symbol:   aliasSym,
				Version:  aliasVersion,
				Segments: destSegs,
			}, old)
		}
		if sameFlowValue(old, newType) {
			continue
		}
		s.setMutableValue(p, keyStr, newType)
	}
	return nil
}

func segmentsPrefix(prefix, full []constraint.Segment) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i := range prefix {
		if prefix[i] != full[i] {
			return false
		}
	}
	return true
}

func (s *Solution) aliasSourceKeyAt(p cfg.Point, path constraint.Path) string {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil || s.pathAliases == nil {
		return ""
	}
	if path.Symbol == 0 || path.IsEmpty() {
		return ""
	}

	// First try alias captured at current version.
	currentKey := s.pkResolver.KeyAt(p, path)
	if currentKey != "" {
		if source, ok := s.pathAliases[string(currentKey)]; ok {
			return source
		}
	}

	// If the root is directly reassigned at this point, do not inherit predecessor aliases.
	if s.hasRootAssignmentAtPoint(p, path.Symbol) {
		return ""
	}

	// Read carried structured aliases from predecessor versions.
	for _, pred := range graphPredecessors(s.inputs.Graph, p) {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if ver.Symbol == 0 || ver.ID == 0 {
			continue
		}
		predKey := s.pkResolver.KeyAtVersion(path.Symbol, ver.ID, path.Segments)
		if predKey == "" {
			continue
		}
		if source, ok := s.pathAliases[string(predKey)]; ok {
			return source
		}
	}
	return ""
}

func (s *Solution) hasRootAssignmentAtPoint(p cfg.Point, sym cfg.SymbolID) bool {
	if s == nil || s.inputs == nil || sym == 0 {
		return false
	}
	for _, assign := range s.inputs.Assignments {
		if assign.Point != p {
			continue
		}
		if assign.TargetPath.Symbol != sym {
			continue
		}
		if len(assign.TargetPath.Segments) == 0 {
			return true
		}
	}
	return false
}

// carryForwardStructuredVersionFacts seeds the current version with predecessor
// root/suffix facts for structured values when assigning through a sub-path.
func (s *Solution) carryForwardStructuredVersionFacts(p cfg.Point, targetPath constraint.Path) []string {
	return newStructuredCarryForward(s, p, targetPath).apply()
}

func (s *Solution) predecessorNarrowedRootTypes(
	p cfg.Point,
	targetPath constraint.Path,
	predBaseKeys []string,
) []typ.Type {
	preds := graphPredecessors(s.inputs.Graph, p)
	baseTypes := make([]typ.Type, 0, len(preds))
	for _, pred := range preds {
		predPath := constraint.Path{Root: targetPath.Root, Symbol: targetPath.Symbol}
		if predType := s.predecessorNarrowedRootType(pred, predPath); predType != nil && !typ.IsNever(predType) {
			baseTypes = append(baseTypes, predType)
		}
	}
	if len(baseTypes) > 0 {
		return baseTypes
	}

	baseTypes = make([]typ.Type, 0, len(predBaseKeys))
	for _, predBaseKey := range predBaseKeys {
		if av, ok := s.values[predBaseKey]; ok {
			if t := projectFlowValue(av); t != nil && !typ.IsNever(t) {
				baseTypes = append(baseTypes, t)
			}
		}
	}
	return baseTypes
}

func (s *Solution) predecessorNarrowedRootType(p cfg.Point, path constraint.Path) typ.Type {
	baseType := s.TypeAt(p, path)
	if baseType == nil {
		return nil
	}
	cond := s.ConditionAt(p)
	if !cond.HasConstraints() && !cond.IsFalse() {
		return baseType
	}
	return s.applyCondition(p, baseType, path, cond)
}

func (s *Solution) normalizeNilFieldAssignmentType(p cfg.Point, targetPath constraint.Path, old typ.Type) typ.Type {
	return typ.Nil
}

// iterVarTypeAt derives iterator variable type from the iterator source at point p.
//
// For Lua's iteration constructs (for-in loops), variable types depend on the
// iteration kind and variable position:
//
//   - IterateIndexed (ipairs): Index is always integer, value is array element
//   - IterateKeyed (pairs): Key is table key type, value is table value type
//
// Special handling for keys-provenance variables: When iterating over a variable
// that holds sorted keys of another table (via sorted_keys() or similar), the
// element type is derived from the original table's key type rather than the
// intermediate array.
func (s *Solution) iterVarTypeAt(p cfg.Point, src AssignmentSource) typ.Type {
	if src.Kind != AssignmentSourceIterator || src.Path.IsEmpty() {
		return nil
	}

	// Index variable for ipairs-style iteration is always integer.
	if src.IteratorKind == IterateIndexed && src.VarIndex == 0 {
		return typ.Integer
	}

	// For indexed iteration (ipairs) over keys-provenance variables,
	// reconcile the original table key type with the keys-array element type.
	// Pattern: local keys = sorted_keys(t); for _, k in ipairs(keys) do
	if src.IteratorKind == IterateIndexed && src.VarIndex == 1 {
		if origTableSym := s.keysProvenanceSource(src.Path.Symbol); origTableSym != 0 {
			sourceElem, _ := s.iterationElementEvidenceAt(p, src)
			origPath := constraint.Path{Root: s.inputs.Graph.NameOf(origTableSym), Symbol: origTableSym}
			if origType := s.NarrowedTypeAt(p, origPath); origType != nil {
				if keyType := s.inputs.Decomposer.KeyType(origType); keyType != nil {
					if merged := mergeKeysProvenanceElemType(sourceElem, keyType); merged != nil {
						return merged
					}
				}
			}
		}
	}

	if elem, placeholderSource := s.iterationElementEvidenceAt(p, src); elem != nil {
		return elem
	} else if placeholderSource {
		return typ.Any
	}
	return nil
}

func usableIteratorElementType(t typ.Type) bool {
	return t != nil && !t.Kind().IsPlaceholder()
}

// iterationElementEvidenceAt is the canonical iterator-source query for loop
// variables. It reads the product-domain evidence for a source path in precision
// order:
//
//   - point-local flow/narrowing state at the loop point,
//   - same-symbol path-domain facts accumulated from all visible versions,
//   - declared template evidence for annotated sources.
//
// The query returns the first usable iterator decomposition. A placeholder
// result means the source exists but carries no concrete element evidence.
func (s *Solution) iterationElementEvidenceAt(p cfg.Point, src AssignmentSource) (typ.Type, bool) {
	if src.Kind != AssignmentSourceIterator || src.Path.IsEmpty() {
		return nil, false
	}

	placeholderSource := false
	for _, srcType := range [...]typ.Type{
		s.pointLocalPathTypeAt(p, src.Path),
		s.pathDomainJoinedType(src.Path),
		s.lookupDeclaredType(src.Path),
	} {
		if srcType == nil {
			continue
		}
		if srcType.Kind().IsPlaceholder() {
			placeholderSource = true
			continue
		}
		if elem := s.elementTypeForIter(srcType, src); usableIteratorElementType(elem) {
			return elem, false
		} else if elem != nil && elem.Kind().IsPlaceholder() {
			placeholderSource = true
		}
	}
	return nil, placeholderSource
}

func (s *Solution) pointLocalPathTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	srcType := s.NarrowedTypeAt(p, path)
	if srcType == nil || srcType.Kind().IsPlaceholder() {
		if preType := s.preAssignmentNarrowedTypeAt(p, path); preType != nil {
			if srcType == nil || srcType.Kind().IsPlaceholder() || !preType.Kind().IsPlaceholder() {
				srcType = preType
			}
		}
	}
	return srcType
}

func mergeKeysProvenanceElemType(elemType, keyType typ.Type) typ.Type {
	if elemType == nil {
		return keyType
	}
	if keyType == nil {
		return elemType
	}
	// Placeholder key domains (any/unknown) add no usable precision.
	if keyType.Kind().IsPlaceholder() {
		return elemType
	}
	// If the keys array element is still placeholder, use provenance.
	if elemType.Kind().IsPlaceholder() {
		return keyType
	}
	// Prefer the more specific type when one subsumes the other.
	if subtype.IsSubtype(elemType, keyType) {
		return elemType
	}
	if subtype.IsSubtype(keyType, elemType) {
		return keyType
	}
	// Reconcile independent evidence; if incompatible, keep source-table key type.
	intersection := subtype.NormalizeIntersection(elemType, keyType)
	if intersection != nil && !typ.IsNever(intersection) {
		return intersection
	}
	return keyType
}

// keysProvenanceSource returns the original table symbol if sym holds keys of that table.
//
// Some patterns extract keys from a table into an array (e.g., local keys = sorted_keys(t)).
// When iterating over such an array, the element type should be the original table's key type,
// not simply string or the array's element type. This method looks up the provenance chain
// to find the original table.
func (s *Solution) keysProvenanceSource(sym cfg.SymbolID) cfg.SymbolID {
	if sym == 0 || s.inputs.KeysProvenance == nil {
		return 0
	}
	return s.inputs.KeysProvenance[sym]
}

// elementTypeForIter extracts the appropriate element/key/value type.
//
// The extraction depends on iterator kind and variable position:
//
//	IterateIndexed (ipairs):
//	  VarIndex 0: Always integer (handled elsewhere)
//	  VarIndex 1: ElementType (array element)
//
//	IterateKeyed (pairs):
//	  VarIndex 0: KeyType (map/record key)
//	  VarIndex 1: ValueType (map/record value)
//
// Uses TypeDecomposer for extraction to support various container types.
func (s *Solution) elementTypeForIter(srcType typ.Type, src AssignmentSource) typ.Type {
	d := s.inputs.Decomposer
	switch src.IteratorKind {
	case IterateIndexed:
		if src.VarIndex == 1 {
			return d.ElementType(srcType)
		}
	case IterateKeyed:
		switch src.VarIndex {
		case 0:
			return d.KeyType(srcType)
		case 1:
			return d.EntryValueType(srcType)
		}
	}
	return nil
}

// containerElementTypeAt derives the element type from a container at point p.
//
// Used for methods like channel:receive() where the return type depends on the
// container's element type at the call site. The element type is derived from:
//
//  1. Instantiated generics (e.g., Channel<T>): Uses first type argument
//  2. Arrays: Uses ElementType via decomposer
//  3. Other containers: Falls back to decomposer.ElementType
//
// This enables accurate return type inference for generic container methods
// even when the container was widened during flow analysis.
func (s *Solution) containerElementTypeAt(p cfg.Point, src AssignmentSource) typ.Type {
	if src.Kind != AssignmentSourceContainerElement || src.ContainerPath.IsEmpty() {
		return nil
	}

	containerType := s.NarrowedTypeAt(p, src.ContainerPath)
	if containerType == nil {
		return nil
	}

	// Extract element type from instantiated generics (e.g., Channel<T>)
	if inst, ok := containerType.(*typ.Instantiated); ok {
		if len(inst.TypeArgs) > 0 {
			return inst.TypeArgs[0]
		}
	}

	// Use structural element evidence for arrays and other containers.
	return s.inputs.Decomposer.ElementType(containerType)
}

// mapElementTypeAt derives the value type from a dynamic map index read source.
//
// For assignments like `local v = t[k]` where the key is non-const, extraction
// records AssignmentSourceMapElement so solve-time flow can derive `v` from current map type.
func (s *Solution) mapElementTypeAt(p cfg.Point, src AssignmentSource) typ.Type {
	if src.Kind != AssignmentSourceMapElement || src.MapPath.IsEmpty() {
		return nil
	}

	mapType := s.NarrowedTypeAt(p, src.MapPath)
	if mapType == nil || mapType.Kind().IsPlaceholder() {
		if preType := s.preAssignmentNarrowedTypeAt(p, src.MapPath); preType != nil {
			// Prefer predecessor flow evidence over placeholder/absent current state.
			if mapType == nil || mapType.Kind().IsPlaceholder() || !preType.Kind().IsPlaceholder() {
				mapType = preType
			}
		}
	}
	if mapType == nil {
		if declType := s.lookupDeclaredType(src.MapPath); declType != nil {
			mapType = declType
		}
	}
	if (mapType == nil || mapType.Kind().IsPlaceholder() || isEmptyRecordNoMapType(mapType)) &&
		src.MapPath.Symbol != 0 && len(src.MapPath.Segments) == 0 {
		if known := s.joinKnownRootTypes(src.MapPath.Symbol); known != nil {
			if mapType == nil {
				mapType = known
			} else {
				mapType = join.Types(mapType, known)
			}
		}
	}
	if mapType == nil {
		return nil
	}

	keyType := s.resolveSymbolKeyType(p, src.KeySymbol, src.KeyVar)
	keyType = normalizeDynamicKeyType(keyType)
	if s.resolver != nil {
		if valueType, ok := s.resolver.Index(mapType, keyType); ok {
			return s.mapElementPresenceTypeAt(p, src, valueType)
		}
	}
	if s.inputs.Decomposer != nil {
		if valueType := s.inputs.Decomposer.ValueType(mapType); valueType != nil {
			return s.mapElementPresenceTypeAt(p, src, valueType)
		}
	}
	if mapType.Kind().IsPlaceholder() {
		return typ.Any
	}
	// Dynamic index on known container shape with no value evidence resolves to nil.
	// This preserves Lua table semantics for missing keys and avoids placeholder
	// pollution in loop fixpoint before map writes are observed.
	return typ.Nil
}

func (s *Solution) mapElementPresenceTypeAt(p cfg.Point, src AssignmentSource, valueType typ.Type) typ.Type {
	if valueType == nil || s == nil || src.Kind != AssignmentSourceMapElement {
		return valueType
	}
	keyPath := s.mapElementKeyPath(src)
	if keyPath.IsEmpty() || src.MapPath.IsEmpty() {
		return valueType
	}
	if s.HasKeyOf(p, src.MapPath, keyPath) {
		return narrow.RemoveNil(valueType)
	}
	return valueType
}

func (s *Solution) mapElementKeyPath(src AssignmentSource) constraint.Path {
	if src.KeySymbol == 0 {
		return constraint.Path{}
	}
	root := src.KeyVar
	if root == "" && s != nil && s.inputs != nil && s.inputs.Graph != nil {
		root = s.inputs.Graph.NameOf(src.KeySymbol)
	}
	return constraint.Path{Root: root, Symbol: src.KeySymbol}
}

func (s *Solution) lengthIndexTypeAt(p cfg.Point, src AssignmentSource, indexResult typ.Type) typ.Type {
	if src.Kind != AssignmentSourceLengthIndex || src.ContainerPath.IsEmpty() {
		return nil
	}
	containerType := s.NarrowedTypeAt(p, src.ContainerPath)
	if containerType == nil || containerType.Kind().IsPlaceholder() {
		if preType := s.preAssignmentNarrowedTypeAt(p, src.ContainerPath); preType != nil {
			if containerType == nil || containerType.Kind().IsPlaceholder() || !preType.Kind().IsPlaceholder() {
				containerType = preType
			}
		}
	}
	if containerType == nil {
		containerType = s.lookupDeclaredType(src.ContainerPath)
	}
	if containerType == nil {
		return nil
	}
	lower, _, ok := s.LengthBoundsAt(p, src.ContainerPath)
	if !ok {
		return nil
	}
	readType := indexResult
	if lengthIndexNeedsSolvedRead(readType) {
		readType = s.lengthIndexReadType(containerType)
	}
	if readType == nil {
		return nil
	}
	if refined := narrow.RefineLengthIndex(containerType, readType, lower, src.Offset); refined != nil {
		return refined
	}
	if lengthIndexNeedsSolvedRead(indexResult) && lengthIndexPresenceProven(containerType, lower, src.Offset) {
		return readType
	}
	return nil
}

func lengthIndexNeedsSolvedRead(t typ.Type) bool {
	return typ.IsAbsentOrUnknown(t) || (t != nil && t.Kind() == kind.Nil)
}

func (s *Solution) lengthIndexReadType(containerType typ.Type) typ.Type {
	if containerType == nil {
		return nil
	}
	if s.resolver != nil {
		if valueType, ok := s.resolver.Index(containerType, typ.Integer); ok {
			return valueType
		}
	}
	if s.inputs != nil && s.inputs.Decomposer != nil {
		return s.inputs.Decomposer.ValueType(containerType)
	}
	return nil
}

func lengthIndexPresenceProven(containerType typ.Type, lower, offset int64) bool {
	index := lower + offset
	if index < 1 {
		return false
	}
	if offset == 0 {
		return true
	}
	return narrow.LengthBoundProvesSequenceIndex(containerType, index)
}

func (s *Solution) callReturnTypeAt(p cfg.Point, src AssignmentSource) typ.Type {
	if src.Kind != AssignmentSourceCallReturn || src.ReturnIndex < 0 {
		return nil
	}
	var callee typ.Type
	var receiver typ.Type
	if !src.ReceiverPath.IsEmpty() && src.Method != "" {
		receiver = s.pointLocalPathTypeAt(p, src.ReceiverPath)
		if receiver == nil || unwrap.IsOptionalLike(receiver) {
			return nil
		}
		if s.resolver == nil {
			return nil
		}
		if method, ok := s.resolver.Field(receiver, src.Method); ok {
			callee = method
		}
	} else if !src.CalleePath.IsEmpty() {
		callee = s.pointLocalPathTypeAt(p, src.CalleePath)
	}
	fn := unwrap.Function(callee)
	if fn == nil || src.ReturnIndex >= len(fn.Returns) {
		return nil
	}
	return callReturnSlotType(fn, src.ReturnIndex, receiver)
}

func callReturnSlotType(fn *typ.Function, returnIndex int, receiver typ.Type) typ.Type {
	if fn == nil || returnIndex < 0 || returnIndex >= len(fn.Returns) {
		return nil
	}
	base := fn.Returns[returnIndex]
	if receiver != nil {
		base = subst.SelfValue(base, receiver)
	}
	if er := contract.ErrorReturnForValue(fn, returnIndex); er != nil && !unwrap.IsOptionalLike(base) {
		return typ.NewOptional(base)
	}
	return base
}

// processMapMutatorAssignmentReturnKey handles dynamic index assignments like t[k] = v.
//
// When a table is indexed with a non-constant key (t[k] = v), the table may need
// widening to accommodate dynamic keys. This method:
//
//  1. Resolves key type from flow state or explicit override
//  2. Preserves sealed annotations while allowing refinable structural slots
//  3. Applies the value-domain indexed-write admission law
//
// Widening rules:
//   - Empty record {} becomes map {[K]: V}
//   - Record with fields gains a map component
//   - Existing map widens key/value types via union
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processMapMutatorAssignmentReturnKey(p cfg.Point, mm MapMutatorAssignment) string {
	if mm.Target.Symbol == 0 {
		return ""
	}

	if s.annotationSealsMutableTarget(mm.Target) {
		return ""
	}
	eval, ok := s.evaluateMapMutation(p, mm)
	if !ok {
		return ""
	}
	if s.mapMutationWritesPairedIteratorValue(mm) {
		if eval.existingType == nil && eval.currentType != nil {
			s.setMutableValue(p, eval.pathKey, eval.currentType)
		}
		return ""
	}
	if !mapMutationAdmits(eval.currentType, eval.keyType, eval.valueType, mm.ValueMode) {
		return ""
	}

	var newType typ.Type
	if mm.ValueMode == MapMutationValueUpdate {
		newType = value.AdmitIndexedValueMutation(eval.currentType, eval.keyType, eval.valueType)
	} else {
		newType = value.AdmitIndexedWrite(eval.currentType, eval.keyType, eval.valueType)
	}
	newType = value.MergeForConvergence(eval.currentType, newType)
	if newType == nil || sameFlowValue(eval.currentType, newType) {
		if eval.existingType == nil && eval.currentType != nil {
			s.setMutableValue(p, eval.pathKey, eval.currentType)
		}
		return ""
	}

	s.setMutableValue(p, eval.pathKey, newType)
	return eval.pathKey
}

func (s *Solution) evaluateMapMutation(p cfg.Point, mm MapMutatorAssignment) (mapMutationEvaluation, bool) {
	if s == nil || s.pkResolver == nil || mm.Target.Symbol == 0 {
		return mapMutationEvaluation{}, false
	}
	keyType := mm.KeyType
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		keyType = s.resolveSymbolKeyType(p, mm.KeySymbol, mm.KeyVar)
	}
	keyType = normalizeDynamicKeyType(keyType)

	valueType := s.mutationValueTypeAt(p, mm.ValuePath, mm.ValueType, mm.Value)
	if valueType == nil {
		return mapMutationEvaluation{}, false
	}

	pathKey := s.pkResolver.KeyAt(p, mm.Target)
	if pathKey == "" {
		return mapMutationEvaluation{}, false
	}
	pathKeyStr := string(pathKey)
	existingAtCurrentKey := s.valueAtPoint(p, pathKeyStr)
	currentType := existingAtCurrentKey
	if currentType == nil {
		currentType = s.joinPredecessorPathTypes(p, mm.Target)
	}
	currentType = s.carryWidenedMutatorSeed(p, pathKeyStr, currentType)
	currentType = preferDeclaredTemplateForWiden(currentType, s.declaredTemplateForPath(mm.Target))

	return mapMutationEvaluation{
		keyType:      keyType,
		valueType:    valueType,
		currentType:  currentType,
		existingType: existingAtCurrentKey,
		pathKey:      pathKeyStr,
	}, true
}

// carryWidenedMutatorSeed folds the private loop-carry into the mutator's current
// seed so the self write-back is monotone across iterations. The carry holds p's
// prior post-state value for this exact self-written key; the public predecessor
// join can drop or partially re-supply it from pass to pass (a back-edge lag),
// which would otherwise toggle the key present/absent and spin the worklist. Merging
// the carry as a convergence-widening seed makes the mutator output depend only on
// the monotone closure of p's own writes, never on the oscillating join membership.
// The carry is private: it is read only here, and the public store still receives
// only the mutator's admitted result (setMutableValue), so no retained value leaks
// to projections or interproc summaries.
func (s *Solution) carryWidenedMutatorSeed(p cfg.Point, key string, current typ.Type) typ.Type {
	if zzNoSeed {
		return current
	}
	carried := s.mutableSelfCarryAt(p, key)
	if carried == nil {
		return current
	}
	if current == nil {
		return carried
	}
	merged := value.MergeForConvergence(current, carried)
	if merged != nil {
		if zzSeedDbg && !sameFlowValue(current, merged) {
			zzLog("ZZSEED key=%s current=%s carry=%s merged=%s", key, current.String(), carried.String(), merged.String())
		}
		return merged
	}
	return current
}

// IndexWriteAdmission returns the value admitted by the solved dynamic
// indexed-write transfer product at q. Diagnostics use this as proof that
// t[k] = v was accepted by the abstract interpreter, rather than rebuilding
// iterator/key provenance locally.
func (s *Solution) IndexWriteAdmission(q IndexWriteQuery) (typ.Type, bool) {
	if s == nil || q.Target.IsEmpty() {
		return nil, false
	}
	q.Target = unversionedTransferPath(q.Target)
	q.ValuePath = unversionedTransferPath(q.ValuePath)
	for _, mm := range s.mapMutatorAssignmentsAt(q.Point) {
		if !mapMutationMatchesQuery(mm, q) {
			continue
		}
		if mm.ValueMode != MapMutationValueWrite || s.annotationSealsMutableTarget(mm.Target) {
			return nil, false
		}
		eval, ok := s.evaluateMapMutation(q.Point, mm)
		if !ok {
			return nil, false
		}
		if s.mapMutationWritesPairedIteratorValue(mm) {
			return eval.valueType, true
		}
		if !mapMutationAdmits(eval.currentType, eval.keyType, eval.valueType, mm.ValueMode) {
			return nil, false
		}
		return eval.valueType, true
	}
	return nil, false
}

func mapMutationAdmits(currentType, keyType, valueType typ.Type, mode MapMutationValueMode) bool {
	if mode == MapMutationValueUpdate {
		return value.IndexedValueMutationAdmits(currentType, keyType, valueType)
	}
	return value.IndexedWriteAdmits(currentType, keyType, valueType)
}

func (s *Solution) mapMutationWritesPairedIteratorValue(mm MapMutatorAssignment) bool {
	if s == nil || s.inputs == nil || mm.ValueMode != MapMutationValueWrite {
		return false
	}
	if mm.Target.IsEmpty() || mm.KeySymbol == 0 || !mm.ValuePath.HasSymbol() || len(mm.ValuePath.Segments) != 0 {
		return false
	}
	keySrc, keyPoint, ok := s.iteratorSourceForSymbol(mm.KeySymbol, IterateKeyed, 0)
	if !ok || !sameTransferPath(keySrc.Path, mm.Target) {
		return false
	}
	valueSrc, valuePoint, ok := s.iteratorSourceForSymbol(mm.ValuePath.Symbol, IterateKeyed, 1)
	if !ok || keyPoint != valuePoint || !sameTransferPath(valueSrc.Path, mm.Target) {
		return false
	}
	return true
}

func (s *Solution) iteratorSourceForSymbol(sym cfg.SymbolID, kind IteratorKind, index int) (AssignmentSource, cfg.Point, bool) {
	if s == nil || s.inputs == nil || sym == 0 {
		return AssignmentSource{}, 0, false
	}
	for _, assign := range s.inputs.Assignments {
		if assign.TargetPath.Symbol != sym || assign.Source.Kind != AssignmentSourceIterator {
			continue
		}
		if assign.Source.IteratorKind != kind || assign.Source.VarIndex != index {
			continue
		}
		return assign.Source, assign.Point, true
	}
	return AssignmentSource{}, 0, false
}

func mapMutationMatchesQuery(mm MapMutatorAssignment, q IndexWriteQuery) bool {
	if mm.Point != q.Point || !sameTransferPath(mm.Target, q.Target) {
		return false
	}
	if q.KeySymbol != 0 && mm.KeySymbol != 0 && q.KeySymbol != mm.KeySymbol {
		return false
	}
	if q.ValuePath.HasSymbol() && mm.ValuePath.HasSymbol() && !sameTransferPath(q.ValuePath, mm.ValuePath) {
		return false
	}
	return true
}

func sameTransferPath(a, b constraint.Path) bool {
	return unversionedTransferPath(a).Equal(unversionedTransferPath(b))
}

func unversionedTransferPath(path constraint.Path) constraint.Path {
	path.Version = 0
	return path
}

func (s *Solution) joinPredecessorPathTypes(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil || path.Symbol == 0 {
		return nil
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	var types []typ.Type
	seen := make(map[string]struct{}, len(preds))
	for _, pred := range preds {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if ver.Symbol == 0 || ver.ID == 0 {
			continue
		}
		key := s.pkResolver.KeyAtVersion(ver.Symbol, ver.ID, path.Segments)
		var t typ.Type
		if key != "" {
			keyStr := string(key)
			if _, ok := seen[keyStr]; ok {
				continue
			}
			seen[keyStr] = struct{}{}
			t = s.valueAtPoint(pred, keyStr)
		}
		if t == nil && len(path.Segments) > 0 {
			rootKey := s.pkResolver.KeyAtVersion(ver.Symbol, ver.ID, nil)
			if rootKey != "" {
				rootKeyStr := string(rootKey)
				if _, ok := seen[rootKeyStr]; ok {
					continue
				}
				if root := s.valueAtPoint(pred, rootKeyStr); root != nil {
					seen[rootKeyStr] = struct{}{}
					if derived, ok := deriveTypeFrom(s.resolver, root, path.Segments); ok {
						t = derived
					}
				}
			}
		}
		if t != nil {
			types = append(types, t)
		}
	}
	if len(types) == 0 {
		return nil
	}
	return join.Types(types...)
}

func (s *Solution) joinKnownRootTypes(sym cfg.SymbolID) typ.Type {
	if s == nil || s.pkResolver == nil || sym == 0 {
		return nil
	}

	var types []typ.Type
	for key, av := range s.values {
		t := projectFlowValue(av)
		if t == nil {
			continue
		}
		parsedSym, _, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(key))
		if !ok || parsedSym != sym || suffix != "" {
			continue
		}
		types = append(types, t)
	}
	if len(types) == 0 {
		return nil
	}
	return join.Types(types...)
}

func (s *Solution) pathDomainJoinedType(path constraint.Path) typ.Type {
	if s == nil || path.Symbol == 0 {
		return nil
	}
	if len(path.Segments) == 0 {
		return s.joinKnownRootTypes(path.Symbol)
	}
	suffix := constraint.FormatSegments(path.Segments)
	var types []typ.Type
	for key, av := range s.values {
		t := projectFlowValue(av)
		if t == nil {
			continue
		}
		parsedSym, _, parsedSuffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(key))
		if !ok || parsedSym != path.Symbol {
			continue
		}
		switch parsedSuffix {
		case suffix:
			types = append(types, t)
		case "":
			if derived, ok := deriveTypeFrom(s.resolver, t, path.Segments); ok {
				types = append(types, derived)
			}
		}
	}
	if len(types) == 0 {
		return nil
	}
	return join.Types(types...)
}

// resolveSymbolKeyType gets the flow-narrowed type for a symbol at point p.
//
// Used to look up key types for dynamic index operations. The symbol ID provides
// unique identity for SSA version lookup, and varName is used to construct
// the path for type lookup.
//
// Returns nil if symbol is zero, as identity-based lookup requires a valid Symbol.
func (s *Solution) resolveSymbolKeyType(p cfg.Point, sym cfg.SymbolID, varName string) typ.Type {
	if sym == 0 {
		return nil
	}
	keyPath := constraint.Path{Root: varName, Symbol: sym}
	if narrowed := s.NarrowedTypeAt(p, keyPath); narrowed != nil {
		return narrowed
	}
	return s.TypeAt(p, keyPath)
}

// processTableMutatorAssignmentReturnKey handles table.insert-like mutations.
//
// When table.insert(t, v) or similar operations are called, the array element
// type may need widening. This method:
//
//  1. Resolves value type from flow state or static extraction
//  2. Preserves sealed annotations while allowing refinable structural slots
//  3. For keyed targets (suites[name]), widens the map value's array element
//  4. For direct targets, widens the array element type
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processTableMutatorAssignmentReturnKey(p cfg.Point, tm TableMutatorAssignment) string {
	if tm.Target.Symbol == 0 {
		return ""
	}

	valueType := s.mutationValueTypeAt(p, tm.ValuePath, tm.ValueType, tm.Value)
	if valueType == nil {
		return ""
	}

	if s.annotationSealsMutableTarget(tm.Target) {
		return ""
	}

	pathKey := s.pkResolver.KeyAt(p, tm.Target)
	if pathKey == "" {
		return ""
	}

	pathKeyStr := string(pathKey)
	existingAtCurrentKey := s.valueAtPoint(p, pathKeyStr)
	currentType := existingAtCurrentKey
	if currentType == nil {
		currentType = s.joinPredecessorPathTypes(p, tm.Target)
	}
	currentType = s.carryWidenedMutatorSeed(p, pathKeyStr, currentType)
	currentType = preferDeclaredTemplateForWiden(currentType, s.declaredTemplateForPath(tm.Target))

	var newType typ.Type
	if tm.KeySymbol != 0 || tm.KeyType != nil {
		keyType := tm.KeyType
		if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
			keyType = s.resolveSymbolKeyType(p, tm.KeySymbol, tm.KeyVar)
		}
		keyType = normalizeDynamicKeyType(keyType)
		newType = value.AdmitMapArrayElementMutation(currentType, keyType, valueType)
	} else {
		newType = value.AdmitArrayElementMutation(currentType, valueType, typ.JoinPreferNonSoft)
	}
	newType = value.MergeForConvergence(currentType, newType)

	if newType == nil || sameFlowValue(currentType, newType) {
		if existingAtCurrentKey == nil && currentType != nil {
			s.setMutableValue(p, pathKeyStr, currentType)
		}
		return ""
	}

	s.setMutableValue(p, pathKeyStr, newType)
	return pathKeyStr
}

func normalizeDynamicKeyType(keyType typ.Type) typ.Type {
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		return typ.Unknown
	}
	return subtype.Widen(keyType)
}

// processContainerMutatorAssignmentReturnKey handles container mutations.
//
// When container:send(v) or similar operations are called, the container's
// element type may need widening to include the sent value's type. This is
// controlled by ContainerElementUnion effects in function specs.
//
// The method:
//  1. Resolves value type from flow state (using ValuePath) or static extraction
//  2. Applies WidenForInference to normalize the value type
//  3. Preserves sealed annotations while allowing refinable structural slots
//  4. Widens the container's element type via union
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processContainerMutatorAssignmentReturnKey(p cfg.Point, cm ContainerMutatorAssignment) string {
	if cm.Target.Symbol == 0 {
		return ""
	}

	valueType := s.mutationValueTypeAt(p, cm.ValuePath, cm.ValueType, cm.Value)
	if valueType == nil {
		return ""
	}
	valueType = subtype.WidenForInference(valueType)

	if s.annotationSealsMutableTarget(cm.Target) {
		return ""
	}

	pathKey := s.pkResolver.KeyAt(p, cm.Target)
	if pathKey == "" {
		return ""
	}

	pathKeyStr := string(pathKey)
	existingAtCurrentKey := s.valueAtPoint(p, pathKeyStr)
	currentType := existingAtCurrentKey
	if currentType == nil {
		currentType = s.joinPredecessorPathTypes(p, cm.Target)
	}
	currentType = s.carryWidenedMutatorSeed(p, pathKeyStr, currentType)
	currentType = preferDeclaredTemplateForWiden(currentType, s.declaredTemplateForPath(cm.Target))
	newType := widenContainerElementType(currentType, valueType)
	if newType != nil {
		newType = value.AdmitObservation(newType)
	}

	if newType == nil || typ.TypeEquals(currentType, newType) {
		if existingAtCurrentKey == nil && currentType != nil {
			s.setMutableValue(p, pathKeyStr, currentType)
		}
		return ""
	}

	s.setMutableValue(p, pathKeyStr, newType)
	return pathKeyStr
}

// widenContainerElementType widens a container's element type by unioning with a new value type.
//
// Supports various container types:
//
//   - Instantiated generics (Channel<T>): Widens first type argument
//   - Arrays: Widens element type
//   - Maps: Widens value type (key unchanged)
//   - Unions: Recursively widens contained containers
//
// Special case for Unknown element type: Rather than unioning (which would keep Unknown),
// replaces Unknown entirely with the value type. This enables precise inference when
// a container starts with unknown element type and is populated with concrete values.
//
// Returns the widened type, or the original if no widening is applicable.
func widenContainerElementType(containerType typ.Type, valueType typ.Type) typ.Type {
	if valueType == nil {
		return containerType
	}
	if containerType == nil {
		return nil
	}

	return typ.Visit(containerType, typ.Visitor[typ.Type]{
		Instantiated: func(inst *typ.Instantiated) typ.Type {
			// Handle instantiated generic containers (e.g., Channel<T>)
			if inst.Generic == nil || len(inst.TypeArgs) == 0 {
				return containerType
			}
			// Get current element type
			oldElem := inst.TypeArgs[0]
			// If old element is unknown, replace it with value type (not union)
			var newElem typ.Type
			if typ.IsAbsentOrUnknown(oldElem) { // Unknown replaced explicitly
				newElem = valueType
			} else {
				newElem = join.Types(oldElem, valueType)
			}
			if typ.TypeEquals(oldElem, newElem) {
				return containerType
			}
			// Create new instantiation with widened element type
			newArgs := make([]typ.Type, len(inst.TypeArgs))
			copy(newArgs, inst.TypeArgs)
			newArgs[0] = newElem
			return typ.Instantiate(inst.Generic, newArgs...)
		},
		Array: func(arr *typ.Array) typ.Type {
			// Handle array types
			oldElem := arr.Element
			var newElem typ.Type
			if typ.IsAbsentOrUnknown(oldElem) { // Unknown replaced explicitly
				newElem = valueType
			} else {
				newElem = join.Types(oldElem, valueType)
			}
			if typ.TypeEquals(oldElem, newElem) {
				return containerType
			}
			return typ.NewArray(newElem)
		},
		Map: func(m *typ.Map) typ.Type {
			// Handle map types (widen value type)
			newVal := join.Types(m.Value, valueType)
			if typ.TypeEquals(m.Value, newVal) {
				return containerType
			}
			return typ.NewMap(m.Key, newVal)
		},
		Union: func(u *typ.Union) typ.Type {
			// Handle unions containing containers
			var updated []typ.Type
			changed := false
			for _, m := range u.Members {
				widened := widenContainerElementType(m, valueType)
				if widened != nil && !typ.TypeEquals(m, widened) {
					updated = append(updated, widened)
					changed = true
				} else {
					updated = append(updated, m)
				}
			}
			if changed {
				return typ.NewUnion(updated...)
			}
			return containerType
		},
		Default: func(t typ.Type) typ.Type {
			// For unknown/any, cannot widen
			return containerType
		},
	})
}

func preferDeclaredTemplateForWiden(current, declared typ.Type) typ.Type {
	if declared == nil {
		return current
	}
	if isEmptyRecordNoMapType(current) && typ.IsRefinableAnnotation(declared) {
		return current
	}
	if current == nil || current.Kind().IsPlaceholder() || isEmptyRecordNoMapType(current) {
		return declared
	}
	return current
}

func (s *Solution) annotationSealsMutableTarget(path constraint.Path) bool {
	if s == nil || s.inputs == nil || path.Symbol == 0 {
		return false
	}
	if s.inputs.AnnotatedVars == nil || !s.inputs.AnnotatedVars[path.Symbol] {
		return false
	}
	declared := s.declaredTemplateForPath(path)
	if declared == nil {
		declared = s.inputs.DeclaredTypes[path.Symbol]
	}
	return !typ.IsRefinableAnnotation(declared)
}

func (s *Solution) declaredTemplateForPath(path constraint.Path) typ.Type {
	if s == nil || path.Symbol == 0 {
		return nil
	}
	root := s.lookupDeclaredType(constraint.Path{Root: path.Root, Symbol: path.Symbol})
	if root == nil || len(path.Segments) == 0 {
		return root
	}
	if t, ok := deriveTypeFrom(s.resolver, root, path.Segments); ok {
		return t
	}
	return nil
}

func isEmptyRecordNoMapType(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isEmptyRecordNoMapType(v.Target)
	case *typ.Record:
		return len(v.Fields) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

// processJoinReturnChangedKeys processes phi nodes at point p.
//
// Phi nodes occur at control flow join points where a variable may have different
// values from different predecessors. This method:
//
//  1. Collects types from each phi operand (applying edge conditions)
//  2. Joins all incoming types into a single result type
//  3. Stores the joined type under the phi target's canonical key
//  4. Processes field suffixes to propagate nested field narrowings through phis
//
// Field suffix processing enables patterns like:
//
//	if x.ok then x = {ok: true, v: 1} else x = {ok: false, e: ""} end
//	-- phi for x.ok merges true and false paths
//
// Returns keys that changed, driving worklist propagation.
func (s *Solution) processJoinReturnChangedKeys(p cfg.Point) []string {
	if s.inputs.Graph == nil {
		return nil
	}

	var changedKeys []string

	for _, phi := range s.phisAt(p) {
		// Collect types from operands, applying edge conditions
		types := s.scratchTypes[:0]
		for _, op := range phi.Operands {
			opType := s.phiOperandTypeAt(p, op, nil)
			if opType == nil {
				continue
			}
			types = append(types, opType)
		}

		if len(types) == 0 {
			continue
		}

		targetKey := s.pkResolver.KeyAtVersion(phi.Target.Symbol, phi.Target.ID, nil)
		if targetKey == "" {
			continue
		}
		// Join and store under phi target canonical key
		joined := s.joinPhiEquation(p, string(targetKey), types)
		old := projectFlowValue(s.values[string(targetKey)])
		joined = stabilizePhiJoin(old, joined)
		if !sameFlowValue(old, joined) {
			s.setValue(string(targetKey), joined)
			changedKeys = append(changedKeys, string(targetKey))
		}

		// Also process field suffixes from phi operands
		suffixMap := s.collectPhiOperandSuffixes(phi)
		suffixes := make([]string, 0, len(suffixMap))
		for suffix := range suffixMap {
			suffixes = append(suffixes, suffix)
		}
		sort.Strings(suffixes)
		for _, suffix := range suffixes {
			segments := pathkey.ParseSuffix(suffix)
			if len(segments) == 0 {
				continue
			}
			types = types[:0]
			for _, op := range phi.Operands {
				if !s.transferPointReachable(op.From) {
					continue
				}
				opType := s.phiOperandTypeAt(p, op, segments)
				if opType == nil {
					opType = typ.Nil
				}
				types = append(types, opType)
			}
			if len(types) == 0 {
				continue
			}
			fullKey := string(targetKey) + suffix
			joined = s.joinPhiEquation(p, fullKey, types)
			old = s.valueAtPoint(p, fullKey)
			joined = stabilizePhiJoin(old, joined)
			if !sameFlowValue(old, joined) {
				s.setMutableValue(p, fullKey, joined)
			}
		}
	}

	return changedKeys
}

// stabilizePhiJoin keeps the existing stored fact when the freshly joined
// candidate is the same point in the product lattice, so a recursive product
// family that re-derives to a different typ.Type instance but the same canonical
// product node reports no change and the worklist drains. The comparison is
// product identity (sameFlowValue / product.Equal), not a typ.Type precision
// comparison, which is what the convergence proof depends on.
func stabilizePhiJoin(existing, candidate typ.Type) typ.Type {
	if existing == nil || candidate == nil {
		return candidate
	}
	if sameFlowValue(existing, candidate) {
		return existing
	}
	return candidate
}

// joinPhiEquation joins phi operands. The join itself stays the precise
// control-flow join (join.Types) so phi nil/optional handling is exact: a branch
// that does not assign keeps nil in the merged type. Recursive-growth convergence
// is owned by the store no-op check, not the join: the abstract-state carrier
// holds product.AbstractValue and sameFlowValue compares product identity, where
// the interner collapses converged recursive families to one canonical node. So a
// self-similar phi result that grows structurally each iteration still reaches a
// product-Equal fixed point and the worklist drains.
func (s *Solution) joinPhiEquation(p cfg.Point, key string, operands []typ.Type) typ.Type {
	if len(operands) == 0 {
		return nil
	}
	if len(operands) == 1 {
		return operands[0]
	}
	if s.phiJoinState == nil {
		s.phiJoinState = make(map[phiJoinKey]phiJoinValue, 8)
	}
	cacheKey := phiJoinKey{point: p, key: key}
	if prev, ok := s.phiJoinState[cacheKey]; ok && sameFlowValueVector(prev.operands, operands) {
		return prev.joined
	}
	joined := join.Types(operands...)
	s.phiJoinState[cacheKey] = phiJoinValue{
		operands: append([]typ.Type(nil), operands...),
		joined:   joined,
	}
	return joined
}

func typeVectorsEqual(a, b []typ.Type) bool {
	return sameFlowValueVector(a, b)
}

func (s *Solution) phiOperandTypeAt(joinPoint cfg.Point, op cfg.PhiOperand, segments []constraint.Segment) typ.Type {
	if s == nil {
		return nil
	}
	if !s.transferPointReachable(op.From) {
		return nil
	}
	path := constraint.Path{
		Root:    op.Version.Root,
		Symbol:  op.Version.Symbol,
		Version: op.Version.ID,
	}
	if len(segments) > 0 {
		path.Segments = append(path.Segments, segments...)
	}

	opType := s.NarrowedTypeAt(op.From, path)
	if opType == nil {
		opType = s.baseTypeAt(op.From, path)
	}
	if opType == nil {
		return nil
	}

	edgeK := edgeKey{from: op.From, to: joinPoint}
	if cond, ok := s.edgeConditions[edgeK]; ok && cond.HasConstraints() {
		if narrowed := s.applyCondition(op.From, opType, path, cond); narrowed != nil {
			opType = narrowed
		}
	}
	return opType
}

// collectPhiOperandSuffixes collects field suffixes from phi operand canonical keys.
//
// When a phi merges versions of a variable, field assignments made on different
// branches also need to be joined. This method scans the values map to find all
// field/index suffixes that have types for any phi operand.
//
// For example, if phi merges x@1 and x@2, and values contains:
//   - x@1.foo -> string
//   - x@2.bar -> number
//
// This returns {".foo", ".bar"} so both field types can be joined at the phi.
//
// Uses scratch buffer s.scratchSuffix to reduce allocations in the hot path.
func (s *Solution) collectPhiOperandSuffixes(phi cfg.PhiNode) map[string]struct{} {
	out := s.scratchSuffix
	if out == nil {
		out = make(map[string]struct{}, 16)
		s.scratchSuffix = out
	}
	clear(out)

	for _, op := range phi.Operands {
		baseKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
		if baseKey == "" {
			continue
		}
		baseKeyStr := string(baseKey)
		for _, suffix := range s.valueSuffixesForRoot(baseKeyStr) {
			out[suffix] = struct{}{}
		}
		for _, suffix := range s.mutableSuffixesForRoot(op.From, baseKeyStr) {
			out[suffix] = struct{}{}
		}
	}
	return out
}

func (s *Solution) valueSuffixesForRoot(baseKey string) []string {
	if s == nil || baseKey == "" || len(s.values) == 0 {
		return nil
	}
	s.ensureValueSuffixIndex()
	return s.valueSuffixIndex[baseKey]
}

func (s *Solution) mutableSuffixesForRoot(p cfg.Point, baseKey string) []string {
	if s == nil || baseKey == "" || len(s.mutableValues[p]) == 0 {
		return nil
	}
	if s.mutableSuffixIndexed == nil || !s.mutableSuffixIndexed[p] {
		s.indexMutableSuffixesForPoint(p)
	}
	return s.mutableSuffixIndex[pointRootKey{point: p, root: baseKey}]
}
