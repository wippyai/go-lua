package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// admissionLane is one reason a lexical body is decided at allocation time.
// The slice order is semantic: the first lane that admits owns both the child
// entry and the obligations that entry may discharge.
type admissionLane struct {
	Name       string
	Admit      func(*admissionLane, *admissionLaneContext) admissionLaneResult
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
}

// admissionLaneResult is the only value an admission descriptor can return.
// The private lane seal makes a zero result a refusal and prevents a parallel
// bool (including one wrapped in another struct) from becoming admission.
type admissionLaneResult struct {
	decision admissionLaneDecision
	lane     *admissionLane
}

func (result admissionLaneResult) admitted() bool {
	return result.lane != nil && result.decision.Root != nil
}

func (lane *admissionLane) admission(decision admissionLaneDecision) admissionLaneResult {
	if lane == nil || decision.Root == nil {
		return admissionLaneResult{}
	}
	decision.Lane = lane
	return admissionLaneResult{decision: decision, lane: lane}
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
	if closedAnyFormal && diagnosticFamilyMatches(family, DiagnosticFamilyCallArgumentType, DiagnosticFamilyCallNotCallable, DiagnosticFamilyReturnContract, DiagnosticFamilyAssignment) {
		return true
	}
	if ctx.decision.Lane == nil || !ctx.decision.Lane.Discharges.ContainsKey(family) {
		return false
	}
	diagnostic := ctx.diagnostic
	switch ctx.decision.Lane.Name {
	case "static-assignment":
		return func() bool {
			var ctx admissionDiagnosticContext = ctx
			artifact, key, partition := ctx.child.DraftArtifact(), ctx.diagnostic.Key, ctx.partition
			prefix := diagnosticFamilyPrefix(DiagnosticFamilyAssignment)
			if !strings.HasPrefix(key, prefix) {
				return false
			}
			operation := strings.TrimPrefix(key, prefix)
			var source string
			for _, candidate := range artifact.Equations {
				if candidate.Target.Name != operation || candidate.Occurrence.Kind != "claim" {
					continue
				}
				value, found := artifactOperand(candidate.Operands, equation.MustOperandRole("value"))
				if !found {
					return false
				}
				source = string(value)
				break
			}
			if source == "" {
				return false
			}
			for _, candidate := range artifact.Equations {
				if candidate.Occurrence.Kind != "apply" {
					continue
				}
				arity, hasArity := artifactOperand(candidate.Operands, equation.MustOperandRole("result-arity"))
				if !hasArity || string(arity) != "0" {
					continue
				}
				for _, operand := range candidate.Operands {
					if _, argument := operand.Role.Index(equation.RoleFamilyArgument); argument && string(operand.Term.Encoding) == source {
						callee, hasCallee := artifactOperand(candidate.Operands, equation.MustOperandRole("callee"))
						if hasCallee && (ctx.bodyIndex.capturedHelperHasOnlyGuardedValidation(ctx.lexical, callee, partition) || ctx.bodyIndex.capturedHelperHasOnlyGuardedNonValidationEffect(ctx.lexical, callee, partition)) {
							continue
						}
						return false
					}
				}
			}
			return true
		}() ||
			func() bool {
				diagnostic := ctx.diagnostic
				if !diagnosticFamilyMatches(diagnostic.Key, DiagnosticFamilyCallNotCallable, DiagnosticFamilyOptionalCallReceiver) {
					return false
				}
				name := diagnosticOperationName(diagnostic.Key)
				for _, operation := range ctx.bodyIndex.operations {
					if operation.Occurrence.Kind != "apply" {
						continue
					}
					if operation.Target.Name != name || operation.Occurrence.Kind != "apply" || len(operation.Guards) != 0 {
						continue
					}
					receiver, hasReceiver := artifactOperand(operation.Operands, equation.MustOperandRole("receiver"))
					method, hasMethod := artifactOperand(operation.Operands, equation.MustOperandRole("method"))
					return hasReceiver && hasMethod && strings.HasPrefix(string(receiver), "path/") && strings.HasPrefix(string(method), "method/")
				}
				return false
			}() ||
			func() bool {
				var ctx admissionDiagnosticContext = ctx
				key := ctx.diagnostic.Key
				if !diagnosticFamilyMatches(key, DiagnosticFamilyCallArgumentType) {
					return false
				}
				operationName := diagnosticOperationName(key)
				published := ctx.bodyIndex.publishedStdlibCalls()
				resultPaths := ctx.bodyIndex.publishedResultPaths(published)
				for _, operation := range ctx.bodyIndex.operations {
					if operation.Occurrence.Kind != "apply" {
						continue
					}
					if operation.Target.Name != operationName || operation.Occurrence.Kind != "apply" {
						continue
					}
					for _, operand := range operation.Operands {
						if _, argument := operand.Role.Index(equation.RoleFamilyArgument); argument && resultPaths[string(operand.Term.Encoding)] {
							return true
						}
					}
				}
				return false
			}()
	case "typed-channel-send":
		return diagnosticFamilyMatches(ctx.diagnostic.Key, DiagnosticFamilyCallArgumentType)
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
			_, operation, subject, ok := concatOperandDiagnosticParts(diagnostic.Key)
			return ok && ctx.decision.Declared.Concat[operation+"/"+subject]
		case diagnosticFamilyMatches(family, DiagnosticFamilyComparisonOperand):
			key := diagnostic.Key
			name, ok := strings.CutPrefix(key, diagnosticFamilyPrefix(DiagnosticFamilyComparisonOperand))
			return ok && ctx.decision.Declared.Comparison[name]
		case diagnosticFamilyMatches(family, DiagnosticFamilyUnprovenClaim):
			key := diagnostic.Key
			name, unproven := strings.CutPrefix(key, diagnosticFamilyPrefix(DiagnosticFamilyUnprovenClaim))
			return unproven && ctx.decision.Declared.Assertions[name]
		case diagnosticFamilyMatches(family, DiagnosticFamilyCallArgumentType):
			return ctx.declaredProviderResultDiagnostic()
		default:
			return false
		}
	case "explicit-any":
		diagnostic := ctx.diagnostic
		if diagnosticFamilyMatches(diagnostic.Key, DiagnosticFamilyAssignment) {
			return true
		}
		name, unproven := diagnosticFamilyTail(DiagnosticFamilyUnprovenClaim, diagnostic.Key)
		if !unproven || typePredicateResultClaim(ctx.child.DraftArtifact(), name) {
			return false
		}
		for _, operation := range ctx.bodyIndex.operations {
			if operation.Occurrence.Kind != "claim" {
				continue
			}
			if operation.Target.Name != name || operation.Occurrence.Kind != "claim" {
				continue
			}
			for _, operand := range operation.Operands {
				if operand.Role.Wire() == "kind" {
					return string(operand.Term.Encoding) == "claim-kind/3"
				}
			}
		}
		return false
	case "declared-formal-call":
		if ctx.decision.SealedEnvironment {
			if !declaredFormalCallDiagnostic(family) {
				return false
			}
			operation, found := diagnosticOperation(family)
			return found && ctx.decision.FormalMemberWrites[operation]
		}
		return declaredFormalCallDiagnostic(family)
	}
	return true
}

