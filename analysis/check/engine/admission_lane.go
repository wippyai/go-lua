package engine

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// admissionLane is one reason a lexical body is decided at allocation time.
// The slice order is semantic: the first lane that admits owns both the child
// entry and the obligations that entry may discharge.
type admissionLane struct {
	Name       string
	Precedence int
	Admit      func(*admissionLane, *admissionLaneContext) (admissionLaneDecision, bool)
	Discharges DiagnosticFamilySet
}

type admissionLaneContext struct {
	lexical   *lexicalEvaluator
	body      equation.BodyID
	operation string
	result    string
	child     front.Compilation
	partition equation.Partition
	bodyIndex admissionBodyIndex
}

type admissionRootPolicy func(admissionLaneContext, admissionLaneDecision) ([]byte, bool, error)

type admissionLaneDecision struct {
	Lane                 *admissionLane
	Seeds                []entrySeed
	Root                 admissionRootPolicy
	Declared             declaredBoundaryAdmission
	DeclaredAssignment   bool
	GradualLogicalTerms  []string
	ContextualParameters []typ.Type
	FormalMemberWrites   map[string]bool
	SealedEnvironment    bool
	ClosedAnyCapture     bool
}

type admissionDiagnosticContext struct {
	lexical         *lexicalEvaluator
	child           front.Compilation
	partition       equation.Partition
	bodyIndex       admissionBodyIndex
	decision        admissionLaneDecision
	diagnostic      equation.Fact
	qualifiedFamily string
}

// admissionDiagnosticDischarged first asks the registry-owned family set, then
// applies the operation-level proof restriction for lanes whose entry decides
// only selected members of that family.
func admissionDiagnosticDischarged(ctx admissionDiagnosticContext, closedAnyFormal bool) bool {
	family := ctx.qualifiedFamily
	if closedAnyFormal && closedAnyFormalObligation(family) {
		return true
	}
	if ctx.decision.Lane == nil || !ctx.decision.Lane.Discharges.ContainsKey(family) {
		return false
	}
	diagnostic := ctx.diagnostic
	switch ctx.decision.Lane.Name {
	case "static-assignment":
		return ctx.lexical.uncalledStaticAssignmentDiagnostic(ctx.child.Artifact, diagnostic.Key, ctx.partition) ||
			ctx.staticOptionalMethodDiagnostic() ||
			ctx.staticResultCallDiagnostic()
	case "typed-channel-send":
		return ctx.typedChannelSendDiagnostic()
	case "declared":
		switch {
		case diagnosticFamilyMatches(family, DiagnosticFamilyMissingMember):
			return true
		case diagnosticFamilyMatches(family, DiagnosticFamilyReturnContract):
			return ctx.decision.Declared.Method || ctx.decision.Declared.ArithmeticReturn
		case diagnosticFamilyMatches(family, DiagnosticFamilyAssignment):
			return ctx.decision.DeclaredAssignment || ctx.declaredProviderResultDiagnostic()
		case diagnosticFamilyMatches(family, DiagnosticFamilyOptionalAssignmentTarget):
			return ctx.decision.Declared.MemberWrite
		case diagnosticFamilyMatches(family, DiagnosticFamilyConcatOperand):
			return declaredConcatDiagnostic(ctx.decision.Declared.Concat, diagnostic.Key)
		case diagnosticFamilyMatches(family, DiagnosticFamilyComparisonOperand):
			return declaredOrderedComparisonDiagnostic(ctx.decision.Declared.Comparison, diagnostic.Key)
		case diagnosticFamilyMatches(family, DiagnosticFamilyUnprovenClaim):
			return declaredAssertionDiagnostic(ctx.decision.Declared.Assertions, diagnostic.Key)
		case diagnosticFamilyMatches(family, DiagnosticFamilyCallArgumentType):
			return ctx.declaredProviderResultDiagnostic()
		default:
			return false
		}
	case "explicit-any":
		return ctx.explicitAnyDiagnostic()
	case "declared-formal-call":
		if ctx.decision.SealedEnvironment {
			return formalMemberWriteDiagnostic(ctx.decision.FormalMemberWrites, family)
		}
		return declaredFormalCallDiagnostic(family)
	}
	return true
}

