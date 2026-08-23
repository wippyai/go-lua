package engine

// This file is the first, deliberately narrow, bridge from the sealed Rule
// program plan to the legacy engine cold composition. It owns no domain
// algebra and accepts no caller-authored Rule geometry: every row below is
// derived from ruleplan.Plan and its sealed axis directory.

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/generated"
	coldcomposition "github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// generatedRuleCell is the engine-only half retained behind a
// GeneratedRuleSlot. The public slot remains an opaque schema identity; this
// cell is deliberately not a generic V/O Rule implementation. The generated
// execution capability is intentionally not retained here; the remaining
// fields are immutable plan-fenced geometry for a later binding phase.
type generatedRuleCell struct {
	planDigest identity.ContentID
	// program is the one Plan-derived, type-neutral descriptor retained for
	// this canonical Rule ordinal. It is copied into Schema's ordinal table at
	// Seal; member rows never reconstruct it from occurrence geometry.
	program generated.CompiledRule

	rule      uint32
	inputs    uint32
	candidate ruleplan.RelationAddr
	join      ruleplan.RelationAddr
	key       ruleplan.ProjectionAddr
	reducer   ruleplan.ReducerAddr

	readInput uint32
	readAxis  uint32
	read      coldcomposition.Key

	output      ruleplan.OutputAddr
	destination ruleplan.ProjectionAddr
	write       coldcomposition.Key

	carryInput uint32
}

// generatedRuleCellAvailable is intentionally private. A later generated
// binding pass can recover the cell from engine code, but no domain package
// can inspect or manufacture its plan geometry.
func (cell *generatedRuleCell) available() bool {
	if cell == nil || !cell.planDigest.Available() || !cell.write.Available() || !cell.program.Available() {
		return false
	}
	if cell.program.ReadCount() == 0 {
		return !cell.read.Available()
	}
	return cell.read.Available()
}

