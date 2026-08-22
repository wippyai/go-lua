package engine

// runtime_artifact_read_index.go owns the contextual inverse for exact Units
// discovered by completed artifact producer Products.  The ordinary demand
// plan is graph-point keyed and therefore cannot be reused by a mounted epoch:
// one graph Point may have several executable StateOrdinal rows.  This inverse
// keeps only the rows a completed producer actually read; it never expands a
// Point, Unit, or closure relation into a dense contextual product.

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// artifactProducerReadKey is the exact publication address for one completed
// producer read.  A Unit alone is insufficient when one mounted artifact is
// executed in more than one context; StateOrdinal retains that context's
// executable source row without manufacturing a Context×Point table.
type artifactProducerReadKey struct {
	state contextfiber.StateOrdinal
	unit  carrier.Unit
}

// artifactProducerReadFactorKey is the sparse factor-publication address for
// exact reads owned by one factor slot in one source StateOrdinal. A carrier
// FactorRegion may intentionally omit UnitRegion rows when the whole factor
// root moves; retaining this slot-level inverse preserves that publication's
// exact contextual consumer set without expanding points or closure rows.
type artifactProducerReadFactorKey struct {
	state contextfiber.StateOrdinal
	slot  shape.Slot
}

// artifactProducerReadIndex is sparse in both dimensions.  The value is the
// existing contextual producer row(s) that read the key, represented by the
// exact StateOrdinal/GroupOrdinal pair used by the epoch candidate cache.
type artifactProducerReadIndex struct {
	byKey    map[artifactProducerReadKey][]stateGroupKey
	byFactor map[artifactProducerReadFactorKey][]stateGroupKey
}

func lessArtifactProducerReadKey(left, right artifactProducerReadKey) bool {
	if left.state != right.state {
		return left.state < right.state
	}
	if left.unit.Same(right.unit) {
		return false
	}
	return left.unit.Less(right.unit)
}

func sameArtifactProducerReadKey(left, right artifactProducerReadKey) bool {
	return left.state == right.state && left.unit.Same(right.unit)
}

func lessArtifactProducerReadFactorKey(left, right artifactProducerReadFactorKey) bool {
	if left.state != right.state {
		return left.state < right.state
	}
	return left.slot < right.slot
}

func sameArtifactProducerReadFactorKey(left, right artifactProducerReadFactorKey) bool {
	return left.state == right.state && left.slot == right.slot
}

func lessArtifactProducerReadConsumer(left, right stateGroupKey) bool {
	if left.state != right.state {
		return left.state < right.state
	}
	return left.group < right.group
}

func sameArtifactProducerReadConsumer(left, right stateGroupKey) bool {
	return left.state == right.state && left.group == right.group
}

