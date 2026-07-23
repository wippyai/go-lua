package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const entryValue = "front/closed-entry/v1"

var errUnknownScalar = errors.New("engine: unknown scalar")

// ErrInternalPanic identifies an engine invariant failure that would otherwise
// escape Check as a panic. Check is the public whole-file boundary, so callers
// always receive a named error instead of an unclassified process crash.
var ErrInternalPanic = errors.New("engine: internal panic")

const branchPredicatePrefix = "front/branch-predicate/v1/"

// branchPredicateWire mirrors the front's closed predicate wire vocabulary.
// It intentionally contains only resolved WIR data, never an AST expression or
// an evaluator callback.
type branchPredicateWire struct {
	Kind           string `json:"kind"`
	Path           string `json:"path,omitempty"`
	OtherPath      string `json:"other_path,omitempty"`
	TypeName       string `json:"type_name,omitempty"`
	Literal        string `json:"literal,omitempty"`
	LenFloor       int64  `json:"len_floor,omitempty"`
	NumFloor       int64  `json:"num_floor,omitempty"`
	NumCeil        int64  `json:"num_ceil,omitempty"`
	HasNumCeil     bool   `json:"has_num_ceil,omitempty"`
	NumCeilNegated bool   `json:"num_ceil_negated,omitempty"`
	Negated        bool   `json:"negated,omitempty"`
	ProducerPoint  uint32 `json:"producer_point,omitempty"`
	HasProducer    bool   `json:"has_producer,omitempty"`
}

// Result is the complete result published by Check. Values use source display
// names for this small entrypoint; outcomes and diagnostics retain their
// equation-kernel candidate keys.
type Result struct {
	Artifact    equation.Artifact
	Values      []equation.Fact
	Outcomes    []equation.Fact
	Diagnostics []equation.Fact
	// PublishedDiagnostics is the source-facing projection of diagnostic facts.
	// Diagnostics remains the canonical equation publication; this companion
	// projection attaches only information already present in the closed
	// artifact and closure so hosts do not need to re-run analysis to render a
	// useful diagnostic.
	PublishedDiagnostics []PublishedDiagnostic
	// DiagnosticSpans maps an operation-scoped diagnostic fact key to the WIR
	// source span that produced it. It is intentionally source-only metadata:
	// equation facts remain portable and position-free.
	DiagnosticSpans map[string]wir.Span
	Transactions    int
	Timings         Timings
}

// PublishedDiagnostic enriches one equation diagnostic fact at the public
// publication boundary.  Its evidence is deliberately a projection of the
// closed claim operation and its abstract value, rather than a second
// diagnostic analysis pass.
type PublishedDiagnostic struct {
	Fact     equation.Fact
	Code     string
	Span     wir.Span
	Message  string
	Evidence []DiagnosticEvidence
	Labels   []DiagnosticLabel
	Help     string
}

// DiagnosticEvidence is engine-neutral evidence data consumed by host
// adapters. Kind and Trust use the diagnostic package's stable display
// vocabulary without making the equation kernel depend on presentation types.
type DiagnosticEvidence struct {
	Span    wir.Span
	Kind    string
	Trust   string
	Reason  string
	Message string
}

// DiagnosticLabel is a source annotation emitted with a published diagnostic.
type DiagnosticLabel struct {
	Span    wir.Span
	Message string
}

// Timings records the two engine-owned phases. Hosts add loading/resolution
// and rendering measurements around this boundary.
type Timings struct {
	ParseBindLower time.Duration
	Evaluate       time.Duration
}

// Check compiles source through the new front, binds its sole formal entry,
// and evaluates the front-selected acyclic or frozen-cyclic artifact. Both
// paths publish the identical result channels.
func Check(source string) (result Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", ErrInternalPanic, recovered)
			result = Result{}
		}
	}()
	parseStarted := time.Now()
	compilation, err := front.Compile(source)
	parseElapsed := time.Since(parseStarted)
	if err != nil {
		result = diagnosticResult("analysis/front", err)
		result.Timings.ParseBindLower = parseElapsed
		return result, nil
	}
	artifact := compilation.Artifact
	if len(artifact.Equations) == 0 {
		return Result{}, fmt.Errorf("engine: front returned an empty artifact")
	}
	evaluateStarted := time.Now()
	entry := artifact.Equations[0].Entry
	binding := equation.EntryBinding{Parameter: entry, Value: []byte(entryValue)}
	lexical := newLexicalEvaluator(compilation)
	closure := equation.OutputClosure{}
	transactions := 0
	if compilation.Cyclic == nil {
		bound, bindErr := equation.BindEntry(artifact, binding)
		if bindErr != nil {
			return Result{}, fmt.Errorf("engine: bind entry: %w", bindErr)
		}
		kernelRegistry, registryErr := registry(lexical)
		if registryErr != nil {
			return Result{}, registryErr
		}
		vm, vmErr := equation.NewAcyclicVM(kernelRegistry)
		if vmErr != nil {
			return Result{}, vmErr
		}
		evaluation, evaluateErr := vm.Evaluate(bound)
		if evaluateErr != nil {
			result = diagnosticResult("analysis/conservative", evaluateErr)
			result.Timings = Timings{ParseBindLower: parseElapsed, Evaluate: time.Since(evaluateStarted)}
			return result, nil
		}
		closure, transactions = evaluation.Closure, evaluation.Transactions
	} else {
		if _, compileErr := equation.CompileCyclicArtifact(*compilation.Cyclic); compileErr != nil {
			return Result{}, fmt.Errorf("engine: compile cyclic artifact: %w", compileErr)
		}
		bound, bindErr := equation.BindCyclicEntry(*compilation.Cyclic, binding)
		if bindErr != nil {
			return Result{}, fmt.Errorf("engine: bind cyclic entry: %w", bindErr)
		}
		kernelRegistry, registryErr := cyclicRegistry(lexical)
		if registryErr != nil {
			return Result{}, registryErr
		}
		vm, vmErr := equation.NewCyclicVM(kernelRegistry)
		if vmErr != nil {
			return Result{}, vmErr
		}
		evaluation, evaluateErr := vm.Evaluate(context.Background(), bound, []string{"published"})
		if evaluateErr != nil {
			result = diagnosticResult("analysis/conservative", evaluateErr)
			result.Timings = Timings{ParseBindLower: parseElapsed, Evaluate: time.Since(evaluateStarted)}
			return result, nil
		}
		closure, transactions = evaluation.Closure, evaluation.Transactions
	}
	diagnosticSpans := diagnosticSpans(compilation.ClaimSpans, compilation.CallSpans, compilation.BranchSpans, compilation.EffectSpans, closure.Diagnostics)
	for key, span := range lexical.diagnosticSpans {
		if diagnosticSpans == nil {
			diagnosticSpans = make(map[string]wir.Span)
		}
		diagnosticSpans[key] = span
	}
	result = Result{
		Artifact: artifact, Values: publishedValues(artifact, closure.Values),
		Outcomes: publishedOutcomes(closure.Outcomes), Diagnostics: closure.Diagnostics,
		PublishedDiagnostics: publishedDiagnostics(artifact, closure, diagnosticSpans, compilation.ClaimTargetSpans, compilation.CallSpans),
		DiagnosticSpans:      diagnosticSpans,
		Transactions:         transactions,
		Timings:              Timings{ParseBindLower: parseElapsed, Evaluate: time.Since(evaluateStarted)},
	}
	return result, nil
}

func diagnosticSpans(claimSpans, callSpans, branchSpans, effectSpans map[string]wir.Span, diagnostics []equation.Fact) map[string]wir.Span {
	if (len(claimSpans) == 0 && len(callSpans) == 0 && len(branchSpans) == 0 && len(effectSpans) == 0) || len(diagnostics) == 0 {
		return nil
	}
	out := make(map[string]wir.Span)
	for _, item := range diagnostics {
		var name string
		switch {
		case strings.HasPrefix(item.Key, "claim/unproven/"):
			name = strings.TrimPrefix(item.Key, "claim/unproven/")
		case strings.HasPrefix(item.Key, "type.assignment/"):
			name = strings.TrimPrefix(item.Key, "type.assignment/")
		case strings.HasPrefix(item.Key, "advice.redundant_claim/"):
			name = strings.TrimPrefix(item.Key, "advice.redundant_claim/")
			if span, ok := callSpans[name+"/call"]; ok {
				out[item.Key] = span
				continue
			}
		case strings.HasPrefix(item.Key, "advice.always_true_guard/"), strings.HasPrefix(item.Key, "lint.condition.redundant/"):
			name = item.Key[strings.LastIndexByte(item.Key, '/')+1:]
			if span, ok := branchSpans[name]; ok {
				out[item.Key] = span
			}
			continue
		case strings.HasPrefix(item.Key, "type.call.direct."):
			if slash := strings.IndexByte(item.Key, '/'); slash >= 0 {
				if span, ok := callSpans[item.Key[slash+1:]]; ok {
					out[item.Key] = span
				}
			}
			continue
		case strings.HasPrefix(item.Key, "send.isolation/"):
			name = strings.TrimPrefix(item.Key, "send.isolation/")
			if span, ok := callSpans[name+"/argument-00000002"]; ok {
				out[item.Key] = span
				continue
			}
			if span, ok := callSpans[name+"/call"]; ok {
				out[item.Key] = span
			}
			continue
		case strings.HasPrefix(item.Key, "effect.freeze.mutation/"):
			parts := strings.Split(item.Key, "/")
			if len(parts) == 3 {
				if span, ok := effectSpans[parts[1]]; ok {
					out[item.Key] = span
				}
			}
			continue
		default:
			continue
		}
		if span, ok := claimSpans[name]; ok {
			out[item.Key] = span
		}
	}
	return out
}

// publishedDiagnostics is the sole rich-diagnostic projection.  Kernels still
// publish equation facts; this function only joins each published fact to the
// claim operation that produced it and to the abstract value already closed by
// the VM.  In particular, it neither evaluates source nor manufactures a
// diagnostic that is absent from closure.Diagnostics.
func publishedDiagnostics(artifact equation.Artifact, closure equation.OutputClosure, spans, claimTargetSpans, callSpans map[string]wir.Span) []PublishedDiagnostic {
	if len(closure.Diagnostics) == 0 {
		return nil
	}
	claims := make(map[string]equation.Equation)
	applies := make(map[string]equation.Equation)
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind == "claim" {
			claims[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "apply" {
			applies[operation.Target.Name] = operation
		}
	}
	out := make([]PublishedDiagnostic, 0, len(closure.Diagnostics))
	for _, fact := range closure.Diagnostics {
		item := PublishedDiagnostic{Fact: cloneFact(fact), Code: diagnosticCode(fact.Key), Span: spans[fact.Key], Message: string(fact.Value)}
		if strings.HasPrefix(fact.Key, "effect.freeze.mutation/") {
			out = append(out, enrichFrozenMutationDiagnostic(item, artifact, callSpans))
			continue
		}
		if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "type.call.direct.argument_type/") {
			out = append(out, enrichCallArgumentDiagnostic(item, operation))
			continue
		}
		if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "advice.redundant_claim/") {
			out = append(out, enrichRedundantCastDiagnostic(item, operation, callSpans))
			continue
		}
		if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "send.isolation/") {
			out = append(out, enrichSendIsolationDiagnostic(item, operation))
			continue
		}
		if operation, ok := claims[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "advice.redundant_claim/") {
			out = append(out, enrichRedundantClaimDiagnostic(item, operation))
			continue
		}
		if operation, ok := branchOperation(artifact, diagnosticOperationName(fact.Key)); ok && (strings.HasPrefix(fact.Key, "advice.always_true_guard/") || strings.HasPrefix(fact.Key, "lint.condition.redundant/")) {
			out = append(out, enrichConstantGuardDiagnostic(item, operation, fact.Key))
			continue
		}
		name, assignment := strings.CutPrefix(fact.Key, "type.assignment/")
		if !assignment {
			out = append(out, item)
			continue
		}
		operation, found := claims[name]
		if !found {
			out = append(out, item)
			continue
		}
		operands, err := artifactOperandsByRole(operation.Operands, "target", "value", "type")
		if err != nil {
			out = append(out, item)
			continue
		}
		display := strings.TrimPrefix(string(operands["target"]), "path/")
		sourceDisplay := display
		for _, operand := range operation.Operands {
			if operand.Role == "display" && len(operand.Term.Encoding) != 0 {
				display = string(operand.Term.Encoding)
			}
			if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
				sourceDisplay = string(operand.Term.Encoding)
			}
		}
		value, available := claimDiagnosticValue(operands["value"], operation, closure)
		// An explicit any may be carried by an ancestor path (for example
		// raw.id).  The scalar read can still retain a literal heap fact, but the
		// ancestor boundary is the authoritative assignment source and is itself
		// sufficient closed evidence for the diagnostic projection.
		anySource := (available && isExplicitAnyValue(value)) || sourceHasExplicitAny(operands["value"], closure.Values)
		if !available && anySource {
			value, available = []byte("scalar/claim/claim-kind/3/\"any\""), true
		}
		if !available {
			out = append(out, item)
			continue
		}
		declared, unquoteErr := strconv.Unquote(strings.TrimPrefix(string(operands["type"]), "claim-type/"))
		if unquoteErr != nil {
			out = append(out, item)
			continue
		}
		valueDescription := assignmentEvidenceValue(value)
		if anySource {
			valueDescription = "any"
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type any", sourceDisplay)},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
				{Span: item.Span, Kind: "unvalidated value", Trust: "unknown", Reason: "explicit boundary validation", Message: fmt.Sprintf("%s comes from any/unknown", sourceDisplay)},
				{Span: item.Span, Kind: "missing proof", Trust: "unknown", Reason: "boundary validation missing", Message: fmt.Sprintf("no proof on this path shows %s satisfies the declared type", sourceDisplay)},
			}
			item.Labels = []DiagnosticLabel{
				{Span: item.Span, Message: "assigned value " + valueDescription},
				{Span: targetSpan, Message: "declared type " + declared},
			}
			item.Help = "Use a value compatible with the expected type, or change the target type if `" + display + "` is valid."
			out = append(out, item)
			continue
		}
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has literal value %s", display, valueDescription)},
			{Span: item.Span, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
		}
		item.Labels = []DiagnosticLabel{
			{Span: item.Span, Message: "assigned value " + valueDescription},
			{Span: item.Span, Message: "declared type " + declared},
		}
		item.Help = "Use a value compatible with the expected type, or change the target type if `" + display + "` is valid."
		out = append(out, item)
	}
	return out
}

