package equation

// This file is the equation-side landing point for the seq4161 activation
// cut.  ActivationRowSpec is a disposable admission recipe.  Once the
// topology seals, the only retained authority is activationRowDirectory: one
// sealed target Batch together with rows addressed by trigger, locator, and
// transport-row identity.  In particular, there is no retained per-candidate
// ownership witness or post-seal origin re-authentication step.

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/internal/canonical"
)

// ActivationRowSpec is a disposable input to Topology sealing.  A row may
// carry a formal target lowering (Binding/Sites/Inputs) and/or direct factor
// transport (Trigger/Entries/Exits/Imports/Export).  The two forms share one
// trigger and locator tuple and are fused into the same sealed directory.
//
// Binding remains the formal-port authority.  Its rows are consumed exactly
// once by the directory sealer, preserving the formal-port reindex and alpha
// substitution semantics without carrying a lowering artifact beyond
// this boundary.
type ActivationRowSpec struct {
	TriggerOrdinal int
	Family         composition.Key
	Application    composition.Key
	Target         composition.Key
	Endpoint       composition.Key
	// Context is the exact owner-authenticated context transition for a
	// mounted row.  Empty is retained only for unqualified programs; a
	// mounted row must carry the complete transition tuple.
	Context ActivationContext

	Binding TemplateBinding
	Sites   []Site
	Inputs  []Input

	Trigger PointRef
	Entries []PointRef
	Exits   []PointRef
	Imports []composition.Key
	Export  composition.Key
}

type activationRowLocator struct {
	triggerOrdinal int
	family         composition.Key
	application    composition.Key
	target         composition.Key
	endpoint       composition.Key
	context        ActivationContext
}

type activationTransportRow struct {
	id     composition.Key
	source PointRef
	target PointRef
	factor composition.Key
}

type activationDirectoryRow struct {
	key       composition.Key
	locator   activationRowLocator
	target    *Batch
	transport []activationTransportRow
}

// activationRowDirectory is deliberately private.  It is the sole retained
// activation authority after Topology sealing; callers can only ask the
// Topology to mint a Member for an exact trigger/locator tuple.
type activationRowDirectory struct {
	base    *Batch
	batch   *Batch
	key     composition.Key
	rows    []activationDirectoryRow
	byKey   map[composition.Key]int
	byTuple map[activationRowLocator]composition.Key
	entries map[loweringRowKey]loweringRows
	inputs  map[composition.Key]Input
	targets []TopologySpec
}

func (directory *activationRowDirectory) available() bool {
	return directory != nil && directory.base != nil && directory.base.Sealed() && directory.batch != nil && directory.batch.Sealed() && directory.key.Available() && directory.byKey != nil && directory.byTuple != nil && directory.entries != nil && directory.inputs != nil
}

// sealActivationRowDirectory consumes all lowering recipes in one operation.
// The lowerer and the target Batch are implementation details of this
// function; no target artifact is stored in the returned directory.
func sealActivationRowDirectory(source *composition.Composition, base *Batch, specs []ActivationRowSpec) (*activationRowDirectory, bool) {
	if source == nil || !source.ID().Available() || base == nil || !base.Sealed() {
		return nil, false
	}

	// Formal target rows are lowered before the single batch-row pass. The
	// temporary values die at return; the directory retains only reissued rows.
	lowered := make([]activationTargetRows, 0, len(specs))
	loweredAt := make(map[int]activationTargetRows, len(specs))
	for index, spec := range specs {
		if !activationRowSpecShape(spec) {
			return nil, false
		}
		if spec.Binding.Available() {
			value, ok := lowerActivationTargetRows(source, spec.Binding, spec.Sites, spec.Inputs)
			if !ok || !value.Available() {
				return nil, false
			}
			lowered = append(lowered, value)
			loweredAt[index] = value
		}
	}
	lowering, ok := sealActivationBatchRows(base, lowered)
	if !ok || !lowering.available() {
		return nil, false
	}

	// Lowering targets are already in canonical key order.  Build
	// the row lookup by target Batch identity so each activation row receives
	// the exact target graph delta it lowered.
	targetByBatch := make(map[*Batch]TopologySpec, len(lowering.targetsValue()))
	for _, target := range lowering.targetsValue() {
		if target.Batch == nil || target.Batch != lowering.batchValue() {
			return nil, false
		}
		targetByBatch[target.Batch] = target
	}
	rows := make([]activationDirectoryRow, 0, len(specs))
	byKey := make(map[composition.Key]int, len(specs))
	byTuple := make(map[activationRowLocator]composition.Key, len(specs))
	for index, spec := range specs {
		locator := activationRowLocator{triggerOrdinal: spec.TriggerOrdinal, family: spec.Family, application: spec.Application, target: spec.Target, endpoint: spec.Endpoint, context: spec.Context}
		if _, duplicate := byTuple[locator]; duplicate {
			return nil, false
		}
		var target *Batch
		if _, loweredOK := loweredAt[index]; loweredOK {
			// The old lowering transaction has a private target Batch, while
			// lowering reissues all target rows into its one directory Batch.
			// Publish only the latter; the former never crosses this boundary.
			target = lowering.batchValue()
			if target == nil || target == base || targetByBatch[target].Batch == nil {
				return nil, false
			}
		}
		transport, ok := activationTransportRows(source, spec)
		if !ok {
			return nil, false
		}
		key, ok := activationDirectoryRowKey(locator, target, transport)
		if !ok {
			return nil, false
		}
		if _, duplicate := byKey[key]; duplicate {
			return nil, false
		}
		row := activationDirectoryRow{key: key, locator: locator, target: target, transport: transport}
		byKey[key] = len(rows)
		byTuple[locator] = key
		rows = append(rows, row)
	}

	entries := make(map[loweringRowKey]loweringRows, len(lowering.data.entries))
	for key, value := range lowering.data.entries {
		entries[key] = value
	}
	inputs := make(map[composition.Key]Input, len(lowering.data.inputs))
	for key, value := range lowering.data.inputs {
		inputs[key] = value
	}
	directory := &activationRowDirectory{base: base, batch: lowering.batchValue(), rows: rows, byKey: byKey, byTuple: byTuple, entries: entries, inputs: inputs, targets: cloneTopologySpecs(lowering.targetsValue())}
	key, ok := activationDirectoryKey(directory)
	if !ok {
		return nil, false
	}
	directory.key = key
	return directory, directory.available()
}