// artifactProducerReadKeys validates and canonicalizes the observations from
// one completed producer.  The source StateOrdinal is derived from the
// producer's own contextual row and the declared input Point; a mounted source
// that has no unique state in that context is refused rather than routed via a
// guessed context or a graph-point fallback.
func (epoch *executorEpoch) artifactProducerReadKeys(stateIndex, group int, reads []demand.Observation) ([]artifactProducerReadKey, bool) {
	if epoch == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || epoch.runtime.graph == nil || epoch.runtime.carrier == nil || stateIndex < 0 || !epoch.activeState(stateIndex) || group < 0 || group >= len(epoch.runtime.producers) {
		return nil, false
	}
	cache, cacheOK := epoch.producerCache(contextfiber.StateOrdinal(stateIndex), group)
	if !cacheOK || cache == nil || cache.state != contextfiber.StateOrdinal(stateIndex) || cache.group != group {
		return nil, false
	}
	producer := &epoch.runtime.producers[group]
	if producer.index != group || !epoch.runtime.graph.OwnsGroup(producer.group) {
		return nil, false
	}
	output := producer.group.Output()
	outputIndex, outputOK := epoch.runtime.graph.PointIndex(output)
	statePoint, statePointIndex, _, statePointOK := epoch.runtime.graphPointAtState(stateIndex)
	if !outputOK || !statePointOK || statePointIndex != outputIndex || statePoint != output || producer.group.InputCount() < 0 {
		return nil, false
	}
	keys := make([]artifactProducerReadKey, 0, len(reads))
	for _, read := range reads {
		if read.Input >= uint64(producer.group.InputCount()) || read.Unit == (carrier.Unit{}) {
			return nil, false
		}
		slot, slotOK := read.Unit.Slot()
		if !slotOK || !epoch.runtime.carrier.OwnsUnit(slot, read.Unit) {
			return nil, false
		}
		_, inputOK := producer.group.InputAt(int(read.Input))
		if !inputOK {
			return nil, false
		}
		// Summary reads are already represented by their sealed structural
		// input/factor surfaces. This inverse is deliberately exact-Unit only;
		// retaining a summary here would broaden a later factor publication.
		if read.Unit.Kind() != carrier.ExactUnit {
			continue
		}
		sourceState, sourceOK := epoch.producerInputSourceState(producer, cache, int(read.Input))
		if !sourceOK || sourceState < 0 || !epoch.activeState(sourceState) {
			return nil, false
		}
		keys = append(keys, artifactProducerReadKey{state: contextfiber.StateOrdinal(sourceState), unit: read.Unit})
	}
	sort.Slice(keys, func(left, right int) bool { return lessArtifactProducerReadKey(keys[left], keys[right]) })
	write := 0
	for _, key := range keys {
		if write != 0 && sameArtifactProducerReadKey(keys[write-1], key) {
			continue
		}
		keys[write] = key
		write++
	}
	return keys[:write], true
}

func (epoch *executorEpoch) artifactProducerReadFactorKeys(keys []artifactProducerReadKey) ([]artifactProducerReadFactorKey, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.carrier == nil {
		return nil, false
	}
	factors := make([]artifactProducerReadFactorKey, 0, len(keys))
	for _, key := range keys {
		if key.state < 0 || !epoch.activeState(int(key.state)) {
			return nil, false
		}
		slot, slotOK := key.unit.Slot()
		if !slotOK || !epoch.runtime.carrier.OwnsUnit(slot, key.unit) {
			return nil, false
		}
		factors = append(factors, artifactProducerReadFactorKey{state: key.state, slot: slot})
	}
	sort.Slice(factors, func(left, right int) bool { return lessArtifactProducerReadFactorKey(factors[left], factors[right]) })
	write := 0
	for _, key := range factors {
		if write != 0 && sameArtifactProducerReadFactorKey(factors[write-1], key) {
			continue
		}
		factors[write] = key
		write++
	}
	return factors[:write], true
}

func artifactProducerReadConsumerPresent(rows []stateGroupKey, consumer stateGroupKey) bool {
	index := sort.Search(len(rows), func(index int) bool {
		return !lessArtifactProducerReadConsumer(rows[index], consumer)
	})
	return index < len(rows) && sameArtifactProducerReadConsumer(rows[index], consumer)
}

func artifactProducerReadConsumersValid(rows []stateGroupKey) bool {
	for index, row := range rows {
		if row.state < 0 || row.group < 0 || index > 0 && !lessArtifactProducerReadConsumer(rows[index-1], row) {
			return false
		}
	}
	return true
}

func insertArtifactProducerReadConsumer(rows []stateGroupKey, consumer stateGroupKey) []stateGroupKey {
	at := sort.Search(len(rows), func(index int) bool {
		return !lessArtifactProducerReadConsumer(rows[index], consumer)
	})
	rows = append(rows, stateGroupKey{})
	copy(rows[at+1:], rows[at:])
	rows[at] = consumer
	return rows
}

