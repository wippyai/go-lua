package delta

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applydifferential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Evaluate redeems one exact Later work entry.  The entry is checked against
// this session's execution before any change reader is bound; a foreign node,
// stale dependency, or Full-root request therefore cannot reach an operator.
func (session Session) Evaluate(entry arrangement.ScheduleEntry) (result Result, evaluated bool) {
	if !session.Available() || !entry.Available() {
		return Result{}, false
	}
	canonical, ok := session.execution.Dependency(entry.Dependency())
	if !ok || !sameEntry(canonical, entry) {
		return Result{}, false
	}
	rootNode := entry.Node()
	if !rootNode.Available() || rootNode.Digest() != entry.Node().Digest() {
		return Result{}, false
	}
	paths, ok := session.execution.Derivation(entry.Expression())
	if !ok || !paths.Available() || paths.Root() != entry.Expression() {
		return Result{}, false
	}
	expandDeltas, _, expandOK := session.expandReaderDeltas(paths)
	if !expandOK {
		return Result{}, false
	}
	targets, targetsOK := expandReplayTargets(expandDeltas)
	if !targetsOK {
		return Result{}, false
	}

	// First redeem only the sealed watcher prefixes.  This is a candidate
	// prepass: no value crosses an Expand yet, and no ordinary occurrence is
	// emitted.  Every watcher contributes its exact C source RowIDs to the
	// single affected-successor replay owned by its Expand.
	for index := 0; index < paths.Len(); index++ {
		path, pathOK := paths.PathAt(index)
		if !pathOK {
			return Result{}, false
		}
		watchers := targets[path.Occurrence()]
		if len(watchers) != 0 {
			for _, target := range watchers {
				value, active, executeOK := session.executePathToExpand(rootNode, path, paths, target.watcher)
				if !executeOK {
					return Result{}, false
				}
				if !active {
					continue
				}
				ids, idsOK := expandSourceCandidates(session.mounted, value.batches, target.contract.Candidate())
				if !idsOK {
					return Result{}, false
				}
				for _, id := range ids {
					if !appendExpandCandidate(&expandDeltas[target.delta], id) {
						return Result{}, false
					}
				}
			}
		}
	}
	for index := range expandDeltas {
		if !sortExpandCandidates(session.mounted, &expandDeltas[index]) {
			return Result{}, false
		}
	}

	// Emit in authored occurrence order.  Watcher paths are suppressed except
	// for the sealed minimum/anchor occurrence, where the full C-left is
	// replayed at Next and crosses Expand exactly once.  Keeping this emission
	// pass in path order preserves the surrounding independent occurrences.
	values := make([]pathValue, 0, paths.Len()+len(expandDeltas))
	emitted := make(map[int]bool, len(expandDeltas))
	for index := 0; index < paths.Len(); index++ {
		path, pathOK := paths.PathAt(index)
		if !pathOK {
			return Result{}, false
		}
		watchers := targets[path.Occurrence()]
		if len(watchers) != 0 {
			for _, target := range watchers {
				delta := &expandDeltas[target.delta]
				if path.Occurrence() != delta.trigger.Replay().EmitOccurrence() || emitted[target.delta] {
					continue
				}
				emitted[target.delta] = true
				if len(delta.candidates) == 0 {
					continue
				}
				value, active, executeOK := session.executeExpandReplayPath(rootNode, path, paths, *delta)
				if !executeOK {
					return Result{}, false
				}
				if !active {
					continue
				}
				if !value.available(session.mounted) {
					return Result{}, false
				}
				values = append(values, value)
			}
			// Every watcher occurrence is part of one sealed Expand replay;
			// ordinary crossing is suppressed even when it has no local C delta.
			continue
		}
		value, active, executeOK := session.executePath(rootNode, path, paths)
		if !executeOK {
			return Result{}, false
		}
		if !active {
			continue
		}
		if !value.available(session.mounted) {
			return Result{}, false
		}
		values = append(values, value)
	}
	for index, delta := range expandDeltas {
		if len(delta.candidates) != 0 && !emitted[index] {
			return Result{}, false
		}
	}
	if len(values) == 0 {
		result, evaluated = session.emptyResult(entry, rootNode)
		return result, evaluated
	}
	result, evaluated = session.finish(entry, rootNode, values)
	return result, evaluated
}

type expandReaderDelta struct {
	trigger    derivation.ExpandReaderTrigger
	binding    arrangement.ExpandBinding
	candidates []model.RowID
}

