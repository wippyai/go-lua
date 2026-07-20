package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type relationRootRef uint32
type boundaryOutcomeRef uint32
type boundaryContributionRef uint32

type boundaryStepKind uint8

const (
	boundaryStepInvalid boundaryStepKind = iota
	boundaryStepEffect
	boundaryStepApply
	boundaryStepExternalCall
	boundaryStepRootAssignment
	boundaryStepEnvironmentWrite
	boundaryStepGenericFor
	boundaryStepContribution
	boundaryStepLoopFeedback
	boundaryStepLoopExit
	boundaryStepBranchRelations
	boundaryStepCallResults
	boundaryStepPresenceImplications
	boundaryStepChannelSelect
	boundaryStepCovariantExposure
)

// relationApplyRef is a frozen lexical application. The frame owns the lazy
// caller-to-callee BindingEnv; variable is linked to reduced callee code only
// after every lexical function in the call SCC has frozen.
type relationApplyRef struct {
	variable relationVar
	frame    callFrameTerm
}

type boundaryStep struct {
	kind            boundaryStepKind
	point           cfg.Point
	effect          EffectTerm
	apply           relationApplyRef
	guard           Guard
	slot            statekey.Value
	value           ValueTerm
	access          []valueAccessTerm
	genericIdentity frozenGenericForIdentityPublication
	operands        callOutcomeOperandTerms
	writes          []ValueTerm
	memberCall      boundaryMemberCallDiagnosticTerm
	binder          loopMuTerm
	contribution    boundaryContributionRef
	route           uint32
	branch          factapply.BranchRelationTransaction
	result          factapply.CallResultTransaction
	resultPhase     factapply.ConcreteCallResultPhase
	presence        factapply.PathValuePresenceImplicationTransaction
	channel         factapply.ChannelSelectTransaction
	covariant       factapply.CovariantExposureTransaction
	rootAssignment  rootAssignmentTerm
}

type relationNodeKind uint8

const (
	relationNodeInvalid relationNodeKind = iota
	relationNodeBottom
	relationNodeNonreturning
	relationNodeSequence
	relationNodeOutcome
	relationNodeChoice
	relationNodeLoopMu
	relationNodeLoopPortal
)

// relationCode is the callable, reduced boundary transformer. It contains no
// CFG points or WorldProgram refs. Calls may reference only relation variables
// in this code layer; therefore Apply cannot traverse a callee lexical body.
type relationCode struct {
	terms         *Arena
	effects       *EffectArena
	descriptors   *DescriptorRegistry
	shape         Shape
	root          relationRootRef
	nodes         []relationNode
	outcomes      []boundaryOutcomeTuple
	contributions []semanticContribution
	variables     []relationVar
	publication   relationPublicationPlan
	// applicationGuards is indexed by caller-owned callFrameTerm. It is frozen
	// only after every arena in the relation forest has sealed its complete
	// boundary syntax, so Apply execution never imports or interns target terms.
	applicationGuards []relationApplicationGuardPlan
	sealed            bool
}

type relationPublicationPlan struct {
	points []relationPointPublication
	edges  []relationEdgePublication
}

type relationPointPublication struct {
	point cfg.Point
	ref   relationRootRef
}

type relationEdgePublication struct {
	from, to cfg.Point
	ref      relationRootRef
}

type relationNode struct {
	kind      relationNodeKind
	steps     []boundaryStep
	next      relationRootRef
	guard     Guard
	whenTrue  relationRootRef
	whenFalse relationRootRef
	outcome   boundaryOutcomeRef
	binder    loopMuTerm
	body      relationRootRef
	exits     []relationRootRef
}

// boundaryOutcomeTuple is one normal-continuation result after symbolic
// reduction. Absence of an outcome is State/world Bottom (no normal
// continuation). Suspension and protected-call evidence use their existing
// typed boundary lanes; no synthetic route enum is introduced. Effect terms
// live on structural Sequence nodes, so shared continuations are not copied
// into terminal tuples. The remaining fields are symbolic boundary/diagnostic
// outputs. Summary is deliberately not part of this equation payload and is
// projected once after stabilization.
type boundaryOutcomeTuple struct {
	suspensionKnown        bool
	maySuspend             bool
	protectedCallTypestate callboundary.ProtectedCallTypestate
	operations             []Operation
	proofs                 []BranchProofTerm
	refinements            []PathRefinementTerm
	observations           []ObservationTerm
	observationObligations []observationObligation
	preserved              paramPreservationLedger
	returnConditions       []returnConditionParamRefinementTerm
	branchLiteralCases     []branchSufficientOutcomeTerm
	resultPublication      factapply.CallResultTransaction
	covariant              factapply.CovariantExposureTransaction
	paramObligations       []callpayload.CallParamObligation
	pathObligations        []boundaryPathObligationTerm
	paramExposures         []boundaryParamExposureTerm
	returnTransaction      returnTransactionTerm
}