var admissionLanes = []admissionLane{
	{
		Name:       "gradual-logical-call",
		Admit:      (*admissionLane).admitGradualLogicalCall,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyCallArgumentType),
	},
	{
		Name:       "declared-local-union-read",
		Admit:      (*admissionLane).admitDeclaredLocalUnionRead,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyMissingMember),
	},
	{
		Name:       "declared-indexed-read",
		Admit:      (*admissionLane).admitDeclaredIndexedRead,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyReturnContract),
	},
	{
		Name:  "static-assignment",
		Admit: (*admissionLane).admitStaticAssignment,
		Discharges: NewDiagnosticFamilySet(
			DiagnosticFamilyAssignment,
			DiagnosticFamilyOptionalCallReceiver,
			DiagnosticFamilyCallNotCallable,
			DiagnosticFamilyCallArgumentType,
		),
	},
	{
		Name:       "typed-channel-send",
		Admit:      (*admissionLane).admitTypedChannelSend,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyCallArgumentType),
	},
	{
		Name:  "declared",
		Admit: (*admissionLane).admitDeclared,
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
		Admit:      (*admissionLane).admitExplicitAny,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyUnprovenClaim),
	},
	{
		Name:       "static-captured-return",
		Admit:      (*admissionLane).admitStaticCapturedReturn,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyReturnContract, DiagnosticFamilyCallTooFewArguments, DiagnosticFamilyCallTooManyArguments),
	},
	{
		Name:       "static-arithmetic",
		Admit:      (*admissionLane).admitStaticArithmetic,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyCallArgumentType),
	},
	{
		Name:       "static-member-read",
		Admit:      (*admissionLane).admitStaticMemberRead,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyMissingMember),
	},
	{
		Name:  "declared-formal-call",
		Admit: (*admissionLane).admitDeclaredFormalCall,
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
		Admit:      (*admissionLane).admitImportedCapture,
		Discharges: NewDiagnosticFamilySet(DiagnosticFamilyAssignment, DiagnosticFamilyMissingMember),
	},
	{
		Name:       "sealed-capture",
		Admit:      (*admissionLane).admitSealedCapture,
		Discharges: RegisteredDiagnosticFamilies(),
	},
	{
		Name:       "contextual-callback",
		Admit:      (*admissionLane).admitContextualCallback,
		Discharges: RegisteredDiagnosticFamilies(),
	},
}

func selectAdmissionLane(ctx *admissionLaneContext) admissionLaneResult {
	// Every descriptor is a query over the one body projection built by the
	// allocation consumer. A missing projection is an unindexed admission
	// request, so it cannot prove any allocation-time boundary.
	if ctx == nil || !ctx.bodyIndex.ready {
		return admissionLaneResult{}
	}
	for index := range admissionLanes {
		lane := &admissionLanes[index]
		result := lane.Admit(lane, ctx)
		if !result.admitted() || result.lane != lane {
			continue
		}
		return result
	}
	return admissionLaneResult{}
}

func (lane *admissionLane) admitTypedChannelSend(ctx *admissionLaneContext) admissionLaneResult {
	if !func() bool {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return false
		}
		child := index.child
		if child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Captures) != 0 {
			return false
		}
		channels := make(map[string]bool, len(child.BodyBoundary().Parameters))
		for _, parameter := range child.BodyBoundary().Parameters {
			if parameter.Vararg || parameter.Type == 0 {
				continue
			}
			if _, ok := ambient.ChannelPayloadType(child.LoweredBody().Type(parameter.Type)); ok {
				channels[boundaryTerm(parameter.Symbol)] = true
			}
		}
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			var receiver, method string
			for _, operand := range operation.Operands {
				switch operand.Role.Wire() {
				case "receiver":
					receiver = string(operand.Term.Encoding)
				case "method":
					method, _ = callMethodName(operand.Term.Encoding)
				}
			}
			if method == "send" && channels[receiver] {
				return true
			}
		}
		return false
	}() {
		return admissionLaneResult{}
	}
	// The old orchestration computed this independent obligation lane after
	// declaration admission. When both hold, the declaration still owns the
	// seed/root policy while channel send narrows the discharged surface.
	if declared := lane.admitDeclared(ctx); declared.admitted() {
		return declared
	}
	return lane.admission(admissionLaneDecision{Root: admissionRootClosedBody})
}