// DeclareGeneratedRuleSlot derives one temporary legacy cold Rule row from
// the Plan at ruleOrdinal. The Plan is resolved from catalog inside this
// constructor; callers cannot provide a detached Plan, semantic identity, or
// any read/write/carry geometry. Runtime Program capabilities are deliberately
// absent from this cold declaration path; a later binding phase owns them.
//
// The admitted vertical is one exact or routed factor output, an ordered table
// of Exact/Selected joins, and an optional identity carry. The descriptor is
// the canonical copy of every Plan row; the cold composition row below is only
// the existing engine shape projection and never a source for missing Plan
// metadata. Summary/Complete reads, structural outputs, and transformed
// carries remain explicit seal refusals.
func DeclareGeneratedRuleSlot(
	builder *SchemaBuilder,
	catalog ruleplan.Catalog,
	ruleOrdinal uint32,
) (*GeneratedRuleSlot, bool) {
	if builder == nil {
		return nil, false
	}
	refuse := func() (*GeneratedRuleSlot, bool) {
		// A generated plan cannot be safely replaced by a legacy caller-shaped
		// declaration after any failed admission. Poisoning is the builder's
		// existing transactional refusal boundary.
		builder.poison()
		return nil, false
	}
	if !builderOpen(builder) || (builder.phase != schemaBuilderFactors && builder.phase != schemaBuilderChildren) ||
		!catalog.Available() {
		return refuse()
	}

	// Catalog.At is the sole authority for both rule bounds and the immutable
	// Plan geometry. A caller cannot splice a detached Plan into this path.
	compiled, canonicalOK := catalog.At(int(ruleOrdinal))
	if !canonicalOK || !compiled.Present() || compiled.Rule() != ruleOrdinal {
		return refuse()
	}
	semantic := compiled.Semantic()
	operandFamily := compiled.OperandFamily()
	if !semantic.Available() || !operandFamily.Available() || !identity.DistinctKeys(semantic, operandFamily) {
		return refuse()
	}
	// Plan axes and solver Factors are different sealed directories. The Plan
	// axis ordinal is only meaningful in catalog space; the generated runtime
	// descriptor must carry the canonical Factor ordinal. Build that semantic
	// map once from the builder's Factor rows, then resolve only the axes the
	// admitted Plan actually references below. In particular, StorageEngine
	// axes in the catalog do not need a Factor row of their own.
	factorDirectory, factorDirectoryOK := buildGeneratedFactorDirectory(builder)
	if !factorDirectoryOK {
		return refuse()
	}

	// The generated descriptor owns the complete ordered join table. Keep only
	// the bounded output arity and scratch shape restrictions imposed by the
	// existing canonical table; do not collapse a transitive join graph into
	// one representative read.
	scratch := compiled.Scratch()
	if uint64(compiled.InputCount()) > schemaSlotMax || compiled.OutputCount() != 1 ||
		scratch.SourceCount != uint32(compiled.SourceCount()) || scratch.JoinCount != uint32(compiled.JoinCount()) ||
		scratch.FoldInputCount != uint32(compiled.FoldInputCount()) || scratch.OutputCount != 1 ||
		uint64(scratch.JoinCount) > uint64(^uint16(0)) || uint64(scratch.OutputCount) > uint64(^uint16(0)) {
		return refuse()
	}
	joins := make([]ruleplan.Join, compiled.JoinCount())
	for joinIndex := range joins {
		join, joinOK := compiled.JoinAt(joinIndex)
		if !joinOK || !generatedPlanJoinShape(compiled, joinIndex, join) {
			return refuse()
		}
		joins[joinIndex] = join
	}
	for inputIndex := 0; inputIndex < compiled.FoldInputCount(); inputIndex++ {
		foldInput, foldInputOK := compiled.FoldInputAt(inputIndex)
		if !foldInputOK || uint64(foldInput) >= uint64(compiled.JoinCount()) {
			return refuse()
		}
	}
	output, outputOK := compiled.OutputAt(0)
	if !outputOK {
		return refuse()
	}

	// Output and reducer addresses are checked against the same sealed axis
	// directory before their factors are selected. Frame/value-slot identity is
	// retained in the generated descriptor; only structural output is refused.
	if (output.Mode != ruleprogram.ModeExact && output.Mode != ruleprogram.ModeRoute) || output.Slot != 0 ||
		compiled.Reducer().Axis != output.Address.Axis {
		return refuse()
	}
	selectedCount := 0
	for joinIndex, join := range joins {
		if join.ReadForm == ruleprogram.Selected {
			selectedCount++
		}
		if output.Mode == ruleprogram.ModeRoute && output.RouteJoinPresent && output.RouteJoin == uint32(joinIndex) && join.ReadForm != ruleprogram.Selected {
			return refuse()
		}
	}
	if output.Mode == ruleprogram.ModeExact {
		if output.RouteJoinPresent || output.RouteJoin != 0 || output.Destination.Axis != compiled.Candidate().Axis {
			return refuse()
		}
	} else if !output.RouteJoinPresent || uint64(output.RouteJoin) >= uint64(len(joins)) || selectedCount != 1 || joins[output.RouteJoin].ReadForm != ruleprogram.Selected || output.Destination.Axis != joins[output.RouteJoin].Relation.Axis {
		return refuse()
	}
	// Validate all addresses while they still use the Plan's own axis
	// directory. This preserves the owner-local member/frame coordinates and
	// keeps the generated constructor's shape checks independent of mapping.
	if !generatedPlanMemberAddressesInRange(catalog, compiled.Candidate(), joins, compiled.Reducer(), output.Address, output.Destination) {
		return refuse()
	}
	outputFactor, outputFactorOK := generatedPlanFactor(factorDirectory, catalog, output.Address.Axis)
	if !outputFactorOK {
		return refuse()
	}
	normalizedCandidate, candidateOK := generatedRuntimeRelation(factorDirectory, catalog, compiled.Candidate())
	normalizedReducer, reducerOK := generatedRuntimeReducer(factorDirectory, catalog, compiled.Reducer())
	normalizedOutput, outputAddressOK := generatedRuntimeOutput(factorDirectory, catalog, output.Address)
	normalizedDestination, destinationOK := generatedRuntimeProjection(factorDirectory, catalog, output.Destination)
	normalizedOutputAxis, outputAxisOK := generatedRuntimeAxis(factorDirectory, catalog, output.Address.Axis)
	if !candidateOK || !reducerOK || !outputAddressOK || !destinationOK || !outputAxisOK {
		return refuse()
	}

	// Normalize every ordered join independently. Relation/key/predicate axes
	// are owner coordinates; ReadAxis is the heterogeneous runtime Factor
	// coordinate. None is inferred from another axis.
	readPlans := make([]generated.ReadPlan, len(joins))
	readFactors := make([]generatedFactorBinding, len(joins))
	for joinIndex, join := range joins {
		readFactor, factorOK := generatedPlanFactor(factorDirectory, catalog, join.ReadAxis)
		normalizedJoin, joinOK := generatedRuntimeRelation(factorDirectory, catalog, join.Relation)
		normalizedKey, keyOK := generatedRuntimeProjection(factorDirectory, catalog, join.Key)
		normalizedReadAxis, readAxisOK := generatedRuntimeAxis(factorDirectory, catalog, join.ReadAxis)
		if !factorOK || !joinOK || !keyOK || !readAxisOK {
			return refuse()
		}
		var normalizedPredicate ruleplan.ProjectionAddr
		if join.PredicatePresent {
			var predicateOK bool
			normalizedPredicate, predicateOK = generatedRuntimeProjection(factorDirectory, catalog, join.Predicate)
			if !predicateOK {
				return refuse()
			}
		}
		readFactors[joinIndex] = readFactor
		readPlans[joinIndex] = generated.ReadPlan{
			Input: join.Input, Factor: readFactor.ordinal, Axis: normalizedReadAxis,
			Sources: join.Sources, Relation: normalizedJoin, Key: normalizedKey,
			Predicate: normalizedPredicate, PredicatePresent: join.PredicatePresent,
			Form: join.ReadForm, Contract: join.ReadContract, Denominator: join.Denominator,
			RowCapacity: uint16(scratch.JoinCount), CellCapacity: uint16(scratch.OutputCount),
		}
	}

	// Identity carry is the only carry disposition representable without a
	// transform identity. A transform address is deliberately not converted
	// from ordinals into a fabricated semantic key. It is refused explicitly.
	carry, carryOK := compiled.Carry()
	if carryOK && (carry.Mode != ruleprogram.CarryIdentity || carry.TransformPresent || uint64(carry.Input) >= uint64(compiled.InputCount())) {
		return refuse()
	}
	if !builder.claim(semantic) {
		return nil, false
	}
	builder.phase = schemaBuilderChildren
	if !schemaSlotCardinality(len(builder.candidate.Rules)) {
		return refuse()
	}

	ruleIndex := len(builder.candidate.Rules)
	row := coldcomposition.Rule{
		Key:           compositionKeyOf(semantic),
		OperandFamily: compositionKeyOf(operandFamily),
		OutputKind:    coldcomposition.FactorOutput,
		Output:        compositionKeyOf(outputFactor.factor.semantic),
		Inputs:        uint64(compiled.InputCount()),
		Writes: []coldcomposition.Write{{
			Kind:   coldcomposition.WriteExact,
			Factor: compositionKeyOf(outputFactor.factor.semantic),
		}},
	}
	if output.Mode == ruleprogram.ModeRoute {
		row.Writes[0] = coldcomposition.Write{Kind: coldcomposition.WriteRoute, Factor: compositionKeyOf(outputFactor.factor.semantic), Route: uint64(output.RouteJoin) + 1}
	}
	row.Reads = make([]coldcomposition.Read, len(joins))
	for joinIndex, join := range joins {
		read := coldcomposition.Read{Kind: coldcomposition.ReadExact, Input: uint64(join.Input), Factor: compositionKeyOf(readFactors[joinIndex].factor.semantic)}
		if join.ReadForm == ruleprogram.Selected {
			read.Kind = coldcomposition.ReadSelect
			read.Semantic = read.Factor
			for offset := uint32(0); offset < join.Sources.Count; offset++ {
				source, sourceOK := compiled.SourceAt(int(join.Sources.Start + offset))
				if !sourceOK {
					return refuse()
				}
				if !source.Candidate {
					read.Dependencies = append(read.Dependencies, uint64(source.Position))
				}
			}
			// The cold row has a selected-read dependency law. A candidate-only
			// route remains a valid Plan, but cannot be projected into this
			// transitional row without inventing a dependency.
			if len(read.Dependencies) == 0 {
				return refuse()
			}
		}
		row.Reads[joinIndex] = read
	}
	if carryOK {
		row.Carries = []coldcomposition.Carry{{Input: uint64(carry.Input), Factor: compositionKeyOf(outputFactor.factor.semantic)}}
	}
	draft := &schemaRuleDraft{builder: builder, index: ruleIndex, output: outputFactor.factor}
	descriptor, descriptorOK := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		Ordinal: compiled.Rule(), AxisCount: factorDirectory.count, InputCount: compiled.InputCount(),
		Candidate: normalizedCandidate, Reducer: normalizedReducer,
		Reads: readPlans,
		Outputs: []generated.OutputPlan{{
			Factor: outputFactor.ordinal, Axis: normalizedOutputAxis, Address: normalizedOutput,
			Destination: normalizedDestination, Mode: output.Mode, Slot: output.Slot,
			RouteJoin: output.RouteJoin, RouteJoinPresent: output.RouteJoinPresent,
			Exact: output.Mode == ruleprogram.ModeExact, Strong: output.Mode == ruleprogram.ModeExact,
		}},
		Carry: func() *generated.CarryPlan {
			if !carryOK {
				return nil
			}
			return &generated.CarryPlan{Input: carry.Input, Factor: outputFactor.ordinal, Mode: ruleprogram.CarryIdentity, Identity: true}
		}(),
	})
	if !descriptorOK {
		return refuse()
	}
	cell := &generatedRuleCell{
		planDigest: catalog.Digest(),
		program:    descriptor,
		rule:       compiled.Rule(),
		inputs:     uint32(compiled.InputCount()),
		candidate:  compiled.Candidate(),
		join: func() ruleplan.RelationAddr {
			if len(joins) == 0 {
				return ruleplan.RelationAddr{}
			}
			return joins[0].Relation
		}(),
		key: func() ruleplan.ProjectionAddr {
			if len(joins) == 0 {
				return ruleplan.ProjectionAddr{}
			}
			return joins[0].Key
		}(),
		reducer: compiled.Reducer(),
		readInput: func() uint32 {
			if len(joins) == 0 {
				return 0
			}
			return joins[0].Input
		}(),
		readAxis: func() uint32 {
			if len(joins) == 0 {
				return 0
			}
			return joins[0].ReadAxis
		}(),
		read: func() coldcomposition.Key {
			if len(joins) == 0 {
				return coldcomposition.Key{}
			}
			return compositionKeyOf(readFactors[0].factor.semantic)
		}(),
		output:      output.Address,
		destination: output.Destination,
		write:       compositionKeyOf(outputFactor.factor.semantic),
		carryInput: func() uint32 {
			if !carryOK {
				return 0
			}
			return carry.Input
		}(),
	}
	draft.generated = cell
	builder.candidate.Rules = append(builder.candidate.Rules, row)
	builder.rules = append(builder.rules, draft)
	handle := issue(builder, draft, SchemaFormInvalid)
	if draft.token == nil {
		return refuse()
	}
	draft.token.generated = cell
	return &GeneratedRuleSlot{slotHandle: handle}, true
}

