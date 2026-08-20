package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// activationBatchRows is the closed batch-composition temporary batch rows for one ordinary
// base Batch and its exact template-activation target outputs. It accepts no
// arbitrary Batch list: every target must carry the TemplateBinding proving it
// was lowered against this exact base. lowering reissues one canonical
// directory Batch, so later topology compilation never needs mixed-Batch
// capabilities.
type activationBatchRows struct{ data *activationBatchRowsData }

type activationBatchRowsData struct {
	base      *Batch
	directory *Batch
	key       composition.Key
	entries   map[loweringRowKey]loweringRows
	inputs    map[composition.Key]Input
	targets   []TopologySpec
}

type loweringRowKey struct {
	batch *Batch
	row   uint32
	kind  uint8
}

const (
	loweringSiteRow uint8 = iota + 1
	loweringOccurrenceRow
	loweringOperandRow
)

type loweringRows struct {
	site       Site
	occurrence Occurrence
	operand    Operand
}

// sealActivationBatchRows consumes the base Batch and only target batches issued
// by lowerActivationTargetRows through a binding whose actual Batch is that
// exact base. It is deliberately one-shot and non-generic.
func sealActivationBatchRows(base *Batch, values []activationTargetRows) (activationBatchRows, bool) {
	if base == nil || !base.Sealed() {
		return activationBatchRows{}, false
	}
	if len(values) == 0 {
		key, ok := activationBatchRowsKey(base, nil, base, map[composition.Key]Input{})
		if !ok {
			return activationBatchRows{}, false
		}
		return activationBatchRows{data: &activationBatchRowsData{base: base, directory: base, key: key, entries: map[loweringRowKey]loweringRows{}, inputs: map[composition.Key]Input{}}}, true
	}
	ordered := append([]activationTargetRows(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return lessKey(ordered[left].Key(), ordered[right].Key()) })
	seenKeys := make(map[composition.Key]struct{}, len(ordered))
	seenBatches := make(map[*Batch]struct{}, len(ordered))
	for _, value := range ordered {
		if !value.Available() || value.data == nil || value.data.batch == nil || value.data.binding == nil || value.data.binding.actuals != base {
			return activationBatchRows{}, false
		}
		if _, duplicate := seenKeys[value.Key()]; duplicate {
			return activationBatchRows{}, false
		}
		if _, duplicate := seenBatches[value.data.batch]; duplicate || value.data.batch == base {
			return activationBatchRows{}, false
		}
		seenKeys[value.Key()] = struct{}{}
		seenBatches[value.data.batch] = struct{}{}
	}
	batches := make([]*Batch, 1, len(ordered)+1)
	batches[0] = base
	for _, value := range ordered {
		batches = append(batches, value.data.batch)
	}
	directory := NewBatch()
	entries := make(map[loweringRowKey]loweringRows)
	for _, batch := range batches {
		for index, row := range batch.sites {
			if row.formal || !validSiteSource(row.source, row.scope, row.init, row.disposition) {
				return activationBatchRows{}, false
			}
			site, ok := directory.AdmitSite(row.source, row.scope, row.init, row.disposition)
			if !ok {
				return activationBatchRows{}, false
			}
			entries[loweringRowKey{batch: batch, row: uint32(index + 1), kind: loweringSiteRow}] = loweringRows{site: site}
		}
	}
	for _, batch := range batches {
		for index, row := range batch.occurrences {
			siteRows, present := entries[loweringRowKey{batch: batch, row: row.site, kind: loweringSiteRow}]
			if !present || !directory.OwnsOpenSite(siteRows.site) {
				return activationBatchRows{}, false
			}
			var occurrence Occurrence
			var ok bool
			switch row.kind {
			case OccurrenceAt:
				occurrence, ok = directory.At(siteRows.site)
			case OccurrenceFrom:
				occurrence, ok = directory.From(siteRows.site, row.entity)
			case OccurrenceRelation:
				occurrence, ok = directory.Relation(siteRows.site, row.entity)
			default:
				return activationBatchRows{}, false
			}
			if !ok || !directory.OwnsOpenOccurrence(occurrence) {
				return activationBatchRows{}, false
			}
			entry := entries[loweringRowKey{batch: batch, row: row.site, kind: loweringSiteRow}]
			entry.occurrence = occurrence
			entries[loweringRowKey{batch: batch, row: row.site, kind: loweringSiteRow}] = entry
			entries[loweringRowKey{batch: batch, row: uint32(index + 1), kind: loweringOccurrenceRow}] = loweringRows{occurrence: occurrence}
		}
	}
	for _, batch := range batches {
		for index, row := range batch.operands {
			occurrenceRows, present := entries[loweringRowKey{batch: batch, row: row.occurrence, kind: loweringOccurrenceRow}]
			if !present || !directory.OwnsOpenOccurrence(occurrenceRows.occurrence) {
				return activationBatchRows{}, false
			}
			operand, ok := directory.admitOperandInRealm(occurrenceRows.occurrence, row.entity, batch.key)
			if !ok {
				return activationBatchRows{}, false
			}
			entries[loweringRowKey{batch: batch, row: uint32(index + 1), kind: loweringOperandRow}] = loweringRows{operand: operand}
		}
	}
	if !directory.Seal() {
		return activationBatchRows{}, false
	}
	inputs := make(map[composition.Key]Input)
	targets := make([]TopologySpec, 0, len(ordered))
	reissueInput := func(batch *Batch, input Input) (Input, bool) {
		if !input.Available() || input.Source().batch != batch || input.Target().batch != batch {
			return Input{}, false
		}
		sourceRows, sourceOK := entries[loweringRowKey{batch: batch, row: input.Source().row, kind: loweringSiteRow}]
		targetRows, targetOK := entries[loweringRowKey{batch: batch, row: input.Target().row, kind: loweringSiteRow}]
		if !sourceOK || !targetOK {
			return Input{}, false
		}
		result := BoundaryInput(sourceRows.site, targetRows.site, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
		return result, result.Available()
	}
	for _, value := range ordered {
		for _, input := range value.data.inputs {
			if !input.Available() || input.Source().batch != value.data.batch || input.Target().batch != value.data.batch {
				return activationBatchRows{}, false
			}
			sourceRows, sourceOK := entries[loweringRowKey{batch: value.data.batch, row: input.Source().row, kind: loweringSiteRow}]
			targetRows, targetOK := entries[loweringRowKey{batch: value.data.batch, row: input.Target().row, kind: loweringSiteRow}]
			if !sourceOK || !targetOK {
				return activationBatchRows{}, false
			}
			reissued := BoundaryInput(sourceRows.site, targetRows.site, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
			if !reissued.Available() {
				return activationBatchRows{}, false
			}
			if existing, duplicate := inputs[input.Key()]; duplicate {
				if existing.Key() != reissued.Key() {
					return activationBatchRows{}, false
				}
				continue
			}
			inputs[input.Key()] = reissued
		}
		// Standalone inputs authored in the formal/target Batch are part of
		// that same activation target temporary batch rows even when no Group or edge owns
		// them. Reissue them through the directory so topology compilation
		// never retains a target-Batch capability.
		targetInputs, targetInputsOK := value.data.batch.TargetInputRows()
		if !targetInputsOK {
			return activationBatchRows{}, false
		}
		for _, input := range targetInputs {
			if !input.Available() || input.Source().batch != value.data.batch || input.Target().batch != value.data.batch {
				return activationBatchRows{}, false
			}
			sourceRows, sourceOK := entries[loweringRowKey{batch: value.data.batch, row: input.Source().row, kind: loweringSiteRow}]
			targetRows, targetOK := entries[loweringRowKey{batch: value.data.batch, row: input.Target().row, kind: loweringSiteRow}]
			if !sourceOK || !targetOK {
				return activationBatchRows{}, false
			}
			reissued := BoundaryInput(sourceRows.site, targetRows.site, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
			if !reissued.Available() {
				return activationBatchRows{}, false
			}
			if existing, duplicate := inputs[input.Key()]; duplicate {
				if existing.Key() != reissued.Key() {
					return activationBatchRows{}, false
				}
				continue
			}
			inputs[input.Key()] = reissued
		}
		targetSpec, targetSpecOK := MaterializeTargetBatch(value.data.batch)
		if !targetSpecOK {
			return activationBatchRows{}, false
		}
		reissuedTarget := TopologySpec{Batch: directory, Points: make([]PointSpec, len(targetSpec.Points)), PointRanks: append([]int(nil), targetSpec.PointRanks...), Rules: make([]RuleInstance, len(targetSpec.Rules)), Groups: make([]Group, len(targetSpec.Groups)), FactorEdges: make([]FactorEdge, len(targetSpec.FactorEdges)), EnvironmentEdges: make([]EnvironmentEdge, len(targetSpec.EnvironmentEdges)), Summaries: append([]SummaryMapping(nil), targetSpec.Summaries...), WeakTargets: append([]WeakTargetMapping(nil), targetSpec.WeakTargets...)}
		pointRefs := make(map[PointRef]PointRef, len(targetSpec.Points))
		for index, point := range targetSpec.Points {
			site, siteOK := loweringSiteFromEntries(entries, value.data.batch, point.Site)
			if !siteOK {
				return activationBatchRows{}, false
			}
			reissuedTarget.Points[index] = PointSpec{Site: site}
			pointRefs[PointAt(index)] = PointAt(index)
		}
		for index, rule := range targetSpec.Rules {
			occurrence, occurrenceOK := loweringOccurrenceFromEntries(entries, value.data.batch, rule.Occurrence)
			operand, operandOK := loweringOperandFromEntries(entries, value.data.batch, rule.Operand)
			if !occurrenceOK || !operandOK {
				return activationBatchRows{}, false
			}
			rule.Occurrence, rule.Operand = occurrence, operand
			reissuedTarget.Rules[index] = rule
		}
		for index, group := range targetSpec.Groups {
			output, outputOK := pointRefs[group.Output]
			if !outputOK {
				return activationBatchRows{}, false
			}
			members := make([]RuleRef, len(group.Members))
			for memberIndex, member := range group.Members {
				if uint64(member) == 0 || uint64(member) > uint64(len(reissuedTarget.Rules)) {
					return activationBatchRows{}, false
				}
				members[memberIndex] = member
			}
			bound := Group{Members: members, Output: output, premise: group.premise}
			for _, input := range group.Inputs {
				valueInput, inputOK := reissueInput(value.data.batch, input)
				if !inputOK {
					return activationBatchRows{}, false
				}
				bound.Inputs = append(bound.Inputs, valueInput)
			}
			if group.EnvironmentInput.Available() {
				valueInput, inputOK := reissueInput(value.data.batch, group.EnvironmentInput)
				if !inputOK {
					return activationBatchRows{}, false
				}
				bound.EnvironmentInput = valueInput
			}
			reissuedTarget.Groups[index] = bound
		}
		for index, edge := range targetSpec.FactorEdges {
			input, inputOK := reissueInput(value.data.batch, edge.Input)
			target, targetOK := pointRefs[edge.Target]
			if !inputOK || !targetOK {
				return activationBatchRows{}, false
			}
			reissuedTarget.FactorEdges[index] = FactorEdge{Target: target, Input: input, Factor: edge.Factor}
		}
		for index, edge := range targetSpec.EnvironmentEdges {
			input, inputOK := reissueInput(value.data.batch, edge.Input)
			target, targetOK := pointRefs[edge.Target]
			if !inputOK || !targetOK {
				return activationBatchRows{}, false
			}
			reissuedTarget.EnvironmentEdges[index] = EnvironmentEdge{Target: target, Input: input, TransportOnly: edge.TransportOnly}
		}
		targets = append(targets, reissuedTarget)
	}
	key, ok := activationBatchRowsKey(base, ordered, directory, inputs)
	if !ok {
		return activationBatchRows{}, false
	}
	return activationBatchRows{data: &activationBatchRowsData{base: base, directory: directory, key: key, entries: entries, inputs: inputs, targets: targets}}, true
}

func loweringSiteFromEntries(entries map[loweringRowKey]loweringRows, batch *Batch, site Site) (Site, bool) {
	row, ok := entries[loweringRowKey{batch: batch, row: site.row, kind: loweringSiteRow}]
	return row.site, ok && row.site.Available()
}

func loweringOccurrenceFromEntries(entries map[loweringRowKey]loweringRows, batch *Batch, occurrence Occurrence) (Occurrence, bool) {
	row, ok := entries[loweringRowKey{batch: batch, row: occurrence.row, kind: loweringOccurrenceRow}]
	return row.occurrence, ok && row.occurrence.Available()
}

func loweringOperandFromEntries(entries map[loweringRowKey]loweringRows, batch *Batch, operand Operand) (Operand, bool) {
	row, ok := entries[loweringRowKey{batch: batch, row: operand.row, kind: loweringOperandRow}]
	return row.operand, ok && row.operand.Available()
}

func activationBatchRowsKey(base *Batch, values []activationTargetRows, directory *Batch, inputs map[composition.Key]Input) (composition.Key, bool) {
	if base == nil || !base.Sealed() || directory == nil || !directory.Sealed() {
		return composition.Key{}, false
	}
	keys := make([]composition.Key, 0, len(inputs))
	for key, input := range inputs {
		if !key.Available() || !input.Available() {
			return composition.Key{}, false
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return lessKey(keys[left], keys[right]) })
	return identityKey("analysis/engine/equation/topology-lowering", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, base.Key()) || !writeKey(writer, directory.Key()) || writer.Count(uint64(len(values))) != nil {
			return false
		}
		for _, value := range values {
			if !writeKey(writer, value.Key()) {
				return false
			}
		}
		if writer.Count(uint64(len(keys))) != nil {
			return false
		}
		for _, key := range keys {
			if !writeKey(writer, key) {
				return false
			}
		}
		return true
	})
}

func (value activationBatchRows) available() bool {
	return value.data != nil && value.data.base != nil && value.data.base.Sealed() && value.data.directory != nil && value.data.directory.Sealed() && value.data.key.Available() && value.data.entries != nil && value.data.inputs != nil
}

func (value activationBatchRows) keyValue() composition.Key {
	if !value.available() {
		return composition.Key{}
	}
	return value.data.key
}

func (value activationBatchRows) batchValue() *Batch {
	if !value.available() {
		return nil
	}
	return value.data.directory
}

func (value activationBatchRows) site(site Site) (Site, bool) {
	if !value.available() || !site.Available() || site.dynamic != nil {
		return Site{}, false
	}
	row, ok := value.data.entries[loweringRowKey{batch: site.batch, row: site.row, kind: loweringSiteRow}]
	return row.site, ok && row.site.Available() && row.site.batch == value.data.directory
}

func (value activationBatchRows) occurrence(occurrence Occurrence) (Occurrence, bool) {
	if !value.available() || !occurrence.Available() || occurrence.dynamic != nil {
		return Occurrence{}, false
	}
	row, ok := value.data.entries[loweringRowKey{batch: occurrence.batch, row: occurrence.row, kind: loweringOccurrenceRow}]
	return row.occurrence, ok && row.occurrence.Available() && row.occurrence.batch == value.data.directory
}

func (value activationBatchRows) operand(operand Operand) (Operand, bool) {
	if !value.available() || !operand.Available() || operand.dynamic != nil {
		return Operand{}, false
	}
	row, ok := value.data.entries[loweringRowKey{batch: operand.batch, row: operand.row, kind: loweringOperandRow}]
	return row.operand, ok && row.operand.Available() && row.operand.batch == value.data.directory
}

func (value activationBatchRows) input(input Input) (Input, bool) {
	if !value.available() || !input.Available() {
		return Input{}, false
	}
	row, ok := value.data.inputs[input.Key()]
	return row, ok && row.Available() && row.Source().batch == value.data.directory && row.Target().batch == value.data.directory
}

func (value activationBatchRows) targetsValue() []TopologySpec {
	if !value.available() {
		return nil
	}
	result := make([]TopologySpec, len(value.data.targets))
	for index, target := range value.data.targets {
		result[index] = copyTopologySpec(target)
	}
	return result
}