// reduceWorldProgram lowers an already-frozen acyclic lexical circuit without
// enumerating paths or duplicating suffixes. Each reachable WorldProgram node
// receives one stable relation node; shared continuations remain shared.
func reduceWorldProgram(program WorldProgram, descriptors *DescriptorRegistry) (*relationCode, relationRootRef, error) {
	code, root, err := reduceWorldProgramUnsealed(program, descriptors)
	if err != nil {
		return nil, 0, err
	}
	return sealRelationCode(code, root)
}

// reduceWorldProgramUnsealed builds the sole transducer representation while
// its term/effect owners remain open for dependency-order acyclic closure.
// Only FreezeRelationProgram may retain this intermediate across bodies.
func reduceWorldProgramUnsealed(program WorldProgram, descriptors *DescriptorRegistry) (*relationCode, relationRootRef, error) {
	if !program.valid(true) {
		return nil, 0, fmt.Errorf("transformer: cannot reduce malformed lexical program")
	}
	if descriptors == nil || !descriptors.validSchema(program.terms.reg) {
		return nil, 0, fmt.Errorf("transformer: reduced relation requires sealed descriptor ownership")
	}
	reachable := make([]bool, len(program.arena.programs))
	stack := []programRef{program.root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref == 0 || reachable[ref] {
			continue
		}
		reachable[ref] = true
		node := program.arena.programs[ref]
		if node.whenFalse != 0 {
			stack = append(stack, node.whenFalse)
		}
		if node.whenTrue != 0 {
			stack = append(stack, node.whenTrue)
		}
		if node.next != 0 {
			stack = append(stack, node.next)
		}
		if node.body != 0 {
			stack = append(stack, node.body)
		}
		stack = append(stack, node.exits...)
	}
	mapping := make([]relationRootRef, len(program.arena.programs))
	nodes := []relationNode{{}}
	for ref := programRef(1); int(ref) < len(reachable); ref++ {
		if reachable[ref] {
			mapping[ref] = relationRootRef(len(nodes))
			nodes = append(nodes, relationNode{})
		}
	}
	code := &relationCode{
		terms: program.terms, effects: program.effects, descriptors: descriptors, shape: program.shape,
		nodes: nodes, outcomes: []boundaryOutcomeTuple{{}}, contributions: []semanticContribution{{}},
	}
	for ref := programRef(1); int(ref) < len(reachable); ref++ {
		if !reachable[ref] {
			continue
		}
		source := program.arena.programs[ref]
		targetIndex := int(mapping[ref])
		switch source.kind {
		case programChoice:
			code.nodes[targetIndex] = relationNode{kind: relationNodeChoice, guard: source.guard, whenTrue: mapping[source.whenTrue], whenFalse: mapping[source.whenFalse]}
		case programLoopMu:
			exits := make([]relationRootRef, len(source.exits))
			for index, exit := range source.exits {
				exits[index] = mapping[exit]
			}
			code.nodes[targetIndex] = relationNode{kind: relationNodeLoopMu, binder: source.binder, body: mapping[source.body], exits: exits}
		case programLoopPortal:
			code.nodes[targetIndex] = relationNode{kind: relationNodeLoopPortal, binder: source.binder, body: mapping[source.body]}
		case programSequence:
			steps := make([]boundaryStep, 0, len(source.instructions))
			var terminal boundaryOutcomeRef
			bottom := false
			for _, instructionRef := range source.instructions {
				instruction := program.arena.instructions[instructionRef]
				switch instruction.kind {
				case instructionEffect:
					steps = append(steps, boundaryStep{kind: boundaryStepEffect, effect: instruction.effect})
				case instructionCallFrame:
					frame := program.terms.callFrames[instruction.call]
					if frame.variable == 0 {
						return nil, 0, fmt.Errorf("transformer: lexical call has no forest relation variable")
					}
					steps = append(steps, boundaryStep{kind: boundaryStepApply, apply: relationApplyRef{variable: frame.variable, frame: instruction.call}, guard: instruction.guard, memberCall: instruction.memberCall})
				case instructionExternalCall:
					steps = append(steps, boundaryStep{kind: boundaryStepExternalCall, point: instruction.point, guard: instruction.guard, access: cloneValueAccessTerms(instruction.access), operands: instruction.operands.clone(), writes: append([]ValueTerm(nil), instruction.writes...), memberCall: instruction.memberCall})
				case instructionRootAssignment:
					steps = append(steps, boundaryStep{kind: boundaryStepRootAssignment, rootAssignment: instruction.rootAssignment})
				case instructionEnvironmentWrite:
					steps = append(steps, boundaryStep{kind: boundaryStepEnvironmentWrite, point: source.point, slot: instruction.slot, value: instruction.value})
				case instructionGenericFor:
					steps = append(steps, boundaryStep{kind: boundaryStepGenericFor, point: instruction.point, access: cloneValueAccessTerms(instruction.access), genericIdentity: instruction.genericIdentity.clone()})
				case instructionContribution:
					payload := program.arena.returns[instruction.ret]
					contribution := boundaryContributionRef(len(code.contributions))
					code.contributions = append(code.contributions, payload.clone())
					steps = append(steps, boundaryStep{kind: boundaryStepContribution, contribution: contribution})
				case instructionLoopFeedback:
					steps = append(steps, boundaryStep{kind: boundaryStepLoopFeedback, binder: instruction.binder})
				case instructionLoopExit:
					steps = append(steps, boundaryStep{kind: boundaryStepLoopExit, binder: instruction.binder, route: instruction.route})
				case instructionBranchRelations:
					steps = append(steps, boundaryStep{kind: boundaryStepBranchRelations, branch: instruction.branch, value: instruction.value})
				case instructionCallResults:
					steps = append(steps, boundaryStep{kind: boundaryStepCallResults, result: instruction.result, resultPhase: instruction.resultPhase})
				case instructionPresenceImplications:
					steps = append(steps, boundaryStep{kind: boundaryStepPresenceImplications, presence: instruction.presence.Clone()})
				case instructionChannelSelect:
					steps = append(steps, boundaryStep{kind: boundaryStepChannelSelect, channel: instruction.channel.Clone()})
				case instructionCovariantExposure:
					steps = append(steps, boundaryStep{kind: boundaryStepCovariantExposure, covariant: instruction.covariant.Clone()})
				case instructionNoNormalReturn:
					bottom = true
				case instructionReturn:
					payload := program.arena.returns[instruction.ret]
					terminal = boundaryOutcomeRef(len(code.outcomes))
					outcome, err := reduceSemanticContribution(program, payload)
					if err != nil {
						return nil, 0, err
					}
					code.outcomes = append(code.outcomes, outcome)
				default:
					return nil, 0, fmt.Errorf("transformer: lexical program has invalid instruction")
				}
			}
			if bottom && terminal != 0 {
				return nil, 0, fmt.Errorf("transformer: no-normal-return sequence also retained an outcome")
			} else if bottom && len(steps) == 0 {
				code.nodes[targetIndex] = relationNode{kind: relationNodeNonreturning}
			} else if bottom {
				bottomNode := relationRootRef(len(code.nodes))
				code.nodes = append(code.nodes, relationNode{kind: relationNodeNonreturning})
				code.nodes[targetIndex] = relationNode{kind: relationNodeSequence, steps: steps, next: bottomNode}
			} else if terminal != 0 && len(steps) == 0 {
				code.nodes[targetIndex] = relationNode{kind: relationNodeOutcome, outcome: terminal}
			} else if terminal != 0 {
				outcomeNode := relationRootRef(len(code.nodes))
				code.nodes = append(code.nodes, relationNode{kind: relationNodeOutcome, outcome: terminal})
				code.nodes[targetIndex] = relationNode{kind: relationNodeSequence, steps: steps, next: outcomeNode}
			} else {
				code.nodes[targetIndex] = relationNode{kind: relationNodeSequence, steps: steps, next: mapping[source.next]}
			}
		default:
			return nil, 0, fmt.Errorf("transformer: invalid lexical program node")
		}
	}
	for _, selector := range program.publication.points {
		if selector.ref == 0 || int(selector.ref) >= len(mapping) {
			return nil, 0, fmt.Errorf("transformer: point publication escaped reduced relation")
		}
		code.publication.points = append(code.publication.points, relationPointPublication{point: selector.point, ref: mapping[selector.ref]})
	}
	for _, selector := range program.publication.edges {
		if selector.ref == 0 || int(selector.ref) >= len(mapping) {
			return nil, 0, fmt.Errorf("transformer: edge publication escaped reduced relation")
		}
		code.publication.edges = append(code.publication.edges, relationEdgePublication{from: selector.from, to: selector.to, ref: mapping[selector.ref]})
	}
	code.root = mapping[program.root]
	return code, code.root, nil
}