var admissionLanes = []admissionLane{
	{
		Name:       "gradual-logical-call",
		Precedence: 0,
		Admit:      (*admissionLane).admitGradualLogicalCall,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyCallArgumentType),
	},
	{
		Name:       "declared-local-union-read",
		Precedence: 1,
		Admit:      (*admissionLane).admitDeclaredLocalUnionRead,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyMissingMember),
	},
	{
		Name:       "declared-indexed-read",
		Precedence: 2,
		Admit:      (*admissionLane).admitDeclaredIndexedRead,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyReturnContract),
	},
	{
		Name:       "static-assignment",
		Precedence: 3,
		Admit:      (*admissionLane).admitStaticAssignment,
		Discharges: NewDiagnosticFamilySet(
			DiagnosticFamilyAssignment,
			DiagnosticFamilyOptionalCallReceiver,
			DiagnosticFamilyCallNotCallable,
			DiagnosticFamilyCallArgumentType,
		),
	},
	{
		Name:       "typed-channel-send",
		Precedence: 4,
		Admit:      (*admissionLane).admitTypedChannelSend,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyCallArgumentType),
	},
	{
		Name:       "declared",
		Precedence: 5,
		Admit:      (*admissionLane).admitDeclared,
		Discharges: NewDiagnosticFamilySet(
			DiagnosticFamilyMissingMember,
			DiagnosticFamilyReturnContract,
			DiagnosticFamilyAssignment,
			DiagnosticFamilyOptionalAssignmentTarget,
			DiagnosticFamilyConcatOperand,
			DiagnosticFamilyComparisonOperand,
			DiagnosticFamilyUnprovenClaim,
			DiagnosticFamilyCallArgumentType,
		),
	},
	{
		Name:       "explicit-any",
		Precedence: 6,
		Admit:      (*admissionLane).admitExplicitAny,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyUnprovenClaim),
	},
	{
		Name:       "static-captured-return",
		Precedence: 7,
		Admit:      (*admissionLane).admitStaticCapturedReturn,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyReturnContract, DiagnosticFamilyCallTooFewArguments, DiagnosticFamilyCallTooManyArguments),
	},
	{
		Name:       "static-arithmetic",
		Precedence: 8,
		Admit:      (*admissionLane).admitStaticArithmetic,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyCallArgumentType),
	},
	{
		Name:       "static-member-read",
		Precedence: 9,
		Admit:      (*admissionLane).admitStaticMemberRead,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyMissingMember),
	},
	{
		Name:       "declared-formal-call",
		Precedence: 10,
		Admit:      (*admissionLane).admitDeclaredFormalCall,
		Discharges: NewDiagnosticFamilySet(
			DiagnosticFamilyMissingMember,
			DiagnosticFamilyAssignment,
			DiagnosticFamilyOptionalAssignmentTarget,
			DiagnosticFamilyReturnContract,
			DiagnosticFamilyOptionalCallReceiver,
			DiagnosticFamilyCallArgumentType,
			DiagnosticFamilyCallNotCallable,
			DiagnosticFamilyCallTooFewArguments,
			DiagnosticFamilyCallTooManyArguments,
		),
	},
	{
		Name:       "imported-capture",
		Precedence: 11,
		Admit:      (*admissionLane).admitImportedCapture,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyMissingMember),
	},
	{
		Name:       "sealed-capture",
		Precedence: 12,
		Admit:      (*admissionLane).admitSealedCapture,
		Discharges: RegisteredDiagnosticFamilies(),
	},
	{
		Name:       "contextual-callback",
		Precedence: 13,
		Admit:      (*admissionLane).admitContextualCallback,
		Discharges: RegisteredDiagnosticFamilies(),
	},
}