func (lane *admissionLane) admitExplicitAny(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := ctx.bodyIndex.explicitAnyBoundary()
	if !admitted {
		return admissionLaneResult{}
	}
	closedCapture := func() bool {
		var ctx admissionLaneContext = *ctx
		if ctx.lexical == nil || ctx.child.LoweredBody() == nil || len(ctx.bodyIndex.captures) == 0 {
			return false
		}
		if _, closedAny := ctx.bodyIndex.explicitAnyBoundary(); !closedAny {
			return false
		}
		return ctx.lexical.sealedCaptureEnvironment(ctx.child, lexicalAllocationSite{body: ctx.body, operation: ctx.operation}, ctx.partition, true)
	}()
	root := admissionRootCaptured
	if len(ctx.child.BodyBoundary().Captures) == 0 {
		root = admissionRootClosedBody
	} else if closedCapture {
		root = admissionRootClosedAnyCapture
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: root})
}

func (lane *admissionLane) admitGradualLogicalCall(ctx *admissionLaneContext) admissionLaneResult {
	seeds, terms, admitted := func() ([]entrySeed, []string, bool) {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return nil, nil, false
		}
		child := index.child
		if child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Captures) == 0 || len(child.BodyBoundary().Parameters) != 1 {
			return nil, nil, false
		}
		seeds := make([]entrySeed, 0, len(child.BodyBoundary().Parameters))
		terms := make([]string, 0, len(child.BodyBoundary().Parameters))
		for position, parameter := range child.BodyBoundary().Parameters {
			if parameter.Vararg || parameter.Type != 0 {
				return nil, nil, false
			}
			term := index.formals[position]
			seeds = append(seeds, entrySeed{Term: term, Value: []byte(shapefact.ScalarTopWire)})
			terms = append(terms, term)
		}
		tainted := make(map[string]bool, len(index.formals))
		for _, formal := range index.formals {
			tainted[formal] = true
		}
		logical := false
		changed := true
		for changed {
			changed = false
			for _, operation := range index.operations {
				switch operation.Occurrence.Kind {
				case "expression":
					kind, found := artifactOperand(operation.Operands, equation.MustOperandRole("kind"))
					if !found || string(kind) != strconv.Itoa(int(wir.OpLogical)) {
						continue
					}
					left, hasLeft := artifactOperand(operation.Operands, equation.MustOperandRole("left"))
					right, hasRight := artifactOperand(operation.Operands, equation.MustOperandRole("right"))
					result, hasResult := artifactOperand(operation.Operands, equation.MustOperandRole("result"))
					if !hasLeft || !hasRight || !hasResult || (!tainted[string(left)] && !tainted[string(right)]) {
						continue
					}
					logical = true
					if !tainted[string(result)] {
						tainted[string(result)] = true
						changed = true
					}
				case "environment-write":
					value, hasValue := artifactOperand(operation.Operands, equation.MustOperandRole("value"))
					target, hasTarget := artifactOperand(operation.Operands, equation.MustOperandRole("target"))
					if hasValue && hasTarget && tainted[string(value)] && !tainted[string(target)] {
						tainted[string(target)] = true
						changed = true
					}
				}
			}
		}
		if !logical {
			return nil, nil, false
		}
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			for _, operand := range operation.Operands {
				if _, argument := operand.Role.Index(equation.RoleFamilyArgument); argument && tainted[string(operand.Term.Encoding)] {
					return seeds, terms, true
				}
			}
		}
		return nil, nil, false
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootGradualLogical, GradualLogicalTerms: terms})
}

func (lane *admissionLane) admitDeclaredLocalUnionRead(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := func() ([]entrySeed, bool) {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return nil, false
		}
		child := index.child
		if child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Parameters) == 0 {
			return nil, false
		}
		if !index.declaredSeeded {
			return nil, false
		}
		formals := index.formals
		for _, parameter := range child.BodyBoundary().Parameters {
			if parameter.Symbol == 0 {
				return nil, false
			}
		}
		applications := make(map[string]bool)
		derived := index.declaredLocalUnionExpressionTerms(formals)
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			callee, hasCallee := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
			arity, hasArity := artifactOperand(operation.Operands, equation.MustOperandRole("result-arity"))
			if !hasCallee || !strings.HasPrefix(string(callee), "path/") || !hasArity || string(arity) != "1" {
				return nil, false
			}
			for _, operand := range operation.Operands {
				if _, argument := operand.Role.FixedIndex(equation.RoleFamilyArgument, 8); argument {
					if !formals.has(string(operand.Term.Encoding)) && !derived[string(operand.Term.Encoding)] {
						return nil, false
					}
				}
			}
			applications["call/"+operation.Target.Name] = true
		}
		if len(applications) == 0 {
			return nil, false
		}
		results := make(map[string]bool)
		for _, operation := range index.operations {
			if operation.Occurrence.Kind != "call-results" {
				continue
			}
			application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
			if !found || !applications[string(application)] {
				return nil, false
			}
			result, found := artifactOperand(operation.Operands, equation.IndexedRole(equation.RoleFamilyResult, 0))
			if !found {
				return nil, false
			}
			results[string(result)] = true
		}
		paths := make(map[string]bool)
		reads := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "environment-write" {
				continue
			}
			value, hasValue := artifactOperand(operation.Operands, equation.MustOperandRole("value"))
			target, hasTarget := artifactOperand(operation.Operands, equation.MustOperandRole("target"))
			if hasValue && hasTarget && results[string(value)] {
				paths[string(target)] = true
				continue
			}
			if hasValue && hasTarget && strings.HasPrefix(string(value), "path/") {
				for path := range paths {
					if strings.HasPrefix(string(value), path+".") {
						reads[string(value)] = true
						reads[string(target)] = true
					}
				}
			}
		}
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "entry", "apply", "call-results", "environment-write", "claim", "expression", "publication":
				continue
			case "branch-relations":
				if !index.declaredLocalUnionBranch(operation, paths, formals, derived) {
					return nil, false
				}
				continue
			case "external-call":
				application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
				if !found || !applications[string(application)] {
					return nil, false
				}
				continue
			case "dynamic-index-read":
				container, hasContainer := artifactOperand(operation.Operands, equation.MustOperandRole("container"))
				key, hasKey := artifactOperand(operation.Operands, equation.MustOperandRole("key"))
				target, hasTarget := artifactOperand(operation.Operands, equation.MustOperandRole("target"))
				if !hasContainer || !paths[string(container)] || !hasKey || !shapefact.IsScalarKind(key, shapefact.ScalarString) || !hasTarget {
					return nil, false
				}
				reads[string(target)] = true
			default:
				return nil, false
			}
		}
		if len(reads) == 0 {
			return nil, false
		}
		for _, operation := range index.operations {
			if operation.Occurrence.Kind != "claim" {
				continue
			}
			value, found := artifactOperand(operation.Operands, equation.MustOperandRole("value"))
			if found && reads[string(value)] {
				return index.declaredSeeds()
			}
		}
		return nil, false
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootLocalUnionRead})
}