func reduceSemanticContribution(program WorldProgram, payload semanticContribution) (boundaryOutcomeTuple, error) {
	return boundaryOutcomeTuple{
		suspensionKnown: payload.suspensionKnown, maySuspend: payload.maySuspend,
		protectedCallTypestate: payload.protectedCallTypestate.Clone(), operations: append([]Operation(nil), payload.operations...),
		proofs: append([]BranchProofTerm(nil), payload.proofs...), refinements: append([]PathRefinementTerm(nil), payload.refinements...),
		observations: append([]ObservationTerm(nil), payload.observations...), observationObligations: append([]observationObligation(nil), payload.observationObligations...), preserved: payload.preserved.clone(),
		returnConditions:   append([]returnConditionParamRefinementTerm(nil), payload.returnConditions...),
		branchLiteralCases: cloneBranchSufficientOutcomeTerms(payload.branchLiteralCases),
		resultPublication:  payload.resultPublication,
		covariant:          payload.covariant.Clone(),
		paramObligations:   cloneBoundaryParamObligations(payload.paramObligations),
		pathObligations:    cloneBoundaryPathObligations(payload.pathObligations),
		paramExposures:     cloneBoundaryParamExposures(payload.paramExposures),
		returnTransaction:  payload.returnTransaction.clone(),
	}, nil
}