type expandReplayTarget struct {
	delta    int
	contract model.ExpandContract
	watcher  derivation.ExpandWatcher
}

// expandReplayTargets redeems the mount-sealed watcher directory. Every
// watcher occurrence is grouped by its exact Expand delta; ordinary path
// execution is suppressed for these occurrences and the canonical anchor is
// emitted once after all affected C RowIDs have been collected.
func expandReplayTargets(deltas []expandReaderDelta) (map[uint32][]expandReplayTarget, bool) {
	if deltas == nil {
		return nil, false
	}
	result := make(map[uint32][]expandReplayTarget)
	for deltaIndex := range deltas {
		delta := deltas[deltaIndex]
		if !delta.trigger.Available() || !delta.binding.Available() {
			return nil, false
		}
		replay := delta.trigger.Replay()
		if !replay.Available() || replay.EmitOccurrence() != replay.Anchor().PathOccurrence() {
			return nil, false
		}
		for watcherIndex := 0; watcherIndex < replay.WatcherCount(); watcherIndex++ {
			watcher, watcherOK := replay.WatcherAt(watcherIndex)
			if !watcherOK || !watcher.Available() || watcher.StopFrameDigest() != delta.trigger.Node() || watcher.StopFrame() != delta.trigger.FrameOrdinal() {
				return nil, false
			}
			occurrence := watcher.PathOccurrence()
			for _, prior := range result[occurrence] {
				if prior.delta == deltaIndex {
					return nil, false
				}
			}
			result[occurrence] = append(result[occurrence], expandReplayTarget{delta: deltaIndex, contract: delta.binding.Contract(), watcher: watcher})
		}
	}
	return result, true
}

func appendExpandCandidate(delta *expandReaderDelta, id model.RowID) bool {
	if delta == nil || !delta.binding.Available() || !id.Available() || id.Relation() != delta.binding.Contract().Candidate() {
		return false
	}
	for _, prior := range delta.candidates {
		if prior == id {
			return true
		}
	}
	delta.candidates = append(delta.candidates, id)
	return true
}

func sortExpandCandidates(mounted witness.Mounted, delta *expandReaderDelta) bool {
	if delta == nil || !mounted.Available() || !delta.binding.Available() {
		return false
	}
	positions := make(map[model.RowID]int, len(delta.candidates))
	for _, candidate := range delta.candidates {
		if candidate.Relation() != delta.binding.Contract().Candidate() {
			return false
		}
		position, positionOK := mounted.RowIndex(delta.binding.Contract().Candidate(), candidate)
		if !positionOK {
			return false
		}
		positions[candidate] = position
	}
	sort.SliceStable(delta.candidates, func(left, right int) bool {
		return positions[delta.candidates[left]] < positions[delta.candidates[right]]
	})
	return true
}

func expandSourceCandidates(mounted witness.Mounted, batches []tuple.Batch, candidate model.RelationID) ([]model.RowID, bool) {
	if !mounted.Available() || batches == nil || !candidate.Available() {
		return nil, false
	}
	result := make([]model.RowID, 0)
	for _, batch := range batches {
		if !batch.ValidFor(mounted) {
			return nil, false
		}
		for index := 0; index < batch.Len(); index++ {
			value, valueOK := batch.At(index)
			if !valueOK || !value.ValidFor(mounted) {
				return nil, false
			}
			id, idOK := value.SourceFor(candidate)
			if !idOK {
				return nil, false
			}
			result = append(result, id)
		}
	}
	return result, true
}