func (lane *admissionLane) admitDeclaredIndexedRead(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := func() ([]entrySeed, bool) {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return nil, false
		}
		child := index.child
		if !capturelessFormalBody(child) {
			return nil, false
		}
		if !index.declaredSeeded {
			return nil, false
		}
		formals := index.formals
		for _, parameter := range child.BodyBoundary().Parameters {
			if parameter.Symbol == 0 {
				return nil, false
			}
		}
		hasIndexedRead := false
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "dynamic-index-read":
				container, found := artifactOperand(operation.Operands, equation.MustOperandRole("container"))
				if !found || !formals.has(string(container)) {
					return nil, false
				}
				hasIndexedRead = true
			case "branch-relations":
				if !index.declaredIndexedBranch(operation) {
					return nil, false
				}
			case "apply", "external-call", "channel-select", "generic-for":
				return nil, false
			}
		}
		if !hasIndexedRead {
			return nil, false
		}
		return index.declaredSeeds()
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootClosedBody})
}

func (lane *admissionLane) admitStaticAssignment(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := func() ([]entrySeed, bool) {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return nil, false
		}
		child := index.child
		if child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Parameters) == 0 || len(child.BodyBoundary().Captures) == 0 {
			return nil, false
		}
		captures := index.captures
		publishedStdlibCalls := index.publishedStdlibCalls()
		capturedCalls := make(map[string]bool)
		callResults := make(map[string]bool)
		callPaths := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "apply":
				callee, arity := "", ""
				for _, operand := range operation.Operands {
					if operand.Role.Wire() == "callee" {
						callee = string(operand.Term.Encoding)
					}
					if operand.Role.Wire() == "result-arity" {
						arity = string(operand.Term.Encoding)
					}
				}
				if captures.has(callee) && arity != "" && arity != "0" {
					capturedCalls["call/"+operation.Target.Name] = true
				}
			case "call-results":
				application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
				if !found || !capturedCalls[string(application)] {
					continue
				}
				for _, operand := range operation.Operands {
					if _, semantic := operand.Role.SemanticResult(); semantic {
						callResults[string(operand.Term.Encoding)] = true
					}
				}
			case "environment-write":
				value, hasValue := artifactOperand(operation.Operands, equation.MustOperandRole("value"))
				target, hasTarget := artifactOperand(operation.Operands, equation.MustOperandRole("target"))
				if hasValue && hasTarget && callResults[string(value)] {
					callPaths[string(target)] = true
				}
			}
		}
		hasAssignment, hasResultCall, hasOptionalMethod := false, false, false
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "claim":
				for _, operand := range operation.Operands {
					if operand.Role.Wire() == "kind" && string(operand.Term.Encoding) == "claim-kind/3" {
						hasAssignment = true
					}
				}
			case "apply":
				callee, receiver := "", ""
				resultArity, method := "", ""
				for _, operand := range operation.Operands {
					if operand.Role.Wire() == "callee" {
						callee = string(operand.Term.Encoding)
					}
					if operand.Role.Wire() == "receiver" {
						receiver = string(operand.Term.Encoding)
					}
					if operand.Role.Wire() == "method" {
						method = string(operand.Term.Encoding)
					}
					if operand.Role.Wire() == "result-arity" {
						resultArity = string(operand.Term.Encoding)
					}
				}
				if resultArity == "" {
					return nil, false
				}
				if method != "" && receiver != "" && callPaths[receiver] && resultArity == "0" {
					hasOptionalMethod = true
					continue
				}
				if resultArity == "0" {
					if captures.has(callee) {
						continue
					}
					if !index.staticCapturedMemberCall(callee, captures) {
						return nil, false
					}
					continue
				}
				if !captures.has(callee) && !publishedStdlibCalls["call/"+operation.Target.Name] {
					return nil, false
				}
				hasResultCall = true
			case "branch-relations":
				if !index.declaredFormalBranch(operation) {
					return nil, false
				}
			case "dynamic-index-read", "channel-select":
				return nil, false
			}
		}
		if (!hasAssignment && !hasOptionalMethod && !hasResultCall) || (!hasResultCall && !index.hasCapturedNoResultCall(captures)) {
			return nil, false
		}
		return index.declaredSeeds()
	}()
	if !admitted || !func() bool {
		var (
			index     *admissionBodyIndex = &ctx.bodyIndex
			l         *lexicalEvaluator   = ctx.lexical
			partition equation.Partition  = ctx.partition
		)
		child := index.child
		captures := make(map[string]bool, len(child.BodyBoundary().Captures))
		for _, capture := range child.BodyBoundary().Captures {
			captures[boundaryTerm(capture.Symbol)] = true
		}
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			callee, hasCallee := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
			arity, hasArity := artifactOperand(operation.Operands, equation.MustOperandRole("result-arity"))
			if hasCallee && hasArity && string(arity) == "0" && captures[string(callee)] && !index.capturedHelperHasOnlyGuardedValidation(l, callee, partition) && !index.capturedHelperHasOnlyGuardedNonValidationEffect(l, callee, partition) {
				return false
			}
		}
		return true
	}() {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootStaticAssignment})
}