// sealRelationCode is the sole whole-code validation transaction. It proves
// rooted closure and acyclicity, drops unreachable nodes/outcomes, verifies all
// arena/shape/descriptor ownership, and only then permanently seals the code.
// The zero child of Choice denotes semantic Bottom; recurrence is forbidden
// here and is represented only by typed LoopMu reduction.
func sealRelationCode(code *relationCode, root relationRootRef) (*relationCode, relationRootRef, error) {
	if code != nil && len(code.outcomes) == 0 {
		code.outcomes = []boundaryOutcomeTuple{{}}
	}
	if code != nil && len(code.contributions) == 0 {
		code.contributions = []semanticContribution{{}}
	}
	if code == nil || code.terms == nil || code.terms.reg == nil || code.effects == nil || code.effects.terms != code.terms ||
		code.descriptors == nil || !code.descriptors.validSchema(code.terms.reg) || code.terms.Sealed() != code.effects.Sealed() ||
		root == 0 || int(root) >= len(code.nodes) || len(code.outcomes) == 0 || len(code.contributions) == 0 {
		return nil, 0, fmt.Errorf("transformer: malformed reduced relation owners")
	}
	reachableNodes, reachableOutcomes, topology, err := relationTopology(code, root)
	if err != nil {
		return nil, 0, err
	}
	if !validateRelationFlow(code, root, topology) {
		return nil, 0, fmt.Errorf("transformer: reduced relation has invalid ordered frame flow")
	}
	variableSet := make(map[relationVar]struct{})
	reachableContributions := make([]bool, len(code.contributions))
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		if reachableNodes[ref] {
			for _, step := range code.nodes[ref].steps {
				if step.kind == boundaryStepApply {
					variableSet[step.apply.variable] = struct{}{}
				}
				if step.kind == boundaryStepContribution {
					if step.contribution == 0 || int(step.contribution) >= len(code.contributions) {
						return nil, 0, fmt.Errorf("transformer: relation contribution is outside frozen table")
					}
					reachableContributions[step.contribution] = true
				}
			}
		}
	}
	nodeMap := make([]relationRootRef, len(code.nodes))
	nodes := []relationNode{{}}
	for old := relationRootRef(1); int(old) < len(code.nodes); old++ {
		if reachableNodes[old] {
			nodeMap[old] = relationRootRef(len(nodes))
			nodes = append(nodes, code.nodes[old])
		}
	}
	outcomeMap := make([]boundaryOutcomeRef, len(code.outcomes))
	outcomes := []boundaryOutcomeTuple{{}}
	for old := boundaryOutcomeRef(1); int(old) < len(code.outcomes); old++ {
		if reachableOutcomes[old] {
			outcomeMap[old] = boundaryOutcomeRef(len(outcomes))
			outcomes = append(outcomes, cloneBoundaryOutcome(code.outcomes[old]))
		}
	}
	contributionMap := make([]boundaryContributionRef, len(code.contributions))
	contributions := []semanticContribution{{}}
	for old := boundaryContributionRef(1); int(old) < len(code.contributions); old++ {
		if reachableContributions[old] {
			contributionMap[old] = boundaryContributionRef(len(contributions))
			contributions = append(contributions, code.contributions[old].clone())
		}
	}
	for index := 1; index < len(nodes); index++ {
		node := &nodes[index]
		node.steps = append([]boundaryStep(nil), node.steps...)
		for step := range node.steps {
			if node.steps[step].kind == boundaryStepContribution {
				node.steps[step].contribution = contributionMap[node.steps[step].contribution]
			}
		}
		node.next = nodeMap[node.next]
		node.whenTrue = nodeMap[node.whenTrue]
		node.whenFalse = nodeMap[node.whenFalse]
		node.body = nodeMap[node.body]
		node.exits = append([]relationRootRef(nil), node.exits...)
		for route := range node.exits {
			node.exits[route] = nodeMap[node.exits[route]]
		}
		node.outcome = outcomeMap[node.outcome]
	}
	for index := range code.publication.points {
		selector := &code.publication.points[index]
		selector.ref = nodeMap[selector.ref]
	}
	for index := range code.publication.edges {
		selector := &code.publication.edges[index]
		selector.ref = nodeMap[selector.ref]
	}
	variables := make([]relationVar, 0, len(variableSet))
	for variable := range variableSet {
		variables = append(variables, variable)
	}
	sort.Slice(variables, func(i, j int) bool { return variables[i] < variables[j] })
	code.nodes, code.outcomes, code.contributions, code.variables, code.root = nodes, outcomes, contributions, variables, nodeMap[root]
	if !code.terms.Sealed() {
		code.terms.Seal()
		code.effects.Seal()
	}
	code.sealed = true
	return code, code.root, nil
}