func enrichFrozenMutationDiagnostic(item PublishedDiagnostic, artifact equation.Artifact, callSpans map[string]wir.Span) PublishedDiagnostic {
	parts := strings.Split(item.Fact.Key, "/")
	if len(parts) != 3 {
		return item
	}
	action, proof := parts[1], parts[2]
	var operation equation.Equation
	found := false
	for _, candidate := range artifact.Equations {
		if candidate.Target.Name == action {
			operation, found = candidate, true
			break
		}
	}
	if !found {
		return item
	}
	display, callMutation := "", operation.Occurrence.Kind == "apply"
	for _, operand := range operation.Operands {
		if operand.Role == "freeze-display" || (callMutation && operand.Role == "argument-display-00000000") {
			display = string(operand.Term.Encoding)
		}
	}
	if display == "" {
		return item
	}
	mutation := "this assignment mutates table " + strconv.Quote(display)
	label := "mutation of frozen table"
	if callMutation {
		mutation = "this call mutates table " + strconv.Quote(display)
		label = "mutating call on frozen table"
	}
	proofMessage := "table " + strconv.Quote(display) + " is already frozen here"
	if proof != "guard" {
		suffix := "assignment"
		if callMutation {
			suffix = "mutating call"
		}
		proofMessage = "table " + strconv.Quote(display) + " was frozen by this call before the " + suffix
	}
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: mutation}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: label}}
	if proof != "guard" {
		if span, ok := callSpans[proof+"/call"]; ok {
			item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: span, Kind: "abstract fact", Trust: "proven", Message: proofMessage})
			item.Labels = append(item.Labels, DiagnosticLabel{Span: span, Message: "freeze proof"})
		} else {
			item.Evidence = append(item.Evidence, DiagnosticEvidence{Kind: "abstract fact", Trust: "proven", Message: proofMessage})
		}
	} else {
		item.Evidence = append(item.Evidence, DiagnosticEvidence{Kind: "abstract fact", Trust: "proven", Message: proofMessage})
	}
	if callMutation {
		item.Help = "Create a mutable copy before calling the mutator, or call it before the table is frozen."
	} else {
		item.Help = "Create a mutable copy before writing, or move this assignment before the table is frozen."
	}
	return item
}

func claimDiagnosticValue(term []byte, operation equation.Equation, closure equation.OutputClosure) ([]byte, bool) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), true
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, false
	}
	prefix := "value/" + string(term) + "/"
	// The claim's own refinement is also a value fact. Its dependencies are
	// the closed input facts the kernel read before that refinement, so prefer
	// those facts when explaining a mismatch.
	for _, dependency := range operation.Dependencies {
		for _, fact := range closure.Values {
			if fact.Key == prefix+dependency.Name {
				return append([]byte(nil), fact.Value...), true
			}
		}
	}
	return nil, false
}

func diagnosticOperationName(key string) string {
	for _, prefix := range []string{"advice.redundant_claim/", "advice.always_true_guard/", "lint.condition.redundant/", "send.isolation/", "effect.freeze.mutation/"} {
		if name, ok := strings.CutPrefix(key, prefix); ok {
			return name
		}
	}
	parts := strings.Split(key, "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

func enrichCallArgumentDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
	operands := make(map[string]string, len(operation.Operands))
	for _, operand := range operation.Operands {
		operands[operand.Role] = string(operand.Term.Encoding)
	}
	callee := operands["callee-display"]
	if callee == "" {
		callee = strings.TrimPrefix(operands["callee"], "path/")
	}
	if callee == "" {
		return item
	}
	message := item.Message
	start := strings.Index(message, " is ")
	end := strings.LastIndex(message, ", not ")
	if start < 0 || end <= start+4 {
		return item
	}
	parts := strings.Split(strings.TrimPrefix(item.Fact.Key, "type.call.direct.argument_type/"), "/")
	if len(parts) != 2 {
		return item
	}
	argument := strings.TrimPrefix(parts[1], "argument-")
	argument = strings.TrimLeft(argument, "0")
	if argument == "" {
		argument = "0"
	}
	argumentIndex, err := strconv.Atoi(argument)
	if err != nil {
		return item
	}
	argumentIndex++
	value, expected := message[start+4:end], message[end+6:]
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("argument %d has literal value %s", argumentIndex, value)},
		{Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s parameter %d expects %s", callee, argumentIndex, expected)},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "argument value " + value}}
	item.Help = fmt.Sprintf("Pass a value for argument %d that satisfies the parameter type, or change the callee signature.", argumentIndex)
	return item
}

func cloneFact(fact equation.Fact) equation.Fact {
	return equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)}
}

func diagnosticCode(key string) string {
	if strings.HasPrefix(key, "child/") {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) == 3 {
			return diagnosticCode(parts[2])
		}
	}
	switch {
	case strings.HasPrefix(key, "advice.redundant_claim/"):
		return "advice.redundant_claim"
	case strings.HasPrefix(key, "advice.always_true_guard/"):
		return "advice.always_true_guard"
	case strings.HasPrefix(key, "lint.condition.redundant/"):
		return "lint.condition.redundant"
	case strings.HasPrefix(key, "type.assignment/"):
		return "type.assignment"
	case strings.HasPrefix(key, "send.isolation/"):
		return "send.isolation"
	case strings.HasPrefix(key, "effect.freeze.mutation/"):
		return "effect.freeze.mutation"
	case strings.HasPrefix(key, "claim/unproven/"):
		return "lint.claim.unproven"
	case strings.HasPrefix(key, "type.call.direct."):
		if slash := strings.IndexByte(key, '/'); slash >= 0 {
			return key[:slash]
		}
	}
	return "lint." + strings.ReplaceAll(key, "/", ".")
}

func branchOperation(artifact equation.Artifact, name string) (equation.Equation, bool) {
	for _, operation := range artifact.Equations {
		if operation.Target.Name == name && operation.Occurrence.Kind == "branch-relations" {
			return operation, true
		}
	}
	return equation.Equation{}, false
}

func enrichRedundantClaimDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
	operands, err := artifactOperandsByRole(operation.Operands, "value", "type")
	if err != nil {
		return item
	}
	target, err := strconv.Unquote(strings.TrimPrefix(string(operands["type"]), "claim-type/"))
	if err != nil {
		return item
	}
	value := strings.TrimPrefix(string(operands["value"]), "path/")
	for _, operand := range operation.Operands {
		if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
			value = string(operand.Term.Encoding)
		}
	}
	item.Message = "type claim is redundant; value is already " + target
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s is proven to be %s before the claim", value, target)},
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("claim checks %s at this site", target)},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "claim site"}, {Span: item.Span, Message: "proven value"}}
	item.Help = "Remove the runtime type claim when the proven source type is sufficient."
	return item
}

func enrichRedundantCastDiagnostic(item PublishedDiagnostic, operation equation.Equation, callSpans map[string]wir.Span) PublishedDiagnostic {
	var argument []byte
	value := "value"
	for _, operand := range operation.Operands {
		if operand.Role == "argument-00000000" {
			argument = operand.Term.Encoding
		}
		if operand.Role == "argument-display-00000000" {
			value = string(operand.Term.Encoding)
		}
	}
	if len(argument) == 0 {
		return item
	}
	argumentSpan := callSpans[operation.Target.Name+"/argument-00000000"]
	if !argumentSpan.Valid() {
		argumentSpan = item.Span
	}
	if value == "value" {
		value = strings.TrimPrefix(string(argument), "path/")
	}
	item.Message = "type cast call is redundant; value is already string"
	item.Evidence = []DiagnosticEvidence{
		{Span: argumentSpan, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s is proven to be string before the claim", value)},
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "claim checks string at this site"},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "claim site"}, {Span: argumentSpan, Message: "proven value"}}
	item.Help = "Remove the runtime type claim when the proven source type is sufficient."
	return item
}

func enrichConstantGuardDiagnostic(item PublishedDiagnostic, operation equation.Equation, key string) PublishedDiagnostic {
	if strings.HasPrefix(key, "advice.always_true_guard/") {
		item.Message = "condition is proven always true"
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "condition is proven to be true on every reachable path"}}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "constant guard"}}
		item.Help = "Remove the guard or move the guarded code out of the branch."
	}
	if strings.HasPrefix(key, "lint.condition.redundant/") {
		always := string(item.Fact.Value) == "true"
		if always {
			item.Message = "condition is always true here"
			item.Help = "Remove this repeated check, or move any needed work into the branch already guarded above."
		} else {
			item.Message = "condition is always false here"
			item.Help = "Remove this unreachable branch, or change the prior guard if this path should still run."
		}
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "condition is proven constant under its enclosing guard"}}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "constant guard"}}
	}
	return item
}

func assignmentEvidenceValue(value []byte) string {
	display, err := displayValue(value)
	if err == nil {
		return string(display)
	}
	return assignmentValueType(value)
}

// diagnosticResult is the whole-file recovery boundary for source-driven
// limitations.  The front and equation VM deliberately reject incomplete
// representations rather than fabricate a precise fact.  At the public API
// boundary those rejections are published as a diagnostic result, so malformed
// or currently-unmodelled Lua remains an analysable input instead of an engine
// failure.  Invariant failures still use ErrInternalPanic above.
func diagnosticResult(code string, cause error) Result {
	return Result{Diagnostics: []equation.Fact{{
		Key:   code,
		Value: []byte(cause.Error()),
	}}}
}

func registry(lexical *lexicalEvaluator) (*equation.KernelRegistry, error) {
	binding := func(kind string, kernel equation.Kernel) (equation.KernelBinding, error) {
		kernelID, known := front.KernelID(kind)
		contract, contracted := front.ContractID(kind)
		if !known || !contracted {
			return equation.KernelBinding{}, fmt.Errorf("engine: missing front kernel contract for %q", kind)
		}
		return equation.KernelBinding{KernelID: kernelID, ContractID: contract, Kernel: kernel}, nil
	}
	entry, err := binding("entry", equation.KernelFunc(entryKernel))
	if err != nil {
		return nil, err
	}
	allocationTemplate, err := binding("allocation-template", equation.KernelFunc(allocationTemplateKernel))
	if err != nil {
		return nil, err
	}
	objectMaterialization, err := binding("object-materialization", equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		return objectMaterializationKernel(lexical, operation, partition)
	}))
	if err != nil {
		return nil, err
	}
	write, err := binding("environment-write", equation.KernelFunc(writeKernel))
	if err != nil {
		return nil, err
	}
	claim, err := binding("claim", equation.KernelFunc(claimKernel))
	if err != nil {
		return nil, err
	}
	expression, err := binding("expression", equation.KernelFunc(expressionKernel))
	if err != nil {
		return nil, err
	}
	branch, err := binding("branch-relations", equation.KernelFunc(branchKernel))
	if err != nil {
		return nil, err
	}
	apply, err := binding("apply", equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		return applyKernel(lexical, operation, partition)
	}))
	if err != nil {
		return nil, err
	}
	results, err := binding("call-results", equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		return callResultsKernel(lexical, operation, partition)
	}))
	if err != nil {
		return nil, err
	}
	external, err := binding("external-call", equation.KernelFunc(externalCallKernel))
	if err != nil {
		return nil, err
	}
	publication, err := binding("publication", equation.KernelFunc(publicationKernel))
	if err != nil {
		return nil, err
	}
	pathReplacement, err := binding("path-replacement", equation.KernelFunc(pathReplacementKernel))
	if err != nil {
		return nil, err
	}
	dynamicIndexRead, err := binding("dynamic-index-read", equation.KernelFunc(dynamicIndexReadKernel))
	if err != nil {
		return nil, err
	}
	pathInvalidation, err := binding("path-invalidation", equation.KernelFunc(pathInvalidationKernel))
	if err != nil {
		return nil, err
	}
	indexMutation, err := binding("index-mutation", equation.KernelFunc(indexMutationKernel))
	if err != nil {
		return nil, err
	}
	genericFor, err := binding("generic-for", equation.KernelFunc(genericForKernel))
	if err != nil {
		return nil, err
	}
	channelSelect, err := binding("channel-select", equation.KernelFunc(channelSelectKernel))
	if err != nil {
		return nil, err
	}
	registry, err := equation.NewKernelRegistry([]equation.KernelBinding{
		entry, allocationTemplate, objectMaterialization, write,
		pathReplacement, dynamicIndexRead, pathInvalidation, indexMutation,
		branch, apply, external, results, genericFor, channelSelect, publication, claim, expression,
	})
	if err != nil {
		return nil, fmt.Errorf("engine: build kernel registry: %w", err)
	}
	return registry, nil
}

// cyclicRegistry binds the same source-owned semantic kernels to the cyclic
// transaction interface. The adapter materializes the immutable snapshot as
// the ordinary kernel partition, so no cyclic-only transfer semantics can
// diverge from the established publication path.
func cyclicRegistry(lexical *lexicalEvaluator) (*equation.CyclicKernelRegistry, error) {
	binding := func(kind string, kernel equation.Kernel) (equation.CyclicKernelBinding, error) {
		kernelID, known := front.KernelID(kind)
		contract, contracted := front.ContractID(kind)
		if !known || !contracted {
			return equation.CyclicKernelBinding{}, fmt.Errorf("engine: missing front cyclic kernel contract for %q", kind)
		}
		return equation.CyclicKernelBinding{
			KernelID: kernelID, ContractID: contract,
			Kernel: equation.CyclicKernelFunc(func(ctx context.Context, operation equation.BoundCyclicEquation, snapshot equation.CyclicSnapshot) (equation.TransactionResult, error) {
				if err := ctx.Err(); err != nil {
					return equation.TransactionResult{}, err
				}
				closures := make([]equation.OutputClosure, 0)
				seen := make(map[equation.CellID]bool)
				var collect func(equation.CellID)
				collect = func(cell equation.CellID) {
					if seen[cell] {
						return
					}
					seen[cell] = true
					for _, predecessor := range snapshot.Predecessors(cell) {
						collect(predecessor)
					}
					for _, leaf := range snapshot.Read(cell).Leaves {
						closures = append(closures, leaf.Closure)
					}
				}
				for _, predecessor := range snapshot.Predecessors(operation.Cell) {
					collect(predecessor)
				}
				partition, err := equation.PartitionFromClosures(closures...)
				if err != nil {
					return equation.TransactionResult{}, fmt.Errorf("engine: cyclic snapshot partition: %w", err)
				}
				return kernel.Execute(operation.Equation, partition)
			}),
		}, nil
	}
	entry, err := binding("entry", equation.KernelFunc(entryKernel))
	if err != nil {
		return nil, err
	}
	allocationTemplate, err := binding("allocation-template", equation.KernelFunc(allocationTemplateKernel))
	if err != nil {
		return nil, err
	}
	objectMaterialization, err := binding("object-materialization", equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		return objectMaterializationKernel(lexical, operation, partition)
	}))
	if err != nil {
		return nil, err
	}
	write, err := binding("environment-write", equation.KernelFunc(writeKernel))
	if err != nil {
		return nil, err
	}
	claim, err := binding("claim", equation.KernelFunc(claimKernel))
	if err != nil {
		return nil, err
	}
	expression, err := binding("expression", equation.KernelFunc(expressionKernel))
	if err != nil {
		return nil, err
	}
	pathReplacement, err := binding("path-replacement", equation.KernelFunc(pathReplacementKernel))
	if err != nil {
		return nil, err
	}
	dynamicIndexRead, err := binding("dynamic-index-read", equation.KernelFunc(dynamicIndexReadKernel))
	if err != nil {
		return nil, err
	}
	pathInvalidation, err := binding("path-invalidation", equation.KernelFunc(pathInvalidationKernel))
	if err != nil {
		return nil, err
	}
	indexMutation, err := binding("index-mutation", equation.KernelFunc(indexMutationKernel))
	if err != nil {
		return nil, err
	}
	branch, err := binding("branch-relations", equation.KernelFunc(branchKernel))
	if err != nil {
		return nil, err
	}
	apply, err := binding("apply", equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		return applyKernel(lexical, operation, partition)
	}))
	if err != nil {
		return nil, err
	}
	results, err := binding("call-results", equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		return callResultsKernel(lexical, operation, partition)
	}))
	if err != nil {
		return nil, err
	}
	external, err := binding("external-call", equation.KernelFunc(externalCallKernel))
	if err != nil {
		return nil, err
	}
	genericFor, err := binding("generic-for", equation.KernelFunc(genericForKernel))
	if err != nil {
		return nil, err
	}
	channelSelect, err := binding("channel-select", equation.KernelFunc(channelSelectKernel))
	if err != nil {
		return nil, err
	}
	publication, err := binding("publication", equation.KernelFunc(publicationKernel))
	if err != nil {
		return nil, err
	}
	registry, err := equation.NewCyclicKernelRegistry([]equation.CyclicKernelBinding{
		entry, allocationTemplate, objectMaterialization, write,
		pathReplacement, dynamicIndexRead, pathInvalidation, indexMutation,
		branch, apply, external, results, genericFor, channelSelect, publication, claim, expression,
	})
	if err != nil {
		return nil, fmt.Errorf("engine: build cyclic kernel registry: %w", err)
	}
	return registry, nil
}