func (lane *admissionLane) admitDeclared(ctx *admissionLaneContext) admissionLaneResult {
	admission := func() declaredBoundaryAdmission {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return declaredBoundaryAdmission{}
		}
		child := index.child
		if !capturelessFormalBody(child) {
			return declaredBoundaryAdmission{}
		}
		formals := index.formals
		if !index.declaredSeeded {
			return declaredBoundaryAdmission{}
		}
		arithmeticTerms := index.declaredFormalArithmeticTerms(formals)
		derivedCells := index.declaredFormalDerivedCells(formals)
		memberCalls := make(map[string]bool)
		hasDirectMethod := false
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			if index.declaredMemberCall(operation, formals) {
				memberCalls["call/"+operation.Target.Name] = true
				hasDirectMethod = hasDirectMethod || hasDeclaredFormalMethodCall(child, operation, formals)
				continue
			}
			if index.declaredStdlibCall(operation, formals) || index.declaredExpandedStdlibCall(operation, formals) {
				memberCalls["call/"+operation.Target.Name] = true
			}
		}
		hasBranch, hasDeclaredMemberRead, hasDeclaredMemberCall, hasDeclaredAssignment, hasDeclaredMemberWrite := false, false, false, false, false
		hasDeclaredArithmeticAssignment, hasArithmeticReturnCandidate := false, false
		localTables := bodyLocalTableTerms(child)
		assertionOperations := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "apply":
				if !memberCalls["call/"+operation.Target.Name] {
					return declaredBoundaryAdmission{}
				}
				hasDeclaredMemberCall = true
			case "external-call":
				application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
				if !found || !memberCalls[string(application)] {
					return declaredBoundaryAdmission{}
				}
			case "dynamic-index-read":
				container, found := artifactOperand(operation.Operands, equation.MustOperandRole("container"))
				if !found || !localTables[string(container)] {
					return declaredBoundaryAdmission{}
				}
			case "channel-select":
				return declaredBoundaryAdmission{}
			case "branch-relations":
				hasBranch = true
			case "path-replacement":
				hasDeclaredMemberWrite = hasDeclaredMemberWrite || index.declaredFormalMemberWrite(operation, formals)
			case "claim":
				hasDeclaredMemberRead = hasDeclaredMemberRead || index.declaredFormalMemberRead(operation, formals)
				hasDeclaredAssignment = hasDeclaredAssignment || index.declaredFormalAssignment(operation, formals, derivedCells)
				hasDeclaredArithmeticAssignment = hasDeclaredArithmeticAssignment || index.declaredFormalArithmeticAssignment(operation, arithmeticTerms)
				if index.declaredFormalAssertion(operation, formals) {
					assertionOperations[operation.Target.Name] = true
				}
			case "publication":
				hasArithmeticReturnCandidate = hasArithmeticReturnCandidate || index.declaredFormalArithmeticReturn(operation, arithmeticTerms)
			}
		}
		if hasDirectMethod && hasBranch {
			return declaredBoundaryAdmission{}
		}
		hasDeclaredArithmeticReturn := hasArithmeticReturnCandidate && !hasBranch
		concatOperations := index.declaredFormalConcatOperations(memberCalls)
		comparisonOperations := index.declaredFormalOrderedComparisonOperations(formals)
		if !hasDeclaredMemberRead && !hasDeclaredMemberCall && !hasDeclaredAssignment && !hasDeclaredMemberWrite && !hasDeclaredArithmeticAssignment && !hasDeclaredArithmeticReturn && len(assertionOperations) == 0 && len(concatOperations) == 0 && len(comparisonOperations) == 0 {
			return declaredBoundaryAdmission{}
		}
		seeds, ok := index.declaredSeeds()
		if !ok {
			return declaredBoundaryAdmission{}
		}
		return declaredBoundaryAdmission{Seeds: seeds, Admitted: true, Method: hasDirectMethod, MemberWrite: hasDeclaredMemberWrite, ArithmeticAssignment: hasDeclaredArithmeticAssignment, ArithmeticReturn: hasDeclaredArithmeticReturn, Concat: concatOperations, Comparison: comparisonOperations, Assertions: assertionOperations}
	}()
	if !admission.Admitted {
		return admissionLaneResult{}
	}
	assignment := admission.ArithmeticAssignment
	formals := ctx.bodyIndex.formals
	allocations := bodyLocalTableTerms(ctx.child)
	allocationReads := func() map[string]bool {
		var child front.DraftsView = ctx.child
		if len(allocations) == 0 {
			return nil
		}
		targets := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "dynamic-index-read" {
				continue
			}
			container, hasContainer := artifactOperand(operation.Operands, equation.MustOperandRole("container"))
			target, hasTarget := artifactOperand(operation.Operands, equation.MustOperandRole("target"))
			if !hasContainer || !hasTarget || !allocations[string(container)] {
				continue
			}
			targets[string(target)] = true
		}
		return targets
	}()
	derivedCells := ctx.bodyIndex.declaredFormalDerivedCells(formals)
	for _, operation := range ctx.bodyIndex.operations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		assignment = assignment ||
			ctx.bodyIndex.declaredFormalAssignment(operation, formals, derivedCells) ||
			func() bool {
				if operation.Occurrence.Kind != "claim" || (len(allocations) == 0 && len(allocationReads) == 0) {
					return false
				}
				assignment, hasKind := artifactOperand(operation.Operands, equation.MustOperandRole("kind"))
				value, hasValue := artifactOperand(operation.Operands, equation.MustOperandRole("value"))
				if !hasKind || !hasValue || string(assignment) != "claim-kind/3" {
					return false
				}
				if allocationReads[string(value)] {
					return true
				}
				root, suffix, member := tableAddress(value)
				return member && allocations[string(root)] && len(suffix) != 0
			}()
	}
	return lane.admission(admissionLaneDecision{
		Seeds:              admission.Seeds,
		Root:               admissionRootDeclared,
		Declared:           admission,
		DeclaredAssignment: assignment,
	})
}