func cloneBoundaryOutcome(in boundaryOutcomeTuple) boundaryOutcomeTuple {
	in.protectedCallTypestate = in.protectedCallTypestate.Clone()
	in.operations = append([]Operation(nil), in.operations...)
	in.proofs = append([]BranchProofTerm(nil), in.proofs...)
	in.refinements = append([]PathRefinementTerm(nil), in.refinements...)
	in.observations = append([]ObservationTerm(nil), in.observations...)
	in.observationObligations = append([]observationObligation(nil), in.observationObligations...)
	in.preserved = in.preserved.clone()
	in.returnConditions = append([]returnConditionParamRefinementTerm(nil), in.returnConditions...)
	in.branchLiteralCases = cloneBranchSufficientOutcomeTerms(in.branchLiteralCases)
	in.resultPublication = in.resultPublication.Clone()
	in.covariant = in.covariant.Clone()
	in.paramObligations = cloneBoundaryParamObligations(in.paramObligations)
	in.pathObligations = cloneBoundaryPathObligations(in.pathObligations)
	in.paramExposures = cloneBoundaryParamExposures(in.paramExposures)
	in.returnTransaction = in.returnTransaction.clone()
	return in
}

func relationTopology(code *relationCode, root relationRootRef) ([]bool, []bool, []relationRootRef, error) {
	reachable := make([]bool, len(code.nodes))
	outcomes := make([]bool, len(code.outcomes))
	stack := []relationRootRef{root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref == 0 || int(ref) >= len(code.nodes) {
			if ref != 0 {
				return nil, nil, nil, fmt.Errorf("transformer: relation edge is outside code")
			}
			continue
		}
		if reachable[ref] {
			continue
		}
		reachable[ref] = true
		node := code.nodes[ref]
		switch node.kind {
		case relationNodeBottom:
			if len(node.steps) != 0 || node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.outcome != 0 || node.binder != 0 || node.body != 0 || len(node.exits) != 0 {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation Bottom node")
			}
		case relationNodeNonreturning:
			if len(node.steps) != 0 || node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.outcome != 0 || node.binder != 0 || node.body != 0 || len(node.exits) != 0 {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation nonreturning node")
			}
		case relationNodeSequence:
			feedbackTerminal := len(node.steps) != 0 && (node.steps[len(node.steps)-1].kind == boundaryStepLoopFeedback || node.steps[len(node.steps)-1].kind == boundaryStepLoopExit)
			if (node.next == 0) != feedbackTerminal || node.next != 0 && int(node.next) >= len(code.nodes) || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.outcome != 0 {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation sequence node")
			}
			stack = append(stack, node.next)
		case relationNodeOutcome:
			if len(node.steps) != 0 || node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.outcome == 0 || int(node.outcome) >= len(code.outcomes) {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation outcome node")
			}
			outcomes[node.outcome] = true
		case relationNodeChoice:
			if len(node.steps) != 0 || node.next != 0 || node.guard == 0 || !code.terms.validGuard(node.guard, code.shape) || node.outcome != 0 || node.whenTrue == 0 && node.whenFalse == 0 {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation choice node")
			}
			if node.whenFalse != 0 {
				stack = append(stack, node.whenFalse)
			}
			if node.whenTrue != 0 {
				stack = append(stack, node.whenTrue)
			}
		case relationNodeLoopMu:
			if len(node.steps) != 0 || node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.outcome != 0 ||
				node.binder == 0 || int(node.binder) >= len(code.terms.loopMus) || node.body == 0 {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation LoopMu node")
			}
			stack = append(stack, node.body)
			stack = append(stack, node.exits...)
		case relationNodeLoopPortal:
			if len(node.steps) != 0 || node.next != 0 || node.guard != 0 || node.whenTrue != 0 || node.whenFalse != 0 || node.outcome != 0 ||
				node.binder == 0 || int(node.binder) >= len(code.terms.loopMus) || node.body == 0 || len(node.exits) != 0 {
				return nil, nil, nil, fmt.Errorf("transformer: malformed relation LoopPortal node")
			}
			stack = append(stack, node.body)
		default:
			return nil, nil, nil, fmt.Errorf("transformer: invalid relation node kind")
		}
	}
	indegree := make([]int, len(code.nodes))
	reachableCount := 0
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		if !reachable[ref] {
			continue
		}
		reachableCount++
		for _, successor := range relationSuccessors(code.nodes[ref]) {
			if successor != 0 {
				indegree[successor]++
			}
		}
	}
	queue := make([]relationRootRef, 0, reachableCount)
	for ref := relationRootRef(1); int(ref) < len(code.nodes); ref++ {
		if reachable[ref] && indegree[ref] == 0 {
			queue = append(queue, ref)
		}
	}
	topology := make([]relationRootRef, 0, reachableCount)
	for len(queue) != 0 {
		ref := queue[0]
		queue = queue[1:]
		topology = append(topology, ref)
		for _, successor := range relationSuccessors(code.nodes[ref]) {
			if successor == 0 {
				continue
			}
			indegree[successor]--
			if indegree[successor] == 0 {
				queue = append(queue, successor)
			}
		}
	}
	if len(topology) != reachableCount {
		return nil, nil, nil, fmt.Errorf("transformer: raw relation recurrence requires typed mu")
	}
	return reachable, outcomes, topology, nil
}

