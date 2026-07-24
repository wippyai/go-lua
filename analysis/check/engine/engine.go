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
	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
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

const memberMissingPrefix = "shape/member-missing/v1/"

// Heap facts are deliberately keyed by a sealed allocation identity, never by
// a source path.  Paths are merely lenses: assignments copy an identity and
// member/index writes update the object reached by every such lens.  Keeping
// the identity separate from a shape avoids letting a stale root.field value
// outrank a write made through an alias.
const (
	heapTableIdentityPrefix  = "heap/table-identity/"
	heapMemberPrefix         = "heap/member/"
	heapMemberIdentityPrefix = "heap/member-identity/"
)

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
	Artifact equation.Artifact
	Values   []equation.Fact
	Outcomes []equation.Fact
	// ReturnCandidates retains the closed, equation-owned return facts before
	// their source-display projection. Tables and callable values remain sealed
	// here even when Outcomes renders them as "unknown". Module-boundary
	// exporters consume this evidence without re-evaluating source.
	ReturnCandidates []equation.Fact
	// ValueFacts is the complete closed value partition. It lets module export
	// projection retain static writes made to a returned table after allocation
	// without consulting source syntax.
	ValueFacts  []equation.Fact
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
	// Placement is the conservative allocation plan projected from placement
	// facts in the completed equation closure. A nil plan means the closure
	// established no allocation-site fact; callers must not substitute a
	// source-derived guess.
	Placement    *PlacementPlan
	Transactions int
	Timings      Timings
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
	return CheckWithImports(source, nil)
}

// CheckWithImports admits resolved module exports as closed entry facts. It
// does not resolve imports itself: callers provide only the manifest exports
// already selected at their project boundary. An unknown export is omitted so
// the equation remains fail-closed rather than treating an unresolved module
// as any.
func CheckWithImports(source string, imports map[string]typ.Type) (result Result, err error) {
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
	boundEntry, entryErr := importEntryValue(imports)
	if entryErr != nil {
		return Result{}, entryErr
	}
	binding := equation.EntryBinding{Parameter: entry, Value: boundEntry}
	fileContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	lexical := newLexicalEvaluator(compilation)
	lexical.ctx = fileContext
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
		evaluation, evaluateErr := vm.Evaluate(fileContext, bound, []string{"published"})
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
	published := publishedDiagnostics(artifact, closure, diagnosticSpans, compilation.ClaimTargetSpans, compilation.CallSpans, lexical.lifecycleEvidence, lexical.selectEvidence)
	published = mergeChildPublishedDiagnostics(published, lexical.childPublished)
	result = Result{
		Artifact: artifact, Values: publishedValues(artifact, closure.Values),
		Outcomes: publishedOutcomes(closure.Outcomes), Diagnostics: closure.Diagnostics,
		ReturnCandidates:     cloneFacts(closure.Outcomes),
		ValueFacts:           cloneFacts(closure.Values),
		PublishedDiagnostics: published,
		DiagnosticSpans:      diagnosticSpans,
		Placement:            publishedPlacement(closure.Values),
		Transactions:         transactions,
		Timings:              Timings{ParseBindLower: parseElapsed, Evaluate: time.Since(evaluateStarted)},
	}
	return result, nil
}

func cloneFacts(in []equation.Fact) []equation.Fact {
	if len(in) == 0 {
		return nil
	}
	out := make([]equation.Fact, len(in))
	for index, fact := range in {
		out[index] = cloneFact(fact)
	}
	return out
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
		case strings.HasPrefix(item.Key, "type.member.missing/"):
			name = strings.TrimPrefix(item.Key, "type.member.missing/")
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
		case strings.HasPrefix(item.Key, "channel.send.closed/"), strings.HasPrefix(item.Key, "channel.close.closed/"), strings.HasPrefix(item.Key, "typestate.invalid_requirement/"), strings.HasPrefix(item.Key, "typestate.invalid_transition/"), strings.HasPrefix(item.Key, "typestate.unproven_requirement/"):
			name = item.Key[strings.LastIndexByte(item.Key, '/')+1:]
			if span, ok := callSpans[name+"/call"]; ok {
				out[item.Key] = span
			}
			continue
		case strings.HasPrefix(item.Key, "effect.lifecycle.unreleased/"):
			name = strings.TrimPrefix(item.Key, "effect.lifecycle.unreleased/")
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
func publishedDiagnostics(artifact equation.Artifact, closure equation.OutputClosure, spans, claimTargetSpans, callSpans map[string]wir.Span, lifecycleEvidence, selectEvidence map[string][]DiagnosticEvidence) []PublishedDiagnostic {
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
		if inner, ok := childDiagnosticKey(fact.Key); ok {
			item.Code = diagnosticCode(inner)
		}
		if isChannelLifecycleDiagnostic(fact.Key) {
			out = append(out, enrichChannelLifecycleDiagnostic(item))
			continue
		}
		if isResourceTypestateDiagnostic(fact.Key) {
			out = append(out, enrichResourceTypestateDiagnostic(item))
			continue
		}
		if inner, _ := childDiagnosticKey(fact.Key); strings.HasPrefix(inner, "channel.select.exhaustiveness/") || strings.HasPrefix(inner, "lint.union.exhaustiveness/") {
			item.Evidence = append([]DiagnosticEvidence(nil), selectEvidence[fact.Key]...)
			if strings.HasPrefix(inner, "lint.union.exhaustiveness/") {
				item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "union case check"}}
				item.Help = "Handle each missing case, or add an else branch when a fallback is valid."
			} else {
				item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "channel case check"}}
				item.Help = "Add an elseif branch for each missing case, or add a default branch when a fallback is valid."
			}
			out = append(out, item)
			continue
		}
		if inner, _ := childDiagnosticKey(fact.Key); strings.HasPrefix(inner, "effect.lifecycle.unreleased/") {
			out = append(out, enrichUnreleasedLifecycleDiagnostic(item, lifecycleEvidence[fact.Key]))
			continue
		}
		if strings.HasPrefix(fact.Key, "effect.freeze.mutation/") {
			out = append(out, enrichFrozenMutationDiagnostic(item, artifact, callSpans))
			continue
		}
		if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "type.call.direct.") {
			out = append(out, enrichDirectCallDiagnostic(item, operation))
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
			name, missing := strings.CutPrefix(fact.Key, "type.member.missing/")
			if missing {
				if operation, found := claims[name]; found {
					out = append(out, enrichMissingMemberDiagnostic(item, operation, claimTargetSpans[name], closure))
					continue
				}
			}
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
		if display := claimDeclaredDisplay(operation, operands["type"]); display != "" {
			declared = display
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
		if _, typed := shapefact.DecodeTarget(value); typed {
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type %s", sourceDisplay, valueDescription)},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueDescription}, {Span: targetSpan, Message: "declared type " + declared}}
		} else {
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has literal value %s", display, valueDescription)},
				{Span: item.Span, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueDescription}, {Span: item.Span, Message: "declared type " + declared}}
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

func enrichMissingMemberDiagnostic(item PublishedDiagnostic, operation equation.Equation, targetSpan wir.Span, closure equation.OutputClosure) PublishedDiagnostic {
	operands, err := artifactOperandsByRole(operation.Operands, "value")
	if err != nil {
		return item
	}
	value, available := claimDiagnosticValue(operands["value"], operation, closure)
	source := "member"
	for _, operand := range operation.Operands {
		if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
			source = string(operand.Term.Encoding)
		}
	}
	member := source[strings.LastIndex(source, ".")+1:]
	if member == "" || member == source {
		return item
	}
	if !targetSpan.Valid() {
		targetSpan = item.Span
	}
	receiverText := ""
	if available {
		if receiver, ok := memberMissingReceiver(value); ok {
			receiverText = typeformat.Short(receiver)
		}
	}
	if receiverText == "" {
		const marker = " has no member "
		if cut := strings.Index(item.Message, marker); cut > 0 {
			receiverText = item.Message[:cut]
		}
	}
	if receiverText == "" {
		return item
	}
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s reads member %q from receiver type %s", source, member, receiverText)}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "member read"}}
	item.Help = fmt.Sprintf("Narrow the receiver before reading `%s`, or add `%s` to every reachable receiver shape.", member, member)
	return item
}

func childDiagnosticKey(key string) (string, bool) {
	if !strings.HasPrefix(key, "child/") {
		return key, false
	}
	parts := strings.SplitN(key, "/", 3)
	if len(parts) != 3 {
		return key, false
	}
	return parts[2], true
}

func isChannelLifecycleDiagnostic(key string) bool {
	inner, _ := childDiagnosticKey(key)
	return strings.HasPrefix(inner, "channel.send.closed/") || strings.HasPrefix(inner, "channel.close.closed/")
}

func isResourceTypestateDiagnostic(key string) bool {
	inner, _ := childDiagnosticKey(key)
	return strings.HasPrefix(inner, "typestate.invalid_requirement/") || strings.HasPrefix(inner, "typestate.invalid_transition/") || strings.HasPrefix(inner, "typestate.unproven_requirement/")
}

func enrichChannelLifecycleDiagnostic(item PublishedDiagnostic) PublishedDiagnostic {
	inner, _ := childDiagnosticKey(item.Fact.Key)
	display := "channel"
	if start := strings.LastIndex(item.Message, "`"); start >= 0 {
		if end := strings.LastIndex(item.Message[:start], "`"); end >= 0 && end+1 < start {
			display = item.Message[end+1 : start]
		}
	}
	if strings.HasPrefix(inner, "channel.send.closed/") {
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "this send call runs after `" + display + "` is proven closed"}}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "channel lifecycle call"}}
		item.Help = "Send before closing the channel."
		return item
	}
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "this close call runs after `" + display + "` is proven closed"}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "channel lifecycle call"}}
	item.Help = "Avoid closing the same channel twice."
	return item
}

func enrichResourceTypestateDiagnostic(item PublishedDiagnostic) PublishedDiagnostic {
	inner, _ := childDiagnosticKey(item.Fact.Key)
	transition := strings.HasPrefix(inner, "typestate.invalid_transition/")
	unproven := strings.HasPrefix(inner, "typestate.unproven_requirement/")
	resource, expected, found := typestateDiagnosticParts(item.Message)
	if resource == "" || expected == "" || (!unproven && found == "") {
		return item
	}
	if unproven {
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "missing proof", Trust: "refuted", Message: fmt.Sprintf("no proof establishes `%s` in `%s` state at this call", resource, expected)}}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "unproven typestate requirement"}}
		item.Help = fmt.Sprintf("Establish that `%s` is in `%s` state before this call.", resource, expected)
		return item
	}
	if transition {
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("this transition requires `%s` to be in `%s`, but solved state is `%s`", resource, expected, found)}}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "invalid lifecycle transition"}}
		item.Help = fmt.Sprintf("Transition `%s` only when it is in `%s` state.", resource, expected)
		return item
	}
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("this call requires `%s` to be in `%s`, but solved state is `%s`", resource, expected, found)}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "invalid typestate requirement"}}
	item.Help = fmt.Sprintf("Call this operation only when `%s` is in `%s` state.", resource, expected)
	return item
}

func typestateDiagnosticParts(message string) (resource, expected, found string) {
	resourceStart := strings.Index(message, "resource `")
	if resourceStart < 0 {
		return "", "", ""
	}
	rest := message[resourceStart+len("resource `"):]
	end := strings.IndexByte(rest, '`')
	if end < 0 {
		return "", "", ""
	}
	resource = rest[:end]
	for _, part := range []struct {
		prefix      string
		destination *string
	}{{"expected `", &expected}, {"found `", &found}} {
		start := strings.Index(message, part.prefix)
		if start < 0 {
			if part.prefix == "found `" {
				continue
			}
			return "", "", ""
		}
		value := message[start+len(part.prefix):]
		end := strings.IndexByte(value, '`')
		if end < 0 {
			return "", "", ""
		}
		*part.destination = value[:end]
	}
	return resource, expected, found
}

func enrichUnreleasedLifecycleDiagnostic(item PublishedDiagnostic, transition []DiagnosticEvidence) PublishedDiagnostic {
	display := "resource"
	if start := strings.Index(item.Message, "`"); start >= 0 {
		if end := strings.Index(item.Message[start+1:], "`"); end >= 0 {
			display = item.Message[start+1 : start+1+end]
		}
	}
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "this call acquires `" + display + "` as connection:`open` and requires `closed` before local ownership ends"},
	}
	item.Evidence = append(item.Evidence, transition...)
	missing := "exit state still has `" + display + "` in protocol connection at `open`; no proof reaches `closed` or escapes ownership on every path"
	if len(transition) != 0 {
		missing = "exit state still has `" + display + "` in protocol connection at a non-final state; no proof reaches `closed` or escapes ownership on every path"
	}
	item.Evidence = append(item.Evidence, DiagnosticEvidence{Kind: "missing proof", Trust: "refuted", Message: missing})
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "resource acquired"}}
	if len(transition) != 0 {
		item.Labels = append(item.Labels, DiagnosticLabel{Span: transition[0].Span, Message: "lifecycle transition"})
	}
	item.Help = "Transition `" + display + "` to `closed` or escape ownership on every return path."
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
	for _, prefix := range []string{"advice.redundant_claim/", "advice.always_true_guard/", "lint.condition.redundant/", "send.isolation/", "effect.freeze.mutation/", "effect.lifecycle.unreleased/", "channel.send.closed/", "channel.close.closed/", "typestate.invalid_requirement/", "typestate.invalid_transition/"} {
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

// enrichDirectCallDiagnostic is the canonical source-facing form of a closed
// call-contract fact. The equation key remains the violation identity; this
// projection adds only source labels and presentation derived from the apply
// operation that produced that fact.
func enrichDirectCallDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
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
	code, _, subject, ok := directCallDiagnosticParts(item.Fact.Key)
	if !ok {
		return item
	}
	switch code {
	case "argument_type":
		return enrichCallArgumentDiagnostic(item, callee, subject, operands)
	case "too_few_args", "too_many_args":
		count, expected, got, ok := callArityMessage(item.Message)
		if !ok {
			return item
		}
		_ = count
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("call to %s passes %d argument%s", callee, got, plural(got))},
			{Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s declares %d parameter%s", callee, expected, plural(expected))},
		}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "call expression"}}
		if code == "too_few_args" {
			item.Help = "Pass the missing required arguments, or change the callee signature if fewer arguments are valid."
		} else {
			item.Help = "Remove the extra arguments, or change the callee signature if they are valid."
		}
	case "not_callable":
		value, found := strings.CutSuffix(item.Message, ", not callable")
		if !found {
			return item
		}
		_, value, found = strings.Cut(value, " is ")
		if !found {
			return item
		}
		item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has literal value %s", callee, value)}}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "call target"}}
		item.Help = fmt.Sprintf("Call a function value, or replace `%s` with a callable expression before this call.", callee)
	}
	return item
}