func activationRowSpecShape(spec ActivationRowSpec) bool {
	if spec.TriggerOrdinal < 0 || !spec.Family.Available() || !spec.Application.Available() || !spec.Target.Available() || !spec.Endpoint.Available() || spec.Target == spec.Endpoint || !spec.Context.WellFormed() {
		return false
	}
	if !spec.Binding.Available() && (spec.Trigger == 0 || len(spec.Entries) == 0 || len(spec.Exits) == 0 || len(spec.Imports) == 0 || !spec.Export.Available()) {
		return false
	}
	if !spec.Binding.Available() && (len(spec.Sites) != 0 || len(spec.Inputs) != 0) {
		return false
	}
	if spec.Binding.Available() && (len(spec.Sites) == 0 || len(spec.Sites) != len(spec.Binding.data.formals.sites)) {
		return false
	}
	return true
}

func activationTransportRows(source *composition.Composition, spec ActivationRowSpec) ([]activationTransportRow, bool) {
	if !spec.Binding.Available() && source == nil {
		return nil, false
	}
	if spec.Trigger == 0 {
		if len(spec.Entries) != 0 || len(spec.Exits) != 0 || len(spec.Imports) != 0 || spec.Export.Available() {
			return nil, false
		}
		return nil, true
	}
	entries, entriesOK := canonicalDirectActivationRefs(spec.Entries)
	exits, exitsOK := canonicalDirectActivationRefs(spec.Exits)
	imports, importsOK := canonicalDirectActivationFactors(source, spec.Imports)
	if !entriesOK || !exitsOK || !importsOK || !spec.Export.Available() {
		return nil, false
	}
	if _, known := source.FactorIndex(spec.Export); !known {
		return nil, false
	}
	for _, factor := range imports {
		if factor == spec.Export {
			return nil, false
		}
	}
	rows := make([]activationTransportRow, 0, len(entries)*len(imports)+len(exits))
	for _, entry := range entries {
		for _, factor := range imports {
			id, ok := activationTransportRowKey(spec.Trigger, entry, factor)
			if !ok {
				return nil, false
			}
			rows = append(rows, activationTransportRow{id: id, source: spec.Trigger, target: entry, factor: factor})
		}
	}
	for _, exit := range exits {
		id, ok := activationTransportRowKey(exit, spec.Trigger, spec.Export)
		if !ok {
			return nil, false
		}
		rows = append(rows, activationTransportRow{id: id, source: exit, target: spec.Trigger, factor: spec.Export})
	}
	return rows, true
}

func activationTransportRowKey(source, target PointRef, factor composition.Key) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/activation-transport-row", func(writer *canonical.DigestWriter) bool {
		return writer.Uint(uint64(source)) == nil && writer.Uint(uint64(target)) == nil && writeKey(writer, factor)
	})
}

func activationDirectoryRowKey(locator activationRowLocator, target *Batch, transport []activationTransportRow) (composition.Key, bool) {
	return identityKey("analysis/engine/equation/activation-row", func(writer *canonical.DigestWriter) bool {
		if writer.Uint(uint64(locator.triggerOrdinal)) != nil || !writeKey(writer, locator.family) || !writeKey(writer, locator.application) || !writeKey(writer, locator.target) || !writeKey(writer, locator.endpoint) || !writeActivationContext(writer, locator.context) {
			return false
		}
		if target == nil {
			if writer.Uint(0) != nil {
				return false
			}
		} else if !target.Sealed() || writer.Uint(1) != nil || !writeKey(writer, target.Key()) {
			return false
		}
		if writer.Count(uint64(len(transport))) != nil {
			return false
		}
		for _, row := range transport {
			if !row.id.Available() || !writeKey(writer, row.id) {
				return false
			}
		}
		return true
	})
}