func removeArtifactProducerReadConsumer(rows []stateGroupKey, consumer stateGroupKey) []stateGroupKey {
	at := sort.Search(len(rows), func(index int) bool {
		return !lessArtifactProducerReadConsumer(rows[index], consumer)
	})
	rows = append(rows[:at], rows[at+1:]...)
	return rows
}

// validateArtifactProducerReadSnapshot authenticates the value a completed
// Rule actually consumed before its exact-read observations become a live
// inverse edge.  The source address is contextual (StateOrdinal, never a
// graph-point fallback), and the value is rebuilt through
// transportProducerInput, the same transport authority used by inputs().
//
// Rule execution cannot publish or re-enter the executor, and registration
// immediately follows evaluation under the solver lock. A different current
// transport therefore violates the executor contract; it is refused instead
// of being converted into a replay ticket.
func (epoch *executorEpoch) validateArtifactProducerReadSnapshot(stateIndex, group int, reads []demand.Observation, cache *producerEpoch) bool {
	// Clearing a producer has no observed input to authenticate. Its sparse
	// predecessor membership is validated by replaceArtifactProducerReads
	// itself, so a zero-read clear remains a valid transactional operation even
	// after the candidate input buffers have been discarded.
	if len(reads) == 0 {
		return true
	}
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || cache == nil || stateIndex < 0 || group < 0 || group >= len(epoch.runtime.producers) {
		return false
	}
	producer := &epoch.runtime.producers[group]
	inputCount := producer.group.InputCount()
	if inputCount < 0 || len(cache.inputs) != inputCount || len(cache.inputStates) != inputCount {
		return false
	}
	seen := make([]bool, inputCount)
	for _, read := range reads {
		if read.Input >= uint64(inputCount) {
			return false
		}
		inputIndex := int(read.Input)
		if seen[inputIndex] {
			continue
		}
		seen[inputIndex] = true
		evaluated := cache.inputs[inputIndex]
		if !epoch.work.OwnsPointState(evaluated) || !evaluated.Valid() || !epoch.work.OwnsState(cache.inputStates[inputIndex]) || !cache.inputStates[inputIndex].Same(evaluated.State()) {
			return false
		}
		current, currentOK := epoch.transportProducerInput(producer, cache, inputIndex)
		if !currentOK {
			return false
		}
		if !epoch.work.ExactSamePointRepresentation(evaluated, current) || !current.State().Same(cache.inputStates[inputIndex]) {
			return false
		}
	}
	return true
}