func relationSuccessors(node relationNode) []relationRootRef {
	switch node.kind {
	case relationNodeSequence:
		return []relationRootRef{node.next}
	case relationNodeChoice:
		return []relationRootRef{node.whenTrue, node.whenFalse}
	case relationNodeLoopMu:
		return append([]relationRootRef{node.body}, node.exits...)
	case relationNodeLoopPortal:
		return []relationRootRef{node.body}
	default:
		return nil
	}
}

func validateRelationFlow(code *relationCode, root relationRootRef, topology []relationRootRef) bool {
	if code == nil || root == 0 || len(topology) == 0 || topology[0] != root {
		return false
	}
	words := (len(code.terms.callFrames) + 63) / 64
	must := make([][]uint64, len(code.nodes))
	may := make([][]uint64, len(code.nodes))
	arrived := make([]bool, len(code.nodes))
	arrived[root] = true
	for _, ref := range topology {
		if !arrived[ref] {
			return false
		}
		if must[ref] == nil {
			must[ref], may[ref] = make([]uint64, words), make([]uint64, words)
		}
		node := code.nodes[ref]
		switch node.kind {
		case relationNodeBottom:
			// Bottom has no outgoing normal-continuation equation.
		case relationNodeNonreturning:
			// A typed terminal retains its incoming State outside normal flow.
		case relationNodeSequence:
			for _, step := range node.steps {
				if step.guard != 0 && !guardFramesOwnedBits(code.terms, step.guard, must[ref]) {
					return false
				}
				switch step.kind {
				case boundaryStepEffect:
					if step.effect == 0 || step.apply != (relationApplyRef{}) || !code.effects.Valid(step.effect, code.shape) || !effectFramesOwnedBits(code.effects, step.effect, must[ref]) {
						return false
					}
				case boundaryStepApply:
					frame := step.apply.frame
					if step.effect != 0 || step.apply.variable == 0 || frame == 0 || frameBitSet(may[ref], frame) || !validCallFrameBits(code.terms, frame, code.shape, must[ref]) || code.terms.callFrames[frame].variable != step.apply.variable {
						return false
					}
					setFrameBit(must[ref], frame)
					setFrameBit(may[ref], frame)
					if step.memberCall.site != 0 && (step.memberCall.site != code.terms.callFrames[frame].point ||
						!validBoundaryMemberCallDiagnostics(code.terms.reg, code.terms, code.shape, []boundaryMemberCallDiagnosticTerm{step.memberCall}) ||
						!valueFramesOwnedBits(code.terms, step.memberCall.receiver, must[ref]) || !valueFramesOwnedBits(code.terms, step.memberCall.provider, must[ref])) {
						return false
					}
				case boundaryStepExternalCall:
					if step.point == 0 || step.effect != 0 || step.apply != (relationApplyRef{}) {
						return false
					}
					for _, access := range step.access {
						term := access.term
						if !access.hasPoint || term == 0 || !code.terms.validValue(term, code.shape, make(map[ValueTerm]bool)) || !valueFramesOwnedBits(code.terms, term, must[ref]) {
							return false
						}
					}
					if !step.operands.validBits(code.terms, code.shape, must[ref]) {
						return false
					}
					for _, term := range step.writes {
						if term == 0 || !code.terms.validValue(term, code.shape, make(map[ValueTerm]bool)) || !valueFramesOwnedBits(code.terms, term, must[ref]) {
							return false
						}
					}
					if step.memberCall.site != 0 && (step.memberCall.site != step.point ||
						!validBoundaryMemberCallDiagnostics(code.terms.reg, code.terms, code.shape, []boundaryMemberCallDiagnosticTerm{step.memberCall}) ||
						!valueFramesOwnedBits(code.terms, step.memberCall.receiver, must[ref]) ||
						!valueFramesOwnedBits(code.terms, step.memberCall.provider, must[ref])) {
						return false
					}
				case boundaryStepRootAssignment:
					if !step.rootAssignment.valid(code.terms, code.shape) || !step.rootAssignment.framesOwnedBits(code.terms, must[ref]) {
						return false
					}
				case boundaryStepEnvironmentWrite:
					if !code.terms.validEnvironmentSlot(step.slot) || step.value == 0 || !code.terms.validValue(step.value, code.shape, make(map[ValueTerm]bool)) || !valueFramesOwnedBits(code.terms, step.value, must[ref]) {
						return false
					}
				case boundaryStepGenericFor:
					if step.point == 0 || len(step.access) != 1 || step.access[0].hasPoint ||
						step.access[0].term != step.genericIdentity.projection || !step.genericIdentity.valid(code.terms, code.shape) {
						return false
					}
					for _, access := range step.access {
						term := access.term
						if access.hasPoint || term == 0 || !code.terms.validValue(term, code.shape, make(map[ValueTerm]bool)) || !valueFramesOwnedBits(code.terms, term, must[ref]) {
							return false
						}
					}
				case boundaryStepContribution:
					if step.contribution == 0 || int(step.contribution) >= len(code.contributions) || !code.validContribution(code.contributions[step.contribution], must[ref]) {
						return false
					}
				case boundaryStepLoopFeedback:
					if step.binder == 0 || int(step.binder) >= len(code.terms.loopMus) {
						return false
					}
				case boundaryStepLoopExit:
					if step.binder == 0 || int(step.binder) >= len(code.terms.loopMus) {
						return false
					}
				case boundaryStepBranchRelations:
					if !step.branch.Valid() || !step.branch.HasStateSteps() && !step.branch.HasSufficientLiteralCases() && !step.branch.HasRefinements() || step.branch.RequiresDynamicPresenceKey() != (step.value != 0) ||
						step.value != 0 && (!code.terms.validValue(step.value, code.shape, make(map[ValueTerm]bool)) || !valueFramesOwnedBits(code.terms, step.value, must[ref])) {
						return false
					}
				case boundaryStepCallResults:
					phaseValid := step.resultPhase == factapply.ConcreteCallResultPhaseMaterialize && step.result.HasMaterializeSteps() ||
						step.resultPhase == factapply.ConcreteCallResultPhasePostconditions && step.result.HasPostconditionSteps()
					if !step.result.Valid(code.terms.reg) || !phaseValid {
						return false
					}
				case boundaryStepPresenceImplications:
					if !step.presence.HasPublicationSteps() || !step.presence.Valid(code.terms.reg) {
						return false
					}
				case boundaryStepChannelSelect:
					if !step.channel.HasPublicationSteps() || !step.channel.Valid(code.terms.reg) {
						return false
					}
				case boundaryStepCovariantExposure:
					if !step.covariant.HasStateSteps() || !step.covariant.Valid(code.terms.reg) {
						return false
					}
				default:
					return false
				}
			}
		case relationNodeChoice:
			if !guardFramesOwnedBits(code.terms, node.guard, must[ref]) {
				return false
			}
		case relationNodeOutcome:
			outcome := code.outcomes[node.outcome]
			if !code.validOutcome(outcome, must[ref]) || !outcome.returnTransaction.valid(code.terms, code.shape) ||
				!outcome.returnTransaction.framesOwnedBits(code.terms, must[ref]) {
				return false
			}
		case relationNodeLoopMu:
			if node.binder == 0 || int(node.binder) >= len(code.terms.loopMus) {
				return false
			}
		case relationNodeLoopPortal:
			if node.binder == 0 || int(node.binder) >= len(code.terms.loopMus) {
				return false
			}
		}
		for _, successor := range relationSuccessors(node) {
			if successor == 0 {
				continue
			}
			if !arrived[successor] {
				must[successor] = append([]uint64(nil), must[ref]...)
				may[successor] = append([]uint64(nil), may[ref]...)
				arrived[successor] = true
				continue
			}
			for word := 0; word < words; word++ {
				must[successor][word] &= must[ref][word]
				may[successor][word] |= may[ref][word]
			}
		}
	}
	return true
}