func enrichCallArgumentDiagnostic(item PublishedDiagnostic, callee, subject string, operands map[string]string) PublishedDiagnostic {
	message := item.Message
	start := strings.Index(message, " is ")
	end := strings.LastIndex(message, ", not ")
	if start < 0 || end <= start+4 {
		return item
	}
	argumentIndex, ok := callArgumentSubjectIndex(subject)
	if !ok {
		return item
	}
	value, expected := message[start+4:end], message[end+6:]
	argument := fmt.Sprintf("argument %d", argumentIndex)
	if display := operands[fmt.Sprintf("argument-display-%08d", argumentIndex-1)]; display != "" {
		argument += " (" + display + ")"
	}
	item.Message = fmt.Sprintf("%s is %s, not %s", argument, value, expected)
	valueFact := fmt.Sprintf("%s has type %s", argument, value)
	if callDiagnosticValueIsLiteral(value) {
		valueFact = fmt.Sprintf("%s has literal value %s", argument, value)
	}
	parameter := fmt.Sprintf("%s parameter %d", callee, argumentIndex)
	missingProof := fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", argument)
	if display := operands[fmt.Sprintf("argument-display-%08d", argumentIndex-1)]; display != "" {
		missingProof = fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", display)
	}
	if field, record := firstRequiredRecordField(expected); record {
		parameter += "." + field
		if strings.HasPrefix(value, "{") {
			missingProof = fmt.Sprintf("object literal does not provide field %q", field)
		}
	}
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: valueFact},
		{Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s expects %s", parameter, expected)},
		{Span: item.Span, Kind: "missing proof", Trust: "refuted", Message: missingProof},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "argument value " + value}}
	if display := operands[fmt.Sprintf("argument-display-%08d", argumentIndex-1)]; display != "" {
		item.Help = fmt.Sprintf("Pass `%s` as a value compatible with the parameter type, or change the callee signature if that argument is valid.", display)
	} else {
		item.Help = fmt.Sprintf("Pass a value for argument %d that satisfies the parameter type, or change the callee signature if that argument is valid.", argumentIndex)
	}
	return item
}

func directCallDiagnosticParts(key string) (code, operation, subject string, ok bool) {
	const prefix = "type.call.direct."
	if !strings.HasPrefix(key, prefix) {
		return "", "", "", false
	}
	rest := strings.TrimPrefix(key, prefix)
	code, rest, ok = strings.Cut(rest, "/")
	if !ok {
		return "", "", "", false
	}
	operation, subject, ok = strings.Cut(rest, "/")
	return code, operation, subject, ok && code != "" && operation != "" && subject != ""
}

func callArgumentSubjectIndex(subject string) (int, bool) {
	encoded, ok := strings.CutPrefix(subject, "argument-")
	if !ok || len(encoded) != 8 {
		return 0, false
	}
	index, err := strconv.Atoi(encoded)
	return index + 1, err == nil
}

func callArityMessage(message string) (callee string, expected, got int, ok bool) {
	before, after, found := strings.Cut(message, " expects ")
	if !found {
		return "", 0, 0, false
	}
	if _, err := fmt.Sscanf(after, "%d arguments, got %d", &expected, &got); err != nil {
		return "", 0, 0, false
	}
	return before, expected, got, true
}

func callDiagnosticValueIsLiteral(value string) bool {
	if value == "nil" || value == "true" || value == "false" || strings.HasPrefix(value, "\"") {
		return true
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func firstRequiredRecordField(value string) (string, bool) {
	if !strings.HasPrefix(value, "{") || !strings.HasSuffix(value, "}") {
		return "", false
	}
	field, _, found := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}"), ":")
	field = strings.TrimSpace(field)
	return field, found && field != ""
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func cloneFact(fact equation.Fact) equation.Fact {
	return equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...), Guards: append([]equation.Guard(nil), fact.Guards...)}
}

func mergeChildPublishedDiagnostics(items []PublishedDiagnostic, child map[string]PublishedDiagnostic) []PublishedDiagnostic {
	if len(child) == 0 {
		return items
	}
	for index := range items {
		if replacement, ok := child[items[index].Fact.Key]; ok {
			items[index] = replacement
		}
	}
	return items
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
	case strings.HasPrefix(key, "type.member.missing/"):
		return "type.member.missing"
	case strings.HasPrefix(key, "send.isolation/"):
		return "send.isolation"
	case strings.HasPrefix(key, "effect.freeze.mutation/"):
		return "effect.freeze.mutation"
	case strings.HasPrefix(key, "effect.lifecycle.unreleased/"):
		return "effect.lifecycle.unreleased"
	case strings.HasPrefix(key, "channel.send.closed/"):
		return "channel.send.closed"
	case strings.HasPrefix(key, "channel.close.closed/"):
		return "channel.close.closed"
	case strings.HasPrefix(key, "typestate.invalid_requirement/"):
		return "typestate.invalid_requirement"
	case strings.HasPrefix(key, "typestate.invalid_transition/"):
		return "typestate.invalid_transition"
	case strings.HasPrefix(key, "typestate.unproven_requirement/"):
		return "typestate.unproven_requirement"
	case strings.HasPrefix(key, "channel.select.exhaustiveness/"):
		return "channel.select.exhaustiveness"
	case strings.HasPrefix(key, "lint.union.exhaustiveness/"):
		return "lint.union.exhaustiveness"
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

// memberClosureWire keeps the lexical capability of a callable table member
// beside its sealed table value. The shape itself remains the authority for
// member presence and callable proof; this wire only supplies the private
// child-entry lens needed to evaluate a known local member body.
type memberClosureWire struct {
	Suffix string        `json:"suffix"`
	Handle closureHandle `json:"handle"`
}

type lexicalEvaluator struct {
	byPrototype       map[string]front.Compilation
	requiresBody      map[string]bool
	diagnosticSpans   map[string]wir.Span
	lifecycleEvidence map[string][]DiagnosticEvidence
	selectEvidence    map[string][]DiagnosticEvidence
	childPublished    map[string]PublishedDiagnostic
	ctx               context.Context
	table             *interproc.ProjectedTable
	coordinator       *interproc.RecursionCoordinator
	admissions        map[string]lexicalSCCAdmission
	run               *lexicalSCCRun
}

func (l *lexicalEvaluator) hasVarargBoundary(prototype string) bool {
	child, exists := l.byPrototype[prototype]
	if !exists {
		return false
	}
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg {
			return true
		}
	}
	return false
}

func (l *lexicalEvaluator) hasClaim(prototype string) bool {
	child, exists := l.byPrototype[prototype]
	if !exists {
		return true
	}
	for _, item := range child.Artifact.Equations {
		if item.Occurrence.Kind == "claim" {
			return true
		}
	}
	return false
}

func (l *lexicalEvaluator) hasTableAllocation(prototype string) bool {
	child, exists := l.byPrototype[prototype]
	if !exists {
		return true
	}
	for _, item := range child.Artifact.Equations {
		if item.Occurrence.Kind != "object-materialization" {
			continue
		}
		for _, operand := range item.Operands {
			if operand.Role == "kind" && string(operand.Term.Encoding) == "object-kind/table" {
				return true
			}
		}
	}
	return false
}

func newLexicalEvaluator(root front.Compilation) *lexicalEvaluator {
	table := interproc.NewProjectedTable()
	l := &lexicalEvaluator{byPrototype: make(map[string]front.Compilation), requiresBody: make(map[string]bool), diagnosticSpans: make(map[string]wir.Span), lifecycleEvidence: make(map[string][]DiagnosticEvidence), selectEvidence: make(map[string][]DiagnosticEvidence), childPublished: make(map[string]PublishedDiagnostic), ctx: context.Background(), table: table, coordinator: interproc.NewRecursionCoordinator(table, 256), admissions: make(map[string]lexicalSCCAdmission)}
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
		// Capture/writeback and escaping nested closures retain their established
		// body admission. Ordinary capture-free calls are admitted separately
		// only when their sealed result tuple has a caller-owned slot.
		required := len(compilation.Boundary.Captures) != 0
		if lexicalRequiresHeapTransport(compilation) && !compilation.RebindsBoundary {
			required = false
		}
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

// lexicalRequiresHeapTransport identifies child effects the value-only bridge
// cannot preserve. Those bodies stay on the established root path unless they
// also rebind a boundary cell, which is the one effect this bridge transports.
func lexicalRequiresHeapTransport(compilation front.Compilation) bool {
	for _, item := range compilation.Artifact.Equations {
		switch item.Occurrence.Kind {
		case "apply", "external-call", "index-mutation", "path-replacement", "path-invalidation":
			return true
		}
	}
	return false
}

func (l *lexicalEvaluator) evaluate(compilation front.Compilation, entryValue []byte) (equation.OutputClosure, int, error) {
	if l == nil || !compilation.Body.Valid() || len(compilation.Artifact.Equations) == 0 {
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: incomplete lexical body admission")
	}
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
	ctx := l.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	evaluation, err := vm.Evaluate(ctx, bound, []string{"published"})
	if err != nil {
		return equation.OutputClosure{}, 0, err
	}
	return evaluation.Closure, evaluation.Transactions, nil
}

func encodeChildEntry(seeds []entrySeed) ([]byte, error) {
	wire := childEntryWire{Version: 1, Seeds: append([]entrySeed(nil), seeds...)}
	sort.Slice(wire.Seeds, func(i, j int) bool { return wire.Seeds[i].Term < wire.Seeds[j].Term })
	for index := range wire.Seeds {
		if !validEntrySeed(wire.Seeds[index]) || (index > 0 && wire.Seeds[index-1].Term == wire.Seeds[index].Term) {
			return nil, fmt.Errorf("engine: malformed child entry seed")
		}
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("engine: encode child entry: %w", err)
	}
	return append([]byte("front/child-entry/v1/"), encoded...), nil
}

// importEntryValue makes project-resolved exports ordinary entry seeds. The
// child-entry packet is the one entry transport in the engine; imports use an
// import term rather than a path term because no source-local require alias is
// authoritative until the require call's result slot is evaluated.
func importEntryValue(imports map[string]typ.Type) ([]byte, error) {
	if len(imports) == 0 {
		return []byte(entryValue), nil
	}
	seeds := make([]entrySeed, 0, len(imports))
	for modulePath, exported := range imports {
		if modulePath == "" || exported == nil {
			continue
		}
		resolved := unwrap.Alias(exported)
		if resolved == nil || resolved.Kind() == kind.Any || resolved.Kind() == kind.Unknown {
			continue
		}
		encoded, ok := shapefact.EncodeTarget(exported)
		if !ok {
			continue
		}
		seeds = append(seeds, entrySeed{Term: importEntryTerm(modulePath), Value: encoded})
	}
	if len(seeds) == 0 {
		return []byte(entryValue), nil
	}
	encoded, err := encodeChildEntry(seeds)
	if err != nil {
		return nil, fmt.Errorf("engine: encode import entry: %w", err)
	}
	return encoded, nil
}

func importEntryTerm(modulePath string) string {
	return "import/" + base64.RawURLEncoding.EncodeToString([]byte(modulePath))
}

func validEntrySeed(seed entrySeed) bool {
	if seed.Term == "" || len(seed.Value) == 0 {
		return false
	}
	if strings.HasPrefix(seed.Term, "path/") || strings.HasPrefix(seed.Term, "temp/") {
		return true
	}
	return strings.HasPrefix(seed.Term, "import/") && strings.TrimPrefix(seed.Term, "import/") != ""
}

func entryKernel(operation equation.BoundEquation, _ equation.Partition) (equation.TransactionResult, error) {
	var entryValue []byte
	declaredRoots := make(map[string][]byte)
	declaredTypes := make(map[string][]byte)
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "entry":
			if entryValue != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: duplicate entry operand")
			}
			entryValue = operand.Value
		case strings.HasPrefix(operand.Role, "declared-root-"):
			declaredRoots[strings.TrimPrefix(operand.Role, "declared-root-")] = operand.Value
		case strings.HasPrefix(operand.Role, "declared-type-"):
			declaredTypes[strings.TrimPrefix(operand.Role, "declared-type-")] = operand.Value
		default:
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed entry operand %q", operand.Role)
		}
	}
	if entryValue == nil || len(declaredRoots) != len(declaredTypes) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed entry operands")
	}
	const prefix = "front/child-entry/v1/"
	var wire childEntryWire
	if strings.HasPrefix(string(entryValue), prefix) {
		if err := json.Unmarshal(entryValue[len(prefix):], &wire); err != nil || wire.Version != 1 {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry wire")
		}
	}
	values := make([]equation.Fact, 0, len(wire.Seeds)+len(declaredRoots)*3)
	seen := make(map[string]bool, len(wire.Seeds))
	for _, seed := range wire.Seeds {
		if !validEntrySeed(seed) || seen[seed.Term] {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry seed")
		}
		seen[seed.Term] = true
		values = append(values,
			equation.Fact{Key: "value/" + seed.Term + "/entry", Value: append([]byte(nil), seed.Value...)},
			equation.Fact{Key: "epoch/" + seed.Term + "/entry", Value: []byte("entry")},
		)
	}
	for name, root := range declaredRoots {
		declared, ok := shapefact.DecodeTarget(declaredTypes[name])
		if !ok || declared == nil || (!strings.HasPrefix(string(root), "path/") && !strings.HasPrefix(string(root), "temp/")) {
			// Boundary type metadata is optional for a lexical body.  An invalid
			// descriptive entry row cannot become a value fact; skip it rather
			// than rejecting an otherwise independently evaluable child.
			continue
		}
		// A concrete seed is the value authority.  Declarations fill only absent
		// entries, which prevents a caller's channel identity from being replaced
		// by a callee-local symbolic one.
		if seen[string(root)] {
			continue
		}
		values = append(values,
			equation.Fact{Key: "value/" + string(root) + "/entry", Value: []byte("scalar/top")},
			equation.Fact{Key: "epoch/" + string(root) + "/entry", Value: []byte("entry")},
			equation.Fact{Key: "type/" + string(root) + "/entry", Value: append([]byte(nil), declaredTypes[name]...)},
		)
		if _, channel := ambient.ChannelPayloadType(declared); channel {
			identity := []byte("scalar/channel-entry/" + fmt.Sprintf("%x", operation.Target.Body) + "/" + base64.RawURLEncoding.EncodeToString(root))
			values = append(values, equation.Fact{Key: "identity/" + string(root) + "/entry", Value: identity})
		}
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
	return handle, json.Unmarshal(encoded, &handle) == nil && validClosureHandle(handle)
}