// replaceArtifactProducerReads commits one producer's completed exact-read
// membership. All metadata and the evaluated input snapshot are authenticated
// before the old inverse rows are removed, so a refusal cannot leave a
// partially replaced sparse relation. Snapshot disagreement refuses the
// transaction; it never manufactures a replay or wake.
func (epoch *executorEpoch) replaceArtifactProducerReads(stateIndex, group int, reads []demand.Observation) bool {
	if epoch == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || epoch.artifactProducerReads.byKey == nil || epoch.artifactProducerReads.byFactor == nil {
		return false
	}
	cache, cacheOK := epoch.producerCache(contextfiber.StateOrdinal(stateIndex), group)
	if !cacheOK {
		return false
	}
	if !epoch.validateArtifactProducerReadSnapshot(stateIndex, group, reads, cache) {
		return false
	}
	keys, keysOK := epoch.artifactProducerReadKeys(stateIndex, group, reads)
	if !keysOK {
		return false
	}
	factors, factorsOK := epoch.artifactProducerReadFactorKeys(keys)
	if !factorsOK {
		return false
	}
	consumer := stateGroupKey{state: contextfiber.StateOrdinal(stateIndex), group: group}
	oldFactors, oldFactorsOK := epoch.artifactProducerReadFactorKeys(cache.artifactReadKeys)
	if !oldFactorsOK || len(oldFactors) != len(cache.artifactReadFactorKeys) {
		return false
	}
	for index := range oldFactors {
		if !sameArtifactProducerReadFactorKey(oldFactors[index], cache.artifactReadFactorKeys[index]) {
			return false
		}
	}
	oldKeys := make(map[artifactProducerReadKey]struct{}, len(cache.artifactReadKeys))
	for _, key := range cache.artifactReadKeys {
		oldKeys[key] = struct{}{}
	}
	oldFactorSet := make(map[artifactProducerReadFactorKey]struct{}, len(cache.artifactReadFactorKeys))
	for _, key := range cache.artifactReadFactorKeys {
		oldFactorSet[key] = struct{}{}
	}
	// Validate the previous membership before mutating any map bucket. A
	// missing old row indicates index corruption and must not be silently
	// repaired by a later candidate completion.
	for index, key := range cache.artifactReadKeys {
		if key.state < 0 || !epoch.activeState(int(key.state)) {
			return false
		}
		slot, slotOK := key.unit.Slot()
		if !slotOK || !epoch.runtime.carrier.OwnsUnit(slot, key.unit) || index > 0 && !lessArtifactProducerReadKey(cache.artifactReadKeys[index-1], key) {
			return false
		}
		rows, present := epoch.artifactProducerReads.byKey[key]
		if !present || !artifactProducerReadConsumersValid(rows) || !artifactProducerReadConsumerPresent(rows, consumer) {
			return false
		}
	}
	for index, key := range cache.artifactReadFactorKeys {
		if key.state < 0 || !epoch.activeState(int(key.state)) || key.slot < 0 || int(key.slot) >= epoch.runtime.carrier.Count() || index > 0 && !lessArtifactProducerReadFactorKey(cache.artifactReadFactorKeys[index-1], key) {
			return false
		}
		rows, present := epoch.artifactProducerReads.byFactor[key]
		if !present || !artifactProducerReadConsumersValid(rows) || !artifactProducerReadConsumerPresent(rows, consumer) {
			return false
		}
	}
	// Canonical keys are unique by construction. Repeated exact observations
	// may therefore only contribute one producer row to one publication key.
	for index, key := range keys {
		if index > 0 && sameArtifactProducerReadKey(keys[index-1], key) {
			return false
		}
	}
	// Validate every destination before removing the old rows. A destination
	// may already contain this consumer only when it is one of this producer's
	// old keys and will be removed below; every other duplicate or malformed
	// bucket refuses without changing either sparse map.
	for _, key := range keys {
		rows, present := epoch.artifactProducerReads.byKey[key]
		if !present {
			continue
		}
		if len(rows) == 0 || !artifactProducerReadConsumersValid(rows) {
			return false
		}
		if artifactProducerReadConsumerPresent(rows, consumer) {
			if _, wasOld := oldKeys[key]; !wasOld {
				return false
			}
		}
	}
	for _, key := range factors {
		rows, present := epoch.artifactProducerReads.byFactor[key]
		if !present {
			continue
		}
		if len(rows) == 0 || !artifactProducerReadConsumersValid(rows) {
			return false
		}
		if artifactProducerReadConsumerPresent(rows, consumer) {
			if _, wasOld := oldFactorSet[key]; !wasOld {
				return false
			}
		}
	}
	for _, key := range cache.artifactReadKeys {
		rows := epoch.artifactProducerReads.byKey[key]
		rows = removeArtifactProducerReadConsumer(rows, consumer)
		if len(rows) == 0 {
			delete(epoch.artifactProducerReads.byKey, key)
		} else {
			epoch.artifactProducerReads.byKey[key] = rows
		}
	}
	for _, key := range cache.artifactReadFactorKeys {
		rows := epoch.artifactProducerReads.byFactor[key]
		rows = removeArtifactProducerReadConsumer(rows, consumer)
		if len(rows) == 0 {
			delete(epoch.artifactProducerReads.byFactor, key)
		} else {
			epoch.artifactProducerReads.byFactor[key] = rows
		}
	}
	for _, key := range keys {
		epoch.artifactProducerReads.byKey[key] = insertArtifactProducerReadConsumer(epoch.artifactProducerReads.byKey[key], consumer)
	}
	for _, key := range factors {
		epoch.artifactProducerReads.byFactor[key] = insertArtifactProducerReadConsumer(epoch.artifactProducerReads.byFactor[key], consumer)
	}
	cache.artifactReadKeys = append(cache.artifactReadKeys[:0], keys...)
	cache.artifactReadFactorKeys = append(cache.artifactReadFactorKeys[:0], factors...)
	return true
}