// childEntryWire is deliberately a closed entry payload.  It is decoded only
// by the entry transaction and becomes ordinary body-local seed facts; no
// caller partition is ever shared with a child evaluator.
type childEntryWire struct {
	Version uint8       `json:"version"`
	Seeds   []entrySeed `json:"seeds"`
}

type entrySeed struct {
	Term  string `json:"term"`
	Value []byte `json:"value"`
}

type closureHandle struct {
	Prototype string   `json:"prototype"`
	Captures  []string `json:"captures"`
}

type lexicalEvaluator struct {
	byPrototype     map[string]front.Compilation
	requiresBody    map[string]bool
	diagnosticSpans map[string]wir.Span
	active          map[equation.BodyID]bool
}

func newLexicalEvaluator(root front.Compilation) *lexicalEvaluator {
	l := &lexicalEvaluator{byPrototype: make(map[string]front.Compilation), requiresBody: make(map[string]bool), diagnosticSpans: make(map[string]wir.Span), active: make(map[equation.BodyID]bool)}
	var add func(front.Compilation)
	add = func(compilation front.Compilation) {
		if compilation.PrototypeName != "" {
			l.byPrototype[compilation.PrototypeName] = compilation
		}
		for _, child := range compilation.Nested {
			add(child)
		}
	}
	add(root)
	var mark func(front.Compilation) bool
	mark = func(compilation front.Compilation) bool {
		required := len(compilation.Boundary.Captures) > 0
		for _, child := range compilation.Nested {
			required = mark(child) || required
		}
		if compilation.PrototypeName != "" {
			l.requiresBody[compilation.PrototypeName] = required
		}
		return required
	}
	mark(root)
	return l
}

func (l *lexicalEvaluator) evaluate(compilation front.Compilation, entryValue []byte) (equation.OutputClosure, int, error) {
	if l == nil || !compilation.Body.Valid() || len(compilation.Artifact.Equations) == 0 {
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: incomplete lexical body admission")
	}
	if l.active[compilation.Body] {
		// The existing evaluator has no SCC coordinator yet.  Treating this as
		// a transaction failure preserves atomicity: no in-flight child closure
		// is exposed as a partial recursive summary.
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: recursive lexical body requires SCC coordination")
	}
	l.active[compilation.Body] = true
	defer delete(l.active, compilation.Body)
	entry := compilation.Artifact.Equations[0].Entry
	binding := equation.EntryBinding{Parameter: entry, Value: append([]byte(nil), entryValue...)}
	if compilation.Cyclic == nil {
		bound, err := equation.BindEntry(compilation.Artifact, binding)
		if err != nil {
			return equation.OutputClosure{}, 0, fmt.Errorf("engine: bind lexical entry: %w", err)
		}
		kernelRegistry, err := registry(l)
		if err != nil {
			return equation.OutputClosure{}, 0, err
		}
		vm, err := equation.NewAcyclicVM(kernelRegistry)
		if err != nil {
			return equation.OutputClosure{}, 0, err
		}
		evaluation, err := vm.Evaluate(bound)
		if err != nil {
			return equation.OutputClosure{}, 0, err
		}
		return evaluation.Closure, evaluation.Transactions, nil
	}
	if _, err := equation.CompileCyclicArtifact(*compilation.Cyclic); err != nil {
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: compile lexical cyclic artifact: %w", err)
	}
	bound, err := equation.BindCyclicEntry(*compilation.Cyclic, binding)
	if err != nil {
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: bind lexical cyclic entry: %w", err)
	}
	kernelRegistry, err := cyclicRegistry(l)
	if err != nil {
		return equation.OutputClosure{}, 0, err
	}
	vm, err := equation.NewCyclicVM(kernelRegistry)
	if err != nil {
		return equation.OutputClosure{}, 0, err
	}
	evaluation, err := vm.Evaluate(context.Background(), bound, []string{"published"})
	if err != nil {
		return equation.OutputClosure{}, 0, err
	}
	return evaluation.Closure, evaluation.Transactions, nil
}

func encodeChildEntry(seeds []entrySeed) ([]byte, error) {
	wire := childEntryWire{Version: 1, Seeds: append([]entrySeed(nil), seeds...)}
	sort.Slice(wire.Seeds, func(i, j int) bool { return wire.Seeds[i].Term < wire.Seeds[j].Term })
	for index := range wire.Seeds {
		if wire.Seeds[index].Term == "" || len(wire.Seeds[index].Value) == 0 || (index > 0 && wire.Seeds[index-1].Term == wire.Seeds[index].Term) {
			return nil, fmt.Errorf("engine: malformed child entry seed")
		}
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("engine: encode child entry: %w", err)
	}
	return append([]byte("front/child-entry/v1/"), encoded...), nil
}

func entryKernel(operation equation.BoundEquation, _ equation.Partition) (equation.TransactionResult, error) {
	operands, err := operandsByRole(operation.Operands, "entry")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	const prefix = "front/child-entry/v1/"
	if !strings.HasPrefix(string(operands["entry"]), prefix) {
		return equation.TransactionResult{Complete: true}, nil
	}
	var wire childEntryWire
	if err := json.Unmarshal(operands["entry"][len(prefix):], &wire); err != nil || wire.Version != 1 {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry wire")
	}
	values := make([]equation.Fact, 0, len(wire.Seeds))
	seen := make(map[string]bool, len(wire.Seeds))
	for _, seed := range wire.Seeds {
		if seed.Term == "" || len(seed.Value) == 0 || seen[seed.Term] || (!strings.HasPrefix(seed.Term, "path/") && !strings.HasPrefix(seed.Term, "temp/")) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry seed")
		}
		seen[seed.Term] = true
		values = append(values, equation.Fact{Key: "value/" + seed.Term + "/entry", Value: append([]byte(nil), seed.Value...)})
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

func closureHandleFor(term []byte, partition equation.Partition) (closureHandle, bool) {
	prefix := "closure/" + string(term) + "/"
	var encoded []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (encoded == nil || fact.Key > latest) {
			encoded, latest = fact.Value, fact.Key
		}
	}
	if encoded == nil {
		return closureHandle{}, false
	}
	var handle closureHandle
	if json.Unmarshal(encoded, &handle) != nil || handle.Prototype == "" {
		return closureHandle{}, false
	}
	for _, capture := range handle.Captures {
		if !strings.HasPrefix(capture, "path/") && !strings.HasPrefix(capture, "temp/") && !strings.HasPrefix(capture, "scalar/") {
			return closureHandle{}, false
		}
	}
	return handle, true
}

func boundaryTerm(symbol wir.SymbolID) string { return fmt.Sprintf("path/sym%d", symbol) }

// applyKnown evaluates a complete lexical child privately, then projects only
// caller-owned results, capture effects, and residual diagnostics. A malformed
// entry or child failure returns an error, so the surrounding VM publishes no
// partial child result.
func (l *lexicalEvaluator) applyKnown(operation equation.BoundEquation, operands directCallOperands, handle closureHandle, partition equation.Partition) (equation.TransactionResult, error) {
	child, exists := l.byPrototype[handle.Prototype]
	if !exists {
		return equation.TransactionResult{}, fmt.Errorf("engine: known lexical target %q is unavailable", handle.Prototype)
	}
	if operands.spread || len(operands.arguments) != len(child.Boundary.Parameters) || len(handle.Captures) != len(child.Boundary.Captures) {
		return equation.TransactionResult{}, fmt.Errorf("engine: unsupported exact lexical boundary for %q", handle.Prototype)
	}
	seeds := make([]entrySeed, 0, len(operands.arguments)+len(handle.Captures))
	for index, parameter := range child.Boundary.Parameters {
		if parameter.Vararg {
			return equation.TransactionResult{}, fmt.Errorf("engine: vararg lexical boundary is unsupported")
		}
		value, known := resolveKnownCurrentValue(operands.arguments[index], partition)
		if !known {
			return equation.TransactionResult{}, fmt.Errorf("engine: incomplete lexical argument %d", index)
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: value})
	}
	for index, capture := range child.Boundary.Captures {
		value, known := resolveKnownCurrentValue([]byte(handle.Captures[index]), partition)
		if !known {
			return equation.TransactionResult{}, fmt.Errorf("engine: incomplete lexical capture %q", capture.Name)
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(capture.Symbol), Value: value})
	}
	entry, err := encodeChildEntry(seeds)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	closure, _, err := l.evaluate(child, entry)
	if err != nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q: %w", handle.Prototype, err)
	}
	returns, err := childReturnValues(closure)
	if err != nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q: %w", handle.Prototype, err)
	}
	projected := equation.OutputClosure{}
	for index, value := range returns {
		projected.Values = append(projected.Values, equation.Fact{Key: fmt.Sprintf("call-result/%s/%08d", operation.Target.Name, index), Value: value})
	}
	for index, capture := range child.Boundary.Captures {
		value, found := latestClosedValue([]byte(boundaryTerm(capture.Symbol)), closure.Values)
		if !found {
			return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q omitted capture cell %q", handle.Prototype, capture.Name)
		}
		projected.Values = append(projected.Values, equation.Fact{Key: "value/" + handle.Captures[index] + "/" + operation.Target.Name, Value: value})
	}
	// A returned lexical closure is a capability, not merely its callable
	// scalar. Preserve its environment lens under the call boundary so the
	// caller can rebind it to its result slot.
	if len(returns) == 1 && strings.HasPrefix(string(returns[0]), "scalar/function/") {
		for _, fact := range closure.Values {
			if strings.HasPrefix(fact.Key, "closure/") {
				var returned closureHandle
				if json.Unmarshal(fact.Value, &returned) != nil {
					return equation.TransactionResult{}, fmt.Errorf("engine: malformed returned closure handle")
				}
				// Rebind captures of an escaping closure through this call's
				// boundary lens. Formal captures therefore retain the caller cell
				// instead of a child-local path that disappears at return.
				for captureIndex, captureTerm := range returned.Captures {
					for parameterIndex, parameter := range child.Boundary.Parameters {
						if captureTerm == boundaryTerm(parameter.Symbol) {
							returned.Captures[captureIndex] = string(operands.arguments[parameterIndex])
							break
						}
					}
				}
				encoded, marshalErr := json.Marshal(returned)
				if marshalErr != nil {
					return equation.TransactionResult{}, marshalErr
				}
				projected.Values = append(projected.Values, equation.Fact{Key: "call-closure/" + strings.TrimPrefix(operation.Target.Name, "call/") + "/00000000", Value: encoded})
				break
			}
		}
	}
	// Child diagnostics are retained in the private outcome. They need a
	// body-qualified descriptor-to-span projection before becoming root facts;
	// publishing them here would attach a child coordinate to an unrelated
	// caller span.
	return equation.TransactionResult{Complete: true, Closure: projected}, nil
}

func latestClosedValue(term []byte, facts []equation.Fact) ([]byte, bool) {
	prefix := "value/" + string(term) + "/"
	var value []byte
	latest := ""
	for _, fact := range facts {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latest) {
			value, latest = fact.Value, fact.Key
		}
	}
	return append([]byte(nil), value...), value != nil
}

func childReturnValues(closure equation.OutputClosure) ([][]byte, error) {
	var prefix string
	for _, outcome := range closure.Outcomes {
		if strings.HasPrefix(outcome.Key, "return-candidate/") && strings.HasSuffix(outcome.Key, "/arity") {
			candidate := strings.TrimSuffix(outcome.Key, "/arity") + "/"
			if prefix != "" && prefix != candidate {
				return nil, fmt.Errorf("multiple child return alternatives")
			}
			if _, err := strconv.Atoi(string(outcome.Value)); err != nil {
				return nil, fmt.Errorf("malformed child return arity")
			}
			prefix = candidate
		}
	}
	if prefix == "" {
		// A body with no return statement has a complete zero-result outcome.
		return nil, nil
	}
	values := map[int][]byte{}
	for _, outcome := range closure.Outcomes {
		if !strings.HasPrefix(outcome.Key, prefix) {
			continue
		}
		indexText := strings.TrimPrefix(outcome.Key, prefix)
		if indexText == "arity" {
			continue
		}
		index, err := strconv.Atoi(indexText)
		if err != nil || index < 0 || values[index] != nil {
			return nil, fmt.Errorf("malformed child return slot")
		}
		values[index] = append([]byte(nil), outcome.Value...)
	}
	result := make([][]byte, len(values))
	for index := range result {
		if values[index] == nil {
			return nil, fmt.Errorf("incomplete child return")
		}
		result[index] = values[index]
	}
	return result, nil
}

// allocationTemplateKernel admits only a sealed, complete allocation graph.
// Its identity transport is structural at this walking stage; it deliberately
// does not turn an absent/nil member into a value fact.
func allocationTemplateKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "site", "result", "kind"); err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true}, nil
}