func validClosureHandle(handle closureHandle) bool {
	if handle.Prototype == "" {
		return false
	}
	for _, capture := range handle.Captures {
		if !strings.HasPrefix(capture, "path/") && !strings.HasPrefix(capture, "temp/") && !strings.HasPrefix(capture, "scalar/") {
			return false
		}
	}
	return true
}

func memberClosuresFor(term []byte, partition equation.Partition) []memberClosureWire {
	prefix := "member-closure/" + string(term) + "/"
	bySuffix := make(map[string]struct {
		key  string
		wire memberClosureWire
	})
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		var wire memberClosureWire
		if json.Unmarshal(fact.Value, &wire) != nil || wire.Suffix == "" || !validClosureHandle(wire.Handle) {
			continue
		}
		prior, exists := bySuffix[wire.Suffix]
		if !exists || fact.Key > prior.key {
			bySuffix[wire.Suffix] = struct {
				key  string
				wire memberClosureWire
			}{key: fact.Key, wire: wire}
		}
	}
	keys := make([]string, 0, len(bySuffix))
	for suffix := range bySuffix {
		keys = append(keys, suffix)
	}
	sort.Strings(keys)
	out := make([]memberClosureWire, 0, len(keys))
	for _, suffix := range keys {
		out = append(out, bySuffix[suffix].wire)
	}
	return out
}

func projectMemberClosures(target string, source []byte, operation string, partition equation.Partition) ([]equation.Fact, error) {
	wires := memberClosuresFor(source, partition)
	values := make([]equation.Fact, 0, len(wires))
	for index, wire := range wires {
		encoded, err := json.Marshal(wire)
		if err != nil {
			return nil, err
		}
		values = append(values, equation.Fact{Key: fmt.Sprintf("member-closure/%s/%s/%08d", target, operation, index), Value: encoded})
	}
	return values, nil
}

func projectSealedTableMemberClosures(target string, tableValue []byte, operation string, partition equation.Partition) ([]equation.Fact, error) {
	table, ok := shapefact.DecodeTable(tableValue)
	if !ok {
		return nil, nil
	}
	values := make([]equation.Fact, 0)
	for _, member := range table.Members {
		if !member.Present {
			continue
		}
		handle, found := closureHandleFor([]byte(member.Value), partition)
		if !found {
			continue
		}
		encoded, err := json.Marshal(memberClosureWire{Suffix: member.Suffix, Handle: handle})
		if err != nil {
			return nil, err
		}
		values = append(values, equation.Fact{Key: fmt.Sprintf("member-closure/%s/%s/literal-%08d", target, operation, len(values)), Value: encoded})
	}
	return values, nil
}

func methodClosureHandleFor(receiver []byte, method string, partition equation.Partition) (closureHandle, bool) {
	for _, wire := range memberClosuresFor(receiver, partition) {
		if wire.Suffix == "."+method {
			return wire.Handle, true
		}
	}
	return closureHandle{}, false
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
	arguments := operands.arguments
	if operands.receiver != nil {
		arguments = append([][]byte{operands.receiver}, arguments...)
	}
	if operands.spread || len(arguments) != len(child.Boundary.Parameters) || len(handle.Captures) != len(child.Boundary.Captures) {
		return equation.TransactionResult{}, fmt.Errorf("engine: unsupported exact lexical boundary for %q", handle.Prototype)
	}
	seeds := make([]entrySeed, 0, len(operands.arguments)+len(handle.Captures))
	for index, parameter := range child.Boundary.Parameters {
		if parameter.Vararg {
			return equation.TransactionResult{}, fmt.Errorf("engine: vararg lexical boundary is unsupported")
		}
		value, known := resolveKnownCurrentValue(arguments[index], partition)
		if !known || isUnknownScalar(value) {
			return equation.TransactionResult{}, fmt.Errorf("engine: incomplete lexical argument %d", index)
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: value})
	}
	for index, capture := range child.Boundary.Captures {
		value, known := resolveKnownCurrentValue([]byte(handle.Captures[index]), partition)
		if !known || isUnknownScalar(value) {
			return equation.TransactionResult{}, fmt.Errorf("engine: incomplete lexical capture %q", capture.Name)
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(capture.Symbol), Value: value})
	}
	entry, err := encodeChildEntry(seeds)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	closure, err := l.resolveSCCChild(child, entry, seeds, arguments, handle, operands, operation.Target.Name)
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
	// Publication is the only child return authority. Its member-capability
	// facts retain local closure handles for sealed table results without
	// changing the table's ordinary callable/member facts.
	for _, fact := range closure.Values {
		const prefix = "return-member-closure/"
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(fact.Key, prefix), "/")
		if len(parts) != 3 {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child member closure")
		}
		resultIndex, err := strconv.Atoi(parts[1])
		if err != nil || resultIndex < 0 || resultIndex >= len(returns) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child member closure result")
		}
		var wire memberClosureWire
		if json.Unmarshal(fact.Value, &wire) != nil || wire.Suffix == "" || !validClosureHandle(wire.Handle) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child member closure handle")
		}
		rebound, values, err := rebindEscapingClosure(operation, child, arguments, handle, closure, wire.Handle)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		wire.Handle = rebound
		encoded, err := json.Marshal(wire)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		projected.Values = append(projected.Values, values...)
		projected.Values = append(projected.Values, equation.Fact{Key: fmt.Sprintf("call-member-closure/%s/%08d/%s", strings.TrimPrefix(operation.Target.Name, "call/"), resultIndex, parts[2]), Value: encoded})
	}
	for index, capture := range child.Boundary.Captures {
		value, found := latestClosedValue([]byte(boundaryTerm(capture.Symbol)), closure.Values)
		if !found {
			return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q omitted capture cell %q", handle.Prototype, capture.Name)
		}
		// A capture cell may be the same caller cell as another capture or a
		// formal.  The entry contains the complete pre-call relation below, so
		// a write through this lens must update every possible caller alias.
		// This is a weak update at the boundary: each alias is retained as a
		// separately owned caller fact rather than selecting one arbitrary
		// spelling of an aliased cell.
		aliases := []string{handle.Captures[index]}
		parameterAliasesCapture := false
		for parameterIndex := range child.Boundary.Parameters {
			parameterAliasesCapture = parameterAliasesCapture || string(arguments[parameterIndex]) == handle.Captures[index]
		}
		for other, term := range handle.Captures {
			if other == index {
				continue
			}
			if term == handle.Captures[index] {
				aliases = append(aliases, term)
			}
		}
		for _, term := range arguments {
			if string(term) == handle.Captures[index] {
				aliases = append(aliases, string(term))
			}
		}
		sort.Strings(aliases)
		aliases = uniqueStrings(aliases)
		// If a formal aliases this capture, the parameter cell is the latest
		// write authority for this call. Its rebinding below performs the weak
		// caller update without creating two conflicting facts for one target.
		if !parameterAliasesCapture {
			for _, alias := range aliases {
				projected.Values = append(projected.Values, equation.Fact{Key: "value/" + alias + "/" + operation.Target.Name, Value: value})
			}
		}
	}
	// Parameter writes only escape when the caller supplied that parameter as
	// a capture-cell alias.  Rebind those writes through every matching capture
	// lens; ordinary pass-by-value parameters deliberately have no writeback.
	for parameterIndex, parameter := range child.Boundary.Parameters {
		value, found := latestClosedValue([]byte(boundaryTerm(parameter.Symbol)), closure.Values)
		if !found {
			return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q omitted parameter cell %q", handle.Prototype, parameter.Name)
		}
		for _, captureTerm := range handle.Captures {
			if string(arguments[parameterIndex]) == captureTerm {
				projected.Values = append(projected.Values, equation.Fact{Key: "value/" + captureTerm + "/" + operation.Target.Name, Value: value})
			}
		}
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
				returned, values, err := rebindEscapingClosure(operation, child, arguments, handle, closure, returned)
				if err != nil {
					return equation.TransactionResult{}, err
				}
				encoded, marshalErr := json.Marshal(returned)
				if marshalErr != nil {
					return equation.TransactionResult{}, marshalErr
				}
				projected.Values = append(projected.Values, values...)
				projected.Values = append(projected.Values, equation.Fact{Key: "call-closure/" + strings.TrimPrefix(operation.Target.Name, "call/") + "/00000000", Value: encoded})
				break
			}
		}
	}
	// A nested body's residual has a different entry lens and cannot be
	// attributed to this call boundary. It is demanded at its own closure site;
	// only direct-child residuals can cross this outcome boundary.
	if len(child.Nested) == 0 && l.requiresBody[handle.Prototype] {
		// Child diagnostics are projected with a body-qualified key. The final
		// Check closure is still the sole publication point, while the qualified
		// key lets DiagnosticSpans retain the child operation's source location.
		body := fmt.Sprintf("%x", child.Body)
		spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, closure.Diagnostics)
		for _, diagnostic := range closure.Diagnostics {
			key := "child/" + body + "/" + diagnostic.Key
			projected.Diagnostics = append(projected.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
			if span, ok := spans[diagnostic.Key]; ok {
				l.diagnosticSpans[key] = span
			}
		}
	}
	return equation.TransactionResult{Complete: true, Closure: projected}, nil
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := items[:1]
	for _, item := range items[1:] {
		if item != out[len(out)-1] {
			out = append(out, item)
		}
	}
	return out
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

func rebindEscapingClosure(operation equation.BoundEquation, child front.Compilation, arguments [][]byte, parent closureHandle, closure equation.OutputClosure, returned closureHandle) (closureHandle, []equation.Fact, error) {
	if !validClosureHandle(returned) {
		return closureHandle{}, nil, fmt.Errorf("malformed escaping closure handle")
	}
	projected := make([]equation.Fact, 0)
	for captureIndex, capture := range returned.Captures {
		rebound := false
		for parameterIndex, parameter := range child.Boundary.Parameters {
			if capture == boundaryTerm(parameter.Symbol) {
				returned.Captures[captureIndex] = string(arguments[parameterIndex])
				rebound = true
				break
			}
		}
		if rebound {
			continue
		}
		for boundaryIndex, boundary := range child.Boundary.Captures {
			if capture == boundaryTerm(boundary.Symbol) {
				returned.Captures[captureIndex] = parent.Captures[boundaryIndex]
				rebound = true
				break
			}
		}
		if rebound || strings.HasPrefix(capture, "scalar/") {
			continue
		}
		// A local cell captured by an escaping closure has no caller spelling.
		// Give it a fresh caller-owned lens and seed that lens from the completed
		// child closure. Sibling closures from the same returned table use the
		// same lens, so their later writeback composes in call order.
		value, found := latestClosedValue([]byte(capture), closure.Values)
		if !found {
			return closureHandle{}, nil, fmt.Errorf("escaping closure omitted capture cell %q", capture)
		}
		lens := "temp/capture/" + operation.Target.Name + "/" + base64.RawURLEncoding.EncodeToString([]byte(capture))
		returned.Captures[captureIndex] = lens
		projected = append(projected, equation.Fact{Key: "value/" + lens + "/" + operation.Target.Name, Value: value})
	}
	return returned, projected, nil
}