// generatedPlanMemberAddressesInRange is the final dense-coordinate fence
// before a Plan projection becomes a generated descriptor. The sealed Plan
// compiler has already authenticated each member against its owner catalog;
// this local check prevents a malformed/sentinel coordinate from entering the
// engine even if a future compiler version broadens its surface.
func generatedPlanMemberAddressesInRange(
	catalog ruleplan.Catalog,
	candidate ruleplan.RelationAddr,
	joins []ruleplan.Join,
	reducer ruleplan.ReducerAddr,
	output ruleplan.OutputAddr,
	destination ruleplan.ProjectionAddr,
) bool {
	if uint64(candidate.Axis) >= uint64(catalog.AxisCount()) || candidate.Member == ^uint32(0) ||
		uint64(reducer.Axis) >= uint64(catalog.AxisCount()) || reducer.Member == ^uint32(0) ||
		uint64(output.Axis) >= uint64(catalog.AxisCount()) || output.Frame == ^uint32(0) ||
		uint64(destination.Axis) >= uint64(catalog.AxisCount()) || destination.Member == ^uint32(0) ||
		reducer.Axis != output.Axis {
		return false
	}
	for _, join := range joins {
		if uint64(join.Relation.Axis) >= uint64(catalog.AxisCount()) || join.Relation.Member == ^uint32(0) ||
			uint64(join.Key.Axis) >= uint64(catalog.AxisCount()) || join.Key.Member == ^uint32(0) ||
			join.Relation.Axis != join.Key.Axis {
			return false
		}
		if join.PredicatePresent && (uint64(join.Predicate.Axis) >= uint64(catalog.AxisCount()) || join.Predicate.Member == ^uint32(0) || join.Predicate.Axis != join.Relation.Axis) {
			return false
		}
	}
	return true
}