// expandReaderDeltas redeems every sealed R trigger through its exact full-R
// ChangeReader. It never scans R: keys come from successor After rows and C
// identities come only from frozen Evidence.CandidatesForKey. P contributes
// only to that frozen evidence's authentication/digest; it is not a delta
// frontier or wake source. A simultaneous C+R transition is handled by the
// sealed affected-candidate pivot; this reader frontier never excludes a
// whole logical RowID.
func (session Session) expandReaderDeltas(paths derivation.Plan) ([]expandReaderDelta, bool, bool) {
	if !session.Available() || !paths.Available() {
		return nil, false, false
	}
	triggers := paths.ExpandReaderTriggers()
	result := make([]expandReaderDelta, 0, len(triggers))
	wake := false
	for _, trigger := range triggers {
		if !trigger.Available() {
			return nil, false, false
		}
		node, nodeOK := session.execution.LogicalNode(trigger.Node())
		if !nodeOK || !node.Available() || node.Kind() != algebra.KindExpand {
			return nil, false, false
		}
		expandBinding, bindingOK := node.Expand()
		contract := frameContract(paths, trigger)
		if !bindingOK || !expandBinding.Available() || !contract.Available() || expandBinding.Contract() != contract {
			return nil, false, false
		}
		readerAccess := trigger.Reader().Access()
		readerLayout, layoutOK := session.layout(trigger.Reader().Physical())
		if !layoutOK || !readerLayout.Available() || readerLayout.Access().Relation() != readerAccess.Relation() || readerLayout.Access().Key().Available() != readerAccess.Key().Available() || !sameColumnIDs(readerLayout.Columns(), readerAccess.Columns()) || readerLayout.Access().Relation() != contract.Reader() || !sameColumnIDs(readerLayout.Columns(), expandBinding.Reader().Columns()) {
			return nil, false, false
		}
		changes, changesOK := read.BindChanges(session.delta, readerLayout, session.geometry, session.scratch)
		if !changesOK || !changes.Available() {
			return nil, false, false
		}
		delta := expandReaderDelta{trigger: trigger, binding: expandBinding, candidates: []model.RowID{}}
		seen := make(map[model.RowID]struct{})
		frontierValid := true
		completed, valid := changes.ScanChanges(func(change read.RowChange) bool {
			if !change.Available() {
				frontierValid = false
				return false
			}
			after, afterPresent := change.After()
			if !afterPresent || after == nil || !after.Available() {
				frontierValid = false
				return false
			}
			afterKey, keyOK := expandKeyCell(after, contract.Key(), expandBinding.Evidence().KeyType())
			if !keyOK {
				frontierValid = false
				return false
			}
			if before, beforePresent := change.Before(); beforePresent {
				beforeKey, beforeOK := expandKeyCell(before, contract.Key(), expandBinding.Evidence().KeyType())
				if !beforeOK || !beforeKey.Same(afterKey) {
					frontierValid = false
					return false
				}
			}
			ids, idsOK := expandBinding.Evidence().CandidatesForKey(afterKey)
			if !idsOK {
				frontierValid = false
				return false
			}
			wake = true
			for _, id := range ids {
				if _, duplicate := seen[id]; duplicate {
					continue
				}
				seen[id] = struct{}{}
				delta.candidates = append(delta.candidates, id)
			}
			return true
		})
		if !completed || !valid || !frontierValid {
			return nil, false, false
		}
		if len(delta.candidates) > 1 {
			positions := make(map[model.RowID]int, len(delta.candidates))
			for _, candidate := range delta.candidates {
				position, positionOK := session.mounted.RowIndex(contract.Candidate(), candidate)
				if !positionOK {
					return nil, false, false
				}
				positions[candidate] = position
			}
			sort.SliceStable(delta.candidates, func(left, right int) bool {
				return positions[delta.candidates[left]] < positions[delta.candidates[right]]
			})
		}
		result = append(result, delta)
	}
	return result, wake, true
}

func frameContract(paths derivation.Plan, trigger derivation.ExpandReaderTrigger) model.ExpandContract {
	path, ok := paths.Path(trigger.PathOccurrence())
	if !ok {
		return model.ExpandContract{}
	}
	frame, ok := path.FrameAt(int(trigger.FrameOrdinal()))
	if !ok || frame.Kind() != algebra.KindExpand || frame.Node() != trigger.Node() {
		return model.ExpandContract{}
	}
	return frame.ExpandContract()
}

func expandKeyCell(row read.Row, column model.ColumnID, typeID model.TypeID) (binding.ValueToken, bool) {
	if row == nil || !row.Available() || !column.Available() || !typeID.Available() {
		return binding.ValueToken{}, false
	}
	var result binding.ValueToken
	found := false
	for _, cell := range row.Cells() {
		if !cell.Available() || cell.Column() != column {
			continue
		}
		if found || (!cell.Presence().Is(model.Present) && !cell.Presence().Is(model.AuthenticatedOpaque)) || !cell.Value().Available() || cell.Value().Type() != typeID {
			return binding.ValueToken{}, false
		}
		result, found = cell.Value(), true
	}
	return result, found && result.Available()
}