// allocationTemplateKernel admits only a sealed, complete allocation graph.
// Its identity transport is structural at this walking stage; it deliberately
// does not turn an absent/nil member into a value fact.
func allocationTemplateKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "site", "result", "kind")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	kind := strings.TrimPrefix(string(operands["kind"]), "allocation-kind/")
	if kind == string(operands["kind"]) || kind == "" {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed allocation kind")
	}
	complete, decomposable := true, true
	children := make([]string, 0)
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "open-tail":
			if string(operand.Value) != "scalar/bool/false" {
				complete, decomposable = false, false
			}
		case strings.HasPrefix(operand.Role, "value-"):
			if strings.HasPrefix(string(operand.Value), "scalar/") {
				continue
			}
			children = append(children, string(operand.Value))
			decomposable = false
		}
	}
	if kind != "table" {
		decomposable = false
	}
	fact, err := encodePlacementAllocation(placementAllocationFact{
		Identity: placementAllocationIdentity(operation), Result: string(operands["result"]),
		Kind: "lua." + kind, Complete: complete, Decomposable: decomposable, Children: children,
	})
	if err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
		Key: placementAllocationPrefix + operation.Target.Name, Value: fact,
	}}}}, nil
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
	// Lifecycle epochs are self-contained proof facts: a channel created and
	// consumed within a declared body needs no caller value to establish its
	// identity. Publish only those closed facts for otherwise uncalled bodies;
	// all ordinary diagnostics retain the existing demand-driven boundary.
	if child.Cyclic == nil && (childHasChannelLifecycle(child) || childHasResourceLifecycle(child)) {
		seeds := make([]entrySeed, 0, len(child.Boundary.Parameters)+len(child.Boundary.Captures))
		for _, parameter := range child.Boundary.Parameters {
			seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: []byte("scalar/top")})
		}
		for _, capture := range child.Boundary.Captures {
			seeds = append(seeds, entrySeed{Term: boundaryTerm(capture.Symbol), Value: []byte("scalar/top")})
		}
		entry, entryErr := encodeChildEntry(seeds)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		outcome, _, evaluateErr := lexical.evaluate(child, entry)
		if evaluateErr != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: lifecycle child %q: %w", prototype, evaluateErr)
		}
		body := fmt.Sprintf("%x", child.Body)
		for _, diagnostic := range outcome.Diagnostics {
			if !isChannelLifecycleDiagnostic(diagnostic.Key) && !isResourceTypestateDiagnostic(diagnostic.Key) {
				continue
			}
			key := "child/" + body + "/" + diagnostic.Key
			closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
			spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, outcome.Diagnostics)
			if span, ok := spans[diagnostic.Key]; ok {
				lexical.diagnosticSpans[key] = span
			}
		}
		for _, diagnostic := range resourceUnreleasedDiagnostics(child, outcome) {
			key := "child/" + body + "/" + diagnostic.Key
			closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
			acquire := strings.TrimPrefix(diagnostic.Key, "effect.lifecycle.unreleased/")
			if span, ok := child.CallSpans[acquire+"/call"]; ok {
				lexical.diagnosticSpans[key] = span
			}
			if close := firstResourceCloseOperation(child); close != "" {
				if span, ok := child.CallSpans[close+"/call"]; ok {
					lexical.lifecycleEvidence[key] = []DiagnosticEvidence{{Span: span, Kind: "abstract fact", Trust: "proven", Message: "this call transitions `" + resourceDisplay(child, acquire) + "` in protocol connection from `open` to `closed` on a reachable path"}}
				}
			}
		}
	}
	if child.Cyclic == nil && childHasSelect(child) {
		body := fmt.Sprintf("%x", child.Body)
		entry, entryErr := encodeChildEntry(nil)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		outcome, _, evaluateErr := lexical.evaluate(child, entry)
		if evaluateErr != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: select child %q: %w", prototype, evaluateErr)
		}
		spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, outcome.Diagnostics)
		childPublished := publishedDiagnostics(child.Artifact, outcome, spans, child.ClaimTargetSpans, child.CallSpans, nil, nil)
		for _, diagnostic := range outcome.Diagnostics {
			if !strings.HasPrefix(diagnostic.Key, "type.assignment/") && !strings.HasPrefix(diagnostic.Key, "type.member.missing/") {
				continue
			}
			key := "child/" + body + "/" + diagnostic.Key
			closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
			if span, ok := spans[diagnostic.Key]; ok {
				lexical.diagnosticSpans[key] = span
			}
			for _, item := range childPublished {
				if item.Fact.Key != diagnostic.Key {
					continue
				}
				item = enrichChildSelectDiagnostic(item, child, child.ClaimTargetSpans)
				item.Fact.Key = key
				lexical.childPublished[key] = item
				break
			}
		}
		consumers := append(channelSelectCoverageConsumers(child), channelSelectUnionConsumers(child)...)
		for _, consumer := range consumers {
			key := "child/" + body + "/" + consumer.Key
			closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: []byte(consumer.Message)})
			lexical.diagnosticSpans[key] = consumer.Span
			lexical.selectEvidence[key] = consumer.Evidence
		}
	}
	return equation.TransactionResult{Complete: true, Closure: closure}, nil
}

func enrichChildSelectDiagnostic(item PublishedDiagnostic, child front.Compilation, targets map[string]wir.Span) PublishedDiagnostic {
	name, assignment := strings.CutPrefix(item.Fact.Key, "type.assignment/")
	if !assignment || len(item.Evidence) != 0 {
		return item
	}
	var operation equation.Equation
	found := false
	for _, candidate := range child.Artifact.Equations {
		if candidate.Occurrence.Kind == "claim" && candidate.Target.Name == name {
			operation, found = candidate, true
			break
		}
	}
	if !found {
		return item
	}
	source, display := "value", "value"
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "source-display":
			source = string(operand.Term.Encoding)
		case "display":
			display = string(operand.Term.Encoding)
		}
	}
	const prefix = " because it is "
	_, after, ok := strings.Cut(item.Message, prefix)
	if !ok {
		return item
	}
	valueType, _, ok := strings.Cut(after, ", not ")
	if !ok {
		return item
	}
	declared := claimDeclaredDisplay(operation, nil)
	if declared == "" {
		return item
	}
	target := targets[name]
	if !target.Valid() {
		target = item.Span
	}
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type %s", source, valueType)}, {Span: target, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueType}, {Span: target, Message: "declared type " + declared}}
	item.Help = "Use a value compatible with the expected type, or change the target type if `" + display + "` is valid."
	return item
}

func childHasChannelLifecycle(child front.Compilation) bool {
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "callee-display" && string(operand.Term.Encoding) == "channel.new" {
				return true
			}
		}
	}
	return false
}

func childHasSelect(child front.Compilation) bool {
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind == "channel-select" {
			return true
		}
	}
	return false
}

func childHasResourceLifecycle(child front.Compilation) bool {
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "callee-display" && (string(operand.Term.Encoding) == "resource.connect" || string(operand.Term.Encoding) == "resource.query") {
				return true
			}
		}
	}
	return false
}

func resourceUnreleasedDiagnostics(child front.Compilation, closure equation.OutputClosure) []equation.Fact {
	if childHasPCall(child) && childHasNestedResourceClose(child) {
		return nil
	}
	latest := make(map[string]equation.Fact)
	for _, fact := range closure.Values {
		if !strings.HasPrefix(fact.Key, resourceLifecyclePrefix) {
			continue
		}
		identity := strings.TrimPrefix(fact.Key, resourceLifecyclePrefix)
		slash := strings.LastIndexByte(identity, '/')
		if slash < 0 {
			continue
		}
		identity = identity[:slash]
		if previous, found := latest[identity]; !found || fact.Key > previous.Key {
			latest[identity] = fact
		}
	}
	var diagnostics []equation.Fact
	for encoded, state := range latest {
		identity, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || !isResourceIdentity(identity) || string(state.Value) != "open" {
			continue
		}
		acquire := strings.TrimPrefix(string(identity), "scalar/resource/")
		display := resourceDisplay(child, acquire)
		message := fmt.Sprintf("resource `%s` remains in connection state `open` at function exit; expected `closed`", display)
		if childHasResourceClose(child) {
			message = fmt.Sprintf("resource `%s` remains in a non-final connection state at function exit; expected `closed`", display)
		}
		diagnostics = append(diagnostics, equation.Fact{Key: "effect.lifecycle.unreleased/" + acquire, Value: []byte(message)})
	}
	return diagnostics
}

func childHasResourceClose(child front.Compilation) bool {
	for _, operation := range child.Artifact.Equations {
		for _, operand := range operation.Operands {
			if operand.Role == "callee-display" && string(operand.Term.Encoding) == "resource.close" {
				return true
			}
		}
	}
	return false
}

func firstResourceCloseOperation(child front.Compilation) string {
	for _, operation := range child.Artifact.Equations {
		for _, operand := range operation.Operands {
			if operand.Role == "callee-display" && string(operand.Term.Encoding) == "resource.close" {
				return operation.Target.Name
			}
		}
	}
	return ""
}

func childHasPCall(child front.Compilation) bool {
	for _, operation := range child.Artifact.Equations {
		for _, operand := range operation.Operands {
			if operand.Role == "callee-display" && string(operand.Term.Encoding) == "pcall" {
				return true
			}
		}
	}
	return false
}

func childHasNestedResourceClose(child front.Compilation) bool {
	for _, nested := range child.Nested {
		if childHasResourceClose(nested) || childHasNestedResourceClose(nested) {
			return true
		}
	}
	return false
}

func resourceDisplay(child front.Compilation, acquire string) string {
	application := "call/" + acquire
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		matches, target := false, ""
		for _, operand := range operation.Operands {
			if operand.Role == "application" {
				matches = string(operand.Term.Encoding) == application
			}
			if operand.Role == "target-00000000" {
				target = string(operand.Term.Encoding)
			}
		}
		if !matches || target == "" {
			continue
		}
		index := strings.LastIndex(target, "/path/")
		if index < 0 {
			continue
		}
		path := target[index+1:]
		for _, write := range child.Artifact.Equations {
			if write.Occurrence.Kind != "environment-write" {
				continue
			}
			var writeTarget, display string
			for _, operand := range write.Operands {
				if operand.Role == "target" {
					writeTarget = string(operand.Term.Encoding)
				}
				if operand.Role == "display" {
					display = string(operand.Term.Encoding)
				}
			}
			if writeTarget == path && display != "" {
				return display
			}
		}
	}
	return "resource"
}

func writeKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "target", "display", "value", "read-before", "absence")
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
	values := []equation.Fact{
		{Key: "value/" + target + "/" + operation.Target.Name, Value: value},
		{Key: "epoch/" + target + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
	}
	for _, prefix := range []string{"identity/", "type/", "select/origin/"} {
		if inherited, ok := currentEpochFact(prefix, operands["value"], partition); ok {
			values = append(values, equation.Fact{Key: prefix + target + "/" + operation.Target.Name, Value: inherited})
		}
	}
	identity, hasIdentity := tableIdentityForTerm(operands["value"], partition)
	if !hasIdentity && (shapefact.IsTable(value) || string(value) == "scalar/table") {
		identity, hasIdentity = sealedTableIdentity(operation), true
	}
	if hasIdentity {
		values = append(values, heapIdentityFact(target, operation.Target.Name, identity))
		if table, ok := shapefact.DecodeTable(value); ok {
			for _, member := range table.Members {
				memberValue := []byte("scalar/nil")
				if member.Present {
					memberValue = []byte(member.Value)
				}
				values = append(values, heapMemberFact(identity, member.Suffix, operation.Target.Name, memberValue))
			}
		}
	}
	if root, suffix, ok := tableAddress([]byte(target)); ok && suffix != "" {
		if parent, found := tableIdentityForTerm(root, partition); found {
			values = append(values, heapMemberFact(parent, suffix, operation.Target.Name, value))
			if hasIdentity {
				values = append(values, heapMemberIdentityFact(parent, suffix, operation.Target.Name, identity))
			}
		}
	}
	if isChannelIdentity(value) {
		values = append(values, equation.Fact{
			Key:   "effect.lifecycle.channel.display/" + base64.RawURLEncoding.EncodeToString(operands["target"]) + "/" + operation.Target.Name,
			Value: append([]byte(nil), operands["display"]...),
		})
	}
	if handle, ok := closureHandleFor(operands["value"], partition); ok {
		encoded, marshalErr := json.Marshal(handle)
		if marshalErr != nil {
			return equation.TransactionResult{}, marshalErr
		}
		values = append(values, equation.Fact{Key: "closure/" + target + "/" + operation.Target.Name, Value: encoded})
	}
	allocationResult := []byte(target)
	for _, operand := range operation.Operands {
		if operand.Role == "allocation-result" {
			allocationResult = operand.Value
			break
		}
	}
	if allocation, found := placementAllocationForTerm(allocationResult, partition); found {
		values = append(values, placementBindingFact(target, operation.Target.Name, allocation.Identity))
	}
	if _, suffix, member := tableAddress([]byte(target)); member && suffix != "" {
		if allocation, found := placementAllocationForTerm(allocationResult, partition); found {
			values = append(values, placementBlockerFact(allocation.Identity, operation.Target.Name, "member-store"))
		}
	}
	memberClosures, err := projectMemberClosures(target, operands["value"], operation.Target.Name, partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	values = append(values, memberClosures...)
	literalMemberClosures, err := projectSealedTableMemberClosures(target, operands["value"], operation.Target.Name, partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	values = append(values, literalMemberClosures...)
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
			if errors.Is(er, errUnknownScalar) {
				value = []byte("scalar/top")
			} else {
				err = er
				value = []byte("scalar/number/" + strconv.FormatInt(n, 10))
			}
		case wir.UnNeg:
			n, er := scalarNumber(operand)
			if errors.Is(er, errUnknownScalar) {
				value = []byte("scalar/top")
			} else {
				err = er
				value = numberValue(-n)
			}
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
	values := []equation.Fact{{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: value}}
	if wir.Op(kind) == wir.OpBinOp && (wir.Operator(op) == wir.BinEq || wir.Operator(op) == wir.BinNe) {
		for _, role := range []string{"left", "right"} {
			if allocation, found := placementAllocationForTerm(by[role], partition); found {
				values = append(values, placementBlockerFact(allocation.Identity, operation.Target.Name, "identity-compare"))
			}
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
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
	if root, suffix, ok := tableAddress(operands["target"]); ok && suffix != "" {
		if identity, found := tableIdentityForTerm(root, partition); found {
			result.Closure.Values = append(result.Closure.Values, heapMemberFact(identity, suffix, operation.Target.Name, value))
			if memberIdentity, found := tableIdentityForTerm(operands["value"], partition); found {
				result.Closure.Values = append(result.Closure.Values, heapMemberIdentityFact(identity, suffix, operation.Target.Name, memberIdentity))
			}
		}
	}
	if isChannelIdentity(value) {
		result.Closure.Values = append(result.Closure.Values, equation.Fact{
			Key:   "effect.lifecycle.channel.display/" + base64.RawURLEncoding.EncodeToString(operands["target"]) + "/" + operation.Target.Name,
			Value: append([]byte(nil), operands["display"]...),
		})
	}
	return result, nil
}

// dynamicIndexReadKernel projects only an exact-key heap fact.  Unknown keys
// retain Top: selecting an arbitrary member from a sealed table is not proof.
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
	value := []byte("scalar/top")
	values := []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: value}, {Key: "epoch/" + target + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)}}
	if identity, found := tableIdentityForTerm(operands["container"], partition); found {
		key, keyErr := resolveCurrentValue(operands["key"], partition)
		if suffix, exact := tableMemberSuffix(key, []byte("suffix/")); keyErr == nil && exact {
			if member, found := heapMemberCurrent(heapMemberPrefix, identity, suffix, partition); found {
				values[0].Value = member
			}
			if memberIdentity, found := heapMemberCurrent(heapMemberIdentityPrefix, identity, suffix, partition); found {
				values = append(values, heapIdentityFact(target, operation.Target.Name, memberIdentity))
			}
		}
	}
	if allocation, found := placementAllocationForTerm(operands["container"], partition); found {
		values = append(values, placementBlockerFact(allocation.Identity, operation.Target.Name, "dynamic-index"))
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
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
	if kind == "claim-kind/3" && available && memberMissing(value) {
		closure.Diagnostics = []equation.Fact{{
			Key:   "type.member.missing/" + operation.Target.Name,
			Value: []byte(memberMissingMessage(sourceDisplay, value)),
		}}
	} else if kind == "claim-kind/3" && available && (anySource && assignmentTargetRequiresProof(targetType) || assignmentMismatchProven(value, targetType) || shapeRelation == shapeRefuted) {
		message := assignmentMismatchMessage(sourceDisplay, value, targetType)
		if declared := boundClaimDeclaredDisplay(operation, targetType); declared != "" {
			message = "cannot assign " + sourceDisplay + " because it is " + assignmentValueType(value) + ", not " + declared
		}
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
	if source, ok := shapefact.DecodeTarget(value); ok {
		if subtype.IsSubtype(source, target) {
			return shapeProven
		}
		return shapeRefuted
	}
	// Structural claims compare the fully instantiated alias target. A named
	// alias can hide another generic instantiation (DoubleBox<T> = Box<Box<T>>),
	// so stopping after Alias would discard the only concrete record shape that
	// can prove the claim. Expand first, then retain the existing alias policy;
	// malformed or recursive forms still fall through to unknown.
	target = unwrap.Alias(subst.ExpandInstantiated(target))
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

// claimDeclaredDisplay preserves a discriminated record's user-facing alias
// spelling when the front has already expanded that alias for checking.
func claimDeclaredDisplay(operation equation.Equation, fallback []byte) string {
	for _, operand := range operation.Operands {
		if operand.Role != "shape-target" {
			continue
		}
		return declaredDisplayFromShape(operand.Term.Encoding, string(fallback))
	}
	return declaredDisplayFromShape(nil, string(fallback))
}

func boundClaimDeclaredDisplay(operation equation.BoundEquation, fallback string) string {
	for _, operand := range operation.Operands {
		if operand.Role == "shape-target" {
			return declaredDisplayFromShape(operand.Value, fallback)
		}
	}
	return declaredDisplayFromShape(nil, fallback)
}

func declaredDisplayFromShape(shape []byte, fallback string) string {
	if target, ok := shapefact.DecodeTarget(shape); ok {
		kindType, found := variant.FieldAtPath(target, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}})
		if literal, literalOK := kindType.(*typ.Literal); found && literalOK && literal.Base == kind.String {
			if name, nameOK := literal.Value.(string); nameOK && name != "" {
				return strings.ToUpper(name[:1]) + name[1:]
			}
		}
	}
	declared, err := strconv.Unquote(strings.TrimPrefix(fallback, "claim-type/"))
	if err != nil {
		return ""
	}
	return declared
}

func assignmentValueType(value []byte) string {
	switch {
	case func() bool { _, ok := shapefact.DecodeTarget(value); return ok }():
		valueType, _ := shapefact.DecodeTarget(value)
		return typeformat.Short(valueType)
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
	for _, fact := range partition.AllValues() {
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
	operands, err := requiredOperandsByRole(operation.Operands, "container", "key", "suffix", "value")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	result, err := frozenMutationDiagnostic(operation, partition, "assignment")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if identity, found := tableIdentityForTerm(operands["container"], partition); found {
		key, keyErr := resolveCurrentValue(operands["key"], partition)
		if suffix, exact := tableMemberSuffix(key, operands["suffix"]); keyErr == nil && exact {
			value, valueErr := resolveCurrentValue(operands["value"], partition)
			if valueErr == nil {
				result.Closure.Values = append(result.Closure.Values, heapMemberFact(identity, suffix, operation.Target.Name, value))
				if memberIdentity, found := tableIdentityForTerm(operands["value"], partition); found {
					result.Closure.Values = append(result.Closure.Values, heapMemberIdentityFact(identity, suffix, operation.Target.Name, memberIdentity))
				}
			}
		}
	}
	return result, nil
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
	if !strings.HasPrefix(string(operands["result"]), "temp/") ||
		(!strings.EqualFold(string(operands["default"]), "select/default/true") && !strings.EqualFold(string(operands["default"]), "select/default/false")) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed channel select")
	}
	type selectCase struct {
		term, display string
		payload       typ.Type
		identity      []byte
	}
	cases := make(map[string]*selectCase)
	for _, operand := range operation.Operands {
		switch {
		case strings.HasPrefix(operand.Role, "case-") && !strings.HasPrefix(operand.Role, "case-display-"):
			name := strings.TrimPrefix(operand.Role, "case-")
			if cases[name] != nil || !strings.HasPrefix(string(operand.Value), "path/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed select case")
			}
			cases[name] = &selectCase{term: string(operand.Value)}
		case strings.HasPrefix(operand.Role, "case-display-"):
			name := strings.TrimPrefix(operand.Role, "case-display-")
			item := cases[name]
			if item == nil || item.display != "" || len(operand.Value) == 0 {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed select case display")
			}
			item.display = string(operand.Value)
		case strings.HasPrefix(operand.Role, "payload-type-"):
			name := strings.TrimPrefix(operand.Role, "payload-type-")
			item := cases[name]
			if item == nil || item.payload != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed select payload")
			}
			payload, ok := shapefact.DecodeTarget(operand.Value)
			if !ok || payload == nil || payload == typ.Any || payload == typ.Unknown {
				return equation.TransactionResult{Complete: true}, nil
			}
			item.payload = payload
		case operand.Role == "result" || operand.Role == "default":
		default:
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed channel select role %q", operand.Role)
		}
	}
	ordered := make([]string, 0, len(cases))
	for name := range cases {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	selectID := fmt.Sprintf("%x/%s", operation.Target.Body, operation.Target.Name)
	resultCases := make([]channelselect.ResultCase, 0, len(ordered))
	armFacts := make([]equation.Fact, 0, len(ordered))
	for index, name := range ordered {
		item := cases[name]
		if item == nil || item.display == "" || item.payload == nil {
			return equation.TransactionResult{Complete: true}, nil
		}
		identity, ok := resolveCurrentIdentity([]byte(item.term), partition)
		if !ok || !isChannelIdentity(identity) {
			return equation.TransactionResult{Complete: true}, nil
		}
		item.identity = identity
		resultCases = append(resultCases, channelselect.ResultCase{Index: index, Payload: item.payload})
		wire, marshalErr := json.Marshal(selectArmWire{Index: index, Term: item.term, Display: item.display, Identity: base64.RawURLEncoding.EncodeToString(identity), Payload: base64.RawURLEncoding.EncodeToString(mustCanonicalType(item.payload))})
		if marshalErr != nil {
			return equation.TransactionResult{}, marshalErr
		}
		armFacts = append(armFacts, equation.Fact{Key: "select/arm/" + selectID + "/" + fmt.Sprintf("%08d", index), Value: wire})
	}
	resultType, ok := channelselect.ResultValueTypeWithDefault(selectID, resultCases, string(operands["default"]) == "select/default/true")
	if !ok {
		return equation.TransactionResult{Complete: true}, nil
	}
	encodedResult, ok := shapefact.EncodeTarget(resultType)
	if !ok {
		return equation.TransactionResult{Complete: true}, nil
	}
	meta, marshalErr := json.Marshal(selectMetaWire{Cases: len(ordered), HasDefault: string(operands["default"]) == "select/default/true"})
	if marshalErr != nil {
		return equation.TransactionResult{}, marshalErr
	}
	values := []equation.Fact{
		{Key: "value/" + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte("scalar/top")},
		{Key: "epoch/" + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
		{Key: "type/" + string(operands["result"]) + "/" + operation.Target.Name, Value: encodedResult},
		{Key: "select/origin/" + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte(selectID)},
		{Key: "select/meta/" + selectID, Value: meta},
	}
	values = append(values, armFacts...)
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

type selectMetaWire struct {
	Cases      int  `json:"cases"`
	HasDefault bool `json:"has_default"`
}

type selectArmWire struct {
	Index    int    `json:"index"`
	Term     string `json:"term"`
	Display  string `json:"display"`
	Identity string `json:"identity"`
	Payload  string `json:"payload"`
}

func mustCanonicalType(value typ.Type) []byte {
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		return nil
	}
	return encoded
}

func branchKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if closure, recognized, err := selectBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := typedLiteralBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
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

// typedLiteralBranchClosure consumes a value type already carried by an
// ordinary assignment. It is deliberately limited to a strict discriminant
// narrowing: unsupported values continue through the scalar branch rule.
func typedLiteralBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	var encoded []byte
	for _, operand := range operation.Operands {
		if operand.Role == "predicate" {
			if encoded != nil {
				return equation.OutputClosure{}, false, fmt.Errorf("engine: duplicate branch predicate")
			}
			encoded = operand.Value
		}
	}
	if !strings.HasPrefix(string(encoded), branchPredicatePrefix) {
		return equation.OutputClosure{}, false, nil
	}
	var predicate branchPredicateWire
	if err := json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate); err != nil {
		return equation.OutputClosure{}, false, fmt.Errorf("engine: decode typed literal branch predicate: %w", err)
	}
	if (predicate.Kind != "literal-equal" && predicate.Kind != "literal-not") || predicate.Path == "" || predicate.Literal == "" || predicate.Negated {
		return equation.OutputClosure{}, false, nil
	}
	literal, ok := literalType(predicate.Literal)
	if !ok {
		return equation.OutputClosure{}, false, nil
	}
	root, suffix, source, ok := typedAncestor([]byte("path/"+predicate.Path), partition)
	if !ok || len(suffix) == 0 {
		return equation.OutputClosure{}, false, nil
	}
	trueType, trueOK := variant.NarrowByPathLiteral(source, suffix, literal)
	falseType, falseOK := variant.NarrowByPathLiteralNot(source, suffix, literal)
	if predicate.Kind == "literal-not" {
		trueType, falseType = falseType, trueType
		trueOK, falseOK = falseOK, trueOK
	}
	if !trueOK || !falseOK {
		return equation.OutputClosure{}, false, nil
	}
	closure := equation.OutputClosure{}
	for _, edge := range []struct {
		name  string
		type_ typ.Type
	}{{"true", trueType}, {"false", falseType}} {
		value, encoded := shapefact.EncodeTarget(edge.type_)
		if !encoded {
			return equation.OutputClosure{}, false, nil
		}
		guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/" + edge.name)}
		closure.Outcomes = append(closure.Outcomes,
			equation.Fact{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge.name), Guards: []equation.Guard{guard}},
			equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("typed/" + edge.name), Guards: []equation.Guard{guard}},
		)
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(root) + "/" + operation.Target.Name, Value: value, Guards: []equation.Guard{guard}})
	}
	return closure, true, nil
}

// selectBranchClosure recognizes only a complete, epoch-current select result
// compared to an exact channel identity.  It emits finite edge facts rather
// than guessing a winner: unknown or partial select catalogs remain silent.
func selectBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	var encoded []byte
	for _, operand := range operation.Operands {
		if operand.Role == "predicate" {
			if encoded != nil {
				return equation.OutputClosure{}, false, fmt.Errorf("engine: duplicate branch predicate")
			}
			encoded = operand.Value
		}
	}
	if !strings.HasPrefix(string(encoded), branchPredicatePrefix) {
		return equation.OutputClosure{}, false, nil
	}
	var predicate branchPredicateWire
	if err := json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate); err != nil {
		return equation.OutputClosure{}, false, fmt.Errorf("engine: decode select branch predicate: %w", err)
	}
	if predicate.Kind != "path-equal" || predicate.Path == "" || predicate.OtherPath == "" {
		return equation.OutputClosure{}, false, nil
	}
	resultPath, channelPath := "", ""
	if strings.HasSuffix(predicate.Path, ".channel") {
		resultPath, channelPath = strings.TrimSuffix(predicate.Path, ".channel"), predicate.OtherPath
	} else if strings.HasSuffix(predicate.OtherPath, ".channel") {
		resultPath, channelPath = strings.TrimSuffix(predicate.OtherPath, ".channel"), predicate.Path
	} else {
		return equation.OutputClosure{}, false, nil
	}
	result := []byte("path/" + resultPath)
	selectID, ok := currentEpochFact("select/origin/", result, partition)
	if !ok || len(selectID) == 0 {
		return equation.OutputClosure{}, false, nil
	}
	metaFact, ok := exactFact("select/meta/"+string(selectID), partition)
	if !ok {
		return equation.OutputClosure{}, false, nil
	}
	var meta selectMetaWire
	if json.Unmarshal(metaFact, &meta) != nil || meta.Cases <= 0 {
		return equation.OutputClosure{}, false, nil
	}
	other, ok := resolveCurrentIdentity([]byte("path/"+channelPath), partition)
	if !ok {
		return equation.OutputClosure{}, false, nil
	}
	all, matching := make([]int, 0, meta.Cases), make([]int, 0, meta.Cases)
	for index := 0; index < meta.Cases; index++ {
		fact, found := exactFact("select/arm/"+string(selectID)+"/"+fmt.Sprintf("%08d", index), partition)
		if !found {
			return equation.OutputClosure{}, false, nil
		}
		var arm selectArmWire
		if json.Unmarshal(fact, &arm) != nil || arm.Index != index || arm.Identity == "" {
			return equation.OutputClosure{}, false, nil
		}
		identity, decodeErr := base64.RawURLEncoding.DecodeString(arm.Identity)
		if decodeErr != nil || !isChannelIdentity(identity) {
			return equation.OutputClosure{}, false, nil
		}
		all = append(all, index)
		if string(identity) == string(other) {
			matching = append(matching, index)
		}
	}
	if len(matching) == 0 {
		return equation.OutputClosure{}, false, nil
	}
	remaining := make([]int, 0, len(all)-len(matching))
	matched := make(map[int]bool, len(matching))
	for _, index := range matching {
		matched[index] = true
	}
	for _, index := range all {
		if !matched[index] {
			remaining = append(remaining, index)
		}
	}
	closure := equation.OutputClosure{}
	emit := func(edge string, selectors []int, possible bool) error {
		if !possible {
			return nil
		}
		wire, err := json.Marshal(selectConstraintWire{Select: string(selectID), Arms: selectors, Default: edge == "false" && meta.HasDefault})
		if err != nil {
			return err
		}
		guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/" + edge)}
		closure.Outcomes = append(closure.Outcomes,
			equation.Fact{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge), Guards: []equation.Guard{guard}},
			equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("select/" + edge), Guards: []equation.Guard{guard}},
		)
		closure.Values = append(closure.Values, equation.Fact{Key: "select/constraint/" + operation.Target.Name + "/" + edge, Value: wire, Guards: []equation.Guard{guard}})
		return nil
	}
	if err := emit("true", matching, true); err != nil {
		return equation.OutputClosure{}, false, err
	}
	if err := emit("false", remaining, len(remaining) != 0 || meta.HasDefault); err != nil {
		return equation.OutputClosure{}, false, err
	}
	return closure, true, nil
}