// generatedPlanJoinShape validates the metadata that is not representable by
// the legacy cold row. The generated descriptor retains it verbatim, so this
// fence must reject malformed or unsupported forms before any projection is
// appended to the builder.
func generatedPlanJoinShape(compiled ruleplan.Plan, joinIndex int, join ruleplan.Join) bool {
	if uint64(join.Input) >= uint64(compiled.InputCount()) || join.Sources.Count == 0 ||
		uint64(join.Sources.Start)+uint64(join.Sources.Count) > uint64(compiled.SourceCount()) ||
		!join.ReadContract.Order.Available() || !join.ReadContract.Sparse.Available() ||
		!join.ReadContract.OnOpaque.Available() || !join.ReadContract.Multiplicity.Available() ||
		join.Cardinality != join.ReadContract.Multiplicity {
		return false
	}
	switch join.ReadForm {
	case ruleprogram.Exact:
		if join.PredicatePresent || join.Predicate != (ruleplan.ProjectionAddr{}) {
			return false
		}
	case ruleprogram.Selected:
		if join.PredicatePresent && join.Predicate == (ruleplan.ProjectionAddr{}) {
			return false
		}
	case ruleprogram.Summary, ruleprogram.Complete:
		return false
	default:
		return false
	}
	if (!join.Denominator.Present && join.Denominator.Ordinal != 0) ||
		join.Denominator.Present && join.Denominator.Ordinal == ^uint32(0) {
		return false
	}
	if (join.ReadForm == ruleprogram.Selected || join.ReadContract.Sparse == ruleprogram.SparseDefault || join.ReadContract.Sparse == ruleprogram.SparseDense) && !join.Denominator.Present {
		return false
	}
	for offset := uint32(0); offset < join.Sources.Count; offset++ {
		source, sourceOK := compiled.SourceAt(int(join.Sources.Start + offset))
		if !sourceOK {
			return false
		}
		if source.Candidate {
			if source.Position != 0 {
				return false
			}
		} else if source.Position >= uint32(joinIndex) {
			return false
		}
	}
	return true
}