func (lane *admissionLane) admitStaticCapturedReturn(ctx *admissionLaneContext) admissionLaneResult {
	if !func() bool {
		var partition equation.Partition = ctx.partition
		child := ctx.bodyIndex.child
		if ctx.lexical == nil || child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Parameters) != 0 || len(child.BodyBoundary().Captures) != 1 || len(child.BodyBoundary().DeclaredReturns) != 1 {
			return false
		}
		capture := boundaryTerm(child.BodyBoundary().Captures[0].Symbol)
		if _, found := closureHandleFor([]byte(capture), partition); !found {
			return false
		}
		called := false
		applications := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "entry", "environment-write", "publication":
				continue
			case "apply":
				callee, hasCallee := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
				arity, hasArity := artifactOperand(operation.Operands, equation.MustOperandRole("result-arity"))
				if !hasCallee || !hasArity || string(callee) != capture || string(arity) != "1" {
					return false
				}
				called = true
				applications["call/"+operation.Target.Name] = true
			case "external-call", "call-results":
				application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
				if !found || !applications[string(application)] {
					return false
				}
			default:
				return false
			}
		}
		return called
	}() {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Root: admissionRootCaptured})
}

func (lane *admissionLane) admitStaticArithmetic(ctx *admissionLaneContext) admissionLaneResult {
	if !func() bool {
		var (
			l         *lexicalEvaluator  = ctx.lexical
			body      equation.BodyID    = ctx.body
			partition equation.Partition = ctx.partition
		)
		child := ctx.bodyIndex.child
		if l == nil || child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Parameters) != 0 || len(child.BodyBoundary().Captures) == 0 {
			return false
		}
		captures := make(map[string]bool, len(child.BodyBoundary().Captures))
		arithmeticCallees := make(map[string]bool)
		for _, capture := range child.BodyBoundary().Captures {
			term := boundaryTerm(capture.Symbol)
			captures[term] = true
			if handle, found := closureHandleFor([]byte(term), partition); found {
				callee, exists := l.byPrototype[handle.Prototype]
				if !exists {
					return false
				}
				if unannotatedArithmeticFormal(callee) {
					arithmeticCallees[term] = true
				}
				continue
			}
			if _, imported := l.importedAuthority(body, term); !imported {
				return false
			}
		}
		foundArithmeticCall := false
		applications := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "entry", "publication", "environment-write":
				continue
			case "apply":
				callee, found := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
				if !found {
					return false
				}
				if arithmeticCallees[string(callee)] {
					foundArithmeticCall = true
					applications["call/"+operation.Target.Name] = true
					continue
				}
				root, _, member := tableAddress(callee)
				if !member || !captures[string(root)] {
					return false
				}
				if _, imported := l.importedAuthority(body, string(root)); !imported {
					return false
				}
				applications["call/"+operation.Target.Name] = true
			case "external-call", "call-results":
				application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
				if !found || !applications[string(application)] {
					return false
				}
			default:
				return false
			}
		}
		return foundArithmeticCall
	}() {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Root: admissionRootArithmetic})
}

func (lane *admissionLane) admitStaticMemberRead(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := func() ([]entrySeed, bool) {
		var index *admissionBodyIndex = &ctx.bodyIndex
		if index == nil {
			return nil, false
		}
		child := index.child
		if child.LoweredBody() == nil || child.CyclicDraft() != nil || len(child.BodyBoundary().Captures) != 0 {
			return nil, false
		}
		formals := index.formals
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "environment-write" {
				continue
			}
			for _, operand := range operation.Operands {
				if operand.Role.Wire() != "value" {
					continue
				}
				root, suffix, ok := tableAddress(operand.Term.Encoding)
				segments, static := segment.ParseFormattedSegments(suffix)
				if formals.has(string(root)) && ok && static && len(segments) == 1 && (segments[0].Kind == segment.SegmentField || segments[0].Kind == segment.SegmentIndexString) {
					declared, seeded := index.declaredSeeds()
					if !seeded {
						return nil, false
					}
					seeds := append([]entrySeed(nil), declared...)
					sort.Slice(seeds, func(i, j int) bool {
						return seeds[i].Term < seeds[j].Term
					})
					return seeds, true
				}
			}
		}
		return nil, false
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootClosedBody})
}

