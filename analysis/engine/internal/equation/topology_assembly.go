package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// TopologyAssembly is the closed batch-composition receipt for one ordinary
// base Batch and its exact template-materialization outputs. It accepts no
// arbitrary Batch list: every target must carry the TemplateBinding proving it
// was materialized against this exact base. Assembly reissues one canonical
// directory Batch, so later topology compilation never needs mixed-Batch
// capabilities.
type TopologyAssembly struct{ data *topologyAssemblyData }

type topologyAssemblyData struct {
	base      *Batch
	directory *Batch
	key       composition.Key
	entries   map[assemblyRowKey]assemblyRows
	inputs    map[composition.Key]Input
	targets   []TopologySpec
}

type assemblyRowKey struct {
	batch *Batch
	row   uint32
	kind  uint8
}

const (
	assemblySiteRow uint8 = iota + 1
	assemblyOccurrenceRow
	assemblyOperandRow
)

type assemblyRows struct {
	site       Site
	occurrence Occurrence
	operand    Operand
}

// SealTopologyAssembly consumes the base Batch and only target batches issued
// by MaterializeTemplateBoundary through a binding whose actual Batch is that
// exact base. It is deliberately one-shot and non-generic.
func SealTopologyAssembly(base *Batch, values []TemplateMaterialization) (TopologyAssembly, bool) {
	if base == nil || !base.Sealed() {
		return TopologyAssembly{}, false
	}
	if len(values) == 0 {
		key, ok := topologyAssemblyKey(base, nil, base, map[composition.Key]Input{})
		if !ok {
			return TopologyAssembly{}, false
		}
		return TopologyAssembly{data: &topologyAssemblyData{base: base, directory: base, key: key, entries: map[assemblyRowKey]assemblyRows{}, inputs: map[composition.Key]Input{}}}, true
	}
	ordered := append([]TemplateMaterialization(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return lessKey(ordered[left].Key(), ordered[right].Key()) })
	seenKeys := make(map[composition.Key]struct{}, len(ordered))
	seenBatches := make(map[*Batch]struct{}, len(ordered))
	for _, value := range ordered {
		if !value.Available() || value.data == nil || value.data.batch == nil || value.data.binding == nil || value.data.binding.actuals != base {
			return TopologyAssembly{}, false
		}
		if _, duplicate := seenKeys[value.Key()]; duplicate {
			return TopologyAssembly{}, false
		}
		if _, duplicate := seenBatches[value.data.batch]; duplicate || value.data.batch == base {
			return TopologyAssembly{}, false
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
	entries := make(map[assemblyRowKey]assemblyRows)
	for _, batch := range batches {
		for index, row := range batch.sites {
			if row.formal || !validSiteSource(row.source, row.scope, row.init, row.disposition) {
				return TopologyAssembly{}, false
			}
			site, ok := directory.AdmitSite(row.source, row.scope, row.init, row.disposition)
			if !ok {
				return TopologyAssembly{}, false
			}
			entries[assemblyRowKey{batch: batch, row: uint32(index + 1), kind: assemblySiteRow}] = assemblyRows{site: site}
		}
	}
	for _, batch := range batches {
		for index, row := range batch.occurrences {
			siteRows, present := entries[assemblyRowKey{batch: batch, row: row.site, kind: assemblySiteRow}]
			if !present || !directory.OwnsOpenSite(siteRows.site) {
				return TopologyAssembly{}, false
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
				return TopologyAssembly{}, false
			}
			if !ok || !directory.OwnsOpenOccurrence(occurrence) {
				return TopologyAssembly{}, false
			}
			entry := entries[assemblyRowKey{batch: batch, row: row.site, kind: assemblySiteRow}]
			entry.occurrence = occurrence
			entries[assemblyRowKey{batch: batch, row: row.site, kind: assemblySiteRow}] = entry
			entries[assemblyRowKey{batch: batch, row: uint32(index + 1), kind: assemblyOccurrenceRow}] = assemblyRows{occurrence: occurrence}
		}
	}
	for _, batch := range batches {
		for index, row := range batch.operands {
			occurrenceRows, present := entries[assemblyRowKey{batch: batch, row: row.occurrence, kind: assemblyOccurrenceRow}]
			if !present || !directory.OwnsOpenOccurrence(occurrenceRows.occurrence) {
				return TopologyAssembly{}, false
			}
			operand, ok := directory.admitOperandInRealm(occurrenceRows.occurrence, row.entity, batch.key)
			if !ok {
				return TopologyAssembly{}, false
			}
			entries[assemblyRowKey{batch: batch, row: uint32(index + 1), kind: assemblyOperandRow}] = assemblyRows{operand: operand}
		}
	}
	if !directory.Seal() {
		return TopologyAssembly{}, false
	}
	inputs := make(map[composition.Key]Input)
	targets := make([]TopologySpec, 0, len(ordered))
	reissueInput := func(batch *Batch, input Input) (Input, bool) {
		if !input.Available() || input.Source().batch != batch || input.Target().batch != batch {
			return Input{}, false
		}
		sourceRows, sourceOK := entries[assemblyRowKey{batch: batch, row: input.Source().row, kind: assemblySiteRow}]
		targetRows, targetOK := entries[assemblyRowKey{batch: batch, row: input.Target().row, kind: assemblySiteRow}]
		if !sourceOK || !targetOK {
			return Input{}, false
		}
		result := BoundaryInput(sourceRows.site, targetRows.site, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
		return result, result.Available()
	}
	for _, value := range ordered {
		for _, input := range value.data.inputs {
			if !input.Available() || input.Source().batch != value.data.batch || input.Target().batch != value.data.batch {
				return TopologyAssembly{}, false
			}
			sourceRows, sourceOK := entries[assemblyRowKey{batch: value.data.batch, row: input.Source().row, kind: assemblySiteRow}]
			targetRows, targetOK := entries[assemblyRowKey{batch: value.data.batch, row: input.Target().row, kind: assemblySiteRow}]
			if !sourceOK || !targetOK {
				return TopologyAssembly{}, false
			}
			reissued := BoundaryInput(sourceRows.site, targetRows.site, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
			if !reissued.Available() {
				return TopologyAssembly{}, false
			}
			if existing, duplicate := inputs[input.Key()]; duplicate {
				if existing.Key() != reissued.Key() {
					return TopologyAssembly{}, false
				}
				continue
			}
			inputs[input.Key()] = reissued
		}
		// Standalone inputs authored in the formal/target Batch are part of
		// that same materialization receipt even when no Group or edge owns
		// them. Reissue them through the directory so topology compilation
		// never retains a target-Batch capability.
		targetInputs, targetInputsOK := value.data.batch.TargetInputRows()
		if !targetInputsOK {
			return TopologyAssembly{}, false
		}
		for _, input := range targetInputs {
			if !input.Available() || input.Source().batch != value.data.batch || input.Target().batch != value.data.batch {
				return TopologyAssembly{}, false
			}
			sourceRows, sourceOK := entries[assemblyRowKey{batch: value.data.batch, row: input.Source().row, kind: assemblySiteRow}]
			targetRows, targetOK := entries[assemblyRowKey{batch: value.data.batch, row: input.Target().row, kind: assemblySiteRow}]
			if !sourceOK || !targetOK {
				return TopologyAssembly{}, false
			}
			reissued := BoundaryInput(sourceRows.site, targetRows.site, input.Provenance(), input.Pre(), input.Reindex(), input.Post())
			if !reissued.Available() {
				return TopologyAssembly{}, false
			}
			if existing, duplicate := inputs[input.Key()]; duplicate {
				if existing.Key() != reissued.Key() {
					return TopologyAssembly{}, false
				}
				continue
			}
			inputs[input.Key()] = reissued
		}
		targetSpec, targetSpecOK := MaterializeTargetBatch(value.data.batch)
		if !targetSpecOK {
			return TopologyAssembly{}, false
		}
		reissuedTarget := TopologySpec{Batch: directory, Points: make([]PointSpec, len(targetSpec.Points)), PointRanks: append([]int(nil), targetSpec.PointRanks...), Rules: make([]RuleInstance, len(targetSpec.Rules)), Groups: make([]Group, len(targetSpec.Groups)), FactorEdges: make([]FactorEdge, len(targetSpec.FactorEdges)), EnvironmentEdges: make([]EnvironmentEdge, len(targetSpec.EnvironmentEdges)), Summaries: append([]SummaryMapping(nil), targetSpec.Summaries...), WeakTargets: append([]WeakTargetMapping(nil), targetSpec.WeakTargets...)}
		pointRefs := make(map[PointRef]PointRef, len(targetSpec.Points))
		for index, point := range targetSpec.Points {
			site, siteOK := assemblySiteFromEntries(entries, value.data.batch, point.Site)
			if !siteOK {
				return TopologyAssembly{}, false
			}
			reissuedTarget.Points[index] = PointSpec{Site: site}
			pointRefs[PointAt(index)] = PointAt(index)
		}
		for index, rule := range targetSpec.Rules {
			occurrence, occurrenceOK := assemblyOccurrenceFromEntries(entries, value.data.batch, rule.Occurrence)
			operand, operandOK := assemblyOperandFromEntries(entries, value.data.batch, rule.Operand)
			if !occurrenceOK || !operandOK {
				return TopologyAssembly{}, false
			}
			rule.Occurrence, rule.Operand = occurrence, operand
			reissuedTarget.Rules[index] = rule
		}
		for index, group := range targetSpec.Groups {
			output, outputOK := pointRefs[group.Output]
			if !outputOK {
				return TopologyAssembly{}, false
			}
			members := make([]RuleRef, len(group.Members))
			for memberIndex, member := range group.Members {
				if uint64(member) == 0 || uint64(member) > uint64(len(reissuedTarget.Rules)) {
					return TopologyAssembly{}, false
				}
				members[memberIndex] = member
			}
			bound := Group{Members: members, Output: output, premise: group.premise}
			for _, input := range group.Inputs {
				valueInput, inputOK := reissueInput(value.data.batch, input)
				if !inputOK {
					return TopologyAssembly{}, false
				}
				bound.Inputs = append(bound.Inputs, valueInput)
			}
			if group.EnvironmentInput.Available() {
				valueInput, inputOK := reissueInput(value.data.batch, group.EnvironmentInput)
				if !inputOK {
					return TopologyAssembly{}, false
				}
				bound.EnvironmentInput = valueInput
			}
			reissuedTarget.Groups[index] = bound
		}
		for index, edge := range targetSpec.FactorEdges {
			input, inputOK := reissueInput(value.data.batch, edge.Input)
			target, targetOK := pointRefs[edge.Target]
			if !inputOK || !targetOK {
				return TopologyAssembly{}, false
			}
			reissuedTarget.FactorEdges[index] = FactorEdge{Target: target, Input: input, Factor: edge.Factor}
		}
		for index, edge := range targetSpec.EnvironmentEdges {
			input, inputOK := reissueInput(value.data.batch, edge.Input)
			target, targetOK := pointRefs[edge.Target]
			if !inputOK || !targetOK {
				return TopologyAssembly{}, false
			}
			reissuedTarget.EnvironmentEdges[index] = EnvironmentEdge{Target: target, Input: input, TransportOnly: edge.TransportOnly}
		}
		targets = append(targets, reissuedTarget)
	}
	key, ok := topologyAssemblyKey(base, ordered, directory, inputs)
	if !ok {
		return TopologyAssembly{}, false
	}
	return TopologyAssembly{data: &topologyAssemblyData{base: base, directory: directory, key: key, entries: entries, inputs: inputs, targets: targets}}, true
}

func assemblySiteFromEntries(entries map[assemblyRowKey]assemblyRows, batch *Batch, site Site) (Site, bool) {
	row, ok := entries[assemblyRowKey{batch: batch, row: site.row, kind: assemblySiteRow}]
	return row.site, ok && row.site.Available()
}

func assemblyOccurrenceFromEntries(entries map[assemblyRowKey]assemblyRows, batch *Batch, occurrence Occurrence) (Occurrence, bool) {
	row, ok := entries[assemblyRowKey{batch: batch, row: occurrence.row, kind: assemblyOccurrenceRow}]
	return row.occurrence, ok && row.occurrence.Available()
}

func assemblyOperandFromEntries(entries map[assemblyRowKey]assemblyRows, batch *Batch, operand Operand) (Operand, bool) {
	row, ok := entries[assemblyRowKey{batch: batch, row: operand.row, kind: assemblyOperandRow}]
	return row.operand, ok && row.operand.Available()
}

func topologyAssemblyKey(base *Batch, values []TemplateMaterialization, directory *Batch, inputs map[composition.Key]Input) (composition.Key, bool) {
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
	return identityKey("analysis/engine/equation/topology-assembly", func(writer *canonical.DigestWriter) bool {
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

func (value TopologyAssembly) Available() bool {
	return value.data != nil && value.data.base != nil && value.data.base.Sealed() && value.data.directory != nil && value.data.directory.Sealed() && value.data.key.Available() && value.data.entries != nil && value.data.inputs != nil
}

func (value TopologyAssembly) Key() composition.Key {
	if !value.Available() {
		return composition.Key{}
	}
	return value.data.key
}

func (value TopologyAssembly) Batch() *Batch {
	if !value.Available() {
		return nil
	}
	return value.data.directory
}

func (value TopologyAssembly) Site(site Site) (Site, bool) {
	if !value.Available() || !site.Available() || site.dynamic != nil {
		return Site{}, false
	}
	row, ok := value.data.entries[assemblyRowKey{batch: site.batch, row: site.row, kind: assemblySiteRow}]
	return row.site, ok && row.site.Available() && row.site.batch == value.data.directory
}

func (value TopologyAssembly) Occurrence(occurrence Occurrence) (Occurrence, bool) {
	if !value.Available() || !occurrence.Available() || occurrence.dynamic != nil {
		return Occurrence{}, false
	}
	row, ok := value.data.entries[assemblyRowKey{batch: occurrence.batch, row: occurrence.row, kind: assemblyOccurrenceRow}]
	return row.occurrence, ok && row.occurrence.Available() && row.occurrence.batch == value.data.directory
}

func (value TopologyAssembly) Operand(operand Operand) (Operand, bool) {
	if !value.Available() || !operand.Available() || operand.dynamic != nil {
		return Operand{}, false
	}
	row, ok := value.data.entries[assemblyRowKey{batch: operand.batch, row: operand.row, kind: assemblyOperandRow}]
	return row.operand, ok && row.operand.Available() && row.operand.batch == value.data.directory
}

func (value TopologyAssembly) Input(input Input) (Input, bool) {
	if !value.Available() || !input.Available() {
		return Input{}, false
	}
	row, ok := value.data.inputs[input.Key()]
	return row, ok && row.Available() && row.Source().batch == value.data.directory && row.Target().batch == value.data.directory
}

func (value TopologyAssembly) Targets() []TopologySpec {
	if !value.Available() {
		return nil
	}
	result := make([]TopologySpec, len(value.data.targets))
	for index, target := range value.data.targets {
		result[index] = copyTopologySpec(target)
	}
	return result
}
