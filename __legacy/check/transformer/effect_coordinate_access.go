package transformer

import "fmt"

// frozenEffectCoordinateAccess is the single structural inventory shared by
// Values access sealing and exact coordinate-family selection. Keeping both
// projections on this value prevents effect semantics from acquiring a second
// path traversal in the guarded evaluator.
type frozenEffectCoordinateAccess struct {
	kind       EffectKind
	readTerms  []ValueTerm
	readPaths  []PathTerm
	writePaths []PathTerm
}

func freezeEffectCoordinateAccess(body *relationProgramBody, term EffectTerm) (frozenEffectCoordinateAccess, error) {
	if body == nil || body.relation.effects == nil || term == 0 || int(term) >= len(body.relation.effects.nodes) {
		return frozenEffectCoordinateAccess{}, fmt.Errorf("transformer: effect coordinate access has no frozen effect")
	}
	return freezeEffectNodeCoordinateAccess(body.relation.effects.nodes[term])
}

// freezeEffectNodeCoordinateAccess is the one effect syntax traversal for both
// coordinate selection and guard-demand discovery. Consumers project paths or
// ValueTerm guards from this frozen inventory; they never restate effect-kind
// layouts independently.
func freezeEffectNodeCoordinateAccess(node effectNode) (frozenEffectCoordinateAccess, error) {
	out := frozenEffectCoordinateAccess{kind: node.kind}
	addTarget := func(target EffectTargetTerm) {
		if target.kind == effectTargetPath {
			out.readPaths = append(out.readPaths, target.path)
			out.writePaths = append(out.writePaths, target.path)
		}
	}
	addInvalidation := func(invalidation InvalidatePathConfig) {
		addTarget(invalidation.Target)
		if invalidation.Precise != nil {
			out.readPaths = append(out.readPaths, invalidation.Precise.Table)
			out.readTerms = append(out.readTerms, invalidation.Precise.Key)
		}
	}
	addObject := func(object PathStoreObjectConfig) {
		for _, heap := range object.Heaps {
			out.readTerms = append(out.readTerms, heap.Root)
			for _, member := range heap.Members {
				out.readTerms = append(out.readTerms, member.Value)
				if member.HasExpected {
					out.readTerms = append(out.readTerms, member.Expected)
				}
			}
		}
		for _, entry := range object.Entries {
			out.readTerms = append(out.readTerms, entry.Value)
			out.readPaths = append(out.readPaths, entry.Target)
			out.writePaths = append(out.writePaths, entry.Target)
			if entry.HasSourcePath {
				out.readPaths = append(out.readPaths, entry.SourcePath)
			}
			if entry.HasExpected {
				out.readTerms = append(out.readTerms, entry.Expected)
			}
		}
	}
	switch node.kind {
	case EffectInvalidatePath:
		addInvalidation(node.invalidation)
	case EffectIndexMutation:
		addInvalidation(node.invalidation)
		addTarget(node.table)
		out.readTerms = append(out.readTerms, node.key, node.value)
		out.readPaths = append(out.readPaths, node.keyPath, node.valuePath)
	case EffectAllocationTemplate:
		// Allocation-only effects own no structural path coordinates.
	case EffectObjectMaterialization:
		addObject(node.pathStoreObject)
	case EffectPathStore:
		writes := []struct {
			enabled bool
			value   PathStoreWriteConfig
		}{{node.pathStoreHasAssignment, node.pathStoreAssignment}, {node.pathStoreHasStatic, node.pathStoreStatic}}
		for _, write := range writes {
			if !write.enabled {
				continue
			}
			out.readTerms = append(out.readTerms, write.value.Value)
			out.readPaths = append(out.readPaths, write.value.Target)
			out.writePaths = append(out.writePaths, write.value.Target)
			if write.value.HasSourcePath {
				out.readPaths = append(out.readPaths, write.value.SourcePath)
			}
		}
		addObject(node.pathStoreObject)
	default:
		return frozenEffectCoordinateAccess{}, fmt.Errorf("transformer: effect coordinate access has invalid kind")
	}
	seenTerms := make(map[ValueTerm]struct{}, len(out.readTerms))
	write := 0
	for _, value := range out.readTerms {
		if value == 0 {
			continue
		}
		if _, present := seenTerms[value]; present {
			continue
		}
		seenTerms[value] = struct{}{}
		out.readTerms[write] = value
		write++
	}
	out.readTerms = out.readTerms[:write]
	return out, nil
}

func (a frozenEffectCoordinateAccess) selection(body *relationProgramBody) (coordinateSelectionContract, error) {
	terms := make([]PathTerm, 0, len(a.readPaths)+len(a.writePaths))
	terms = append(terms, a.readPaths...)
	terms = append(terms, a.writePaths...)
	return freezeSemanticCoordinateSelection(body, fmt.Sprintf("effect:%d", a.kind), a.readTerms, terms)
}
