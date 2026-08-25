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
// The admitted vertical is one exact, routed, or structural output, an ordered
// table of Exact/Selected joins, and an optional identity carry. The descriptor
// is the canonical copy of every Plan row; the cold composition row below is
// only the existing engine shape projection and never a source for missing Plan
// metadata. Summary/Complete reads and transformed carries remain explicit seal
// refusals.
//
// A structural output publishes no fact. Its cold row names no Output Factor
// and no Write, and carries instead the activation family its candidate
// branches are grouped under - the family the Plan resolved from the rule's own
// declared role. The output axis it still names is the axis its rows are
// indexed by, which is what routes the row to a typed plane at execution.
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
	// An issued-row candidate has no Factor relation to normalize. Its rows are
	// Program rows, and the ordinal reaches the runtime on the mounted
	// placement, so the address stays zero and the tag carries the choice.
	_, issuedCandidate := compiled.IssuedCandidate()

	// Output and reducer addresses are checked against the same sealed axis
	// directory before their factors are selected. Frame/value-slot identity is
	// retained in the generated descriptor; only structural output is refused.
	if !output.Mode.Available() || output.Slot != 0 || compiled.Reducer().Axis != output.Address.Axis {
		return refuse()
	}
	// The transport vector and the activation family are the structural arm's
	// own declaration, and the Program layer holds them to each other. Stating
	// the biconditional again here is what keeps a fact-writing rule from
	// reaching a cold row that claims a family, and a structural one from
	// reaching a row that claims none.
	activationFamily := compiled.ActivationFamily()
	if (output.Mode == ruleprogram.ModeStructural) != (compiled.TransportCount() != 0) ||
		(output.Mode == ruleprogram.ModeStructural) != activationFamily.Available() {
		return refuse()
	}
	for joinIndex, join := range joins {
		if output.Mode == ruleprogram.ModeRoute && output.RouteJoinPresent && output.RouteJoin == uint32(joinIndex) && join.ReadForm != ruleprogram.Selected {
			return refuse()
		}
	}
	if output.Mode == ruleprogram.ModeExact || output.Mode == ruleprogram.ModeStructural {
		// A structural row is addressed only by its candidate.  An exact row
		// has two sealed normal forms: the candidate owner's own projection, or
		// a consumer projection declared by the output axis for that candidate.
		// The latter is what lets a heterogeneous fold write its own key without
		// copying the foreign candidate directory into the consumer.
		destinationAxisOK := output.Destination.Axis == compiled.Candidate().Axis
		if output.Mode == ruleprogram.ModeExact {
			destinationAxisOK = destinationAxisOK || output.Destination.Axis == output.Address.Axis
		}
		if issuedCandidate || output.RouteJoinPresent || output.RouteJoin != 0 || !destinationAxisOK {
			return refuse()
		}
	} else if !output.RouteJoinPresent || uint64(output.RouteJoin) >= uint64(len(joins)) || joins[output.RouteJoin].ReadForm != ruleprogram.Selected || output.Destination.Axis != joins[output.RouteJoin].Relation.Axis {
		return refuse()
	}
	// A structural publication computes no fact, so there is no prior fact for
	// it to carry: a carry names the output Factor's own value, and this arm
	// has no output Factor value at all.
	if output.Mode == ruleprogram.ModeStructural {
		if _, carryDeclared := compiled.Carry(); carryDeclared {
			return refuse()
		}
	}
	// Validate all addresses while they still use the Plan's own axis
	// directory. This preserves the owner-local member/frame coordinates and
	// keeps the generated constructor's shape checks independent of mapping.
	if !generatedPlanMemberAddressesInRange(catalog, compiled.Candidate(), issuedCandidate, joins, compiled.Reducer(), output.Address, output.Destination) {
		return refuse()
	}
	outputFactor, outputFactorOK := generatedPlanFactor(factorDirectory, catalog, output.Address.Axis)
	if !outputFactorOK {
		return refuse()
	}
	normalizedCandidate, candidateOK := generatedRuntimeRelation(factorDirectory, catalog, compiled.Candidate())
	if issuedCandidate {
		normalizedCandidate, candidateOK = ruleplan.RelationAddr{}, true
	}
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
		var normalizedParent ruleplan.RelationAddr
		if join.ParentPresent {
			var parentOK bool
			normalizedParent, parentOK = generatedRuntimeRelation(factorDirectory, catalog, join.Parent)
			if !parentOK {
				return refuse()
			}
		}
		var normalizedKeyVector ruleplan.RelationAddr
		if join.KeyVectorPresent {
			var keyVectorOK bool
			normalizedKeyVector, keyVectorOK = generatedRuntimeRelation(factorDirectory, catalog, join.KeyVector)
			if !keyVectorOK {
				return refuse()
			}
		}
		// The addressing directory is normalized into the same runtime Factor
		// directory as the candidate it is compared against. A directory that
		// stayed in catalog coordinates would compare equal to the candidate
		// only by accident.
		var normalizedAddressing ruleplan.RelationAddr
		if join.AddressingPresent {
			var addressingOK bool
			normalizedAddressing, addressingOK = generatedRuntimeRelation(factorDirectory, catalog, join.Addressing)
			if !addressingOK {
				return refuse()
			}
		}
		readFactors[joinIndex] = readFactor
		readPlans[joinIndex] = generated.ReadPlan{
			Input: join.Input, Factor: readFactor.ordinal, Axis: normalizedReadAxis,
			Sources: join.Sources, Relation: normalizedJoin, Key: normalizedKey,
			Predicate: normalizedPredicate, PredicatePresent: join.PredicatePresent,
			Parent: normalizedParent, ParentPresent: join.ParentPresent,
			KeyVector: normalizedKeyVector, KeyVectorPresent: join.KeyVectorPresent,
			Addressing: normalizedAddressing, AddressingPresent: join.AddressingPresent,
			Form: join.ReadForm, Contract: join.ReadContract, Denominator: join.Denominator,
			PointBound:  join.PointBound,
			RowCapacity: uint16(scratch.JoinCount), CellCapacity: uint16(scratch.OutputCount),
		}
	}

	// A carry is sealed in the disposition the Plan compiled: identity carries
	// the prior output fact unchanged, and a transform names one owner-issued
	// transform member the Plan already resolved against the writing axis. The
	// address is normalized into the runtime Factor directory like every other
	// address in the descriptor; no semantic key is fabricated from an ordinal.
	carry, carryOK := compiled.Carry()
	var normalizedCarryTransform ruleplan.CarryTransformAddr
	if carryOK {
		if !carry.Mode.Available() || uint64(carry.Input) >= uint64(compiled.InputCount()) ||
			carry.TransformPresent != (carry.Mode == ruleprogram.CarryTransform) {
			return refuse()
		}
		if carry.TransformPresent {
			transformAxis, transformAxisOK := generatedRuntimeAxis(factorDirectory, catalog, carry.Transform.Axis)
			if !transformAxisOK || !carry.TransformAxis.Available() || !carry.TransformKey.Available() {
				return refuse()
			}
			normalizedCarryTransform = ruleplan.CarryTransformAddr{Axis: transformAxis, Member: carry.Transform.Member}
		}
	}
	// The transport vector is normalized into the runtime Factor directory like
	// every other address in the descriptor. A transported axis is a Factor the
	// candidate route instantiates, so an axis with no Factor row is a
	// transport nothing could carry.
	transports := make([]ruleplan.Transport, compiled.TransportCount())
	for transportIndex := range transports {
		transport, transportOK := compiled.TransportAt(transportIndex)
		if !transportOK {
			return refuse()
		}
		transportAxis, transportAxisOK := generatedRuntimeAxis(factorDirectory, catalog, transport.Axis)
		if !transportAxisOK {
			return refuse()
		}
		transports[transportIndex] = ruleplan.Transport{Axis: transportAxis, Exported: transport.Exported}
	}
	// The branch vocabulary rides beside the vector it stands with. Its
	// projection addresses are already the owner's own member ordinals and are
	// not normalized into the Factor directory: they address rows of an axis
	// catalog, not Factor columns.
	var activationBranch *ruleplan.Activation
	if branch, branchOK := compiled.ActivationBranch(); branchOK {
		normalized := branch
		normalizedBranchAxis, branchAxisOK := generatedRuntimeAxis(factorDirectory, catalog, branch.Branch.Axis)
		normalizedApplication, applicationOK := generatedRuntimeAxis(factorDirectory, catalog, branch.Application.Axis)
		normalizedTarget, targetOK := generatedRuntimeAxis(factorDirectory, catalog, branch.Target.Axis)
		normalizedEndpoint, endpointOK := generatedRuntimeAxis(factorDirectory, catalog, branch.Endpoint.Axis)
		normalizedMount, mountOK := generatedRuntimeAxis(factorDirectory, catalog, branch.Mount.Axis)
		normalizedBody, bodyOK := generatedRuntimeAxis(factorDirectory, catalog, branch.Body.Axis)
		if !branchAxisOK || !applicationOK || !targetOK || !endpointOK || !mountOK || !bodyOK {
			return refuse()
		}
		normalized.Branch.Axis = normalizedBranchAxis
		normalized.Application.Axis = normalizedApplication
		normalized.Target.Axis = normalizedTarget
		normalized.Endpoint.Axis = normalizedEndpoint
		normalized.Mount.Axis = normalizedMount
		normalized.Body.Axis = normalizedBody
		activationBranch = &normalized
	}
	if !builder.claim(semantic) {
		return nil, false
	}
	builder.phase = schemaBuilderChildren
	if !schemaSlotCardinality(len(builder.candidate.Rules)) {
		return refuse()
	}
	// The family the structural arm's branches are grouped under is declared
	// here, on the one declaration that names it. A family several activation
	// rules share is one cold row: the first rule that names it declares it and
	// every later one resolves the same row, so the composition never holds two
	// authorities over one family.
	activationRange, activationRangeOK := declareGeneratedActivationFamily(builder, activationFamily)
	if !activationRangeOK {
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
	switch output.Mode {
	case ruleprogram.ModeRoute:
		row.Writes[0] = coldcomposition.Write{Kind: coldcomposition.WriteRoute, Factor: compositionKeyOf(outputFactor.factor.semantic), Route: uint64(output.RouteJoin) + 1}
	case ruleprogram.ModeStructural:
		// A structural row publishes no fact. It names no Output Factor and no
		// Write, and its one declared capability is the activation family its
		// branches are admitted under.
		row.OutputKind, row.Output, row.Writes = coldcomposition.StructuralOutput, coldcomposition.Key{}, nil
		row.Activations = []coldcomposition.ActivationRange{activationRange}
	}
	row.Reads = make([]coldcomposition.Read, len(joins))
	for joinIndex, join := range joins {
		read := coldcomposition.Read{
			Kind: generatedColdReadKind(join.ReadForm, join.ParentPresent || join.KeyVectorPresent), Input: uint64(join.Input),
			Factor:     compositionKeyOf(readFactors[joinIndex].factor.semantic),
			PointBound: join.PointBound == ruleprogram.PointBound,
		}
		if read.Kind == coldcomposition.ReadSummary {
			// A vector read is delivered over the Factor's own declared summary
			// form. The form's semantic is the Factor's statement, read off the
			// row it was declared on rather than named again here, and a Factor
			// that declares no summary form cannot answer a vector read at all.
			summary, summaryOK := summaryReadFormSemantic(builder, readFactors[joinIndex])
			if !summaryOK {
				return refuse()
			}
			read.Semantic, read.Normalizer = summary, summary
			row.Reads[joinIndex] = read
			continue
		}
		if read.Kind == coldcomposition.ReadSelect {
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
			// A selection over the candidate alone has no predecessor read,
			// and the cold row now says so with an empty list rather than
			// being handed an invented dependency to satisfy a shape law.
		}
		row.Reads[joinIndex] = read
	}
	if carryOK {
		row.Carries = []coldcomposition.Carry{{Input: uint64(carry.Input), Factor: compositionKeyOf(outputFactor.factor.semantic)}}
	}
	// A structural row owns no output Factor draft. The draft's output is what
	// licenses a write or carry slot to be minted against this rule, and a rule
	// that publishes no fact has neither to mint.
	draft := &schemaRuleDraft{builder: builder, index: ruleIndex, output: outputFactor.factor}
	if output.Mode == ruleprogram.ModeStructural {
		draft.output = nil
	}
	descriptor, descriptorOK := generated.NewPlanCompiledRule(generated.CompiledRuleSpec{
		AxisCount: factorDirectory.count, InputCount: compiled.InputCount(),
		Candidate: normalizedCandidate, IssuedCandidate: issuedCandidate, Reducer: normalizedReducer,
		Reads: readPlans,
		Outputs: []generated.OutputPlan{{
			Factor: outputFactor.ordinal, Axis: normalizedOutputAxis, Address: normalizedOutput,
			Destination: normalizedDestination, Mode: output.Mode, Slot: output.Slot,
			RouteJoin: output.RouteJoin, RouteJoinPresent: output.RouteJoinPresent,
			Exact: output.Mode == ruleprogram.ModeExact, Strong: output.Mode == ruleprogram.ModeExact,
		}},
		Transports: transports,
		Activation: activationBranch,
		Carry: func() *generated.CarryPlan {
			if !carryOK {
				return nil
			}
			return &generated.CarryPlan{
				Input: carry.Input, Factor: outputFactor.ordinal, Mode: carry.Mode,
				Transform: normalizedCarryTransform, TransformPresent: carry.TransformPresent,
				Identity: carry.Mode == ruleprogram.CarryIdentity,
			}
		}(),
	})
	if !descriptorOK {
		return refuse()
	}
	cell := &generatedRuleCell{
		planDigest: catalog.Digest(),
		program:    descriptor,
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

// declareGeneratedActivationFamily resolves the one cold activation family row
// a structural generated rule's branches are grouped under, declaring it if no
// earlier rule already named it.
//
// The row is the family's whole cold content, so a family two activation rules
// share is one row reached twice rather than two authorities over one identity.
// An absent semantic is the fact-writing arm and yields no range.
func declareGeneratedActivationFamily(builder *SchemaBuilder, semantic identity.SemanticKey) (coldcomposition.ActivationRange, bool) {
	if builder == nil {
		return coldcomposition.ActivationRange{}, false
	}
	if !semantic.Available() {
		return coldcomposition.ActivationRange{}, true
	}
	for _, declared := range builder.families {
		if declared != nil && declared.semantic == semantic {
			return coldcomposition.ActivationRange{Family: compositionKeyOf(semantic)}, true
		}
	}
	family, ok := DeclareSchemaActivationFamily(builder, semantic)
	if !ok || !family.available() {
		return coldcomposition.ActivationRange{}, false
	}
	return coldcomposition.ActivationRange{Family: compositionKeyOf(semantic)}, true
}

// generatedPlanMemberAddressesInRange is the final dense-coordinate fence
// before a Plan projection becomes a generated descriptor. The sealed Plan
// compiler has already authenticated each member against its owner catalog;
// this local check prevents a malformed/sentinel coordinate from entering the
// engine even if a future compiler version broadens its surface.
func generatedPlanMemberAddressesInRange(
	catalog ruleplan.Catalog,
	candidate ruleplan.RelationAddr,
	candidateIssued bool,
	joins []ruleplan.Join,
	reducer ruleplan.ReducerAddr,
	output ruleplan.OutputAddr,
	destination ruleplan.ProjectionAddr,
) bool {
	// An issued-row candidate has no relation address. Range-checking its zero
	// would report that the first relation of the first axis is in range,
	// which is a true answer to a question the declaration never asked. The
	// converse is deliberately not stated: the first relation of the first
	// axis is a real address, so a zero value does not identify the arm.
	if candidateIssued {
		if candidate != (ruleplan.RelationAddr{}) {
			return false
		}
	} else if uint64(candidate.Axis) >= uint64(catalog.AxisCount()) || candidate.Member == ^uint32(0) {
		return false
	}
	if uint64(reducer.Axis) >= uint64(catalog.AxisCount()) || reducer.Member == ^uint32(0) ||
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
		// A parent restatement addresses a relation of the same owner axis:
		// a member set nests under a candidate directory its own axis issues.
		if join.ParentPresent && (uint64(join.Parent.Axis) >= uint64(catalog.AxisCount()) || join.Parent.Member == ^uint32(0) || join.Parent.Axis != join.Relation.Axis) {
			return false
		}
		// An addressing directory is deliberately NOT held to the read
		// relation's own axis: a corresponded directory belongs to the axis
		// that issued the candidate, which is the foreign one whenever the
		// correspondence is doing any work.
		if join.AddressingPresent && (uint64(join.Addressing.Axis) >= uint64(catalog.AxisCount()) || join.Addressing.Member == ^uint32(0)) {
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
	if !generated.ReadFormAddressShape(join.ReadForm, join.Predicate, join.PredicatePresent, join.Parent, join.ParentPresent, join.KeyVector, join.KeyVectorPresent) {
		return false
	}
	_, joinIssuedCandidate := compiled.IssuedCandidate()
	if !generated.ReadAddressingShape(join.ReadForm, joinIssuedCandidate, join.Addressing, join.AddressingPresent) {
		return false
	}
	if (!join.Denominator.Present && join.Denominator.Ordinal != 0) ||
		join.Denominator.Present && join.Denominator.Ordinal == ^uint32(0) {
		return false
	}
	if ruleprogram.RequiresDenominator(join.ReadForm, join.ReadContract.Sparse) && !join.Denominator.Present {
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

// summaryReadFormSemantic resolves the summary read form one Factor declared.
//
// A vector read is not a shape a rule chooses: it is delivered over a form the
// Factor published, and the form carries the semantic the cold row and every
// later summary surface are keyed by. A Factor that declares no summary form
// answers no vector read, and two declared forms are two authorities over one
// delivery, so both are refused here rather than resolved by order.
func summaryReadFormSemantic(builder *SchemaBuilder, factor generatedFactorBinding) (coldcomposition.Key, bool) {
	if builder == nil || factor.factor == nil {
		return coldcomposition.Key{}, false
	}
	index := factor.factor.index
	if index < 0 || index >= len(builder.candidate.Factors) {
		return coldcomposition.Key{}, false
	}
	resolved := coldcomposition.Key{}
	for _, form := range builder.candidate.Factors[index].Forms {
		if form.Kind != coldcomposition.FactorSummaryRead && form.Kind != coldcomposition.FactorDistributiveSummaryRead {
			continue
		}
		if !form.Semantic.Available() || resolved.Available() {
			return coldcomposition.Key{}, false
		}
		resolved = form.Semantic
	}
	if !resolved.Available() {
		return coldcomposition.Key{}, false
	}
	return resolved, true
}