func frameBitSet(bits []uint64, frame callFrameTerm) bool {
	word, bit := int(frame)/64, uint(frame)%64
	return word < len(bits) && bits[word]&(uint64(1)<<bit) != 0
}

func setFrameBit(bits []uint64, frame callFrameTerm) {
	word, bit := int(frame)/64, uint(frame)%64
	bits[word] |= uint64(1) << bit
}

// valueFramesOwnedBits validates frame ownership by walking the hash-consed
// value DAG once. It has no recursion depth and never enumerates root-to-leaf
// paths; shared suffixes are visited once.
func valueFramesOwnedBits(arena *Arena, term ValueTerm, frames []uint64) bool {
	if arena == nil || term == 0 || int(term) >= len(arena.values) {
		return false
	}
	seen := make([]bool, len(arena.values))
	stack := []ValueTerm{term}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == 0 || int(current) >= len(arena.values) {
			return false
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		node := arena.values[current]
		if node.op == valueFrameResult && !frameBitSet(frames, node.frame) {
			return false
		}
		stack = append(stack, node.args...)
	}
	return true
}

func guardFramesOwnedBits(arena *Arena, guard Guard, frames []uint64) bool {
	if arena == nil || guard == 0 || int(guard) >= len(arena.guards) {
		return false
	}
	seen := make([]bool, len(arena.guards))
	stack := []Guard{guard}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current == 0 || int(current) >= len(arena.guards) {
			return false
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		node := arena.guards[current]
		if node.value != 0 && !valueFramesOwnedBits(arena, node.value, frames) {
			return false
		}
		stack = append(stack, node.args...)
	}
	return true
}