func (lane *admissionLane) admitDeclaredFormalCall(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := func() ([]entrySeed, bool) {
		var (
			index      *admissionBodyIndex   = &ctx.bodyIndex
			l          *lexicalEvaluator     = ctx.lexical
			allocation lexicalAllocationSite = lexicalAllocationSite{body: ctx.body, operation: ctx.operation}
			partition  equation.Partition    = ctx.partition
		)
		if !index.ready {
			return nil, false
		}
		child := index.child
		if child.LoweredBody() == nil || len(child.BodyBoundary().Parameters) == 0 {
			return nil, false
		}
		if len(child.BodyBoundary().Captures) != 0 && (child.CyclicDraft() != nil || !l.sealedCaptureEnvironment(child, allocation, partition, false)) {
			return nil, false
		}
		formals := index.formals
		formalFunctions := make(map[string]bool, len(child.BodyBoundary().Parameters))
		seeds, seeded := index.declaredSeeds()
		if !seeded {
			return nil, false
		}
		for _, parameter := range child.BodyBoundary().Parameters {
			declared := child.LoweredBody().Type(parameter.Type)
			term := boundaryTerm(parameter.Symbol)
			if _, isFunction := functionFormalType(declared); isFunction {
				formalFunctions[term] = true
			}
		}
		externalApplications := make(map[string]bool)
		for _, operation := range index.operations {
			if operation.Occurrence.Kind != "external-call" {
				continue
			}
			application, hasApplication := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
			if _, hasProvider := artifactOperand(operation.Operands, equation.MustOperandRole("provider")); hasApplication && hasProvider {
				externalApplications[string(application)] = true
			}
		}
		localClosures := bodyLocalObjectTerms(child, "object-kind/closure")
		for _, capture := range child.BodyBoundary().Captures {
			term := boundaryTerm(capture.Symbol)
			if _, found := closureHandleFor([]byte(term), partition); found {
				localClosures[term] = true
			}
		}
		localTables := bodyLocalTableTerms(child)
		memberCalls := make(map[string]bool)
		hasClosedCall := false
		for _, operation := range index.operations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			application := "call/" + operation.Target.Name
			if index.declaredFormalFunctionCall(operation, formalFunctions) || index.localClosureCall(operation, localClosures) || index.localMemberClosureCall(operation, localClosures, localTables) {
				memberCalls[application] = true
				hasClosedCall = true
				continue
			}
			if index.declaredMemberCall(operation, formals) || externalApplications[application] {
				memberCalls[application] = true
				continue
			}
			return nil, false
		}
		if !hasClosedCall {
			return nil, false
		}
		for _, operation := range child.DraftArtifact().Equations {
			switch operation.Occurrence.Kind {
			case "apply":
				if !memberCalls["call/"+operation.Target.Name] {
					return nil, false
				}
			case "external-call":
				application, found := artifactOperand(operation.Operands, equation.MustOperandRole("application"))
				if !found || !memberCalls[string(application)] {
					return nil, false
				}
			case "dynamic-index-read":
				container, found := artifactOperand(operation.Operands, equation.MustOperandRole("container"))
				if !found || !formals.has(string(container)) {
					return nil, false
				}
			case "channel-select":
				return nil, false
			}
		}
		return seeds, true
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	if len(ctx.child.BodyBoundary().Captures) == 0 {
		return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootDeclared})
	}
	operations := func() map[string]bool {
		var (
			l         *lexicalEvaluator             = ctx.lexical
			child     front.DraftsBoundaryGraphView = ctx.child
			partition equation.Partition            = ctx.partition
		)
		if l == nil || child.LoweredBody() == nil {
			return nil
		}
		formals := make(map[string]bool, len(child.BodyBoundary().Parameters))
		for _, parameter := range child.BodyBoundary().Parameters {
			formals[boundaryTerm(parameter.Symbol)] = true
		}
		written := make(map[string]string, len(formals))
		for _, operation := range child.DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			term, hasCallee := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
			if !hasCallee {
				continue
			}
			prototype, resolved := l.calleePrototype(child, term, partition)
			if !resolved || len(l.parameterWrites[prototype]) == 0 {
				continue
			}
			callee, known := l.byPrototype[prototype]
			if !known {
				continue
			}
			arguments := applicationArgumentTerms(operation)
			for index, parameter := range callee.BodyBoundary().Parameters {
				if index >= len(arguments) || !l.parameterWrites[prototype][boundaryTerm(parameter.Symbol)] {
					continue
				}
				argument := string(arguments[index])
				if !formals[argument] {
					continue
				}
				if prior, exists := written[argument]; !exists || operation.Target.Name < prior {
					written[argument] = operation.Target.Name
				}
			}
		}
		if len(written) == 0 {
			return nil
		}
		obligations := make(map[string]bool)
		for _, operation := range child.DraftArtifact().Equations {
			for term, writeOperation := range written {
				if operation.Target.Name <= writeOperation {
					continue
				}
				for _, operand := range operation.Operands {
					read := string(operand.Term.Encoding)
					if read == term || strings.HasPrefix(read, term+".") || strings.HasPrefix(read, term+"[") {
						obligations[operation.Target.Name] = true
						break
					}
				}
			}
		}
		return obligations
	}()
	if len(operations) == 0 {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{
		Seeds:              seeds,
		Root:               admissionRootSealedEnvironment,
		FormalMemberWrites: operations,
		SealedEnvironment:  true,
	})
}

func (lane *admissionLane) admitImportedCapture(ctx *admissionLaneContext) admissionLaneResult {
	seeds, admitted := func() ([]entrySeed, bool) {
		var (
			index     *admissionBodyIndex = &ctx.bodyIndex
			l         *lexicalEvaluator   = ctx.lexical
			body      equation.BodyID     = ctx.body
			partition equation.Partition  = ctx.partition
		)
		if !index.ready {
			return nil, false
		}
		child := index.child
		if l == nil || child.LoweredBody() == nil || len(child.BodyBoundary().Captures) == 0 {
			return nil, false
		}
		if childHasChannelLifecycle(child) || childHasResourceLifecycle(child) || childHasSelect(child) {
			return nil, false
		}
		for _, capture := range child.BodyBoundary().Captures {
			if capture.Mutable {
				return nil, false
			}
			term := boundaryTerm(capture.Symbol)
			if _, imported := l.importedAuthority(body, term); !imported {
				return nil, false
			}
			if _, closure := closureHandleFor([]byte(term), partition); closure {
				return nil, false
			}
		}
		return index.declaredSeeds()
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootImportedCapture})
}