// objectMaterializationKernel runs only after its template dependency.  The
// current engine retains no heap state, but validates the sealed identity and
// object-kind surface so unsupported materialization cannot pass as a hidden
// fallback transaction.
func objectMaterializationKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "site", "result", "kind"); err != nil {
		return equation.TransactionResult{}, err
	}
	if lexical == nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: missing lexical resolver")
	}
	var prototype, result string
	captures := make([]string, 0)
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "prototype":
			if prototype != "" || !strings.HasPrefix(string(operand.Value), "prototype/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed closure prototype")
			}
			prototype = strings.TrimPrefix(string(operand.Value), "prototype/")
		case operand.Role == "result":
			result = string(operand.Value)
		case strings.HasPrefix(operand.Role, "capture-"):
			captures = append(captures, string(operand.Value))
		}
	}
	if prototype == "" {
		return equation.TransactionResult{Complete: true}, nil
	}
	if result == "" || (!strings.HasPrefix(result, "path/") && !strings.HasPrefix(result, "temp/")) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed closure result")
	}
	if _, exists := lexical.byPrototype[prototype]; !exists {
		// A syntactically known lexical closure with no admitted catalog body is
		// not an unknown dynamic call.  Refusing it is the fail-closed boundary.
		return equation.TransactionResult{}, fmt.Errorf("engine: known lexical prototype %q is not admitted", prototype)
	}
	handle, err := json.Marshal(closureHandle{Prototype: prototype, Captures: captures})
	if err != nil {
		return equation.TransactionResult{}, err
	}
	closure := equation.OutputClosure{Values: []equation.Fact{{Key: "closure/" + result + "/" + operation.Target.Name, Value: handle}}}
	child := lexical.byPrototype[prototype]
	// A parameter-free, capture-free child has a complete diagnostic entry at
	// allocation time. Demand it privately and qualify its facts by body before
	// they enter the root publication closure.
	if len(child.Boundary.Parameters) == 0 && len(child.Boundary.Captures) == 0 && child.Cyclic == nil && len(child.Artifact.Equations) <= 4 {
		entry, entryErr := encodeChildEntry(nil)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		outcome, _, evaluateErr := lexical.evaluate(child, entry)
		if evaluateErr != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: uncalled lexical child %q: %w", prototype, evaluateErr)
		}
		body := fmt.Sprintf("%x", child.Body)
		for _, diagnostic := range outcome.Diagnostics {
			key := "child/" + body + "/" + diagnostic.Key
			closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
			spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, outcome.Diagnostics)
			if span, ok := spans[diagnostic.Key]; ok {
				lexical.diagnosticSpans[key] = span
			}
		}
	}
	return equation.TransactionResult{Complete: true, Closure: closure}, nil
}

func writeKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := operandsByRole(operation.Operands, "target", "display", "value", "read-before", "absence")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	target := string(operands["target"])
	if !strings.HasPrefix(target, "path/") && !strings.HasPrefix(target, "temp/") {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed assignment target")
	}
	value, err := resolveValue(operands["value"], operands["read-before"], operands["absence"], partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	value, err = sealShapeValue(value, partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	values := []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: value}}
	if handle, ok := closureHandleFor(operands["value"], partition); ok {
		encoded, marshalErr := json.Marshal(handle)
		if marshalErr != nil {
			return equation.TransactionResult{}, marshalErr
		}
		values = append(values, equation.Fact{Key: "closure/" + target + "/" + operation.Target.Name, Value: encoded})
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

// sealShapeValue resolves a literal's members when that literal is made. This
// prevents a later source-path write from retroactively changing its fact.
func sealShapeValue(value []byte, partition equation.Partition) ([]byte, error) {
	table, ok := shapefact.DecodeTable(value)
	if !ok {
		return append([]byte(nil), value...), nil
	}
	for index := range table.Members {
		member := &table.Members[index]
		if !member.Present {
			continue
		}
		resolved, err := resolveCurrentValue([]byte(member.Value), partition)
		if err != nil {
			member.Value = "scalar/top"
			continue
		}
		resolved, err = sealShapeValue(resolved, partition)
		if err != nil {
			return nil, err
		}
		member.Value = string(resolved)
	}
	sealed, ok := shapefact.EncodeTable(table)
	if !ok {
		return nil, fmt.Errorf("engine: malformed table shape")
	}
	return sealed, nil
}

func expressionKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	by := map[string][]byte{}
	for _, operand := range operation.Operands {
		by[operand.Role] = operand.Value
	}
	result, ok := by["result"]
	if !ok || (!strings.HasPrefix(string(result), "temp/") && !strings.HasPrefix(string(result), "path/")) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed expression result")
	}
	kind, e1 := strconv.Atoi(string(by["kind"]))
	op, e2 := strconv.Atoi(string(by["operator"]))
	if e1 != nil || e2 != nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed expression")
	}
	resolve := func(role string) ([]byte, error) {
		value, ok := by[role]
		if !ok {
			return nil, fmt.Errorf("engine: missing expression operand %q", role)
		}
		return resolveCurrentValue(value, partition)
	}
	var value []byte
	var err error
	switch wir.Op(kind) {
	case wir.OpLogical:
		left, er := resolve("left")
		if er != nil {
			return equation.TransactionResult{}, er
		}
		if string(left) == "scalar/top" {
			value = left
			break
		}
		truth, er := luaTruthy(left)
		if er != nil {
			return equation.TransactionResult{}, er
		}
		if (wir.Operator(op) == wir.LogAnd && !truth) || (wir.Operator(op) == wir.LogOr && truth) {
			value = left
		} else {
			value, err = resolve("right")
		}
	case wir.OpUnOp:
		operand, er := resolve("value")
		if er != nil {
			return equation.TransactionResult{}, er
		}
		if string(operand) == "scalar/top" {
			value = operand
			break
		}
		switch wir.Operator(op) {
		case wir.UnNot:
			truth, er := luaTruthy(operand)
			err = er
			value = []byte("scalar/bool/" + strconv.FormatBool(!truth))
		case wir.UnLen:
			n, er := scalarLength(operand)
			err = er
			value = []byte("scalar/number/" + strconv.FormatInt(n, 10))
		case wir.UnNeg:
			n, er := scalarNumber(operand)
			err = er
			value = numberValue(-n)
		default:
			err = fmt.Errorf("engine: unsupported unary operator")
		}
	case wir.OpConcat:
		var b strings.Builder
		for i := 0; ; i++ {
			term, found := by[fmt.Sprintf("value-%08d", i)]
			if !found {
				if i < 2 {
					err = fmt.Errorf("engine: missing concat operand")
				}
				break
			}
			v, er := resolveCurrentValue(term, partition)
			if er != nil {
				err = er
				break
			}
			if string(v) == "scalar/top" {
				value = v
				break
			}
			var s string
			if strings.HasPrefix(string(v), "scalar/string/") {
				s, er = scalarString(v)
			} else if strings.HasPrefix(string(v), "scalar/number/") {
				s = strings.TrimPrefix(string(v), "scalar/number/")
			} else {
				value = []byte("scalar/top")
				break
			}
			if er != nil {
				err = er
				break
			}
			b.WriteString(s)
		}
		if value == nil && err == nil {
			value = []byte("scalar/string/" + strconv.Quote(b.String()))
		}
	case wir.OpBinOp:
		left, er := resolve("left")
		if er != nil {
			return equation.TransactionResult{}, er
		}
		right, er := resolve("right")
		if er != nil {
			return equation.TransactionResult{}, er
		}
		if string(left) == "scalar/top" || string(right) == "scalar/top" {
			value = []byte("scalar/top")
		} else {
			value, err = basicBinary(wir.Operator(op), left, right)
		}
	default:
		err = fmt.Errorf("engine: unsupported expression kind")
	}
	if err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: value}}}}, nil
}

func numberValue(v float64) []byte {
	return []byte("scalar/number/" + strconv.FormatFloat(v, 'g', -1, 64))
}
func basicBinary(op wir.Operator, a, b []byte) ([]byte, error) {
	switch op {
	case wir.BinEq:
		return []byte("scalar/bool/" + strconv.FormatBool(bytes.Equal(a, b))), nil
	case wir.BinNe:
		return []byte("scalar/bool/" + strconv.FormatBool(!bytes.Equal(a, b))), nil
	}
	x, e := scalarNumber(a)
	if e != nil {
		return []byte("scalar/top"), nil
	}
	y, e := scalarNumber(b)
	if e != nil {
		return []byte("scalar/top"), nil
	}
	switch op {
	case wir.BinAdd:
		return numberValue(x + y), nil
	case wir.BinSub:
		return numberValue(x - y), nil
	case wir.BinMul:
		return numberValue(x * y), nil
	case wir.BinDiv:
		return numberValue(x / y), nil
	case wir.BinIDiv:
		return numberValue(math.Floor(x / y)), nil
	case wir.BinMod:
		return numberValue(x - math.Floor(x/y)*y), nil
	case wir.BinPow:
		return numberValue(math.Pow(x, y)), nil
	case wir.BinLt:
		return []byte("scalar/bool/" + strconv.FormatBool(x < y)), nil
	case wir.BinLe:
		return []byte("scalar/bool/" + strconv.FormatBool(x <= y)), nil
	case wir.BinGt:
		return []byte("scalar/bool/" + strconv.FormatBool(x > y)), nil
	case wir.BinGe:
		return []byte("scalar/bool/" + strconv.FormatBool(x >= y)), nil
	default:
		return []byte("scalar/top"), nil
	}
}

// The path-store families are admitted whole-file operations. Their current
// walking semantics validate the complete sealed shape and preserve ordering;
// they intentionally do not manufacture heap facts that have not yet been
// modeled by the equation domain.
func pathReplacementKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "target", "display", "value")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if !strings.HasPrefix(string(operands["target"]), "path/") || len(operands["display"]) == 0 || len(operands["value"]) == 0 {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed path replacement")
	}
	value, err := resolveCurrentValue(operands["value"], partition)
	if err != nil {
		value = []byte("scalar/top")
	}
	result, err := frozenMutationDiagnostic(operation, partition, "assignment")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	result.Closure.Values = append(result.Closure.Values, equation.Fact{
		Key: "value/" + string(operands["target"]) + "/" + operation.Target.Name, Value: value,
	})
	return result, nil
}

// dynamicIndexReadKernel has no closed heap fact to project at this stage.
// It nevertheless publishes an explicit Top for the result, so a dynamic read
// never masquerades as an absent value and never selects a branch by accident.
func dynamicIndexReadKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "target", "container", "key")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	target := string(operands["target"])
	if (!strings.HasPrefix(target, "path/") && !strings.HasPrefix(target, "temp/")) || len(operands["container"]) == 0 || len(operands["key"]) == 0 {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed dynamic index read")
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
		Key: "value/" + target + "/" + operation.Target.Name, Value: []byte("scalar/top"),
	}}}}, nil
}

// claimKernel makes user claims explicit checked refinements. An unproven
// claim remains a downstream assumption but never becomes reusable proof.
func claimKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "target", "value", "kind", "type")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	target, source, kind, targetType := string(operands["target"]), operands["value"], string(operands["kind"]), string(operands["type"])
	display := strings.TrimPrefix(target, "path/")
	for _, operand := range operation.Operands {
		if operand.Role == "display" && len(operand.Value) != 0 {
			display = string(operand.Value)
			break
		}
	}
	if (!strings.HasPrefix(target, "path/") && !strings.HasPrefix(target, "temp/")) || !validClaimKind(kind) || !validClaimType(kind, targetType) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed claim")
	}
	value, available, err := resolveClaimValue(source, partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	shapeRelation := shapeUnknown
	for _, operand := range operation.Operands {
		if operand.Role == "shape-target" {
			shapeRelation = assignmentShapeRelation(value, operand.Value)
			break
		}
	}
	// A sealed table whose declared record includes callable members retains a
	// concrete dispatch witness even when function-variance proof is outside
	// this scalar relation. Do not turn that missing compatibility proof into a
	// diagnostic; apply can still emit only a proven call-contract violation.
	callableRecordShape := shapefact.IsTable(value) && strings.Contains(targetType, "fun(")
	if available && (claimProven(value, kind, targetType) || shapeRelation == shapeProven) {
		closure := equation.OutputClosure{Values: []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: value}}}
		if kind == "claim-kind/1" {
			closure.Diagnostics = []equation.Fact{{Key: "advice.redundant_claim/" + operation.Target.Name, Value: []byte("proven runtime claim")}}
		}
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	refined := []byte("scalar/claim/" + kind + "/" + targetType)
	closure := equation.OutputClosure{Values: []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: refined}}}
	// An annotation is an assignment contract.  Only a concrete scalar that
	// the equation has already derived can refute it; top and refinements stay
	// unproven and deliberately publish no assignment failure.  This keeps the
	// diagnostic in the operation that owns both the guard and abstract value.
	sourceDisplay := display
	for _, operand := range operation.Operands {
		if operand.Role == "source-display" && len(operand.Value) != 0 {
			sourceDisplay = string(operand.Value)
			break
		}
	}
	anySource := isExplicitAnyValue(value) || sourceHasExplicitAny(source, partition.Values())
	if kind == "claim-kind/3" && available && (anySource && assignmentTargetRequiresProof(targetType) || assignmentMismatchProven(value, targetType) || shapeRelation == shapeRefuted) {
		message := assignmentMismatchMessage(sourceDisplay, value, targetType)
		if anySource {
			message = assignmentAnyMismatchMessage(sourceDisplay, targetType)
		}
		closure.Diagnostics = []equation.Fact{{
			Key:   "type.assignment/" + operation.Target.Name,
			Value: []byte(message),
		}}
	} else if !claimTypeIsAny(targetType) && !callableRecordShape {
		// The closure keys facts by identity, so separate unproven claims must
		// retain their operation identity.
		closure.Diagnostics = []equation.Fact{{Key: "claim/unproven/" + operation.Target.Name, Value: []byte("claim " + strings.TrimPrefix(targetType, "claim-type/") + " is not proven")}}
	}
	return equation.TransactionResult{Complete: true, Closure: closure}, nil
}

type shapeRelation uint8

const (
	shapeUnknown shapeRelation = iota
	shapeProven
	shapeRefuted
)

// assignmentShapeRelation is deliberately proof-oriented: a malformed or
// unsupported target/type member is unknown, never a compatibility result.
func assignmentShapeRelation(value, encodedTarget []byte) shapeRelation {
	target, ok := shapefact.DecodeTarget(encodedTarget)
	if !ok {
		return shapeUnknown
	}
	return valueAgainstType(value, target)
}

