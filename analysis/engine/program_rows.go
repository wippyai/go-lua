package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// programRows is the constructor's private row workspace. It has no caller
// visible lifecycle: ConstructProgram creates it, seals the source Batch once,
// and passes only the finished row declaration to constructTopology. The
// workspace cannot be retained or reopened by a domain owner.
type programRows struct {
	state       *schemaBindingState
	authority   *schemaBindingAuthority
	binding     *SchemaBinding
	batch       *equation.Batch
	sourceKey   composition.Key
	spec        equation.TopologySpec
	mountedRows *mountedArtifactRows
}

func newProgramRows(binding *SchemaBinding) (*programRows, bool) {
	state := bindingState(binding)
	if state == nil {
		return nil, false
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != schemaBindingSealed || state.authority == nil || state.schema == nil || len(state.factors) != schemaFactorCount(state.schema) {
		return nil, false
	}
	for _, factor := range state.factors {
		if factor == nil || !factor.schemaFactorComplete() {
			return nil, false
		}
	}
	batch := equation.NewBatch()
	rows := &programRows{
		state: state, authority: state.authority, binding: binding, batch: batch,
	}
	rows.spec.Batch = batch
	return rows, true
}

func (rows *programRows) admitSite(source composition.Key, scope equation.Scope, init equation.Expr, disposition equation.InitDisposition) (equation.Site, bool) {
	if rows == nil || rows.batch == nil || rows.batch.Sealed() {
		return equation.Site{}, false
	}
	return rows.batch.AdmitSite(source, scope, init, disposition)
}

func (rows *programRows) admitFrom(site equation.Site, entity composition.Key) (equation.Occurrence, bool) {
	if rows == nil || rows.batch == nil || rows.batch.Sealed() {
		return equation.Occurrence{}, false
	}
	return rows.batch.From(site, entity)
}

func (rows *programRows) admitOperand(occurrence equation.Occurrence, entity composition.Key) (equation.Operand, bool) {
	if rows == nil || rows.batch == nil || rows.batch.Sealed() {
		return equation.Operand{}, false
	}
	return rows.batch.AdmitOperand(occurrence, entity)
}

func (rows *programRows) setMountedRows(mounted *mountedArtifactRows) bool {
	if rows == nil || rows.batch == nil || rows.batch.Sealed() || rows.mountedRows != nil || mounted == nil {
		return false
	}
	rows.mountedRows = mounted
	return true
}

func (rows *programRows) seal() equation.SealFailure {
	if rows == nil || rows.batch == nil || rows.batch.Sealed() || rows.spec.Batch != rows.batch {
		return equation.SealFailureSourcePrecondition
	}
	if failure := rows.batch.SealWithFailure(); failure.Available() {
		return failure
	}
	key := rows.batch.Key()
	if !rows.batch.Sealed() || !key.Available() || rows.spec.Batch != rows.batch {
		return equation.SealFailureSourceBatchIdentity
	}
	rows.sourceKey = key
	return equation.SealFailure{}
}

type programSealFailurePhase uint8

const (
	programSealFailureNone programSealFailurePhase = iota
	programSealFailureSources
	programSealFailureArtifactRows
	programSealFailureRuleRow
	programSealFailureQueryBatch
)

type programSourceSealFailure = equation.SealFailure

var (
	programSourceSealFailurePrecondition  = equation.SealFailureSourcePrecondition
	programSourceSealFailureBatchIdentity = equation.SealFailureSourceBatchIdentity
)

// programSealFailure is scalar refusal evidence for the declaration seal.
// It never carries a mutable transaction or a recoverable row object.
type programSealFailure struct {
	phase    programSealFailurePhase
	ordinal  uint32
	source   programSourceSealFailure
	mounted  RuleSlotCapability
	link     RuleSlotCapability
	artifact programArtifactRowFailure
}

func (failure programSealFailure) Phase() programSealFailurePhase { return failure.phase }
func (failure programSealFailure) Ordinal() uint32                { return failure.ordinal }
func (failure programSealFailure) Source() (programSourceSealFailure, bool) {
	return failure.source, failure.phase == programSealFailureSources && failure.source.Available()
}
func (failure programSealFailure) MountedCapability() (RuleSlotCapability, bool) {
	return failure.mounted, failure.phase == programSealFailureRuleRow && failure.mounted.mounted() && !failure.link.available()
}
func (failure programSealFailure) LinkCapability() (RuleSlotCapability, bool) {
	return failure.link, failure.phase == programSealFailureRuleRow && failure.link.link() && !failure.mounted.available()
}
func (failure programSealFailure) ArtifactRow() (programArtifactRowFailure, bool) {
	return failure.artifact, failure.phase == programSealFailureArtifactRows && failure.artifact != programArtifactRowFailureNone
}
func (failure programSealFailure) Failure() SolveFailure {
	if failure.phase == programSealFailureNone {
		return SolveFailure{}
	}
	sourceFamily, sourceSite := failure.source.Ordinals()
	return boundaryFailure(SolveFailureFamilyCompile, "program-seal", uint64(failure.phase), uint64(failure.ordinal), sourceFamily, sourceSite, uint64(failure.artifact))
}

func bindingOwnsFactorSchema(schema *Schema, key composition.Key) bool {
	if schema == nil {
		return false
	}
	_, ok := schema.factorOrdinalOf(key)
	return ok
}

func bindingOwnsRuleSchema(schema *Schema, key composition.Key) bool {
	if schema == nil {
		return false
	}
	_, ok := schema.ruleOrdinalOf(key)
	return ok
}

func bindingOwnsQuerySchema(schema *Schema, key composition.Key) bool {
	if schema == nil || !key.Available() {
		return false
	}
	_, ok := schema.queryOrdinalOf(key)
	return ok
}

func validateBindingRuleRows(schema *Schema, rule equation.RuleInstance) bool {
	if schema == nil || !rule.ValidFor(schema.cold) {
		return false
	}
	ordinal, found := schema.ruleOrdinalOf(rule.Schema)
	shape, shapeOK := schema.ruleShapeAt(ordinal)
	if !found || !shapeOK || rule.OperandFamily != shape.OperandFamily || uint64(len(rule.Reads)) != shape.ReadCount || uint64(len(rule.Carries)) != shape.CarryCount || uint64(len(rule.Writes)) != shape.WriteCount {
		return false
	}
	for index, read := range rule.Reads {
		want, ok := schema.ruleReadShapeAt(ordinal, uint64(index))
		if !ok || read.Index != uint64(index) || !validBindingReadSurface(want, read.Surface) {
			return false
		}
	}
	for index, carry := range rule.Carries {
		want, ok := schema.ruleCarryShapeAt(ordinal, uint64(index))
		if !ok || carry.Index != uint64(index) || !bindingOwnsFactorSchema(schema, want.Factor) {
			return false
		}
	}
	for index, write := range rule.Writes {
		want, ok := schema.ruleWriteShapeAt(ordinal, uint64(index))
		if !ok || write.Index != uint64(index) || !validBindingWriteSurface(want, write) {
			return false
		}
	}
	return true
}

func validBindingReadSurface(shape composition.RuleReadShape, surface equation.Surface) bool {
	if !surface.Available() || surface.Factor != shape.Factor || surface.Mode != equation.TargetModeNone {
		return false
	}
	switch shape.Kind {
	case composition.ReadExact:
		return surface.Form == equation.SurfaceReadExact && !surface.Semantic.Available() && !surface.Normalizer.Available()
	case composition.ReadSummary:
		return surface.Form == equation.SurfaceReadSummary && surface.Semantic == shape.Normalizer && surface.Normalizer == shape.Normalizer && shape.Normalizer.Available()
	case composition.ReadSelect:
		return surface.Form == equation.SurfaceReadSelect && surface.Semantic == shape.Semantic && !surface.Normalizer.Available() && shape.Semantic.Available()
	default:
		return false
	}
}

func validBindingWriteSurface(shape composition.RuleWriteShape, write equation.ResolvedWrite) bool {
	if !write.Surface.Available() || write.Surface.Factor != shape.Factor || write.Route != shape.Route {
		return false
	}
	switch shape.Kind {
	case composition.WriteExact:
		return write.Surface.Form == equation.SurfaceWriteExact
	case composition.WriteRoute:
		return write.Surface.Form == equation.SurfaceWriteRoute
	default:
		return false
	}
}

func bindingOwnsInput(batch *equation.Batch, input equation.Input) bool {
	return batch != nil && input.Available() && batch.OwnsSite(input.Source()) && batch.OwnsSite(input.Target())
}

func validBindingGroup(batch *equation.Batch, group equation.Group) bool {
	if batch == nil || len(group.Members) == 0 || group.Output == 0 {
		return false
	}
	for _, input := range group.Inputs {
		if !bindingOwnsInput(batch, input) {
			return false
		}
	}
	return !group.EnvironmentInput.Available() || bindingOwnsInput(batch, group.EnvironmentInput)
}

func ruleInputReindex(source, target equation.Scope) (equation.Reindex, bool) {
	if !source.Available() || !target.Available() {
		return equation.Reindex{}, false
	}
	targets := make(map[composition.Key]equation.Decision, target.Count())
	for index := 0; index < target.Count(); index++ {
		decision, ok := target.At(index)
		if !ok || !decision.Available() {
			return equation.Reindex{}, false
		}
		targets[decision.Key()] = decision
	}
	maps := make([]equation.DecisionMap, source.Count())
	for index := range maps {
		decision, ok := source.At(index)
		if !ok || !decision.Available() {
			return equation.Reindex{}, false
		}
		if targetDecision, retained := targets[decision.Key()]; retained {
			maps[index] = equation.Identity(decision)
			if targetDecision != decision {
				maps[index] = equation.Rename(decision, targetDecision)
			}
		} else {
			maps[index] = equation.Forget(decision)
		}
	}
	return equation.NewReindex(source, target, maps)
}

func cloneBindingRuleRow(row equation.RuleInstance) equation.RuleInstance {
	row.Reads = append([]equation.ResolvedRead(nil), row.Reads...)
	row.Carries = append([]equation.ResolvedCarry(nil), row.Carries...)
	row.Writes = append([]equation.ResolvedWrite(nil), row.Writes...)
	row.Supports = append([]equation.ResolvedSupport(nil), row.Supports...)
	row.Prunes = append([]equation.ResolvedPrune(nil), row.Prunes...)
	return row
}