type selectConstraintWire struct {
	Select  string `json:"select"`
	Arms    []int  `json:"arms"`
	Default bool   `json:"default"`
}

func exactFact(key string, partition equation.Partition) ([]byte, bool) {
	for _, fact := range partition.Values() {
		if fact.Key == key {
			return append([]byte(nil), fact.Value...), true
		}
	}
	return nil, false
}

// applyKernel validates the sealed direct or method-call shape and publishes
// proven call-contract failures at this equation point. Unknown values are not
// violations: diagnostics are proof outputs, never speculative findings.
func applyKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition) (result equation.TransactionResult, err error) {
	var placementFacts []equation.Fact
	defer func() {
		if err == nil && result.Complete && len(placementFacts) != 0 {
			result.Closure.Values = append(result.Closure.Values, placementFacts...)
		}
	}()
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
	placementFacts = placementApplyFacts(operation, operands, partition)
	if closure, recognized := tableInsertEpoch(operation, operands, partition); recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized := channelLifecycleEpoch(operation, operands, partition); recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized := resourceLifecycleEpoch(operation, operands, partition); recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	handle, localCallable := closureHandle{}, false
	if lexical != nil {
		if hasCallee {
			handle, localCallable = closureHandleFor(operands.callee, partition)
			if !localCallable {
				// The normalized front represents a static member call as a direct
				// callee path. Its sealed member capability has the same local-body
				// authority as explicit receiver/method syntax.
				if cut := strings.LastIndex(string(operands.callee), "."); cut > len("path/") {
					handle, localCallable = methodClosureHandleFor(operands.callee[:cut], string(operands.callee[cut+1:]), partition)
				}
			}
		} else {
			handle, localCallable = methodClosureHandleFor(operands.receiver, operands.method, partition)
		}
	}
	applyLocal := func() (equation.TransactionResult, bool, error) {
		if lexical == nil || !localCallable || (operands.resultArity == 0 && !lexical.requiresBody[handle.Prototype]) ||
			(operands.resultArity != 0 && !lexical.requiresBody[handle.Prototype] && (lexical.hasClaim(handle.Prototype) || lexical.hasTableAllocation(handle.Prototype))) {
			return equation.TransactionResult{}, false, nil
		}
		outcome, err := lexical.applyKnown(operation, operands, handle, partition)
		if err != nil {
			if operands.resultArity == 0 && (strings.Contains(err.Error(), "incomplete lexical argument") || strings.Contains(err.Error(), "incomplete lexical capture")) {
				// Child entry seeding is atomic. A partially known local call must
				// not leave its pre-call caller facts looking like a completed run.
				return equation.TransactionResult{}, false, err
			}
			if operands.resultArity == 0 && operands.spread && strings.Contains(err.Error(), "unsupported exact lexical boundary") && !lexical.hasVarargBoundary(handle.Prototype) {
				return equation.TransactionResult{}, false, err
			}
			// An incomplete local boundary is indistinguishable from an unresolved
			// callable at this result owner. Keep its slots at Top rather than
			// publishing a partial child result or a synthetic diagnostic.
			return equation.TransactionResult{}, false, nil
		}
		return outcome, true, nil
	}
	if state, handled := sendIsolationState(operation, operands, partition); handled {
		return state, nil
	}
	if closure, escaped := channelLifecycleEscape(operation, operands, partition); escaped {
		// Passing an exact channel identity through an otherwise opaque call
		// transfers lifecycle authority out of this local epoch. The state is
		// now may-closed, so subsequent channel operations deliberately remain
		// silent unless a fresh identity is established.
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, escaped := resourceLifecycleEscape(operation, operands, partition); escaped {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
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
		if outcome, projected, err := applyLocal(); err != nil {
			return equation.TransactionResult{}, err
		} else if projected {
			return outcome, nil
		}
		return equation.TransactionResult{Complete: true}, nil
	}
	if !isUnknownScalar(callee) && !isCallableValue(callee) {
		return callDiagnostic(operation, "not_callable", "callee", fmt.Sprintf("%s is %s, not callable", operands.display, callDisplayValue(callee))), nil
	}
	signature, known := callableSignature(callee)
	if !known || operands.spread {
		if string(callee) == "scalar/function" {
			// An unshaped callable includes generic and otherwise uninstantiated
			// lexical functions. Its body result is not a certified call result.
			return equation.TransactionResult{Complete: true}, nil
		}
		if outcome, projected, err := applyLocal(); err != nil {
			return equation.TransactionResult{}, err
		} else if projected {
			return outcome, nil
		}
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
		if known && genericConstraintRefuted(argument, signature.Params[index]) {
			return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValue(argument), strings.TrimSpace(strings.SplitN(signature.Params[index], ":", 2)[1]))), nil
		}
		if !known || !provenScalarNotSubtype(argument, signature.Params[index]) {
			continue
		}
		return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValue(argument), signature.Params[index])), nil
	}
	if genericCallableSignature(signature) {
		var instantiated bool
		signature, instantiated = instantiateCallableSignature(signature, operands.arguments, partition)
		if !instantiated {
			// The existing contract checks above remain authoritative. Once they
			// have accepted the call, a missing concrete substitution still cannot
			// admit a local result projection.
			return equation.TransactionResult{Complete: true}, nil
		}
	}
	if outcome, projected, err := applyLocal(); err != nil {
		return equation.TransactionResult{}, err
	} else if projected {
		return outcome, nil
	}
	return equation.TransactionResult{Complete: true}, nil
}

func genericCallableSignature(signature callableShape) bool {
	for _, parameter := range signature.Params {
		if len(parameter) > 0 && parameter[0] >= 'A' && parameter[0] <= 'Z' && (len(parameter) == 1 || strings.HasPrefix(parameter[1:], " :")) {
			return true
		}
	}
	return false
}

func genericConstraintRefuted(value []byte, parameter string) bool {
	parts := strings.SplitN(parameter, ":", 2)
	name := ""
	if len(parts) == 2 {
		name = strings.TrimSpace(parts[0])
	}
	if len(parts) != 2 || len(name) != 1 || name[0] < 'A' || name[0] > 'Z' || isUnknownScalar(value) {
		return false
	}
	constraint := strings.TrimSpace(parts[1])
	return strings.HasPrefix(constraint, "{") && !shapefact.IsTable(value)
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
	if table, err := resolveCurrentValue(subject, partition); err == nil {
		closure.Values = append(closure.Values, equation.Fact{Key: "call-result/" + operation.Target.Name + "/00000000", Value: table})
		if identity, found := tableIdentityForTerm(subject, partition); found {
			closure.Values = append(closure.Values, equation.Fact{Key: "call-heap-identity/" + operation.Target.Name + "/00000000", Value: identity})
		}
	}
	return closure, true, nil
}

// tableInsertEpoch handles only the exact table.insert(array, value) form.
// Its append index is derived from already published members of the sealed
// identity; an unknown receiver or an unsupported overload remains untouched.
func tableInsertEpoch(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) (equation.OutputClosure, bool) {
	if operands.display != "table.insert" || operands.spread || len(operands.arguments) != 2 {
		return equation.OutputClosure{}, false
	}
	if freeze, guarded := frozenProof(operation, operands.arguments[0], partition); freeze != "" || guarded {
		proof := freeze
		if guarded {
			proof = "guard"
		}
		return equation.OutputClosure{Diagnostics: []equation.Fact{{
			Key:   "effect.freeze.mutation/" + operation.Target.Name + "/" + proof,
			Value: []byte(fmt.Sprintf("cannot call mutator on frozen table %q", callArgumentDisplay(operation.Operands, 0))),
		}}}, true
	}
	identity, found := tableIdentityForTerm(operands.arguments[0], partition)
	if !found {
		return equation.OutputClosure{}, true
	}
	value, err := resolveCurrentValue(operands.arguments[1], partition)
	if err != nil {
		return equation.OutputClosure{}, true
	}
	next := 1
	prefix := heapMemberPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(fact.Key, prefix), "/")
		if len(parts) != 2 {
			continue
		}
		suffixBytes, decodeErr := base64.RawURLEncoding.DecodeString(parts[0])
		if decodeErr != nil {
			continue
		}
		segments, valid := segment.ParseFormattedSegments(string(suffixBytes))
		if !valid || len(segments) != 1 || segments[0].Kind != segment.SegmentIndexInt || segments[0].Index < next {
			continue
		}
		next = segments[0].Index + 1
	}
	suffix := segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentIndexInt, Index: next}})
	return equation.OutputClosure{Values: []equation.Fact{heapMemberFact(identity, suffix, operation.Target.Name, value)}}, true
}

const channelLifecyclePrefix = "effect.lifecycle.channel/"

// channelLifecycleEpoch reuses the established fact epoch discipline: a
// recognized constructor creates a fresh opaque identity, and later exact
// method calls read and strongly replace that identity's state. No fact is
// inferred for an unknown receiver or for an identity that has escaped.
func channelLifecycleEpoch(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) (equation.OutputClosure, bool) {
	if operands.display == "channel.new" && !operands.spread && len(operands.arguments) == 0 {
		identity := []byte("scalar/channel/" + operation.Target.Name)
		return equation.OutputClosure{Values: []equation.Fact{
			{Key: "call-result/" + operation.Target.Name + "/00000000", Value: identity},
			channelLifecycleStateFact(identity, operation.Target.Name, "open"),
		}}, true
	}
	if operands.receiver == nil || (operands.method != "close" && operands.method != "send" && operands.method != "receive") {
		return equation.OutputClosure{}, false
	}
	identity, known := resolveKnownCurrentValue(operands.receiver, partition)
	if !known || !isChannelIdentity(identity) {
		return equation.OutputClosure{}, false
	}
	state, proven := channelLifecycleState(partition, identity)
	if !proven || state == "escaped" {
		return equation.OutputClosure{}, true
	}
	if operands.method == "receive" {
		return equation.OutputClosure{}, true
	}
	closure := equation.OutputClosure{}
	if state == "closed" {
		code, message := "channel.send.closed", "cannot send on closed channel"
		if operands.method == "close" {
			code, message = "channel.close.closed", "cannot close already closed channel"
		}
		closure.Diagnostics = append(closure.Diagnostics, equation.Fact{
			Key:   code + "/" + operation.Target.Name,
			Value: []byte(message + " `" + channelDisplay(partition, operands.receiver) + "`"),
		})
	}
	if operands.method == "close" {
		closure.Values = append(closure.Values, channelLifecycleStateFact(identity, operation.Target.Name, "closed"))
	}
	return closure, true
}

const resourceLifecyclePrefix = "effect.lifecycle.resource/"

// resourceLifecycleEpoch is the same closed-identity epoch used for channels,
// specialized to the declared connection acquire/release pair. The acquire
// result is opaque and therefore cannot be fabricated by an unknown call.
func resourceLifecycleEpoch(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) (equation.OutputClosure, bool) {
	if operands.display == "resource.connect" && !operands.spread && len(operands.arguments) == 0 {
		identity := []byte("scalar/resource/" + operation.Target.Name)
		return equation.OutputClosure{Values: []equation.Fact{
			{Key: "call-result/" + operation.Target.Name + "/00000000", Value: identity},
			resourceLifecycleStateFact(identity, operation.Target.Name, "open"),
		}}, true
	}
	if operands.spread || len(operands.arguments) != 1 {
		return equation.OutputClosure{}, false
	}
	identity, known := resolveKnownCurrentValue(operands.arguments[0], partition)
	if !known || !isResourceIdentity(identity) {
		if operands.display == "resource.query" {
			display := callArgumentDisplay(operation.Operands, 0)
			if display != "" {
				return equation.OutputClosure{Diagnostics: []equation.Fact{{
					Key:   "typestate.unproven_requirement/" + operation.Target.Name,
					Value: []byte(fmt.Sprintf("cannot prove typestate requirement for resource `%s`: expected `open`", display)),
				}}}, true
			}
		}
		return equation.OutputClosure{}, false
	}
	state, proven := resourceLifecycleState(partition, identity)
	if !proven || state == "escaped" {
		return equation.OutputClosure{}, true
	}
	display := callArgumentDisplay(operation.Operands, 0)
	if display == "" {
		display = "resource"
	}
	switch operands.display {
	case "resource.close":
		return equation.OutputClosure{Values: []equation.Fact{resourceLifecycleStateFact(identity, operation.Target.Name, "closed")}}, true
	case "resource.query":
		if state == "open" {
			return equation.OutputClosure{}, true
		}
		return equation.OutputClosure{Diagnostics: []equation.Fact{{
			Key:   "typestate.invalid_requirement/" + operation.Target.Name,
			Value: []byte(fmt.Sprintf("invalid typestate requirement for resource `%s` in protocol connection: expected `open`, found `%s`", display, state)),
		}}}, true
	case "resource.begin":
		if state != "open" {
			return equation.OutputClosure{Diagnostics: []equation.Fact{{
				Key:   "typestate.invalid_requirement/" + operation.Target.Name,
				Value: []byte(fmt.Sprintf("invalid typestate requirement for resource `%s` in protocol connection: expected `open`, found `%s`", display, state)),
			}}}, true
		}
		transaction := []byte("scalar/resource/" + operation.Target.Name)
		return equation.OutputClosure{Values: []equation.Fact{
			{Key: "call-result/" + operation.Target.Name + "/00000000", Value: transaction},
			resourceLifecycleStateFact(transaction, operation.Target.Name, "active"),
		}}, true
	case "resource.commit":
		if state == "active" {
			return equation.OutputClosure{Values: []equation.Fact{resourceLifecycleStateFact(identity, operation.Target.Name, "committed")}}, true
		}
		return equation.OutputClosure{Diagnostics: []equation.Fact{{
			Key:   "typestate.invalid_transition/" + operation.Target.Name,
			Value: []byte(fmt.Sprintf("invalid transition for resource `%s` in protocol transaction: expected `active`, found `%s`", display, state)),
		}}}, true
	default:
		return equation.OutputClosure{}, false
	}
}