func valueAgainstType(value []byte, target typ.Type) shapeRelation {
	if target == nil || isUnknownScalar(value) {
		return shapeUnknown
	}
	target = unwrap.Alias(target)
	if target == nil {
		return shapeUnknown
	}
	switch target.Kind() {
	case kind.Optional:
		optional, ok := target.(*typ.Optional)
		if !ok || optional.Inner == nil {
			return shapeUnknown
		}
		if string(value) == "scalar/nil" {
			return shapeProven
		}
		return valueAgainstType(value, optional.Inner)
	case kind.Union:
		union, ok := target.(*typ.Union)
		if !ok || len(union.Members) == 0 {
			return shapeUnknown
		}
		unknown := false
		for _, member := range union.Members {
			relation := valueAgainstType(value, member)
			if relation == shapeProven {
				return shapeProven
			}
			unknown = unknown || relation == shapeUnknown
		}
		if unknown {
			return shapeUnknown
		}
		return shapeRefuted
	case kind.Intersection:
		intersection, ok := target.(*typ.Intersection)
		if !ok || len(intersection.Members) == 0 {
			return shapeUnknown
		}
		unknown := false
		for _, member := range intersection.Members {
			relation := valueAgainstType(value, member)
			if relation == shapeRefuted {
				return shapeRefuted
			}
			unknown = unknown || relation == shapeUnknown
		}
		if unknown {
			return shapeUnknown
		}
		return shapeProven
	case kind.Record:
		return tableAgainstRecord(value, target)
	case kind.Nil:
		return scalarRelation(value, func() bool { return string(value) == "scalar/nil" })
	case kind.Boolean:
		return scalarRelation(value, func() bool { return strings.HasPrefix(string(value), "scalar/bool/") })
	case kind.String:
		return scalarRelation(value, func() bool { return strings.HasPrefix(string(value), "scalar/string/") })
	case kind.Number:
		return scalarRelation(value, func() bool { return strings.HasPrefix(string(value), "scalar/number/") })
	case kind.Integer:
		if !strings.HasPrefix(string(value), "scalar/number/") {
			return knownScalarRelation(value, false)
		}
		_, err := strconv.ParseInt(strings.TrimPrefix(string(value), "scalar/number/"), 10, 64)
		return scalarRelation(value, func() bool { return err == nil })
	case kind.Literal:
		literal, ok := target.(*typ.Literal)
		if !ok {
			return shapeUnknown
		}
		return scalarRelation(value, func() bool { return string(value) == literalValue(literal) })
	default:
		return shapeUnknown
	}
}

func tableAgainstRecord(value []byte, target typ.Type) shapeRelation {
	table, ok := shapefact.DecodeTable(value)
	if !ok {
		if string(value) == "scalar/table" {
			return shapeUnknown
		}
		return knownScalarRelation(value, false)
	}
	record, ok := unwrap.Alias(target).(*typ.Record)
	if !ok {
		return shapeUnknown
	}
	unknown := false
	for _, field := range record.Fields {
		member, found := table.Lookup("." + field.Name)
		if !found || !member.Present {
			if field.Optional {
				continue
			}
			if table.Closed {
				return shapeRefuted
			}
			unknown = true
			continue
		}
		relation := valueAgainstType([]byte(member.Value), field.Type)
		if relation == shapeRefuted {
			return shapeRefuted
		}
		unknown = unknown || relation == shapeUnknown
	}
	if unknown {
		return shapeUnknown
	}
	return shapeProven
}

func scalarRelation(value []byte, compatible func() bool) shapeRelation {
	return knownScalarRelation(value, compatible())
}

func knownScalarRelation(value []byte, compatible bool) shapeRelation {
	if shapefact.IsTable(value) || string(value) == "scalar/table" || !strings.HasPrefix(string(value), "scalar/") || isUnknownScalar(value) {
		return shapeUnknown
	}
	if compatible {
		return shapeProven
	}
	return shapeRefuted
}

func literalValue(literal *typ.Literal) string {
	if literal == nil {
		return ""
	}
	switch literal.Base {
	case kind.Boolean:
		return "scalar/bool/" + literal.String()
	case kind.Integer, kind.Number:
		return "scalar/number/" + literal.String()
	case kind.String:
		return "scalar/string/" + literal.String()
	default:
		return ""
	}
}

// assignmentMismatchProven is intentionally narrower than incompatibility in
// general.  The scalar lattice is the evidence available at this equation;
// when it says top or a prior claim, the relation is unknown rather than a
// proven failure.  Type-shape and richer subtype evidence can extend this
// predicate as those axes become available to this kernel.
func assignmentMismatchProven(value []byte, targetType string) bool {
	if isUnknownScalar(value) {
		return false
	}
	target, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil {
		return false
	}
	switch target {
	case "nil":
		return string(value) != "scalar/nil"
	case "boolean":
		return !strings.HasPrefix(string(value), "scalar/bool/") && string(value) != "scalar/boolean"
	case "string":
		return !strings.HasPrefix(string(value), "scalar/string/")
	case "number":
		return !strings.HasPrefix(string(value), "scalar/number/")
	case "integer":
		if !strings.HasPrefix(string(value), "scalar/number/") {
			return true
		}
		_, err := strconv.ParseInt(strings.TrimPrefix(string(value), "scalar/number/"), 10, 64)
		return err != nil
	default:
		return false
	}
}

func claimTypeIsAny(targetType string) bool {
	target, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	return err == nil && target == "any"
}

// Explicit any is a proven precision-boundary fact, unlike scalar/top (an
// unknown value).  It therefore fails a declared assignment contract only
// when that contract requires evidence the boundary cannot supply.
func isExplicitAnyValue(value []byte) bool {
	return strings.HasSuffix(string(value), "/\"any\"") && strings.HasPrefix(string(value), "scalar/claim/")
}

func sourceHasExplicitAny(source []byte, values []equation.Fact) bool {
	path := strings.TrimPrefix(string(source), "path/")
	if path == string(source) || path == "" {
		return false
	}
	for {
		prefix := "value/path/" + path + "/"
		for _, fact := range values {
			if strings.HasPrefix(fact.Key, prefix) && isExplicitAnyValue(fact.Value) {
				return true
			}
		}
		cut := strings.LastIndexAny(path, ".[")
		if cut < 0 {
			return false
		}
		path = path[:cut]
	}
}

func assignmentTargetRequiresProof(targetType string) bool {
	if claimTypeIsAny(targetType) {
		return false
	}
	target, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	return err == nil && target != ""
}

func assignmentAnyMismatchMessage(source string, targetType string) string {
	declared, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil {
		declared = strings.TrimPrefix(targetType, "claim-type/")
	}
	if strings.HasPrefix(declared, "{") {
		return "cannot assign " + source + " because " + source + " comes from any/unknown; no proof shows it satisfies the declared type"
	}
	return "cannot assign " + source + " because it is any, not " + declared
}

func assignmentMismatchMessage(target string, value []byte, targetType string) string {
	declared, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil {
		declared = strings.TrimPrefix(targetType, "claim-type/")
	}
	return "cannot assign " + target + " because it is " + assignmentValueType(value) + ", not " + declared
}

func assignmentValueType(value []byte) string {
	switch {
	case shapefact.IsTable(value):
		return "table"
	case string(value) == "scalar/nil":
		return "nil"
	case strings.HasPrefix(string(value), "scalar/bool/"):
		return "boolean"
	case strings.HasPrefix(string(value), "scalar/string/"):
		return "string"
	case strings.HasPrefix(string(value), "scalar/number/"):
		return "number"
	case string(value) == "scalar/table":
		return "table"
	case string(value) == "scalar/function":
		return "function"
	default:
		return "unknown"
	}
}

func validClaimKind(kind string) bool {
	return kind == "claim-kind/1" || kind == "claim-kind/2" || kind == "claim-kind/3" || kind == "claim-kind/4"
}

func validClaimType(kind, targetType string) bool {
	if kind == "claim-kind/2" {
		return targetType == "claim-type/non-nil"
	}
	return strings.HasPrefix(targetType, "claim-type/\"") && len(strings.TrimPrefix(targetType, "claim-type/")) > 2
}

func resolveClaimValue(term []byte, partition equation.Partition) ([]byte, bool, error) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), true, nil
	}
	if shapefact.IsTable(term) {
		return append([]byte(nil), term...), true, nil
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, false, fmt.Errorf("engine: unsupported claim value %q", term)
	}
	value, err := resolveCurrentValue(term, partition)
	if err != nil {
		return nil, false, nil
	}
	return value, true, nil
}

func claimProven(value []byte, kind, targetType string) bool {
	if strings.HasPrefix(string(value), "scalar/claim/") || string(value) == "scalar/top" {
		return false
	}
	if kind == "claim-kind/2" {
		return string(value) != "scalar/nil"
	}
	typeName, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil {
		return false
	}
	switch typeName {
	case "nil":
		return string(value) == "scalar/nil"
	case "boolean":
		return strings.HasPrefix(string(value), "scalar/bool/") || string(value) == "scalar/boolean"
	case "string":
		return strings.HasPrefix(string(value), "scalar/string/")
	case "number":
		return strings.HasPrefix(string(value), "scalar/number/")
	case "integer":
		if !strings.HasPrefix(string(value), "scalar/number/") {
			return false
		}
		_, err := strconv.ParseInt(strings.TrimPrefix(string(value), "scalar/number/"), 10, 64)
		return err == nil
	default:
		return false
	}
}

func pathInvalidationKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "container", "key", "suffix"); err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true}, nil
}

// frozenMutationDiagnostic is deliberately fact-only: a prior freeze epoch,
// or the true edge of table.isfrozen, is the complete proof. It never turns an
// unknown heap value or a merely reachable freeze into a violation.
func frozenMutationDiagnostic(operation equation.BoundEquation, partition equation.Partition, action string) (equation.TransactionResult, error) {
	var subject, display []byte
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "freeze-subject":
			subject = operand.Value
		case "freeze-display":
			display = operand.Value
		}
	}
	if !strings.HasPrefix(string(subject), "path/") || len(display) == 0 {
		return equation.TransactionResult{Complete: true}, nil
	}
	freeze, guarded := frozenProof(operation, subject, partition)
	if freeze == "" && !guarded {
		return equation.TransactionResult{Complete: true}, nil
	}
	proof := freeze
	if guarded {
		proof = "guard"
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
		Key:   "effect.freeze.mutation/" + operation.Target.Name + "/" + proof,
		Value: []byte(fmt.Sprintf("cannot mutate frozen table %q", display)),
	}}}}, nil
}

// frozenProof observes only facts already published into the partition. A
// guarded epoch is usable on its true edge; complementary guarded epochs prove
// a post-join freeze without treating either arm as globally executed.
func frozenProof(operation equation.BoundEquation, subject []byte, partition equation.Partition) (string, bool) {
	prefix := "effect.freeze/" + strings.TrimPrefix(string(subject), "path/") + "/"
	seen := make(map[string]map[string]string)
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		op := strings.TrimPrefix(fact.Key, prefix)
		if string(fact.Value) == "unconditional" {
			return op, false
		}
		parts := strings.Split(string(fact.Value), "/")
		if len(parts) == 3 && parts[0] == "guard" && (parts[2] == "true" || parts[2] == "false") {
			if seen[parts[1]] == nil {
				seen[parts[1]] = make(map[string]string)
			}
			seen[parts[1]][parts[2]] = op
		}
	}
	for _, guard := range operation.Guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) == 4 && parts[0] == "front" && parts[1] == "branch" && parts[3] == "true" && hasOutcome(partition, "frozen-branch/"+parts[2]) {
			return "", true
		}
	}
	for _, arms := range seen {
		if arms["true"] != "" && arms["false"] != "" {
			return arms["false"], false
		}
	}
	return "", false
}

func hasOutcome(partition equation.Partition, key string) bool {
	for _, item := range partition.Outcomes() {
		if item.Key == key && string(item.Value) == "proven" {
			return true
		}
	}
	return false
}

func indexMutationKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "container", "key", "suffix", "value"); err != nil {
		return equation.TransactionResult{}, err
	}
	return frozenMutationDiagnostic(operation, partition, "assignment")
}

func genericForKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "iterator", "state", "control"); err != nil {
		return equation.TransactionResult{}, err
	}
	values := make([]equation.Fact, 0)
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "result-") {
			continue
		}
		result := string(operand.Value)
		if !strings.HasPrefix(result, "path/") {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed generic-for result %q", operand.Role)
		}
		values = append(values, equation.Fact{Key: "value/" + result + "/" + operation.Target.Name, Value: []byte("scalar/top")})
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

func channelSelectKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "result", "default")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if !strings.HasPrefix(string(operands["result"]), "value/temp/") ||
		(!strings.EqualFold(string(operands["default"]), "select/default/true") && !strings.EqualFold(string(operands["default"]), "select/default/false")) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed channel select")
	}
	return equation.TransactionResult{Complete: true}, nil
}

func branchKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	frozenCondition := false
	for _, operand := range operation.Operands {
		if operand.Role == "predicate" && frozenPredicate(operand.Value) {
			frozenCondition = true
			break
		}
	}
	for _, operand := range operation.Operands {
		if operand.Role != "condition" {
			continue
		}
		value, err := resolveCurrentValue(operand.Value, partition)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		if isUnknownScalar(value) && !frozenCondition {
			return equation.TransactionResult{Complete: true}, nil
		}
	}
	truth, frozenGuard, err := branchTruth(operation.Operands, partition)
	if errors.Is(err, errUnknownScalar) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if err != nil {
		return equation.TransactionResult{}, err
	}
	edge := strconv.FormatBool(truth)
	narrowing := "falsy"
	if truth {
		narrowing = "truthy"
	}
	closure := equation.OutputClosure{Outcomes: []equation.Fact{
		{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge)},
		{Key: "narrowing/" + operation.Target.Name, Value: []byte(narrowing)},
	}}
	if frozenGuard && truth {
		closure.Outcomes = append(closure.Outcomes, equation.Fact{Key: "frozen-branch/" + operation.Target.Name, Value: []byte("proven")})
	}
	if truth {
		closure.Diagnostics = []equation.Fact{{Key: "advice.always_true_guard/" + operation.Target.Name, Value: []byte("proven constant guard")}}
	}
	if len(operation.Guards) != 0 {
		closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: "lint.condition.redundant/" + operation.Target.Name, Value: []byte(strconv.FormatBool(truth))})
	}
	return equation.TransactionResult{Complete: true, Closure: closure}, nil
}