func activationDirectoryKey(directory *activationRowDirectory) (composition.Key, bool) {
	if directory == nil || directory.base == nil || directory.batch == nil || !directory.base.Sealed() || !directory.batch.Sealed() {
		return composition.Key{}, false
	}
	rows := append([]activationDirectoryRow(nil), directory.rows...)
	sort.Slice(rows, func(left, right int) bool { return lessKey(rows[left].key, rows[right].key) })
	return identityKey("analysis/engine/equation/activation-row-directory", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, directory.base.Key()) || !writeKey(writer, directory.batch.Key()) || writer.Count(uint64(len(rows))) != nil {
			return false
		}
		for _, row := range rows {
			if !writeKey(writer, row.key) {
				return false
			}
		}
		return true
	})
}

func (directory *activationRowDirectory) row(key composition.Key) (activationDirectoryRow, bool) {
	if !directory.available() || !key.Available() {
		return activationDirectoryRow{}, false
	}
	index, found := directory.byKey[key]
	if !found || index < 0 || index >= len(directory.rows) {
		return activationDirectoryRow{}, false
	}
	row := directory.rows[index]
	return row, row.key == key
}

func (directory *activationRowDirectory) rowFor(locator activationRowLocator) (activationDirectoryRow, bool) {
	if !directory.available() {
		return activationDirectoryRow{}, false
	}
	key, found := directory.byTuple[locator]
	if !found {
		return activationDirectoryRow{}, false
	}
	return directory.row(key)
}

func (directory *activationRowDirectory) site(batch *Batch, value Site) (Site, bool) {
	if !directory.available() || batch == nil || value.batch != batch || value.dynamic != nil || value.row == 0 {
		return Site{}, false
	}
	row, found := directory.entries[loweringRowKey{batch: batch, row: value.row, kind: loweringSiteRow}]
	return row.site, found && row.site.Available() && row.site.batch == directory.batch
}

func (directory *activationRowDirectory) occurrence(batch *Batch, value Occurrence) (Occurrence, bool) {
	if !directory.available() || batch == nil || value.batch != batch || value.dynamic != nil || value.row == 0 {
		return Occurrence{}, false
	}
	row, found := directory.entries[loweringRowKey{batch: batch, row: value.row, kind: loweringOccurrenceRow}]
	return row.occurrence, found && row.occurrence.Available() && row.occurrence.batch == directory.batch
}

func (directory *activationRowDirectory) operand(batch *Batch, value Operand) (Operand, bool) {
	if !directory.available() || batch == nil || value.batch != batch || value.dynamic != nil || value.row == 0 {
		return Operand{}, false
	}
	row, found := directory.entries[loweringRowKey{batch: batch, row: value.row, kind: loweringOperandRow}]
	return row.operand, found && row.operand.Available() && row.operand.batch == directory.batch
}

func (directory *activationRowDirectory) input(key composition.Key) (Input, bool) {
	if !directory.available() || !key.Available() {
		return Input{}, false
	}
	value, found := directory.inputs[key]
	return value, found && value.Available() && value.Source().batch == directory.batch && value.Target().batch == directory.batch
}

func (directory *activationRowDirectory) transports(key composition.Key) ([]DirectActivationTransport, bool) {
	row, ok := directory.row(key)
	if !ok {
		return nil, false
	}
	result := make([]DirectActivationTransport, len(row.transport))
	for index, value := range row.transport {
		result[index] = DirectActivationTransport{Source: value.source, Target: value.target, Factor: value.factor}
	}
	return result, true
}

func (directory *activationRowDirectory) targetSpecs() []TopologySpec {
	if directory == nil || !directory.available() {
		return nil
	}
	return cloneTopologySpecs(directory.targets)
}

func cloneTopologySpecs(values []TopologySpec) []TopologySpec {
	result := make([]TopologySpec, len(values))
	for index, value := range values {
		result[index] = copyTopologySpec(value)
	}
	return result
}

func cloneActivationRowSpecs(values []ActivationRowSpec) []ActivationRowSpec {
	if values == nil {
		return nil
	}
	result := make([]ActivationRowSpec, len(values))
	for index, value := range values {
		result[index] = ActivationRowSpec{
			TriggerOrdinal: value.TriggerOrdinal,
			Family:         value.Family,
			Application:    value.Application,
			Target:         value.Target,
			Endpoint:       value.Endpoint,
			Context:        value.Context,
			Binding:        value.Binding,
			Sites:          append([]Site(nil), value.Sites...),
			Inputs:         append([]Input(nil), value.Inputs...),
			Trigger:        value.Trigger,
			Entries:        append([]PointRef(nil), value.Entries...),
			Exits:          append([]PointRef(nil), value.Exits...),
			Imports:        append([]composition.Key(nil), value.Imports...),
			Export:         value.Export,
		}
	}
	return result
}

func cloneQueryInstances(rows []QueryInstance) []QueryInstance {
	result := make([]QueryInstance, len(rows))
	for index, row := range rows {
		result[index] = QueryInstance{Context: row.Context, Family: row.Family, Point: row.Point, Surfaces: append([]Surface(nil), row.Surfaces...)}
	}
	return result
}