// clearArtifactProducerReads removes a completed producer from the sparse
// inverse before a Region restart drops its candidate/read cache.
func (epoch *executorEpoch) clearArtifactProducerReads(stateIndex, group int) bool {
	return epoch.replaceArtifactProducerReads(stateIndex, group, nil)
}

// markArtifactProducerReadConsumers is the contextual publication wake. It
// considers only exact Units carried by this factor publication and only
// producer rows already present in the completed-read inverse; no graph-point
// or closure-wide expansion is permitted.
func (epoch *executorEpoch) markArtifactProducerReadConsumers(sourceState int, changes carrier.ChangeSet) bool {
	if epoch == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || epoch.artifactProducerReads.byKey == nil || epoch.artifactProducerReads.byFactor == nil || sourceState < 0 || !epoch.activeState(sourceState) || !epoch.runtime.carrier.OwnsChangeSet(changes) {
		return false
	}
	if changes.Count() != 0 || changes.FactorCount() != 0 {
		if _, slotsOK := changes.Slots(); !slotsOK {
			return false
		}
	}
	seen := make(map[stateGroupKey]struct{})
	wake := func(consumers []stateGroupKey) bool {
		if !artifactProducerReadConsumersValid(consumers) {
			return false
		}
		for _, consumer := range consumers {
			if _, duplicate := seen[consumer]; duplicate {
				continue
			}
			if consumer.state < 0 || !epoch.activeState(int(consumer.state)) || consumer.group < 0 {
				return false
			}
			cache, rowExists := epoch.producerCache(consumer.state, consumer.group)
			if !rowExists || cache == nil || cache.state != consumer.state || cache.group != consumer.group {
				return false
			}
			seen[consumer] = struct{}{}
			if !epoch.markDirtyForState(int(consumer.state), consumer.group) {
				return false
			}
		}
		return true
	}
	for index := 0; index < changes.Count(); index++ {
		row, rowOK := changes.At(index)
		if !rowOK || !row.Region().Valid() || support.Empty(row.Region()) || row.Unit() == (carrier.Unit{}) {
			return false
		}
		unit := row.Unit()
		slot, slotOK := unit.Slot()
		if !slotOK || !epoch.runtime.carrier.OwnsUnit(slot, unit) {
			return false
		}
		if unit.Kind() != carrier.ExactUnit {
			continue
		}
		consumers, present := epoch.artifactProducerReads.byKey[artifactProducerReadKey{state: contextfiber.StateOrdinal(sourceState), unit: unit}]
		if present && len(consumers) == 0 {
			return false
		}
		if !wake(consumers) {
			return false
		}
	}
	for index := 0; index < changes.FactorCount(); index++ {
		row, rowOK := changes.FactorAt(index)
		if !rowOK || !row.Region().Valid() || support.Empty(row.Region()) || row.Slot() < 0 || int(row.Slot()) >= epoch.runtime.carrier.Count() {
			return false
		}
		consumers, present := epoch.artifactProducerReads.byFactor[artifactProducerReadFactorKey{state: contextfiber.StateOrdinal(sourceState), slot: row.Slot()}]
		if present && len(consumers) == 0 {
			return false
		}
		if !wake(consumers) {
			return false
		}
	}
	return true
}