func effectFramesOwnedBits(effects *EffectArena, term EffectTerm, frames []uint64) bool {
	if effects == nil || effects.terms == nil || term == 0 || int(term) >= len(effects.nodes) {
		return false
	}
	node := effects.nodes[term]
	values := [5]ValueTerm{node.key, node.value}
	if node.pathStoreHasAssignment {
		values[2] = node.pathStoreAssignment.Value
	}
	if node.pathStoreHasStatic {
		values[3] = node.pathStoreStatic.Value
	}
	if node.invalidation.Precise != nil {
		values[4] = node.invalidation.Precise.Key
	}
	for _, value := range values {
		if value != 0 && !valueFramesOwnedBits(effects.terms, value, frames) {
			return false
		}
	}
	return true
}

func validCallFrameBits(arena *Arena, term callFrameTerm, caller Shape, available []uint64) bool {
	if arena == nil || term == 0 || int(term) >= len(arena.callFrames) {
		return false
	}
	node := arena.callFrames[term]
	if (node.target == (CellRef{})) == (node.variable == 0) || len(node.values) != node.shape.InputCount() || len(node.paths) != len(node.values) {
		return false
	}
	valueSeen := make(map[ValueTerm]bool)
	for index, value := range node.values {
		if !arena.validValue(value, caller, valueSeen) || !valueFramesOwnedBits(arena, value, available) || node.paths[index] != 0 && !arena.validPath(node.paths[index], caller) {
			return false
		}
	}
	if node.closureProducer != 0 && !frameBitSet(available, node.closureProducer) {
		return false
	}
	return true
}

func (c *relationCode) validOutcome(outcome boundaryOutcomeTuple, owned []uint64) bool {
	if !outcome.preserved.valid(c.shape.Params+c.shape.Captures) || outcome.preserved.boundaryParams != c.shape.Params {
		return false
	}
	for _, operation := range outcome.operations {
		if operation.Kind >= outputKindCount || operation.Value == 0 || c.descriptors.handlers[operation.Descriptor] == nil ||
			!c.terms.validValue(operation.Value, c.shape, make(map[ValueTerm]bool)) || !valueFramesOwnedBits(c.terms, operation.Value, owned) {
			return false
		}
	}
	for _, proof := range outcome.proofs {
		if !proof.valid(c.terms, c.shape) || proof.Key != 0 && !valueFramesOwnedBits(c.terms, proof.Key, owned) {
			return false
		}
	}
	for _, refinement := range outcome.refinements {
		if !refinement.validPreservedBoundaryRoot(c.terms, c.shape) || !valueFramesOwnedBits(c.terms, refinement.Value, owned) {
			return false
		}
	}
	for _, observation := range outcome.observations {
		if !observation.valid(c.terms, c.shape) || !guardFramesOwnedBits(c.terms, observation.Guard, owned) ||
			!valueFramesOwnedBits(c.terms, observation.Actual, owned) || observation.Expected != 0 && !valueFramesOwnedBits(c.terms, observation.Expected, owned) {
			return false
		}
	}
	for _, obligation := range outcome.observationObligations {
		if !obligation.valid(c.terms, c.shape) || !guardFramesOwnedBits(c.terms, obligation.Guard, owned) {
			return false
		}
	}
	for _, term := range outcome.branchLiteralCases {
		if term.literalCase.TargetPathRef().IsEmpty() || !product.BelongsToRegistry(c.terms.reg, term.literalCase.LiteralValue()) {
			return false
		}
	}
	if outcome.resultPublication.Len() != 0 && (!outcome.resultPublication.Valid(c.terms.reg) || !outcome.resultPublication.HasPublicationSteps()) {
		return false
	}
	if outcome.covariant.Len() != 0 && (!outcome.covariant.Valid(c.terms.reg) || !outcome.covariant.HasStateSteps()) {
		return false
	}
	if !validBoundaryParamObligations(c.terms.reg, c.shape.Params, outcome.paramObligations) ||
		!validBoundaryPathObligations(c.terms, c.shape, outcome.pathObligations) ||
		!validBoundaryParamExposures(c.terms, c.shape, outcome.paramExposures) {
		return false
	}
	return true
}

func (c *relationCode) validContribution(contribution semanticContribution, owned []uint64) bool {
	return c.validOutcome(boundaryOutcomeTuple{
		suspensionKnown: contribution.suspensionKnown, maySuspend: contribution.maySuspend,
		protectedCallTypestate: contribution.protectedCallTypestate, operations: contribution.operations,
		proofs: contribution.proofs, refinements: contribution.refinements, observations: contribution.observations,
		observationObligations: contribution.observationObligations, preserved: contribution.preserved,
		returnConditions:   contribution.returnConditions,
		branchLiteralCases: contribution.branchLiteralCases,
		resultPublication:  contribution.resultPublication,
		covariant:          contribution.covariant,
		paramObligations:   contribution.paramObligations,
		pathObligations:    contribution.pathObligations,
		paramExposures:     contribution.paramExposures,
		returnTransaction:  contribution.returnTransaction,
	}, owned)
}

func (c *relationCode) valid(root relationRootRef) bool {
	return c != nil && c.sealed && root != 0 && root == c.root && c.terms != nil && c.effects != nil && c.effects.terms == c.terms &&
		c.descriptors != nil && c.terms.Sealed() && c.effects.Sealed() && int(root) < len(c.nodes)
}