// applyKernel validates the sealed direct or method-call shape and publishes
// proven call-contract failures at this equation point. Unknown values are not
// violations: diagnostics are proof outputs, never speculative findings.
func applyKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if closure, recognized, err := freezeCallEpoch(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	hasCallee, hasReceiver, hasMethod := false, false, false
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "callee":
			hasCallee = true
		case "receiver":
			hasReceiver = true
		case "method":
			hasMethod = true
		}
	}
	if hasCallee == (hasReceiver && hasMethod) || hasReceiver != hasMethod {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed call application shape")
	}
	operands, err := callOperands(operation.Operands)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if lexical != nil && hasCallee {
		if handle, found := closureHandleFor(operands.callee, partition); found {
			if lexical.requiresBody[handle.Prototype] {
				if outcome, err := lexical.applyKnown(operation, operands, handle, partition); err == nil {
					return outcome, nil
				} else if strings.Contains(err.Error(), "unsupported exact lexical boundary") {
					// A missing certified selector is an admission failure: preserve
					// atomicity rather than exposing a partial child result.
					return equation.TransactionResult{}, err
				}
			}
			// The existing root rule remains the conservative residue for a
			// boundary that this compact bridge cannot yet certify. In particular,
			// no incomplete child closure is published.
		}
	}
	if state, handled := sendIsolationState(operation, operands, partition); handled {
		return state, nil
	}
	if operands.display == "table.isfrozen" {
		return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
			Key: "effect.call-bool/" + operation.Target.Name, Value: []byte("scalar/boolean"),
		}}}}, nil
	}
	if operands.display == "table.insert" && len(operands.arguments) != 0 && strings.HasPrefix(string(operands.arguments[0]), "path/") {
		display := callArgumentDisplay(operation.Operands, 0)
		if display != "" {
			freeze, guarded := frozenProof(operation, operands.arguments[0], partition)
			if freeze != "" || guarded {
				proof := freeze
				if guarded {
					proof = "guard"
				}
				return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
					Key:   "effect.freeze.mutation/" + operation.Target.Name + "/" + proof,
					Value: []byte(fmt.Sprintf("cannot call mutator on frozen table %q", display)),
				}}}}, nil
			}
		}
	}
	if operands.display == "string" && !operands.spread && len(operands.arguments) == 1 {
		if argument, known := resolveKnownCurrentValue(operands.arguments[0], partition); known && strings.HasPrefix(string(argument), "scalar/string/") {
			return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{Key: "advice.redundant_claim/" + operation.Target.Name, Value: []byte("proven string cast")}}}}, nil
		}
	}
	callee, known := resolveKnownCurrentValue(operands.callee, partition)
	if !hasCallee {
		receiver, receiverKnown := resolveKnownCurrentValue(operands.receiver, partition)
		if !receiverKnown {
			return equation.TransactionResult{Complete: true}, nil
		}
		callee, known = currentMethodCallable(operands.receiver, receiver, operands.method, partition)
		if !known && isClaimRefinement(receiver) {
			// An unproven annotation is a downstream assumption, not a new
			// runtime value. Walk only its contiguous predecessor claims back
			// to the immediately underlying sealed shape; a real intervening
			// write stops dispatch and therefore cannot create a stale proof.
			if sealed, found := priorSealedTableValue(operands.receiver, partition); found {
				callee, known = currentMethodCallable(operands.receiver, sealed, operands.method, partition)
			}
		}
		if !known {
			return equation.TransactionResult{Complete: true}, nil
		}
	}
	if !known {
		return equation.TransactionResult{Complete: true}, nil
	}
	if !isUnknownScalar(callee) && !isCallableValue(callee) {
		return callDiagnostic(operation, "not_callable", "callee", fmt.Sprintf("%s is %s, not callable", operands.display, callDisplayValue(callee))), nil
	}
	signature, known := callableSignature(callee)
	if !known || operands.spread {
		return equation.TransactionResult{Complete: true}, nil
	}
	if !hasCallee {
		// Lua colon calls always pass the receiver in the first position. Once
		// dispatch has proved the method member, the remaining signature is
		// the exact explicit-argument contract at this call site.
		if len(signature.Params) > 0 {
			signature.Params = signature.Params[1:]
		}
		if signature.Required > 0 {
			signature.Required--
		}
	}
	if len(operands.arguments) < signature.Required {
		return callDiagnostic(operation, "too_few_args", "call", fmt.Sprintf("%s expects %d arguments, got %d", operands.display, signature.Required, len(operands.arguments))), nil
	}
	if !signature.Variadic && len(operands.arguments) > len(signature.Params) {
		return callDiagnostic(operation, "too_many_args", "call", fmt.Sprintf("%s expects %d arguments, got %d", operands.display, len(signature.Params), len(operands.arguments))), nil
	}
	for index, term := range operands.arguments {
		if index >= len(signature.Params) {
			break
		}
		argument, known := resolveKnownCurrentValue(term, partition)
		if !known || !provenScalarNotSubtype(argument, signature.Params[index]) {
			continue
		}
		return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValue(argument), signature.Params[index])), nil
	}
	return equation.TransactionResult{Complete: true}, nil
}

// freezeCallEpoch captures only closed root-table identities. It publishes the
// existing send-isolation state only on the same selected path as the prior
// send-isolation implementation, so a guarded epoch cannot become an
// unconditional send proof.
func freezeCallEpoch(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	callee, subject := "", []byte(nil)
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "callee-display":
			callee = string(operand.Value)
		case "argument-00000000":
			subject = operand.Value
		}
	}
	if callee != "table.freeze" {
		return equation.OutputClosure{}, false, nil
	}
	closure := equation.OutputClosure{}
	if guardsHold(operation.Guards, partition) && len(subject) != 0 {
		closure = isolationStateFact(isolationFrozenPrefix, subject).Closure
	}
	if !strings.HasPrefix(string(subject), "path/") {
		return closure, true, nil
	}
	value := "unconditional"
	if len(operation.Guards) == 1 {
		parts := strings.Split(string(operation.Guards[0].Encoding), "/")
		if len(parts) == 4 && parts[0] == "front" && parts[1] == "branch" && (parts[3] == "true" || parts[3] == "false") {
			value = "guard/" + parts[2] + "/" + parts[3]
		} else {
			return closure, true, nil
		}
	} else if len(operation.Guards) != 0 {
		return closure, true, nil
	}
	closure.Values = append(closure.Values, equation.Fact{
		Key:   "effect.freeze/" + strings.TrimPrefix(string(subject), "path/") + "/" + operation.Target.Name,
		Value: []byte(value),
	})
	return closure, true, nil
}

func callArgumentDisplay(operands []equation.BoundOperand, index int) string {
	want := fmt.Sprintf("argument-display-%08d", index)
	for _, operand := range operands {
		if operand.Role == want {
			return string(operand.Value)
		}
	}
	return ""
}

const sendIsolationHelp = "No checker error is emitted by default; unknown send-safety uses the runtime copy fallback."

// sendIsolationState is deliberately a small extension of the existing apply
// equation. It records only closed, syntactic boundary facts (freeze/store),
// then lets the send application publish a judgement from those facts. There
// is no source-side second pass and unknown aliasing remains a copy fallback.
func sendIsolationState(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) (equation.TransactionResult, bool) {
	if len(operands.arguments) == 0 {
		return equation.TransactionResult{}, false
	}
	switch operands.display {
	case "table.freeze":
		return isolationStateFact(isolationFrozenPrefix, operands.arguments[0]), true
	case "ownership.store":
		return isolationStateFact(isolationEscapedPrefix, operands.arguments[0]), true
	case "process.send":
		if operands.spread || len(operands.arguments) < 3 {
			return equation.TransactionResult{Complete: true}, true
		}
		payload := operands.arguments[2]
		if isolationStatePresent(partition, isolationEscapedPrefix, payload) {
			return sendIsolationDiagnostic(operation, "send payload has a proven escaping alias; zero-copy transfer is rejected"), true
		}
		if isolationStatePresent(partition, isolationFrozenPrefix, payload) {
			return sendIsolationDiagnostic(operation, "send payload is proven immutable for zero-copy sharing"), true
		}
		value, known := resolveKnownCurrentValue(payload, partition)
		if strings.HasPrefix(string(payload), "temp/") && known && isIsolatedLiteral(value) {
			return sendIsolationDiagnostic(operation, "send payload is proven isolated for zero-copy transfer"), true
		}
		return sendIsolationDiagnostic(operation, "send payload is not proven isolated or immutable; runtime will copy"), true
	default:
		return equation.TransactionResult{}, false
	}
}

const (
	isolationFrozenPrefix  = "send.isolation.state/frozen/"
	isolationEscapedPrefix = "send.isolation.state/escaped/"
)

func isolationStateFact(prefix string, term []byte) equation.TransactionResult {
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
		Key: prefix + base64.RawURLEncoding.EncodeToString(term), Value: []byte("proven"),
	}}}}
}

func isolationStatePresent(partition equation.Partition, prefix string, term []byte) bool {
	key := prefix + base64.RawURLEncoding.EncodeToString(term)
	for _, fact := range partition.Values() {
		if fact.Key == key && string(fact.Value) == "proven" {
			return true
		}
	}
	return false
}

func sendIsolationDiagnostic(operation equation.BoundEquation, message string) equation.TransactionResult {
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
		Key: "send.isolation/" + operation.Target.Name, Value: []byte(message),
	}}}}
}

// isIsolatedLiteral recognizes only a sealed table whose complete graph is
// made of scalar leaves. A named local is intentionally not admitted: its
// alias set is not closed at the send operation.
func isIsolatedLiteral(value []byte) bool {
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return false
	}
	for _, member := range table.Members {
		if !member.Present || !isTransferScalar([]byte(member.Value)) {
			return false
		}
	}
	return true
}

func isTransferScalar(value []byte) bool {
	return string(value) == "scalar/nil" || strings.HasPrefix(string(value), "scalar/bool/") ||
		strings.HasPrefix(string(value), "scalar/string/") || strings.HasPrefix(string(value), "scalar/number/")
}

type directCallOperands struct {
	callee    []byte
	receiver  []byte
	method    string
	display   string
	arguments [][]byte
	spread    bool
}

func callOperands(operands []equation.BoundOperand) (directCallOperands, error) {
	result := directCallOperands{display: "target"}
	arguments := make(map[int][]byte)
	for _, operand := range operands {
		switch {
		case operand.Role == "callee":
			if result.callee != nil {
				return directCallOperands{}, fmt.Errorf("engine: duplicate call callee")
			}
			result.callee = operand.Value
		case operand.Role == "receiver":
			if result.receiver != nil {
				return directCallOperands{}, fmt.Errorf("engine: duplicate call receiver")
			}
			result.receiver = operand.Value
		case operand.Role == "method":
			if result.method != "" {
				return directCallOperands{}, fmt.Errorf("engine: duplicate call method")
			}
			name, ok := callMethodName(operand.Value)
			if !ok {
				return directCallOperands{}, fmt.Errorf("engine: malformed call method")
			}
			result.method = name
		case operand.Role == "callee-display":
			if result.display != "target" || len(operand.Value) == 0 {
				return directCallOperands{}, fmt.Errorf("engine: malformed call display")
			}
			result.display = string(operand.Value)
		case operand.Role == "list-spread":
			if string(operand.Value) == "scalar/bool/true" {
				result.spread = true
			} else if string(operand.Value) != "scalar/bool/false" {
				return directCallOperands{}, fmt.Errorf("engine: malformed call argument spread")
			}
		case strings.HasPrefix(operand.Role, "argument-display-"):
			continue
		case strings.HasPrefix(operand.Role, "argument-"):
			index, err := callArgumentIndex(operand.Role)
			if err != nil || arguments[index] != nil {
				return directCallOperands{}, fmt.Errorf("engine: malformed call argument role %q", operand.Role)
			}
			arguments[index] = operand.Value
		}
	}
	hasCallee := result.callee != nil
	hasMethod := result.receiver != nil && result.method != ""
	if hasCallee == hasMethod || (result.receiver != nil) != (result.method != "") {
		return directCallOperands{}, fmt.Errorf("engine: incomplete call dispatch")
	}
	result.arguments = make([][]byte, len(arguments))
	for index := range result.arguments {
		if arguments[index] == nil {
			return directCallOperands{}, fmt.Errorf("engine: non-contiguous call arguments")
		}
		result.arguments[index] = arguments[index]
	}
	return result, nil
}

func callMethodName(value []byte) (string, bool) {
	encoded := strings.TrimPrefix(string(value), "method/")
	if encoded == string(value) || encoded == "" {
		return "", false
	}
	name, err := strconv.Unquote(encoded)
	return name, err == nil && name != ""
}

// methodCallable extracts only an exact member from a sealed table shape.
// Unknown/non-table receivers may have a metatable or later mutation outside
// the fact model, so they deliberately yield no callable proof or diagnostic.
func methodCallable(receiver []byte, method string) ([]byte, bool) {
	table, ok := shapefact.DecodeTable(receiver)
	if !ok || method == "" {
		return nil, false
	}
	member, found := table.Lookup("." + method)
	if !found {
		return nil, false
	}
	if !member.Present {
		return []byte("scalar/nil"), true
	}
	return []byte(member.Value), true
}

// currentMethodCallable gives an explicit member write precedence over a
// constructor shape. A later method definition or assignment therefore
// invalidates the constructor's closed-absence proof before apply decides
// whether the member is callable.
func currentMethodCallable(receiverTerm, receiver []byte, method string, partition equation.Partition) ([]byte, bool) {
	if strings.HasPrefix(string(receiverTerm), "path/") {
		memberTerm := []byte(string(receiverTerm) + "." + method)
		if value, known := resolveKnownCurrentValue(memberTerm, partition); known {
			return value, true
		}
	}
	return methodCallable(receiver, method)
}

func isClaimRefinement(value []byte) bool {
	return strings.HasPrefix(string(value), "scalar/claim/")
}

// priorSealedTableValue follows only a contiguous sequence of claim outputs
// for one value term. A claim is a refinement of its preceding source fact;
// any non-claim output is a real value boundary and must itself be a sealed
// table before method dispatch can proceed.
func priorSealedTableValue(term []byte, partition equation.Partition) ([]byte, bool) {
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, false
	}
	prefix := "value/" + string(term) + "/"
	facts := make([]equation.Fact, 0)
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) {
			facts = append(facts, fact)
		}
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Key > facts[j].Key })
	if len(facts) < 2 || !isClaimRefinement(facts[0].Value) {
		return nil, false
	}
	for _, fact := range facts[1:] {
		if isClaimRefinement(fact.Value) {
			continue
		}
		if shapefact.IsTable(fact.Value) {
			return append([]byte(nil), fact.Value...), true
		}
		return nil, false
	}
	return nil, false
}

func callArgumentIndex(role string) (int, error) {
	text := strings.TrimPrefix(role, "argument-")
	if len(text) != 8 {
		return 0, fmt.Errorf("invalid index")
	}
	return strconv.Atoi(text)
}

func callDiagnostic(operation equation.BoundEquation, code, subject, message string) equation.TransactionResult {
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
		Key: "type.call.direct." + code + "/" + operation.Target.Name + "/" + subject, Value: []byte(message),
	}}}}
}

func enrichSendIsolationDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
	item.Help = sendIsolationHelp
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "send payload"}}
	switch item.Message {
	case "send payload is proven isolated for zero-copy transfer":
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "isolation proof: direct fresh object literal has no retained graph identity"},
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "direct literal birth site has no retained graph identity"},
		}
		item.Labels = append(item.Labels, DiagnosticLabel{Span: item.Span, Message: "send-safety proof"})
	case "send payload is proven immutable for zero-copy sharing":
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "immutable proof: sent exact identity is frozen"},
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "exact identity is frozen before send"},
		}
		item.Labels = append(item.Labels, DiagnosticLabel{Span: item.Span, Message: "send-safety proof"})
	case "send payload has a proven escaping alias; zero-copy transfer is rejected":
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "escape proof: payload has already crossed a retaining boundary"}}
	case "send payload is not proven isolated or immutable; runtime will copy":
		payload, reason := sendIsolationPayload(operation)
		_ = payload
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "unknown", Message: reason},
			{Span: item.Span, Kind: "missing proof", Trust: "unknown", Message: reason},
		}
	}
	return item
}

func sendIsolationPayload(operation equation.Equation) ([]byte, string) {
	for _, operand := range operation.Operands {
		if operand.Role != "argument-00000002" {
			continue
		}
		if strings.HasPrefix(string(operand.Term.Encoding), "temp/") {
			return operand.Term.Encoding, "copy fallback: object graph contains another identity that may still be aliased"
		}
		return operand.Term.Encoding, "copy fallback: stack-local path may have aliases across the send"
	}
	return nil, "copy fallback: stack-local path may have aliases across the send"
}

func indexedCallSubject(prefix string, index int) string {
	return fmt.Sprintf("%s-%08d", prefix, index)
}

type callableShape struct {
	Params   []string `json:"params"`
	Required int      `json:"required"`
	Variadic bool     `json:"variadic"`
}

func isCallableValue(value []byte) bool {
	return string(value) == "scalar/function" || strings.HasPrefix(string(value), "scalar/function/")
}

func callableSignature(value []byte) (callableShape, bool) {
	encoded := strings.TrimPrefix(string(value), "scalar/function/")
	if encoded == string(value) || encoded == "" {
		return callableShape{}, false
	}
	wire, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return callableShape{}, false
	}
	var signature callableShape
	if err := json.Unmarshal(wire, &signature); err != nil || signature.Required < 0 || signature.Required > len(signature.Params) {
		return callableShape{}, false
	}
	for _, parameter := range signature.Params {
		if parameter == "" {
			return callableShape{}, false
		}
	}
	return signature, true
}

func resolveKnownCurrentValue(term []byte, partition equation.Partition) ([]byte, bool) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), true
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, false
	}
	prefix := "value/" + string(term) + "/"
	var value []byte
	latestKey := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latestKey) {
			value, latestKey = fact.Value, fact.Key
		}
	}
	return append([]byte(nil), value...), value != nil
}

func provenScalarNotSubtype(value []byte, expected string) bool {
	if isUnknownScalar(value) || expected == "any" || expected == "unknown" {
		return false
	}
	if strings.HasSuffix(expected, "?") {
		return string(value) != "scalar/nil" && provenScalarNotSubtype(value, strings.TrimSuffix(expected, "?"))
	}
	switch expected {
	case "nil":
		return string(value) != "scalar/nil"
	case "boolean":
		return !strings.HasPrefix(string(value), "scalar/bool/")
	case "string":
		return !strings.HasPrefix(string(value), "scalar/string/")
	case "number":
		return !strings.HasPrefix(string(value), "scalar/number/")
	case "integer":
		if !strings.HasPrefix(string(value), "scalar/number/") {
			return true
		}
		_, err := strconv.ParseInt(strings.TrimPrefix(string(value), "scalar/number/"), 10, 64)
		return err != nil
	default:
		return false
	}
}

func callDisplayValue(value []byte) string {
	display, err := displayValue(value)
	if err != nil {
		return "unknown"
	}
	return string(display)
}

// externalCallKernel is a sealed provider-boundary factor.  It intentionally
// owns no result term: call-results remains the sole result-slot owner for
// every call, whether the callee is local or external.
func externalCallKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "application", "provider", "argument-spread", "result-arity", "result-spread", "context")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if !strings.HasPrefix(string(operands["application"]), "call/") ||
		(!strings.HasPrefix(string(operands["provider"]), "provider/global/") && !strings.HasPrefix(string(operands["provider"]), "provider/module/")) ||
		(string(operands["argument-spread"]) != "scalar/bool/true" && string(operands["argument-spread"]) != "scalar/bool/false") ||
		(string(operands["result-spread"]) != "scalar/bool/true" && string(operands["result-spread"]) != "scalar/bool/false") ||
		!strings.HasPrefix(string(operands["context"]), "call-context/") {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external call boundary")
	}
	if _, err := strconv.Atoi(string(operands["result-arity"])); err != nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external result arity")
	}
	for _, operand := range operation.Operands {
		if strings.HasPrefix(operand.Role, "argument-") || operand.Role == "receiver" || operand.Role == "method" || operand.Role == "application" || operand.Role == "provider" || operand.Role == "argument-spread" || operand.Role == "result-arity" || operand.Role == "result-spread" || operand.Role == "context" {
			continue
		}
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external call role %q", operand.Role)
	}
	return equation.TransactionResult{Complete: true}, nil
}

// callResultsKernel publishes explicit Top facts for unresolved owned result
// slots, never a missing slot or an invented concrete value.
func callResultsKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	resultTerms := map[string][]byte{}
	targetTerms := map[string][]byte{}
	hasApplication := false
	var application []byte
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "application":
			if hasApplication || !strings.HasPrefix(string(operand.Value), "call/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result application")
			}
			hasApplication = true
			application = operand.Value
		case strings.HasPrefix(operand.Role, "result-"):
			resultTerms[strings.TrimPrefix(operand.Role, "result-")] = operand.Value
		case strings.HasPrefix(operand.Role, "target-"):
			targetTerms[strings.TrimPrefix(operand.Role, "target-")] = operand.Value
		default:
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result role %q", operand.Role)
		}
	}
	if !hasApplication || (len(targetTerms) != 0 && len(resultTerms) != len(targetTerms)) {
		return equation.TransactionResult{}, fmt.Errorf("engine: incomplete call result transaction")
	}
	values := make([]equation.Fact, 0, len(resultTerms))
	for key, result := range resultTerms {
		if len(result) == 0 || !strings.HasPrefix(string(result), "temp/") || (len(targetTerms) != 0 && len(targetTerms[key]) == 0) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result %q", key)
		}
		value := []byte("scalar/top")
		// A known lexical apply seals its child outcome under the same
		// application coordinate. call-results is the sole owner of caller
		// result terms, so it consumes that private projection rather than
		// falling through to Top.
		projectedKey := "call-result/" + strings.TrimPrefix(string(application), "call/") + "/" + key
		for _, fact := range partition.Values() {
			if fact.Key == projectedKey {
				value = append([]byte(nil), fact.Value...)
				break
			}
		}
		if key == "00000000" && hasValue(partition, "effect.call-bool/"+strings.TrimPrefix(string(application), "call/")) {
			value = []byte("scalar/boolean")
		}
		values = append(values, equation.Fact{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: value})
		closureKey := "call-closure/" + strings.TrimPrefix(string(application), "call/") + "/" + key
		for _, fact := range partition.Values() {
			if fact.Key == closureKey {
				values = append(values, equation.Fact{Key: "closure/" + string(result) + "/" + operation.Target.Name, Value: append([]byte(nil), fact.Value...)})
				break
			}
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

func hasValue(partition equation.Partition, key string) bool {
	for _, item := range partition.Values() {
		if item.Key == key && string(item.Value) == "scalar/boolean" {
			return true
		}
	}
	return false
}

// publicationKernel resolves every selected return slot before publishing any
// output.  A false or unknown guard contributes no tuple; a selected guard
// contributes the complete indexed tuple, including nil-valued slots.
func publicationKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	values := make([][]byte, len(operation.Operands))
	for _, operand := range operation.Operands {
		const prefix = "return-value-"
		if !strings.HasPrefix(operand.Role, prefix) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		indexText := strings.TrimPrefix(operand.Role, prefix)
		if len(indexText) != 8 {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		index, err := strconv.Atoi(indexText)
		if err != nil || index < 0 || index >= len(values) || values[index] != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		value, err := resolveCurrentValue(operand.Value, partition)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		values[index] = value
	}
	for index, value := range values {
		if value == nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: missing return value %d", index)
		}
	}
	// Every return occurrence owns its internal tuple.  A file can have more
	// than one reachable return (for example, a loop return plus the fallthrough
	// return), and those alternatives must not collide in the equation fact map.
	// publishedOutcomes joins them conservatively back into the public slots.
	prefix := "return-candidate/" + operation.Target.Name + "/"
	outcomes := make([]equation.Fact, 0, len(values)+1)
	outcomes = append(outcomes, equation.Fact{Key: prefix + "arity", Value: []byte(strconv.Itoa(len(values)))})
	for index, value := range values {
		outcomes = append(outcomes, equation.Fact{Key: prefix + strconv.Itoa(index), Value: value})
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Outcomes: outcomes}}, nil
}

// branchTruth evaluates exactly one selector.  An unavailable selector is an
// error, not a false edge: absence, bottom, and a complete falsy value stay
// distinct throughout a branch transaction.
func branchTruth(operands []equation.BoundOperand, partition equation.Partition) (bool, bool, error) {
	var condition, predicate []byte
	for _, operand := range operands {
		switch operand.Role {
		case "condition":
			if condition != nil {
				return false, false, fmt.Errorf("engine: duplicate branch condition")
			}
			condition = operand.Value
		case "predicate":
			if predicate != nil {
				return false, false, fmt.Errorf("engine: duplicate branch predicate")
			}
			predicate = operand.Value
		default:
			// Evidence, arm boundaries, and difference constraints are closed
			// branch metadata. They are intentionally not alternate selectors.
			if !strings.HasPrefix(operand.Role, "implied-") && !strings.HasPrefix(operand.Role, "sufficient-") && !strings.HasPrefix(operand.Role, "difference-") {
				return false, false, fmt.Errorf("engine: malformed branch operand role %q", operand.Role)
			}
		}
	}
	if frozenPredicate(predicate) {
		// The true edge of table.isfrozen is a direct runtime witness. This does
		// not assert global truth; it records proof for equations guarded by it.
		return true, true, nil
	}
	if condition != nil {
		value, err := resolveCurrentValue(condition, partition)
		if err != nil {
			return false, false, err
		}
		truth, err := luaTruthy(value)
		return truth, false, err
	}
	if predicate == nil {
		return false, false, fmt.Errorf("engine: branch has no selector")
	}
	truth, err := evaluateBranchPredicate(predicate, partition)
	return truth, false, err
}

func frozenPredicate(encoded []byte) bool {
	if !strings.HasPrefix(string(encoded), branchPredicatePrefix) {
		return false
	}
	var predicate branchPredicateWire
	return json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate) == nil && predicate.Kind == "frozen-table"
}

func evaluateBranchPredicate(encoded []byte, partition equation.Partition) (bool, error) {
	if !strings.HasPrefix(string(encoded), branchPredicatePrefix) {
		return false, fmt.Errorf("engine: malformed branch predicate")
	}
	var predicate branchPredicateWire
	if err := json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate); err != nil || predicate.Kind == "" {
		if err != nil {
			return false, fmt.Errorf("engine: decode branch predicate: %w", err)
		}
		return false, fmt.Errorf("engine: branch predicate has no kind")
	}
	value, err := branchPathValue(predicate.Path, partition)
	if err != nil {
		return false, err
	}
	var result bool
	switch predicate.Kind {
	case "truthy":
		result, err = luaTruthy(value)
	case "falsy":
		result, err = luaTruthy(value)
		result = !result
	case "nil":
		result = string(value) == "scalar/nil"
	case "not-nil":
		result = string(value) != "scalar/nil"
	case "literal-equal", "literal-not":
		if predicate.Literal == "" {
			return false, fmt.Errorf("engine: literal predicate has no literal")
		}
		result = string(value) == predicate.Literal
		if predicate.Kind == "literal-not" {
			result = !result
		}
	case "path-equal", "path-not":
		other, valueErr := branchPathValue(predicate.OtherPath, partition)
		if valueErr != nil {
			return false, valueErr
		}
		result = string(value) == string(other)
		if predicate.Kind == "path-not" {
			result = !result
		}
	case "type-equal", "type-not":
		typeName := predicate.TypeName
		if typeName == "" {
			other, valueErr := branchPathValue(predicate.OtherPath, partition)
			if valueErr != nil {
				return false, valueErr
			}
			typeName, valueErr = scalarString(other)
			if valueErr != nil {
				return false, valueErr
			}
		}
		actual, valueErr := scalarType(value)
		if valueErr != nil {
			return false, valueErr
		}
		result = actual == typeName
		if predicate.Kind == "type-not" {
			result = !result
		}
	case "len-ge":
		length, valueErr := scalarLength(value)
		if valueErr != nil {
			return false, valueErr
		}
		result = length >= predicate.LenFloor
	case "num-ge":
		number, valueErr := scalarNumber(value)
		if valueErr != nil {
			return false, valueErr
		}
		result = number >= float64(predicate.NumFloor)
	case "num-le":
		number, valueErr := scalarNumber(value)
		if valueErr != nil {
			return false, valueErr
		}
		if !predicate.HasNumCeil {
			return false, fmt.Errorf("engine: numeric ceiling predicate has no ceiling")
		}
		result = number <= float64(predicate.NumCeil)
	case "index-in-range", "frozen-table":
		// Heap/index evidence is outside this scalar evaluator. Treat it as an
		// unavailable selector: neither branch is selected until a dedicated
		// heap-domain evaluator can prove one.
		return false, errUnknownScalar
	default:
		return false, fmt.Errorf("engine: unknown branch predicate %q", predicate.Kind)
	}
	if err != nil {
		return false, err
	}
	if predicate.Negated {
		result = !result
	}
	return result, nil
}

func branchPathValue(key string, partition equation.Partition) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("engine: branch predicate has an absent path")
	}
	return resolveCurrentValue([]byte("path/"+key), partition)
}

func scalarType(value []byte) (string, error) {
	if isUnknownScalar(value) {
		return "", errUnknownScalar
	}
	switch {
	case shapefact.IsTable(value):
		return "table", nil
	case string(value) == "scalar/nil":
		return "nil", nil
	case strings.HasPrefix(string(value), "scalar/bool/"):
		return "boolean", nil
	case strings.HasPrefix(string(value), "scalar/number/"):
		return "number", nil
	case strings.HasPrefix(string(value), "scalar/string/"):
		return "string", nil
	case string(value) == "scalar/table":
		return "table", nil
	case string(value) == "scalar/function":
		return "function", nil
	default:
		return "", fmt.Errorf("engine: malformed scalar value %q", value)
	}
}