// generatedFactorBinding is the one declaration-time proof that joins a
// sealed Plan semantic identity to a canonical runtime Factor ordinal. The
// draft is retained only while declaring the cold row; the descriptor stores
// the ordinal and no lookup table survives into Schema or runtime execution.
type generatedFactorBinding struct {
	factor  *keyDraft[factorRole]
	ordinal uint32
}

type generatedFactorDirectory struct {
	bySemantic map[identity.SemanticKey]generatedFactorBinding
	count      int
}

// generatedFactorDirectory validates the complete builder Factor rows once
// and computes the same canonical ordering used by internal/composition.Seal.
// Factor declaration order is intentionally irrelevant. The candidate row,
// draft owner, and draft index are checked together so a foreign or manually
// mismatched row cannot be laundered into a valid semantic binding.
func buildGeneratedFactorDirectory(builder *SchemaBuilder) (generatedFactorDirectory, bool) {
	if builder == nil || len(builder.factors) == 0 || len(builder.factors) != len(builder.candidate.Factors) || !schemaSlotCardinality(len(builder.factors)) {
		return generatedFactorDirectory{}, false
	}
	ordered := make([]*keyDraft[factorRole], len(builder.factors))
	seen := make(map[identity.SemanticKey]struct{}, len(builder.factors))
	for index, factor := range builder.factors {
		if factor == nil || factor.builder != builder || factor.index != index || !factor.semantic.Available() || index >= len(builder.candidate.Factors) {
			return generatedFactorDirectory{}, false
		}
		candidate := builder.candidate.Factors[index]
		if !candidate.Key.Available() || candidate.Key != compositionKeyOf(factor.semantic) {
			return generatedFactorDirectory{}, false
		}
		if _, duplicate := seen[factor.semantic]; duplicate {
			return generatedFactorDirectory{}, false
		}
		seen[factor.semantic] = struct{}{}
		ordered[index] = factor
	}
	sort.Slice(ordered, func(left, right int) bool {
		return identity.CompareSemanticKey(ordered[left].semantic, ordered[right].semantic) < 0
	})
	bySemantic := make(map[identity.SemanticKey]generatedFactorBinding, len(ordered))
	for ordinal, factor := range ordered {
		if factor == nil || ordinal > int(^uint32(0)) {
			return generatedFactorDirectory{}, false
		}
		bySemantic[factor.semantic] = generatedFactorBinding{factor: factor, ordinal: uint32(ordinal)}
	}
	return generatedFactorDirectory{bySemantic: bySemantic, count: len(ordered)}, true
}

