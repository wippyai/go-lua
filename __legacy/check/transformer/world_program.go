package transformer

import (
	"fmt"
	mathbits "math/bits"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type programRef uint32
type instructionRef uint32
type returnPayloadRef uint32

type programNodeKind uint8

const (
	programInvalid programNodeKind = iota
	programSequence
	programChoice
	programLoopMu
	programLoopPortal
)

type instructionKind uint8

const (
	instructionInvalid instructionKind = iota
	instructionEffect
	instructionCallFrame
	instructionExternalCall
	instructionRootAssignment
	instructionReturn
	instructionEnvironmentWrite
	instructionGenericFor
	instructionContribution
	instructionLoopFeedback
	instructionLoopExit
	instructionBranchRelations
	instructionCallResults
	instructionPresenceImplications
	instructionChannelSelect
	instructionCovariantExposure
	instructionNoNormalReturn
)

// WorldProgram is one sealed owner-local semantic circuit. It is compiled
// code, not a lattice value: joins, widening, and narrowing belong only to the
// bound application cells created by its evaluator.
type WorldProgram struct {
	terms       *Arena
	effects     *EffectArena
	shape       Shape
	arena       *worldProgramArena
	root        programRef
	publication worldPublicationPlan
}

// worldPublicationPlan names source-CFG boundaries in the compiled circuit.
// It is syntax only: no State, solver callback, or observation cache is
// retained. Point outputs and branch-edge outputs become identity checkpoints
// in the sole boundaryProgram equation graph during closure.
type worldPublicationPlan struct {
	points []worldPointPublication
	edges  []worldEdgePublication
}

type worldPointPublication struct {
	point cfg.Point
	ref   programRef
}

type worldEdgePublication struct {
	from, to cfg.Point
	ref      programRef
}

// worldProgramArena is published only by worldProgramFreezer.seal. Every
// contained slice is privately copied before publication and neither this
// arena nor the term/effect arenas can grow afterwards.
type worldProgramArena struct {
	programs     []programNode
	instructions []instructionNode
	returns      []returnPayload
	sealed       bool
}

type programNode struct {
	kind         programNodeKind
	point        cfg.Point
	instructions []instructionRef
	next         programRef
	guard        Guard
	whenTrue     programRef
	whenFalse    programRef
	binder       loopMuTerm
	body         programRef
	exits        []programRef
}

type instructionNode struct {
	kind            instructionKind
	point           cfg.Point
	effect          EffectTerm
	call            callFrameTerm
	guard           Guard
	ret             returnPayloadRef
	slot            statekey.Value
	value           ValueTerm
	access          []valueAccessTerm
	genericIdentity frozenGenericForIdentityPublication
	operands        callOutcomeOperandTerms
	writes          []ValueTerm
	memberCall      boundaryMemberCallDiagnosticTerm
	binder          loopMuTerm
	route           uint32
	branch          factapply.BranchRelationTransaction
	result          factapply.CallResultTransaction
	resultPhase     factapply.CallResultPhase
	presence        factapply.PathValuePresenceImplicationTransaction
	channel         factapply.ChannelSelectTransaction
	covariant       factapply.CovariantExposureTransaction
	rootAssignment  rootAssignmentTerm
}

// returnPayload is a typed terminal projection, not a path alternative. Its
// control guard is structural: the Program path reaching the instruction.
// Multiple source returns therefore remain distinct shared circuit terminals
// rather than a flat row/DNF inventory.
type returnPayload struct {
	suspensionKnown        bool
	maySuspend             bool
	protectedCallTypestate callboundary.ProtectedCallTypestate
	operations             []Operation
	proofs                 []BranchProofTerm
	returnConditions       []returnConditionParamRefinementTerm
	branchLiteralCases     []branchSufficientOutcomeTerm
	resultPublication      factapply.CallResultTransaction
	covariant              factapply.CovariantExposureTransaction
	paramObligations       []callpayload.CallParamObligation
	pathObligations        []boundaryPathObligationTerm
	paramExposures         []boundaryParamExposureTerm
	returnTransaction      returnTransactionTerm
}

// branchSufficientOutcomeTerm is exact converse branch metadata carried to a
// terminal tuple. selectedEdge identifies the path reaching the outcome; the
// case retains the edge selected by its literal. Publication can therefore
// derive direct and complementary finite-literal correlations from stabilized
// coordinates without replaying a body or pretending the case refines State.
type branchSufficientOutcomeTerm struct {
	branch       cfg.Point
	selectedEdge bool
	literalCase  factflow.BranchSufficientLiteralCase
}

func cloneBranchSufficientOutcomeTerms(in []branchSufficientOutcomeTerm) []branchSufficientOutcomeTerm {
	if len(in) == 0 {
		return nil
	}
	out := make([]branchSufficientOutcomeTerm, len(in))
	for index, term := range in {
		term.literalCase = factflow.NewBranchSufficientLiteralCase(
			term.literalCase.TargetPath(), term.literalCase.LiteralValue(), term.literalCase.Edge(),
		)
		out[index] = term
	}
	return out
}

type returnConditionParamRefinementTerm struct {
	ReturnIndex int
	ReturnValue bool
	Target      pathdom.Path
	Value       product.Value
}

type worldFlowEdge struct {
	target programRef
	loop   loopMuTerm
}

type worldFlowBits []uint64

func (b worldFlowBits) clone() worldFlowBits {
	return append(worldFlowBits(nil), b...)
}

func (b *worldFlowBits) add(id uint32) {
	word := int(id / 64)
	for len(*b) <= word {
		*b = append(*b, 0)
	}
	(*b)[word] |= uint64(1) << (id % 64)
}

func (b worldFlowBits) has(id uint32) bool {
	word := int(id / 64)
	return word < len(b) && b[word]&(uint64(1)<<(id%64)) != 0
}

func (b *worldFlowBits) intersect(other worldFlowBits) {
	limit := len(*b)
	if len(other) < limit {
		limit = len(other)
	}
	for index := 0; index < limit; index++ {
		(*b)[index] &= other[index]
	}
	for index := limit; index < len(*b); index++ {
		(*b)[index] = 0
	}
}

func (b *worldFlowBits) union(other worldFlowBits) {
	for len(*b) < len(other) {
		*b = append(*b, 0)
	}
	for index := range other {
		(*b)[index] |= other[index]
	}
}

func (b worldFlowBits) frameSet() map[callFrameTerm]struct{} {
	out := make(map[callFrameTerm]struct{})
	for word, bits := range b {
		for bits != 0 {
			bit := uint32(mathbits.TrailingZeros64(bits))
			out[callFrameTerm(uint32(word)*64+bit)] = struct{}{}
			bits &= bits - 1
		}
	}
	return out
}

func (b worldFlowBits) loopSet() map[loopMuTerm]struct{} {
	out := make(map[loopMuTerm]struct{})
	for word, bits := range b {
		for bits != 0 {
			bit := uint32(mathbits.TrailingZeros64(bits))
			out[loopMuTerm(uint32(word)*64+bit)] = struct{}{}
			bits &= bits - 1
		}
	}
	return out
}

type worldFlowState struct {
	reached               bool
	framesMust, framesMay worldFlowBits
	loopsMust, loopsMay   worldFlowBits
}

func (s *worldFlowState) merge(framesMust, framesMay, loopsMust, loopsMay worldFlowBits) {
	if !s.reached {
		s.reached = true
		s.framesMust, s.framesMay = framesMust.clone(), framesMay.clone()
		s.loopsMust, s.loopsMay = loopsMust.clone(), loopsMay.clone()
		return
	}
	s.framesMust.intersect(framesMust)
	s.framesMay.union(framesMay)
	s.loopsMust.intersect(loopsMust)
	s.loopsMay.union(loopsMay)
}

// semanticContribution is immutable event/projection syntax attached to one
// structural point or edge. It is not a lattice value and is never joined with
// State: the executor uses the event to update typed State/control coordinates,
// while final projection evaluates the remaining static selectors exactly once.
type semanticContribution = returnPayload

type worldProgramFreezer struct {
	terms        *Arena
	effects      *EffectArena
	shape        Shape
	programs     []programNode
	instructions []instructionNode
	returns      []returnPayload
	publication  worldPublicationPlan
}

func newWorldProgramFreezer(terms *Arena, effects *EffectArena, shape Shape, programs int) (*worldProgramFreezer, error) {
	if terms == nil || terms.reg == nil || effects == nil || effects.terms != terms || terms.Sealed() || effects.Sealed() || programs <= 0 {
		return nil, fmt.Errorf("transformer: world freezer requires open owner arenas and finite topology")
	}
	return &worldProgramFreezer{
		terms: terms, effects: effects, shape: shape,
		programs:     make([]programNode, programs+1),
		instructions: []instructionNode{{}},
		returns:      []returnPayload{{}},
	}, nil
}

func (f *worldProgramFreezer) appendInstruction(node instructionNode) instructionRef {
	ref := instructionRef(len(f.instructions))
	node.genericIdentity = node.genericIdentity.clone()
	f.instructions = append(f.instructions, node)
	return ref
}

func (f *worldProgramFreezer) appendReturn(payload returnPayload) returnPayloadRef {
	ref := returnPayloadRef(len(f.returns))
	payload.protectedCallTypestate = payload.protectedCallTypestate.Clone()
	payload.operations = append([]Operation(nil), payload.operations...)
	payload.proofs = append([]BranchProofTerm(nil), payload.proofs...)
	payload.returnConditions = append([]returnConditionParamRefinementTerm(nil), payload.returnConditions...)
	payload.branchLiteralCases = cloneBranchSufficientOutcomeTerms(payload.branchLiteralCases)
	payload.resultPublication = payload.resultPublication.Clone()
	payload.covariant = payload.covariant.Clone()
	payload.paramObligations = cloneBoundaryParamObligations(payload.paramObligations)
	payload.pathObligations = cloneBoundaryPathObligations(payload.pathObligations)
	payload.paramExposures = cloneBoundaryParamExposures(payload.paramExposures)
	payload.returnTransaction = payload.returnTransaction.clone()
	f.returns = append(f.returns, payload)
	return ref
}

func (f *worldProgramFreezer) seal(root programRef) (WorldProgram, error) {
	if f == nil || root == 0 || int(root) >= len(f.programs) {
		return WorldProgram{}, fmt.Errorf("transformer: world freezer has no root")
	}
	arena := &worldProgramArena{
		programs:     make([]programNode, len(f.programs)),
		instructions: append([]instructionNode(nil), f.instructions...),
		returns:      make([]returnPayload, len(f.returns)),
	}
	for index, node := range f.programs {
		node.instructions = append([]instructionRef(nil), node.instructions...)
		node.exits = append([]programRef(nil), node.exits...)
		arena.programs[index] = node
	}
	for index := range arena.instructions {
		if arena.instructions[index].memberCall.site != 0 {
			arena.instructions[index].memberCall = cloneBoundaryMemberCallDiagnostics([]boundaryMemberCallDiagnosticTerm{arena.instructions[index].memberCall})[0]
		}
		arena.instructions[index].branch = arena.instructions[index].branch.Clone()
		arena.instructions[index].result = arena.instructions[index].result.Clone()
		arena.instructions[index].presence = arena.instructions[index].presence.Clone()
		arena.instructions[index].channel = arena.instructions[index].channel.Clone()
		arena.instructions[index].covariant = arena.instructions[index].covariant.Clone()
	}
	for index, payload := range f.returns {
		payload.protectedCallTypestate = payload.protectedCallTypestate.Clone()
		payload.operations = append([]Operation(nil), payload.operations...)
		payload.proofs = append([]BranchProofTerm(nil), payload.proofs...)
		payload.returnConditions = append([]returnConditionParamRefinementTerm(nil), payload.returnConditions...)
		payload.branchLiteralCases = cloneBranchSufficientOutcomeTerms(payload.branchLiteralCases)
		payload.resultPublication = payload.resultPublication.Clone()
		payload.covariant = payload.covariant.Clone()
		payload.paramObligations = cloneBoundaryParamObligations(payload.paramObligations)
		payload.pathObligations = cloneBoundaryPathObligations(payload.pathObligations)
		payload.paramExposures = cloneBoundaryParamExposures(payload.paramExposures)
		payload.returnTransaction = payload.returnTransaction.clone()
		arena.returns[index] = payload
	}
	publication := worldPublicationPlan{
		points: append([]worldPointPublication(nil), f.publication.points...),
		edges:  append([]worldEdgePublication(nil), f.publication.edges...),
	}
	program := WorldProgram{terms: f.terms, effects: f.effects, shape: f.shape, arena: arena, root: root, publication: publication}
	if !program.valid(false) {
		return WorldProgram{}, fmt.Errorf("transformer: world freezer produced malformed code")
	}
	arena.sealed = true
	if !program.valid(true) {
		return WorldProgram{}, fmt.Errorf("transformer: sealed world program is malformed")
	}
	return program, nil
}

func (p WorldProgram) valid(requireSealed bool) bool {
	if p.terms == nil || p.effects == nil || p.effects.terms != p.terms || p.arena == nil || p.root == 0 || int(p.root) >= len(p.arena.programs) ||
		requireSealed && !p.arena.sealed || !requireSealed && p.arena.sealed || len(p.arena.instructions) == 0 || len(p.arena.returns) == 0 {
		return false
	}
	for ref := programRef(1); int(ref) < len(p.arena.programs); ref++ {
		node := p.arena.programs[ref]
		switch node.kind {
		case programSequence:
			if node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.binder != 0 || node.body != 0 || len(node.exits) != 0 {
				return false
			}
		case programChoice:
			if node.next != 0 || node.guard == 0 || !p.terms.validGuard(node.guard, p.shape) || node.whenTrue == 0 || node.whenFalse == 0 || len(node.instructions) != 0 || node.binder != 0 || node.body != 0 || len(node.exits) != 0 {
				return false
			}
		case programLoopMu:
			if node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || len(node.instructions) != 0 ||
				node.binder == 0 || int(node.binder) >= len(p.terms.loopMus) || node.body == 0 {
				return false
			}
		case programLoopPortal:
			if node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || len(node.instructions) != 0 ||
				node.binder == 0 || int(node.binder) >= len(p.terms.loopMus) || node.body == 0 || len(node.exits) != 0 {
				return false
			}
		default:
			return false
		}
		for _, instruction := range node.instructions {
			if instruction == 0 || int(instruction) >= len(p.arena.instructions) {
				return false
			}
		}
		for _, target := range []programRef{node.next, node.whenTrue, node.whenFalse, node.body} {
			if target != 0 && int(target) >= len(p.arena.programs) {
				return false
			}
		}
		for _, target := range node.exits {
			if target == 0 || int(target) >= len(p.arena.programs) {
				return false
			}
		}
	}
	for ref := instructionRef(1); int(ref) < len(p.arena.instructions); ref++ {
		node := p.arena.instructions[ref]
		switch node.kind {
		case instructionEffect:
			if node.effect == 0 || node.call != 0 || node.ret != 0 || !p.effects.Valid(node.effect, p.shape) {
				return false
			}
		case instructionCallFrame:
			if node.call == 0 || node.effect != 0 || node.ret != 0 || int(node.call) >= len(p.terms.callFrames) || node.guard != 0 && !p.terms.validGuard(node.guard, p.shape) {
				return false
			}
			if node.memberCall.site != 0 && (node.memberCall.site != p.terms.callFrames[node.call].point ||
				!validBoundaryMemberCallDiagnostics(p.terms.reg, p.terms, p.shape, []boundaryMemberCallDiagnosticTerm{node.memberCall})) {
				return false
			}
		case instructionExternalCall:
			if node.point == 0 || node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 || node.binder != 0 || node.route != 0 || node.guard != 0 && !p.terms.validGuard(node.guard, p.shape) {
				return false
			}
			if node.memberCall.site != 0 && (node.memberCall.site != node.point || !validBoundaryMemberCallDiagnostics(p.terms.reg, p.terms, p.shape, []boundaryMemberCallDiagnosticTerm{node.memberCall})) {
				return false
			}
		case instructionRootAssignment:
			if !node.rootAssignment.valid(p.terms, p.shape) || node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 || node.binder != 0 || node.route != 0 {
				return false
			}
		case instructionReturn:
			if node.ret == 0 || node.effect != 0 || node.call != 0 || int(node.ret) >= len(p.arena.returns) {
				return false
			}
		case instructionEnvironmentWrite:
			if node.slot == 0 || !p.terms.validEnvironmentSlot(node.slot) || node.value == 0 || !p.terms.validValue(node.value, p.shape, make(map[ValueTerm]bool)) || node.effect != 0 || node.call != 0 || node.ret != 0 || node.binder != 0 {
				return false
			}
		case instructionGenericFor:
			if node.point == 0 || len(node.access) != 1 || node.access[0].hasPoint || node.access[0].term != node.genericIdentity.projection ||
				!node.genericIdentity.valid(p.terms, p.shape) || node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 || node.binder != 0 || node.route != 0 {
				return false
			}
		case instructionContribution:
			if node.ret == 0 || int(node.ret) >= len(p.arena.returns) || node.effect != 0 || node.call != 0 || node.slot != 0 || node.value != 0 || node.binder != 0 || node.route != 0 {
				return false
			}
		case instructionLoopFeedback:
			if node.binder == 0 || int(node.binder) >= len(p.terms.loopMus) || node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 || node.route != 0 {
				return false
			}
		case instructionLoopExit:
			if node.binder == 0 || int(node.binder) >= len(p.terms.loopMus) || node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 {
				return false
			}
		case instructionBranchRelations:
			requiresKey := node.branch.RequiresDynamicPresenceKey()
			if !node.branch.Valid() || !node.branch.HasStateSteps() && !node.branch.HasSufficientLiteralCases() && !node.branch.HasRefinements() || requiresKey != (node.value != 0) || node.value != 0 && !p.terms.validValue(node.value, p.shape, make(map[ValueTerm]bool)) ||
				node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.binder != 0 || node.route != 0 {
				return false
			}
		case instructionCallResults:
			phaseValid := node.resultPhase == factapply.CallResultPhaseMaterialize && node.result.HasMaterializeSteps() ||
				node.resultPhase == factapply.CallResultPhasePostconditions && node.result.HasPostconditionSteps()
			if !node.result.Valid(p.terms.reg) || !phaseValid || node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 || node.binder != 0 || node.route != 0 {
				return false
			}
		case instructionPresenceImplications:
			if !node.presence.HasPublicationSteps() || !node.presence.Valid(p.terms.reg) {
				return false
			}
		case instructionChannelSelect:
			if !node.channel.HasPublicationSteps() || !node.channel.Valid(p.terms.reg) {
				return false
			}
		case instructionCovariantExposure:
			if !node.covariant.HasStateSteps() || !node.covariant.Valid(p.terms.reg) {
				return false
			}
		case instructionNoNormalReturn:
			if node.effect != 0 || node.call != 0 || node.ret != 0 || node.slot != 0 || node.value != 0 || node.binder != 0 || node.route != 0 || node.result.Len() != 0 || node.resultPhase != factapply.CallResultPhaseInvalid {
				return false
			}
		default:
			return false
		}
	}
	return p.validFlow()
}

func (p WorldProgram) validFlow() bool {
	loopRoutes := make(map[loopMuTerm]int)
	for ref := programRef(1); int(ref) < len(p.arena.programs); ref++ {
		node := p.arena.programs[ref]
		if node.kind != programLoopMu {
			continue
		}
		if _, duplicate := loopRoutes[node.binder]; duplicate {
			return false
		}
		loopRoutes[node.binder] = len(node.exits)
	}

	// A program is an acyclic structural circuit: loop recurrence is named by
	// LoopMu/LoopFeedback terms rather than a raw program edge. Validate frame
	// and loop ownership as the standard must/may dataflow problem over that
	// DAG. The previous validator memoized (node, complete-owned-set), which is
	// semantically exact but enumerates 2^N ownership subsets for N independent
	// branches. Must proves every use is dominated by its definition; may proves
	// a definition cannot occur twice on any path. Those are exactly the two
	// predicates the path enumeration checked, without retaining correlations
	// that no validation rule observes.
	edges := make([][]worldFlowEdge, len(p.arena.programs))
	reachable := make([]bool, len(p.arena.programs))
	stack := []programRef{p.root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref == 0 || int(ref) >= len(p.arena.programs) {
			return false
		}
		if reachable[ref] {
			continue
		}
		reachable[ref] = true
		node := p.arena.programs[ref]
		add := func(target programRef, loop loopMuTerm) {
			if target != 0 {
				edges[ref] = append(edges[ref], worldFlowEdge{target: target, loop: loop})
				stack = append(stack, target)
			}
		}
		switch node.kind {
		case programSequence:
			add(node.next, 0)
		case programChoice:
			add(node.whenTrue, 0)
			add(node.whenFalse, 0)
		case programLoopMu:
			add(node.body, node.binder)
			for _, exit := range node.exits {
				add(exit, 0)
			}
		case programLoopPortal:
			add(node.body, node.binder)
		default:
			return false
		}
	}

	indegree := make([]int, len(p.arena.programs))
	reachableCount := 0
	for ref := programRef(1); int(ref) < len(p.arena.programs); ref++ {
		if !reachable[ref] {
			continue
		}
		reachableCount++
		for _, edge := range edges[ref] {
			if edge.target == 0 || int(edge.target) >= len(p.arena.programs) || !reachable[edge.target] {
				return false
			}
			indegree[edge.target]++
		}
	}
	queue := make([]programRef, 0, reachableCount)
	for ref := programRef(1); int(ref) < len(p.arena.programs); ref++ {
		if reachable[ref] && indegree[ref] == 0 {
			queue = append(queue, ref)
		}
	}
	states := make([]worldFlowState, len(p.arena.programs))
	states[p.root].reached = true
	processed := 0
	for len(queue) != 0 {
		ref := queue[0]
		queue = queue[1:]
		processed++
		state := states[ref]
		if !state.reached {
			return false
		}
		framesMust, framesMay := state.framesMust.clone(), state.framesMay.clone()
		loopsMust, loopsMay := state.loopsMust.clone(), state.loopsMay.clone()
		owned, loops := framesMust.frameSet(), loopsMust.loopSet()
		node := p.arena.programs[ref]
		if node.kind == programChoice {
			if !p.terms.guardFramesOwned(node.guard, owned, make(map[Guard]bool)) {
				return false
			}
		} else if node.kind == programLoopMu || node.kind == programLoopPortal {
			if loopsMay.has(uint32(node.binder)) {
				return false
			}
		}
		terminal := false
		for index, instructionRef := range node.instructions {
			instruction := p.arena.instructions[instructionRef]
			switch instruction.kind {
			case instructionEffect:
				if !effectFramesOwned(p.effects, instruction.effect, owned) {
					return false
				}
			case instructionCallFrame:
				if instruction.guard != 0 && !p.terms.guardFramesOwned(instruction.guard, owned, make(map[Guard]bool)) ||
					!p.terms.validCallFrame(instruction.call, p.shape, owned) {
					return false
				}
				if framesMay.has(uint32(instruction.call)) {
					return false
				}
				owned[instruction.call] = struct{}{}
				framesMust.add(uint32(instruction.call))
				framesMay.add(uint32(instruction.call))
				if instruction.memberCall.site != 0 &&
					(!p.terms.valueFramesOwned(instruction.memberCall.receiver, owned, make(map[ValueTerm]bool)) || !p.terms.valueFramesOwned(instruction.memberCall.provider, owned, make(map[ValueTerm]bool))) {
					return false
				}
			case instructionExternalCall:
				if terminal || instruction.point != node.point ||
					instruction.guard != 0 && !p.terms.guardFramesOwned(instruction.guard, owned, make(map[Guard]bool)) {
					return false
				}
				for _, access := range instruction.access {
					term := access.term
					if !access.hasPoint || term == 0 || !p.terms.valueFramesOwned(term, owned, make(map[ValueTerm]bool)) {
						return false
					}
				}
				if !instruction.operands.valid(p.terms, p.shape, owned) {
					return false
				}
				for _, term := range instruction.writes {
					if term == 0 || !p.terms.valueFramesOwned(term, owned, make(map[ValueTerm]bool)) {
						return false
					}
				}
				if instruction.memberCall.site != 0 &&
					(!p.terms.valueFramesOwned(instruction.memberCall.receiver, owned, make(map[ValueTerm]bool)) ||
						!p.terms.valueFramesOwned(instruction.memberCall.provider, owned, make(map[ValueTerm]bool))) {
					return false
				}
			case instructionRootAssignment:
				if terminal || !instruction.rootAssignment.framesOwned(p.terms, owned) {
					return false
				}
			case instructionReturn:
				publication := p.arena.returns[instruction.ret].resultPublication
				transaction := p.arena.returns[instruction.ret].returnTransaction
				if terminal || index != len(node.instructions)-1 || node.next != 0 || !p.validReturnPayload(instruction.ret, owned) ||
					!transaction.valid(p.terms, p.shape) || !transaction.framesOwned(p.terms, owned) ||
					publication.Len() != 0 && publication.Point() != node.point {
					return false
				}
				terminal = true
			case instructionEnvironmentWrite:
				if terminal || !p.terms.valueFramesOwned(instruction.value, owned, make(map[ValueTerm]bool)) {
					return false
				}
			case instructionGenericFor:
				if terminal || instruction.point != node.point || len(instruction.access) != 1 ||
					instruction.access[0].hasPoint || instruction.access[0].term != instruction.genericIdentity.projection ||
					!instruction.genericIdentity.valid(p.terms, p.shape) {
					return false
				}
				for _, access := range instruction.access {
					term := access.term
					if access.hasPoint || term == 0 || !p.terms.valueFramesOwned(term, owned, make(map[ValueTerm]bool)) {
						return false
					}
				}
			case instructionContribution:
				if terminal || !p.validReturnPayload(instruction.ret, owned) {
					return false
				}
			case instructionLoopFeedback:
				if terminal || index != len(node.instructions)-1 || node.next != 0 {
					return false
				}
				if _, active := loops[instruction.binder]; !active {
					return false
				}
				terminal = true
			case instructionLoopExit:
				if terminal || index != len(node.instructions)-1 || node.next != 0 {
					return false
				}
				routes, declared := loopRoutes[instruction.binder]
				if _, active := loops[instruction.binder]; !active || !declared || int(instruction.route) >= routes {
					return false
				}
				terminal = true
			case instructionBranchRelations:
				if terminal || !instruction.branch.Valid() || !instruction.branch.HasStateSteps() && !instruction.branch.HasSufficientLiteralCases() && !instruction.branch.HasRefinements() || instruction.branch.Point() != node.point ||
					instruction.branch.RequiresDynamicPresenceKey() != (instruction.value != 0) || instruction.value != 0 && !p.terms.valueFramesOwned(instruction.value, owned, make(map[ValueTerm]bool)) {
					return false
				}
			case instructionCallResults:
				phaseValid := instruction.resultPhase == factapply.CallResultPhaseMaterialize && instruction.result.HasMaterializeSteps() ||
					instruction.resultPhase == factapply.CallResultPhasePostconditions && instruction.result.HasPostconditionSteps()
				if terminal || !instruction.result.Valid(p.terms.reg) || !phaseValid || instruction.result.Point() != node.point {
					return false
				}
			case instructionPresenceImplications:
				if terminal || !instruction.presence.HasPublicationSteps() || !instruction.presence.Valid(p.terms.reg) || instruction.presence.Point() != node.point {
					return false
				}
			case instructionChannelSelect:
				if terminal || !instruction.channel.HasPublicationSteps() || !instruction.channel.Valid(p.terms.reg) || instruction.channel.Point() != node.point {
					return false
				}
			case instructionCovariantExposure:
				if terminal || !instruction.covariant.HasStateSteps() || !instruction.covariant.Valid(p.terms.reg) || instruction.covariant.Point() != node.point {
					return false
				}
			case instructionNoNormalReturn:
				if terminal || index != len(node.instructions)-1 || node.next != 0 {
					return false
				}
				terminal = true
			default:
				return false
			}
		}
		if node.kind == programSequence && !terminal && node.next == 0 {
			return false
		}
		for _, edge := range edges[ref] {
			outLoopsMust, outLoopsMay := loopsMust, loopsMay
			if edge.loop != 0 {
				outLoopsMust, outLoopsMay = loopsMust.clone(), loopsMay.clone()
				outLoopsMust.add(uint32(edge.loop))
				outLoopsMay.add(uint32(edge.loop))
			}
			states[edge.target].merge(framesMust, framesMay, outLoopsMust, outLoopsMay)
			indegree[edge.target]--
			if indegree[edge.target] == 0 {
				queue = append(queue, edge.target)
			}
		}
	}
	return processed == reachableCount
}

func (p WorldProgram) validReturnPayload(ref returnPayloadRef, owned map[callFrameTerm]struct{}) bool {
	if ref == 0 || int(ref) >= len(p.arena.returns) {
		return false
	}
	payload := p.arena.returns[ref]
	for _, operation := range payload.operations {
		if operation.Kind >= outputKindCount || operation.Value == 0 || !p.terms.validValue(operation.Value, p.shape, make(map[ValueTerm]bool)) ||
			!p.terms.valueFramesOwned(operation.Value, owned, make(map[ValueTerm]bool)) {
			return false
		}
	}
	for _, proof := range payload.proofs {
		if !proof.valid(p.terms, p.shape) || proof.Key != 0 && !p.terms.valueFramesOwned(proof.Key, owned, make(map[ValueTerm]bool)) {
			return false
		}
	}
	for _, term := range payload.branchLiteralCases {
		if term.literalCase.TargetPathRef().IsEmpty() || !product.BelongsToRegistry(p.terms.reg, term.literalCase.LiteralValue()) {
			return false
		}
	}
	if payload.resultPublication.Len() != 0 && (!payload.resultPublication.Valid(p.terms.reg) || !payload.resultPublication.HasPublicationSteps()) {
		return false
	}
	if payload.covariant.Len() != 0 && (!payload.covariant.Valid(p.terms.reg) || !payload.covariant.HasStateSteps()) {
		return false
	}
	if !validBoundaryParamObligations(p.terms.reg, p.shape.Params, payload.paramObligations) ||
		!validBoundaryPathObligations(p.terms, p.shape, payload.pathObligations) ||
		!validBoundaryParamExposures(p.terms, p.shape, payload.paramExposures) {
		return false
	}
	return true
}