func (lane *admissionLane) admitSealedCapture(ctx *admissionLaneContext) admissionLaneResult {
	if len(ctx.child.BodyBoundary().Parameters) == 0 && len(ctx.child.BodyBoundary().Captures) == 0 {
		return lane.admission(admissionLaneDecision{Root: admissionRootClosedBody})
	}
	admitted := func() bool {
		var ctx admissionLaneContext = *ctx
		if ctx.child.CyclicDraft() != nil || len(ctx.bodyIndex.formals) != 0 {
			return false
		}
		return ctx.lexical.sealedCaptureEnvironment(ctx.child, lexicalAllocationSite{body: ctx.body, operation: ctx.operation}, ctx.partition, false)
	}()
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Root: admissionRootSealedCapture})
}

func (lane *admissionLane) admitContextualCallback(ctx *admissionLaneContext) admissionLaneResult {
	seeds, parameters, admitted := ctx.lexical.contextualCallbackBoundary(ctx.body, ctx.child, ctx.result, ctx.partition)
	if !admitted {
		return admissionLaneResult{}
	}
	return lane.admission(admissionLaneDecision{Seeds: seeds, Root: admissionRootClosedBody, ContextualParameters: parameters})
}

func admissionRootClosedBody(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	seeds, closureSeeds := func() ([]entrySeed, []entryClosureSeed) {
		var seeds []entrySeed = decision.Seeds
		bound := make(map[string]bool, len(seeds))
		for _, seed := range seeds {
			bound[seed.Term] = true
		}
		for _, operation := range front.DraftsView(ctx.child).DraftArtifact().Equations {
			if operation.Occurrence.Kind != "environment-write" {
				continue
			}
			if target, found := artifactOperand(operation.Operands, equation.MustOperandRole("target")); found {
				bound[string(target)] = true
			}
		}
		callees := make([]string, 0, len(front.DraftsView(ctx.child).DraftArtifact().Equations))
		seen := make(map[string]bool, len(front.DraftsView(ctx.child).DraftArtifact().Equations))
		for _, operation := range front.DraftsView(ctx.child).DraftArtifact().Equations {
			if operation.Occurrence.Kind != "apply" {
				continue
			}
			callee, found := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
			if !found || bound[string(callee)] || seen[string(callee)] || !strings.HasPrefix(string(callee), "path/") {
				continue
			}
			seen[string(callee)] = true
			callees = append(callees, string(callee))
		}
		sort.Strings(callees)
		closureSeeds := make([]entryClosureSeed, 0, len(callees))
		for _, callee := range callees {
			term := []byte(callee)
			value, known := resolveKnownCurrentValue(term, ctx.partition)
			if !known || !isCallableValue(value) {
				continue
			}
			seed := entrySeed{Term: callee, Value: value}
			if !validEntrySeed(seed) {
				continue
			}
			handle, hasHandle := closureHandleFor(term, ctx.partition)
			if hasHandle {
				if _, admitted := ctx.lexical.byPrototype[handle.Prototype]; !admitted {
					continue
				}
			}
			seeds = append(seeds, seed)
			if hasHandle {
				closureSeeds = append(closureSeeds, entryClosureSeed{Term: callee, Handle: handle})
			}
		}
		sort.Slice(seeds, func(i, j int) bool {
			return seeds[i].Term < seeds[j].Term
		})
		return seeds, closureSeeds
	}()
	entry, err := encodeChildEntry(seeds, closureSeeds...)
	return entry, true, err
}

func admissionRootDeclared(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	entry, err := encodeDeclaredChildEntryWithCapabilities(decision.Seeds, nil, nil, declaredBoundaryIdentitySeeds(ctx.body, ctx.child, decision.Seeds, nil), nil)
	return entry, true, err
}

func admissionRootLocalUnionRead(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	var (
		l         *lexicalEvaluator  = ctx.lexical
		partition equation.Partition = ctx.partition
	)
	child := ctx.bodyIndex.child
	seeds := append([]entrySeed(nil), decision.Seeds...)
	closures := make([]entryClosureSeed, 0)
	seen := make(map[string]bool)
	for _, operation := range child.DraftArtifact().Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		callee, found := artifactOperand(operation.Operands, equation.MustOperandRole("callee"))
		if !found || !strings.HasPrefix(string(callee), "path/") || seen[string(callee)] {
			continue
		}
		value, known := resolveKnownCurrentValue(callee, partition)
		handle, callable := closureHandleFor(callee, partition)
		if !known || isUnknownScalar(value) || !callable || !ctx.bodyIndex.localUnionCalleeHasAlternativeReturns(l, handle) {
			return nil, false, nil
		}
		seen[string(callee)] = true
		seeds = append(seeds, entrySeed{Term: string(callee), Value: value})
		closures = append(closures, entryClosureSeed{Term: string(callee), Handle: handle})
	}
	if len(closures) == 0 {
		return nil, false, nil
	}
	entry, err := encodeChildEntryWithCapabilities(seeds, closures, childEntryMemberClosureSeeds(seeds, nil, partition), tableIdentitySeedsForEntry(seeds, nil, partition), memberCellSeedsForEntry(seeds, nil, partition))
	if err != nil {
		return nil, false, err
	}
	return entry, true, nil
}

func admissionRootImportedCapture(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, false, true, false, false)
}

func admissionRootClosedAnyCapture(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, true, false, false)
}

func admissionRootSealedEnvironment(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, false, true, true)
}

func admissionRootGradualLogical(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	if len(ctx.child.BodyBoundary().Captures) == 0 {
		entry, err := encodeChildEntryWithCapabilities(decision.Seeds, nil, nil, nil, nil, decision.GradualLogicalTerms)
		return entry, true, err
	}
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, false, false, false, decision.GradualLogicalTerms)
}

func admissionRootStaticAssignment(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, false, false, true)
}

func admissionRootCaptured(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, false, false, false)
}

func admissionRootArithmetic(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, true, true, false)
}

func admissionRootSealedCapture(ctx admissionLaneContext, decision admissionLaneDecision) ([]byte, bool, error) {
	return ctx.bodyIndex.childEntry(ctx.lexical, ctx.body, decision.Seeds, ctx.partition, true, false, true, false)
}