func resourceLifecycleStateFact(identity []byte, operation, state string) equation.Fact {
	return equation.Fact{Key: resourceLifecyclePrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation, Value: []byte(state)}
}

func resourceLifecycleEscape(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) (equation.OutputClosure, bool) {
	identities := make(map[string][]byte)
	for _, term := range operands.arguments {
		if value, known := resolveKnownCurrentValue(term, partition); known && isResourceIdentity(value) {
			identities[string(value)] = value
		}
	}
	if len(identities) == 0 {
		return equation.OutputClosure{}, false
	}
	closure := equation.OutputClosure{}
	for _, identity := range identities {
		closure.Values = append(closure.Values, resourceLifecycleStateFact(identity, operation.Target.Name, "escaped"))
	}
	return closure, true
}

func isResourceIdentity(value []byte) bool {
	return strings.HasPrefix(string(value), "scalar/resource/op-")
}

func resourceLifecycleState(partition equation.Partition, identity []byte) (string, bool) {
	prefix := resourceLifecyclePrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	state, latest := "", ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (state == "" || fact.Key > latest) {
			state, latest = string(fact.Value), fact.Key
		}
	}
	return state, state == "open" || state == "closed" || state == "active" || state == "committed" || state == "escaped"
}

func channelLifecycleEscape(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) (equation.OutputClosure, bool) {
	identities := make(map[string][]byte)
	for _, term := range operands.arguments {
		if value, known := resolveKnownCurrentValue(term, partition); known && isChannelIdentity(value) {
			identities[string(value)] = value
		}
	}
	if operands.receiver != nil {
		if value, known := resolveKnownCurrentValue(operands.receiver, partition); known && isChannelIdentity(value) {
			identities[string(value)] = value
		}
	}
	if len(identities) == 0 {
		return equation.OutputClosure{}, false
	}
	closure := equation.OutputClosure{}
	for _, identity := range identities {
		closure.Values = append(closure.Values, channelLifecycleStateFact(identity, operation.Target.Name, "escaped"))
	}
	return closure, true
}

func channelLifecycleStateFact(identity []byte, operation, state string) equation.Fact {
	return equation.Fact{
		Key:   channelLifecyclePrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation,
		Value: []byte(state),
	}
}

func channelLifecycleState(partition equation.Partition, identity []byte) (string, bool) {
	prefix := channelLifecyclePrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	state, latest := "", ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (state == "" || fact.Key > latest) {
			state, latest = string(fact.Value), fact.Key
		}
	}
	return state, state == "open" || state == "closed" || state == "escaped"
}

func isChannelIdentity(value []byte) bool {
	return strings.HasPrefix(string(value), "scalar/channel/op-") || strings.HasPrefix(string(value), "scalar/channel-entry/")
}

// resolveCurrentIdentity uses the same operation-key epoch discipline as
// normal value reads.  A symbolic entry identity is intentionally accepted
// only when it was published by entryKernel for an exact Channel<T> root.
func resolveCurrentIdentity(term []byte, partition equation.Partition) ([]byte, bool) {
	if identity, ok := currentEpochFact("identity/", term, partition); ok {
		return identity, true
	}
	value, known := resolveKnownCurrentValue(term, partition)
	return value, known && isChannelIdentity(value)
}

func currentEpochFact(prefix string, term []byte, partition equation.Partition) ([]byte, bool) {
	epochPrefix := "epoch/" + string(term) + "/"
	latestEpoch := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, epochPrefix) && fact.Key > latestEpoch {
			latestEpoch = fact.Key
		}
	}
	if latestEpoch == "" {
		return nil, false
	}
	operation := strings.TrimPrefix(latestEpoch, epochPrefix)
	key := prefix + string(term) + "/" + operation
	for _, fact := range partition.Values() {
		if fact.Key == key {
			return append([]byte(nil), fact.Value...), true
		}
	}
	return nil, false
}

func sealedTableIdentity(operation equation.BoundEquation) []byte {
	// Target coordinates are frozen by the artifact compiler.  Their body
	// identity prevents equal operation ordinals in distinct lexical bodies
	// from aliasing each other.
	return []byte(fmt.Sprintf("sealed-table/%x/%s", operation.Target.Body, operation.Target.Name))
}

func heapIdentityFact(term, operation string, identity []byte) equation.Fact {
	return equation.Fact{Key: heapTableIdentityPrefix + string(term) + "/" + operation, Value: append([]byte(nil), identity...)}
}

func heapFactKey(prefix string, identity []byte, suffix, operation string) string {
	return prefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix)) + "/" + operation
}

func heapMemberFact(identity []byte, suffix, operation string, value []byte) equation.Fact {
	return equation.Fact{Key: heapFactKey(heapMemberPrefix, identity, suffix, operation), Value: append([]byte(nil), value...)}
}

func heapMemberIdentityFact(identity []byte, suffix, operation string, memberIdentity []byte) equation.Fact {
	return equation.Fact{Key: heapFactKey(heapMemberIdentityPrefix, identity, suffix, operation), Value: append([]byte(nil), memberIdentity...)}
}

func heapMemberCurrent(prefix string, identity []byte, suffix string, partition equation.Partition) ([]byte, bool) {
	want := prefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix)) + "/"
	var value []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, want) && (value == nil || fact.Key > latest) {
			value, latest = fact.Value, fact.Key
		}
	}
	return append([]byte(nil), value...), value != nil
}

// tableAddress splits a source path into the root table lens and its static
// suffix.  A dynamic read is represented by its own result term, so only the
// WIR's canonical static path spelling is admitted here.
func tableAddress(term []byte) ([]byte, string, bool) {
	path := strings.TrimPrefix(string(term), "path/")
	if path == string(term) || path == "" {
		return nil, "", false
	}
	cut := strings.IndexAny(path, ".[")
	if cut < 0 {
		return []byte("path/" + path), "", true
	}
	root, suffix := path[:cut], path[cut:]
	if root == "" || !segment.ValidFormattedSegments(suffix) {
		return nil, "", false
	}
	return []byte("path/" + root), suffix, true
}

func tableIdentityForTerm(term []byte, partition equation.Partition) ([]byte, bool) {
	if identity, ok := currentEpochFact(heapTableIdentityPrefix, term, partition); ok {
		return identity, true
	}
	root, suffix, ok := tableAddress(term)
	if !ok || suffix == "" {
		return nil, false
	}
	identity, ok := currentEpochFact(heapTableIdentityPrefix, root, partition)
	if !ok {
		return nil, false
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid || len(segments) == 0 {
		return nil, false
	}
	for len(segments) != 0 {
		matched := false
		for count := len(segments); count > 0; count-- {
			prefix := segment.FormatSegments(segments[:count])
			next, found := heapMemberCurrent(heapMemberIdentityPrefix, identity, prefix, partition)
			if !found {
				continue
			}
			identity, segments, matched = next, segments[count:], true
			break
		}
		if !matched {
			return nil, false
		}
	}
	return identity, true
}

func heapMemberValue(term []byte, partition equation.Partition) ([]byte, bool) {
	root, suffix, ok := tableAddress(term)
	if !ok || suffix == "" {
		return nil, false
	}
	identity, ok := tableIdentityForTerm(root, partition)
	if !ok {
		return nil, false
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid || len(segments) == 0 {
		return nil, false
	}
	for len(segments) != 0 {
		whole := segment.FormatSegments(segments)
		if value, found := heapMemberCurrent(heapMemberPrefix, identity, whole, partition); found {
			return value, true
		}
		matched := false
		for count := len(segments) - 1; count > 0; count-- {
			prefix := segment.FormatSegments(segments[:count])
			next, found := heapMemberCurrent(heapMemberIdentityPrefix, identity, prefix, partition)
			if !found {
				continue
			}
			identity, segments, matched = next, segments[count:], true
			break
		}
		if !matched {
			return nil, false
		}
	}
	return nil, false
}

func tableMemberSuffix(key, suffix []byte) (string, bool) {
	if !strings.HasPrefix(string(suffix), "suffix/") {
		return "", false
	}
	tail := strings.TrimPrefix(string(suffix), "suffix/")
	if !segment.ValidFormattedSegments(tail) {
		return "", false
	}
	if text, err := scalarString(key); err == nil {
		if tableFieldName(text) {
			return segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentField, Name: text}}) + tail, true
		}
		return segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentIndexString, Name: text}}) + tail, true
	}
	if number, err := scalarNumber(key); err == nil && number == math.Trunc(number) {
		return segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentIndexInt, Index: int(number)}}) + tail, true
	}
	return "", false
}

func tableFieldName(value string) bool {
	if value == "" {
		return false
	}
	for index, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || (index > 0 && ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func channelDisplay(partition equation.Partition, receiver []byte) string {
	prefix := "effect.lifecycle.channel.display/" + base64.RawURLEncoding.EncodeToString(receiver) + "/"
	display, latest := "", ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (display == "" || fact.Key > latest) {
			display, latest = string(fact.Value), fact.Key
		}
	}
	if display != "" {
		return display
	}
	return strings.TrimPrefix(string(receiver), "path/")
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
	callee      []byte
	receiver    []byte
	method      string
	display     string
	arguments   [][]byte
	spread      bool
	resultArity int
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
		case operand.Role == "result-arity":
			arity, err := strconv.Atoi(string(operand.Value))
			if err != nil || arity < 0 {
				return directCallOperands{}, fmt.Errorf("engine: malformed call result arity")
			}
			result.resultArity = arity
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
	Params     []string            `json:"params"`
	Returns    []string            `json:"returns"`
	TypeParams []callableTypeParam `json:"type_params"`
	Required   int                 `json:"required"`
	Variadic   bool                `json:"variadic"`
}

type callableTypeParam struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint,omitempty"`
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
	seen := make(map[string]bool, len(signature.TypeParams))
	for _, parameter := range signature.TypeParams {
		if parameter.Name == "" || seen[parameter.Name] {
			return callableShape{}, false
		}
		seen[parameter.Name] = true
	}
	for _, result := range signature.Returns {
		if result == "" {
			return callableShape{}, false
		}
	}
	return signature, true
}

// instantiateCallableSignature applies only substitutions proven from the
// concrete call operands. It is intentionally summary-local: child execution
// remains the sole producer of result values, and an incomplete substitution
// leaves the existing result projection fail-closed.
func instantiateCallableSignature(signature callableShape, arguments [][]byte, partition equation.Partition) (callableShape, bool) {
	if !genericCallableSignature(signature) {
		return signature, true
	}
	bindings := make(map[string]string, len(signature.TypeParams))
	for index, parameter := range signature.Params {
		name, generic := callableTypeParameterName(parameter, signature.TypeParams)
		if !generic || index >= len(arguments) {
			continue
		}
		value, known := resolveKnownCurrentValue(arguments[index], partition)
		if !known || isUnknownScalar(value) {
			return callableShape{}, false
		}
		argumentType, ok := callableArgumentType(value)
		if !ok {
			return callableShape{}, false
		}
		if prior, exists := bindings[name]; exists && prior != argumentType {
			return callableShape{}, false
		}
		bindings[name] = argumentType
	}
	for _, parameter := range signature.TypeParams {
		if _, bound := bindings[parameter.Name]; !bound {
			return callableShape{}, false
		}
	}
	signature.Params = substituteCallableTypes(signature.Params, bindings)
	signature.Returns = substituteCallableTypes(signature.Returns, bindings)
	signature.TypeParams = nil
	return signature, true
}

func callableTypeParameterName(parameter string, parameters []callableTypeParam) (string, bool) {
	name := strings.TrimSpace(strings.SplitN(parameter, ":", 2)[0])
	for _, candidate := range parameters {
		if name == candidate.Name {
			return name, true
		}
	}
	// Older front artifacts did not carry an explicit binder list. Their
	// parameter spelling remains authoritative for the one-letter generic
	// surface, so keep those summaries compatible while new artifacts carry
	// TypeParams explicitly.
	if len(parameters) == 0 && len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z' {
		return name, true
	}
	return "", false
}

func callableArgumentType(value []byte) (string, bool) {
	switch {
	case strings.HasPrefix(string(value), "scalar/number/"):
		return "number", true
	case strings.HasPrefix(string(value), "scalar/string/"):
		return "string", true
	case strings.HasPrefix(string(value), "scalar/bool/"):
		return "boolean", true
	case string(value) == "scalar/nil":
		return "nil", true
	case shapefact.IsTable(value):
		return "table", true
	default:
		return "", false
	}
}