func (session Session) emptyResult(entry arrangement.ScheduleEntry, root arrangement.Node) (Result, bool) {
	if !session.Available() || !entry.Available() || !root.Available() {
		return Result{}, false
	}
	value := Result{dependency: entry.Dependency(), expression: entry.Expression(), node: root.Digest(), kind: root.Kind(), batches: []tuple.Batch{}, applications: []apply.Results{}, settlements: []publish.Settlement{}, inputDelta: session.delta, base: session.delta.Base(), next: session.delta.Next(), sealed: true}
	return value, value.valid()
}

func sameEntry(left, right arrangement.ScheduleEntry) bool {
	return left.Available() && right.Available() && left.Dependency() == right.Dependency() && left.Expression() == right.Expression() && left.Component() == right.Component() && left.Node().Digest() == right.Node().Digest()
}

type pathValue struct {
	node         identity.ContentID
	kind         algebra.Kind
	batches      []tuple.Batch
	applications []apply.Results
	// differentials are private signed Apply transport. They remain beside
	// signed until the Publish boundary and are never projected into the
	// positive Applications ABI.
	differentials []applydifferential.Results
	settlements   []publish.Settlement
	// signed is populated only by the private unary/Apply vertical. It is
	// intentionally absent from Result: signed sides must survive operator
	// ascent but cannot become a second public relation representation.
	signed *signedValue
}

func relationValue(node identity.ContentID, kind algebra.Kind, batches []tuple.Batch) (pathValue, bool) {
	if !node.Available() || !relationKind(kind) || batches == nil {
		return pathValue{}, false
	}
	for _, batch := range batches {
		if !batch.Available() {
			return pathValue{}, false
		}
	}
	copyOf := make([]tuple.Batch, len(batches))
	copy(copyOf, batches)
	value := pathValue{node: node, kind: kind, batches: copyOf, applications: []apply.Results{}, differentials: []applydifferential.Results{}, settlements: []publish.Settlement{}}
	return value, value.availableNoMount()
}

func applyValue(node identity.ContentID, values []apply.Results) (pathValue, bool) {
	if !node.Available() || values == nil || !applicationsAvailable(values) {
		return pathValue{}, false
	}
	applications := make([]apply.Results, len(values))
	copy(applications, values)
	value := pathValue{node: node, kind: algebra.KindApply, batches: []tuple.Batch{}, applications: applications, differentials: []applydifferential.Results{}, settlements: []publish.Settlement{}}
	return value, value.availableNoMount()
}

func carriedValue(node identity.ContentID, kind algebra.Kind, batches []tuple.Batch, values []apply.Results) (pathValue, bool) {
	if !node.Available() || !composedKind(kind) || batches == nil || values == nil || !applicationsAvailable(values) {
		return pathValue{}, false
	}
	for _, batch := range batches {
		if !batch.Available() {
			return pathValue{}, false
		}
	}
	copyOf := make([]tuple.Batch, len(batches))
	copy(copyOf, batches)
	applications := make([]apply.Results, len(values))
	copy(applications, values)
	value := pathValue{node: node, kind: kind, batches: copyOf, applications: applications, differentials: []applydifferential.Results{}, settlements: []publish.Settlement{}}
	return value, value.availableNoMount()
}

func (value pathValue) availableNoMount() bool {
	if !value.node.Available() || value.batches == nil || value.applications == nil || value.differentials == nil || value.settlements == nil {
		return false
	}
	if value.signed != nil {
		if !value.signed.availableNoMount() || len(value.batches) != 0 || len(value.applications) != 0 || len(value.settlements) != 0 {
			return false
		}
		if len(value.differentials) != len(value.signed.differentials) {
			return false
		}
		for index, transport := range value.differentials {
			if !transport.Available() || !value.signed.differentials[index].Available() || transport.Operation() != value.signed.differentials[index].Operation() {
				return false
			}
		}
		return true
	}
	switch value.kind {
	case algebra.KindApply:
		return len(value.batches) == 0 && len(value.settlements) == 0 && applicationsAvailable(value.applications)
	case algebra.KindPublish:
		// A path-level Publish is pre-settlement transport. It carries the
		// authenticated applications produced by its child until finish redeems
		// all authored occurrences through the one publication door. Settlements
		// belong only to the finished Result.
		return len(value.batches) == 0 && len(value.settlements) == 0 && applicationsAvailable(value.applications)
	default:
		return relationKind(value.kind) && len(value.settlements) == 0 && valuesAvailable(value.batches) && (len(value.applications) == 0 || composedKind(value.kind)) && applicationsAvailable(value.applications)
	}
}