func selectAdmissionLane(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	// Every descriptor is a query over the one body projection built by the
	// allocation consumer. A missing projection is an unindexed admission
	// request, so it cannot prove any allocation-time boundary.
	if ctx == nil || !ctx.bodyIndex.ready {
		return admissionLaneDecision{}, false
	}
	for index := range admissionLanes {
		lane := &admissionLanes[index]
		decision, admitted := lane.Admit(lane, ctx)
		if !admitted {
			continue
		}
		decision.Lane = lane
		return decision, true
	}
	return admissionLaneDecision{}, false
}

func (lane *admissionLane) admitTypedChannelSend(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	if !ctx.bodyIndex.typedChannelSendBoundary() {
		return admissionLaneDecision{}, false
	}
	// The old orchestration computed this independent obligation lane after
	// declaration admission. When both hold, the declaration still owns the
	// seed/root policy while channel send narrows the discharged surface.
	if declared, admitted := lane.admitDeclared(ctx); admitted {
		return declared, true
	}
	return admissionLaneDecision{Root: admissionRootClosedBody}, true
}

func (lane *admissionLane) admitExplicitAny(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.bodyIndex.explicitAnyBoundary()
	if !admitted {
		return admissionLaneDecision{}, false
	}
	closedCapture := ctx.closedAnyCaptureBoundary()
	root := admissionRootCaptured
	if len(ctx.child.Boundary.Captures) == 0 {
		root = admissionRootClosedBody
	} else if closedCapture {
		root = admissionRootClosedAnyCapture
	}
	return admissionLaneDecision{Seeds: seeds, Root: root, ClosedAnyCapture: closedCapture}, true
}

func (lane *admissionLane) admitGradualLogicalCall(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, terms, admitted := ctx.bodyIndex.gradualLogicalCallBoundary()
	if !admitted {
		return admissionLaneDecision{}, false
	}
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootGradualLogical, GradualLogicalTerms: terms}, true
}

func (lane *admissionLane) admitDeclaredLocalUnionRead(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.bodyIndex.declaredLocalUnionReadBoundary()
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootLocalUnionRead}, admitted
}

func (lane *admissionLane) admitDeclaredIndexedRead(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.bodyIndex.declaredIndexedReadBoundary()
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootClosedBody}, admitted
}

func (lane *admissionLane) admitStaticAssignment(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.bodyIndex.staticAssignmentBoundary()
	if !admitted || !ctx.lexical.uncalledStaticCapturedCallsAreGuardedValidation(ctx.child, ctx.partition) {
		return admissionLaneDecision{}, false
	}
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootStaticAssignment}, true
}

func (lane *admissionLane) admitDeclared(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	admission := ctx.bodyIndex.declaredBoundary()
	if !admission.Admitted {
		return admissionLaneDecision{}, false
	}
	assignment := admission.ArithmeticAssignment
	formals := ctx.bodyIndex.formals
	allocations := bodyLocalTableTerms(ctx.child)
	allocationReads := bodyLocalAllocationReadTargets(ctx.child, allocations)
	derivedCells := ctx.bodyIndex.declaredFormalDerivedCells(formals)
	for _, operation := range ctx.bodyIndex.operations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		assignment = assignment ||
			ctx.bodyIndex.declaredFormalAssignment(operation, formals, derivedCells) ||
			ctx.bodyIndex.declaredLocalAllocationAssignment(operation, allocations, allocationReads)
	}
	return admissionLaneDecision{
		Seeds:              admission.Seeds,
		Root:               admissionRootDeclared,
		Declared:           admission,
		DeclaredAssignment: assignment,
	}, true
}

func (lane *admissionLane) admitStaticCapturedReturn(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	if !ctx.lexical.uncalledStaticCapturedReturnBoundary(ctx.child, ctx.partition) {
		return admissionLaneDecision{}, false
	}
	return admissionLaneDecision{Root: admissionRootCaptured}, true
}