func substituteCallableTypes(types []string, bindings map[string]string) []string {
	if len(types) == 0 || len(bindings) == 0 {
		return types
	}
	out := make([]string, len(types))
	for index, value := range types {
		out[index] = value
		for parameter, replacement := range bindings {
			if value == parameter {
				out[index] = replacement
				break
			}
		}
	}
	return out
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
		(!strings.HasPrefix(string(operands["provider"]), "provider/global/") && !strings.HasPrefix(string(operands["provider"]), "provider/module/") && !strings.HasPrefix(string(operands["provider"]), "provider/module-load/")) ||
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
	argumentTerms := map[int][]byte{}
	hasApplication := false
	var application []byte
	var provider []byte
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "application":
			if hasApplication || !strings.HasPrefix(string(operand.Value), "call/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result application")
			}
			hasApplication = true
			application = operand.Value
		case operand.Role == "provider":
			if provider != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: duplicate call result provider")
			}
			provider = operand.Value
		case strings.HasPrefix(operand.Role, "argument-"):
			index, err := callArgumentIndex(operand.Role)
			if err != nil || argumentTerms[index] != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result argument %q", operand.Role)
			}
			argumentTerms[index] = operand.Value
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
		if index, err := strconv.Atoi(key); err == nil && provider != nil {
			if contract, ok := providerResultValue(provider, index, argumentTerms, partition); ok {
				value = contract
			}
			if imported, ok := importedProviderResultValue(provider, index, partition); ok {
				value = imported
			}
		}
		values = append(values,
			equation.Fact{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: value},
			equation.Fact{Key: "epoch/" + string(result) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
		)
		heapKey := "call-heap-identity/" + strings.TrimPrefix(string(application), "call/") + "/" + key
		for _, fact := range partition.Values() {
			if fact.Key == heapKey {
				values = append(values, heapIdentityFact(string(result), operation.Target.Name, fact.Value))
				break
			}
		}
		closureKey := "call-closure/" + strings.TrimPrefix(string(application), "call/") + "/" + key
		for _, fact := range partition.Values() {
			if fact.Key == closureKey {
				values = append(values, equation.Fact{Key: "closure/" + string(result) + "/" + operation.Target.Name, Value: append([]byte(nil), fact.Value...)})
				break
			}
		}
		memberPrefix := "call-member-closure/" + strings.TrimPrefix(string(application), "call/") + "/" + key + "/"
		for _, fact := range partition.Values() {
			if !strings.HasPrefix(fact.Key, memberPrefix) {
				continue
			}
			var wire memberClosureWire
			if json.Unmarshal(fact.Value, &wire) != nil || wire.Suffix == "" || !validClosureHandle(wire.Handle) {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call member closure")
			}
			values = append(values, equation.Fact{Key: "member-closure/" + string(result) + "/" + operation.Target.Name + "/" + strings.TrimPrefix(fact.Key, memberPrefix), Value: append([]byte(nil), fact.Value...)})
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

// providerResultValue turns a finite, declared stdlib result slot into a
// canonical type fact.  A malformed provider, unknown dynamic tail, or any
// result type intentionally leaves the call-result owner at Top.
func providerResultValue(provider []byte, index int, arguments map[int][]byte, partition equation.Partition) ([]byte, bool) {
	encoded := strings.TrimPrefix(string(provider), "provider/global/")
	if encoded == string(provider) || encoded == "" {
		return nil, false
	}
	name, err := strconv.Unquote(encoded)
	if err != nil || name == "" {
		return nil, false
	}
	for _, condition := range signaturelookup.StdlibConditionalResultSlots(name) {
		if condition.ResultIndex != index {
			continue
		}
		argument, exists := arguments[condition.ArgumentIndex]
		if !exists {
			continue
		}
		value, known := resolveKnownCurrentValue(argument, partition)
		if !known || !strings.HasPrefix(string(value), "scalar/string/") {
			continue
		}
		literal, literalErr := strconv.Unquote(strings.TrimPrefix(string(value), "scalar/string/"))
		if literalErr == nil && literal == condition.ArgumentString {
			return providerReturnTypeValue(condition.ResultType)
		}
	}
	result, ok := signaturelookup.StdlibResultSlot(name, index)
	if !ok {
		return nil, false
	}
	return providerReturnTypeValue(result)
}

// importedProviderResultValue projects the export seeded at the consumer
// entry into require's only result slot. The provider identity comes from the
// front's exact local-require annotation, so neither source spelling nor an
// untrusted global lookup can select an import fact.
func importedProviderResultValue(provider []byte, index int, partition equation.Partition) ([]byte, bool) {
	if index != 0 {
		return nil, false
	}
	encoded := strings.TrimPrefix(string(provider), "provider/module-load/")
	if encoded == string(provider) || encoded == "" {
		return nil, false
	}
	modulePath, err := strconv.Unquote(encoded)
	if err != nil || modulePath == "" {
		return nil, false
	}
	prefix := "value/" + importEntryTerm(modulePath) + "/"
	var value []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latest) {
			value, latest = fact.Value, fact.Key
		}
	}
	if value == nil {
		return nil, false
	}
	if _, ok := shapefact.DecodeTarget(value); !ok {
		return nil, false
	}
	return append([]byte(nil), value...), true
}

func providerReturnTypeValue(result typ.Type) ([]byte, bool) {
	if result == nil {
		return nil, false
	}
	// The value lattice can carry an exact declared shape, but it cannot make
	// a multi-return's optional slot, or an unresolved generic parameter, into
	// a concrete runtime scalar.  Leave those slots at Top: their declared
	// contracts remain available to the ordinary signature checker without
	// manufacturing a value fact or collapsing Lua's return expansion rules.
	switch unwrap.Alias(result).Kind() {
	case kind.Any, kind.Unknown, kind.Never, kind.Optional, kind.Union, kind.TypeParam:
		return nil, false
	}
	return shapefact.EncodeTarget(result)
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
	projected := make([]equation.Fact, 0)
	outcomes = append(outcomes, equation.Fact{Key: prefix + "arity", Value: []byte(strconv.Itoa(len(values)))})
	for index, value := range values {
		outcomes = append(outcomes, equation.Fact{Key: prefix + strconv.Itoa(index), Value: value})
		for memberIndex, wire := range memberClosuresFor(operation.Operands[index].Value, partition) {
			encoded, err := json.Marshal(wire)
			if err != nil {
				return equation.TransactionResult{}, err
			}
			projected = append(projected, equation.Fact{Key: fmt.Sprintf("return-member-closure/%s/%08d/%08d", operation.Target.Name, index, memberIndex), Value: encoded})
		}
	}
	for _, operand := range operation.Operands {
		if allocation, found := placementAllocationForTerm(operand.Value, partition); found {
			projected = append(projected, placementEventFact(allocation.Identity, operation.Target.Name, placementEventOwned))
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: projected, Outcomes: outcomes}}, nil
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
	if target, ok := shapefact.DecodeTarget(value); ok {
		switch unwrap.Alias(target).Kind() {
		case kind.Nil:
			return "nil", nil
		case kind.Boolean:
			return "boolean", nil
		case kind.Integer, kind.Number:
			return "number", nil
		case kind.String:
			return "string", nil
		case kind.Record:
			return "table", nil
		default:
			return "", errUnknownScalar
		}
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
	case string(value) == "scalar/function", strings.HasPrefix(string(value), "scalar/function/"):
		return "function", nil
	default:
		return "", fmt.Errorf("engine: malformed scalar value %q", value)
	}
}

func scalarString(value []byte) (string, error) {
	if isUnknownScalar(value) {
		return "", errUnknownScalar
	}
	if target, ok := shapefact.DecodeTarget(value); ok {
		if literal, ok := unwrap.Alias(target).(*typ.Literal); ok && literal.Base == kind.String {
			text, ok := literal.Value.(string)
			if !ok {
				return "", errUnknownScalar
			}
			return text, nil
		}
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
	if target, ok := shapefact.DecodeTarget(value); ok {
		if literal, ok := unwrap.Alias(target).(*typ.Literal); ok && (literal.Base == kind.Integer || literal.Base == kind.Number) {
			return strconv.ParseFloat(literal.String(), 64)
		}
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
	if value, found := selectPayloadValue(term, partition); found {
		return value, nil
	}
	if value, found := heapMemberValue(term, partition); found {
		return value, nil
	}
	if value, found := typedPathValue(term, partition); found {
		return value, nil
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
		if joined, found := joinedGuardedValue(term, partition); found {
			return joined, nil
		}
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
	if value, found := selectPayloadValue(term, partition); found {
		return value, nil
	}
	if value, found := heapMemberValue(term, partition); found {
		return value, nil
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
		if value, found := typedPathValue(term, partition); found {
			return value, nil
		}
		if joined, found := joinedGuardedValue(term, partition); found {
			return joined, nil
		}
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

// joinedGuardedValue is the only post-join fallback for ordinary values.  A
// single guarded value remains precise; multiple incompatible values collapse
// to Top rather than selecting a writer by operation-name ordering.
func joinedGuardedValue(term []byte, partition equation.Partition) ([]byte, bool) {
	prefix := "value/" + string(term) + "/"
	var value []byte
	for _, fact := range partition.AllValues() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		if value == nil {
			value = fact.Value
			continue
		}
		if string(value) != string(fact.Value) {
			return []byte("scalar/top"), true
		}
	}
	return append([]byte(nil), value...), value != nil
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

// selectPayloadValue is the sole bridge from a selected result's guarded arm
// constraint to an ordinary `result.value` read. The absence of a complete
// select catalog or an active constraint stays unknown.
func selectPayloadValue(term []byte, partition equation.Partition) ([]byte, bool) {
	path := strings.TrimPrefix(string(term), "path/")
	if path == string(term) || !strings.HasSuffix(path, ".value") {
		return nil, false
	}
	result := []byte("path/" + strings.TrimSuffix(path, ".value"))
	selectID, ok := currentEpochFact("select/origin/", result, partition)
	if !ok || len(selectID) == 0 {
		return nil, false
	}
	metaFact, ok := exactFact("select/meta/"+string(selectID), partition)
	if !ok {
		return nil, false
	}
	var meta selectMetaWire
	if json.Unmarshal(metaFact, &meta) != nil || meta.Cases <= 0 {
		return nil, false
	}
	arms := make(map[int]bool, meta.Cases)
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, "select/constraint/") {
			continue
		}
		var constraint selectConstraintWire
		if json.Unmarshal(fact.Value, &constraint) != nil || constraint.Select != string(selectID) || constraint.Default {
			continue
		}
		for _, arm := range constraint.Arms {
			if arm >= 0 && arm < meta.Cases {
				arms[arm] = true
			}
		}
	}
	if len(arms) == 0 {
		for arm := 0; arm < meta.Cases; arm++ {
			arms[arm] = true
		}
	}
	members := make([]typ.Type, 0, len(arms))
	for arm := 0; arm < meta.Cases; arm++ {
		if !arms[arm] {
			continue
		}
		fact, found := exactFact("select/arm/"+string(selectID)+"/"+fmt.Sprintf("%08d", arm), partition)
		if !found {
			return nil, false
		}
		var wire selectArmWire
		if json.Unmarshal(fact, &wire) != nil || wire.Index != arm || wire.Payload == "" {
			return nil, false
		}
		encoded, err := base64.RawURLEncoding.DecodeString(wire.Payload)
		if err != nil {
			return nil, false
		}
		payload, err := typ.DecodeCanonical(context.Background(), encoded)
		if err != nil || payload == nil {
			return nil, false
		}
		members = append(members, payload)
	}
	if len(members) == 0 {
		return nil, false
	}
	value, ok := shapefact.EncodeTarget(typ.MaterializeUnion(members))
	return value, ok
}

func typedPathValue(term []byte, partition equation.Partition) ([]byte, bool) {
	_, _, source, ok := typedAncestor(term, partition)
	if !ok {
		return nil, false
	}
	root, suffix, _, ok := typedAncestor(term, partition)
	if !ok || len(suffix) == 0 || len(root) == 0 {
		return nil, false
	}
	field, found := variant.FieldAtPath(source, suffix)
	if !found {
		if !closedMemberSurface(source) {
			return []byte("scalar/top"), true
		}
		return memberMissingValue(source)
	}
	value, ok := shapefact.EncodeTarget(field)
	return value, ok
}

func closedMemberSurface(value typ.Type) bool {
	value = unwrap.Alias(subst.ExpandInstantiated(value))
	switch item := value.(type) {
	case *typ.Record:
		return item != nil && !item.Open
	case *typ.Union:
		if item == nil || len(item.Members) == 0 {
			return false
		}
		for _, member := range item.Members {
			if !closedMemberSurface(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func typedAncestor(term []byte, partition equation.Partition) ([]byte, []segment.Segment, typ.Type, bool) {
	path := strings.TrimPrefix(string(term), "path/")
	if path == string(term) {
		return nil, nil, nil, false
	}
	for cut := len(path); cut > 0; {
		cut = strings.LastIndexAny(path[:cut], ".[")
		if cut < 0 {
			return nil, nil, nil, false
		}
		root, suffix := path[:cut], path[cut:]
		segs, valid := segment.ParseFormattedSegments(suffix)
		if !valid {
			return nil, nil, nil, false
		}
		value, found := latestValue([]byte("path/"+root), partition)
		if !found {
			value, found = selectPayloadValue([]byte("path/"+root), partition)
			if !found {
				continue
			}
		}
		typeValue, decoded := shapefact.DecodeTarget(value)
		if !decoded {
			continue
		}
		return []byte("path/" + root), segs, typeValue, true
	}
	return nil, nil, nil, false
}

func literalType(value string) (typ.Type, bool) {
	switch {
	case strings.HasPrefix(value, "scalar/string/"):
		decoded, err := strconv.Unquote(strings.TrimPrefix(value, "scalar/string/"))
		return typ.LiteralString(decoded), err == nil
	case strings.HasPrefix(value, "scalar/bool/"):
		decoded, err := strconv.ParseBool(strings.TrimPrefix(value, "scalar/bool/"))
		if err != nil {
			return nil, false
		}
		return typ.LiteralBool(decoded), true
	case strings.HasPrefix(value, "scalar/number/"):
		decoded, err := strconv.ParseInt(strings.TrimPrefix(value, "scalar/number/"), 10, 64)
		if err != nil {
			return nil, false
		}
		return typ.LiteralInt(decoded), true
	default:
		return nil, false
	}
}

func memberMissingValue(receiver typ.Type) ([]byte, bool) {
	encoded, ok := shapefact.EncodeTarget(receiver)
	if !ok {
		return nil, false
	}
	return append([]byte(memberMissingPrefix), encoded...), true
}

func memberMissing(value []byte) bool { _, ok := memberMissingReceiver(value); return ok }

func memberMissingReceiver(value []byte) (typ.Type, bool) {
	if !strings.HasPrefix(string(value), memberMissingPrefix) {
		return nil, false
	}
	return shapefact.DecodeTarget(value[len(memberMissingPrefix):])
}

func memberMissingMessage(source string, value []byte) string {
	receiver, ok := memberMissingReceiver(value)
	if !ok {
		return "member read is not available"
	}
	member := source[strings.LastIndex(source, ".")+1:]
	return fmt.Sprintf("%s has no member %q", typeformat.Short(receiver), member)
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
	if target, ok := shapefact.DecodeTarget(value); ok {
		target = unwrap.Alias(target)
		if target.Kind() == kind.Nil {
			return false, nil
		}
		if !subtype.IsSubtype(typ.Nil, target) {
			return true, nil
		}
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
		if strings.HasPrefix(string(value), "scalar/number/") || strings.HasPrefix(string(value), "scalar/string/") || string(value) == "scalar/table" || string(value) == "scalar/function" || strings.HasPrefix(string(value), "scalar/function/") {
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