func (value pathValue) available(mounted witness.Mounted) bool {
	if !value.availableNoMount() || !mounted.Available() {
		return false
	}
	if value.signed != nil && !value.signed.validFor(mounted) {
		return false
	}
	for _, transport := range value.differentials {
		if !transport.Available() {
			return false
		}
	}
	for _, batch := range value.batches {
		if !batch.ValidFor(mounted) {
			return false
		}
	}
	return true
}

// finish folds independent occurrence paths into one root result and, for a
// Publish root, redeems all distinct applications through one ordered door.
func (session Session) finish(entry arrangement.ScheduleEntry, root arrangement.Node, values []pathValue) (Result, bool) {
	if !session.Available() || !entry.Available() || !root.Available() || len(values) == 0 {
		return Result{}, false
	}
	kind := root.Kind()
	publicationBase := session.delta.Next()
	result := Result{dependency: entry.Dependency(), expression: entry.Expression(), node: root.Digest(), kind: kind, inputDelta: session.delta, base: session.delta.Base(), next: publicationBase, batches: []tuple.Batch{}, applications: []apply.Results{}, settlements: []publish.Settlement{}}
	switch kind {
	case algebra.KindPublish:
		binding, ok := root.Publish()
		if !ok || !binding.Available() {
			return Result{}, false
		}
		current := publicationBase
		for _, value := range values {
			if value.kind != algebra.KindPublish && !composedKind(value.kind) && value.kind != algebra.KindApply {
				return Result{}, false
			}
			// A path is one authored occurrence. Ordinary and signed Apply
			// transport are mutually exclusive at that occurrence; accepting
			// both would require inventing an intra-path order not sealed by
			// derivation.
			if len(value.applications) != 0 && len(value.differentials) != 0 {
				return Result{}, false
			}
			if len(value.applications) != 0 {
				ordinarySettlements, next, publishOK := session.publish(entry, binding, value.applications, current)
				if !publishOK {
					return Result{}, false
				}
				result.applications = append(result.applications, value.applications...)
				result.settlements = append(result.settlements, ordinarySettlements...)
				current = next
			}
			if len(value.differentials) != 0 {
				differentialSettlements, next, publishOK := session.publishDifferentials(entry, binding, value.differentials, current)
				if !publishOK {
					return Result{}, false
				}
				result.settlements = append(result.settlements, differentialSettlements...)
				current = next
			}
		}
		// Keep the exact ordinary child Apply extents beside the settlements.
		// Signed transport remains private; its authenticated publication is
		// represented by the ordered settlements and final successor root.
		result.next = current
	case algebra.KindApply:
		for _, value := range values {
			if value.kind != algebra.KindApply {
				return Result{}, false
			}
			if len(value.differentials) != 0 {
				return Result{}, false
			}
			result.applications = append(result.applications, value.applications...)
		}
	case algebra.KindGroup:
		// Group remains non-distributive in the Later evaluator. Its signed
		// extent requires a separately sealed key replay and must refuse rather
		// than reinterpret a changed row as a complete group.
		return Result{}, false
	default:
		if !relationKind(kind) && !composedKind(kind) {
			return Result{}, false
		}
		for _, value := range values {
			if value.kind != kind && !(composedKind(kind) && composedKind(value.kind)) {
				return Result{}, false
			}
			if value.signed != nil {
				if value.kind != kind {
					return Result{}, false
				}
				// The public relation result is positive-only and has no
				// owner-authenticated removal destination. Refuse any signed
				// transition with a non-empty Before output instead of
				// collapsing it to After-only output and leaving stale derived
				// rows published. A present-but-empty Before output is an
				// authenticated no-selection and remains representable.
				// This is the frozen Result/transaction ABI boundary: insertion
				// (Before absent) remains representable, while deletion,
				// replacement, and true-to-false selection are explicit refusal.
				if value.signed.hasBeforeOutput() {
					return Result{}, false
				}
				batches, batchesOK := value.signed.afterBatches(session.mounted)
				if !batchesOK {
					return Result{}, false
				}
				result.batches = append(result.batches, batches...)
			} else {
				result.batches = append(result.batches, value.batches...)
			}
			result.applications = append(result.applications, value.applications...)
		}
	}
	result.sealed = true
	if !result.valid() {
		return Result{}, false
	}
	return result, true
}