// generatedPlanFactor resolves a Plan axis by its sealed semantic row. It
// deliberately does not inspect the Plan axis ordinal as a Factor position.
func generatedPlanFactor(directory generatedFactorDirectory, catalog ruleplan.Catalog, ordinal uint32) (generatedFactorBinding, bool) {
	if directory.bySemantic == nil || uint64(ordinal) >= uint64(catalog.AxisCount()) {
		return generatedFactorBinding{}, false
	}
	axis, axisOK := catalog.AxisAt(int(ordinal))
	if !axisOK || !axis.Semantic.Available() {
		return generatedFactorBinding{}, false
	}
	binding, factorOK := directory.bySemantic[axis.Semantic]
	if !factorOK || binding.factor == nil || binding.factor.builder == nil || binding.factor.semantic != axis.Semantic {
		return generatedFactorBinding{}, false
	}
	return binding, true
}

func generatedRuntimeAxis(directory generatedFactorDirectory, catalog ruleplan.Catalog, axis uint32) (uint32, bool) {
	binding, ok := generatedPlanFactor(directory, catalog, axis)
	if !ok {
		return 0, false
	}
	return binding.ordinal, true
}

func generatedRuntimeRelation(directory generatedFactorDirectory, catalog ruleplan.Catalog, address ruleplan.RelationAddr) (ruleplan.RelationAddr, bool) {
	axis, ok := generatedRuntimeAxis(directory, catalog, address.Axis)
	if !ok {
		return ruleplan.RelationAddr{}, false
	}
	address.Axis = axis
	return address, true
}

func generatedRuntimeProjection(directory generatedFactorDirectory, catalog ruleplan.Catalog, address ruleplan.ProjectionAddr) (ruleplan.ProjectionAddr, bool) {
	axis, ok := generatedRuntimeAxis(directory, catalog, address.Axis)
	if !ok {
		return ruleplan.ProjectionAddr{}, false
	}
	address.Axis = axis
	return address, true
}

func generatedRuntimeReducer(directory generatedFactorDirectory, catalog ruleplan.Catalog, address ruleplan.ReducerAddr) (ruleplan.ReducerAddr, bool) {
	axis, ok := generatedRuntimeAxis(directory, catalog, address.Axis)
	if !ok {
		return ruleplan.ReducerAddr{}, false
	}
	address.Axis = axis
	return address, true
}

func generatedRuntimeOutput(directory generatedFactorDirectory, catalog ruleplan.Catalog, address ruleplan.OutputAddr) (ruleplan.OutputAddr, bool) {
	axis, ok := generatedRuntimeAxis(directory, catalog, address.Axis)
	if !ok {
		return ruleplan.OutputAddr{}, false
	}
	address.Axis = axis
	return address, true
}