func scalarString(value []byte) (string, error) {
	if isUnknownScalar(value) {
		return "", errUnknownScalar
	}
	if !strings.HasPrefix(string(value), "scalar/string/") {
		return "", fmt.Errorf("engine: type predicate path is not a string")
	}
	decoded, err := strconv.Unquote(strings.TrimPrefix(string(value), "scalar/string/"))
	if err != nil {
		return "", err
	}
	return decoded, nil
}

func scalarLength(value []byte) (int64, error) {
	decoded, err := scalarString(value)
	if err != nil {
		return 0, err
	}
	return int64(len(decoded)), nil
}

func scalarNumber(value []byte) (float64, error) {
	if isUnknownScalar(value) {
		return 0, errUnknownScalar
	}
	if !strings.HasPrefix(string(value), "scalar/number/") {
		return 0, fmt.Errorf("engine: numeric predicate path is not a number")
	}
	parsed, err := strconv.ParseFloat(strings.TrimPrefix(string(value), "scalar/number/"), 64)
	if err != nil {
		return 0, fmt.Errorf("engine: decode numeric predicate: %w", err)
	}
	return parsed, nil
}

func operandsByRole(operands []equation.BoundOperand, roles ...string) (map[string][]byte, error) {
	wanted := make(map[string]bool, len(roles))
	for _, role := range roles {
		wanted[role] = true
	}
	result := make(map[string][]byte, len(roles))
	for _, operand := range operands {
		if !wanted[operand.Role] || result[operand.Role] != nil {
			return nil, fmt.Errorf("engine: malformed operand role %q", operand.Role)
		}
		result[operand.Role] = operand.Value
	}
	for _, role := range roles {
		if len(result[role]) == 0 {
			return nil, fmt.Errorf("engine: missing operand %q", role)
		}
	}
	return result, nil
}

// requiredOperandsByRole is the structural counterpart to operandsByRole: it
// checks required roles exactly once while allowing the allocation family to
// carry its sealed member/capture inventory as additional closed operands.
func requiredOperandsByRole(operands []equation.BoundOperand, roles ...string) (map[string][]byte, error) {
	wanted := make(map[string]bool, len(roles))
	for _, role := range roles {
		wanted[role] = true
	}
	result := make(map[string][]byte, len(roles))
	for _, operand := range operands {
		if !wanted[operand.Role] {
			continue
		}
		if result[operand.Role] != nil {
			return nil, fmt.Errorf("engine: duplicate operand role %q", operand.Role)
		}
		result[operand.Role] = operand.Value
	}
	for _, role := range roles {
		if len(result[role]) == 0 {
			return nil, fmt.Errorf("engine: missing operand %q", role)
		}
	}
	return result, nil
}

func guardsHold(guards []equation.Guard, partition equation.Partition) bool {
	outcomes := make(map[string][]byte, len(partition.Outcomes()))
	for _, outcome := range partition.Outcomes() {
		outcomes[outcome.Key] = outcome.Value
	}
	for _, guard := range guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || (parts[3] != "true" && parts[3] != "false") {
			return false
		}
		if string(outcomes["branch/"+parts[2]]) != "scalar/bool/"+parts[3] {
			return false
		}
	}
	return true
}

func resolveValue(term, readBefore, absence []byte, partition equation.Partition) ([]byte, error) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), nil
	}
	if shapefact.IsTable(term) {
		return append([]byte(nil), term...), nil
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, fmt.Errorf("engine: unsupported scalar term %q", term)
	}
	const readBeforePrefix = "front/read-before/"
	if !strings.HasPrefix(string(readBefore), readBeforePrefix) {
		return nil, fmt.Errorf("engine: malformed assignment read boundary %q", readBefore)
	}
	cutoff := strings.TrimPrefix(string(readBefore), readBeforePrefix)
	if cutoff == "" {
		return nil, fmt.Errorf("engine: empty assignment read boundary")
	}
	prefix := "value/" + string(term) + "/"
	var value []byte
	latestKey := ""
	for _, fact := range partition.Values() {
		name := strings.TrimPrefix(fact.Key, prefix)
		if strings.HasPrefix(fact.Key, prefix) && name <= cutoff && (value == nil || fact.Key > latestKey) {
			value = fact.Value
			latestKey = fact.Key
		}
	}
	if value == nil {
		switch string(absence) {
		case "front/absence/nil":
			return []byte("scalar/nil"), nil
		case "front/absence/top":
			return []byte("scalar/top"), nil
		case "front/absence/error":
			return nil, fmt.Errorf("engine: value path %q has no completed write before %q", term, cutoff)
		default:
			return nil, fmt.Errorf("engine: malformed assignment absence policy %q", absence)
		}
	}
	return append([]byte(nil), value...), nil
}

// resolveCurrentValue is for non-assignment families whose own contract owns
// their read timing. Environment-write deliberately does not use it: every
// assignment read carries an explicit pre-write boundary instead.
func resolveCurrentValue(term []byte, partition equation.Partition) ([]byte, error) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), nil
	}
	if shapefact.IsTable(term) {
		return append([]byte(nil), term...), nil
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, fmt.Errorf("engine: unsupported scalar term %q", term)
	}
	prefix := "value/" + string(term) + "/"
	var value []byte
	latestKey := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latestKey) {
			value = fact.Value
			latestKey = fact.Key
		}
	}
	if value == nil {
		if member, found := shapeMemberValue(term, partition); found {
			return member, nil
		}
		// A member/index path has a concrete Lua source but its heap write can
		// be outside this scalar model. Root paths remain strict so an absent
		// variable is never fabricated as a truthy or falsy value.
		if derivedPathTerm(term) {
			return []byte("scalar/top"), nil
		}
		return nil, fmt.Errorf("engine: value path %q has no completed write", term)
	}
	return append([]byte(nil), value...), nil
}

// shapeMemberValue projects a member only from a prior, sealed literal fact.
// It never guesses through an unrecorded heap object: absence is available
// only when the literal itself recorded that member path as absent (or closed).
func shapeMemberValue(term []byte, partition equation.Partition) ([]byte, bool) {
	if !strings.HasPrefix(string(term), "path/") {
		return nil, false
	}
	key := strings.TrimPrefix(string(term), "path/")
	for cut := len(key); cut > 0; {
		cut = strings.LastIndexAny(key[:cut], ".[")
		if cut < 0 {
			return nil, false
		}
		ancestor, suffix := key[:cut], key[cut:]
		value, found := latestValue([]byte("path/"+ancestor), partition)
		if !found {
			continue
		}
		table, ok := shapefact.DecodeTable(value)
		if !ok {
			continue
		}
		member, found := table.Lookup(suffix)
		if !found {
			return nil, false
		}
		if !member.Present {
			return []byte("scalar/nil"), true
		}
		return []byte(member.Value), true
	}
	return nil, false
}

func latestValue(term []byte, partition equation.Partition) ([]byte, bool) {
	prefix := "value/" + string(term) + "/"
	var value []byte
	latestKey := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latestKey) {
			value, latestKey = fact.Value, fact.Key
		}
	}
	return append([]byte(nil), value...), value != nil
}

func luaTruthy(value []byte) (bool, error) {
	if isUnknownScalar(value) {
		return false, errUnknownScalar
	}
	if shapefact.IsTable(value) {
		return true, nil
	}
	switch string(value) {
	case "scalar/boolean":
		return false, errUnknownScalar
	case "scalar/nil", "scalar/bool/false":
		return false, nil
	case "scalar/bool/true":
		return true, nil
	default:
		if strings.HasPrefix(string(value), "scalar/number/") || strings.HasPrefix(string(value), "scalar/string/") || string(value) == "scalar/table" || string(value) == "scalar/function" {
			return true, nil
		}
		return false, fmt.Errorf("engine: malformed scalar value %q", value)
	}
}

func isUnknownScalar(value []byte) bool {
	return string(value) == "scalar/top" || strings.HasPrefix(string(value), "scalar/claim/")
}

func derivedPathTerm(term []byte) bool {
	path := strings.TrimPrefix(string(term), "path/")
	return strings.Contains(path, ".") || strings.Contains(path, "[")
}

func publishedValues(artifact equation.Artifact, stored []equation.Fact) []equation.Fact {
	storedByKey := make(map[string][]byte, len(stored))
	for _, fact := range stored {
		storedByKey[fact.Key] = fact.Value
	}
	type candidate struct {
		fact       equation.Fact
		display    string
		coordinate equation.Coordinate
	}
	// A stored fact key is canonical kernel output, so it is the publication
	// identity.  Display names deliberately do not participate in identity:
	// several source operations may display as the same local variable.
	byIdentity := make(map[string]candidate)
	for _, operation := range artifact.Equations {
		var target, display []byte
		switch operation.Occurrence.Kind {
		case "environment-write", "claim":
			operands, err := artifactOperandsByRole(operation.Operands, "target", "display")
			if err != nil {
				continue
			}
			target, display = operands["target"], operands["display"]
		case "expression":
			operands, err := artifactOperandsByRole(operation.Operands, "result", "display")
			if err != nil || !strings.HasPrefix(string(operands["result"]), "path/") {
				continue
			}
			target, display = operands["result"], operands["display"]
		default:
			continue
		}
		if strings.HasPrefix(string(display), "front/hidden/") {
			continue
		}
		key := "value/" + string(target) + "/" + operation.Target.Name
		value, found := storedByKey[key]
		if !found {
			continue
		}
		decoded, err := displayValue(value)
		if err != nil {
			continue
		}
		byIdentity[key] = candidate{fact: equation.Fact{Key: key, Value: decoded}, display: string(display), coordinate: operation.Target}
	}

	dependencies := make(map[equation.Coordinate][]equation.Coordinate, len(artifact.Equations))
	for _, operation := range artifact.Equations {
		dependencies[operation.Target] = operation.Dependencies
	}
	dependsOn := func(from, want equation.Coordinate) bool {
		seen := make(map[equation.Coordinate]bool)
		var visit func(equation.Coordinate) bool
		visit = func(current equation.Coordinate) bool {
			if current == want {
				return true
			}
			if seen[current] {
				return false
			}
			seen[current] = true
			for _, dependency := range dependencies[current] {
				if visit(dependency) {
					return true
				}
			}
			return false
		}
		return visit(from)
	}
	byDisplay := make(map[string][]candidate)
	for _, value := range byIdentity {
		byDisplay[value.display] = append(byDisplay[value.display], value)
	}
	values := make([]equation.Fact, 0, len(byDisplay))
	for display, candidates := range byDisplay {
		var selected *candidate
		for index := range candidates {
			latest := true
			for other := range candidates {
				if index != other && !dependsOn(candidates[index].coordinate, candidates[other].coordinate) {
					latest = false
					break
				}
			}
			if latest {
				selected = &candidates[index]
				break
			}
		}
		if selected != nil {
			values = append(values, equation.Fact{Key: display, Value: selected.fact.Value})
			continue
		}
		value := candidates[0].fact.Value
		for _, candidate := range candidates[1:] {
			if !bytes.Equal(value, candidate.fact.Value) {
				// Incomparable writes are alternate facts.  Publishing a concrete
				// winner from their display or sort order would be unsound.
				value = []byte("unknown")
				break
			}
		}
		values = append(values, equation.Fact{Key: display, Value: value})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Key < values[j].Key })
	return values
}

func publishedOutcomes(stored []equation.Fact) []equation.Fact {
	// Candidate tuples are operation-scoped inside the equation closure.  Join
	// alternatives only at publication: identical alternatives retain their
	// scalar result, while any disagreement becomes Top/unknown.
	returnCandidates := make(map[string][][]byte)
	outcomes := make([]equation.Fact, 0, len(stored))
	for _, storedOutcome := range stored {
		if strings.HasPrefix(storedOutcome.Key, "return-candidate/") {
			parts := strings.Split(storedOutcome.Key, "/")
			if len(parts) == 3 && parts[2] != "" {
				returnCandidates[parts[2]] = append(returnCandidates[parts[2]], append([]byte(nil), storedOutcome.Value...))
			}
			continue
		}
		outcome := equation.Fact{Key: storedOutcome.Key, Value: append([]byte(nil), storedOutcome.Value...)}
		if strings.HasPrefix(outcome.Key, "return/") && outcome.Key != "return/arity" {
			decoded, err := displayValue(outcome.Value)
			if err == nil {
				outcome.Value = decoded
			}
		}
		outcomes = append(outcomes, outcome)
	}
	for slot, candidates := range returnCandidates {
		value := candidates[0]
		for _, candidate := range candidates[1:] {
			if !bytes.Equal(value, candidate) {
				value = []byte("scalar/top")
				break
			}
		}
		key := "return/" + slot
		if slot == "arity" {
			outcomes = append(outcomes, equation.Fact{Key: key, Value: value})
			continue
		}
		decoded, err := displayValue(value)
		if err == nil {
			value = decoded
		}
		outcomes = append(outcomes, equation.Fact{Key: key, Value: value})
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Key < outcomes[j].Key })
	return outcomes
}

func artifactOperandsByRole(operands []equation.Operand, roles ...string) (map[string][]byte, error) {
	wanted := make(map[string]bool, len(roles))
	for _, role := range roles {
		wanted[role] = true
	}
	result := make(map[string][]byte, len(roles))
	for _, operand := range operands {
		if operand.Term.Entry {
			return nil, fmt.Errorf("engine: unexpected entry operand")
		}
		if wanted[operand.Role] {
			if result[operand.Role] != nil {
				return nil, fmt.Errorf("engine: duplicate artifact operand %q", operand.Role)
			}
			result[operand.Role] = operand.Term.Encoding
		}
	}
	for _, role := range roles {
		if len(result[role]) == 0 {
			return nil, fmt.Errorf("engine: missing artifact operand %q", role)
		}
	}
	return result, nil
}

func displayValue(value []byte) ([]byte, error) {
	encoded := string(value)
	switch {
	case encoded == "scalar/nil":
		return []byte("nil"), nil
	case strings.HasPrefix(encoded, "scalar/bool/"):
		return []byte(strings.TrimPrefix(encoded, "scalar/bool/")), nil
	case strings.HasPrefix(encoded, "scalar/number/"):
		return []byte(strings.TrimPrefix(encoded, "scalar/number/")), nil
	case strings.HasPrefix(encoded, "scalar/string/"):
		stringValue, err := strconv.Unquote(strings.TrimPrefix(encoded, "scalar/string/"))
		if err != nil {
			return nil, err
		}
		return []byte(strconv.Quote(stringValue)), nil
	case encoded == "scalar/table", encoded == "scalar/function":
		return []byte("unknown"), nil
	case strings.HasPrefix(encoded, "scalar/claim/"):
		return []byte("unknown"), nil
	case encoded == "scalar/top":
		return []byte("unknown"), nil
	default:
		return nil, fmt.Errorf("engine: invalid stored scalar")
	}
}