func (lane *admissionLane) admitStaticArithmetic(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	if !ctx.lexical.uncalledStaticArithmeticBoundary(ctx.body, ctx.child, ctx.partition) {
		return admissionLaneDecision{}, false
	}
	return admissionLaneDecision{Root: admissionRootArithmetic}, true
}

func (lane *admissionLane) admitStaticMemberRead(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.bodyIndex.staticMemberReadSeeds()
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootClosedBody}, admitted
}

func (lane *admissionLane) admitDeclaredFormalCall(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.lexical.uncalledDeclaredFormalCallBoundary(ctx.bodyIndex, lexicalAllocationSite{body: ctx.body, operation: ctx.operation}, ctx.partition)
	if !admitted {
		return admissionLaneDecision{}, false
	}
	if len(ctx.child.Boundary.Captures) == 0 {
		return admissionLaneDecision{Seeds: seeds, Root: admissionRootDeclared}, true
	}
	operations := ctx.lexical.formalMemberWriteObligations(ctx.child, ctx.partition)
	if len(operations) == 0 {
		return admissionLaneDecision{}, false
	}
	return admissionLaneDecision{
		Seeds:              seeds,
		Root:               admissionRootSealedEnvironment,
		FormalMemberWrites: operations,
		SealedEnvironment:  true,
	}, true
}

func (lane *admissionLane) admitImportedCapture(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, admitted := ctx.lexical.uncalledImportedCaptureBoundary(ctx.body, ctx.bodyIndex, ctx.partition)
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootImportedCapture}, admitted
}

func (lane *admissionLane) admitSealedCapture(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	if len(ctx.child.Boundary.Parameters) == 0 && len(ctx.child.Boundary.Captures) == 0 {
		return admissionLaneDecision{Root: admissionRootClosedBody}, true
	}
	admitted := ctx.sealedCaptureBoundary()
	return admissionLaneDecision{Root: admissionRootSealedCapture}, admitted
}

func (lane *admissionLane) admitContextualCallback(ctx *admissionLaneContext) (admissionLaneDecision, bool) {
	seeds, parameters, admitted := ctx.lexical.contextualCallbackBoundary(ctx.body, ctx.child, ctx.result, ctx.partition)
	return admissionLaneDecision{Seeds: seeds, Root: admissionRootClosedBody, ContextualParameters: parameters}, admitted
}

func admissionRootClosedBody(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	seeds, closureSeeds := ctx.lexical.closedBodyCalleeSeeds(ctx.child, decision.Seeds, ctx.partition)
	entry, err := encodeChildEntry(seeds, closureSeeds...)
	return entry, true, err
}

func admissionRootDeclared(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	entry, err := encodeDeclaredChildEntryWithCapabilities(decision.Seeds, nil, nil, declaredBoundaryIdentitySeeds(ctx.body, ctx.child, decision.Seeds, nil), nil)
	return entry, true, err
}

func admissionRootLocalUnionRead(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledLocalUnionReadEntry(ctx.child, decision.Seeds, ctx.partition)
}

func admissionRootImportedCapture(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, false, true, false, false)
}

func admissionRootClosedAnyCapture(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, true, false, false)
}

func admissionRootSealedEnvironment(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, false, true, true)
}

func admissionRootGradualLogical(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	if len(ctx.child.Boundary.Captures) == 0 {
		entry, err := encodeChildEntryWithCapabilities(decision.Seeds, nil, nil, nil, nil, decision.GradualLogicalTerms)
		return entry, true, err
	}
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, false, false, false, decision.GradualLogicalTerms)
}

func admissionRootStaticAssignment(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, false, false, true)
}

func admissionRootCaptured(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, false, false, false)
}

func admissionRootArithmetic(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, true, true, false)
}

func admissionRootSealedCapture(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.lexical.uncalledChildEntry(ctx.body, ctx.child, decision.Seeds, ctx.partition, true, false, true, false)
}
