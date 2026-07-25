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
	"sync"
	"time"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/module/exportrelation"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	typeformat "github.com/wippyai/go-lua/analysis/type/format"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const entryValue = "front/closed-entry/v1"

var errUnknownScalar = errors.New("engine: unknown scalar")

var errMultipleChildReturnAlternatives = errors.New("multiple child return alternatives")

// ErrInternalPanic identifies an engine invariant failure that would otherwise
// escape Check as a panic. Check is the public whole-file boundary, so callers
// always receive a named error instead of an unclassified process crash.
var ErrInternalPanic = errors.New("engine: internal panic")

const branchPredicatePrefix = "front/branch-predicate/v1/"
const branchEvidencePrefix = "front/branch-evidence/v1/"

const memberMissingPrefix = "shape/member-missing/v1/"
const literalDiagnosticPrefix = "diagnostic/literal-source/"

// epochFactPrefix keys the sole current-version publication of a term.  Every
// derived fact for that term is published at the operation named by the term's
// latest epoch, so a later epoch is exactly the event that revokes the facts
// established at the earlier one.
const epochFactPrefix = "epoch/"

const summaryTypePrefix = "summary-type/"
const methodReturnSummaryPrefix = "method-return-summary/"
const returnMemberSummaryPrefix = "return-member-summary/"
const assignmentMapReadMissingPrefix = "assignment-map-read-missing/v1/"
const channelPayloadPrefix = "channel-payload/"
const iteratorElementPrefix = "iterator-element/"
const iteratorKeyPrefix = "iterator-key/"
const numericForInductionPrefix = "numeric-for-induction/"

// correlationConeValuePrefix carries a branch-local projection whose authority
// is the exact set of path versions that established it.  It is deliberately
// separate from ordinary value facts: a correlation is not a heap write and
// must disappear as soon as any path in its own proof cone is replaced.
const correlationConeValuePrefix = "correlation-cone/value/"
const returnTupleTruePrefix = "return-tuple-true/"

// Heap facts are deliberately keyed by a sealed allocation identity, never by
// a source path.  Paths are merely lenses: assignments copy an identity and
// member/index writes update the object reached by every such lens.  Keeping
// the identity separate from a shape avoids letting a stale root.field value
// outrank a write made through an alias.
const (
	heapTableIdentityPrefix    = "heap/table-identity/"
	heapTableClosedPrefix      = "heap/table-closed/"
	heapMemberPrefix           = "heap/member/"
	heapMemberIdentityPrefix   = "heap/member-identity/"
	heapStaticReplacePrefix    = "heap/static-replace/"
	memberCellPrefix           = "heap/member-cell/"
	heapMemberOriginPrefix     = "heap/member-origin/"
	heapMetaAttachedPrefix     = "heap/meta-attached/"
	heapMetaNewIndexPrefix     = "heap/meta-newindex/"
	heapExternalCallbackPrefix = "heap/external-callback/"
	heapIndexPresencePrefix    = "heap/index-presence/"
	heapIndexRevokePrefix      = "heap/index-revoke/"
	heapIndexLowerPrefix       = "heap/index-lower/"
	heapIndexUpperPrefix       = "heap/index-upper/"
	heapLengthFloorPrefix      = "heap/length-floor/"
	indexReadDisplayPrefix     = "index-read-display/"
	indexReadScalarPrefix      = "index-read-scalar/"
	typedOptionalReadPrefix    = "typed-optional-read/"
	typePredicateTargetPrefix  = "type-predicate-target/"
	typePredicatePairPrefix    = "type-predicate-pair/"
	typePredicateValuePrefix   = "type-predicate-value/"
	callTypePredicatePrefix    = "call-type-predicate/"
	// optionalResultOriginPrefix names the callee slot whose declared optional
	// result established a value's nil possibility. It is presentation
	// provenance for that same published witness, never a second proof.
	optionalResultOriginPrefix = "optional-provider-origin/"
	// concatOperandOriginPrefix classifies why one concat operand can be nil.
	// The classification is produced by the transaction that already refuted
	// the operand, so publication renders it without re-deriving the cause.
	concatOperandOriginPrefix = "concat-operand-origin/"
	// concatOriginOptionalField and concatOriginOptionalResult are the two
	// closed origins the engine can prove for a nilable concat operand.
	concatOriginOptionalField  = "optional-field"
	concatOriginOptionalResult = "optional-result/"
	// optionalWriteContainerPrefix carries the container witness that refuted a
	// member write, so publication renders the same proof the kernel used.
	optionalWriteContainerPrefix = "optional-write-container/"
	// inlineEvidenceTypeLimit bounds how much rendered type text one evidence
	// line may carry before it reports the relation without the shape.
	inlineEvidenceTypeLimit = 96
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

type Result struct {
	Artifact equation.Artifact
	Values   []equation.Fact
	Outcomes []equation.Fact
	// ReturnCandidates remain equation-owned closed facts for module exporters.
	ReturnCandidates []equation.Fact
	// ValueFacts retain the complete closed partition for module exporters.
	ValueFacts  []equation.Fact
	Diagnostics []equation.Fact
	// PublishedDiagnostics projects canonical diagnostic facts without re-analysis.
	PublishedDiagnostics []PublishedDiagnostic
	// PolicyDiagnostics retain complete source-owned lint facts for adapters
	// that explicitly enable their codes. They never alter engine diagnostics.
	PolicyDiagnostics []PublishedDiagnostic
	// DiagnosticSpans are source-only metadata; equation facts remain portable.
	DiagnosticSpans map[string]wir.Span
	// Placement is nil unless the closure establishes an allocation-site fact.
	Placement *PlacementPlan
	// Native projects every published fact row for native code generators.
	// It reads the same closure channels this result already carries.
	Native *NativeFactIndex
	// TypeDefinitions preserve provider-owned declaration identity at consumers.
	TypeDefinitions map[string]typ.Type
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
	Span        wir.Span
	Kind        string
	Trust       string
	Reason      string
	Message     string
	CausalOrder uint32
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
	return CheckWithImportsAndResolver(source, nil, nil)
}

// CheckWithImports admits resolved module exports as closed entry facts. It
// does not resolve imports itself: callers provide only the manifest exports
// already selected at their project boundary. An unknown export is omitted so
// the equation remains fail-closed rather than treating an unresolved module
// as any.
func CheckWithImports(source string, imports map[string]typ.Type) (result Result, err error) {
	return CheckWithImportsAndResolver(source, imports, nil)
}

// CheckWithImportsAndResolver admits closed import values and the matching
// named-type resolver together. The resolver cannot create runtime facts: it
// only rehydrates annotations whose module manifests were already selected by
// the project boundary.
func CheckWithImportsAndResolver(source string, imports map[string]typ.Type, resolver typeannotation.Resolver) (result Result, err error) {
	return CheckWithImportsResolverAndGlobals(source, imports, nil, resolver)
}

// CheckWithImportsResolverAndGlobals admits project-selected host globals in
// addition to resolved module exports. Global values are a separate capability:
// they are not require results and therefore cannot be reconstructed from an
// import path or source spelling.
func CheckWithImportsResolverAndGlobals(source string, imports, globals map[string]typ.Type, resolver typeannotation.Resolver) (result Result, err error) {
	return CheckWithImportsResolverAndGlobalsAndRelations(source, imports, globals, resolver, nil)
}

// CheckWithImportsResolverAndGlobalsAndRelations consumes project-selected
// finite module result relations alongside their ordinary export types.
// CheckWithImportsResolverAndGlobalsAndRelations consumes project-selected
// finite module result relations alongside their ordinary export types.
// Evaluation always runs to completion: the analysis carries its own
// termination argument (frozen WTO, widening, finite lattices), so no
// resource cap may truncate a file's verdict.
// sourcePath is optional: a project adapter that knows the entry's file name
// supplies it so origin diagnostics can cite the source location they prove.
// Without it those diagnostics stay unpublished rather than naming a file the
// engine cannot know.
func CheckWithImportsResolverAndGlobalsAndRelations(source string, imports, globals map[string]typ.Type, resolver typeannotation.Resolver, relations map[string]exportrelation.Summary, sourcePath ...string) (result Result, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", ErrInternalPanic, recovered)
			result = Result{}
		}
	}()
	compilation, parseElapsed, compileResult, err := compileCheck(source, resolver)
	if err != nil || compileResult.Diagnostics != nil {
		return compileResult, err
	}
	artifact := compilation.Artifact
	evaluateStarted := time.Now()
	binding, err := bindCheckEntry(artifact, imports)
	if err != nil {
		return Result{}, err
	}
	fileContext := context.Background()
	lexical := newLexicalEvaluator(compilation)
	lexical.setImportedTypes(imports)
	lexical.setImportedRelations(relations)
	lexical.setGlobalTypes(globals)
	lexical.setTypeOrigins(compilation)
	if len(sourcePath) != 0 {
		lexical.sourcePath = sourcePath[0]
	}
	lexical.ctx = fileContext
	closure, transactions, err := evaluateCheck(compilation, binding, lexical, fileContext)
	if err != nil {
		var failed conservativeEvaluationError
		if !errors.As(err, &failed) {
			return Result{}, err
		}
		result = diagnosticResult("analysis/conservative", failed.error)
		result.Timings = Timings{ParseBindLower: parseElapsed, Evaluate: time.Since(evaluateStarted)}
		return result, nil
	}
	return projectCheck(compilation, lexical, closure, transactions, parseElapsed, time.Since(evaluateStarted)), nil
}

type conservativeEvaluationError struct{ error }

func compileCheck(source string, resolver typeannotation.Resolver) (front.Compilation, time.Duration, Result, error) {
	started := time.Now()
	compilation, err := front.CompileWithResolver(source, resolver)
	elapsed := time.Since(started)
	if err != nil {
		result := diagnosticResult("analysis/front", err)
		result.Timings.ParseBindLower = elapsed
		return front.Compilation{}, elapsed, result, nil
	}
	if len(compilation.Artifact.Equations) == 0 {
		return front.Compilation{}, elapsed, Result{}, fmt.Errorf("engine: front returned an empty artifact")
	}
	return compilation, elapsed, Result{}, nil
}

func bindCheckEntry(artifact equation.Artifact, imports map[string]typ.Type) (equation.EntryBinding, error) {
	value, err := importEntryValue(imports)
	if err != nil {
		return equation.EntryBinding{}, err
	}
	return equation.EntryBinding{Parameter: artifact.Equations[0].Entry, Value: value}, nil
}

func evaluateCheck(compilation front.Compilation, binding equation.EntryBinding, lexical *lexicalEvaluator, ctx context.Context) (equation.OutputClosure, int, error) {
	artifact := compilation.Artifact
	if compilation.Cyclic == nil {
		bound, err := equation.BindEntry(artifact, binding)
		if err != nil {
			return equation.OutputClosure{}, 0, fmt.Errorf("engine: bind entry: %w", err)
		}
		kernels, err := registry(lexical, importedResultPaths(artifact))
		if err != nil {
			return equation.OutputClosure{}, 0, err
		}
		vm, err := equation.NewAcyclicVM(kernels)
		if err != nil {
			return equation.OutputClosure{}, 0, err
		}
		evaluation, err := vm.Evaluate(bound)
		if err != nil {
			return equation.OutputClosure{}, 0, conservativeEvaluationError{err}
		}
		return evaluation.Closure, evaluation.Transactions, nil
	}
	if _, err := equation.CompileCyclicArtifact(*compilation.Cyclic); err != nil {
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: compile cyclic artifact: %w", err)
	}
	bound, err := equation.BindCyclicEntry(*compilation.Cyclic, binding)
	if err != nil {
		return equation.OutputClosure{}, 0, fmt.Errorf("engine: bind cyclic entry: %w", err)
	}
	kernels, err := cyclicRegistry(lexical, importedResultPaths(artifact))
	if err != nil {
		return equation.OutputClosure{}, 0, err
	}
	vm, err := equation.NewCyclicVM(kernels)
	if err != nil {
		return equation.OutputClosure{}, 0, err
	}
	evaluation, err := vm.Evaluate(ctx, bound, []string{"published"})
	if err != nil {
		return equation.OutputClosure{}, 0, conservativeEvaluationError{err}
	}
	return evaluation.Closure, evaluation.Transactions, nil
}

func projectCheck(compilation front.Compilation, lexical *lexicalEvaluator, closure equation.OutputClosure, transactions int, parseElapsed, evaluateElapsed time.Duration) Result {
	artifact := compilation.Artifact
	closure.Values = append(closure.Values, publishedNativeContracts(compilation)...)
	closure.Values = append(closure.Values, publishedPublicationIdentities(compilation)...)
	closure.Values = append(closure.Values, publishedConstantValues(compilation)...)
	// An unbound annotation has a direct lexical diagnostic. Its unresolved
	// reference must not also be presented as an ordinary failed type claim:
	// no declared type witness exists to validate in the first place.
	if len(compilation.ControlDiagnostics) != 0 {
		unresolvedTypes := make(map[string]bool)
		for _, diagnostic := range compilation.ControlDiagnostics {
			if diagnostic.Code != "type.reference.unresolved" {
				continue
			}
			if name := strings.TrimPrefix(diagnostic.Message, "unknown type "); name != "" {
				unresolvedTypes[name] = true
			}
		}
		if len(unresolvedTypes) != 0 {
			kept := closure.Diagnostics[:0]
			for _, diagnostic := range closure.Diagnostics {
				if strings.HasPrefix(diagnostic.Key, "claim/unproven/") {
					suppressed := false
					for name := range unresolvedTypes {
						if strings.Contains(string(diagnostic.Value), `"`+name+`"`) {
							suppressed = true
							break
						}
					}
					if suppressed {
						continue
					}
				}
				kept = append(kept, diagnostic)
			}
			closure.Diagnostics = kept
		}
	}
	// Static-member-read facts are produced while a child closure is evaluated
	// so object materialization can selectively publish its declared-boundary
	// result. A root write has no allocation boundary to authorize that extra
	// publication; keep the pre-existing root surface unchanged.
	closure.Diagnostics = rootPublishedDiagnostics(artifact, closure.Diagnostics)
	diagnosticSpans := diagnosticSpans(compilation.ClaimSpans, compilation.CallSpans, compilation.BranchSpans, compilation.EffectSpans, compilation.ExpressionSpans, compilation.ReturnSpans, closure.Diagnostics)
	structuralAssignmentDiagnosticSpans(compilation, closure, diagnosticSpans)
	for key, span := range lexical.diagnosticSpans {
		if diagnosticSpans == nil {
			diagnosticSpans = make(map[string]wir.Span)
		}
		diagnosticSpans[key] = span
	}
	diagnosticSpans = lexical.calleeReturnContractSpans(artifact, closure, diagnosticSpans)
	published := publishedDiagnostics(artifact, closure, diagnosticSpans, compilation.ClaimTargetSpans, compilation.CallSpans, compilation.BranchSpans, compilation.ReturnSpans, lexical.lifecycleEvidence, lexical.selectEvidence)
	published = mergeChildPublishedDiagnostics(published, lexical.childPublished)
	for _, diagnostic := range compilation.ControlDiagnostics {
		fact := equation.Fact{Key: diagnostic.Key, Value: []byte(diagnostic.Message)}
		closure.Diagnostics = append(closure.Diagnostics, fact)
		if diagnosticSpans == nil {
			diagnosticSpans = make(map[string]wir.Span)
		}
		diagnosticSpans[diagnostic.Key] = diagnostic.Span
		if diagnostic.Code != "" {
			item := PublishedDiagnostic{Fact: fact, Code: diagnostic.Code, Span: diagnostic.Span, Message: diagnostic.Message, Help: diagnostic.Help}
			for _, evidence := range diagnostic.Evidence {
				item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: evidence.Span, Kind: evidence.Kind, Trust: evidence.Trust, Message: evidence.Message})
			}
			for _, label := range diagnostic.Labels {
				item.Labels = append(item.Labels, DiagnosticLabel{Span: label.Span, Message: label.Message})
			}
			published = append(published, item)
		}
	}
	placement := publishedPlacement(closure.Values)
	for _, item := range published {
		if item.Code != "advice.invariant_loop_read" {
			continue
		}
		if placement == nil {
			placement = &PlacementPlan{Complete: true}
		}
		placement.HoistableLoads = append(placement.HoistableLoads, PlacementHoistableLoad{Target: item.Fact.Key})
	}
	// A returned member closure with an undeclared but statically finite return
	// carries that inferred summary to importers through the module export. The
	// producer owns this projection: the child body it infers from is available
	// only here, never at a downstream consumer.
	closure.Values = append(closure.Values, inferredReturnMemberSummaries(lexical, closure.Values)...)
	outcomes, valueFacts := publishedOutcomes(closure.Outcomes, closure.Values), cloneFacts(closure.Values)
	return Result{
		Artifact: artifact, Values: publishedValues(artifact, closure.Values),
		Outcomes: outcomes, Diagnostics: closure.Diagnostics,
		ReturnCandidates:     cloneFacts(closure.Outcomes),
		ValueFacts:           valueFacts,
		Native:               publishedNativeFactsForCompilation(compilation, valueFacts, outcomes, closure.Diagnostics),
		PublishedDiagnostics: published,
		PolicyDiagnostics:    publishedPolicyDiagnostics(compilation.PolicyDiagnostics),
		DiagnosticSpans:      diagnosticSpans,
		Placement:            placement,
		TypeDefinitions:      cloneTypeDefinitions(compilation.TypeDefinitions),
		Transactions:         transactions,
		Timings:              Timings{ParseBindLower: parseElapsed, Evaluate: evaluateElapsed},
	}
}

// publishedNativeContracts is deliberately a closure publisher, not a side
// channel. The descriptors were derived by front from binder/WIR topology and
// become visible to every existing consumer only once the same evaluation has
// closed successfully.
func publishedNativeContracts(compilation front.Compilation) []equation.Fact {
	if len(compilation.NativeContracts) == 0 {
		return nil
	}
	values := make([]equation.Fact, 0, len(compilation.NativeContracts))
	for index, contract := range compilation.NativeContracts {
		if contract.Family == "" || contract.Value == "" {
			continue
		}
		key := fmt.Sprintf("%s/contract/%08d", contract.Family, index)
		// The subject stays a key segment so the ordinary anchor scan recovers
		// the term and its source display name from published data alone.
		if contract.Subject != "" {
			key += "/" + contract.Subject
		}
		// A contract with several invalidators is one grant with a set of deopt
		// points, so its whole class set travels in one key suffix. Emitting a
		// fact per event would publish the same grant several times over.
		if events := nativeContractEvents(contract.Revocations); events != "" {
			key += "/contract-revocation/" + events
		}
		values = append(values, equation.Fact{Key: key, Value: []byte(contract.Value)})
	}
	return values
}

func nativeContractEvents(revocations []string) string {
	events := make([]string, 0, len(revocations))
	for _, event := range revocations {
		if event != "" {
			events = append(events, event)
		}
	}
	return strings.Join(events, ",")
}

func publishedPolicyDiagnostics(diagnostics []front.ControlDiagnostic) []PublishedDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	out := make([]PublishedDiagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "" || !diagnostic.Span.Valid() {
			continue
		}
		item := PublishedDiagnostic{
			Fact:    equation.Fact{Key: diagnostic.Key, Value: []byte(diagnostic.Message)},
			Code:    diagnostic.Code,
			Span:    diagnostic.Span,
			Message: diagnostic.Message,
			Help:    diagnostic.Help,
		}
		for _, evidence := range diagnostic.Evidence {
			item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: evidence.Span, Kind: evidence.Kind, Trust: evidence.Trust, Message: evidence.Message})
		}
		for _, label := range diagnostic.Labels {
			item.Labels = append(item.Labels, DiagnosticLabel{Span: label.Span, Message: label.Message})
		}
		out = append(out, item)
	}
	return out
}

func cloneTypeDefinitions(in map[string]typ.Type) map[string]typ.Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(in))
	for name, definition := range in {
		if name != "" && definition != nil {
			out[name] = definition
		}
	}
	return out
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

func rootPublishedDiagnostics(artifact equation.Artifact, diagnostics []equation.Fact) []equation.Fact {
	staticReads := make(map[string]bool)
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "source-display" {
				staticReads[operation.Target.Name] = true
				break
			}
		}
	}
	if len(staticReads) == 0 {
		return diagnostics
	}
	filtered := make([]equation.Fact, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		name, missing := strings.CutPrefix(diagnostic.Key, "type.member.missing/")
		if missing && staticReads[name] {
			continue
		}
		filtered = append(filtered, diagnostic)
	}
	return filtered
}

func diagnosticSpans(claimSpans, callSpans, branchSpans, effectSpans, expressionSpans, returnSpans map[string]wir.Span, diagnostics []equation.Fact) map[string]wir.Span {
	if (len(claimSpans) == 0 && len(callSpans) == 0 && len(branchSpans) == 0 && len(effectSpans) == 0 && len(expressionSpans) == 0 && len(returnSpans) == 0) || len(diagnostics) == 0 {
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
			if span, ok := effectSpans[name]; ok {
				out[item.Key] = span
				continue
			}
			if span, ok := callSpans[name+"/call"]; ok {
				out[item.Key] = span
				continue
			}
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
		case strings.HasPrefix(item.Key, "type.operator.concat_operand/"):
			if name := diagnosticOperationName(item.Key); name != "" {
				parts := strings.Split(item.Key, "/")
				if len(parts) == 3 {
					if span, ok := expressionSpans[name+"/"+parts[2]]; ok {
						out[item.Key] = span
					}
				}
			}
			continue
		case strings.HasPrefix(item.Key, "type.operator.comparison_operand/"):
			if name := diagnosticOperationName(item.Key); name != "" {
				if span, ok := expressionSpans[name]; ok {
					out[item.Key] = span
				}
			}
			continue
		case strings.HasPrefix(item.Key, "type.call.optional_receiver/"):
			if span, ok := callSpans[strings.TrimPrefix(item.Key, "type.call.optional_receiver/")+"/call"]; ok {
				out[item.Key] = span
			}
			continue
		case strings.HasPrefix(item.Key, "type.assignment.optional_target/"):
			if span, ok := effectSpans[strings.TrimPrefix(item.Key, "type.assignment.optional_target/")]; ok {
				out[item.Key] = span
			}
			continue
		case strings.HasPrefix(item.Key, "type.return.contract/"):
			name = strings.TrimPrefix(item.Key, "type.return.contract/")
			operation, slot, indexed := strings.Cut(name, "/")
			if indexed {
				if span, ok := returnSpans[operation+"/return-value-"+slot]; ok {
					out[item.Key] = span
					continue
				}
				name = operation
			}
			if span, ok := returnSpans[name]; ok {
				out[item.Key] = span
			}
			continue
		default:
			continue
		}
		if span, ok := claimSpans[name]; ok {
			out[item.Key] = span
		} else if span, ok := effectSpans[name]; ok {
			out[item.Key] = span
		}
	}
	return out
}

// structuralAssignmentDiagnosticSpans replaces a root annotation's source
// anchor only when its closed table literal has a proven refuting member. The
// member coordinates come from WIR TableEntry metadata; no source tree is
// revisited and an open/malformed literal retains the ordinary claim span.
func structuralAssignmentDiagnosticSpans(compilation front.Compilation, closure equation.OutputClosure, spans map[string]wir.Span) {
	if compilation.WIR == nil || len(closure.Diagnostics) == 0 || spans == nil {
		return
	}
	literalMembers := make(map[string]map[string]wir.Span)
	for index := 0; index < compilation.WIR.Len(); index++ {
		instruction := compilation.WIR.Instr(index)
		if instruction.Op != wir.OpMakeTable {
			continue
		}
		members := make(map[string]wir.Span)
		for _, entry := range compilation.WIR.TableEntries(instruction.TableEntries) {
			if entry.ValueSpan.Valid() {
				members[segment.FormatSegments(entry.Suffix.Segments)] = entry.ValueSpan
			}
		}
		if term, ok := tableOperandTerm(compilation.WIR, instruction.Dst); ok && len(members) != 0 {
			literalMembers[term] = members
		}
	}
	for _, diagnostic := range closure.Diagnostics {
		name, assignment := strings.CutPrefix(diagnostic.Key, "type.assignment/")
		if !assignment {
			continue
		}
		var operation equation.Equation
		for _, candidate := range compilation.Artifact.Equations {
			if candidate.Target.Name == name && candidate.Occurrence.Kind == "claim" {
				operation = candidate
				break
			}
		}
		if operation.Target.Name == "" {
			continue
		}
		operands, err := artifactOperandsByRole(operation.Operands, "value")
		if err != nil {
			continue
		}
		members := literalMembers[string(operands["value"])]
		if len(members) == 0 {
			continue
		}
		// claimKernel has already selected a closed, refuted member for this
		// fact. Reuse that semantic suffix solely to recover its WIR source
		// coordinate; a suffix absent from the exact constructor has no span.
		for suffix, span := range members {
			if strings.Contains(string(diagnostic.Value), suffix+" because") {
				spans[diagnostic.Key] = span
				break
			}
		}
	}
}

func tableOperandTerm(body *wir.Body, operand wir.Operand) (string, bool) {
	if body == nil {
		return "", false
	}
	switch operand.Kind {
	case wir.OperandPath:
		path := body.Path(wir.PathRef(operand.Ref))
		if path.IsEmpty() || path.Key() == "" {
			return "", false
		}
		return "path/" + string(path.Key()), true
	case wir.OperandTemp:
		return "temp/" + strconv.FormatUint(uint64(operand.Ref), 10), true
	default:
		return "", false
	}
}

// publishedDiagnostics is the sole rich-diagnostic projection.  Kernels still
// publish equation facts; this function only joins each published fact to the
// claim operation that produced it and to the abstract value already closed by
// the VM.  In particular, it neither evaluates source nor manufactures a
// diagnostic that is absent from closure.Diagnostics.
func publishedDiagnostics(artifact equation.Artifact, closure equation.OutputClosure, spans, claimTargetSpans, callSpans, branchSpans, returnSpans map[string]wir.Span, lifecycleEvidence, selectEvidence map[string][]DiagnosticEvidence) []PublishedDiagnostic {
	if len(closure.Diagnostics) == 0 {
		return nil
	}
	claims := make(map[string]equation.Equation)
	applies := make(map[string]equation.Equation)
	expressions := make(map[string]equation.Equation)
	writes := make(map[string]equation.Equation)
	indexMutations := make(map[string]equation.Equation)
	pathReplacements := make(map[string]equation.Equation)
	publications := make(map[string]equation.Equation)
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind == "publication" {
			publications[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "claim" {
			claims[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "apply" {
			applies[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "expression" {
			expressions[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "environment-write" {
			writes[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "index-mutation" {
			indexMutations[operation.Target.Name] = operation
		}
		if operation.Occurrence.Kind == "path-replacement" {
			pathReplacements[operation.Target.Name] = operation
		}
	}
	out := make([]PublishedDiagnostic, 0, len(closure.Diagnostics))
	for _, fact := range closure.Diagnostics {
		item := PublishedDiagnostic{Fact: cloneFact(fact), Code: diagnosticCode(fact.Key), Span: spans[fact.Key], Message: string(fact.Value)}
		key := fact.Key
		if inner, ok := childDiagnosticKey(fact.Key); ok {
			item.Code = diagnosticCode(inner)
			key = inner
		}
		if projected, ok := projectLifecycleDiagnostic(item, fact, lifecycleEvidence); ok {
			out = append(out, projected)
			continue
		}
		if projected, ok := projectSelectDiagnostic(item, fact, selectEvidence); ok {
			out = append(out, projected)
			continue
		}
		if projected, ok := projectOperationDiagnostic(item, fact, key, artifact, claims, applies, expressions, callSpans, branchSpans, closure.Values); ok {
			out = append(out, projected)
			continue
		}
		if contract, ok := strings.CutPrefix(key, "type.return.contract/"); ok {
			operationName, slot, indexed := strings.Cut(contract, "/")
			if operation, found := publications[operationName]; found && indexed {
				out = append(out, enrichReturnContractDiagnostic(item, operation, operationName, slot, returnSpans))
				continue
			}
			out = append(out, item)
			continue
		}
		if optionalTarget, ok := strings.CutPrefix(fact.Key, "type.assignment.optional_target/"); ok {
			if operation, found := pathReplacements[optionalTarget]; found {
				out = append(out, enrichOptionalWriteTargetDiagnostic(item, operation, optionalTarget, closure.Values))
				continue
			}
			out = append(out, item)
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
				if operation, found := writes[name]; found {
					out = append(out, enrichMissingStaticPathDiagnostic(item, operation))
					continue
				}
				if operation, found := applies[name]; found {
					out = append(out, enrichMissingMemberCallDiagnostic(item, operation))
					continue
				}
			}
			out = append(out, item)
			continue
		}
		operation, found := claims[name]
		if !found {
			if mutation, found := indexMutations[name]; found {
				out = append(out, enrichClosedDynamicWriteDiagnostic(item, mutation, claimTargetSpans[name]))
				continue
			}
			if replacement, found := pathReplacements[name]; found && functionContractDiagnostic(name, closure.Values) {
				out = append(out, enrichFunctionContractWriteDiagnostic(item, replacement, claimTargetSpans[name], closure))
				continue
			}
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
		resultDisplay, callResultOperation, hasResultDisplay := callResultDisplay(artifact, operands["value"])
		hasResultDisplay = hasResultDisplay && (hasImportedRelationResult(closure.Values, operands["value"]) || hasCurrentSummaryFact(methodReturnSummaryPrefix, operands["value"], closure.Values) || isUnvalidatedAnyValue(value))
		if hasResultDisplay {
			sourceDisplay = resultDisplay
			if span := callSpans[callResultOperation+"/call"]; span.Valid() {
				item.Span = span
			}
		}
		// An explicit any may be carried by an ancestor path (for example
		// raw.id).  The scalar read can still retain a literal heap fact, but the
		// ancestor boundary is the authoritative assignment source and is itself
		// sufficient closed evidence for the diagnostic projection.
		anySource := (available && isUnvalidatedAnyValue(value)) || sourceHasAnyBoundary(operands["value"], closure.Values)
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
		methodSelector := false
		for _, operand := range operation.Operands {
			methodSelector = methodSelector || operand.Role == "source-method-selector"
		}
		mapReadMissing := strings.HasPrefix(item.Message, assignmentMapReadMissingPrefix)
		if mapReadMissing {
			item.Message = strings.TrimPrefix(item.Message, assignmentMapReadMissingPrefix)
		}
		mayBeNil := strings.HasSuffix(item.Message, " because it may be nil") || (methodSelector && diagnosticValueMayBeNil(value))
		if mayBeNil {
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			concrete := "value"
			hasConcrete := false
			if witness, ok := shapefact.DecodeTarget(value); ok && witness != nil {
				concrete, hasConcrete = optionalEvidenceDisplay(witness)
			}
			if !hasConcrete {
				concrete, hasConcrete = currentIndexScalarDisplay(operands["value"], operation, closure.Values)
			}
			if mapReadMissing {
				concrete = "nil"
				hasConcrete = false
			}
			valueEvidence := fmt.Sprintf("%s can be %s or nil here", sourceDisplay, concrete)
			if !hasConcrete || concrete == "nil" {
				valueEvidence = fmt.Sprintf("%s can be nil here", sourceDisplay)
			}
			item.Message = fmt.Sprintf("cannot assign %s because it may be nil", sourceDisplay)
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: valueEvidence},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
			}
			missing, reason := fmt.Sprintf("no guard on this path proves %s is non-nil", sourceDisplay), "boundary validation missing"
			if strings.Contains(sourceDisplay, "[") {
				if dot := strings.LastIndex(sourceDisplay, "."); dot > 0 && strings.Contains(sourceDisplay[:dot], "[") {
					parent := sourceDisplay[:dot]
					item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s may be nil before reading %s", parent, sourceDisplay[dot:])})
				}
				missing = fmt.Sprintf("%s is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared type here", sourceDisplay)
				reason = "index read validation missing"
			} else if dot := strings.LastIndex(sourceDisplay, "."); dot > 0 && methodSelector {
				parent := sourceDisplay[:dot]
				item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s may be nil before reading %s", parent, sourceDisplay[dot:])})
			}
			item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: item.Span, Kind: "missing proof", Trust: "unknown", Reason: reason, Message: missing})
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value may be nil"}, {Span: targetSpan, Message: "declared type " + declared}}
			item.Help = fmt.Sprintf("Guard `%s` with a nil check before assigning it, or change the target type to accept nil.", sourceDisplay)
			out = append(out, item)
			continue
		}
		valueDescription := assignmentEvidenceValue(value)
		memberSurface, hasMemberSurface := assignmentMemberSurface(name, closure.Values)
		if hasMemberSurface {
			if actual, expected, ok := assignmentDiagnosticFunctionTypes(item.Message); ok {
				valueDescription, declared = actual, expected
				if source, ok := assignmentDiagnosticSource(item.Message); ok {
					sourceDisplay = source
				}
			} else {
				valueDescription = "fun() -> " + assignmentEvidenceValue(memberSurface)
			}
		}
		structuralMismatch := assignmentMismatch{}
		hasStructuralMismatch := false
		missingField, missingFieldType := "", typ.Type(nil)
		hasMissingField := false
		// An explicit-any boundary is the authoritative source fact. Its earlier
		// initializer can be a sealed table, but that table is not a validation
		// proof and must not replace the boundary diagnostic with a member error.
		if !anySource {
			for _, operand := range operation.Operands {
				if operand.Role != "shape-target" {
					continue
				}
				if target, ok := shapefact.DecodeTarget(operand.Term.Encoding); ok {
					structuralMismatch, hasStructuralMismatch = firstAssignmentMismatch(value, target)
					if !hasStructuralMismatch {
						missingField, missingFieldType, hasMissingField = missingRequiredField(value, target)
					}
				}
				break
			}
		}
		if returnSpan, ok := spans[fact.Key+declaredReturnSpanSuffix]; ok && returnSpan.Valid() && !anySource {
			shapeTarget, _ := artifactOperand(operation.Operands, "shape-target")
			if projected, published := directCallResultAssignment(artifact, closure.Values, operands["value"], shapeTarget); published {
				targetSpan := claimTargetSpans[name]
				if !targetSpan.Valid() {
					targetSpan = item.Span
				}
				subject := fmt.Sprintf("call result %d", projected.index+1)
				item.Code = "type.call.direct.result_assignment"
				item.Message = fmt.Sprintf("%s is %s, not %s", subject, projected.result, declared)
				item.Evidence = []DiagnosticEvidence{
					{Span: returnSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s declares %s as %s", projected.callee, subject, projected.result)},
					{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("assignment target %s requires %s", display, declared)},
				}
				item.Labels = []DiagnosticLabel{
					{Span: item.Span, Message: "call result"},
					{Span: targetSpan, Message: "declared type " + declared},
					{Span: returnSpan, Message: "declared return type " + projected.result},
				}
				item.Help = "Assign the call result to a compatible target type, or change the callee return type if this result is valid."
				out = append(out, item)
				continue
			}
		}
		if hasMissingField {
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			fieldPath := display + "." + missingField
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "object literal has type " + boundaryShapeEvidenceValue(value)},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("required field %s has type %s, but the object literal does not provide it", fieldPath, typeformat.Short(missingFieldType))},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "object literal"}, {Span: targetSpan, Message: "declared type " + declared}}
			item.Help = fmt.Sprintf("Add field `%s`, or make it optional in the declared type if it may be absent.", missingField)
			out = append(out, item)
			continue
		}
		if hasStructuralMismatch {
			sourceDisplay += structuralMismatch.Suffix
			valueDescription = assignmentEvidenceValue(structuralMismatch.Value)
			declared = typeformat.Short(structuralMismatch.Expected)
		}
		if anySource {
			valueDescription = "any"
			if hasResultDisplay {
				item.Message = fmt.Sprintf("cannot assign %s because it is %s, not %s", sourceDisplay, valueDescription, declared)
			}
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
			item.Help = "Use a value compatible with the expected type, or change the target type if `" + sourceDisplay + "` is valid."
			out = append(out, item)
			continue
		}
		if hasStructuralMismatch {
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has literal value %s", sourceDisplay, valueDescription)},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", sourceDisplay, declared)},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueDescription}, {Span: targetSpan, Message: "declared type " + declared}}
		} else if hasMemberSurface {
			// The callable result comes from a closed member-surface publication.
			// Keep that expression as the proven evidence subject; the annotation
			// target is an independent user claim with its own source span.
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type %s", sourceDisplay, valueDescription)},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueDescription}, {Span: targetSpan, Message: "declared type " + declared}}
		} else if _, typed := shapefact.DecodeTarget(value); typed || string(value) == "scalar/nil" {
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
			targetSpan := claimTargetSpans[name]
			if !targetSpan.Valid() {
				targetSpan = item.Span
			}
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has literal value %s", sourceDisplay, valueDescription)},
				{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s is declared as %s", display, declared)},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueDescription}, {Span: targetSpan, Message: "declared type " + declared}}
		}
		if hasResultDisplay && strings.HasPrefix(string(value), "scalar/") && string(value) != "scalar/nil" {
			item.Message = fmt.Sprintf("cannot assign %s because it is %s, not %s", sourceDisplay, valueDescription, declared)
		}
		item.Help = "Use a value compatible with the expected type, or change the target type if `" + sourceDisplay + "` is valid."
		out = append(out, item)
	}
	return out
}

// diagnosticValueMayBeNil accepts only an already-closed shape witness. It is
// used by the publisher to narrate a union-with-nil assignment as the same
// nil-proof obligation that the checker established, never to infer nilability
// from a rendered message.
func diagnosticValueMayBeNil(value []byte) bool {
	witness, ok := shapefact.DecodeTarget(value)
	return ok && witness != nil && unwrap.IsOptionalLike(witness)
}

// optionalEvidenceDisplay preserves a concrete spelling only for scalar
// leaves. A canonical structural witness proves nilability, but it does not
// retain a stable public name for an imported record or callable. Publishing
// that reconstructed shape as an explanation would overstate what crossed the
// transport, so those reads report only the proven nil possibility.
func optionalEvidenceDisplay(witness typ.Type) (string, bool) {
	witness = unwrap.Alias(subst.ExpandInstantiated(proof.ProjectionWithoutNil(witness)))
	if !scalarEvidenceType(witness) {
		return "", false
	}
	return typeformat.Short(witness), true
}

func scalarEvidenceType(witness typ.Type) bool {
	if witness == nil {
		return false
	}
	switch witness.Kind() {
	case kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return true
	case kind.Union:
		union, ok := unwrap.Alias(subst.ExpandInstantiated(witness)).(*typ.Union)
		if !ok || union == nil || len(union.Members) == 0 {
			return false
		}
		for _, member := range union.Members {
			if !scalarEvidenceType(member) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// currentIndexScalarDisplay admits a diagnostic spelling only when the
// producing index read explicitly classified its existing type witness as a
// scalar leaf. The marker is carried through ordinary writes with the value's
// current epoch, so an older display cannot survive reassignment.
func currentIndexScalarDisplay(term []byte, operation equation.Equation, values []equation.Fact) (string, bool) {
	valuePrefix := "value/" + string(term) + "/"
	latest := ""
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, valuePrefix) && fact.Key > latest {
			latest = fact.Key
		}
	}
	if latest == "" {
		return dependentIndexScalarDisplay(operation, values)
	}
	operationName := strings.TrimPrefix(latest, valuePrefix)
	marker := indexReadScalarPrefix + string(term) + "/" + operationName
	display := indexReadDisplayPrefix + string(term) + "/" + operationName
	for _, fact := range values {
		if fact.Key == marker && string(fact.Value) == "scalar" {
			for _, candidate := range values {
				if candidate.Key == display && len(candidate.Value) != 0 {
					return string(candidate.Value), true
				}
			}
		}
	}
	return dependentIndexScalarDisplay(operation, values)
}

func dependentIndexScalarDisplay(operation equation.Equation, values []equation.Fact) (string, bool) {
	display := ""
	for _, dependency := range operation.Dependencies {
		suffix := "/" + dependency.Name
		for _, marker := range values {
			if !strings.HasPrefix(marker.Key, indexReadScalarPrefix) || !strings.HasSuffix(marker.Key, suffix) || string(marker.Value) != "scalar" {
				continue
			}
			candidate := ""
			for _, fact := range values {
				if fact.Key == indexReadDisplayPrefix+strings.TrimPrefix(marker.Key, indexReadScalarPrefix) && len(fact.Value) != 0 {
					candidate = string(fact.Value)
					break
				}
			}
			if candidate == "" || (display != "" && display != candidate) {
				return "", false
			}
			display = candidate
		}
	}
	return display, display != ""
}

func functionContractDiagnostic(operation string, facts []equation.Fact) bool {
	for _, fact := range facts {
		if fact.Key == "assignment-function-contract/"+operation && string(fact.Value) == "refuted" {
			return true
		}
	}
	return false
}

// enrichReturnContractDiagnostic narrates the refuted return slot against the
// annotation it must satisfy. Both anchors are already-published source
// metadata: the returned expression's own span and the authored return type's
// span. The declared contract is a user assertion, never a proven fact.
func enrichReturnContractDiagnostic(item PublishedDiagnostic, operation equation.Equation, operationName, slot string, returnSpans map[string]wir.Span) PublishedDiagnostic {
	index, err := strconv.Atoi(slot)
	if err != nil || index < 0 {
		return item
	}
	display := ""
	for _, operand := range operation.Operands {
		if operand.Role == fmt.Sprintf("return-display-%08d", index) {
			display = string(operand.Term.Encoding)
			break
		}
	}
	declared := ""
	for _, operand := range operation.Operands {
		if operand.Role != fmt.Sprintf("declared-return-%08d", index) {
			continue
		}
		if target, ok := shapefact.DecodeTarget(operand.Term.Encoding); ok && target != nil {
			declared = typeformat.Short(target)
		}
		break
	}
	if declared == "" {
		return item
	}
	subject := returnValueSubject(index, display)
	declaredSpan := returnSpans[fmt.Sprintf("%s/declared-return-%08d", operationName, index)]
	if !declaredSpan.Valid() {
		declaredSpan = item.Span
	}
	valueEvidence := fmt.Sprintf("%s has literal value %s", subject, returnContractObservedValue(item.Message, subject))
	if strings.Contains(item.Message, " may be nil, not ") {
		valueEvidence = fmt.Sprintf("%s can be nil here", subject)
	}
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: valueEvidence},
		{Span: declaredSpan, Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("returned value %d must satisfy declared return type %s", index+1, declared)},
	}
	item.Labels = []DiagnosticLabel{
		{Span: item.Span, Message: "returned value"},
		{Span: declaredSpan, Message: "declared return type " + declared},
	}
	item.Help = "Return a value compatible with the declared return type, or change the return annotation if this value is valid."
	return item
}

// returnContractObservedValue recovers the observed value the publication
// transaction already reported. The message is that transaction's own closed
// rendering, so no value is re-derived here.
func returnContractObservedValue(message, subject string) string {
	rest, ok := strings.CutPrefix(message, subject+" is ")
	if !ok {
		return ""
	}
	cut := strings.LastIndex(rest, ", not ")
	if cut < 0 {
		return rest
	}
	return rest[:cut]
}

// enrichOptionalWriteTargetDiagnostic narrates the container proof that
// refuted this member write. The container witness is the fact the write
// transaction published; the target and container spellings come from the same
// operation's source operands.
func enrichOptionalWriteTargetDiagnostic(item PublishedDiagnostic, operation equation.Equation, name string, values []equation.Fact) PublishedDiagnostic {
	target, container := "", ""
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "display":
			target = string(operand.Term.Encoding)
		case "write-container-display":
			container = string(operand.Term.Encoding)
		}
	}
	if target == "" || container == "" {
		return item
	}
	witness := []byte(nil)
	for _, fact := range values {
		if fact.Key == optionalWriteContainerPrefix+name {
			witness = fact.Value
			break
		}
	}
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: optionalContainerEvidence(container, witness)},
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("writing %s requires its container to be non-nil", target)},
	}
	item.Labels = []DiagnosticLabel{
		{Span: item.Span, Message: "possibly nil container"},
		{Span: item.Span, Message: "assignment target"},
	}
	item.Help = fmt.Sprintf("Guard `%s` with a nil check before assigning through it, or write to a non-optional container.", container)
	return item
}

// optionalContainerEvidence renders the container's proven nilability. The
// present projection is inlined only while it stays readable; a large shape
// reports the nil possibility alone rather than a wall of type text.
func optionalContainerEvidence(container string, witness []byte) string {
	decoded, ok := shapefact.DecodeTarget(witness)
	if ok && decoded != nil {
		if present := proof.ProjectionWithoutNil(decoded); present != nil && !typ.IsNever(present) {
			if rendered := typeformat.Short(present); rendered != "" && len(rendered) <= inlineEvidenceTypeLimit {
				return fmt.Sprintf("%s can be %s or nil here", container, rendered)
			}
		}
	}
	return fmt.Sprintf("%s can be nil here", container)
}

func enrichFunctionContractWriteDiagnostic(item PublishedDiagnostic, operation equation.Equation, targetSpan wir.Span, closure equation.OutputClosure) PublishedDiagnostic {
	operands, err := artifactOperandsByRole(operation.Operands, "display", "value")
	if err != nil {
		return item
	}
	value, available := claimDiagnosticValue(operands["value"], operation, closure)
	if !available {
		return item
	}
	actual := assignmentEvidenceValue(value)
	display := string(operands["display"])
	_, suffix, found := strings.Cut(item.Message, " because assigned value is ")
	if !found {
		return item
	}
	_, declared, found := strings.Cut(suffix, ", not ")
	if !found || declared == "" {
		return item
	}
	if !targetSpan.Valid() {
		targetSpan = item.Span
	}
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "assigned value has literal value " + actual},
		{Span: targetSpan, Kind: "user assertion", Trust: "claimed", Message: display + " is declared as " + declared},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + actual}, {Span: targetSpan, Message: "declared type " + declared}}
	item.Help = "Use a value compatible with the expected type, or change the target type if the assigned value is valid."
	return item
}

// callResultDisplay returns display metadata only from the call-results
// operation that owns this exact result term. The metadata is presentation
// evidence; missing, ambiguous, or malformed displays remain unavailable.
func callResultDisplay(artifact equation.Artifact, result []byte) (string, string, bool) {
	if len(result) == 0 {
		return "", "", false
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		matched := false
		display := ""
		application := ""
		provider := []byte(nil)
		method := ""
		for _, operand := range operation.Operands {
			switch {
			case strings.HasPrefix(operand.Role, "result-") && string(operand.Term.Encoding) == string(result):
				matched = true
			case operand.Role == "result-display":
				display = string(operand.Term.Encoding)
			case operand.Role == "application":
				application = strings.TrimPrefix(string(operand.Term.Encoding), "call/")
			case operand.Role == "provider":
				provider = operand.Term.Encoding
			case operand.Role == "method":
				method, _ = callMethodName(operand.Term.Encoding)
			}
		}
		if display == "" && method != "" && providerName(provider) != "" {
			display = providerName(provider) + ":" + method + "(...)"
		}
		if matched && display != "" {
			return display, application, true
		}
	}
	return "", "", false
}

// directCallResultProjection is the callee-owned contract an assignment
// diagnostic reports when its source is a local call result.
type directCallResultProjection struct {
	callee string
	result string
	index  int
}

// directCallResultAssignment reports the callee contract that the assignment
// target refutes. The declared result is the callee's own published return
// slot, so the diagnostic names the contract the reader has to change rather
// than whichever value this call happened to produce.
func directCallResultAssignment(artifact equation.Artifact, values []equation.Fact, source, targetType []byte) (directCallResultProjection, bool) {
	application, index, resolved := directCallResultSlot(artifact, source)
	if !resolved {
		return directCallResultProjection{}, false
	}
	apply, found := artifactEquation(artifact, application, "apply")
	if !found {
		return directCallResultProjection{}, false
	}
	callee, hasCallee := artifactOperand(apply.Operands, "callee")
	display, hasDisplay := artifactOperand(apply.Operands, "callee-display")
	if !hasCallee || !hasDisplay || len(display) == 0 {
		return directCallResultProjection{}, false
	}
	result, declared := calleeDeclaredResult(values, callee, index)
	if !declared {
		return directCallResultProjection{}, false
	}
	encoded, ok := shapefact.EncodeTarget(result)
	target, decoded := shapefact.DecodeTarget(targetType)
	if !ok || !decoded || target == nil || valueAgainstType(encoded, target) != shapeRefuted {
		return directCallResultProjection{}, false
	}
	return directCallResultProjection{callee: string(display), result: typeformat.Short(result), index: index}, true
}

// declaredReturnSpanSuffix names the secondary anchor an assignment diagnostic
// carries when its source is a call result: the callee's own authored return
// annotation, which lives in a different body.
const declaredReturnSpanSuffix = "/declared-return"

// calleeReturnContractSpans anchors every assignment whose source is a local
// call result to the callee's authored return annotation. The callee body is
// already admitted by this evaluator, so the anchor is existing metadata rather
// than a second source read. A callee without an authored annotation, or one
// this evaluator never admitted, contributes no anchor and the assignment keeps
// its ordinary presentation.
func (l *lexicalEvaluator) calleeReturnContractSpans(artifact equation.Artifact, closure equation.OutputClosure, spans map[string]wir.Span) map[string]wir.Span {
	if l == nil {
		return spans
	}
	for _, fact := range closure.Diagnostics {
		name, assignment := strings.CutPrefix(fact.Key, "type.assignment/")
		if !assignment {
			continue
		}
		claim, found := artifactEquation(artifact, name, "claim")
		if !found {
			continue
		}
		source, hasSource := artifactOperand(claim.Operands, "value")
		if !hasSource {
			continue
		}
		application, index, resolved := directCallResultSlot(artifact, source)
		if !resolved {
			continue
		}
		apply, found := artifactEquation(artifact, application, "apply")
		if !found {
			continue
		}
		callee, hasCallee := artifactOperand(apply.Operands, "callee")
		if !hasCallee {
			continue
		}
		span, ok := l.declaredReturnSpan(closure.Values, callee, index)
		if !ok {
			continue
		}
		if spans == nil {
			spans = make(map[string]wir.Span)
		}
		spans[fact.Key+declaredReturnSpanSuffix] = span
	}
	return spans
}

// declaredReturnSpan reads the authored return annotation of the body a callee
// term currently holds. The closure capability is the identity proof: an opaque
// or reassigned callable has no admitted body and therefore no anchor.
func (l *lexicalEvaluator) declaredReturnSpan(values []equation.Fact, callee []byte, index int) (wir.Span, bool) {
	handle, found := closureHandleFromValues(values, callee)
	if !found {
		return wir.Span{}, false
	}
	compilation, admitted := l.byPrototype[handle.Prototype]
	if !admitted || compilation.WIR == nil {
		return wir.Span{}, false
	}
	declared := compilation.WIR.DeclaredReturnSpans()
	if index < 0 || index >= len(declared) || !declared[index].Valid() {
		return wir.Span{}, false
	}
	return declared[index], true
}

// closureHandleFromValues resolves a term's current closure capability from an
// already-published fact list, using the same latest-epoch ordering as the
// partition lookup.
func closureHandleFromValues(values []equation.Fact, term []byte) (closureHandle, bool) {
	prefix := "closure/" + string(term) + "/"
	latest, encoded := "", []byte(nil)
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, prefix) && fact.Key > latest {
			latest, encoded = fact.Key, fact.Value
		}
	}
	if encoded == nil {
		return closureHandle{}, false
	}
	var handle closureHandle
	return handle, json.Unmarshal(encoded, &handle) == nil && validClosureHandle(handle)
}

// artifactEquation finds one operation by name and occurrence kind.
func artifactEquation(artifact equation.Artifact, name, kind string) (equation.Equation, bool) {
	for _, operation := range artifact.Equations {
		if operation.Target.Name == name && operation.Occurrence.Kind == kind {
			return operation, true
		}
	}
	return equation.Equation{}, false
}

// directCallResultSlot resolves the call-results operation that owns a term and
// the result slot that term fills.
func directCallResultSlot(artifact equation.Artifact, result []byte) (string, int, bool) {
	if len(result) == 0 {
		return "", 0, false
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		application, index, matched := "", 0, false
		for _, operand := range operation.Operands {
			if operand.Role == "application" {
				application = strings.TrimPrefix(string(operand.Term.Encoding), "call/")
				continue
			}
			if slot, value, indexed := indexedRoleValue(operand.Role, "result-", operand.Term.Encoding); indexed && value == string(result) {
				index, matched = slot, true
			}
		}
		if matched && application != "" {
			return application, index, true
		}
	}
	return "", 0, false
}

// calleeDeclaredResult reads the declared result slot of the sealed function
// value a callee term currently holds.
func calleeDeclaredResult(values []equation.Fact, callee []byte, index int) (typ.Type, bool) {
	prefix := "value/" + string(callee) + "/"
	latest, encoded := "", []byte(nil)
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, prefix) && fact.Key > latest {
			latest, encoded = fact.Key, fact.Value
		}
	}
	functionType, ok := sealedFunctionType(encoded)
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(functionType)).(*typ.Function)
	if !ok || function == nil || len(function.TypeParams) != 0 || index < 0 || index >= len(function.Returns) {
		return nil, false
	}
	return function.Returns[index], function.Returns[index] != nil
}

func hasImportedRelationResult(values []equation.Fact, result []byte) bool {
	if len(result) == 0 {
		return false
	}
	prefix := "imported-relation-result/" + base64.RawURLEncoding.EncodeToString(result) + "/"
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "scalar/bool/true" {
			return true
		}
	}
	return false
}

func assignmentMemberSurface(operation string, facts []equation.Fact) ([]byte, bool) {
	prefix := "assignment-member-surface/" + operation
	for _, fact := range facts {
		if fact.Key == prefix && len(fact.Value) != 0 {
			return append([]byte(nil), fact.Value...), true
		}
	}
	return nil, false
}

func assignmentDiagnosticFunctionTypes(message string) (actual, expected string, ok bool) {
	_, tail, found := strings.Cut(message, " because it is ")
	if !found {
		return "", "", false
	}
	actual, expected, found = strings.Cut(tail, ", not ")
	return actual, expected, found && strings.HasPrefix(actual, "fun() -> ") && strings.HasPrefix(expected, "fun() -> ")
}

func assignmentDiagnosticSource(message string) (string, bool) {
	prefix := "cannot assign "
	if !strings.HasPrefix(message, prefix) {
		return "", false
	}
	source, _, found := strings.Cut(strings.TrimPrefix(message, prefix), " because it is ")
	return source, found && source != ""
}

func enrichMissingStaticPathDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
	operands, err := artifactOperandsByRole(operation.Operands, "source-display")
	if err != nil {
		return item
	}
	source := string(operands["source-display"])
	member := source[strings.LastIndex(source, ".")+1:]
	if bracket := strings.LastIndex(member, "["); bracket >= 0 {
		member = strings.Trim(strings.TrimSuffix(member[bracket+1:], "]"), "\"")
	}
	if source == "" || member == "" {
		return item
	}
	separator := strings.Index(item.Message, " has no member ")
	if separator < 0 {
		return item
	}
	receiver := item.Message[:separator]
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s reads member %q from receiver type %s", source, member, receiver)}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "member read"}}
	item.Help = fmt.Sprintf("Narrow the receiver before reading `%s`, or add `%s` to every reachable receiver shape.", member, member)
	return item
}

func projectLifecycleDiagnostic(item PublishedDiagnostic, fact equation.Fact, evidence map[string][]DiagnosticEvidence) (PublishedDiagnostic, bool) {
	if isChannelLifecycleDiagnostic(fact.Key) {
		return enrichChannelLifecycleDiagnostic(item), true
	}
	if isResourceTypestateDiagnostic(fact.Key) {
		return enrichResourceTypestateDiagnostic(item), true
	}
	if inner, _ := childDiagnosticKey(fact.Key); strings.HasPrefix(inner, "effect.lifecycle.unreleased/") {
		return enrichUnreleasedLifecycleDiagnostic(item, evidence[fact.Key]), true
	}
	return PublishedDiagnostic{}, false
}

func projectSelectDiagnostic(item PublishedDiagnostic, fact equation.Fact, evidence map[string][]DiagnosticEvidence) (PublishedDiagnostic, bool) {
	inner, _ := childDiagnosticKey(fact.Key)
	if !strings.HasPrefix(inner, "channel.select.exhaustiveness/") && !strings.HasPrefix(inner, "lint.union.exhaustiveness/") {
		return PublishedDiagnostic{}, false
	}
	item.Evidence = append([]DiagnosticEvidence(nil), evidence[fact.Key]...)
	if strings.HasPrefix(inner, "lint.union.exhaustiveness/") {
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "union case check"}}
		item.Help = "Handle each missing case, or add an else branch when a fallback is valid."
	} else {
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "channel case check"}}
		item.Help = "Add an elseif branch for each missing case, or add a default branch when a fallback is valid."
	}
	return item, true
}

func projectOperationDiagnostic(item PublishedDiagnostic, fact equation.Fact, key string, artifact equation.Artifact, claims, applies, expressions map[string]equation.Equation, callSpans, branchSpans map[string]wir.Span, values []equation.Fact) (PublishedDiagnostic, bool) {
	if strings.HasPrefix(fact.Key, "effect.freeze.mutation/") {
		return enrichFrozenMutationDiagnostic(item, artifact, callSpans), true
	}
	if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "type.call.direct.") {
		return enrichDirectCallDiagnostic(item, operation, values), true
	}
	if name, ok := strings.CutPrefix(key, "type.call.optional_receiver/"); ok {
		if operation, found := applies[name]; found {
			return enrichOptionalReceiverDiagnostic(item, operation), true
		}
	}
	if operation, ok := expressions[diagnosticOperationName(key)]; ok && strings.HasPrefix(key, "type.operator.concat_operand/") {
		return enrichConcatOperandDiagnostic(item, operation, values), true
	}
	if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "advice.redundant_claim/") {
		return enrichRedundantCastDiagnostic(item, operation, callSpans), true
	}
	if operation, ok := applies[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "send.isolation/") {
		return enrichSendIsolationDiagnostic(item, operation), true
	}
	if operation, ok := claims[diagnosticOperationName(fact.Key)]; ok && strings.HasPrefix(fact.Key, "advice.redundant_claim/") {
		return enrichRedundantClaimDiagnostic(item, operation), true
	}
	if operation, ok := branchOperation(artifact, diagnosticOperationName(fact.Key)); ok && (strings.HasPrefix(fact.Key, "advice.always_true_guard/") || strings.HasPrefix(fact.Key, "lint.condition.redundant/")) {
		return enrichConstantGuardDiagnostic(item, operation, fact.Key, artifact, branchSpans), true
	}
	return PublishedDiagnostic{}, false
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
		if operand.Role == "write-container-display" || (callMutation && operand.Role == "argument-display-00000000") {
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

// enrichOptionalReceiverDiagnostic names the receiver and the member the call
// selects. Both come from the call operation's own source operands, so the
// explanation identifies the guard the reader has to add.
func enrichOptionalReceiverDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
	receiver, method := "", ""
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "receiver-display":
			receiver = string(operand.Term.Encoding)
		case "method":
			method, _ = callMethodName(operand.Term.Encoding)
		}
	}
	if receiver == "" || method == "" {
		return item
	}
	selector := receiver + "." + method
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("receiver %s is optional at call to %s", receiver, selector)},
		{Span: item.Span, Kind: "missing proof", Trust: "unknown", Message: fmt.Sprintf("no nil check proves receiver %s is present before calling %s", receiver, selector)},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "method call"}}
	item.Help = fmt.Sprintf("check %s ~= nil before calling %s.", receiver, selector)
	return item
}

// enrichMissingMemberCallDiagnostic narrates a method call whose receiver
// contract has no such member. The receiver spelling and selector come from
// the call operation; the receiver type is the closed publication the kernel
// already reported.
func enrichMissingMemberCallDiagnostic(item PublishedDiagnostic, operation equation.Equation) PublishedDiagnostic {
	receiver, method := "", ""
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "receiver-display":
			receiver = string(operand.Term.Encoding)
		case "method":
			method, _ = callMethodName(operand.Term.Encoding)
		}
	}
	const marker = " has no member "
	cut := strings.Index(item.Message, marker)
	if receiver == "" || method == "" || cut <= 0 {
		return item
	}
	item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s.%s has receiver type %s", receiver, method, item.Message[:cut])}}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "member call"}}
	item.Help = fmt.Sprintf("Narrow the receiver before reading `%s`, or add `%s` to every reachable receiver shape.", method, method)
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
	// A constructor can publish its root shape before later per-member writes
	// complete the same statement. Those writes are the claim's direct CFG
	// predecessor, while the root shape remains its exact source value. Recover
	// only that earlier, non-refinement root fact; a later source write or a
	// claim output is never accepted as evidence for this diagnostic.
	var source []byte
	latest := ""
	for _, fact := range closure.Values {
		if !strings.HasPrefix(fact.Key, prefix) || isClaimRefinement(fact.Value) {
			continue
		}
		name := strings.TrimPrefix(fact.Key, prefix)
		if name >= operation.Target.Name || (latest != "" && name <= latest) {
			continue
		}
		source, latest = fact.Value, name
	}
	if source != nil && shapefact.IsTable(source) {
		return append([]byte(nil), source...), true
	}
	// A dynamic member read publishes its resolved value at the read-result
	// term, while a following annotation retains the equivalent static member
	// path for source presentation. The claim's direct dependency is the
	// authoritative bridge between those terms. Accept one and only one value
	// published by that dependency; multiple results remain ambiguous and fail
	// closed.
	for _, dependency := range operation.Dependencies {
		suffix := "/" + dependency.Name
		var value []byte
		for _, fact := range closure.Values {
			if !strings.HasPrefix(fact.Key, "value/") || !strings.HasSuffix(fact.Key, suffix) {
				continue
			}
			if value != nil {
				value = nil
				break
			}
			value = fact.Value
		}
		if value != nil {
			return append([]byte(nil), value...), true
		}
	}
	return nil, false
}

func diagnosticOperationName(key string) string {
	for _, prefix := range []string{"advice.redundant_claim/", "advice.always_true_guard/", "lint.condition.redundant/", "send.isolation/", "effect.freeze.mutation/", "effect.lifecycle.unreleased/", "channel.send.closed/", "channel.close.closed/", "typestate.invalid_requirement/", "typestate.invalid_transition/", "type.operator.concat_operand/", "type.operator.comparison_operand/", "type.call.optional_receiver/"} {
		if name, ok := strings.CutPrefix(key, prefix); ok {
			if prefix == "type.operator.concat_operand/" {
				name, _, _ = strings.Cut(name, "/")
			}
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
func enrichDirectCallDiagnostic(item PublishedDiagnostic, operation equation.Equation, values []equation.Fact) PublishedDiagnostic {
	operands := make(map[string]string, len(operation.Operands))
	for _, operand := range operation.Operands {
		operands[operand.Role] = string(operand.Term.Encoding)
	}
	callee := operands["callee-display"]
	if callee == "" {
		callee = strings.TrimPrefix(operands["callee"], "path/")
	}
	if callee == "" && operands["receiver-display"] != "" {
		if method, ok := callMethodName([]byte(operands["method"])); ok {
			callee = operands["receiver-display"] + "." + method
		}
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
		return enrichCallArgumentDiagnostic(item, callee, subject, operands, values)
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
		if item.Message == fmt.Sprintf("cannot call %s because it may be nil", callee) {
			item.Evidence = []DiagnosticEvidence{
				{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has a callable type, but may also be nil", callee)},
				{Span: item.Span, Kind: "missing proof", Trust: "unknown", Reason: "boundary validation missing", Message: fmt.Sprintf("no guard on this path proves %s is non-nil before this call", callee)},
			}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "call target"}}
			item.Help = fmt.Sprintf("Guard `%s` with a nil check before calling it.", callee)
			return item
		}
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

// enrichConcatOperandDiagnostic renders a nilability warning from the closed
// operand fact and its WIR-provided source anchor. It does not inspect source
// or guess a value type after equation evaluation.
func enrichConcatOperandDiagnostic(item PublishedDiagnostic, operation equation.Equation, values []equation.Fact) PublishedDiagnostic {
	_, operationName, subject, ok := concatOperandDiagnosticParts(item.Fact.Key)
	if !ok {
		return item
	}
	index, ok := concatOperandIndex(subject)
	if !ok {
		return item
	}
	display := "value"
	for _, operand := range operation.Operands {
		if operand.Role == fmt.Sprintf("value-display-%08d", index) && len(operand.Term.Encoding) != 0 {
			display = string(operand.Term.Encoding)
			break
		}
	}
	side := "left"
	if index > 0 {
		side = "right"
	}
	item.Message = fmt.Sprintf("%s operand `%s` of `..` may be nil", side, display)
	item.Evidence = nil
	if origin, found := concatOperandOriginEvidence(operationName, index, display, values); found {
		item.Evidence = append(item.Evidence, DiagnosticEvidence{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: origin})
	}
	item.Evidence = append(item.Evidence,
		DiagnosticEvidence{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s operand `%s` has type nil", side, display)},
		DiagnosticEvidence{Span: item.Span, Kind: "missing proof", Trust: "unknown", Message: fmt.Sprintf("no guard on this path proves %s is non-nil", display)},
	)
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "value may be nil"}}
	item.Help = fmt.Sprintf("Guard `%s` or provide a default string before using `..`.", display)
	return item
}

// concatOperandOriginEvidence renders the closed origin classification that the
// refuting transaction published for this operand. Absent that fact the operand
// has no proven provenance beyond its own nil possibility.
func concatOperandOriginEvidence(operation string, index int, display string, values []equation.Fact) (string, bool) {
	key := fmt.Sprintf("%s%s/value-%08d", concatOperandOriginPrefix, operation, index)
	for _, fact := range values {
		if fact.Key != key {
			continue
		}
		classification := string(fact.Value)
		if classification == concatOriginOptionalField {
			return fmt.Sprintf("%s is an optional field and may be nil", display), true
		}
		if subject, isResult := strings.CutPrefix(classification, concatOriginOptionalResult); isResult && subject != "" {
			return fmt.Sprintf("%s has type nil and may be nil", subject), true
		}
	}
	return "", false
}

func concatOperandDiagnosticParts(key string) (code, operation, subject string, ok bool) {
	if inner, child := childDiagnosticKey(key); child {
		key = inner
	}
	const prefix = "type.operator.concat_operand/"
	if !strings.HasPrefix(key, prefix) {
		return "", "", "", false
	}
	operation, subject, ok = strings.Cut(strings.TrimPrefix(key, prefix), "/")
	return "concat_operand", operation, subject, ok && operation != "" && subject != ""
}

func concatOperandIndex(subject string) (int, bool) {
	encoded, ok := strings.CutPrefix(subject, "value-")
	if !ok || len(encoded) != 8 {
		return 0, false
	}
	index, err := strconv.Atoi(encoded)
	return index, err == nil && index >= 0
}

func enrichCallArgumentDiagnostic(item PublishedDiagnostic, callee, subject string, operands map[string]string, values []equation.Fact) PublishedDiagnostic {
	argumentIndex, suffix, ok := callArgumentSubject(subject)
	if !ok {
		return item
	}
	message := item.Message
	value, expected, nilable := "", "", false
	if before, after, found := strings.Cut(message, " may be nil, not "); found && before == fmt.Sprintf("argument %d%s", argumentIndex, suffix) && after != "" {
		value, expected, nilable = "may be nil", after, true
	} else {
		start := strings.Index(message, " is ")
		end := strings.LastIndex(message, ", not ")
		if start < 0 || end <= start+4 {
			return item
		}
		value, expected = message[start+4:end], message[end+6:]
	}
	argument := fmt.Sprintf("argument %d", argumentIndex) + suffix
	if display := operands[fmt.Sprintf("argument-display-%08d", argumentIndex-1)]; display != "" {
		argument += " (" + display + ")"
	}
	argumentTerm := []byte(operands[fmt.Sprintf("argument-%08d", argumentIndex-1)])
	if summaryTypeIsAny(argumentTerm, values) || sourceHasGradualLogicalBoundary(argumentTerm, values) {
		display := strings.TrimPrefix(argument, fmt.Sprintf("argument %d (", argumentIndex))
		display = strings.TrimSuffix(display, ")")
		if display == argument {
			display = argument
		}
		item.Message = fmt.Sprintf("%s comes from any/unknown; no proof shows it is %s", argument, expected)
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type any", argument)},
			{Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s parameter %d%s expects %s", callee, argumentIndex, suffix, expected)},
			{Span: item.Span, Kind: "unvalidated value", Trust: "unknown", Reason: "explicit boundary validation", Message: fmt.Sprintf("%s comes from any/unknown", display)},
			{Span: item.Span, Kind: "missing proof", Trust: "unknown", Reason: "boundary validation missing", Message: fmt.Sprintf("no proof on this path shows %s satisfies the parameter type", display)},
		}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "argument value any"}}
		item.Help = fmt.Sprintf("Validate or narrow `%s` before passing it; any/unknown values do not prove parameter contracts.", display)
		return item
	}
	if nilable {
		display := operands[fmt.Sprintf("argument-display-%08d", argumentIndex-1)]
		if display == "" {
			display = argument
		}
		item.Message = fmt.Sprintf("cannot pass %s as argument %d because it may be nil", display, argumentIndex)
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s can be %s or nil here", argument, expected)},
			{Kind: "user assertion", Trust: "claimed", Message: fmt.Sprintf("%s parameter %d expects %s", callee, argumentIndex, expected)},
			{Span: item.Span, Kind: "missing proof", Trust: "refuted", Reason: "boundary validation missing", Message: fmt.Sprintf("no guard on this path proves %s is non-nil", display)},
		}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "argument value"}}
		item.Help = fmt.Sprintf("Guard `%s` with a nil check, provide a default argument value, or change the parameter type to accept nil.", display)
		return item
	}
	item.Message = fmt.Sprintf("%s is %s, not %s", argument, value, expected)
	valueFact := fmt.Sprintf("%s has type %s", argument, value)
	if callDiagnosticValueIsLiteral(value) {
		valueFact = fmt.Sprintf("%s has literal value %s", argument, value)
	}
	parameter := fmt.Sprintf("%s parameter %d", callee, argumentIndex) + suffix
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
	index, _, ok := callArgumentSubject(subject)
	return index, ok
}

func callArgumentSubject(subject string) (int, string, bool) {
	encoded, suffix, found := strings.Cut(subject, ".")
	if !found {
		encoded = subject
	}
	encoded, ok := strings.CutPrefix(encoded, "argument-")
	if !ok || len(encoded) != 8 {
		return 0, "", false
	}
	index, err := strconv.Atoi(encoded)
	if err != nil || index < 0 {
		return 0, "", false
	}
	if found {
		suffix = "." + suffix
	}
	return index + 1, suffix, true
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
	case strings.HasPrefix(key, "type.nil.unsafe_use/"):
		return "type.nil.unsafe_use"
	case strings.HasPrefix(key, "type.assignment/"):
		return "type.assignment"
	case strings.HasPrefix(key, "type.assignment.optional_target/"):
		return "type.assignment.optional_target"
	case strings.HasPrefix(key, "type.return.contract/"):
		return "type.return.contract"
	case strings.HasPrefix(key, "type.call.optional_receiver/"):
		return "type.call.optional_receiver"
	case strings.HasPrefix(key, "type.member.missing/"):
		return "type.member.missing"
	case strings.HasPrefix(key, "type.operator.concat_operand/"):
		return "type.operator.concat_operand"
	case strings.HasPrefix(key, "type.operator.comparison_operand/"):
		return "type.operator.comparison_operand"
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

func enrichConstantGuardDiagnostic(item PublishedDiagnostic, operation equation.Equation, key string, artifact equation.Artifact, branchSpans map[string]wir.Span) PublishedDiagnostic {
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
		current, currentOK := branchPredicateDescription(operation)
		prior, priorSpan, priorOK := enclosingBranchProof(operation, artifact, branchSpans)
		if !currentOK || !priorOK {
			item.Evidence = []DiagnosticEvidence{{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "condition is proven constant under its enclosing guard"}}
			item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "constant guard"}}
			return item
		}
		item.Evidence = []DiagnosticEvidence{
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: "current check: " + current},
			{Span: priorSpan, Kind: "abstract fact", Trust: "proven", Message: "prior guard established " + prior},
			{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: strings.Split(current, " ")[0] + " is unchanged between the prior guard and this check"},
		}
		item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "current check"}, {Span: priorSpan, Message: "prior guard"}}
	}
	return item
}

func branchPredicateDescription(operation equation.Equation) (string, bool) {
	var predicate branchPredicateWire
	display := ""
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "predicate":
			if !strings.HasPrefix(string(operand.Term.Encoding), branchPredicatePrefix) || json.Unmarshal(operand.Term.Encoding[len(branchPredicatePrefix):], &predicate) != nil {
				return "", false
			}
		case "predicate-display":
			display = string(operand.Term.Encoding)
		}
	}
	if display == "" {
		return "", false
	}
	switch predicate.Kind {
	case "truthy":
		return display + " is checked as truthy", true
	case "falsy":
		return display + " is checked as falsy", true
	case "literal-equal":
		literal, err := displayValue([]byte(predicate.Literal))
		if predicate.Literal == "" || err != nil {
			return "", false
		}
		return display + " equals " + string(literal), true
	case "literal-not":
		literal, err := displayValue([]byte(predicate.Literal))
		if predicate.Literal == "" || err != nil {
			return "", false
		}
		return display + " does not equal " + string(literal), true
	default:
		return "", false
	}
}

func enclosingBranchProof(operation equation.Equation, artifact equation.Artifact, spans map[string]wir.Span) (string, wir.Span, bool) {
	for _, guard := range operation.Guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || parts[3] != "true" {
			continue
		}
		prior, found := branchOperation(artifact, parts[2])
		if !found {
			continue
		}
		description, ok := branchPredicateDescription(prior)
		if !ok {
			continue
		}
		if left, right, found := strings.Cut(description, " equals "); found {
			return left + " is " + right, spans[parts[2]], true
		}
		if strings.HasSuffix(description, " is checked as truthy") {
			return strings.TrimSuffix(description, " is checked as truthy") + " is truthy", spans[parts[2]], true
		}
		if strings.HasSuffix(description, " is checked as falsy") {
			return strings.TrimSuffix(description, " is checked as falsy") + " is falsy", spans[parts[2]], true
		}
	}
	return "", wir.Span{}, false
}

func assignmentEvidenceValue(value []byte) string {
	display, err := displayValue(value)
	if err == nil {
		return string(display)
	}
	return assignmentValueType(value)
}

// boundaryShapeEvidenceValue formats a closed explicit-any boundary's already
// published literal shape solely as a counterexample. It is never used as a
// compatibility proof: incomplete or non-literal shapes use the ordinary
// abstract-value rendering.
func boundaryShapeEvidenceValue(value []byte) string {
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed || len(table.Members) == 0 {
		return assignmentEvidenceValue(value)
	}
	type entry struct {
		segment segment.Segment
		value   string
	}
	entries := make([]entry, 0, len(table.Members))
	for _, member := range table.Members {
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid || len(segments) != 1 || !member.Present {
			return assignmentEvidenceValue(value)
		}
		display, err := displayValue([]byte(member.Value))
		if err != nil {
			return assignmentEvidenceValue(value)
		}
		entries = append(entries, entry{segment: segments[0], value: string(display)})
	}
	array := true
	for index, item := range entries {
		if item.segment.Kind != segment.SegmentIndexInt || item.segment.Index != index+1 {
			array = false
			break
		}
	}
	if array {
		values := make([]string, len(entries))
		for index, item := range entries {
			values[index] = item.value
		}
		return "(" + strings.Join(values, ", ") + ")"
	}
	fields := make([]string, 0, len(entries))
	for _, item := range entries {
		var key string
		switch item.segment.Kind {
		case segment.SegmentField:
			key = item.segment.Name
		case segment.SegmentIndexString:
			key = strconv.Quote(item.segment.Name)
		default:
			return assignmentEvidenceValue(value)
		}
		fields = append(fields, key+": "+item.value)
	}
	return "{" + strings.Join(fields, ", ") + "}"
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

type kernelBindingSpec struct {
	kind   string
	kernel equation.Kernel
}

func kernelBindingSpecs(lexical *lexicalEvaluator, imported map[string]bool) []kernelBindingSpec {
	return []kernelBindingSpec{
		{"entry", equation.KernelFunc(entryKernel)}, {"allocation-template", equation.KernelFunc(allocationTemplateKernel)},
		{"object-materialization", equation.KernelFunc(func(o equation.BoundEquation, p equation.Partition) (equation.TransactionResult, error) {
			return objectMaterializationKernel(lexical, o, p)
		})},
		{"environment-write", equation.KernelFunc(func(o equation.BoundEquation, p equation.Partition) (equation.TransactionResult, error) {
			return writeKernel(lexical, o, p)
		})},
		{"path-replacement", equation.KernelFunc(pathReplacementKernel)}, {"dynamic-index-read", equation.KernelFunc(dynamicIndexReadKernel)}, {"path-invalidation", equation.KernelFunc(pathInvalidationKernel)}, {"index-mutation", equation.KernelFunc(indexMutationKernel)},
		{"branch-relations", equation.KernelFunc(branchKernel)}, {"apply", equation.KernelFunc(func(o equation.BoundEquation, p equation.Partition) (equation.TransactionResult, error) {
			return applyKernel(lexical, o, p)
		})},
		{"external-call", equation.KernelFunc(func(o equation.BoundEquation, p equation.Partition) (equation.TransactionResult, error) {
			return externalCallKernel(lexical, o, p)
		})}, {"call-results", equation.KernelFunc(func(o equation.BoundEquation, p equation.Partition) (equation.TransactionResult, error) {
			return callResultsKernel(lexical, o, p)
		})},
		{"generic-for", equation.KernelFunc(genericForKernel)}, {"channel-select", equation.KernelFunc(channelSelectKernel)}, {"publication", equation.KernelFunc(publicationKernel)},
		{"claim", equation.KernelFunc(func(o equation.BoundEquation, p equation.Partition) (equation.TransactionResult, error) {
			return claimKernel(lexical, o, p, imported)
		})}, {"expression", equation.KernelFunc(expressionKernel)}, {"eval-node", equation.KernelFunc(evalNodeKernel)},
	}
}

func kernelIDs(kind string) (string, equation.ContentID, error) {
	kernelID, known := front.KernelID(kind)
	contract, contracted := front.ContractID(kind)
	if !known || !contracted {
		return "", equation.ContentID{}, fmt.Errorf("engine: missing front kernel contract for %q", kind)
	}
	return kernelID, contract, nil
}

func registry(lexical *lexicalEvaluator, imported map[string]bool) (*equation.KernelRegistry, error) {
	specs := kernelBindingSpecs(lexical, imported)
	bindings := make([]equation.KernelBinding, 0, len(specs))
	for _, spec := range specs {
		kernelID, contract, err := kernelIDs(spec.kind)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, equation.KernelBinding{KernelID: kernelID, ContractID: contract, Kernel: spec.kernel})
	}
	result, err := equation.NewKernelRegistry(bindings)
	if err != nil {
		return nil, fmt.Errorf("engine: build kernel registry: %w", err)
	}
	return result, nil
}

func cyclicRegistry(lexical *lexicalEvaluator, imported map[string]bool) (*equation.CyclicKernelRegistry, error) {
	specs := kernelBindingSpecs(lexical, imported)
	bindings := make([]equation.CyclicKernelBinding, 0, len(specs))
	for _, spec := range specs {
		kernelID, contract, err := kernelIDs(spec.kind)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, equation.CyclicKernelBinding{KernelID: kernelID, ContractID: contract, Kernel: cyclicKernel(spec.kernel)})
	}
	result, err := equation.NewCyclicKernelRegistry(bindings)
	if err != nil {
		return nil, fmt.Errorf("engine: build cyclic kernel registry: %w", err)
	}
	return result, nil
}

func cyclicKernel(kernel equation.Kernel) equation.CyclicKernel {
	return equation.CyclicKernelFunc(func(ctx context.Context, operation equation.BoundCyclicEquation, snapshot equation.CyclicSnapshot) (equation.TransactionResult, error) {
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
		partition, err := equation.PartitionFromClosuresWithGuards(nil, closures...)
		if err != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: cyclic snapshot partition: %w", err)
		}
		if guards, exact := typedLiteralPartitionGuards(operation.Equation.Guards, closures); exact {
			candidate, candidateErr := equation.PartitionFromClosuresWithGuards(guards, closures...)
			if candidateErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: cyclic guarded snapshot partition: %w", candidateErr)
			}
			if cyclicOptionalClaim(operation.Equation, candidate) {
				partition = candidate
			}
		}
		return kernel.Execute(operation.Equation, partition)
	})
}

// typedLiteralPartitionGuards admits a cyclic branch view only after every
// requested guard is backed by the matching literal-discriminant narrowing.
// A branch fact from another predicate may coexist in the snapshot, but it
// cannot make an arm-specific value visible to this consumer.
func typedLiteralPartitionGuards(guards []equation.Guard, closures []equation.OutputClosure) ([]equation.Guard, bool) {
	if len(guards) == 0 {
		return nil, false
	}
	active := make([]equation.Guard, 0, len(guards))
	for _, guard := range guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || (parts[3] != "true" && parts[3] != "false") {
			return nil, false
		}
		key, value := "narrowing/"+parts[2], "typed/"+parts[3]
		found := false
		for _, closure := range closures {
			for _, fact := range closure.Outcomes {
				if fact.Key != key || string(fact.Value) != value {
					continue
				}
				for _, factGuard := range fact.Guards {
					if factGuard.Body == guard.Body && bytes.Equal(factGuard.Encoding, guard.Encoding) {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return nil, false
		}
		active = append(active, guard)
	}
	return active, true
}

// cyclicOptionalClaim admits only a positive branch-local optional assignment
// into the cyclic snapshot. The claim already owns the source and declared
// target; calls, expressions, and non-optional writes remain unavailable until
// the cyclic evaluator can model their recurrence without borrowing an arm.
func cyclicOptionalClaim(operation equation.BoundEquation, partition equation.Partition) bool {
	if operation.Occurrence.Kind != "claim" || len(operation.Guards) == 0 {
		return false
	}
	for _, guard := range operation.Guards {
		if !strings.HasSuffix(string(guard.Encoding), "/true") {
			return false
		}
	}
	operands, err := requiredOperandsByRole(operation.Operands, "value", "type")
	if err != nil {
		return false
	}
	value, available, err := resolveClaimValue(operands["value"], partition)
	if err != nil || !available {
		return false
	}
	source, decoded := shapefact.DecodeTarget(value)
	if !decoded || source == nil || !proof.OptionalTypeHasConcreteValue(source) {
		return false
	}
	target, err := strconv.Unquote(strings.TrimPrefix(string(operands["type"]), "claim-type/"))
	return err == nil && target != "" && target != "any" && target != "nil" && !strings.HasSuffix(target, "?")
}

// childEntryWire is deliberately a closed entry payload.  It is decoded only
// by the entry transaction and becomes ordinary body-local seed facts; no
// caller partition is ever shared with a child evaluator.
type childEntryWire struct {
	Version            uint8                    `json:"version"`
	Seeds              []entrySeed              `json:"seeds"`
	ClosureSeeds       []entryClosureSeed       `json:"closure_seeds,omitempty"`
	MemberClosureSeeds []entryMemberClosureSeed `json:"member_closure_seeds,omitempty"`
	TableIdentitySeeds []entryTableIdentitySeed `json:"table_identity_seeds,omitempty"`
	MemberCellSeeds    []entryMemberCellSeed    `json:"member_cell_seeds,omitempty"`
	PlacementSeeds     []entryPlacementSeed     `json:"placement_seeds,omitempty"`
	GradualAnyTerms    []string                 `json:"gradual_any_terms,omitempty"`
	DeclaredBoundary   bool                     `json:"declared_boundary,omitempty"`
}

type entrySeed struct {
	Term  string `json:"term"`
	Value []byte `json:"value"`
}

// entryClosureSeed carries an already-published lexical capability across a
// private child entry. It is deliberately separate from a value seed: an
// ordinary scalar/function proof does not authorize local-body demand.
type entryClosureSeed struct {
	Term   string        `json:"term"`
	Handle closureHandle `json:"handle"`
}

// entryMemberClosureSeed carries a callable-member capability only with the
// exact sealed receiver value that published it.  It is a distinct transport
// lane because a table shape proves member presence/value but cannot authorize
// evaluation of a local member body by itself.
type entryMemberClosureSeed struct {
	Term string            `json:"term"`
	Wire memberClosureWire `json:"wire"`
}

// entryMemberCellSeed is a versioned heap-member publication carried across a
// private lexical entry.  The identity, rather than a source path, owns the
// cell: aliases therefore observe the same newest write and a replacement
// cannot revive a closure capability from an older path spelling.
type entryMemberCellSeed struct {
	Identity []byte         `json:"identity"`
	Suffix   string         `json:"suffix"`
	Wire     memberCellWire `json:"wire"`
}

type entryTableIdentitySeed struct {
	Term     string `json:"term"`
	Identity []byte `json:"identity"`
}

// entryPlacementSeed is an exact caller allocation lens for a child formal.
// It is accepted only with the corresponding value seed and carries the
// existing allocation fact unchanged; children therefore publish observed
// local-call boundaries against the caller's identity instead of fabricating
// an alias from a parameter name.
type entryPlacementSeed struct {
	Term       string                  `json:"term"`
	Allocation placementAllocationFact `json:"allocation"`
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

// memberCellWire couples the runtime member value, its optional child-body
// capability, and an optional child-table identity in one write-owned cell.
// A missing Handle is intentional: callable proof never arises from a table
// shape alone.
type memberCellWire struct {
	Value          []byte         `json:"value"`
	Handle         *closureHandle `json:"handle,omitempty"`
	MemberIdentity []byte         `json:"member_identity,omitempty"`
}

type lexicalEvaluator struct {
	byPrototype       map[string]front.Compilation
	byBody            map[equation.BodyID]front.Compilation
	requiresBody      map[string]bool
	diagnosticSpans   map[string]wir.Span
	callSpans         map[string]wir.Span
	lifecycleEvidence map[string][]DiagnosticEvidence
	selectEvidence    map[string][]DiagnosticEvidence
	childPublished    map[string]PublishedDiagnostic
	ctx               context.Context
	table             *interproc.ProjectedTable
	coordinator       *interproc.RecursionCoordinator
	admissions        map[string]lexicalSCCAdmission
	// captureWrites holds, per prototype, the captured boundary cells whose
	// members that body statically writes. It is a compilation property, so it
	// is computed once with the body catalog rather than at every application.
	captureWrites map[string]map[string]bool
	run           *lexicalSCCRun
	// Imported authority is selected at the project boundary. It is scoped to
	// this evaluator and exists only for exact result paths published by those
	// imports; no structural reconstruction is admitted as an authority.
	importedTypes       map[string]typ.Type
	importedRelations   map[string]exportrelation.Summary
	globalTypes         map[string]typ.Type
	importedAuthorities map[string]typ.Type
	typeOrigins         map[string]typ.Type
	typeDefinitions     map[string]typ.Type
	typeFieldSpans      map[string]map[string]wir.Span
	// sourcePath is the project-supplied file name for this compilation unit.
	// It is presentation identity only: no fact, type, or proof depends on it.
	sourcePath          string
	importedAuthorityMu sync.RWMutex
}

func (l *lexicalEvaluator) setImportedTypes(types map[string]typ.Type) {
	if l == nil || len(types) == 0 {
		return
	}
	l.importedAuthorityMu.Lock()
	defer l.importedAuthorityMu.Unlock()
	l.importedTypes = make(map[string]typ.Type, len(types))
	for path, value := range types {
		if path != "" && value != nil {
			l.importedTypes[path] = value
		}
	}
}

func (l *lexicalEvaluator) setImportedRelations(relations map[string]exportrelation.Summary) {
	if l == nil || len(relations) == 0 {
		return
	}
	l.importedAuthorityMu.Lock()
	defer l.importedAuthorityMu.Unlock()
	l.importedRelations = make(map[string]exportrelation.Summary, len(relations))
	for path, summary := range relations {
		if path != "" && summary.Type != nil {
			l.importedRelations[path] = summary
		}
	}
}

func (l *lexicalEvaluator) setGlobalTypes(types map[string]typ.Type) {
	if l == nil || len(types) == 0 {
		return
	}
	l.importedAuthorityMu.Lock()
	defer l.importedAuthorityMu.Unlock()
	l.globalTypes = make(map[string]typ.Type, len(types))
	for name, value := range types {
		if name != "" && value != nil {
			l.globalTypes[name] = value
		}
	}
}

func (l *lexicalEvaluator) setTypeOrigins(root front.Compilation) {
	if l == nil {
		return
	}
	origins := make(map[string]typ.Type)
	var collect func(front.Compilation)
	collect = func(compilation front.Compilation) {
		if compilation.WIR != nil {
			compilation.WIR.ForEachType(func(value typ.Type) bool {
				if encoded, ok := shapefact.EncodeTarget(value); ok {
					origins[string(encoded)] = value
				}
				return true
			})
		}
		for _, child := range compilation.Nested {
			collect(child)
		}
	}
	collect(root)
	if len(origins) == 0 {
		return
	}
	l.importedAuthorityMu.Lock()
	defer l.importedAuthorityMu.Unlock()
	l.typeOrigins = origins
}

func (l *lexicalEvaluator) typeOrigin(encoded []byte) (typ.Type, bool) {
	if l == nil || len(encoded) == 0 {
		return nil, false
	}
	l.importedAuthorityMu.RLock()
	defer l.importedAuthorityMu.RUnlock()
	value, ok := l.typeOrigins[string(encoded)]
	return value, ok
}

func (l *lexicalEvaluator) importedType(path string) (typ.Type, bool) {
	if l == nil || path == "" {
		return nil, false
	}
	l.importedAuthorityMu.RLock()
	defer l.importedAuthorityMu.RUnlock()
	value, ok := l.importedTypes[path]
	return value, ok
}

func (l *lexicalEvaluator) globalType(name string) (typ.Type, bool) {
	if l == nil || name == "" {
		return nil, false
	}
	l.importedAuthorityMu.RLock()
	defer l.importedAuthorityMu.RUnlock()
	value, ok := l.globalTypes[name]
	return value, ok
}

func (l *lexicalEvaluator) setImportedAuthority(term string, value typ.Type) {
	if l == nil || term == "" || value == nil {
		return
	}
	l.importedAuthorityMu.Lock()
	defer l.importedAuthorityMu.Unlock()
	if l.importedAuthorities == nil {
		l.importedAuthorities = make(map[string]typ.Type)
	}
	l.importedAuthorities[term] = value
}

func (l *lexicalEvaluator) importedAuthority(term string) (typ.Type, bool) {
	if l == nil || term == "" {
		return nil, false
	}
	l.importedAuthorityMu.RLock()
	defer l.importedAuthorityMu.RUnlock()
	value, ok := l.importedAuthorities[term]
	return value, ok
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

// hasRuntimeCastClaim distinguishes a checked cast from an ordinary
// annotation claim. A cast's result may carry a nilability witness into the
// child's declared return, so an exact invoked child must be evaluated for
// that caller-owned contract. Ordinary claims retain the existing deferred
// body policy.
func (l *lexicalEvaluator) hasRuntimeCastClaim(prototype string) bool {
	child, exists := l.byPrototype[prototype]
	if !exists {
		return false
	}
	for _, item := range child.Artifact.Equations {
		if item.Occurrence.Kind != "claim" {
			continue
		}
		for _, operand := range item.Operands {
			if operand.Role == "kind" && string(operand.Term.Encoding) == "claim-kind/1" {
				return true
			}
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

// hasProjectableTableResult identifies the narrow result boundary where an
// already-evaluated child allocation may cross to its caller. An inferred
// return has no declared tuple contract, but applyKnown still projects only
// the child body's actual returned slots; it is therefore no less closed than
// a one-slot declaration. Declared multi-return tuples retain Lua expansion
// semantics of their own, and recursive return graphs need the cyclic summary
// path; neither can reuse this finite allocation projection.
func hasProjectableTableResult(child front.Compilation) bool {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.DeclaredReturns) > 1 {
		return false
	}
	if len(child.Boundary.DeclaredReturns) == 0 {
		return true
	}
	return !typ.ContainsRecursive(child.WIR.Type(child.Boundary.DeclaredReturns[0]))
}

func newLexicalEvaluator(root front.Compilation) *lexicalEvaluator {
	table := interproc.NewProjectedTable()
	l := &lexicalEvaluator{byPrototype: make(map[string]front.Compilation), byBody: make(map[equation.BodyID]front.Compilation), requiresBody: make(map[string]bool), diagnosticSpans: make(map[string]wir.Span), callSpans: make(map[string]wir.Span), lifecycleEvidence: make(map[string][]DiagnosticEvidence), selectEvidence: make(map[string][]DiagnosticEvidence), childPublished: make(map[string]PublishedDiagnostic), ctx: context.Background(), table: table, coordinator: interproc.NewRecursionCoordinator(table, 256), admissions: make(map[string]lexicalSCCAdmission), captureWrites: make(map[string]map[string]bool), typeDefinitions: cloneTypeDefinitions(root.TypeDefinitions), typeFieldSpans: root.TypeFieldSpans}
	var add func(front.Compilation)
	add = func(compilation front.Compilation) {
		l.byBody[compilation.Body] = compilation
		if compilation.PrototypeName != "" {
			l.byPrototype[compilation.PrototypeName] = compilation
		}
		for name, span := range compilation.CallSpans {
			l.callSpans[lexicalSpanKey(compilation.Body, name)] = span
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
		diagnosticRelay := compilationRequiresDiagnosticPublication(compilation)
		captureWrites := capturedMemberWriteTerms(compilation)
		required := len(compilation.Boundary.Captures) != 0 || diagnosticRelay
		// A body that statically writes a member of one of its own captures owns
		// an effect the caller can observe. Its writeback is exactly the capture
		// lens this evaluator already transports, so it keeps body admission
		// rather than degrading to the fail-closed revocation.
		if lexicalRequiresHeapTransport(compilation) && !compilation.RebindsBoundary && !diagnosticRelay && len(captureWrites) == 0 {
			required = false
		}
		for _, child := range compilation.Nested {
			required = mark(child) || required
		}
		if compilation.PrototypeName != "" {
			if len(captureWrites) != 0 {
				l.captureWrites[compilation.PrototypeName] = captureWrites
			}
			l.requiresBody[compilation.PrototypeName] = required
		}
		return required
	}
	mark(root)
	return l
}

// compilationRequiresDiagnosticPublication admits a lexical body when one of
// its equation-owned diagnostic templates needs the body's closed entry facts.
// This is a template property, not a source scan: the child remains dormant
// unless its local capability is actually applied by the enclosing closure.
func compilationRequiresDiagnosticPublication(compilation front.Compilation) bool {
	if forwardedStaticMemberContractBoundary(compilation) {
		return true
	}
	// A no-result, static member relay has a caller-owned diagnostic consumer:
	// the exact enclosing application. Its child cannot publish a result or a
	// heap write, but a member closure already transported with the complete
	// formal entry may refute its argument contract. Admit only this closed
	// relay shape; arbitrary calls, branches, writes, and dynamic lookup remain
	// demand-driven.
	memberRelay := false
	for _, operation := range compilation.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry", "external-call", "call-results":
			continue
		case "expression":
			for _, operand := range operation.Operands {
				if operand.Role == "kind" && string(operand.Term.Encoding) == strconv.Itoa(int(wir.OpConcat)) {
					return true
				}
			}
			if declaredOrderedComparisonExpression(operation) {
				return true
			}
			return false
		case "apply":
			callee, hasCallee := artifactOperand(operation.Operands, "callee")
			resultArity, hasResultArity := artifactOperand(operation.Operands, "result-arity")
			if !hasCallee || !hasResultArity || string(resultArity) != "0" {
				return false
			}
			member := strings.LastIndex(string(callee), ".")
			if !strings.HasPrefix(string(callee), "path/") || member <= len("path/") {
				return false
			}
			memberRelay = true
		default:
			return false
		}
	}
	return memberRelay
}

// forwardedStaticMemberContractBoundary identifies an invoked helper whose
// only effectful use of one formal is forwarding it to a static member of
// another formal. Admission carries only the caller's completed entry facts to
// the ordinary call-contract kernel; it does not infer a contract from source.
func forwardedStaticMemberContractBoundary(compilation front.Compilation) bool {
	if compilation.WIR == nil || compilation.Cyclic != nil || len(compilation.Boundary.Parameters) == 0 {
		return false
	}
	formals := make(map[string]bool, len(compilation.Boundary.Parameters))
	for _, parameter := range compilation.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 {
			return false
		}
		formals[boundaryTerm(parameter.Symbol)] = true
	}
	found := false
	for _, operation := range compilation.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry", "allocation-template", "object-materialization", "external-call", "call-results", "publication":
			continue
		case "environment-write":
			target, present := artifactOperand(operation.Operands, "target")
			if !present {
				return false
			}
			for formal := range formals {
				if string(target) == formal || strings.HasPrefix(string(target), formal+".") || strings.HasPrefix(string(target), formal+"[") {
					return false
				}
			}
		case "apply":
			if found || len(operation.Guards) != 0 {
				return false
			}
			callee, present := artifactOperand(operation.Operands, "callee")
			if !present {
				return false
			}
			root, suffix, member := tableAddress(callee)
			segments, static := segment.ParseFormattedSegments(suffix)
			if !member || !formals[string(root)] || !static || len(segments) != 1 || segments[0].Kind != segment.SegmentField {
				return false
			}
			forwardsFormal := false
			for _, operand := range operation.Operands {
				if strings.HasPrefix(operand.Role, "argument-") && !strings.HasPrefix(operand.Role, "argument-display-") {
					forwardsFormal = forwardsFormal || formals[string(operand.Term.Encoding)]
				}
			}
			if !forwardsFormal {
				return false
			}
			found = true
		default:
			return false
		}
	}
	return found
}

// lexicalRequiresHeapTransport identifies child effects the value-only bridge
// cannot preserve. Those bodies stay on the established root path unless they
// also rebind a boundary cell, which is the one effect this bridge transports.
// capturedMemberWriteTerms lists the captured boundary cells whose members a
// body statically writes. The caller holds an allocation-time closed shape for
// those cells; once this body runs, that shape can no longer prove any member
// absent. Detection is keyed to the exact captured boundary term, so writes
// through parameters, temporaries, and dynamic keys are not part of the relation.
func capturedMemberWriteTerms(compilation front.Compilation) map[string]bool {
	if len(compilation.Boundary.Captures) == 0 {
		return nil
	}
	captured := make(map[string]bool, len(compilation.Boundary.Captures))
	for _, capture := range compilation.Boundary.Captures {
		captured[boundaryTerm(capture.Symbol)] = true
	}
	var written map[string]bool
	for _, item := range compilation.Artifact.Equations {
		if item.Occurrence.Kind != "path-replacement" && item.Occurrence.Kind != "environment-write" {
			continue
		}
		target, found := artifactOperand(item.Operands, "target")
		if !found {
			continue
		}
		root, suffix, ok := heapTableAddress(target)
		if !ok || suffix == "" || !captured[string(root)] {
			continue
		}
		if written == nil {
			written = make(map[string]bool, len(captured))
		}
		written[string(root)] = true
	}
	return written
}

// capturedMemberWriteInvalidations revokes the caller's closed-table proof for
// every capture the callee statically writes a member of. It is the fail-closed
// transport for a body this application does not evaluate: the written member is
// never invented, but the table stops proving that any member is absent.
func (l *lexicalEvaluator) capturedMemberWriteInvalidations(handle closureHandle, operation string, partition equation.Partition) []equation.Fact {
	if l == nil {
		return nil
	}
	written := l.captureWrites[handle.Prototype]
	if len(written) == 0 {
		return nil
	}
	child, known := l.byPrototype[handle.Prototype]
	if !known || len(child.Boundary.Captures) != len(handle.Captures) {
		return nil
	}
	var facts []equation.Fact
	for index, capture := range child.Boundary.Captures {
		if !written[boundaryTerm(capture.Symbol)] {
			continue
		}
		callerTerm := handle.Captures[index]
		value, resolved := resolveKnownCurrentValue([]byte(callerTerm), partition)
		if !resolved {
			continue
		}
		table, sealed := shapefact.DecodeTable(value)
		if !sealed || !table.Closed {
			continue
		}
		table.Closed = false
		opened, encoded := shapefact.EncodeTable(table)
		if !encoded {
			continue
		}
		facts = append(facts,
			equation.Fact{Key: "value/" + callerTerm + "/" + operation, Value: opened},
			equation.Fact{Key: epochFactPrefix + callerTerm + "/" + operation, Value: []byte(operation)},
		)
	}
	return facts
}

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
		kernelRegistry, err := registry(l, importedResultPaths(compilation.Artifact))
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
	kernelRegistry, err := cyclicRegistry(l, importedResultPaths(compilation.Artifact))
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

func encodeChildEntry(seeds []entrySeed, closureSeeds ...entryClosureSeed) ([]byte, error) {
	return encodeChildEntryWithCapabilities(seeds, closureSeeds, nil, nil, nil)
}

// encodeDeclaredChildEntry marks the private entry used solely for a
// declaration-owned boundary. Kernels may consume that marker only together
// with a separately published contract fact; the marker itself is neither a
// value nor a type witness.
func encodeDeclaredChildEntry(seeds []entrySeed) ([]byte, error) {
	wire := childEntryWire{Version: 5, Seeds: append([]entrySeed(nil), seeds...), DeclaredBoundary: true}
	return encodeChildEntryWire(wire)
}

func encodeDeclaredChildEntryWithCapabilities(seeds []entrySeed, closureSeeds []entryClosureSeed, memberClosureSeeds []entryMemberClosureSeed, tableIdentitySeeds []entryTableIdentitySeed, memberCellSeeds []entryMemberCellSeed) ([]byte, error) {
	wire := childEntryWire{Version: 5, Seeds: append([]entrySeed(nil), seeds...), ClosureSeeds: append([]entryClosureSeed(nil), closureSeeds...), MemberClosureSeeds: append([]entryMemberClosureSeed(nil), memberClosureSeeds...), TableIdentitySeeds: append([]entryTableIdentitySeed(nil), tableIdentitySeeds...), MemberCellSeeds: append([]entryMemberCellSeed(nil), memberCellSeeds...), DeclaredBoundary: true}
	return encodeChildEntryWire(wire)
}

func encodeChildEntryWithCapabilities(seeds []entrySeed, closureSeeds []entryClosureSeed, memberClosureSeeds []entryMemberClosureSeed, tableIdentitySeeds []entryTableIdentitySeed, memberCellSeeds []entryMemberCellSeed, gradualAnyTerms ...[]string) ([]byte, error) {
	var terms []string
	for _, supplied := range gradualAnyTerms {
		terms = append(terms, supplied...)
	}
	return encodeChildEntryWire(childEntryWire{Version: 4, Seeds: append([]entrySeed(nil), seeds...), ClosureSeeds: append([]entryClosureSeed(nil), closureSeeds...), MemberClosureSeeds: append([]entryMemberClosureSeed(nil), memberClosureSeeds...), TableIdentitySeeds: append([]entryTableIdentitySeed(nil), tableIdentitySeeds...), MemberCellSeeds: append([]entryMemberCellSeed(nil), memberCellSeeds...), GradualAnyTerms: terms})
}

func encodeChildEntryWithPlacementCapabilities(seeds []entrySeed, closureSeeds []entryClosureSeed, memberClosureSeeds []entryMemberClosureSeed, tableIdentitySeeds []entryTableIdentitySeed, memberCellSeeds []entryMemberCellSeed, placementSeeds []entryPlacementSeed, gradualAnyTerms ...[]string) ([]byte, error) {
	var terms []string
	for _, supplied := range gradualAnyTerms {
		terms = append(terms, supplied...)
	}
	return encodeChildEntryWire(childEntryWire{Version: 6, Seeds: append([]entrySeed(nil), seeds...), ClosureSeeds: append([]entryClosureSeed(nil), closureSeeds...), MemberClosureSeeds: append([]entryMemberClosureSeed(nil), memberClosureSeeds...), TableIdentitySeeds: append([]entryTableIdentitySeed(nil), tableIdentitySeeds...), MemberCellSeeds: append([]entryMemberCellSeed(nil), memberCellSeeds...), PlacementSeeds: append([]entryPlacementSeed(nil), placementSeeds...), GradualAnyTerms: terms})
}

func encodeChildEntryWire(wire childEntryWire) ([]byte, error) {
	if err := validateChildEntryWire(&wire); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("engine: encode child entry: %w", err)
	}
	return append([]byte(fmt.Sprintf("front/child-entry/v%d/", wire.Version)), encoded...), nil
}

func validateChildEntryWire(wire *childEntryWire) error {
	if wire.Version < 1 || wire.Version > 6 || (wire.DeclaredBoundary && wire.Version != 5) {
		return fmt.Errorf("engine: malformed child entry version")
	}
	sort.Slice(wire.Seeds, func(i, j int) bool { return wire.Seeds[i].Term < wire.Seeds[j].Term })
	for index := range wire.Seeds {
		if !validEntrySeed(wire.Seeds[index]) || (index > 0 && wire.Seeds[index-1].Term == wire.Seeds[index].Term) {
			return fmt.Errorf("engine: malformed child entry seed")
		}
	}
	sort.Slice(wire.ClosureSeeds, func(i, j int) bool { return wire.ClosureSeeds[i].Term < wire.ClosureSeeds[j].Term })
	for index, seed := range wire.ClosureSeeds {
		if seed.Term == "" || !validClosureHandle(seed.Handle) || (index > 0 && wire.ClosureSeeds[index-1].Term == seed.Term) {
			return fmt.Errorf("engine: malformed child entry closure seed")
		}
	}
	sort.Slice(wire.MemberClosureSeeds, func(i, j int) bool {
		if wire.MemberClosureSeeds[i].Term != wire.MemberClosureSeeds[j].Term {
			return wire.MemberClosureSeeds[i].Term < wire.MemberClosureSeeds[j].Term
		}
		return wire.MemberClosureSeeds[i].Wire.Suffix < wire.MemberClosureSeeds[j].Wire.Suffix
	})
	for index, seed := range wire.MemberClosureSeeds {
		if seed.Term == "" || seed.Wire.Suffix == "" || !validClosureHandle(seed.Wire.Handle) ||
			(index > 0 && wire.MemberClosureSeeds[index-1].Term == seed.Term && wire.MemberClosureSeeds[index-1].Wire.Suffix == seed.Wire.Suffix) {
			return fmt.Errorf("engine: malformed child entry member closure seed")
		}
	}
	sort.Slice(wire.TableIdentitySeeds, func(i, j int) bool { return wire.TableIdentitySeeds[i].Term < wire.TableIdentitySeeds[j].Term })
	for index, seed := range wire.TableIdentitySeeds {
		if seed.Term == "" || len(seed.Identity) == 0 || (index > 0 && wire.TableIdentitySeeds[index-1].Term == seed.Term) {
			return fmt.Errorf("engine: malformed child entry table identity seed")
		}
	}
	sort.Slice(wire.PlacementSeeds, func(i, j int) bool { return wire.PlacementSeeds[i].Term < wire.PlacementSeeds[j].Term })
	for index, seed := range wire.PlacementSeeds {
		if seed.Term == "" || seed.Allocation.Identity == "" || seed.Allocation.Result == "" || seed.Allocation.Kind == "" ||
			(index > 0 && wire.PlacementSeeds[index-1].Term == seed.Term) {
			return fmt.Errorf("engine: malformed child entry placement seed")
		}
	}
	sort.Slice(wire.MemberCellSeeds, func(i, j int) bool {
		if string(wire.MemberCellSeeds[i].Identity) != string(wire.MemberCellSeeds[j].Identity) {
			return string(wire.MemberCellSeeds[i].Identity) < string(wire.MemberCellSeeds[j].Identity)
		}
		return wire.MemberCellSeeds[i].Suffix < wire.MemberCellSeeds[j].Suffix
	})
	for index, seed := range wire.MemberCellSeeds {
		if len(seed.Identity) == 0 || seed.Suffix == "" || !segment.ValidFormattedSegments(seed.Suffix) || len(seed.Wire.Value) == 0 ||
			(seed.Wire.Handle != nil && !validClosureHandle(*seed.Wire.Handle)) ||
			(index > 0 && bytes.Equal(wire.MemberCellSeeds[index-1].Identity, seed.Identity) && wire.MemberCellSeeds[index-1].Suffix == seed.Suffix) {
			return fmt.Errorf("engine: malformed child entry member cell seed")
		}
	}
	sort.Strings(wire.GradualAnyTerms)
	for index, term := range wire.GradualAnyTerms {
		if !strings.HasPrefix(term, "path/") || (index > 0 && wire.GradualAnyTerms[index-1] == term) {
			return fmt.Errorf("engine: malformed child gradual-any boundary")
		}
	}
	return nil
}

func decodeChildEntryWire(value []byte) (childEntryWire, error) {
	for version := uint8(1); version <= 6; version++ {
		prefix := fmt.Sprintf("front/child-entry/v%d/", version)
		if !strings.HasPrefix(string(value), prefix) {
			continue
		}
		var wire childEntryWire
		if err := json.Unmarshal(value[len(prefix):], &wire); err != nil || wire.Version != version {
			return childEntryWire{}, fmt.Errorf("engine: malformed child entry wire")
		}
		return wire, nil
	}
	return childEntryWire{}, nil
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
	wire, err := decodeChildEntryWire(entryValue)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	values := make([]equation.Fact, 0, len(wire.Seeds)+len(wire.ClosureSeeds)+len(wire.MemberCellSeeds)*3+len(wire.GradualAnyTerms)+len(declaredRoots)*4)
	if wire.DeclaredBoundary {
		values = append(values, equation.Fact{Key: declaredEntryBoundaryKey(operation.Target.Body), Value: []byte("declared")})
	}
	seen := make(map[string]bool, len(wire.Seeds))
	seedValues := make(map[string][]byte, len(wire.Seeds))
	for _, seed := range wire.Seeds {
		if !validEntrySeed(seed) || seen[seed.Term] {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry seed")
		}
		seen[seed.Term] = true
		seedValues[seed.Term] = append([]byte(nil), seed.Value...)
		values = append(values,
			equation.Fact{Key: "value/" + seed.Term + "/entry", Value: append([]byte(nil), seed.Value...)},
			equation.Fact{Key: epochFactPrefix + seed.Term + "/entry", Value: []byte("entry")},
		)
	}
	for _, term := range wire.GradualAnyTerms {
		if !seen[term] {
			return equation.TransactionResult{}, fmt.Errorf("engine: gradual-any boundary has no entry seed")
		}
		values = append(values, equation.Fact{Key: "gradual-any/" + term + "/entry", Value: []byte(term)})
	}
	tableIdentityTerms := make(map[string]bool, len(wire.TableIdentitySeeds))
	for _, seed := range wire.TableIdentitySeeds {
		if !seen[seed.Term] || len(seed.Identity) == 0 || tableIdentityTerms[seed.Term] {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry table identity seed")
		}
		tableIdentityTerms[seed.Term] = true
		values = append(values, heapIdentityFact(seed.Term, "entry", seed.Identity))
	}
	placementTerms := make(map[string]bool, len(wire.PlacementSeeds))
	for _, seed := range wire.PlacementSeeds {
		if !seen[seed.Term] || placementTerms[seed.Term] || seed.Allocation.Identity == "" || seed.Allocation.Result == "" || seed.Allocation.Kind == "" {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry placement seed")
		}
		encoded, encodeErr := encodePlacementAllocation(seed.Allocation)
		if encodeErr != nil {
			return equation.TransactionResult{}, encodeErr
		}
		placementTerms[seed.Term] = true
		values = append(values,
			equation.Fact{Key: placementAllocationFactKey(seed.Allocation.Identity), Value: encoded},
			placementBindingFact(seed.Term, "entry", seed.Allocation.Identity),
		)
	}
	closureTerms := make(map[string]bool, len(wire.ClosureSeeds))
	for _, seed := range wire.ClosureSeeds {
		if seed.Term == "" || !seen[seed.Term] || closureTerms[seed.Term] || !validClosureHandle(seed.Handle) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry closure seed")
		}
		encoded, err := json.Marshal(seed.Handle)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		closureTerms[seed.Term] = true
		values = append(values, equation.Fact{Key: "closure/" + seed.Term + "/entry", Value: encoded})
	}
	memberClosures := make(map[string]bool, len(wire.MemberClosureSeeds))
	for index, seed := range wire.MemberClosureSeeds {
		key := seed.Term + "\x00" + seed.Wire.Suffix
		if !seen[seed.Term] || seed.Wire.Suffix == "" || !validClosureHandle(seed.Wire.Handle) || memberClosures[key] {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry member closure seed")
		}
		encoded, err := json.Marshal(seed.Wire)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		memberClosures[key] = true
		values = append(values, equation.Fact{Key: fmt.Sprintf("member-closure/%s/entry/%08d", seed.Term, index), Value: encoded})
	}
	memberCells := make(map[string]bool, len(wire.MemberCellSeeds))
	for _, seed := range wire.MemberCellSeeds {
		key := string(seed.Identity) + "\x00" + seed.Suffix
		if len(seed.Identity) == 0 || seed.Suffix == "" || !segment.ValidFormattedSegments(seed.Suffix) || len(seed.Wire.Value) == 0 ||
			(seed.Wire.Handle != nil && !validClosureHandle(*seed.Wire.Handle)) || memberCells[key] {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed child entry member cell seed")
		}
		memberCells[key] = true
		values = append(values, heapMemberFact(seed.Identity, seed.Suffix, "entry", seed.Wire.Value))
		if len(seed.Wire.MemberIdentity) != 0 {
			values = append(values, heapMemberIdentityFact(seed.Identity, seed.Suffix, "entry", seed.Wire.MemberIdentity))
		}
		if seed.Wire.Handle != nil {
			encoded, marshalErr := json.Marshal(seed.Wire)
			if marshalErr != nil {
				return equation.TransactionResult{}, marshalErr
			}
			values = append(values, equation.Fact{Key: memberCellFactKey(seed.Identity, seed.Suffix, "entry"), Value: encoded})
		}
	}
	for name, root := range declaredRoots {
		declared, ok := shapefact.DecodeTarget(declaredTypes[name])
		if !ok || declared == nil || (!strings.HasPrefix(string(root), "path/") && !strings.HasPrefix(string(root), "temp/")) {
			// Boundary type metadata is optional for a lexical body.  An invalid
			// descriptive entry row cannot become a value fact; skip it rather
			// than rejecting an otherwise independently evaluable child.
			continue
		}
		// Keep the declaration as metadata even when a concrete seed is already
		// present. It is not a value proof, but it preserves an explicit-any
		// boundary for later caller-owned contract checks.
		values = append(values, equation.Fact{Key: "declared-type/" + string(root) + "/entry", Value: append([]byte(nil), declaredTypes[name]...)})
		// A concrete seed is the value authority.  A typed channel seed which
		// carries no channel identity (notably `nil :: Channel<T>`) still needs
		// the declaration-owned identity used by the select kernel.  Preserve a
		// caller-provided channel identity unchanged; only the absent-identity
		// case can use this already-published boundary contract.
		if seen[string(root)] {
			// A concrete caller seed owns the runtime value, but the boundary
			// declaration still owns the channel payload relation below that
			// root.  In particular, a Top seed is an honest unknown value, not a
			// reason to erase the declared Channel<T> witnesses that select reads.
			values = append(values, channelPayloadSummaryFacts(string(root), "entry", declared)...)
			if _, channel := ambient.ChannelPayloadType(declared); channel {
				if !isChannelIdentity(seedValues[string(root)]) {
					identity := []byte("scalar/channel-entry/" + fmt.Sprintf("%x", operation.Target.Body) + "/" + base64.RawURLEncoding.EncodeToString(root))
					values = append(values,
						equation.Fact{Key: "type/" + string(root) + "/entry", Value: append([]byte(nil), declaredTypes[name]...)},
						equation.Fact{Key: "identity/" + string(root) + "/entry", Value: identity},
					)
				}
			}
			continue
		}
		values = append(values,
			equation.Fact{Key: "value/" + string(root) + "/entry", Value: []byte("scalar/top")},
			equation.Fact{Key: epochFactPrefix + string(root) + "/entry", Value: []byte("entry")},
			equation.Fact{Key: "type/" + string(root) + "/entry", Value: append([]byte(nil), declaredTypes[name]...)},
		)
		if declaredIndexableContainer(declared) {
			identity := []byte("declared-table-entry/" + fmt.Sprintf("%x", operation.Target.Body) + "/" + base64.RawURLEncoding.EncodeToString(root))
			values = append(values, heapIdentityFact(string(root), "entry", identity))
		}
		if _, channel := ambient.ChannelPayloadType(declared); channel {
			identity := []byte("scalar/channel-entry/" + fmt.Sprintf("%x", operation.Target.Body) + "/" + base64.RawURLEncoding.EncodeToString(root))
			values = append(values, equation.Fact{Key: "identity/" + string(root) + "/entry", Value: identity})
		}
		// A declared boundary type can carry Channel<T> below its root, as a
		// record of channels does. That is the same closed payload witness an
		// imported result summary publishes, so the boundary declaration uses
		// the same fact family: a select over `source.primary` then reads its
		// payload and identity from the declaration instead of failing closed.
		values = append(values, channelPayloadSummaryFacts(string(root), "entry", declared)...)
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

func declaredIndexableContainer(value typ.Type) bool {
	value = unwrap.Alias(value)
	if value == nil {
		return false
	}
	switch value.Kind() {
	case kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple:
		return true
	default:
		return false
	}
}

func closureHandleFor(term []byte, partition equation.Partition) (closureHandle, bool) {
	if cell, found := memberCellForTerm(term, partition); found {
		if cell.Handle != nil {
			return *cell.Handle, true
		}
	}
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

// memberCellForTerm resolves the final static member through its current
// parent identity. A nested replacement (for example M.dep = {...}) changes
// that parent identity, so consulting M's old flattened .dep.get cell would
// revive a superseded callable capability.
func memberCellForTerm(term []byte, partition equation.Partition) (memberCellWire, bool) {
	root, suffix, member := heapTableAddress(term)
	if !member || suffix == "" {
		return memberCellWire{}, false
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid || len(segments) == 0 {
		return memberCellWire{}, false
	}
	parent := append([]byte(nil), root...)
	if len(segments) > 1 {
		parent = append(parent, []byte(segment.FormatSegments(segments[:len(segments)-1]))...)
	}
	identity, found := tableIdentityForTerm(parent, partition)
	if !found {
		return memberCellWire{}, false
	}
	return currentMemberCell(identity, segment.FormatSegments(segments[len(segments)-1:]), partition)
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

// projectSealedTableMemberValues preserves exact members of an already closed
// table at the replacement path. A later direct member write owns a newer
// epoch and therefore supersedes this projection without source heuristics.
func projectSealedTableMemberValues(target string, tableValue []byte, operation string) ([]equation.Fact, error) {
	table, ok := shapefact.DecodeTable(tableValue)
	if !ok || !table.Closed {
		return nil, nil
	}
	values := make([]equation.Fact, 0, len(table.Members)*2)
	for _, member := range table.Members {
		if !member.Present || member.Suffix == "" {
			continue
		}
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid || len(segments) == 0 {
			return nil, fmt.Errorf("engine: malformed sealed table member suffix")
		}
		memberTarget := target + member.Suffix
		values = append(values,
			equation.Fact{Key: "value/" + memberTarget + "/" + operation, Value: []byte(member.Value)},
			equation.Fact{Key: epochFactPrefix + memberTarget + "/" + operation, Value: []byte(operation)},
		)
	}
	return values, nil
}

// returnMemberClosures collects the member capabilities a table carries out of
// the body that built it. A member closure wire published against the table is
// one source; a static member definition, which publishes its capability at the
// exact member path rather than against the table, is the other. Both are
// existing publications of this partition and a member's latest publication
// wins. The union is computed only where a value leaves its partition, so
// dispatch inside the body keeps consuming the member-path fact directly.
func returnMemberClosures(term []byte, partition equation.Partition) []memberClosureWire {
	prefix := "closure/" + string(term)
	direct := partition.ValuesPrefix(prefix)
	published := memberClosuresFor(term, partition)
	if len(direct) == 0 {
		return published
	}
	bySuffix := make(map[string]struct {
		key  string
		wire memberClosureWire
	}, len(published)+len(direct))
	for _, wire := range published {
		bySuffix[wire.Suffix] = struct {
			key  string
			wire memberClosureWire
		}{wire: wire}
	}
	for _, fact := range direct {
		rest := strings.TrimPrefix(fact.Key, prefix)
		cut := strings.LastIndex(rest, "/")
		if cut <= 0 {
			continue
		}
		suffix := rest[:cut]
		if !segment.ValidFormattedSegments(suffix) {
			continue
		}
		var handle closureHandle
		if json.Unmarshal(fact.Value, &handle) != nil || !validClosureHandle(handle) {
			continue
		}
		if prior, exists := bySuffix[suffix]; exists && prior.key > fact.Key {
			continue
		}
		bySuffix[suffix] = struct {
			key  string
			wire memberClosureWire
		}{key: fact.Key, wire: memberClosureWire{Suffix: suffix, Handle: handle}}
	}
	suffixes := make([]string, 0, len(bySuffix))
	for suffix := range bySuffix {
		suffixes = append(suffixes, suffix)
	}
	sort.Strings(suffixes)
	out := make([]memberClosureWire, 0, len(suffixes))
	for _, suffix := range suffixes {
		out = append(out, bySuffix[suffix].wire)
	}
	return out
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
	if identity, found := tableIdentityForTerm(receiver, partition); found {
		if cell, found := currentMemberCell(identity, "."+method, partition); found && cell.Handle != nil {
			return *cell.Handle, true
		}
	}
	// A static member definition publishes its closure at the exact member
	// path.  Prefer that direct publication: it is invalidated by a later
	// member write just like currentMethodCallable, and it does not infer a
	// capability from the receiver's declared type.
	if strings.HasPrefix(string(receiver), "path/") && method != "" {
		if handle, found := closureHandleFor(append(append([]byte(nil), receiver...), []byte("."+method)...), partition); found {
			return handle, true
		}
	}
	for _, wire := range memberClosuresFor(receiver, partition) {
		if wire.Suffix == "."+method {
			return wire.Handle, true
		}
	}
	return closureHandle{}, false
}

// closureDemandRecurses proves a lexical feedback edge from the already
// published closure capabilities in this caller partition. Missing or opaque
// capabilities contribute no edge; only a closed path returning to root
// activates recursive body demand.
func (l *lexicalEvaluator) closureDemandRecurses(root closureHandle, partition equation.Partition) bool {
	if l == nil || !validClosureHandle(root) {
		return false
	}
	queue := []closureHandle{root}
	seen := make(map[string]bool)
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current.Prototype] {
			continue
		}
		seen[current.Prototype] = true
		if _, exists := l.byPrototype[current.Prototype]; !exists {
			return false
		}
		for _, capture := range current.Captures {
			next, found := closureHandleFor([]byte(capture), partition)
			if !found {
				continue
			}
			if next.Prototype == root.Prototype {
				return true
			}
			queue = append(queue, next)
		}
	}
	return false
}

func boundaryTerm(symbol wir.SymbolID) string { return fmt.Sprintf("path/sym%d", symbol) }

// uncalledExplicitAnyBoundary returns the declaration-owned formal seeds that
// may be published before a call. Captures are admitted separately, and only
// when their exact values and closure capabilities are already published by
// the allocating partition.
func uncalledExplicitAnyBoundary(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || len(child.Boundary.Parameters) == 0 {
		return nil, false
	}
	seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			return nil, false
		}
		declared := unwrap.Alias(child.WIR.Type(parameter.Type))
		if declared == nil || declared.Kind() != kind.Any {
			return nil, false
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: []byte("scalar/claim/claim-kind/3/\"any\"")})
	}
	return seeds, true
}

// uncalledGradualLogicalCallBoundary admits an otherwise closed helper only
// when the lowered graph itself connects an unannotated formal, through Lua
// short-circuit expressions, to a call argument. The entry carries Top plus
// the formal's gradual boundary; the ordinary call-contract kernel remains the
// only authority that can turn that boundary into a diagnostic.
func uncalledGradualLogicalCallBoundary(child front.Compilation) ([]entrySeed, []string, bool) {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) == 0 || len(child.Boundary.Parameters) != 1 {
		return nil, nil, false
	}
	formals := make(map[string]bool, len(child.Boundary.Parameters))
	seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
	terms := make([]string, 0, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type != 0 {
			return nil, nil, false
		}
		term := boundaryTerm(parameter.Symbol)
		formals[term] = true
		seeds = append(seeds, entrySeed{Term: term, Value: []byte("scalar/top")})
		terms = append(terms, term)
	}
	tainted := make(map[string]bool, len(formals))
	for formal := range formals {
		tainted[formal] = true
	}
	logical := false
	changed := true
	for changed {
		changed = false
		for _, operation := range child.Artifact.Equations {
			switch operation.Occurrence.Kind {
			case "expression":
				kind, found := artifactOperand(operation.Operands, "kind")
				if !found || string(kind) != strconv.Itoa(int(wir.OpLogical)) {
					continue
				}
				left, hasLeft := artifactOperand(operation.Operands, "left")
				right, hasRight := artifactOperand(operation.Operands, "right")
				result, hasResult := artifactOperand(operation.Operands, "result")
				if !hasLeft || !hasRight || !hasResult || (!tainted[string(left)] && !tainted[string(right)]) {
					continue
				}
				logical = true
				if !tainted[string(result)] {
					tainted[string(result)] = true
					changed = true
				}
			case "environment-write":
				value, hasValue := artifactOperand(operation.Operands, "value")
				target, hasTarget := artifactOperand(operation.Operands, "target")
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
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "argument-") && !strings.HasPrefix(operand.Role, "argument-display-") && tainted[string(operand.Term.Encoding)] {
				return seeds, terms, true
			}
		}
	}
	return nil, nil, false
}

// cyclicPlacementWitnessEntry supplies only declaration-owned parameter facts
// to a cyclic lexical body. Captures are added exclusively by
// uncalledChildEntry from the allocating partition's published values. The
// result is filtered to boundary-free stack witnesses, so no uncalled body can
// claim a retain, share, or caller-visible graph.
func cyclicPlacementWitnessEntry(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || child.Cyclic == nil || len(child.Boundary.Parameters) == 0 {
		return nil, false
	}
	seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			return nil, false
		}
		declared := unwrap.Alias(child.WIR.Type(parameter.Type))
		if declared == nil || declared.Kind() == kind.Any {
			return nil, false
		}
		value, ok := shapefact.EncodeTarget(declared)
		if !ok {
			return nil, false
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: value})
	}
	return seeds, true
}

// placementReturnWitnessEntry admits a prospective table-returning closure
// only from its own declared entry.  The closure must be capture-free and its
// result slot non-recursive, so the evaluated allocation is a closed function
// summary rather than a caller-specific reconstruction.
func placementReturnWitnessEntry(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 ||
		len(child.Boundary.DeclaredReturns) != 1 || !hasProjectableTableResult(child) {
		return nil, false
	}
	returned := unwrap.Alias(child.WIR.Type(child.Boundary.DeclaredReturns[0]))
	if returned == nil {
		return nil, false
	}
	switch returned.Kind() {
	case kind.Array, kind.Map, kind.Record, kind.ReadonlyMap:
	default:
		return nil, false
	}
	seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			return nil, false
		}
		declared := unwrap.Alias(child.WIR.Type(parameter.Type))
		if declared == nil || declared.Kind() == kind.Any {
			return nil, false
		}
		value, ok := shapefact.EncodeTarget(declared)
		if !ok {
			return nil, false
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: value})
	}
	return seeds, true
}

// placementDeclaredScalarResultWitnesses exposes an immutable scalar result
// only when both its exact provider and result slot are already published by
// an evaluated declaration entry.  It does not turn a broad local Top into a
// scalar: the provider's closed return contract is the independent authority.
func placementDeclaredScalarResultWitnesses(child front.Compilation, outcome equation.OutputClosure) []equation.Fact {
	values := make(map[string]bool)
	for _, fact := range outcome.Values {
		if strings.HasPrefix(fact.Key, "value/") {
			if termOperation := strings.TrimPrefix(fact.Key, "value/"); strings.LastIndex(termOperation, "/") > 0 {
				values[termOperation[:strings.LastIndex(termOperation, "/")]] = true
			}
		}
	}
	providers := make(map[string]string)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "external-call" {
			continue
		}
		application, hasApplication := artifactOperand(operation.Operands, "application")
		provider, hasProvider := artifactOperand(operation.Operands, "provider")
		if !hasApplication || !hasProvider {
			continue
		}
		if name, ok := placementGlobalProviderName(provider); ok {
			providers[string(application)] = name
		}
	}
	var facts []equation.Fact
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		application, found := artifactOperand(operation.Operands, "application")
		name, foundProvider := providers[string(application)]
		if !found || !foundProvider {
			continue
		}
		signature, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(name)
		if !found || signature.Type == nil {
			continue
		}
		for _, operand := range operation.Operands {
			if !strings.HasPrefix(operand.Role, "result-") || operand.Role == "result-display" {
				continue
			}
			index, err := strconv.Atoi(strings.TrimPrefix(operand.Role, "result-"))
			if err != nil || index < 0 || index >= len(signature.Type.Returns) || !values[string(operand.Term.Encoding)] || !placementClosedScalarType(signature.Type.Returns[index]) {
				continue
			}
			identity := "scalar/provider/" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%x/%s/%d", child.Body, operation.Target.Name, index)))
			encoded, err := encodePlacementAllocation(placementAllocationFact{
				Identity: identity, Result: "placement/scalar/" + base64.RawURLEncoding.EncodeToString([]byte(identity)), Kind: "lua.scalar", Complete: true,
			})
			if err == nil {
				facts = append(facts, equation.Fact{Key: placementAllocationFactKey(identity), Value: encoded})
			}
		}
	}
	return facts
}

// placementDeclaredScalarLocalWitnesses projects the body's own declarations. A
// local whose declared type is a closed scalar holds no object identity: there
// is nothing for another frame, actor, or send boundary to retain, so its
// storage is the frame that declares it whatever the body then does with the
// value. The claim's sealed shape target is the sole authority; a claim without
// one, or one whose target is not a closed scalar, contributes nothing.
func placementDeclaredScalarLocalWitnesses(child front.Compilation) []equation.Fact {
	var facts []equation.Fact
	seen := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		claimKind, hasKind := artifactOperand(operation.Operands, "kind")
		target, hasTarget := artifactOperand(operation.Operands, "target")
		shape, hasShape := artifactOperand(operation.Operands, "shape-target")
		if !hasKind || !hasTarget || !hasShape || string(claimKind) != "claim-kind/"+strconv.Itoa(int(wir.ClaimAnnotation)) {
			continue
		}
		declared, decoded := shapefact.DecodeTarget(shape)
		if !decoded || !placementClosedScalarType(declared) {
			continue
		}
		identity := "scalar/declaration/" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%x/%s", child.Body, target)))
		if seen[identity] {
			continue
		}
		seen[identity] = true
		encoded, err := encodePlacementAllocation(placementAllocationFact{
			Identity: identity, Result: "placement/scalar/" + base64.RawURLEncoding.EncodeToString([]byte(identity)), Kind: "lua.scalar", Complete: true,
		})
		if err != nil {
			continue
		}
		facts = append(facts, equation.Fact{Key: placementAllocationFactKey(identity), Value: encoded})
	}
	return facts
}

// placementReturnedClosureWitnesses projects only facts whose two authorities
// are already published: the enclosing module returns this exact closure, and
// the child artifact stores one declared table formal into one of its captured
// tables. The returned closure keeps its captured state live; the matching
// store is a prospective shared-input boundary, not a reconstruction from the
// function's source spelling.
func placementReturnedClosureWitnesses(child front.Compilation, partition equation.Partition) []equation.Fact {
	// This projection reads the child's boundary and its frozen artifact only.
	// Both are published for every admitted body, so a loop inside the child
	// changes nothing it inspects: the cyclic signal selects an execution path,
	// not the availability of the static store evidence.
	if child.WIR == nil {
		return nil
	}
	captures := make(map[string]bool, len(child.Boundary.Captures))
	for _, capture := range child.Boundary.Captures {
		term := boundaryTerm(capture.Symbol)
		captures[term] = true
	}
	formals := make(map[string]typ.Type, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			continue
		}
		value := unwrap.Alias(child.WIR.Type(parameter.Type))
		if placementTableWitnessType(value) {
			formals[boundaryTerm(parameter.Symbol)] = value
		}
	}
	if len(captures) == 0 || len(formals) == 0 {
		return nil
	}
	var facts []equation.Fact
	seen := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "index-mutation" {
			continue
		}
		target, hasTarget := artifactOperand(operation.Operands, "container")
		value, hasValue := artifactOperand(operation.Operands, "value")
		if !hasTarget || !hasValue || !captures[string(target)] || !placementTableWitnessType(formals[string(value)]) {
			continue
		}
		identity := "formal-store/" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%x/%s/%s", child.Body, value, operation.Target.Name)))
		if seen[identity] {
			continue
		}
		seen[identity] = true
		encoded, err := encodePlacementAllocation(placementAllocationFact{
			Identity: identity,
			Result:   "placement/formal/" + base64.RawURLEncoding.EncodeToString([]byte(identity)),
			Kind:     "lua.table",
			Complete: true,
		})
		if err != nil {
			continue
		}
		facts = append(facts,
			equation.Fact{Key: placementAllocationFactKey(identity), Value: encoded},
			placementEventFact(identity, operation.Target.Name, placementEventShared),
		)
	}
	for capture := range captures {
		if allocation, found := placementAllocationForTerm([]byte(capture), partition); found {
			facts = append(facts, placementEventFact(allocation.Identity, "closure-return", placementEventOwned))
		}
	}
	return facts
}

func placementTableWitnessType(value typ.Type) bool {
	if value = unwrap.Alias(value); value == nil {
		return false
	}
	switch value.Kind() {
	case kind.Array, kind.Map, kind.Record, kind.ReadonlyMap:
		return true
	default:
		return false
	}
}

func placementClosedScalarType(value typ.Type) bool {
	value = unwrap.Alias(value)
	if value == nil {
		return false
	}
	switch value.Kind() {
	case kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return true
	default:
		return false
	}
}

// publishesClosureReturn is an exact artifact edge from a closure allocation
// result to the enclosing module's return tuple.  Member-owned closures remain
// demand-driven: publishing their surrounding table is not a publication of
// each callable's future allocation sites.
func (l *lexicalEvaluator) publishesClosureReturn(body equation.BodyID, result string) bool {
	if l == nil || result == "" {
		return false
	}
	parent, found := l.byBody[body]
	if !found {
		return false
	}
	for _, operation := range parent.Artifact.Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "return-value-") && string(operand.Term.Encoding) == result {
				return true
			}
		}
	}
	return false
}

// declaredBoundaryAdmission records which obligations a declaration-only entry
// was admitted for. Each flag names the exact diagnostic family the boundary
// can discharge; a family with no admission reason stays demand-driven.
type declaredBoundaryAdmission struct {
	Seeds       []entrySeed
	Admitted    bool
	Method      bool
	MemberWrite bool
	Concat      map[string]bool
	Comparison  map[string]bool
}

// uncalledDeclaredBoundary materializes only the checker-owned type witnesses
// already present on a capture-free function boundary. These are not runtime
// values and carry no invented member facts: the shape encoder preserves the
// declared union/optional relation for the child's ordinary claim and branch
// consumers. A missing, variadic, recursive, or captured boundary stays
// dormant.
func uncalledDeclaredBoundary(child front.Compilation) declaredBoundaryAdmission {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 || len(child.Boundary.Parameters) == 0 {
		return declaredBoundaryAdmission{}
	}
	// The declaration-only entry supports a closed discriminant proof, not an
	// arbitrary body execution. A direct static member call is also admissible
	// when its receiver is one of those declared formals: resolving a missing
	// member consumes that same published declaration and produces no call
	// result or capability. Calls through any other value, dynamic reads, and
	// select operations need a caller-owned value/heap entry and remain
	// demand-driven.
	formals := make(map[string]bool, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			return declaredBoundaryAdmission{}
		}
		formals[boundaryTerm(parameter.Symbol)] = true
	}
	memberCalls := make(map[string]bool)
	hasDirectMethod := false
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		if uncalledDeclaredMemberCall(child, operation, formals) {
			memberCalls["call/"+operation.Target.Name] = true
			hasDirectMethod = hasDirectMethod || hasDeclaredFormalMethodCall(child, operation, formals)
			continue
		}
		// A published standard-library contract reached only from terms this
		// declaration entry already closes needs no caller-owned value: the
		// registry signature is the result authority. Admitting it keeps the
		// rest of the body — its declared member reads and their claims —
		// evaluable instead of dormant behind an ambient call.
		if uncalledDeclaredStdlibCall(child.Artifact.Equations, operation, formals) ||
			uncalledDeclaredExpandedStdlibCall(child.Artifact.Equations, operation, formals) {
			memberCalls["call/"+operation.Target.Name] = true
		}
	}
	hasBranch, hasDeclaredMemberRead, hasDeclaredMemberCall, hasDeclaredAssignment, hasDeclaredMemberWrite := false, false, false, false, false
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "apply":
			if !memberCalls["call/"+operation.Target.Name] {
				return declaredBoundaryAdmission{}
			}
			hasDeclaredMemberCall = true
		case "external-call":
			application, found := artifactOperand(operation.Operands, "application")
			if !found || !memberCalls[string(application)] {
				return declaredBoundaryAdmission{}
			}
		case "dynamic-index-read", "channel-select":
			return declaredBoundaryAdmission{}
		case "branch-relations":
			hasBranch = true
		case "path-replacement":
			hasDeclaredMemberWrite = hasDeclaredMemberWrite || uncalledDeclaredFormalMemberWrite(operation, formals)
		case "claim":
			hasDeclaredMemberRead = hasDeclaredMemberRead || uncalledDeclaredFormalMemberRead(child, operation, formals)
			hasDeclaredAssignment = hasDeclaredAssignment || uncalledDeclaredFormalAssignment(child, operation, formals)
		}
	}
	// A declared method return can depend on a branch-local refinement. Its
	// allocation-time boundary is therefore limited to straight-line bodies;
	// declared missing-member reads retain their independent diagnostic path.
	if hasDirectMethod && hasBranch {
		return declaredBoundaryAdmission{}
	}
	concatOperations := uncalledDeclaredFormalConcatOperations(child.Artifact.Equations, memberCalls)
	comparisonOperations := uncalledDeclaredFormalOrderedComparisonOperations(child, formals)
	if !hasDeclaredMemberRead && !hasDeclaredMemberCall && !hasDeclaredAssignment && !hasDeclaredMemberWrite && len(concatOperations) == 0 && len(comparisonOperations) == 0 {
		return declaredBoundaryAdmission{}
	}
	seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			return declaredBoundaryAdmission{}
		}
		declared := child.WIR.Type(parameter.Type)
		value, ok := shapefact.EncodeTarget(declared)
		if !ok || declared == nil {
			return declaredBoundaryAdmission{}
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: value})
	}
	return declaredBoundaryAdmission{Seeds: seeds, Admitted: true, Method: hasDirectMethod, MemberWrite: hasDeclaredMemberWrite, Concat: concatOperations, Comparison: comparisonOperations}
}

// uncalledDeclaredLocalUnionReadBoundary admits one allocation-time relay:
// declared formals feed an already-published local closure, whose sole result
// is assigned and read through an exact static key. The call capability and
// declared return are supplied by the enclosing allocation partition; the
// child receives neither an invented value nor an arbitrary capture. This is
// sufficient to check an unguarded union member assignment and one closed
// equality discriminant over that result. Other branches, dynamic keys,
// external calls, and child effects remain demand-driven.
func uncalledDeclaredLocalUnionReadBoundary(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Parameters) == 0 {
		return nil, false
	}
	formals := make(map[string]entrySeed, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 || parameter.Type == 0 {
			return nil, false
		}
		declared := child.WIR.Type(parameter.Type)
		value, ok := shapefact.EncodeTarget(declared)
		if !ok || declared == nil {
			return nil, false
		}
		term := boundaryTerm(parameter.Symbol)
		formals[term] = entrySeed{Term: term, Value: value}
	}
	applications := make(map[string]bool)
	derived := uncalledDeclaredLocalUnionExpressionTerms(child, formals)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		callee, hasCallee := artifactOperand(operation.Operands, "callee")
		arity, hasArity := artifactOperand(operation.Operands, "result-arity")
		if !hasCallee || !strings.HasPrefix(string(callee), "path/") || !hasArity || string(arity) != "1" {
			return nil, false
		}
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "argument-") {
				if _, err := callArgumentIndex(operand.Role); err != nil {
					continue
				}
				if _, formal := formals[string(operand.Term.Encoding)]; !formal && !derived[string(operand.Term.Encoding)] {
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
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		application, found := artifactOperand(operation.Operands, "application")
		if !found || !applications[string(application)] {
			return nil, false
		}
		result, found := artifactOperand(operation.Operands, "result-00000000")
		if !found {
			return nil, false
		}
		results[string(result)] = true
	}
	paths := make(map[string]bool)
	reads := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		value, hasValue := artifactOperand(operation.Operands, "value")
		target, hasTarget := artifactOperand(operation.Operands, "target")
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
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry", "apply", "call-results", "environment-write", "claim", "expression", "publication":
			continue
		case "branch-relations":
			if !uncalledDeclaredLocalUnionBranch(operation, paths, formals) && !uncalledDeclaredLocalUnionExpressionBranch(operation, formals, derived) {
				return nil, false
			}
			continue
		case "external-call":
			application, found := artifactOperand(operation.Operands, "application")
			if !found || !applications[string(application)] {
				return nil, false
			}
			continue
		case "dynamic-index-read":
			container, hasContainer := artifactOperand(operation.Operands, "container")
			key, hasKey := artifactOperand(operation.Operands, "key")
			target, hasTarget := artifactOperand(operation.Operands, "target")
			if !hasContainer || !paths[string(container)] || !hasKey || !strings.HasPrefix(string(key), "scalar/string/") || !hasTarget {
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
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		value, found := artifactOperand(operation.Operands, "value")
		if found && reads[string(value)] {
			seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
			for _, parameter := range child.Boundary.Parameters {
				seeds = append(seeds, formals[boundaryTerm(parameter.Symbol)])
			}
			return seeds, true
		}
	}
	return nil, false
}

// uncalledDeclaredLocalUnionExpressionTerms closes only expression temporaries
// whose value is computed from declared formals and exact scalar literals. The
// resulting terms may supply an uncalled local union call, but they never
// authorize a concrete result arm by themselves.
func uncalledDeclaredLocalUnionExpressionTerms(child front.Compilation, formals map[string]entrySeed) map[string]bool {
	known := make(map[string]bool, len(formals))
	for term := range formals {
		known[term] = true
	}
	for changed := true; changed; {
		changed = false
		for _, operation := range child.Artifact.Equations {
			result, hasResult := artifactOperand(operation.Operands, "result")
			if operation.Occurrence.Kind != "expression" || !hasResult || !strings.HasPrefix(string(result), "temp/") || known[string(result)] {
				continue
			}
			valid := true
			for _, operand := range operation.Operands {
				if operand.Role != "left" && operand.Role != "right" {
					continue
				}
				if !known[string(operand.Term.Encoding)] && !exactRelationScalar(operand.Term.Encoding) {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			known[string(result)] = true
			changed = true
		}
	}
	for term := range formals {
		delete(known, term)
	}
	return known
}

// uncalledDeclaredLocalUnionBranch accepts the branch relation that is
// already justified by the child entry: one returned union member is compared
// with one exact declared formal. It rejects negation, compound evidence, and
// any relation not rooted in the admitted call result, so a declaration-only
// entry cannot import arbitrary caller control flow.
func uncalledDeclaredLocalUnionBranch(operation equation.Equation, results map[string]bool, formals map[string]entrySeed) bool {
	if operation.Occurrence.Kind != "branch-relations" || len(operation.Guards) != 0 {
		return false
	}
	predicate, found := artifactOperand(operation.Operands, "predicate")
	if !found || !strings.HasPrefix(string(predicate), branchPredicatePrefix) {
		return false
	}
	var relation branchPredicateWire
	if json.Unmarshal(predicate[len(branchPredicatePrefix):], &relation) != nil || relation.Kind != "path-equal" || relation.Negated || relation.Path == "" || relation.OtherPath == "" {
		return false
	}
	left, right := "path/"+relation.Path, "path/"+relation.OtherPath
	rootedInResult := func(path string) bool {
		for result := range results {
			if strings.HasPrefix(path, result+".") || strings.HasPrefix(path, result+"[") {
				return true
			}
		}
		return false
	}
	_, leftFormal := formals[left]
	_, rightFormal := formals[right]
	return (rootedInResult(left) && rightFormal) || (rootedInResult(right) && leftFormal)
}

// uncalledDeclaredLocalUnionExpressionBranch accepts only the control edges
// emitted for a finite expression temporary or a declared boolean formal. The
// expression remains an unknown selector at the local call boundary; this
// helper admits evaluation so its result union is retained for the later read.
func uncalledDeclaredLocalUnionExpressionBranch(operation equation.Equation, formals map[string]entrySeed, derived map[string]bool) bool {
	if operation.Occurrence.Kind != "branch-relations" || len(operation.Guards) != 0 {
		return false
	}
	if condition, found := artifactOperand(operation.Operands, "condition"); found {
		return derived[string(condition)]
	}
	predicate, found := artifactOperand(operation.Operands, "predicate")
	if !found || !strings.HasPrefix(string(predicate), branchPredicatePrefix) {
		return false
	}
	var relation branchPredicateWire
	if json.Unmarshal(predicate[len(branchPredicatePrefix):], &relation) != nil || relation.Kind != "truthy" || relation.Negated || relation.Path == "" {
		return false
	}
	_, formal := formals["path/"+relation.Path]
	return formal
}

// uncalledLocalUnionReadEntry carries the exact direct-call capability already
// published at the enclosing allocation point into the admitted child. The
// child artifact itself names that path, while closureHandleFor verifies it is
// a local capability rather than a source-level function spelling.
func (l *lexicalEvaluator) uncalledLocalUnionReadEntry(child front.Compilation, formalSeeds []entrySeed, partition equation.Partition) ([]byte, bool, error) {
	seeds := append([]entrySeed(nil), formalSeeds...)
	closures := make([]entryClosureSeed, 0)
	seen := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		callee, found := artifactOperand(operation.Operands, "callee")
		if !found || !strings.HasPrefix(string(callee), "path/") || seen[string(callee)] {
			continue
		}
		value, known := resolveKnownCurrentValue(callee, partition)
		handle, callable := closureHandleFor(callee, partition)
		if !known || isUnknownScalar(value) || !callable || !l.uncalledLocalUnionCalleeHasAlternativeReturns(handle) {
			return nil, false, nil
		}
		seen[string(callee)] = true
		seeds = append(seeds, entrySeed{Term: string(callee), Value: value})
		closures = append(closures, entryClosureSeed{Term: string(callee), Handle: handle})
	}
	if len(closures) == 0 {
		return nil, false, nil
	}
	entry, err := encodeChildEntryWithCapabilities(seeds, closures, childEntryMemberClosureSeeds(seeds, nil, partition), tableIdentitySeedsForEntry(seeds, partition), memberCellSeedsForEntry(seeds, partition))
	if err != nil {
		return nil, false, err
	}
	return entry, true, nil
}

// uncalledLocalUnionCalleeHasAlternativeReturns requires the callee's own
// front artifact to publish more than one normal return candidate. A single
// concrete return is not a union witness: admitting it through the
// declaration-only path would make an unreachable else arm look like an
// ordinary assignment error.
func (l *lexicalEvaluator) uncalledLocalUnionCalleeHasAlternativeReturns(handle closureHandle) bool {
	child, found := l.byPrototype[handle.Prototype]
	if !found {
		return false
	}
	returns := 0
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		if _, found := artifactOperand(operation.Operands, "return-value-00000000"); found {
			returns++
		}
	}
	return returns > 1
}

// publishStaticNilCallDiagnostic projects a call contract from an existing
// closure capture only when either the child has already published the
// argument's exact nil write or the allocating body published an unconditional
// nil write to that capture. A nested body's guard does not transport the
// enclosing body's branch correlation across the closure boundary. The guarded
// call remains a possible path, so its parameter mismatch is source-owned; no
// branch truth, call result, or inferred alias is manufactured here.
func (l *lexicalEvaluator) publishStaticNilCallDiagnostic(closure *equation.OutputClosure, child front.Compilation, parentOperations []equation.Equation, allocationTarget string, captures []string, partition equation.Partition) {
	if l == nil || closure == nil || child.Cyclic != nil || len(child.Boundary.Captures) == 0 || len(child.Boundary.Captures) != len(captures) {
		return
	}
	captured := make(map[string][]byte, len(captures))
	captureSources := make(map[string][]byte, len(captures))
	for index, capture := range child.Boundary.Captures {
		term := captures[index]
		captureSources[boundaryTerm(capture.Symbol)] = []byte(term)
		value, known := resolveKnownCurrentValue([]byte(term), partition)
		if known && !isUnknownScalar(value) {
			captured[boundaryTerm(capture.Symbol)] = value
		}
	}
	for _, item := range child.Artifact.Equations {
		if item.Occurrence.Kind != "apply" || len(item.Guards) == 0 {
			continue
		}
		callee, hasCallee := artifactOperand(item.Operands, "callee")
		calleeValue, capturedCallee := captured[string(callee)]
		if !hasCallee || !capturedCallee {
			continue
		}
		signature, callable := callableSignature(calleeValue)
		if !callable {
			continue
		}
		for _, operand := range item.Operands {
			index, err := callArgumentIndex(operand.Role)
			if err != nil {
				continue
			}
			expected, accepts := callableParameterAt(signature, index)
			capturedNil := string(captured[string(operand.Term.Encoding)]) == "scalar/nil"
			capturedSource := captureSources[string(operand.Term.Encoding)]
			capturedUnconditionalNil := latestUnconditionalNilWriteBefore(parentOperations, capturedSource, allocationTarget)
			childNilWrite := latestPriorWriteIsNilOnCallPath(child.Artifact.Equations, operand.Term.Encoding, item.Target.Name, item.Guards)
			if !accepts || !callableParameterRejectsNil(expected) || (!capturedNil && !capturedUnconditionalNil && !childNilWrite) {
				continue
			}
			fact := equation.Fact{
				Key:   "type.call.direct.argument_type/" + item.Target.Name + "/" + indexedCallSubject("argument", index),
				Value: []byte(fmt.Sprintf("argument %d may be nil, not %s", index+1, callableParameterType(expected))),
			}
			spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, []equation.Fact{fact})
			for _, published := range publishedDiagnostics(child.Artifact, equation.OutputClosure{Diagnostics: []equation.Fact{fact}}, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil) {
				key := "child/" + fmt.Sprintf("%x", child.Body) + "/" + published.Fact.Key
				closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), published.Fact.Value...)})
				if published.Span.Valid() {
					l.diagnosticSpans[key] = published.Span
				}
				published.Fact.Key = key
				l.childPublished[key] = published
			}
			return
		}
	}
	for _, nested := range child.Nested {
		for _, item := range child.Artifact.Equations {
			captures, matches := nestedClosureAllocationCaptures(item, nested.PrototypeName)
			if !matches {
				continue
			}
			l.publishStaticNilCallDiagnostic(closure, nested, child.Artifact.Equations, item.Target.Name, captures, partition)
		}
	}
}

// nestedClosureAllocationCaptures exposes the front's sealed capture list for
// one nested closure allocation. It accepts only an exact object-materialization
// publication whose prototype is already catalogued by the front; malformed or
// reordered capture roles leave the nested body dormant.
func nestedClosureAllocationCaptures(operation equation.Equation, prototype string) ([]string, bool) {
	if operation.Occurrence.Kind != "object-materialization" || prototype == "" {
		return nil, false
	}
	encodedPrototype, found := artifactOperand(operation.Operands, "prototype")
	if !found || string(encodedPrototype) != "prototype/"+prototype {
		return nil, false
	}
	type captureOperand struct {
		index int
		value string
	}
	items := make([]captureOperand, 0)
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "capture-") {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(operand.Role, "capture-"))
		if err != nil || index < 0 {
			return nil, false
		}
		items = append(items, captureOperand{index: index, value: string(operand.Term.Encoding)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].index < items[j].index })
	for index, item := range items {
		if item.index != index || item.value == "" {
			return nil, false
		}
	}
	captures := make([]string, len(items))
	for index, item := range items {
		captures[index] = item.value
	}
	return captures, true
}

// latestUnconditionalNilWriteBefore reads only the allocating body's exact
// environment-write publications. A guarded write cannot replace this nil at
// a nested closure boundary because its branch relation belongs to the parent
// body and is not a fact in the child's independent guard space. An alias, a
// dynamic value, or a later unconditional write makes the result unavailable.
func latestUnconditionalNilWriteBefore(operations []equation.Equation, target []byte, before string) bool {
	if len(target) == 0 {
		return false
	}
	latest := ""
	nilValue := false
	for _, operation := range operations {
		if operation.Occurrence.Kind != "environment-write" || len(operation.Guards) != 0 || operation.Target.Name >= before || operation.Target.Name <= latest {
			continue
		}
		writeTarget, hasTarget := artifactOperand(operation.Operands, "target")
		value, hasValue := artifactOperand(operation.Operands, "value")
		if !hasTarget || !hasValue || !bytes.Equal(writeTarget, target) {
			continue
		}
		latest, nilValue = operation.Target.Name, string(value) == "scalar/nil"
	}
	return latest != "" && nilValue
}

// publishCapturedOptionalMemberCallDiagnostic follows only a closed front
// dataflow chain: a captured typed provider's direct static call result is
// written to a local, whose static optional member is passed to another
// captured typed provider. The guarded call is still a possible source path;
// this helper merely publishes the two existing member contracts without
// choosing any intervening branch.
func (l *lexicalEvaluator) publishCapturedOptionalMemberCallDiagnostic(closure *equation.OutputClosure, child front.Compilation, captures []string, partition equation.Partition) {
	// This allocation-time projector is deliberately bounded. Large bodies are
	// evaluated only through ordinary demanded flow so this source-only scan
	// cannot consume the solver budget needed by their recursive summaries.
	if l == nil || closure == nil || child.Cyclic != nil || len(child.Artifact.Equations) > 128 || len(child.Boundary.Captures) == 0 || len(child.Boundary.Captures) != len(captures) {
		return
	}
	captured := make(map[string][]byte, len(captures))
	for index, capture := range child.Boundary.Captures {
		value, known := resolveKnownCurrentValue([]byte(captures[index]), partition)
		if !known || isUnknownScalar(value) {
			return
		}
		if imported, found := l.importedAuthority(captures[index]); found {
			encoded, encodedOK := shapefact.EncodeTarget(imported)
			if !encodedOK {
				return
			}
			value = encoded
		}
		captured[boundaryTerm(capture.Symbol)] = value
	}
	for _, item := range child.Artifact.Equations {
		if item.Occurrence.Kind != "apply" || len(item.Guards) == 0 {
			continue
		}
		callee, hasCallee := artifactOperand(item.Operands, "callee")
		function, callable := capturedStaticFunction(callee, captured)
		if !hasCallee || !callable {
			continue
		}
		for _, operand := range item.Operands {
			index, err := callArgumentIndex(operand.Role)
			if err != nil || index >= len(function.Params) || function.Params[index].Type == nil || !callableParameterRejectsNil(function.Params[index].Type.String()) {
				continue
			}
			if !capturedCallResultOptionalMember(l, child.Artifact.Equations, operand.Term.Encoding, item.Target.Name, captured, partition) {
				continue
			}
			fact := equation.Fact{
				Key:   "type.call.direct.argument_type/" + item.Target.Name + "/" + indexedCallSubject("argument", index),
				Value: []byte(fmt.Sprintf("argument %d may be nil, not %s", index+1, callableParameterType(function.Params[index].Type.String()))),
			}
			spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, []equation.Fact{fact})
			for _, published := range publishedDiagnostics(child.Artifact, equation.OutputClosure{Diagnostics: []equation.Fact{fact}}, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil) {
				key := "child/" + fmt.Sprintf("%x", child.Body) + "/" + published.Fact.Key
				closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), published.Fact.Value...)})
				if published.Span.Valid() {
					l.diagnosticSpans[key] = published.Span
				}
				published.Fact.Key = key
				l.childPublished[key] = published
			}
			return
		}
	}
}

func capturedStaticFunction(callee []byte, captured map[string][]byte) (*typ.Function, bool) {
	root, suffix, member := tableAddress(callee)
	segments, static := segment.ParseFormattedSegments(suffix)
	value, available := captured[string(root)]
	if !member || !static || len(segments) == 0 || !available {
		return nil, false
	}
	receiver, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		table, sealed := shapefact.DecodeTable(value)
		if !sealed || len(segments) < 2 {
			return nil, false
		}
		first := segment.FormatSegments(segments[:1])
		for _, member := range table.Members {
			if member.Suffix != first || !member.Present {
				continue
			}
			receiver, decoded = shapefact.DecodeTarget([]byte(member.Value))
			break
		}
		if !decoded {
			return nil, false
		}
		segments = segments[1:]
	}
	calleeType, found := variant.FieldAtPath(receiver, segments)
	if !found {
		return nil, false
	}
	function, callable := unwrap.Alias(subst.ExpandInstantiated(calleeType)).(*typ.Function)
	return function, callable && function != nil
}

func capturedCallResultOptionalMember(lexical *lexicalEvaluator, operations []equation.Equation, argument []byte, before string, captured map[string][]byte, partition equation.Partition) bool {
	root, suffix, member := tableAddress(argument)
	segments, static := segment.ParseFormattedSegments(suffix)
	if !member || !static || len(segments) == 0 {
		return false
	}
	latest, value := "", []byte(nil)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "environment-write" || operation.Target.Name >= before {
			continue
		}
		target, hasTarget := artifactOperand(operation.Operands, "target")
		written, hasValue := artifactOperand(operation.Operands, "value")
		if hasTarget && hasValue && bytes.Equal(target, root) && operation.Target.Name > latest {
			latest, value = operation.Target.Name, written
		}
	}
	if latest == "" || !strings.HasPrefix(string(value), "temp/") {
		return false
	}
	for _, result := range operations {
		if result.Occurrence.Kind != "call-results" {
			continue
		}
		application, hasApplication := artifactOperand(result.Operands, "application")
		resultValue, hasResult := artifactOperand(result.Operands, "result-00000000")
		if !hasApplication || !hasResult || !bytes.Equal(resultValue, value) {
			continue
		}
		applyName, valid := strings.CutPrefix(string(application), "call/")
		if !valid {
			return false
		}
		provider, hasProvider := artifactOperand(result.Operands, "provider")
		if hasProvider {
			if returned, found := hostGlobalProviderResultType(lexical, provider, 0, nil, partition); found {
				field, projected := typedPathSegments(returned, segments)
				return projected && optionalConcreteWitnessType(field)
			}
			if signature, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(providerName(provider)); found && signature.Type != nil {
				if returned, instantiated := instantiateProviderReturn(signature.Type, nil, partition, 0); instantiated {
					field, projected := typedPathSegments(returned, segments)
					return projected && optionalConcreteWitnessType(field)
				}
			}
		}
		for _, apply := range operations {
			if apply.Target.Name != applyName || apply.Occurrence.Kind != "apply" {
				continue
			}
			callee, found := artifactOperand(apply.Operands, "callee")
			function, callable := capturedStaticFunction(callee, captured)
			if !found || !callable || len(function.Returns) == 0 || function.Returns[0] == nil {
				return false
			}
			field, found := typedPathSegments(function.Returns[0], segments)
			return found && optionalConcreteWitnessType(field)
		}
	}
	return false
}

// latestPriorWriteIsNilOnCallPath accepts nil only when it is the last write
// that can reach the guarded call. A later write under the same edge of a
// boolean that was directly inverted before the call is excluded: the two
// edges cannot co-occur. This reads the front's exact branch and expression
// publications; an alias, an intervening write, or any unrecognized control
// relation leaves the later write reachable and fails closed.
func latestPriorWriteIsNilOnCallPath(operations []equation.Equation, target []byte, before string, callGuards []equation.Guard) bool {
	latest := ""
	nilValue := false
	for _, operation := range operations {
		if operation.Occurrence.Kind != "environment-write" || operation.Target.Name >= before {
			continue
		}
		writeTarget, hasTarget := artifactOperand(operation.Operands, "target")
		value, hasValue := artifactOperand(operation.Operands, "value")
		if !hasTarget || !hasValue || !bytes.Equal(writeTarget, target) || operation.Target.Name <= latest {
			continue
		}
		if guardedOperationsAreExclusive(operations, operation.Guards, callGuards) {
			continue
		}
		latest, nilValue = operation.Target.Name, len(operation.Guards) == 0 && string(value) == "scalar/nil"
	}
	return latest != "" && nilValue
}

// guardedOperationsAreExclusive recognizes only contradictory branch edges
// already represented by the front. Besides opposite edges of one branch, it
// admits equal edges when the later branch reads the exact path after its last
// intervening write was that path's directly lowered `not` expression.
func guardedOperationsAreExclusive(operations []equation.Equation, left, right []equation.Guard) bool {
	for _, leftGuard := range left {
		leftBranch, leftEdge, leftOK := branchGuardEdge(leftGuard)
		if !leftOK {
			continue
		}
		for _, rightGuard := range right {
			rightBranch, rightEdge, rightOK := branchGuardEdge(rightGuard)
			if !rightOK {
				continue
			}
			if leftBranch == rightBranch && leftEdge != rightEdge {
				return true
			}
			if leftEdge == rightEdge && directlyInvertedBranchConditions(operations, leftBranch, rightBranch) {
				return true
			}
		}
	}
	return false
}

func branchGuardEdge(guard equation.Guard) (string, string, bool) {
	parts := strings.Split(string(guard.Encoding), "/")
	if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || (parts[3] != "true" && parts[3] != "false") {
		return "", "", false
	}
	return parts[2], parts[3], true
}

func directlyInvertedBranchConditions(operations []equation.Equation, earlier, later string) bool {
	if earlier == later {
		return false
	}
	earlierCondition, earlierOK := branchConditionTerm(operations, earlier)
	laterCondition, laterOK := branchConditionTerm(operations, later)
	if !earlierOK || !laterOK || !bytes.Equal(earlierCondition, laterCondition) {
		return false
	}
	return latestBranchConditionMutationIsNot(operations, earlierCondition, earlier, later)
}

func branchConditionTerm(operations []equation.Equation, target string) ([]byte, bool) {
	for _, operation := range operations {
		if operation.Occurrence.Kind != "branch-relations" || operation.Target.Name != target {
			continue
		}
		if condition, found := artifactOperand(operation.Operands, "condition"); found {
			return condition, true
		}
		for _, operand := range operation.Operands {
			if operand.Role != "predicate" {
				continue
			}
			var predicate branchPredicateWire
			encoded := operand.Term.Encoding
			if !strings.HasPrefix(string(encoded), branchPredicatePrefix) || json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate) != nil || predicate.Path == "" {
				return nil, false
			}
			return []byte("path/" + predicate.Path), true
		}
		return nil, false
	}
	return nil, false
}

func latestBranchConditionMutationIsNot(operations []equation.Equation, target []byte, after, before string) bool {
	latest := ""
	inverted := false
	for _, operation := range operations {
		if operation.Target.Name <= after || operation.Target.Name >= before || operation.Target.Name <= latest {
			continue
		}
		switch operation.Occurrence.Kind {
		case "environment-write":
			writeTarget, hasTarget := artifactOperand(operation.Operands, "target")
			if hasTarget && bytes.Equal(writeTarget, target) {
				latest, inverted = operation.Target.Name, false
			}
		case "expression":
			kind, hasKind := artifactOperand(operation.Operands, "kind")
			operator, hasOperator := artifactOperand(operation.Operands, "operator")
			result, hasResult := artifactOperand(operation.Operands, "result")
			value, hasValue := artifactOperand(operation.Operands, "value")
			if hasKind && hasOperator && hasResult && hasValue && bytes.Equal(result, target) {
				latest = operation.Target.Name
				inverted = bytes.Equal(value, target) && string(kind) == strconv.Itoa(int(wir.OpUnOp)) && string(operator) == strconv.Itoa(int(wir.UnNot))
			}
		}
	}
	return latest != "" && inverted
}

// uncalledDeclaredFormalConcatOperations returns only concat operands whose
// operand is the exact result of an already admitted direct formal method
// call.  It follows the front's closed apply -> call-results -> local-write
// chain, so an arbitrary local or a separate optional value cannot obtain an
// allocation-time warning merely because the body also calls a formal method.
func uncalledDeclaredFormalConcatOperations(operations []equation.Equation, memberCalls map[string]bool) map[string]bool {
	results := make(map[string]bool)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		application, found := artifactOperand(operation.Operands, "application")
		if !found || !memberCalls[string(application)] {
			continue
		}
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "result-") {
				results[string(operand.Term.Encoding)] = true
			}
		}
	}
	paths := make(map[string]bool)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		value, hasValue := artifactOperand(operation.Operands, "value")
		target, hasTarget := artifactOperand(operation.Operands, "target")
		if hasValue && hasTarget && results[string(value)] {
			paths[string(target)] = true
		}
	}
	concat := make(map[string]bool)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "expression" {
			continue
		}
		operands := 0
		directOperands := make([]string, 0, 1)
		for _, operand := range operation.Operands {
			if _, valid := concatOperandIndex(operand.Role); !valid {
				continue
			}
			operands++
			if paths[string(operand.Term.Encoding)] {
				directOperands = append(directOperands, operand.Role)
			}
		}
		if operands >= 2 {
			for _, operand := range directOperands {
				concat[operation.Target.Name+"/"+operand] = true
			}
		}
	}
	return concat
}

// uncalledDeclaredFormalOrderedComparisonOperations admits only an ordered
// comparison between one declared formal and a closed numeric literal. Both
// facts are already published at the declaration boundary, so this adds no
// inference for an opaque operand or a branch-selected value.
func uncalledDeclaredFormalOrderedComparisonOperations(child front.Compilation, formals map[string]bool) map[string]bool {
	allowed := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if !declaredOrderedComparisonExpression(operation) {
			continue
		}
		left, hasLeft := artifactOperand(operation.Operands, "left")
		right, hasRight := artifactOperand(operation.Operands, "right")
		if !hasLeft || !hasRight {
			continue
		}
		leftFormal := formals[string(left)] && uncalledDeclaredFormalValue(child, left)
		rightFormal := formals[string(right)] && uncalledDeclaredFormalValue(child, right)
		leftNumber := strings.HasPrefix(string(left), "scalar/number/")
		rightNumber := strings.HasPrefix(string(right), "scalar/number/")
		if leftFormal && rightNumber || rightFormal && leftNumber {
			allowed[operation.Target.Name] = true
		}
	}
	return allowed
}

func declaredOrderedComparisonExpression(operation equation.Equation) bool {
	if operation.Occurrence.Kind != "expression" {
		return false
	}
	kindValue, hasKind := artifactOperand(operation.Operands, "kind")
	operatorValue, hasOperator := artifactOperand(operation.Operands, "operator")
	if !hasKind || !hasOperator || string(kindValue) != strconv.Itoa(int(wir.OpBinOp)) {
		return false
	}
	operatorID, err := strconv.Atoi(string(operatorValue))
	if err != nil {
		return false
	}
	_, ordered := orderedComparisonOperator(wir.Operator(operatorID))
	return ordered
}

// uncalledDeclaredFormalMemberWrite identifies a static member write whose
// container is an exact declared formal. The declaration alone establishes
// whether that container admits nil, so the write's non-nil requirement is
// decidable at the allocation boundary.
func uncalledDeclaredFormalMemberWrite(operation equation.Equation, formals map[string]bool) bool {
	for _, operand := range operation.Operands {
		if operand.Role == "write-container" && formals[string(operand.Term.Encoding)] {
			return true
		}
	}
	return false
}

// uncalledDeclaredFormalAssignment identifies an annotation claim from an
// exact declared formal or its static member path. The value has the same
// declaration-owned witness as an admitted missing-member read; arbitrary
// locals and branch-only refinements remain demand-driven.
func uncalledDeclaredFormalAssignment(child front.Compilation, operation equation.Equation, formals map[string]bool) bool {
	if operation.Occurrence.Kind != "claim" {
		return false
	}
	var value []byte
	assignment := false
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "value":
			value = operand.Term.Encoding
		case "kind":
			assignment = string(operand.Term.Encoding) == "claim-kind/3"
		}
	}
	if !assignment || len(value) == 0 {
		return false
	}
	if formals[string(value)] {
		// A direct boundary assignment is publication-safe only before control
		// flow can refine that formal. Guarded direct assignments retain the
		// ordinary demand-driven path, where the branch proof is caller-owned.
		if len(operation.Guards) != 0 {
			return false
		}
		return uncalledDeclaredFormalValue(child, value)
	}
	return uncalledDeclaredFormalMemberRead(child, operation, formals)
}

// uncalledDeclaredFormalMemberRead identifies an annotation claim whose value
// is a static member lens of a declaration-owned formal. The child entry may
// evaluate that narrow shape to publish an absent-member diagnostic. A branch
// alone is not an obligation: its runtime predicate can be unknown even when
// the formal's declared type is complete.
func uncalledDeclaredFormalMemberRead(child front.Compilation, operation equation.Equation, formals map[string]bool) bool {
	if operation.Occurrence.Kind != "claim" {
		return false
	}
	value, found := artifactOperand(operation.Operands, "value")
	if !found {
		return false
	}
	root, suffix, member := tableAddress(value)
	if !member || !formals[string(root)] || !uncalledDeclaredFormalValue(child, root) {
		return false
	}
	segments, static := segment.ParseFormattedSegments(suffix)
	return static && len(segments) != 0
}

// uncalledDeclaredFormalValue accepts only an exact, non-gradual boundary
// formal. Its type is emitted by the front end and encoded as an entry seed;
// no call result, branch result, or inferred local is used as authority.
func uncalledDeclaredFormalValue(child front.Compilation, value []byte) bool {
	for _, parameter := range child.Boundary.Parameters {
		if boundaryTerm(parameter.Symbol) != string(value) || parameter.Type == 0 {
			continue
		}
		declared := unwrap.Alias(child.WIR.Type(parameter.Type))
		return declared != nil && declared.Kind() != kind.Any
	}
	return false
}

// uncalledStaticAssignmentBoundary admits a straight-line lexical body whose
// only externally callable values are its own captured local closures.  The
// entry contains ordinary declared formal witnesses; captured closures are
// supplied separately by uncalledChildEntry, which refuses an absent or opaque
// capability.  This is deliberately narrower than a general declaration-only
// execution: branch-selected facts, indexing, and channel operations still
// require a caller-owned partition.
//
// Its sole publication consumer is an assignment claim in this same body.  A
// direct local call must produce an owned result slot; the declaration-only
// caller never imports a guard effect from a no-result helper.
func uncalledStaticAssignmentBoundary(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Parameters) == 0 || len(child.Boundary.Captures) == 0 {
		return nil, false
	}
	captures := make(map[string]bool, len(child.Boundary.Captures))
	for _, capture := range child.Boundary.Captures {
		captures[boundaryTerm(capture.Symbol)] = true
	}
	publishedStdlibCalls := uncalledPublishedStdlibCalls(child.Artifact.Equations)
	// A result slot is owned only by a captured local call's exact
	// apply -> call-results -> write chain. A subsequent unguarded method call
	// may consume that slot's declared optional contract, but no other local
	// can enter this declaration-only boundary.
	capturedCalls := make(map[string]bool)
	callResults := make(map[string]bool)
	callPaths := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "apply":
			callee, arity := "", ""
			for _, operand := range operation.Operands {
				if operand.Role == "callee" {
					callee = string(operand.Term.Encoding)
				}
				if operand.Role == "result-arity" {
					arity = string(operand.Term.Encoding)
				}
			}
			if captures[callee] && arity != "" && arity != "0" {
				capturedCalls["call/"+operation.Target.Name] = true
			}
		case "call-results":
			application, found := artifactOperand(operation.Operands, "application")
			if !found || !capturedCalls[string(application)] {
				continue
			}
			for _, operand := range operation.Operands {
				if strings.HasPrefix(operand.Role, "result-") && operand.Role != "result-display" {
					callResults[string(operand.Term.Encoding)] = true
				}
			}
		case "environment-write":
			value, hasValue := artifactOperand(operation.Operands, "value")
			target, hasTarget := artifactOperand(operation.Operands, "target")
			if hasValue && hasTarget && callResults[string(value)] {
				callPaths[string(target)] = true
			}
		}
	}
	hasAssignment, hasResultCall, hasOptionalMethod := false, false, false
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "claim":
			for _, operand := range operation.Operands {
				if operand.Role == "kind" && string(operand.Term.Encoding) == "claim-kind/3" {
					hasAssignment = true
				}
			}
		case "apply":
			callee, receiver := "", ""
			resultArity, method := "", ""
			for _, operand := range operation.Operands {
				if operand.Role == "callee" {
					callee = string(operand.Term.Encoding)
				}
				if operand.Role == "receiver" {
					receiver = string(operand.Term.Encoding)
				}
				if operand.Role == "method" {
					method = string(operand.Term.Encoding)
				}
				if operand.Role == "result-arity" {
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
				if captures[callee] {
					continue
				}
				// A no-result static member call cannot supply a value or a
				// branch fact to this declaration-only evaluation.  It is safe
				// only when the receiver is an already-captured table; the entry
				// transport below requires that table's sealed identity and member
				// cells rather than reconstructing a callable from source spelling.
				if !uncalledStaticCapturedMemberCall(callee, captures) {
					return nil, false
				}
				continue
			}
			if !captures[callee] && !publishedStdlibCalls["call/"+operation.Target.Name] {
				return nil, false
			}
			hasResultCall = true
		case "branch-relations", "dynamic-index-read", "channel-select":
			return nil, false
		}
	}
	if (!hasAssignment && !hasOptionalMethod && !hasResultCall) || (!hasResultCall && !hasCapturedNoResultCall(child.Artifact.Equations, captures)) {
		return nil, false
	}
	seeds := make([]entrySeed, 0, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			return nil, false
		}
		declared := child.WIR.Type(parameter.Type)
		value, ok := shapefact.EncodeTarget(declared)
		if !ok || declared == nil {
			return nil, false
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(parameter.Symbol), Value: value})
	}
	return seeds, true
}

// uncalledStaticCapturedReturnBoundary admits one parameter-free return
// contract only when its sole captured dependency is an already-published
// local closure and every call in the body targets that capability. The child
// entry transports the capability itself; no callable, argument, result, or
// return value is reconstructed from syntax. Branches, dynamic reads, and
// external calls remain demand-driven.
func (l *lexicalEvaluator) uncalledStaticCapturedReturnBoundary(child front.Compilation, partition equation.Partition) bool {
	if l == nil || child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Parameters) != 0 || len(child.Boundary.Captures) != 1 || len(child.Boundary.DeclaredReturns) != 1 {
		return false
	}
	capture := boundaryTerm(child.Boundary.Captures[0].Symbol)
	if _, found := closureHandleFor([]byte(capture), partition); !found {
		return false
	}
	called := false
	applications := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry", "environment-write", "publication":
			continue
		case "apply":
			callee, hasCallee := artifactOperand(operation.Operands, "callee")
			arity, hasArity := artifactOperand(operation.Operands, "result-arity")
			if !hasCallee || !hasArity || string(callee) != capture || string(arity) != "1" {
				return false
			}
			called = true
			applications["call/"+operation.Target.Name] = true
		case "external-call", "call-results":
			application, found := artifactOperand(operation.Operands, "application")
			if !found || !applications[string(application)] {
				return false
			}
		default:
			return false
		}
	}
	return called
}

// uncalledStaticArithmeticBoundary admits a parameter-free closure only when
// its entire call graph segment is already closed: imported member calls use a
// project-published import authority and the local call targets a captured
// closure whose unannotated formal is directly consumed by numeric arithmetic.
// The body has no branches, dynamic lookup, or writes beyond ordinary call
// result bindings, so evaluating it at allocation time cannot invent a path
// condition or a caller-owned heap fact.
func (l *lexicalEvaluator) uncalledStaticArithmeticBoundary(child front.Compilation, partition equation.Partition) bool {
	if l == nil || child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Parameters) != 0 || len(child.Boundary.Captures) == 0 {
		return false
	}
	captures := make(map[string]bool, len(child.Boundary.Captures))
	arithmeticCallees := make(map[string]bool)
	for _, capture := range child.Boundary.Captures {
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
		if _, imported := l.importedAuthority(term); !imported {
			return false
		}
	}
	foundArithmeticCall := false
	applications := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry", "publication", "environment-write":
			continue
		case "apply":
			callee, found := artifactOperand(operation.Operands, "callee")
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
			if _, imported := l.importedAuthority(string(root)); !imported {
				return false
			}
			applications["call/"+operation.Target.Name] = true
		case "external-call", "call-results":
			application, found := artifactOperand(operation.Operands, "application")
			if !found || !applications[string(application)] {
				return false
			}
		default:
			return false
		}
	}
	return foundArithmeticCall
}

func unannotatedArithmeticFormal(child front.Compilation) bool {
	if child.WIR == nil || child.Cyclic != nil {
		return false
	}
	formals := make(map[string]bool)
	for _, parameter := range child.Boundary.Parameters {
		if !parameter.Vararg && parameter.Type == 0 {
			formals[boundaryTerm(parameter.Symbol)] = true
		}
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "expression" {
			continue
		}
		expressionKind, hasKind := artifactOperand(operation.Operands, "kind")
		operator, hasOperator := artifactOperand(operation.Operands, "operator")
		if !hasKind || !hasOperator || string(expressionKind) != strconv.Itoa(int(wir.OpBinOp)) {
			continue
		}
		operatorID, err := strconv.Atoi(string(operator))
		if err != nil {
			continue
		}
		operatorText, supported := expressionOperatorText(wir.Operator(operatorID))
		if !supported {
			continue
		}
		result, numeric := typeoperator.BinaryOp(typ.Number, operatorText, typ.Number)
		if !numeric || result == nil || (result.Kind() != kind.Number && result.Kind() != kind.Integer) {
			continue
		}
		left, leftFound := artifactOperand(operation.Operands, "left")
		right, rightFound := artifactOperand(operation.Operands, "right")
		if leftFound && formals[string(left)] || rightFound && formals[string(right)] {
			return true
		}
	}
	return false
}

// uncalledPublishedStdlibCalls returns exact apply applications whose
// external-call companion carries a standard-library provider publication.
// The declaration-only child entry consumes that registry contract; it does
// not recover a callable from source spelling or manufacture a result type.
func uncalledPublishedStdlibCalls(operations []equation.Equation) map[string]bool {
	out := make(map[string]bool)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "external-call" {
			continue
		}
		application, hasApplication := artifactOperand(operation.Operands, "application")
		provider, hasProvider := artifactOperand(operation.Operands, "provider")
		if !hasApplication || !hasProvider {
			continue
		}
		name := providerName(provider)
		if name == "" {
			continue
		}
		if _, found := signaturelookup.StdlibResultSlot(name, 0); found {
			out[string(application)] = true
		}
	}
	return out
}

// uncalledStaticOptionalMethodDiagnostic admits only the call failure owned by
// an unguarded method application that the static boundary already proved to
// consume a captured call result. It cannot publish a result, a refinement, or
// a diagnostic from an unrelated method call.
func uncalledStaticOptionalMethodDiagnostic(artifact equation.Artifact, diagnostic equation.Fact) bool {
	if !strings.HasPrefix(diagnostic.Key, "type.call.direct.not_callable/") && !strings.HasPrefix(diagnostic.Key, "type.call.optional_receiver/") {
		return false
	}
	name := diagnosticOperationName(diagnostic.Key)
	for _, operation := range artifact.Equations {
		if operation.Target.Name != name || operation.Occurrence.Kind != "apply" || len(operation.Guards) != 0 {
			continue
		}
		receiver, hasReceiver := artifactOperand(operation.Operands, "receiver")
		method, hasMethod := artifactOperand(operation.Operands, "method")
		return hasReceiver && hasMethod && strings.HasPrefix(string(receiver), "path/") && strings.HasPrefix(string(method), "method/")
	}
	return false
}

// uncalledStaticCapturedMemberCall recognizes the no-result counterpart to a
// direct captured closure call.  The callee must be one static member below a
// captured table root; dynamic indexing and deeper paths remain caller-driven.
func uncalledStaticCapturedMemberCall(callee string, captures map[string]bool) bool {
	root, suffix, member := tableAddress([]byte(callee))
	if !member || !captures[string(root)] {
		return false
	}
	segments, static := segment.ParseFormattedSegments(suffix)
	return static && len(segments) == 1 && (segments[0].Kind == segment.SegmentField || segments[0].Kind == segment.SegmentIndexString)
}

func hasCapturedNoResultCall(operations []equation.Equation, captures map[string]bool) bool {
	for _, operation := range operations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		callee, hasCallee := artifactOperand(operation.Operands, "callee")
		arity, hasArity := artifactOperand(operation.Operands, "result-arity")
		if hasCallee && hasArity && string(arity) == "0" &&
			(captures[string(callee)] || uncalledStaticCapturedMemberCall(string(callee), captures)) {
			return true
		}
	}
	return false
}

// uncalledStaticAssignmentDiagnostic retains a declaration-only assignment
// diagnostic only when no no-result call in the same body consumes its source
// term. A no-result call has no closed return slot, so its assertion or
// validation effect cannot be replayed without a real invocation. An
// independent declared formal remains a complete existing boundary witness.
func (l *lexicalEvaluator) uncalledStaticAssignmentDiagnostic(artifact equation.Artifact, key string, partition equation.Partition) bool {
	const prefix = "type.assignment/"
	if !strings.HasPrefix(key, prefix) {
		return false
	}
	operation := strings.TrimPrefix(key, prefix)
	var source string
	for _, candidate := range artifact.Equations {
		if candidate.Target.Name != operation || candidate.Occurrence.Kind != "claim" {
			continue
		}
		value, found := artifactOperand(candidate.Operands, "value")
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
		arity, hasArity := artifactOperand(candidate.Operands, "result-arity")
		if !hasArity || string(arity) != "0" {
			continue
		}
		for _, operand := range candidate.Operands {
			if strings.HasPrefix(operand.Role, "argument-") && string(operand.Term.Encoding) == source {
				callee, hasCallee := artifactOperand(candidate.Operands, "callee")
				if hasCallee && (l.uncalledCapturedHelperHasOnlyGuardedValidation(callee, partition) ||
					l.uncalledCapturedHelperHasOnlyGuardedNonValidationEffect(callee, partition)) {
					continue
				}
				return false
			}
		}
	}
	return true
}

// uncalledCapturedHelperHasOnlyGuardedValidation admits one very limited
// no-result helper shape: its only application is a validation operation that
// itself is guarded by the helper's branch. The call therefore cannot publish
// an unconditional postcondition for the caller's argument.
func (l *lexicalEvaluator) uncalledCapturedHelperHasOnlyGuardedValidation(callee []byte, partition equation.Partition) bool {
	if l == nil {
		return false
	}
	handle, found := closureHandleFor(callee, partition)
	if !found {
		return false
	}
	child, found := l.byPrototype[handle.Prototype]
	if !found || child.Cyclic != nil {
		return false
	}
	foundValidation := false
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		if len(operation.Guards) == 0 {
			return false
		}
		hasCheck := false
		for _, operand := range operation.Operands {
			hasCheck = hasCheck || operand.Role == "check"
		}
		if !hasCheck {
			return false
		}
		foundValidation = true
	}
	return foundValidation
}

// uncalledCapturedHelperHasOnlyGuardedNonValidationEffect recognizes a closed
// local helper whose only action is a guarded call to a registered stdlib
// provider, without a check operand. Such a helper cannot establish a caller
// proof: it has no writes, claims, result slots, captures, or validation call.
// Its provider identity comes from the existing stdlib publication, rather
// than from a source-level callee spelling.
func (l *lexicalEvaluator) uncalledCapturedHelperHasOnlyGuardedNonValidationEffect(callee []byte, partition equation.Partition) bool {
	if l == nil {
		return false
	}
	handle, found := closureHandleFor(callee, partition)
	if !found {
		return false
	}
	child, found := l.byPrototype[handle.Prototype]
	if !found || child.Cyclic != nil || len(child.Boundary.Captures) != 0 {
		return false
	}
	applications := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry", "branch-relations":
			continue
		case "apply":
			arity, hasArity := artifactOperand(operation.Operands, "result-arity")
			if !hasArity || string(arity) != "0" || len(operation.Guards) == 0 {
				return false
			}
			for _, operand := range operation.Operands {
				if operand.Role == "check" {
					return false
				}
			}
			applications["call/"+operation.Target.Name] = true
		case "external-call":
			application, hasApplication := artifactOperand(operation.Operands, "application")
			provider, hasProvider := artifactOperand(operation.Operands, "provider")
			if !hasApplication || !applications[string(application)] || !hasProvider {
				return false
			}
			if !nonValidatingNoResultStdlibProvider(provider) {
				return false
			}
		case "call-results":
			application, hasApplication := artifactOperand(operation.Operands, "application")
			if !hasApplication || !applications[string(application)] {
				return false
			}
		default:
			return false
		}
	}
	return len(applications) != 0
}

// nonValidatingNoResultStdlibProvider accepts only an existing, closed stdlib
// contract that cannot return, mutate, retain, dispatch, or publish control
// flow. BorrowAll is the sole permitted effect: it consumes arguments without
// producing a postcondition. This keeps an error-like zero-result statement
// from being mistaken for a no-op helper.
func nonValidatingNoResultStdlibProvider(provider []byte) bool {
	signature, published := (signaturelookup.Source{IncludeStdlib: true}).LookupView(providerName(provider))
	if !published || signature.Type == nil || len(signature.Type.Returns) != 0 || !signature.Effect.IsClosed() {
		return false
	}
	for _, label := range signature.Effect.Labels {
		if _, borrowsOnly := effect.NormalizeLabel(label).(ownership.BorrowAll); !borrowsOnly {
			return false
		}
	}
	return true
}

func (l *lexicalEvaluator) uncalledStaticCapturedCallsAreGuardedValidation(child front.Compilation, partition equation.Partition) bool {
	captures := make(map[string]bool, len(child.Boundary.Captures))
	for _, capture := range child.Boundary.Captures {
		captures[boundaryTerm(capture.Symbol)] = true
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		callee, hasCallee := artifactOperand(operation.Operands, "callee")
		arity, hasArity := artifactOperand(operation.Operands, "result-arity")
		if hasCallee && hasArity && string(arity) == "0" && captures[string(callee)] &&
			!l.uncalledCapturedHelperHasOnlyGuardedValidation(callee, partition) &&
			!l.uncalledCapturedHelperHasOnlyGuardedNonValidationEffect(callee, partition) {
			return false
		}
	}
	return true
}

// uncalledStaticResultCallDiagnostic retains only an argument contract on a
// captured local callable when that exact argument is the local write of an
// already-published stdlib result. The apply/external-call/call-results/write
// chain prevents unrelated local calls or source-shaped names from entering a
// declaration-only child evaluation.
func uncalledStaticResultCallDiagnostic(artifact equation.Artifact, key string) bool {
	if !strings.HasPrefix(key, "type.call.direct.argument_type/") {
		return false
	}
	operationName := diagnosticOperationName(key)
	published := uncalledPublishedStdlibCalls(artifact.Equations)
	resultPaths := uncalledPublishedResultPaths(artifact.Equations, published)
	for _, operation := range artifact.Equations {
		if operation.Target.Name != operationName || operation.Occurrence.Kind != "apply" {
			continue
		}
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "argument-") && resultPaths[string(operand.Term.Encoding)] {
				return true
			}
		}
	}
	return false
}

// uncalledDeclaredProviderResultDiagnostic retains an assignment or argument
// contract only when its source is the local write of a standard-library
// result from a call that the declaration-only boundary already admits. The
// exact apply -> external-call -> call-results -> write chain prevents an
// opaque call or source spelling from entering the private evaluation.
func uncalledDeclaredProviderResultDiagnostic(child front.Compilation, key string) bool {
	if !strings.HasPrefix(key, "type.assignment/") && !strings.HasPrefix(key, "type.call.direct.argument_type/") {
		return false
	}
	formals := make(map[string]bool, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		formals[boundaryTerm(parameter.Symbol)] = true
	}
	applications := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind == "apply" && uncalledDeclaredStdlibCall(child.Artifact.Equations, operation, formals) {
			applications["call/"+operation.Target.Name] = true
		}
	}
	paths := uncalledPublishedResultPaths(child.Artifact.Equations, applications)
	if len(paths) == 0 {
		return false
	}
	operationName := strings.TrimPrefix(key, "type.assignment/")
	if operationName == key {
		operationName = diagnosticOperationName(key)
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Target.Name != operationName {
			continue
		}
		switch operation.Occurrence.Kind {
		case "claim":
			value, found := artifactOperand(operation.Operands, "value")
			return found && paths[string(value)]
		case "apply":
			for _, operand := range operation.Operands {
				if _, err := callArgumentIndex(operand.Role); err == nil && paths[string(operand.Term.Encoding)] {
					return true
				}
			}
		}
	}
	return false
}

func uncalledPublishedResultPaths(operations []equation.Equation, applications map[string]bool) map[string]bool {
	results := make(map[string]bool)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		application, found := artifactOperand(operation.Operands, "application")
		if !found || !applications[string(application)] {
			continue
		}
		for _, operand := range operation.Operands {
			if strings.HasPrefix(operand.Role, "result-") {
				results[string(operand.Term.Encoding)] = true
			}
		}
	}
	paths := make(map[string]bool)
	for _, operation := range operations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		value, hasValue := artifactOperand(operation.Operands, "value")
		target, hasTarget := artifactOperand(operation.Operands, "target")
		if hasValue && hasTarget && results[string(value)] {
			paths[string(target)] = true
		}
	}
	return paths
}

// uncalledDeclaredIndexedReadBoundary admits a capture-free indexed read whose
// container is an exact declared formal. RuntimeIndex already publishes the
// selected slot's nilability from that array or map witness; the bounded
// branch facts and a later direct mutation are existing transactions over that
// same witness. Calls, channel operations, and generic iteration still require
// a caller-owned entry.
func uncalledDeclaredIndexedReadBoundary(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 || len(child.Boundary.Parameters) == 0 {
		return nil, false
	}
	formals := make(map[string]entrySeed, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 || parameter.Type == 0 {
			return nil, false
		}
		declared := child.WIR.Type(parameter.Type)
		value, ok := shapefact.EncodeTarget(declared)
		if !ok || declared == nil {
			return nil, false
		}
		term := boundaryTerm(parameter.Symbol)
		formals[term] = entrySeed{Term: term, Value: value}
	}
	hasIndexedRead := false
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "dynamic-index-read":
			container, found := artifactOperand(operation.Operands, "container")
			if _, exactFormal := formals[string(container)]; !found || !exactFormal {
				return nil, false
			}
			hasIndexedRead = true
		case "branch-relations":
			if !uncalledDeclaredIndexedBranch(operation) {
				return nil, false
			}
		case "apply", "external-call", "channel-select", "generic-for":
			return nil, false
		}
	}
	if !hasIndexedRead {
		return nil, false
	}
	seeds := make([]entrySeed, 0, len(formals))
	for _, parameter := range child.Boundary.Parameters {
		seeds = append(seeds, formals[boundaryTerm(parameter.Symbol)])
	}
	return seeds, true
}

// uncalledDeclaredIndexedBranch admits only the numeric bound evidence that
// typedIndexBranchClosure already carries as guarded publications. Other
// branch families can refine arbitrary values, so they remain caller-owned.
func uncalledDeclaredIndexedBranch(operation equation.Equation) bool {
	for _, operand := range operation.Operands {
		predicate, trueEdge, ok := branchEvidencePredicate(equation.BoundOperand{Role: operand.Role, Value: operand.Term.Encoding})
		if !ok || !trueEdge || predicate.Negated {
			continue
		}
		if predicate.Kind == "num-ge" || predicate.Kind == "index-in-range" {
			return true
		}
	}
	return false
}

func artifactOperand(operands []equation.Operand, role string) ([]byte, bool) {
	for _, operand := range operands {
		if operand.Role == role {
			return append([]byte(nil), operand.Term.Encoding...), true
		}
	}
	return nil, false
}

// uncalledDeclaredMemberCall recognizes the narrow call shape that can only
// report a missing declared member. It accepts neither a dynamic index nor a
// receiver whose identity was not supplied by the declaration-owned entry.
func uncalledDeclaredMemberCall(child front.Compilation, operation equation.Equation, formals map[string]bool) bool {
	if hasDeclaredFormalMethodCall(child, operation, formals) {
		return true
	}
	for _, operand := range operation.Operands {
		if operand.Role != "callee" {
			continue
		}
		root, suffix, member := tableAddress(operand.Term.Encoding)
		segments, static := segment.ParseFormattedSegments(suffix)
		return member && formals[string(root)] && static && len(segments) == 1 && (segments[0].Kind == segment.SegmentField || segments[0].Kind == segment.SegmentIndexString)
	}
	return false
}

// declaredEntryClosedTerms computes the terms a declaration-only entry already
// closes. The entry seeds every declared formal and this boundary admits no
// captures, so a local first written from an already-closed value is itself
// closed: nothing outside the declaration contributes to it. The set is a
// least fixed point over the body's own writes; a local reached from a global,
// an unwritten path, or any other open source stays outside it.
func declaredEntryClosedTerms(operations []equation.Equation, formals map[string]bool) map[string]bool {
	closed := make(map[string]bool, len(formals))
	for formal := range formals {
		closed[formal] = true
	}
	closedTerm := func(term string) bool {
		if strings.HasPrefix(term, "scalar/") || shapefact.IsTable([]byte(term)) {
			return true
		}
		if closed[term] {
			return true
		}
		root := term
		if trimmed, ok := strings.CutPrefix(term, "path/"); ok {
			if cut := strings.IndexAny(trimmed, ".["); cut >= 0 {
				root = "path/" + trimmed[:cut]
			}
		}
		return closed[root]
	}
	for changed := true; changed; {
		changed = false
		for _, operation := range operations {
			if operation.Occurrence.Kind != "environment-write" {
				continue
			}
			target, hasTarget := artifactOperand(operation.Operands, "target")
			value, hasValue := artifactOperand(operation.Operands, "value")
			if !hasTarget || !hasValue || closed[string(target)] || !closedTerm(string(value)) {
				continue
			}
			closed[string(target)], changed = true, true
		}
	}
	return closed
}

// uncalledDeclaredStdlibCall identifies only a registered standard-library
// result reached from exact child formals, scalar literals, or locals the
// declaration entry already closes. It is a result provenance check, not an
// admission rule: the declaration boundary still decides independently whether
// the call itself can be evaluated.
func uncalledDeclaredStdlibCall(operations []equation.Equation, apply equation.Equation, formals map[string]bool) bool {
	application := "call/" + apply.Target.Name
	closed := declaredEntryClosedTerms(operations, formals)
	for _, operand := range apply.Operands {
		if _, err := callArgumentIndex(operand.Role); err != nil {
			continue
		}
		argument := string(operand.Term.Encoding)
		if !closed[argument] && !strings.HasPrefix(argument, "scalar/") {
			return false
		}
	}
	for _, operation := range operations {
		if operation.Occurrence.Kind != "external-call" {
			continue
		}
		candidate, hasApplication := artifactOperand(operation.Operands, "application")
		provider, hasProvider := artifactOperand(operation.Operands, "provider")
		if !hasApplication || !hasProvider || string(candidate) != application {
			continue
		}
		_, published := signaturelookup.StdlibResultSlot(providerName(provider), 0)
		return published
	}
	return false
}

// uncalledDeclaredExpandedStdlibCall is the narrow declaration-only admission
// for Lua's open multi-return boundary. A global call remains dormant unless
// its published registry contract has exactly one explicit-any result and the
// front has expanded that result into additional slots. The later slots are
// therefore real conservative contract boundaries, not inferred values.
func uncalledDeclaredExpandedStdlibCall(operations []equation.Equation, apply equation.Equation, formals map[string]bool) bool {
	if !uncalledDeclaredStdlibCall(operations, apply, formals) {
		return false
	}
	resultArity := 0
	for _, operand := range apply.Operands {
		if operand.Role != "result-arity" {
			continue
		}
		value, err := strconv.Atoi(string(operand.Term.Encoding))
		if err != nil || value <= 1 {
			return false
		}
		resultArity = value
		break
	}
	if resultArity <= 1 {
		return false
	}
	application := "call/" + apply.Target.Name
	for _, operation := range operations {
		if operation.Occurrence.Kind != "external-call" {
			continue
		}
		candidate, hasApplication := artifactOperand(operation.Operands, "application")
		provider, hasProvider := artifactOperand(operation.Operands, "provider")
		if hasApplication && hasProvider && string(candidate) == application {
			return providerAnyResult(providerName(provider), 1, resultArity)
		}
	}
	return false
}

// hasDeclaredFormalMethodCall recognizes a colon call whose receiver is an
// exact declared formal.  Its capability can only come from the receiver's
// published boundary type, so it is safe to evaluate solely for a missing
// member or declared-return diagnostic.
func hasDeclaredFormalMethodCall(child front.Compilation, operation equation.Equation, formals map[string]bool) bool {
	if child.WIR == nil || operation.Occurrence.Kind != "apply" {
		return false
	}
	var receiver []byte
	var method string
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "receiver":
			receiver = operand.Term.Encoding
		case "method":
			method, _ = callMethodName(operand.Term.Encoding)
		}
	}
	if method == "" || !formals[string(receiver)] {
		return false
	}
	for _, parameter := range child.Boundary.Parameters {
		if boundaryTerm(parameter.Symbol) != string(receiver) || parameter.Type == 0 {
			continue
		}
		receiverType := child.WIR.Type(parameter.Type)
		if receiverType == nil {
			return false
		}
		if _, provider := signaturelookup.StdlibMethodProvider(receiverType, method); provider {
			return true
		}
		return declaredMethodMissing(receiverType, method)
	}
	return false
}

// uncalledChildEntry closes an allocation-time child entry from the same
// caller partition that allocated it. This allocation-time admission is
// limited to exact local closure captures: an arbitrary captured value can
// participate in later validation, while a sealed local closure contributes
// only its already-published call capability. Unknown or non-callable captures
// therefore leave the child dormant rather than receiving a synthetic entry.
func (l *lexicalEvaluator) uncalledChildEntry(child front.Compilation, formalSeeds []entrySeed, partition equation.Partition, allowTypedCaptures, allowImportedCaptures, includeClosureDependencies, declaredProviderBoundary bool, gradualBoundaryTerms ...[]string) ([]byte, bool, error) {
	seeds := append([]entrySeed(nil), formalSeeds...)
	closureSeeds := make([]entryClosureSeed, 0, len(child.Boundary.Captures))
	for _, capture := range child.Boundary.Captures {
		term := boundaryTerm(capture.Symbol)
		value, known := resolveKnownCurrentValue([]byte(term), partition)
		if !known || isUnknownScalar(value) {
			return nil, false, nil
		}
		handle, found := closureHandleFor([]byte(term), partition)
		if !found {
			if isUnknownScalar(value) {
				return nil, false, nil
			}
			// A require binding can carry its project-selected export type as
			// entry metadata. This is not a reconstructed module value: the
			// exact authority was published when require's result was written.
			// It is admitted only on the typed static paths that already require
			// a complete capture entry.
			if imported, found := l.importedAuthority(term); allowImportedCaptures {
				encoded, encodedOK := shapefact.EncodeTarget(imported)
				if !found || !encodedOK {
					return nil, false, nil
				}
				seeds = append(seeds, entrySeed{Term: term, Value: encoded})
				continue
			}
			// A sealed table capture is transported only for the narrow
			// declaration-only static-member path. Its identity and current
			// member cells are existing parent publications added below; an open
			// table or a table without an identity cannot become a capability.
			if !allowTypedCaptures || !uncalledSealedTableCapture([]byte(term), value, partition) {
				return nil, false, nil
			}
			seeds = append(seeds, entrySeed{Term: term, Value: value})
			continue
		}
		captured, found := l.byPrototype[handle.Prototype]
		if !found {
			return nil, false, nil
		}
		// A closure handle is an existing capability publication. The static
		// declaration boundary may carry that handle even when its ordinary
		// scalar lane remains Top; no table shape or callable is reconstructed.
		if isUnknownScalar(value) && !allowTypedCaptures {
			return nil, false, nil
		}
		if _, explicitAny := uncalledExplicitAnyBoundary(captured); !explicitAny && !allowTypedCaptures {
			if !l.uncalledTypePredicateCaptureBoundary(child, term, handle) {
				return nil, false, nil
			}
		}
		seeds = append(seeds, entrySeed{Term: term, Value: value})
		closureSeeds = append(closureSeeds, entryClosureSeed{Term: term, Handle: handle})
	}
	if includeClosureDependencies {
		seen := make(map[string]bool, len(seeds))
		for _, seed := range seeds {
			seen[seed.Term] = true
		}
		for index := 0; index < len(closureSeeds); index++ {
			for _, capture := range closureSeeds[index].Handle.Captures {
				if strings.HasPrefix(capture, "scalar/") || seen[capture] {
					continue
				}
				value, known := resolveKnownCurrentValue([]byte(capture), partition)
				if !known || isUnknownScalar(value) {
					return nil, false, nil
				}
				seen[capture] = true
				seeds = append(seeds, entrySeed{Term: capture, Value: value})
				if handle, found := closureHandleFor([]byte(capture), partition); found {
					if _, admitted := l.byPrototype[handle.Prototype]; !admitted {
						return nil, false, nil
					}
					closureSeeds = append(closureSeeds, entryClosureSeed{Term: capture, Handle: handle})
				}
			}
		}
	}
	boundarySeeds := append([]entrySeed(nil), seeds...)
	seeds = append(seeds, childEntryDescendantSeeds(boundarySeeds, partition)...)
	memberClosureSeeds := childEntryMemberClosureSeeds(seeds, nil, partition)
	tableIdentitySeeds := tableIdentitySeedsForEntry(seeds, partition)
	memberCellSeeds := memberCellSeedsForEntry(seeds, partition)
	var entry []byte
	var err error
	if declaredProviderBoundary {
		entry, err = encodeDeclaredChildEntryWithCapabilities(seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds)
	} else {
		entry, err = encodeChildEntryWithCapabilities(seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, gradualBoundaryTerms...)
	}
	if err != nil {
		return nil, false, err
	}
	return entry, true, nil
}

// uncalledTypePredicateCaptureBoundary recognizes the closed two-result path
// through a local wrapper around a front-published T:is(value) relation. The
// enclosing child must consume the exact captured helper, write both result
// slots, and guard a strict claim with the helper's error slot. This admits no
// arbitrary uncalled helper: any extra operation, an open result tuple, or a
// missing predicate target leaves the child dormant.
func (l *lexicalEvaluator) uncalledTypePredicateCaptureBoundary(child front.Compilation, capture string, handle closureHandle) bool {
	if l == nil || child.Cyclic != nil || capture == "" {
		return false
	}
	helper, found := l.byPrototype[handle.Prototype]
	if !found || !uncalledTypePredicateHelper(helper) {
		return false
	}
	application, valueResult, errorResult := "", "", ""
	valuePath, errorPath := "", ""
	branch := ""
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry":
			continue
		case "apply":
			callee, hasCallee := artifactOperand(operation.Operands, "callee")
			arity, hasArity := artifactOperand(operation.Operands, "result-arity")
			if application != "" || !hasCallee || string(callee) != capture || !hasArity || string(arity) != "2" {
				return false
			}
			application = "call/" + operation.Target.Name
		case "external-call":
			candidate, found := artifactOperand(operation.Operands, "application")
			if !found || application == "" || string(candidate) != application {
				return false
			}
		case "call-results":
			candidate, found := artifactOperand(operation.Operands, "application")
			if !found || application == "" || string(candidate) != application || valueResult != "" || errorResult != "" {
				return false
			}
			value, hasValue := artifactOperand(operation.Operands, "result-00000000")
			err, hasError := artifactOperand(operation.Operands, "result-00000001")
			if !hasValue || !hasError {
				return false
			}
			valueResult, errorResult = string(value), string(err)
		case "environment-write":
			target, hasTarget := artifactOperand(operation.Operands, "target")
			value, hasValue := artifactOperand(operation.Operands, "value")
			if !hasTarget || !hasValue {
				return false
			}
			switch string(value) {
			case valueResult:
				if valuePath != "" {
					return false
				}
				valuePath = string(target)
			case errorResult:
				if errorPath != "" {
					return false
				}
				errorPath = string(target)
			case valuePath:
				// This is the strict assignment's source write. The following
				// claim verifies its target and branch guard below.
			default:
				return false
			}
		case "branch-relations":
			if branch != "" || errorPath == "" || !uncalledNotNilBranch(operation, errorPath) {
				return false
			}
			branch = operation.Target.Name
		case "claim":
			value, hasValue := artifactOperand(operation.Operands, "value")
			if branch == "" || !hasValue || string(value) != valuePath || !hasGuardEncoding(operation.Guards, "front/branch/"+branch+"/true") {
				return false
			}
		default:
			return false
		}
	}
	return application != "" && valueResult != "" && errorResult != "" && valuePath != "" && errorPath != "" && branch != ""
}

func uncalledTypePredicateHelper(child front.Compilation) bool {
	if child.Cyclic != nil || child.WIR == nil || len(child.Boundary.Captures) != 0 || len(child.Boundary.Parameters) != 1 {
		return false
	}
	application, result := "", ""
	hasTarget, hasReturn := false, false
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "entry":
			continue
		case "apply":
			arity, found := artifactOperand(operation.Operands, "result-arity")
			if application != "" || !found || string(arity) != "1" {
				return false
			}
			application = "call/" + operation.Target.Name
		case "external-call":
			candidate, found := artifactOperand(operation.Operands, "application")
			if !found || application == "" || string(candidate) != application {
				return false
			}
		case "call-results":
			candidate, found := artifactOperand(operation.Operands, "application")
			value, hasValue := artifactOperand(operation.Operands, "result-00000000")
			target, hasTargetOperand := artifactOperand(operation.Operands, "type-predicate-error-target")
			if !found || application == "" || string(candidate) != application || !hasValue || result != "" || !hasTargetOperand {
				return false
			}
			if _, valid := shapefact.DecodeTarget(target); !valid {
				return false
			}
			result, hasTarget = string(value), true
		case "publication":
			value, found := artifactOperand(operation.Operands, "return-value-00000000")
			if hasReturn || !found || result == "" || string(value) != result {
				return false
			}
			hasReturn = true
		default:
			return false
		}
	}
	return application != "" && result != "" && hasTarget && hasReturn
}

func uncalledNotNilBranch(operation equation.Equation, errorPath string) bool {
	for _, operand := range operation.Operands {
		predicate, trueEdge, ok := branchEvidencePredicate(equation.BoundOperand{Role: operand.Role, Value: operand.Term.Encoding})
		if ok && trueEdge && !predicate.Negated && predicate.Kind == "not-nil" && "path/"+predicate.Path == errorPath {
			return true
		}
	}
	return false
}

// publishUncalledFalseEdgeAnyAssignment admits one declaration-owned
// obligation from an otherwise dormant local helper body. The front must have
// published the complete chain: an explicit-any formal, an exact captured
// read-only local predicate call, its branch result, and a strict assignment
// guarded by that branch's false edge. A local predicate's false result is not
// a type proof unless the front published a relation for it, so this existing
// false path remains possible and the assignment cannot be accepted.
func (l *lexicalEvaluator) publishUncalledFalseEdgeAnyAssignment(closure *equation.OutputClosure, child front.Compilation, captures []string, partition equation.Partition) {
	if l == nil || closure == nil || child.Cyclic != nil {
		return
	}
	formals, explicitAny := uncalledExplicitAnyBoundary(child)
	if !explicitAny || len(formals) == 0 || len(captures) != len(child.Boundary.Captures) {
		return
	}
	anyFormal := make(map[string]bool, len(formals))
	for _, seed := range formals {
		anyFormal[seed.Term] = true
	}
	readOnlyCapture := make(map[string]bool, len(captures))
	for _, capture := range captures {
		handle, found := closureHandleFor([]byte(capture), partition)
		if !found {
			continue
		}
		candidate, found := l.byPrototype[handle.Prototype]
		if found && uncalledReadOnlyClosure(candidate) {
			readOnlyCapture[capture] = true
		}
	}
	if len(readOnlyCapture) == 0 {
		return
	}
	callResultCallee := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		callee, hasCallee := artifactOperand(operation.Operands, "callee")
		if !hasCallee || !readOnlyCapture[string(callee)] {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "result-00000000" {
				callResultCallee[string(operand.Term.Encoding)] = true
			}
		}
	}
	for _, branch := range child.Artifact.Equations {
		if branch.Occurrence.Kind != "branch-relations" {
			continue
		}
		condition, hasCondition := artifactOperand(branch.Operands, "condition")
		if !hasCondition || !callResultCallee[string(condition)] {
			continue
		}
		for _, operand := range branch.Operands {
			if operand.Role == "predicate" {
				return
			}
		}
		falseEdge := "front/branch/" + branch.Target.Name + "/false"
		for _, claim := range child.Artifact.Equations {
			if claim.Occurrence.Kind != "claim" || !hasGuardEncoding(claim.Guards, falseEdge) {
				continue
			}
			source, hasSource := artifactOperand(claim.Operands, "value")
			targetType, hasType := artifactOperand(claim.Operands, "type")
			if !hasSource || !hasType || !anyFormal[string(source)] || !assignmentTargetRequiresProof(string(targetType)) {
				continue
			}
			display := strings.TrimPrefix(string(source), "path/")
			for _, operand := range claim.Operands {
				if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
					display = string(operand.Term.Encoding)
					break
				}
			}
			shapeTarget, _ := artifactOperand(claim.Operands, "shape-target")
			fact := equation.Fact{Key: "type.assignment/" + claim.Target.Name, Value: []byte(assignmentAnyMismatchMessage(display, string(targetType), shapeTarget))}
			spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, []equation.Fact{fact})
			for _, item := range publishedDiagnostics(child.Artifact, equation.OutputClosure{Diagnostics: []equation.Fact{fact}}, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil) {
				key := "child/" + fmt.Sprintf("%x", child.Body) + "/" + item.Fact.Key
				closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), item.Fact.Value...)})
				if item.Span.Valid() {
					l.diagnosticSpans[key] = item.Span
				}
				item.Fact.Key = key
				l.childPublished[key] = item
			}
			return
		}
	}
}

func hasGuardEncoding(guards []equation.Guard, want string) bool {
	for _, guard := range guards {
		if string(guard.Encoding) == want {
			return true
		}
	}
	return false
}

// uncalledReadOnlyClosure identifies a lexical capability that can accompany
// an existing explicit-any boundary without importing caller state. It is
// deliberately narrower than general uncalled-child admission: the helper has
// no captures and cannot write, mutate, allocate a returned closure, select,
// or iterate. Its operations are therefore only value/condition consumers of
// the exact formal supplied by the enclosing child entry.
func uncalledReadOnlyClosure(child front.Compilation) bool {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 {
		return false
	}
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "environment-write", "path-replacement", "index-mutation", "path-invalidation", "object-materialization", "channel-select", "generic-for":
			return false
		}
	}
	return true
}

func uncalledSealedTableCapture(term, value []byte, partition equation.Partition) bool {
	table, sealed := shapefact.DecodeTable(value)
	if !sealed || !table.Closed {
		return false
	}
	_, identified := tableIdentityForTerm(term, partition)
	return identified
}

// uncalledExplicitAnyDiagnostic retains only strict assignment contracts. A
// runtime claim may validate an any value only along an invoked path, but an
// annotation assignment is a source-owned obligation at the closed boundary.
func uncalledExplicitAnyDiagnostic(artifact equation.Artifact, diagnostic equation.Fact) bool {
	if strings.HasPrefix(diagnostic.Key, "type.assignment/") {
		return true
	}
	name, unproven := strings.CutPrefix(diagnostic.Key, "claim/unproven/")
	if !unproven || typePredicateResultClaim(artifact, name) {
		return false
	}
	for _, operation := range artifact.Equations {
		if operation.Target.Name != name || operation.Occurrence.Kind != "claim" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "kind" {
				return string(operand.Term.Encoding) == "claim-kind/3"
			}
		}
	}
	return false
}

// typePredicateResultClaim recognizes an annotation sourced from the first
// result slot of an already-published T:is(value) call. The relation follows
// only the front's exact call-results -> write -> claim chain, so an arbitrary
// value named like a predicate result cannot suppress an unproven-claim fact.
func typePredicateResultClaim(artifact equation.Artifact, claim string) bool {
	var source string
	for _, operation := range artifact.Equations {
		if operation.Target.Name != claim || operation.Occurrence.Kind != "claim" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "value" {
				source = string(operand.Term.Encoding)
				break
			}
		}
	}
	if source == "" {
		return false
	}
	var result string
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		var target, value string
		for _, operand := range operation.Operands {
			switch operand.Role {
			case "target":
				target = string(operand.Term.Encoding)
			case "value":
				value = string(operand.Term.Encoding)
			}
		}
		if target == source && strings.HasPrefix(value, "temp/") {
			result = value
			break
		}
	}
	if result == "" {
		return false
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "call-results" {
			continue
		}
		predicate, firstResult := false, ""
		for _, operand := range operation.Operands {
			switch operand.Role {
			case "type-predicate-error-target":
				predicate = true
			case "result-00000000":
				firstResult = string(operand.Term.Encoding)
			}
		}
		if predicate && firstResult == result {
			return true
		}
	}
	return false
}

// uncalledTypedChannelSendBoundary recognizes the narrow declaration-owned
// contract that can be checked before a function is called. The receiver must
// be an exact typed Channel<T> formal already published by the child entry;
// no runtime value or type spelling is invented for a different boundary.
func uncalledTypedChannelSendBoundary(child front.Compilation) bool {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 {
		return false
	}
	channels := make(map[string]bool, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			continue
		}
		if _, ok := ambient.ChannelPayloadType(child.WIR.Type(parameter.Type)); ok {
			channels[boundaryTerm(parameter.Symbol)] = true
		}
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		var receiver, method string
		for _, operand := range operation.Operands {
			switch operand.Role {
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
}

func uncalledTypedChannelSendDiagnostic(diagnostic equation.Fact) bool {
	return strings.HasPrefix(diagnostic.Key, "type.call.direct.argument_type/")
}

// applyKnown evaluates a complete lexical child privately, then projects only
// caller-owned results, capture effects, and residual diagnostics. A malformed
// entry or child failure returns an error, so the surrounding VM publishes no
// partial child result.
func (l *lexicalEvaluator) applyKnown(operation equation.BoundEquation, operands directCallOperands, handle closureHandle, partition equation.Partition) (equation.TransactionResult, error) {
	child, arguments, err := l.knownLexicalBoundary(operands, handle)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	seeds := make([]entrySeed, 0, len(operands.arguments)+len(handle.Captures))
	gradualAnyTerms := make([]string, 0)
	closureSeedByTerm := make(map[string]closureHandle)
	for index, parameter := range child.Boundary.Parameters {
		if parameter.Vararg {
			return equation.TransactionResult{}, fmt.Errorf("engine: vararg lexical boundary is unsupported")
		}
		value, known := resolveKnownCurrentValue(arguments[index], partition)
		if !known || isUnknownScalar(value) {
			// An explicit-any source is itself a completed precision-boundary
			// publication. Preserve that exact boundary through the child entry;
			// replacing it with Top would erase the downstream contract check.
			if _, boundary, explicit := explicitAnySourceFact(arguments[index], partition.Values()); explicit {
				value, known = boundary, true
			} else if summaryTypeIsAny(arguments[index], partition.Values()) {
				value, known = []byte("scalar/claim/claim-kind/3/\"any\""), true
			}
		}
		if !known || (isUnknownScalar(value) && !isExplicitAnyValue(value)) {
			return equation.TransactionResult{}, fmt.Errorf("engine: incomplete lexical argument %d", index)
		}
		// A lexical entry can transport a concrete allocation shape, but that
		// does not validate a value which previously crossed an explicit-any
		// boundary.  Reject it at the caller-owned application while the
		// declared formal contract and the published boundary fact are both
		// available.  The child is not entered with a forged typed seed.
		declared := unwrap.Alias(subst.ExpandInstantiated(child.WIR.Type(parameter.Type)))
		if (isExplicitAnyValue(value) || sourceHasAnyBoundary(arguments[index], partition.Values()) || declaredExplicitAny(arguments[index], partition)) && typeRequiresBoundaryProof(declared) {
			return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is any, not %s", index+1, typeformat.Short(declared))), nil
		}
		term := boundaryTerm(parameter.Symbol)
		seeds = append(seeds, entrySeed{Term: term, Value: value})
		if (declared != nil && declared.Kind() == kind.Any) || sourceHasAnyBoundary(arguments[index], partition.Values()) {
			gradualAnyTerms = append(gradualAnyTerms, term)
		}
		if callback, found := closureHandleFor(arguments[index], partition); found {
			closureSeedByTerm[term] = callback
		}
	}
	for index, capture := range child.Boundary.Captures {
		value, known := resolveKnownCurrentValue([]byte(handle.Captures[index]), partition)
		if !known || isUnknownScalar(value) {
			return equation.TransactionResult{}, fmt.Errorf("engine: incomplete lexical capture %q", capture.Name)
		}
		seeds = append(seeds, entrySeed{Term: boundaryTerm(capture.Symbol), Value: value})
	}
	closureSeeds := make([]entryClosureSeed, 0, len(child.Boundary.Captures))
	if l.closureDemandRecurses(handle, partition) {
		for index, capture := range child.Boundary.Captures {
			// A local capability is admitted only from the same closed caller
			// partition that supplied the capture value. In particular, a plain
			// scalar/function entry value cannot manufacture a recursive edge.
			if captured, found := closureHandleFor([]byte(handle.Captures[index]), partition); found {
				closureSeedByTerm[boundaryTerm(capture.Symbol)] = captured
			}
		}
	}
	for term, closure := range closureSeedByTerm {
		closureSeeds = append(closureSeeds, entryClosureSeed{Term: term, Handle: closure})
	}
	boundarySeeds := append([]entrySeed(nil), seeds...)
	entrySeeds := append([]entrySeed(nil), boundarySeeds...)
	entrySeeds = append(entrySeeds, childEntryDescendantSeeds(boundarySeeds, partition)...)
	memberSources := make(map[string][]byte, len(boundarySeeds))
	for index, parameter := range child.Boundary.Parameters {
		memberSources[boundaryTerm(parameter.Symbol)] = append([]byte(nil), arguments[index]...)
	}
	for index, capture := range child.Boundary.Captures {
		memberSources[boundaryTerm(capture.Symbol)] = []byte(handle.Captures[index])
	}
	memberClosureSeeds := childEntryMemberClosureSeeds(entrySeeds, memberSources, partition)
	tableIdentitySeeds := tableIdentitySeedsForEntry(entrySeeds, partition)
	memberCellSeeds := memberCellSeedsForEntry(entrySeeds, partition)
	placementSeeds := placementSeedsForEntry(entrySeeds, partition)
	entry, err := encodeChildEntryWithPlacementCapabilities(entrySeeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, placementSeeds, gradualAnyTerms)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	// The coordinator owns a discovered recursive demand, not every ordinary
	// lexical invocation.  A non-recursive child retains the established direct
	// evaluation path; once a coordinator callback is active every lexical
	// child is nevertheless admitted through it so discovery sees the complete
	// reachable call graph before any approximation is read.
	var closure equation.OutputClosure
	if l.run != nil || l.closureDemandRecurses(handle, partition) {
		closure, err = l.resolveSCCChild(child, entry, boundarySeeds, arguments, handle, operands, operation.Target.Name)
	} else {
		closure, _, err = l.evaluate(child, entry)
	}
	if err != nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q: %w", handle.Prototype, err)
	}
	returns, err := childReturnValues(closure, !childHasSelect(child))
	if err != nil {
		// A select evaluates every feasible arm, so its branch-local return
		// tuples cannot be collapsed into one caller result slot here. Its
		// completed allocation facts are independent of that tuple transport and
		// may still cross this exact invocation boundary.
		if errors.Is(err, errMultipleChildReturnAlternatives) && childHasSelect(child) {
			return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: placementFactsFromChild(closure.Values)}}, nil
		}
		return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q: %w", handle.Prototype, err)
	}
	projected := projectCallResults(operation, returns, closure)
	projected.Values = append(projected.Values, projectExternalCallbackReturnFacts(operation, child, closure)...)
	for _, fact := range closure.Values {
		if !strings.HasPrefix(fact.Key, typePredicateTargetPrefix) {
			continue
		}
		if _, ok := shapefact.DecodeTarget(fact.Value); !ok {
			continue
		}
		projected.Values = append(projected.Values, equation.Fact{Key: callTypePredicatePrefix + operation.Target.Name + "/" + strings.TrimPrefix(fact.Key, typePredicateTargetPrefix), Value: append([]byte(nil), fact.Value...)})
		break
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
	// The child snapshot is only needed to read a capture cell back, so it is
	// built once on demand rather than for every application.
	childPartition, childPartitionBuilt := equation.Partition{}, false
	for index, capture := range child.Boundary.Captures {
		value, found := latestClosedValue([]byte(boundaryTerm(capture.Symbol)), closure.Values)
		if !found {
			return equation.TransactionResult{}, fmt.Errorf("engine: lexical child %q omitted capture cell %q", handle.Prototype, capture.Name)
		}
		// A static member write inside the child advances that heap cell without
		// republishing the capture's aggregate value. Consume the child's member
		// authority so the writeback carries the mutation the callee made.
		if !childPartitionBuilt {
			built, partitionErr := equation.PartitionFromClosuresWithGuards(nil, closure)
			if partitionErr != nil {
				return equation.TransactionResult{}, partitionErr
			}
			childPartition, childPartitionBuilt = built, true
		}
		value = heapMemberSurface([]byte(boundaryTerm(capture.Symbol)), value, childPartition)
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
			// A member defined on the captured table inside the child is a
			// capability of the caller's cell once the call completes. Rebind each
			// published capability to caller spellings; nothing is admitted that
			// the child did not itself publish.
			for wireIndex, wire := range returnMemberClosures([]byte(boundaryTerm(capture.Symbol)), childPartition) {
				rebound, values, rebindErr := rebindEscapingClosure(operation, child, arguments, handle, closure, wire.Handle)
				if rebindErr != nil {
					return equation.TransactionResult{}, rebindErr
				}
				wire.Handle = rebound
				encoded, marshalErr := json.Marshal(wire)
				if marshalErr != nil {
					return equation.TransactionResult{}, marshalErr
				}
				projected.Values = append(projected.Values, values...)
				for _, alias := range aliases {
					projected.Values = append(projected.Values, equation.Fact{Key: fmt.Sprintf("member-closure/%s/%s/capture-%08d", alias, operation.Target.Name, wireIndex), Value: encoded})
				}
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
	l.projectChildDiagnostics(&projected, operation, operands, child, handle, closure, partition)
	return equation.TransactionResult{Complete: true, Closure: projected}, nil
}

// declaredBranchAssignmentDiagnostics checks one declaration-owned body
// obligation at an exact local call boundary. An initialized-to-nil local
// remains nil on the false edge of a declared boolean guard, even when this
// particular invocation supplies a concrete true value. The helper consumes
// the front's nil write, branch guard, and strict claim as one closed witness;
// it does not extrapolate from a source name or from an uncalled body.
func (l *lexicalEvaluator) declaredBranchAssignmentDiagnostics(child front.Compilation) (bool, equation.OutputClosure, error) {
	claims := declaredFalseEdgeNilAssignmentClaims(child)
	if len(claims) == 0 {
		return false, equation.OutputClosure{}, nil
	}
	entry, err := encodeDeclaredChildEntry(nil)
	if err != nil {
		return false, equation.OutputClosure{}, err
	}
	closure, _, err := l.evaluate(child, entry)
	if err != nil {
		return false, equation.OutputClosure{}, err
	}
	for key, witness := range claims {
		alternative, known := expressionValueType(witness.alternative)
		if !known || alternative == nil {
			continue
		}
		joined, encoded := shapefact.EncodeTarget(typ.MaterializeOptional(alternative))
		if !encoded {
			continue
		}
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + witness.source + "/" + witness.predecessor, Value: joined})
		for index := range closure.Diagnostics {
			if closure.Diagnostics[index].Key == key {
				closure.Diagnostics[index].Value = []byte("cannot assign " + witness.display + " because it may be nil")
			}
		}
	}
	spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, closure.Diagnostics)
	projected := equation.OutputClosure{}
	body := fmt.Sprintf("%x", child.Body)
	for _, item := range publishedDiagnostics(child.Artifact, closure, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil) {
		if _, found := claims[item.Fact.Key]; !found {
			continue
		}
		// This closed witness has a fact-to-claim-to-missing-proof dependency
		// chain. Preserve that semantic order when the source declaration and
		// value use share a line; ordinary evidence remains source ordered.
		for index := range item.Evidence {
			item.Evidence[index].CausalOrder = uint32(index + 1)
		}
		item.Help = "Guard `" + claims[item.Fact.Key].display + "` with a nil check, provide a default value, or change the target type to accept nil."
		key := "child/" + body + "/" + item.Fact.Key
		fact := equation.Fact{Key: key, Value: append([]byte(nil), item.Fact.Value...)}
		projected.Diagnostics = append(projected.Diagnostics, fact)
		if item.Span.Valid() {
			l.diagnosticSpans[key] = item.Span
		}
		item.Fact = fact
		l.childPublished[key] = item
	}
	return true, projected, nil
}

// nilOriginWitness is the closed origin chain of one nil-born binding: the
// annotation that admitted nil, the unconditional write that created it, and
// every non-exhaustive branch merge the nil crossed intact. It is derived from
// the front's own publications, so it names no source text of its own.
type nilOriginWitness struct {
	display  string
	declared string
	claim    string
	joins    []string
	valid    bool
}

// nilOriginUnsafeUses recognizes a possibly-nil binding that reaches a method
// call with no guard on that path. The proof is the body's own control flow:
// the binding is written nil unconditionally, every later write to it is the
// true edge of a branch whose predicate can be false, and the use itself is
// unguarded, so the all-false path carries the original nil into the call.
// A write this recognizer cannot account for, a branch whose false edge is not
// feasible, and a guarded use each leave the binding unreported.
func nilOriginUnsafeUses(child front.Compilation) map[string]nilOriginWitness {
	if child.WIR == nil || child.Cyclic != nil {
		return nil
	}
	witnesses := nilOriginCandidates(child)
	if len(witnesses) == 0 {
		return nil
	}
	uses := make(map[string]nilOriginWitness)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" || len(operation.Guards) != 0 {
			continue
		}
		receiver, hasReceiver := artifactOperand(operation.Operands, "receiver")
		method, hasMethod := artifactOperand(operation.Operands, "method")
		if !hasReceiver || !hasMethod {
			continue
		}
		if name, ok := callMethodName(method); !ok || name == "" {
			continue
		}
		witness, candidate := witnesses[string(receiver)]
		if !candidate || !witness.valid || operation.Target.Name <= witness.claim {
			continue
		}
		lastJoin := ""
		if len(witness.joins) != 0 {
			lastJoin = witness.joins[len(witness.joins)-1]
		}
		if operation.Target.Name <= lastJoin {
			continue
		}
		uses[operation.Target.Name] = witness
	}
	return uses
}

// nilOriginCandidates collects every binding whose complete write history is
// one unconditional nil plus true-edge replacements under feasible branches.
// Any other write invalidates the binding: the recognizer owns the whole cell,
// never a prefix of its history.
func nilOriginCandidates(child front.Compilation) map[string]nilOriginWitness {
	formals := make(map[string]typ.Type, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 || parameter.Type == 0 {
			continue
		}
		formals[boundaryTerm(parameter.Symbol)] = child.WIR.Type(parameter.Type)
	}
	feasibleBranch := make(map[string]bool)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" || len(operation.Guards) != 0 {
			continue
		}
		if !child.BranchJoinSpans[operation.Target.Name].Valid() {
			continue
		}
		predicate, found := artifactOperand(operation.Operands, "predicate")
		if !found || !strings.HasPrefix(string(predicate), branchPredicatePrefix) {
			continue
		}
		var relation branchPredicateWire
		if json.Unmarshal(predicate[len(branchPredicatePrefix):], &relation) != nil {
			continue
		}
		if relation.Kind != "truthy" || relation.Negated || relation.Path == "" {
			continue
		}
		declared, formal := formals["path/"+relation.Path]
		if !formal || declared == nil || !typ.AdmitsFalse(unwrap.Alias(declared)) {
			continue
		}
		feasibleBranch[operation.Target.Name] = true
	}
	witnesses := make(map[string]nilOriginWitness)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		target, hasTarget := artifactOperand(operation.Operands, "target")
		value, hasValue := artifactOperand(operation.Operands, "value")
		if !hasTarget || !hasValue || !strings.HasPrefix(string(target), "path/") {
			continue
		}
		witness := witnesses[string(target)]
		if len(operation.Guards) == 0 && string(value) == "scalar/nil" && len(witness.joins) == 0 {
			witness.valid = true
			witnesses[string(target)] = witness
			continue
		}
		if !witness.valid || len(operation.Guards) != 1 || string(value) == "scalar/nil" {
			witness.valid = false
			witnesses[string(target)] = witness
			continue
		}
		parts := strings.Split(string(operation.Guards[0].Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || parts[3] != "true" || !feasibleBranch[parts[2]] {
			witness.valid = false
			witnesses[string(target)] = witness
			continue
		}
		witness.joins = appendNilOriginJoin(witness.joins, parts[2])
		witnesses[string(target)] = witness
	}
	for term, witness := range witnesses {
		if !witness.valid || len(witness.joins) == 0 {
			delete(witnesses, term)
			continue
		}
		claim, display, declared, annotated := nilOriginDeclaration(child, term)
		if !annotated || nilOriginTermEscapes(child, term, claim) {
			delete(witnesses, term)
			continue
		}
		witness.claim, witness.display, witness.declared = claim, display, declared
		witnesses[term] = witness
	}
	return witnesses
}

func appendNilOriginJoin(joins []string, branch string) []string {
	for _, existing := range joins {
		if existing == branch {
			return joins
		}
	}
	joins = append(joins, branch)
	sort.Strings(joins)
	return joins
}

// nilOriginDeclaration returns the unguarded optional annotation that admitted
// nil into this binding. An unannotated local is deliberately excluded: its
// merged assignment obligation is already owned by the declared-branch
// assignment witness.
func nilOriginDeclaration(child front.Compilation, term string) (string, string, string, bool) {
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "claim" || len(operation.Guards) != 0 {
			continue
		}
		target, hasTarget := artifactOperand(operation.Operands, "target")
		value, hasValue := artifactOperand(operation.Operands, "value")
		kind, hasKind := artifactOperand(operation.Operands, "kind")
		claimType, hasType := artifactOperand(operation.Operands, "type")
		if !hasTarget || !hasValue || !hasKind || !hasType || string(target) != term || string(value) != term || string(kind) != "claim-kind/3" {
			continue
		}
		declared, err := strconv.Unquote(strings.TrimPrefix(string(claimType), "claim-type/"))
		if err != nil || !strings.HasSuffix(declared, "?") {
			continue
		}
		if !child.ClaimNameSpans[operation.Target.Name].Valid() || !child.ClaimTargetSpans[operation.Target.Name].Valid() {
			continue
		}
		display := strings.TrimPrefix(term, "path/")
		for _, operand := range operation.Operands {
			if operand.Role == "display" && len(operand.Term.Encoding) != 0 {
				display = string(operand.Term.Encoding)
			}
		}
		return operation.Target.Name, display, declared, true
	}
	return "", "", "", false
}

// nilOriginTermEscapes reports whether a binding leaves this recognizer's
// write model: a nested closure may capture and rebind it, a heap-level
// operation may replace it outside the ordinary environment-write lane, or a
// second annotation may refine the same cell.
func nilOriginTermEscapes(child front.Compilation, term string, claim string) bool {
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "path-replacement", "path-invalidation", "index-mutation", "generic-for", "channel-select", "dynamic-index-read":
			for _, operand := range operation.Operands {
				encoding := string(operand.Term.Encoding)
				if encoding == term || strings.HasPrefix(encoding, term+".") || strings.HasPrefix(encoding, term+"[") {
					return true
				}
			}
		case "claim":
			if operation.Target.Name == claim {
				continue
			}
			if target, found := artifactOperand(operation.Operands, "target"); found && string(target) == term {
				return true
			}
		}
	}
	for _, nested := range child.Nested {
		for _, capture := range nested.Boundary.Captures {
			if boundaryTerm(capture.Symbol) == term {
				return true
			}
		}
	}
	return false
}

// publishNilOriginUnsafeUse publishes the origin-ordered witness trace for a
// possibly-nil binding that reaches an unguarded method call. The chain is the
// binding's birth, the declaration that admitted nil, every merge it survived,
// and the use itself, each anchored at the front's own source publication.
func (l *lexicalEvaluator) publishNilOriginUnsafeUse(closure *equation.OutputClosure, child front.Compilation) {
	if l == nil || closure == nil || l.sourcePath == "" {
		return
	}
	uses := nilOriginUnsafeUses(child)
	if len(uses) == 0 {
		return
	}
	body := fmt.Sprintf("%x", child.Body)
	for _, operation := range child.Artifact.Equations {
		witness, unsafe := uses[operation.Target.Name]
		if !unsafe {
			continue
		}
		useSpan := child.CallSpans[operation.Target.Name+"/callee"]
		if !useSpan.Valid() {
			useSpan = child.CallSpans[operation.Target.Name+"/call"]
		}
		if !useSpan.Valid() {
			continue
		}
		fact := equation.Fact{
			Key:   "type.nil.unsafe_use/" + operation.Target.Name,
			Value: []byte(witness.display + " may be nil at method call"),
		}
		item := PublishedDiagnostic{
			Fact:    fact,
			Code:    "type.nil.unsafe_use",
			Span:    useSpan,
			Message: string(fact.Value),
			Evidence: []DiagnosticEvidence{{
				Span: child.ClaimNameSpans[witness.claim], Kind: "abstract fact", Trust: "proven",
				Message: fmt.Sprintf("%s born nil at %s:%d (else branch had no assignment)", witness.display, l.sourcePath, child.ClaimNameSpans[witness.claim].StartLine),
			}, {
				Span: child.ClaimTargetSpans[witness.claim], Kind: "user assertion", Trust: "claimed",
				Message: fmt.Sprintf("%s declared with optional type %s", witness.display, witness.declared),
			}},
			Labels: []DiagnosticLabel{{Span: useSpan, Message: "possibly-nil value"}},
			Help:   fmt.Sprintf("Guard %s against nil before the method call, or assign it on every branch.", witness.display),
		}
		for _, join := range witness.joins {
			joinSpan := child.BranchJoinSpans[join]
			item.Evidence = append(item.Evidence, DiagnosticEvidence{
				Span: joinSpan, Kind: "abstract fact", Trust: "proven",
				Message: fmt.Sprintf("%s survives the if/else join at %s:%d (no else assignment)", witness.display, l.sourcePath, joinSpan.StartLine),
			})
		}
		item.Evidence = append(item.Evidence, DiagnosticEvidence{
			Span: useSpan, Kind: "abstract fact", Trust: "proven",
			Message: fmt.Sprintf("%s reaches use at %s:%d (method call on possibly-nil value)", witness.display, l.sourcePath, useSpan.StartLine),
		})
		key := "child/" + body + "/" + fact.Key
		item.Fact = equation.Fact{Key: key, Value: append([]byte(nil), fact.Value...)}
		closure.Diagnostics = append(closure.Diagnostics, item.Fact)
		l.diagnosticSpans[key] = useSpan
		l.childPublished[key] = item
	}
}

// nilOriginFieldWitness is the origin chain of a declared-optional record
// field that reaches a method call: the declaration that admitted nil and the
// unguarded read that consumes it.
type nilOriginFieldWitness struct {
	display   string
	field     string
	fieldType string
	declared  wir.Span
}

// nilOriginOptionalFieldUses recognizes an unguarded method call on a static
// field of a declared formal whose declared type makes that field optional.
// The nil possibility is created by the declaration itself, so no body write
// history is needed; conversely, any write reaching that formal or its field
// leaves the read unreported, because the recognizer does not model it.
func (l *lexicalEvaluator) nilOriginOptionalFieldUses(child front.Compilation) map[string]nilOriginFieldWitness {
	if l == nil || child.WIR == nil || child.Cyclic != nil || len(l.typeFieldSpans) == 0 {
		return nil
	}
	formals := make(map[string]typ.Type, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 || parameter.Type == 0 {
			continue
		}
		formals[boundaryTerm(parameter.Symbol)] = child.WIR.Type(parameter.Type)
	}
	if len(formals) == 0 {
		return nil
	}
	uses := make(map[string]nilOriginFieldWitness)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" || len(operation.Guards) != 0 {
			continue
		}
		receiver, hasReceiver := artifactOperand(operation.Operands, "receiver")
		method, hasMethod := artifactOperand(operation.Operands, "method")
		if !hasReceiver || !hasMethod {
			continue
		}
		if name, ok := callMethodName(method); !ok || name == "" {
			continue
		}
		root, suffix, member := tableAddress(receiver)
		if !member || suffix == "" {
			continue
		}
		segments, static := segment.ParseFormattedSegments(suffix)
		if !static || len(segments) != 1 || segments[0].Kind != segment.SegmentField || segments[0].Name == "" {
			continue
		}
		declared, formal := formals[string(root)]
		if !formal || declared == nil {
			continue
		}
		fieldType, found := access.Field(unwrap.Alias(declared), segments[0].Name)
		if !found || !optionalConcreteWitnessType(fieldType) {
			continue
		}
		span, known := l.declaredFieldNameSpan(declared, segments[0].Name)
		if !known || nilOriginFormalFieldEscapes(child, string(root)) {
			continue
		}
		display := strings.TrimPrefix(string(receiver), "path/")
		for _, operand := range operation.Operands {
			if operand.Role == "receiver-display" && len(operand.Term.Encoding) != 0 {
				display = string(operand.Term.Encoding)
			}
		}
		uses[operation.Target.Name] = nilOriginFieldWitness{
			display: display, field: segments[0].Name,
			fieldType: typeformat.Short(fieldType), declared: span,
		}
	}
	return uses
}

// declaredFieldNameSpan resolves the authored field-name position of a
// top-level record declaration. The declaration is matched by the exact type
// value the resolver produced, so a structurally similar but separately
// declared record cannot borrow another declaration's source location.
func (l *lexicalEvaluator) declaredFieldNameSpan(declared typ.Type, field string) (wir.Span, bool) {
	span, found := wir.Span{}, false
	for name, definition := range l.typeDefinitions {
		if definition != declared {
			continue
		}
		candidate, known := l.typeFieldSpans[name][field]
		if !known || !candidate.Valid() {
			return wir.Span{}, false
		}
		if found && candidate != span {
			return wir.Span{}, false
		}
		span, found = candidate, true
	}
	return span, found
}

// nilOriginFormalFieldEscapes reports whether the body writes through the
// formal at all. Any such write is outside this recognizer's model, so the
// field read stays unreported rather than being proven against a stale
// declaration.
func nilOriginFormalFieldEscapes(child front.Compilation, root string) bool {
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "environment-write", "path-replacement", "path-invalidation", "index-mutation", "generic-for", "channel-select", "dynamic-index-read":
			for _, operand := range operation.Operands {
				if operand.Role != "target" && !strings.HasPrefix(operand.Role, "target-") && operand.Role != "container" {
					continue
				}
				encoding := string(operand.Term.Encoding)
				if encoding == root || strings.HasPrefix(encoding, root+".") || strings.HasPrefix(encoding, root+"[") {
					return true
				}
			}
		}
	}
	for _, nested := range child.Nested {
		for _, capture := range nested.Boundary.Captures {
			if boundaryTerm(capture.Symbol) == root {
				return true
			}
		}
	}
	return false
}

// publishNilOriginOptionalFieldUse publishes the declaration-to-use origin
// trace of an unguarded method call on a declared-optional field.
func (l *lexicalEvaluator) publishNilOriginOptionalFieldUse(closure *equation.OutputClosure, child front.Compilation) {
	if l == nil || closure == nil || l.sourcePath == "" {
		return
	}
	uses := l.nilOriginOptionalFieldUses(child)
	if len(uses) == 0 {
		return
	}
	body := fmt.Sprintf("%x", child.Body)
	for _, operation := range child.Artifact.Equations {
		witness, unsafe := uses[operation.Target.Name]
		if !unsafe {
			continue
		}
		useSpan := child.CallSpans[operation.Target.Name+"/callee"]
		if !useSpan.Valid() {
			useSpan = child.CallSpans[operation.Target.Name+"/call"]
		}
		if !useSpan.Valid() {
			continue
		}
		fact := equation.Fact{
			Key:   "type.nil.unsafe_use/" + operation.Target.Name,
			Value: []byte(witness.display + " may be nil at method call"),
		}
		key := "child/" + body + "/" + fact.Key
		item := PublishedDiagnostic{
			Fact:    equation.Fact{Key: key, Value: append([]byte(nil), fact.Value...)},
			Code:    "type.nil.unsafe_use",
			Span:    useSpan,
			Message: string(fact.Value),
			Evidence: []DiagnosticEvidence{{
				Span: witness.declared, Kind: "user assertion", Trust: "claimed",
				Message: fmt.Sprintf("field %s declared optional at %s:%d (type %s)", witness.field, l.sourcePath, witness.declared.StartLine, witness.fieldType),
			}, {
				Span: useSpan, Kind: "abstract fact", Trust: "proven",
				Message: fmt.Sprintf("%s reaches use at %s:%d (method call on possibly-nil field)", witness.display, l.sourcePath, useSpan.StartLine),
			}},
			Labels: []DiagnosticLabel{{Span: useSpan, Message: "possibly-nil field"}},
			Help:   fmt.Sprintf("Guard %s against nil before the method call.", witness.display),
		}
		closure.Diagnostics = append(closure.Diagnostics, item.Fact)
		l.diagnosticSpans[key] = useSpan
		l.childPublished[key] = item
	}
}

type declaredNilAssignmentWitness struct {
	source, predecessor, display string
	alternative                  []byte
}

// declaredFalseEdgeNilAssignmentClaims recognizes the exact control-flow
// shape whose merge must retain the declaration's Lua nil value: an
// unconditional nil initialization, a truthy test of a declared formal, a
// true-edge replacement of that same cell, and an unguarded strict assignment
// claim that reads it afterwards. Each component is an existing front
// publication, which keeps imported, captured, and arbitrary branch bodies
// outside this declaration-only check.
func declaredFalseEdgeNilAssignmentClaims(child front.Compilation) map[string]declaredNilAssignmentWitness {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 || len(child.Boundary.Parameters) == 0 {
		return nil
	}
	formals := make(map[string]bool, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 {
			return nil
		}
		formals[boundaryTerm(parameter.Symbol)] = true
	}
	nilWrites := make(map[string]bool)
	trueWrites := make(map[string]map[string][]byte)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		target, hasTarget := artifactOperand(operation.Operands, "target")
		value, hasValue := artifactOperand(operation.Operands, "value")
		if !hasTarget || !hasValue || !strings.HasPrefix(string(target), "path/") {
			continue
		}
		if len(operation.Guards) == 0 && string(value) == "scalar/nil" {
			nilWrites[string(target)] = true
			continue
		}
		if len(operation.Guards) != 1 || string(value) == "scalar/nil" {
			continue
		}
		parts := strings.Split(string(operation.Guards[0].Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || parts[3] != "true" {
			continue
		}
		if trueWrites[parts[2]] == nil {
			trueWrites[parts[2]] = make(map[string][]byte)
		}
		trueWrites[parts[2]][string(target)] = append([]byte(nil), value...)
	}
	if len(nilWrites) == 0 || len(trueWrites) == 0 {
		return nil
	}
	candidates := make(map[string][]byte)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" || len(operation.Guards) != 0 || trueWrites[operation.Target.Name] == nil {
			continue
		}
		predicate, found := artifactOperand(operation.Operands, "predicate")
		if !found || !strings.HasPrefix(string(predicate), branchPredicatePrefix) {
			continue
		}
		var relation branchPredicateWire
		if json.Unmarshal(predicate[len(branchPredicatePrefix):], &relation) != nil || relation.Kind != "truthy" || relation.Negated || relation.Path == "" || !formals["path/"+relation.Path] {
			continue
		}
		for target, alternative := range trueWrites[operation.Target.Name] {
			if nilWrites[target] {
				candidates[target] = alternative
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	claims := make(map[string]declaredNilAssignmentWitness)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "claim" || len(operation.Guards) != 0 {
			continue
		}
		value, hasValue := artifactOperand(operation.Operands, "value")
		kind, hasKind := artifactOperand(operation.Operands, "kind")
		targetType, hasType := artifactOperand(operation.Operands, "type")
		alternative, candidate := candidates[string(value)]
		if !hasValue || !hasKind || !hasType || !candidate || string(kind) != "claim-kind/3" || !assignmentTargetRequiresProof(string(targetType)) || len(operation.Dependencies) != 1 {
			continue
		}
		display := strings.TrimPrefix(string(value), "path/")
		for _, operand := range operation.Operands {
			if operand.Role == "source-display" && len(operand.Term.Encoding) != 0 {
				display = string(operand.Term.Encoding)
				break
			}
		}
		claims["type.assignment/"+operation.Target.Name] = declaredNilAssignmentWitness{source: string(value), predecessor: operation.Dependencies[0].Name, display: display, alternative: alternative}
	}
	return claims
}

func (l *lexicalEvaluator) knownLexicalBoundary(operands directCallOperands, handle closureHandle) (front.Compilation, [][]byte, error) {
	child, exists := l.byPrototype[handle.Prototype]
	if !exists {
		return front.Compilation{}, nil, fmt.Errorf("engine: known lexical target %q is unavailable", handle.Prototype)
	}
	arguments := operands.arguments
	if operands.receiver != nil {
		arguments = append([][]byte{operands.receiver}, arguments...)
	}
	if operands.spread || len(arguments) != len(child.Boundary.Parameters) || len(handle.Captures) != len(child.Boundary.Captures) {
		return front.Compilation{}, nil, fmt.Errorf("engine: unsupported exact lexical boundary for %q", handle.Prototype)
	}
	return child, arguments, nil
}

func projectCallResults(operation equation.BoundEquation, returns [][]byte, closure equation.OutputClosure) equation.OutputClosure {
	projected := equation.OutputClosure{}
	for index, value := range returns {
		projected.Values = append(projected.Values, equation.Fact{Key: fmt.Sprintf("call-result/%s/%08d", operation.Target.Name, index), Value: value})
	}
	projected.Values = append(projected.Values, placementFactsFromChild(closure.Values)...)
	return projected
}

// projectExternalCallbackReturnFacts preserves a callback-mutation hazard only
// when the child returns the exact captured table identity carrying that fact.
// The caller's call-results owner already knows how to transport this identity
// to its result slot, so the hazard remains attached to a real publication
// rather than to a guessed alias or source name.
func projectExternalCallbackReturnFacts(operation equation.BoundEquation, child front.Compilation, closure equation.OutputClosure) []equation.Fact {
	var facts []equation.Fact
	seen := make(map[string]bool)
	for _, publication := range child.Artifact.Equations {
		if publication.Occurrence.Kind != "publication" {
			continue
		}
		for _, operand := range publication.Operands {
			if !strings.HasPrefix(operand.Role, "return-value-") {
				continue
			}
			index, err := strconv.Atoi(strings.TrimPrefix(operand.Role, "return-value-"))
			if err != nil || index < 0 {
				continue
			}
			identity, found := closureTableIdentity(operand.Term.Encoding, closure.Values)
			if !found || !heapHasExternalCallbackFacts(identity, closure.Values) {
				continue
			}
			key := fmt.Sprintf("call-heap-identity/%s/%08d", operation.Target.Name, index)
			if !seen[key] {
				facts = append(facts, equation.Fact{Key: key, Value: identity})
				seen[key] = true
			}
			for _, fact := range closure.Values {
				if strings.HasPrefix(fact.Key, heapExternalCallbackPrefix+base64.RawURLEncoding.EncodeToString(identity)+"/") && !seen[fact.Key] {
					facts = append(facts, cloneFact(fact))
					seen[fact.Key] = true
				}
			}
		}
	}
	return facts
}

func closureTableIdentity(term []byte, values []equation.Fact) ([]byte, bool) {
	prefix := heapTableIdentityPrefix + string(term) + "/"
	var identity []byte
	latest := ""
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, prefix) && (identity == nil || fact.Key > latest) {
			identity, latest = append([]byte(nil), fact.Value...), fact.Key
		}
	}
	return identity, identity != nil
}

func heapHasExternalCallbackFacts(identity []byte, values []equation.Fact) bool {
	prefix := heapExternalCallbackPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "may-mutate" {
			return true
		}
	}
	return false
}

func (l *lexicalEvaluator) projectChildDiagnostics(projected *equation.OutputClosure, caller equation.BoundEquation, callerOperands directCallOperands, child front.Compilation, handle closureHandle, closure equation.OutputClosure, partition equation.Partition) {
	// applyKnown reaches this projector only after it has built an exact child
	// entry from the current caller partition and completed the child run.  That
	// call evidence is the publication authority; requiresBody is merely an
	// allocation-time demand hint and must not suppress diagnostics from an
	// already-admitted invocation.
	// A normal call does not create the allocation-time declaration boundary
	// used by static-member-read admission. Keep those write-owned facts local;
	// objectMaterializationKernel publishes them only after its closed formal
	// seeds have established that boundary.
	closure.Diagnostics = rootPublishedDiagnostics(child.Artifact, closure.Diagnostics)
	body := fmt.Sprintf("%x", child.Body)
	spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, closure.Diagnostics)
	transported := make(map[string]bool)
	for _, item := range publishedDiagnostics(child.Artifact, closure, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil) {
		if !childCallDiagnostic(item.Fact) {
			continue
		}
		if summary, ok := l.projectSummaryCallDiagnostic(caller, callerOperands, child, item, partition); ok {
			projected.Diagnostics = append(projected.Diagnostics, cloneFact(summary.Fact))
			l.diagnosticSpans[summary.Fact.Key] = summary.Span
			l.childPublished[summary.Fact.Key] = summary
			transported[item.Fact.Key] = true
			continue
		}
		l.childPublished["child/"+body+"/"+item.Fact.Key] = PublishedDiagnostic{Fact: equation.Fact{Key: "child/" + body + "/" + item.Fact.Key, Value: append([]byte(nil), item.Fact.Value...)}, Code: item.Code, Span: item.Span, Message: item.Message, Evidence: append([]DiagnosticEvidence(nil), item.Evidence...), Labels: append([]DiagnosticLabel(nil), item.Labels...), Help: item.Help}
	}
	for _, diagnostic := range closure.Diagnostics {
		if !childCallDiagnostic(diagnostic) {
			continue
		}
		if transported[diagnostic.Key] {
			continue
		}
		key := "child/" + body + "/" + diagnostic.Key
		projected.Diagnostics = append(projected.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
		if span, ok := spans[diagnostic.Key]; ok {
			l.diagnosticSpans[key] = span
		}
	}
}

// projectSummaryCallDiagnostic moves a refuted member-call contract to its
// caller only when the child argument is exactly one declared formal and the
// enclosing application supplies that same formal position.  The child fact,
// formal boundary, call display, and caller argument span are all published
// front data; aliases, derived expressions, and incomplete spans remain
// child-local rather than receiving a guessed caller explanation.
func (l *lexicalEvaluator) projectSummaryCallDiagnostic(caller equation.BoundEquation, callerOperands directCallOperands, child front.Compilation, item PublishedDiagnostic, partition equation.Partition) (PublishedDiagnostic, bool) {
	code, childOperation, subject, ok := directCallDiagnosticParts(item.Fact.Key)
	if !ok || code != "argument_type" {
		return PublishedDiagnostic{}, false
	}
	childArgument, _, ok := callArgumentSubject(subject)
	if !ok || childArgument == 0 {
		return PublishedDiagnostic{}, false
	}
	var application equation.Equation
	found := false
	for _, candidate := range child.Artifact.Equations {
		if candidate.Target.Name == childOperation && candidate.Occurrence.Kind == "apply" {
			application, found = candidate, true
			break
		}
	}
	if !found {
		return PublishedDiagnostic{}, false
	}
	argumentTerm := []byte(nil)
	argumentRole := fmt.Sprintf("argument-%08d", childArgument-1)
	callee := ""
	for _, operand := range application.Operands {
		switch operand.Role {
		case argumentRole:
			argumentTerm = operand.Term.Encoding
		case "callee-display":
			callee = string(operand.Term.Encoding)
		}
	}
	if len(argumentTerm) == 0 || callee == "" || callerOperands.display == "" || callerOperands.display == "target" {
		return PublishedDiagnostic{}, false
	}
	formal := -1
	for index, parameter := range child.Boundary.Parameters {
		if string(argumentTerm) == boundaryTerm(parameter.Symbol) {
			formal = index
			break
		}
	}
	if formal < 0 || formal >= len(callerOperands.arguments) {
		return PublishedDiagnostic{}, false
	}
	span := l.callSpans[lexicalSpanKey(caller.Target.Body, caller.Target.Name+"/"+indexedCallSubject("argument", formal))]
	if !span.Valid() {
		return PublishedDiagnostic{}, false
	}
	start := strings.Index(item.Message, " is ")
	end := strings.LastIndex(item.Message, ", not ")
	if start < 0 || end <= start+4 {
		return PublishedDiagnostic{}, false
	}
	value, expected := item.Message[start+4:end], item.Message[end+6:]
	innerArgument := fmt.Sprintf("argument %d", childArgument)
	if formalName := child.Boundary.Parameters[formal].Name; formalName != "" {
		innerArgument += " (" + formalName + ")"
	}
	outerArgument := formal + 1
	key := "type.call.direct.argument_type/" + caller.Target.Name + "/" + indexedCallSubject("argument", formal)
	if sourceHasAnyBoundary(callerOperands.arguments[formal], partition.Values()) {
		display := "argument " + strconv.Itoa(outerArgument)
		for _, operand := range caller.Operands {
			if operand.Role == fmt.Sprintf("argument-display-%08d", formal) && len(operand.Value) != 0 {
				display = string(operand.Value)
				break
			}
		}
		innerDisplay := innerArgument
		message := fmt.Sprintf("argument %d (%s) comes from any/unknown; no proof shows it is %s", outerArgument, display, expected)
		return PublishedDiagnostic{
			Fact:    equation.Fact{Key: key, Value: []byte(message)},
			Code:    "type.call.direct.argument_type",
			Span:    span,
			Message: message,
			Evidence: []DiagnosticEvidence{
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type any", innerDisplay)},
				{Span: span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("inside %s, %s is passed to %s parameter %d, which requires %s", callerOperands.display, innerDisplay, callee, childArgument, expected)},
				{Span: span, Kind: "unvalidated value", Trust: "unknown", Reason: "explicit boundary validation", Message: fmt.Sprintf("%s comes from any/unknown", display)},
				{Span: span, Kind: "user assertion", Trust: "claimed", Message: "user asserted any; not abstract-interpreter proof"},
				{Span: span, Kind: "missing proof", Trust: "unknown", Reason: "boundary validation missing", Message: fmt.Sprintf("no proof on this path shows %s is %s", innerDisplay, expected)},
			},
			Labels: []DiagnosticLabel{{Span: span, Message: "argument value any"}},
			Help:   fmt.Sprintf("Validate or narrow `%s` before passing it; any/unknown values do not prove parameter contracts.", display),
		}, true
	}
	return PublishedDiagnostic{
		Fact:    equation.Fact{Key: key, Value: []byte(fmt.Sprintf("argument %d is %s, not %s", outerArgument, value, expected))},
		Code:    "type.call.direct.argument_type",
		Span:    span,
		Message: fmt.Sprintf("argument %d is %s, not %s", outerArgument, value, expected),
		Evidence: []DiagnosticEvidence{
			{Span: span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has literal value %s", innerArgument, value)},
			{Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("inside %s, %s is passed to %s parameter %d, which requires %s", callerOperands.display, innerArgument, callee, childArgument, expected)},
			{Span: span, Kind: "missing proof", Trust: "unknown", Message: fmt.Sprintf("no proof on this path shows %s is %s", innerArgument, expected)},
		},
		Labels: []DiagnosticLabel{{Span: span, Message: "argument value " + value}},
		Help:   fmt.Sprintf("Pass a value for argument %d that satisfies the parameter type, or change the callee signature if that argument is valid.", outerArgument),
	}, true
}

func lexicalSpanKey(body equation.BodyID, occurrence string) string {
	return fmt.Sprintf("%x/%s", body, occurrence)
}

// childCallDiagnostic accepts assignment and call-contract facts whose consumer
// is already owned by an exact child entry. Branch advice and other body-local
// conclusions have no caller-owned consumer and remain private.
func childCallDiagnostic(fact equation.Fact) bool {
	return strings.HasPrefix(fact.Key, "type.assignment/") || strings.HasPrefix(fact.Key, "type.return.contract/") || strings.HasPrefix(fact.Key, "type.call.direct.") || strings.HasPrefix(fact.Key, "type.operator.concat_operand/") || strings.HasPrefix(fact.Key, "type.operator.comparison_operand/")
}

// childEntryDescendantSeeds preserves exact path facts below a captured entry
// root. A child already shares that lexical root; omitting a caller-published
// descendant would silently roll the child back to the root's older literal
// shape. Only completed value facts are copied, keyed by their existing path
// identity; unknown facts remain explicit Top rather than being refined.
func childEntryDescendantSeeds(roots []entrySeed, partition equation.Partition) []entrySeed {
	rootTerms := make([]string, 0, len(roots))
	for _, root := range roots {
		if strings.HasPrefix(root.Term, "path/") {
			rootTerms = append(rootTerms, root.Term)
		}
	}
	if len(rootTerms) == 0 {
		return nil
	}
	latest := make(map[string]equation.Fact)
	for _, fact := range partition.Values() {
		rest := strings.TrimPrefix(fact.Key, "value/")
		cut := strings.LastIndexByte(rest, '/')
		if cut <= 0 || cut == len(rest)-1 {
			continue
		}
		term := rest[:cut]
		if !strings.HasPrefix(term, "path/") {
			continue
		}
		isDescendant := false
		for _, root := range rootTerms {
			isDescendant = strings.HasPrefix(term, root+".") || strings.HasPrefix(term, root+"[")
			if isDescendant {
				break
			}
		}
		if !isDescendant {
			continue
		}
		if prior, exists := latest[term]; !exists || fact.Key > prior.Key {
			latest[term] = fact
		}
	}
	terms := make([]string, 0, len(latest))
	for term := range latest {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	seeds := make([]entrySeed, 0, len(terms))
	for _, term := range terms {
		fact := latest[term]
		seed := entrySeed{Term: term, Value: append([]byte(nil), fact.Value...)}
		if validEntrySeed(seed) {
			seeds = append(seeds, seed)
		}
	}
	return seeds
}

// childEntryMemberClosureSeeds transports only member capabilities already
// published at one of the exact child entry terms.  The child still receives
// the corresponding sealed value seed; no receiver type, annotation, or
// source spelling can manufacture a local-body capability.
func childEntryMemberClosureSeeds(seeds []entrySeed, sources map[string][]byte, partition equation.Partition) []entryMemberClosureSeed {
	byTerm := make(map[string][]memberClosureWire, len(seeds))
	for _, seed := range seeds {
		if !validEntrySeed(seed) {
			continue
		}
		source := sources[seed.Term]
		if len(source) == 0 {
			source = []byte(seed.Term)
		}
		wires := memberClosuresFor(source, partition)
		if len(wires) != 0 {
			byTerm[seed.Term] = wires
		}
	}
	terms := make([]string, 0, len(byTerm))
	for term := range byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	out := make([]entryMemberClosureSeed, 0)
	for _, term := range terms {
		for _, wire := range byTerm[term] {
			out = append(out, entryMemberClosureSeed{Term: term, Wire: wire})
		}
	}
	return out
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

// childReturnValues reads the child's completed return. Several reachable
// return statements are alternatives, and the caller owns a single result
// tuple: they are joinable only when every candidate published the identical
// arity and slot values, which is one result rather than a choice between two.
// Any disagreement remains an unresolved alternative for the caller to reject.
// A select body is excluded: its arms are evaluated in separate partitions, so
// its branch-local return tuples are not comparable at this boundary.
func childReturnValues(closure equation.OutputClosure, joinAlternatives bool) ([][]byte, error) {
	candidates := make([]string, 0, 2)
	seen := make(map[string]bool, 2)
	for _, outcome := range closure.Outcomes {
		if !strings.HasPrefix(outcome.Key, "return-candidate/") || !strings.HasSuffix(outcome.Key, "/arity") {
			continue
		}
		if _, err := strconv.Atoi(string(outcome.Value)); err != nil {
			return nil, fmt.Errorf("malformed child return arity")
		}
		candidate := strings.TrimSuffix(outcome.Key, "/arity") + "/"
		if !seen[candidate] {
			seen[candidate], candidates = true, append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		// A body with no return statement has a complete zero-result outcome.
		return nil, nil
	}
	if len(candidates) > 1 && !joinAlternatives {
		return nil, errMultipleChildReturnAlternatives
	}
	joined, err := childReturnCandidateValues(closure, candidates[0])
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates[1:] {
		values, err := childReturnCandidateValues(closure, candidate)
		if err != nil {
			return nil, err
		}
		if len(values) != len(joined) {
			return nil, errMultipleChildReturnAlternatives
		}
		for index := range joined {
			if !bytes.Equal(joined[index], values[index]) {
				return nil, errMultipleChildReturnAlternatives
			}
		}
	}
	return joined, nil
}

func childReturnCandidateValues(closure equation.OutputClosure, prefix string) ([][]byte, error) {
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
		if err != nil || index < 0 || (index < len(values) && values[index] != nil) {
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
		Key: placementAllocationFactKey(placementAllocationIdentity(operation)), Value: fact,
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
	memberOperands := make([][]byte, 0)
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "prototype":
			if prototype != "" || !strings.HasPrefix(string(operand.Value), "prototype/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed closure prototype")
			}
			prototype = strings.TrimPrefix(string(operand.Value), "prototype/")
		case operand.Role == "result":
			result = string(operand.Value)
		case strings.HasPrefix(operand.Role, "member-"):
			memberOperands = append(memberOperands, operand.Value)
		case strings.HasPrefix(operand.Role, "capture-"):
			captures = append(captures, string(operand.Value))
		}
	}
	memberOrigins := make([]equation.Fact, 0, len(memberOperands))
	for _, member := range memberOperands {
		suffix, source, ok := materializedMemberOrigin(member)
		if !ok {
			// Scalar members do not carry a table identity. Only table-valued
			// member sources need an origin bridge for alias-preserving writes.
			continue
		}
		memberOrigins = append(memberOrigins, heapMemberOriginFact(result, suffix, operation.Target.Name, source))
	}
	if prototype == "" {
		return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: memberOrigins}}, nil
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
	closure := equation.OutputClosure{Values: append(memberOrigins, equation.Fact{Key: "closure/" + result + "/" + operation.Target.Name, Value: handle})}
	child := lexical.byPrototype[prototype]
	// A local nil write is a completed value publication. When an exact
	// captured callable is subsequently invoked with that same cell, its
	// published parameter contract can refute the call even if the surrounding
	// branch is not selected at allocation time. This is a path-local fact: a
	// later write, an opaque callee, or an unclosed capture leaves it dormant.
	if parent, found := lexical.byBody[operation.Target.Body]; found {
		lexical.publishStaticNilCallDiagnostic(&closure, child, parent.Artifact.Equations, operation.Target.Name, captures, partition)
	}
	lexical.publishNilOriginUnsafeUse(&closure, child)
	lexical.publishNilOriginOptionalFieldUse(&closure, child)
	lexical.publishCapturedOptionalMemberCallDiagnostic(&closure, child, captures, partition)
	lexical.publishUncalledFalseEdgeAnyAssignment(&closure, child, captures, partition)
	// A parameter-free child is closed at allocation time. A capture-free child
	// whose entire boundary is explicitly any is closed too: each formal has a
	// concrete precision-boundary fact, rather than an invented top value.
	// Demand either form privately and qualify its facts by body before they
	// enter the root publication closure.
	uncalledSeeds, explicitAnyBoundary := uncalledExplicitAnyBoundary(child)
	gradualLogicalTerms := []string(nil)
	gradualLogicalBoundary := false
	declaredBoundary, declaredMethodBoundary, declaredAssignmentBoundary, declaredConcatBoundary, declaredComparisonBoundary, staticAssignmentBoundary, indexedReadBoundary, localUnionReadBoundary := false, false, false, false, false, false, false, false
	declaredMemberWriteBoundary := false
	declaredConcatOperations := map[string]bool(nil)
	declaredComparisonOperations := map[string]bool(nil)
	if admission := uncalledDeclaredBoundary(child); admission.Admitted {
		uncalledSeeds = admission.Seeds
		explicitAnyBoundary = false
		declaredBoundary = true
		declaredMethodBoundary = admission.Method
		declaredMemberWriteBoundary = admission.MemberWrite
		declaredConcatOperations = admission.Concat
		declaredConcatBoundary = len(admission.Concat) != 0
		declaredComparisonOperations = admission.Comparison
		declaredComparisonBoundary = len(admission.Comparison) != 0
		formals := make(map[string]bool, len(child.Boundary.Parameters))
		for _, parameter := range child.Boundary.Parameters {
			formals[boundaryTerm(parameter.Symbol)] = true
		}
		for _, childOperation := range child.Artifact.Equations {
			declaredAssignmentBoundary = declaredAssignmentBoundary || uncalledDeclaredFormalAssignment(child, childOperation, formals)
		}
	}
	if staticSeeds, admitted := uncalledStaticAssignmentBoundary(child); admitted && lexical.uncalledStaticCapturedCallsAreGuardedValidation(child, partition) {
		uncalledSeeds = staticSeeds
		explicitAnyBoundary = false
		staticAssignmentBoundary = true
	}
	if indexedSeeds, admitted := uncalledDeclaredIndexedReadBoundary(child); admitted {
		uncalledSeeds = indexedSeeds
		explicitAnyBoundary = false
		indexedReadBoundary = true
	}
	if unionSeeds, admitted := uncalledDeclaredLocalUnionReadBoundary(child); admitted {
		uncalledSeeds = unionSeeds
		explicitAnyBoundary = false
		localUnionReadBoundary = true
	}
	if gradualSeeds, terms, admitted := uncalledGradualLogicalCallBoundary(child); admitted {
		uncalledSeeds = gradualSeeds
		gradualLogicalTerms = terms
		explicitAnyBoundary = false
		gradualLogicalBoundary = true
	}
	staticCapturedReturnBoundary := lexical.uncalledStaticCapturedReturnBoundary(child, partition)
	arithmeticBoundary := lexical.uncalledStaticArithmeticBoundary(child, partition)
	typedChannelSendBoundary := uncalledTypedChannelSendBoundary(child)
	// The static member read boundary is the fallback for a declared receiver
	// that no richer boundary admits. It authorizes missing members alone, so a
	// boundary already carrying this body keeps its own seeds and its own wider
	// diagnostic surface.
	staticSeeds, staticMemberReadBoundary := uncalledStaticMemberReadSeeds(child)
	staticMemberReadBoundary = staticMemberReadBoundary && !explicitAnyBoundary && !declaredBoundary &&
		!staticAssignmentBoundary && !indexedReadBoundary && !localUnionReadBoundary && !gradualLogicalBoundary
	if staticMemberReadBoundary {
		uncalledSeeds = staticSeeds
		explicitAnyBoundary = false
	}
	// A parameter-free, capture-free body is closed by its own entry: nothing
	// a caller can supply refines it. Admission must therefore not depend on
	// an arbitrary body-size cap.
	closedBoundary := len(child.Boundary.Parameters) == 0 && len(child.Boundary.Captures) == 0
	if child.Cyclic == nil && (closedBoundary || len(uncalledSeeds) != 0 || staticCapturedReturnBoundary || arithmeticBoundary || typedChannelSendBoundary || staticMemberReadBoundary || gradualLogicalBoundary) {
		entry, admitted, entryErr := []byte(nil), true, error(nil)
		if localUnionReadBoundary {
			entry, admitted, entryErr = lexical.uncalledLocalUnionReadEntry(child, uncalledSeeds, partition)
		} else if declaredBoundary {
			entry, entryErr = encodeDeclaredChildEntry(uncalledSeeds)
		} else if len(child.Boundary.Captures) == 0 {
			if gradualLogicalBoundary {
				entry, entryErr = encodeChildEntryWithCapabilities(uncalledSeeds, nil, nil, nil, nil, gradualLogicalTerms)
			} else {
				seeds, closureSeeds := lexical.closedBodyCalleeSeeds(child, uncalledSeeds, partition)
				entry, entryErr = encodeChildEntry(seeds, closureSeeds...)
			}
		} else {
			entry, admitted, entryErr = lexical.uncalledChildEntry(child, uncalledSeeds, partition, gradualLogicalBoundary || staticAssignmentBoundary || staticCapturedReturnBoundary || arithmeticBoundary, arithmeticBoundary, arithmeticBoundary, staticAssignmentBoundary, gradualLogicalTerms)
		}
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		if !admitted {
			return equation.TransactionResult{Complete: true, Closure: closure}, nil
		}
		outcome, _, evaluateErr := lexical.evaluate(child, entry)
		if evaluateErr != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: uncalled lexical child %q: %w", prototype, evaluateErr)
		}
		if gradualLogicalBoundary {
			for _, term := range gradualLogicalTerms {
				outcome.Values = append(outcome.Values, equation.Fact{Key: "gradual-logical/" + term + "/entry", Value: []byte(term)})
			}
		}
		body := fmt.Sprintf("%x", child.Body)
		spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, outcome.Diagnostics)
		claimOwnedReads := claimConsumedStaticReads(child.Artifact)
		for _, diagnostic := range outcome.Diagnostics {
			if claimOwnedReads[strings.TrimPrefix(diagnostic.Key, "type.member.missing/")] && strings.HasPrefix(diagnostic.Key, "type.member.missing/") {
				continue
			}
			// An allocation-time any boundary can prove only a strict assignment
			// contract lacks validation. Other child diagnostics still require a
			// concrete call path, since a cast or operation may establish their
			// proof before that path reaches publication.
			if explicitAnyBoundary && !uncalledExplicitAnyDiagnostic(child.Artifact, diagnostic) {
				continue
			}
			if typedChannelSendBoundary && !uncalledTypedChannelSendDiagnostic(diagnostic) {
				continue
			}
			if gradualLogicalBoundary && !strings.HasPrefix(diagnostic.Key, "type.call.direct.argument_type/") {
				continue
			}
			// A declaration-only entry is sufficient for a member that is absent
			// from a reachable declared union arm. Other obligations may depend on
			// a call-specific refinement and therefore remain demand-driven.
			if declaredBoundary && !strings.HasPrefix(diagnostic.Key, "type.member.missing/") &&
				(!declaredMethodBoundary || !strings.HasPrefix(diagnostic.Key, "type.return.contract/")) &&
				(!declaredAssignmentBoundary || !strings.HasPrefix(diagnostic.Key, "type.assignment/")) &&
				(!declaredMemberWriteBoundary || !strings.HasPrefix(diagnostic.Key, "type.assignment.optional_target/")) &&
				(!declaredConcatBoundary || !declaredConcatDiagnostic(declaredConcatOperations, diagnostic.Key)) &&
				(!declaredComparisonBoundary || !declaredOrderedComparisonDiagnostic(declaredComparisonOperations, diagnostic.Key)) &&
				!uncalledDeclaredProviderResultDiagnostic(child, diagnostic.Key) {
				continue
			}
			if staticMemberReadBoundary && !strings.HasPrefix(diagnostic.Key, "type.member.missing/") {
				continue
			}
			if staticAssignmentBoundary && !lexical.uncalledStaticAssignmentDiagnostic(child.Artifact, diagnostic.Key, partition) && !uncalledStaticOptionalMethodDiagnostic(child.Artifact, diagnostic) && !uncalledStaticResultCallDiagnostic(child.Artifact, diagnostic.Key) {
				continue
			}
			if arithmeticBoundary && !strings.HasPrefix(diagnostic.Key, "type.call.direct.argument_type/") {
				continue
			}
			if staticCapturedReturnBoundary && !strings.HasPrefix(diagnostic.Key, "type.return.contract/") {
				continue
			}
			if indexedReadBoundary && !strings.HasPrefix(diagnostic.Key, "type.assignment/") && !strings.HasPrefix(diagnostic.Key, "type.return.contract/") {
				continue
			}
			// The union this boundary admits is the declaration itself, so a
			// member absent from one of its arms is refuted by that declaration
			// alone, exactly as it is for a declared-parameter boundary.
			if localUnionReadBoundary && !strings.HasPrefix(diagnostic.Key, "type.assignment/") && !strings.HasPrefix(diagnostic.Key, "type.member.missing/") {
				continue
			}
			key := "child/" + body + "/" + diagnostic.Key
			closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: key, Value: append([]byte(nil), diagnostic.Value...)})
			if span, ok := spans[diagnostic.Key]; ok {
				lexical.diagnosticSpans[key] = span
			}
		}
		// The child has already evaluated its own closed entry.  Preserve the
		// corresponding source-facing projection alongside the qualified fact so
		// allocation-time diagnostics retain the evidence published by their
		// owning claim rather than falling back to an unadorned parent fact.
		for _, item := range publishedDiagnostics(child.Artifact, outcome, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil) {
			if claimOwnedReads[strings.TrimPrefix(item.Fact.Key, "type.member.missing/")] && strings.HasPrefix(item.Fact.Key, "type.member.missing/") {
				continue
			}
			if typedChannelSendBoundary && !uncalledTypedChannelSendDiagnostic(item.Fact) {
				continue
			}
			if gradualLogicalBoundary && !strings.HasPrefix(item.Fact.Key, "type.call.direct.argument_type/") {
				continue
			}
			if declaredBoundary && !strings.HasPrefix(item.Fact.Key, "type.member.missing/") &&
				(!declaredMethodBoundary || !strings.HasPrefix(item.Fact.Key, "type.return.contract/")) &&
				(!declaredAssignmentBoundary || !strings.HasPrefix(item.Fact.Key, "type.assignment/")) &&
				(!declaredMemberWriteBoundary || !strings.HasPrefix(item.Fact.Key, "type.assignment.optional_target/")) &&
				(!declaredConcatBoundary || !declaredConcatDiagnostic(declaredConcatOperations, item.Fact.Key)) &&
				(!declaredComparisonBoundary || !declaredOrderedComparisonDiagnostic(declaredComparisonOperations, item.Fact.Key)) &&
				!uncalledDeclaredProviderResultDiagnostic(child, item.Fact.Key) {
				continue
			}
			if staticMemberReadBoundary && !strings.HasPrefix(item.Fact.Key, "type.member.missing/") {
				continue
			}
			if staticAssignmentBoundary && !lexical.uncalledStaticAssignmentDiagnostic(child.Artifact, item.Fact.Key, partition) && !uncalledStaticOptionalMethodDiagnostic(child.Artifact, item.Fact) && !uncalledStaticResultCallDiagnostic(child.Artifact, item.Fact.Key) {
				continue
			}
			if arithmeticBoundary && !strings.HasPrefix(item.Fact.Key, "type.call.direct.argument_type/") {
				continue
			}
			if staticCapturedReturnBoundary && !strings.HasPrefix(item.Fact.Key, "type.return.contract/") {
				continue
			}
			if indexedReadBoundary && !strings.HasPrefix(item.Fact.Key, "type.assignment/") && !strings.HasPrefix(item.Fact.Key, "type.return.contract/") {
				continue
			}
			if localUnionReadBoundary && !strings.HasPrefix(item.Fact.Key, "type.assignment/") && !strings.HasPrefix(item.Fact.Key, "type.member.missing/") {
				continue
			}
			key := "child/" + body + "/" + item.Fact.Key
			lexical.childPublished[key] = PublishedDiagnostic{
				Fact:     equation.Fact{Key: key, Value: append([]byte(nil), item.Fact.Value...)},
				Code:     item.Code,
				Span:     item.Span,
				Message:  item.Message,
				Evidence: append([]DiagnosticEvidence(nil), item.Evidence...),
				Labels:   append([]DiagnosticLabel(nil), item.Labels...),
				Help:     item.Help,
			}
		}
		if typedChannelSendBoundary {
			closure.Values = append(closure.Values, placementFactsFromChild(outcome.Values)...)
		}
	}
	// A closure with no formals or captures has a fully closed body at its
	// allocation boundary.  Its placement conclusions are independent of the
	// optional diagnostic publication above, so carry the already-closed facts
	// even when the body is larger than the small diagnostic admission budget.
	// This remains fail-closed: any unresolved body evaluation aborts the
	// constructor, and the child projector accepts only complete allocations
	// and their own self-contained boundary evidence.
	if child.Cyclic == nil && len(child.Boundary.Parameters) == 0 && len(child.Boundary.Captures) == 0 {
		entry, entryErr := encodeChildEntry(nil)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		outcome, _, evaluateErr := lexical.evaluate(child, entry)
		if evaluateErr != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: closed lexical child %q: %w", prototype, evaluateErr)
		}
		closure.Values = append(closure.Values, placementFactsFromChild(outcome.Values)...)
	}
	// The cyclic evaluator may establish a local allocation while traversing a
	// loop even though the closure is not invoked at this allocation site. Its
	// declared formals and already-published captures form a closed entry; only
	// stack witnesses without any boundary evidence cross the child projector.
	if seeds, eligible := cyclicPlacementWitnessEntry(child); eligible {
		entry, admitted, entryErr := lexical.uncalledChildEntry(child, seeds, partition, true, false, false, false)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		if admitted {
			outcome, _, evaluateErr := lexical.evaluate(child, entry)
			if evaluateErr == nil {
				closure.Values = append(closure.Values, placementStackWitnessFacts(outcome.Values)...)
			}
		}
	}
	// A capture-free closure with a declared, non-recursive return slot has an
	// independently published entry contract.  Its returned table is a real
	// prospective allocation site even before a caller supplies an invocation;
	// projection retains only the child's completed placement facts, so any
	// opaque boundary remains conservative in the public plan.
	if seeds, eligible := placementReturnWitnessEntry(child); eligible && lexical.publishesClosureReturn(operation.Target.Body, result) {
		entry, admitted, entryErr := lexical.uncalledChildEntry(child, seeds, partition, false, false, false, false)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		if admitted {
			outcome, _, evaluateErr := lexical.evaluate(child, entry)
			if evaluateErr == nil {
				closure.Values = append(closure.Values, placementFactsFromChild(outcome.Values)...)
				closure.Values = append(closure.Values, placementFactsFromChild(placementDeclaredScalarResultWitnesses(child, outcome))...)
			}
		}
	}
	if lexical.publishesClosureReturn(operation.Target.Body, result) {
		closure.Values = append(closure.Values, placementReturnedClosureWitnesses(child, partition)...)
		closure.Values = append(closure.Values, placementDeclaredScalarLocalWitnesses(child)...)
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
			spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, outcome.Diagnostics)
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
		entry, entryErr := lexical.selectChildEntry(child, captures, partition)
		if entryErr != nil {
			return equation.TransactionResult{}, entryErr
		}
		outcome, _, evaluateErr := lexical.evaluate(child, entry)
		if evaluateErr != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: select child %q: %w", prototype, evaluateErr)
		}
		spans := diagnosticSpans(child.ClaimSpans, child.CallSpans, child.BranchSpans, child.EffectSpans, child.ExpressionSpans, child.ReturnSpans, outcome.Diagnostics)
		childPublished := publishedDiagnostics(child.Artifact, outcome, spans, child.ClaimTargetSpans, child.CallSpans, child.BranchSpans, child.ReturnSpans, nil, nil)
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

func declaredConcatDiagnostic(allowed map[string]bool, key string) bool {
	_, operation, subject, ok := concatOperandDiagnosticParts(key)
	return ok && allowed[operation+"/"+subject]
}

func declaredOrderedComparisonDiagnostic(allowed map[string]bool, key string) bool {
	name, ok := strings.CutPrefix(key, "type.operator.comparison_operand/")
	return ok && allowed[name]
}

// staticMemberSegment reports whether a path segment names one member of a
// declared receiver. Field and quoted-key spellings address the same member,
// so both establish the static member read boundary.
func staticMemberSegment(item segment.Segment) bool {
	return item.Kind == segment.SegmentField || item.Kind == segment.SegmentIndexString
}

// claimConsumedStaticReads names the environment writes whose read a claim in
// the same body also consumes — either by reading the same source path or by
// reading back the local the write produced. The claim owns that read's
// diagnostic surface: it carries the annotation, the source display, and the
// span, so the write's own missing-member fact would republish the same read.
func claimConsumedStaticReads(artifact equation.Artifact) map[string]bool {
	sources := make(map[string]bool, len(artifact.Equations))
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "claim" {
			continue
		}
		if value, found := artifactOperand(operation.Operands, "value"); found {
			sources[string(value)] = true
		}
	}
	consumed := make(map[string]bool, len(sources))
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		value, hasValue := artifactOperand(operation.Operands, "value")
		target, hasTarget := artifactOperand(operation.Operands, "target")
		if hasValue && sources[string(value)] || hasTarget && sources[string(target)] {
			consumed[operation.Target.Name] = true
		}
	}
	return consumed
}

func uncalledStaticMemberReadSeeds(child front.Compilation) ([]entrySeed, bool) {
	if child.WIR == nil || child.Cyclic != nil || len(child.Boundary.Captures) != 0 {
		return nil, false
	}
	formals := make(map[string]entrySeed, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Symbol == 0 || parameter.Type == 0 || child.WIR.Type(parameter.Type) == nil {
			continue
		}
		encoded, ok := shapefact.EncodeTarget(child.WIR.Type(parameter.Type))
		if !ok {
			return nil, false
		}
		formals[boundaryTerm(parameter.Symbol)] = entrySeed{Term: boundaryTerm(parameter.Symbol), Value: encoded}
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role != "value" {
				continue
			}
			root, suffix, ok := tableAddress(operand.Term.Encoding)
			segments, static := segment.ParseFormattedSegments(suffix)
			if _, found := formals[string(root)]; ok && found && static && len(segments) == 1 && staticMemberSegment(segments[0]) {
				seeds := make([]entrySeed, 0, len(formals))
				for _, item := range formals {
					seeds = append(seeds, item)
				}
				sort.Slice(seeds, func(i, j int) bool { return seeds[i].Term < seeds[j].Term })
				return seeds, true
			}
		}
	}
	return nil, false
}

// closedImportEntrySeeds reuses only import values already published by the
// parent's entry transaction. Child evaluation needs these same module facts
// to project an imported member call's result; the captured table value alone
// cannot recreate the provider's exact module identity.
func closedImportEntrySeeds(partition equation.Partition) []entrySeed {
	const prefix = "value/import/"
	byTerm := make(map[string][]byte)
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(fact.Key, "value/")
		cut := strings.LastIndexByte(rest, '/')
		if cut <= 0 {
			continue
		}
		term := rest[:cut]
		if !validEntrySeed(entrySeed{Term: term, Value: fact.Value}) {
			continue
		}
		byTerm[term] = append([]byte(nil), fact.Value...)
	}
	terms := make([]string, 0, len(byTerm))
	for term := range byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	seeds := make([]entrySeed, 0, len(terms))
	for _, term := range terms {
		seeds = append(seeds, entrySeed{Term: term, Value: byTerm[term]})
	}
	return seeds
}

// closedBodyCalleeSeeds carries the module-lexical callables a body invokes but
// does not bind. A parameter-free, capture-free body is closed only once those
// bindings travel with it: without them its own calls resolve to nothing, and
// every contract they carry is lost. Only a currently published callable value
// and its admitted prototype cross; mutable state and unavailable bindings stay
// out of the private entry.
func (l *lexicalEvaluator) closedBodyCalleeSeeds(child front.Compilation, seeds []entrySeed, partition equation.Partition) ([]entrySeed, []entryClosureSeed) {
	bound := make(map[string]bool, len(seeds))
	for _, seed := range seeds {
		bound[seed.Term] = true
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		if target, found := artifactOperand(operation.Operands, "target"); found {
			bound[string(target)] = true
		}
	}
	callees := make([]string, 0, len(child.Artifact.Equations))
	seen := make(map[string]bool, len(child.Artifact.Equations))
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "apply" {
			continue
		}
		callee, found := artifactOperand(operation.Operands, "callee")
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
		value, known := resolveKnownCurrentValue(term, partition)
		if !known || !isCallableValue(value) {
			continue
		}
		seed := entrySeed{Term: callee, Value: value}
		if !validEntrySeed(seed) {
			continue
		}
		handle, hasHandle := closureHandleFor(term, partition)
		if hasHandle {
			if _, admitted := l.byPrototype[handle.Prototype]; !admitted {
				continue
			}
		}
		seeds = append(seeds, seed)
		if hasHandle {
			closureSeeds = append(closureSeeds, entryClosureSeed{Term: callee, Handle: handle})
		}
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].Term < seeds[j].Term })
	return seeds, closureSeeds
}

// selectChildEntry seeds the complete private environment of the optional
// select-publication evaluation.  This evaluator is deliberately independent
// of whichever unrelated fact families happened to publish in its parent:
// every root the child can read has either its exact captured value/capability
// or an explicit Top seed.  Absence is reserved for a malformed entry packet,
// never used to represent an unknown lexical value.
func (l *lexicalEvaluator) selectChildEntry(child front.Compilation, captures []string, partition equation.Partition) ([]byte, error) {
	byTerm := make(map[string]entrySeed)
	sources := make(map[string][]byte)
	for _, seed := range closedImportEntrySeeds(partition) {
		byTerm[seed.Term] = seed
	}
	closureByTerm := make(map[string]closureHandle)
	for index, capture := range child.Boundary.Captures {
		term := boundaryTerm(capture.Symbol)
		value := []byte("scalar/top")
		if index < len(captures) {
			source := []byte(captures[index])
			sources[term] = append([]byte(nil), source...)
			if current, known := resolveKnownCurrentValue(source, partition); known {
				value = current
			}
			if handle, found := closureHandleFor(source, partition); found {
				if _, admitted := l.byPrototype[handle.Prototype]; admitted {
					closureByTerm[term] = handle
				}
			}
		}
		byTerm[term] = entrySeed{Term: term, Value: value}
	}
	// Terms occur in the compiled artifact rather than in a source scan, so
	// this covers every evaluator-visible root (including local roots created
	// by select lowering) without coupling entry construction to publication
	// order. Descendants inherit their root's entry state and are evaluated only
	// after their own body write, as usual.
	for _, operation := range child.Artifact.Equations {
		for _, operand := range operation.Operands {
			term := selectChildRootTerm(string(operand.Term.Encoding))
			if term == "" {
				continue
			}
			if _, seeded := byTerm[term]; !seeded {
				byTerm[term] = entrySeed{Term: term, Value: []byte("scalar/top")}
			}
		}
	}
	terms := make([]string, 0, len(byTerm))
	for term := range byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	seeds := make([]entrySeed, 0, len(terms))
	for _, term := range terms {
		seed := byTerm[term]
		if !validEntrySeed(seed) {
			return nil, fmt.Errorf("engine: malformed select child entry seed %q", term)
		}
		seeds = append(seeds, seed)
	}
	seeds = append(seeds, childEntryDescendantSeeds(seeds, partition)...)
	// Descendant transport can overlap a root only on malformed input; retain
	// the root's explicit entry seed as the canonical value in that case.
	byTerm = make(map[string]entrySeed, len(seeds))
	for _, seed := range seeds {
		if _, exists := byTerm[seed.Term]; !exists {
			byTerm[seed.Term] = seed
		}
	}
	terms = terms[:0]
	for term := range byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	seeds = seeds[:0]
	for _, term := range terms {
		seeds = append(seeds, byTerm[term])
	}
	closureSeeds := make([]entryClosureSeed, 0, len(closureByTerm))
	for _, term := range terms {
		if handle, found := closureByTerm[term]; found {
			closureSeeds = append(closureSeeds, entryClosureSeed{Term: term, Handle: handle})
		}
	}
	memberClosureSeeds := childEntryMemberClosureSeeds(seeds, sources, partition)
	tableIdentitySeeds := tableIdentitySeedsForEntry(seeds, partition)
	memberCellSeeds := memberCellSeedsForEntry(seeds, partition)
	placementSeeds := placementSeedsForEntry(seeds, partition)
	return encodeChildEntryWithPlacementCapabilities(seeds, closureSeeds, memberClosureSeeds, tableIdentitySeeds, memberCellSeeds, placementSeeds)
}

func selectChildRootTerm(term string) string {
	if !strings.HasPrefix(term, "path/") {
		return ""
	}
	path := strings.TrimPrefix(term, "path/")
	if path == "" {
		return ""
	}
	if cut := strings.IndexAny(path, ".["); cut >= 0 {
		path = path[:cut]
	}
	if path == "" {
		return ""
	}
	return "path/" + path
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
	// A transition published under a branch guard happened on one edge only, so
	// it is not the state the function exits in. The unguarded transitions are
	// the ones every path performs; a guarded one counts only for a resource
	// whose whole lifetime is inside that same arm, which is exactly the case
	// with no unguarded transition to read.
	latest, guarded := make(map[string]equation.Fact), make(map[string]equation.Fact)
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
		target := latest
		if len(fact.Guards) != 0 {
			target = guarded
		}
		if previous, found := target[identity]; !found || fact.Key > previous.Key {
			target[identity] = fact
		}
	}
	for identity, fact := range guarded {
		if _, found := latest[identity]; !found {
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

func writeKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
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
		{Key: epochFactPrefix + target + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
	}
	// A direct alias of an optional declared value keeps that declaration as
	// descriptive metadata for a later non-nil guard. It remains unavailable to
	// concrete writes: a reassignment replaces the target's current value and
	// cannot inherit this contract.
	if !derivedPathTerm(operands["value"]) {
		if declared, found := declaredTypeForTerm(operands["value"], partition); found {
			if encoded, ok := shapefact.EncodeTarget(declared); ok && optionalConcreteWitnessType(declared) {
				values = append(values, equation.Fact{Key: "declared-type/" + target + "/" + operation.Target.Name, Value: encoded})
			}
		}
	}
	// A literal read through a resolved static bracket member carries exact
	// source evidence independently of the destination binding. Preserve that
	// narrow publication for the immediate annotation diagnostic; ordinary
	// scalar assignments retain their existing type-level presentation.
	if staticBracketMemberPath(operands["value"]) && scalarLiteralDiagnosticValue(value) {
		values = append(values, equation.Fact{Key: literalDiagnosticPrefix + target + "/" + operation.Target.Name, Value: append([]byte(nil), value...)})
	}
	// A static bracket path may already have a closed declared type through its
	// source root. Preserve only its present element display for the exact
	// destination: the value equation remains the authority for nilability.
	if derivedIndexedPath(operands["value"]) {
		if sourceType, known := typedPathType(operands["value"], partition); known && sourceType != nil && proof.OptionalTypeHasConcreteValue(sourceType) {
			values = append(values, equation.Fact{
				Key:   indexReadDisplayPrefix + target + "/" + operation.Target.Name,
				Value: []byte(typeformat.Short(proof.ProjectionWithoutNil(sourceType))),
			})
			if _, scalar := optionalEvidenceDisplay(sourceType); scalar {
				values = append(values, equation.Fact{Key: indexReadScalarPrefix + target + "/" + operation.Target.Name, Value: []byte("scalar")})
			}
		}
	}
	var diagnostics []equation.Fact
	if memberMissing(value) {
		root, _, declaredSource := tableAddress(operands["value"])
		_, declared := declaredTypeForTerm(root, partition)
		if declaredSource && declared {
			for _, operand := range operation.Operands {
				if operand.Role == "source-display" {
					diagnostics = append(diagnostics, equation.Fact{Key: "type.member.missing/" + operation.Target.Name, Value: []byte(memberMissingMessage(string(operand.Value), value))})
					break
				}
			}
		}
	}
	if boundary, ok := gradualAnyBoundaryFact(target, operands["value"], operation.Target.Name, partition.Values()); ok {
		values = append(values, boundary)
	} else if root, _, explicit := explicitAnySourceFact(operands["value"], partition.Values()); explicit {
		values = append(values, equation.Fact{Key: "gradual-any/" + target + "/" + operation.Target.Name, Value: root})
	}
	if optional, ok := currentEpochFact("optional-provider-result/", operands["value"], partition); ok {
		values = append(values, equation.Fact{Key: "optional-provider-result/" + target + "/" + operation.Target.Name, Value: optional})
	}
	if providerAny, ok := currentEpochFact("provider-any-result/", operands["value"], partition); ok {
		values = append(values, equation.Fact{Key: "provider-any-result/" + target + "/" + operation.Target.Name, Value: providerAny})
	}
	if authority, ok := lexical.importedAuthority(string(operands["value"])); ok {
		lexical.setImportedAuthority(target, authority)
	}
	// An ordinary write can establish a table alias. Preserve the table identity
	// through that already-published write so a later exact dynamic mutation is
	// applied to the same heap cell. Without this fact, alias[key] loses the
	// authority needed by indexMutationKernel and a subsequent static read can
	// only fall back to an optional shape.
	for _, prefix := range []string{"identity/", "type/", summaryTypePrefix, methodReturnSummaryPrefix, "select/origin/", heapTableIdentityPrefix, "local-call-result/", indexReadDisplayPrefix, indexReadScalarPrefix} {
		if inherited, ok := currentEpochFact(prefix, operands["value"], partition); ok {
			values = append(values, equation.Fact{Key: prefix + target + "/" + operation.Target.Name, Value: inherited})
		} else if prefix == "select/origin/" {
			if _, stale := currentEpochFact(prefix, []byte(target), partition); !stale {
				continue
			}
			// A select correlation belongs to the value currently bound at this
			// path.  A later ordinary write must publish its revocation, otherwise
			// currentEpochFact can resurrect the former select's arm constraint
			// after the result record has been replaced.
			values = append(values, equation.Fact{Key: prefix + target + "/" + operation.Target.Name, Value: nil})
		}
	}
	if _, inherited := currentEpochFact(summaryTypePrefix, operands["value"], partition); !inherited {
		if summary, known := typedPathType(operands["value"], partition); known {
			encoded, encodeErr := typ.EncodeCanonical(context.Background(), summary)
			if encodeErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: encode derived summary type: %w", encodeErr)
			}
			values = append(values, equation.Fact{Key: summaryTypePrefix + target + "/" + operation.Target.Name, Value: encoded})
		}
	}
	values = append(values, rebaseChannelPayloadFacts(operands["value"], target, operation.Target.Name, partition)...)
	identity, hasIdentity := tableIdentityForTerm(operands["value"], partition)
	if !hasIdentity && (shapefact.IsTable(value) || string(value) == "scalar/table") {
		identity, hasIdentity = sealedTableIdentity(operation), true
	}
	if hasIdentity {
		values = append(values, heapIdentityFact(target, operation.Target.Name, identity))
		if table, ok := shapefact.DecodeTable(value); ok {
			if table.Closed {
				values = append(values, heapClosedFact(identity, operation.Target.Name))
			}
			for _, member := range table.Members {
				memberValue := []byte("scalar/nil")
				if member.Present {
					memberValue = []byte(member.Value)
				}
				values = append(values, heapMemberFact(identity, member.Suffix, operation.Target.Name, memberValue))
				memberSource, memberIdentity := memberValue, []byte(nil)
				if allocationResult, found := operationOperand(operation.Operands, "allocation-result"); found {
					if source, found := heapMemberOriginCurrent(allocationResult, member.Suffix, partition); found {
						memberSource = source
						if resolved, found := tableIdentityForTerm(source, partition); found {
							memberIdentity = resolved
						}
					}
				}
				if cell, published, cellErr := memberCellFactWithSource(identity, member.Suffix, operation.Target.Name, memberValue, memberSource, memberIdentity, partition); cellErr != nil {
					return equation.TransactionResult{}, cellErr
				} else if published {
					values = append(values, cell)
				}
				if len(memberIdentity) != 0 {
					values = append(values, heapMemberIdentityFact(identity, member.Suffix, operation.Target.Name, memberIdentity))
				}
			}
		}
	}
	if root, suffix, ok := heapTableAddress([]byte(target)); ok && suffix != "" {
		if parent, found := tableIdentityForTerm(root, partition); found {
			values = append(values, heapMemberFact(parent, suffix, operation.Target.Name, value))
			memberIdentity, _ := tableIdentityForTerm(operands["value"], partition)
			if hasIdentity {
				memberIdentity = identity
			}
			if cell, published, cellErr := memberCellFactWithIdentity(parent, suffix, operation.Target.Name, value, memberIdentity, partition); cellErr != nil {
				return equation.TransactionResult{}, cellErr
			} else if published {
				values = append(values, cell)
			}
			if hasIdentity {
				values = append(values, heapMemberIdentityFact(parent, suffix, operation.Target.Name, identity))
			}
		}
	}
	values = append(values, projectTypePredicateWriteRelation(target, operands["value"], operation.Target.Name, partition)...)
	values = append(values, projectReturnTupleWriteRelation(target, operands["value"], operation.Target.Name, partition)...)
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
	} else if allocation, found := placementAllocationForTerm(operands["value"], partition); found {
		// Ordinary writes have no allocation-result operand. Their source
		// binding is the only established alias proof; never infer one from a
		// matching name or shape.
		values = append(values, placementBindingFact(target, operation.Target.Name, allocation.Identity))
	}
	if _, suffix, member := tableAddress([]byte(target)); member && suffix != "" {
		if allocation, found := placementAllocationForTerm(allocationResult, partition); found {
			values = append(values, placementBlockerFact(allocation.Identity, operation.Target.Name, "member-store"))
		}
		if root, _, ok := tableAddress([]byte(target)); ok {
			parent, hasParent := placementAllocationForTerm(root, partition)
			child, hasChild := placementAllocationForTerm(operands["value"], partition)
			if hasParent && hasChild && parent.Identity != child.Identity {
				values = append(values, placementContainmentFact(parent.Identity, child.Identity, operation.Target.Name))
			}
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
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values, Diagnostics: diagnostics}}, nil
}

// projectReturnTupleWriteRelation follows the same explicit assignment chain
// as the existing type-predicate pair transport. A call result relation is
// not a path alias: each side is rebound only by the exact ordinary write that
// owns that result slot, and any unrelated write leaves the relation absent.
func projectReturnTupleWriteRelation(target string, source []byte, operation string, partition equation.Partition) []equation.Fact {
	if !strings.HasPrefix(target, "path/") || (!strings.HasPrefix(string(source), "path/") && !strings.HasPrefix(string(source), "temp/")) {
		return nil
	}
	encodedSource := base64.RawURLEncoding.EncodeToString(source)
	encodedTarget := base64.RawURLEncoding.EncodeToString([]byte(target))
	var facts []equation.Fact
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, returnTupleTruePrefix) || string(fact.Value) != "proven" {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(fact.Key, returnTupleTruePrefix), "/")
		if len(parts) != 2 {
			continue
		}
		left, right := parts[0], parts[1]
		if left == encodedSource {
			left = encodedTarget
		}
		if right == encodedSource {
			right = encodedTarget
		}
		if left == parts[0] && right == parts[1] {
			continue
		}
		facts = append(facts, equation.Fact{Key: returnTupleTruePrefix + left + "/" + right, Value: []byte("proven")})
	}
	return facts
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
		// A table member transported from a completed typed operation is already
		// a value witness, not a source term awaiting another lookup.  In
		// particular, table.insert may append the generic-for element's sealed
		// target; resolving that encoding as a path would discard it as Top when
		// the returned table is rebound by the caller.
		if _, typed := shapefact.DecodeTarget([]byte(member.Value)); typed {
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
	var diagnostics []equation.Fact
	var originFacts []equation.Fact
	var boundarySources [][]byte
	var err error
	switch wir.Op(kind) {
	case wir.OpLogical:
		left, er := resolve("left")
		if er != nil {
			return equation.TransactionResult{}, er
		}
		if string(left) == "scalar/top" {
			// A Top left operand leaves either short-circuit result reachable.
			// Keep only an already-published gradual boundary from either operand;
			// the expression does not manufacture a boundary or a value.
			boundarySources = append(boundarySources, by["left"], by["right"])
			value = left
			break
		}
		truth, er := luaTruthy(left)
		if errors.Is(er, errUnknownScalar) && string(left) == optionalNilComparison {
			if wir.Operator(op) == wir.LogAnd {
				boundarySources = append(boundarySources, by["right"])
				value, err = resolve("right")
				break
			}
		}
		if errors.Is(er, errUnknownScalar) {
			// Neither short-circuit is decided, so both results stay reachable.
			// An undecided operand is not an evaluation failure: the expression
			// publishes the join of the two reachable outcomes.
			right, rightErr := resolve("right")
			if rightErr != nil {
				return equation.TransactionResult{}, rightErr
			}
			boundarySources = append(boundarySources, by["left"], by["right"])
			value = undecidedLogicalValue(left, right, wir.Operator(op))
			break
		}
		if er != nil {
			return equation.TransactionResult{}, er
		}
		if (wir.Operator(op) == wir.LogAnd && !truth) || (wir.Operator(op) == wir.LogOr && truth) {
			boundarySources = append(boundarySources, by["left"])
			value = left
		} else {
			boundarySources = append(boundarySources, by["right"])
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
			if errors.Is(er, errUnknownScalar) {
				// not on an undecided operand still yields a boolean; which one
				// it yields is exactly what remains unknown.
				value = []byte("scalar/boolean")
				break
			}
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
				if _, found := optionalProviderConcatWitness(term, partition); found {
					diagnostics = append(diagnostics, equation.Fact{
						Key:   fmt.Sprintf("type.operator.concat_operand/%s/value-%08d", operation.Target.Name, i),
						Value: []byte("concat operand may be nil"),
					})
					originFacts = append(originFacts, concatOperandOriginFacts(operation.Target.Name, i, term, partition)...)
				}
				value = v
				break
			}
			if concatOperandMayBeNil(v) {
				diagnostics = append(diagnostics, equation.Fact{
					Key:   fmt.Sprintf("type.operator.concat_operand/%s/value-%08d", operation.Target.Name, i),
					Value: []byte("concat operand may be nil"),
				})
				originFacts = append(originFacts, concatOperandOriginFacts(operation.Target.Name, i, term, partition)...)
				value = []byte("scalar/top")
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
		// Concatenation that reaches its normal result still produces a string.
		// A nilable operand remains a separately published warning, but must not
		// turn a following string annotation into an unrelated unproven claim.
		// The result type comes from this already-lowered operator, not an
		// annotation or a source-level special case.
		if len(diagnostics) != 0 && string(value) == "scalar/top" {
			if encoded, ok := shapefact.EncodeTarget(typ.String); ok {
				value = encoded
			}
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
		operator := wir.Operator(op)
		boundaryComparison := (operator == wir.BinEq || operator == wir.BinNe) &&
			(isExplicitAnyValue(left) || sourceHasAnyBoundary(by["left"], partition.Values()) ||
				isExplicitAnyValue(right) || sourceHasAnyBoundary(by["right"], partition.Values()))
		if string(left) == "scalar/top" || string(right) == "scalar/top" || boundaryComparison {
			value = []byte("scalar/top")
		} else {
			value, err = basicBinary(operator, left, right)
		}
	default:
		err = fmt.Errorf("engine: unsupported expression kind")
	}
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if string(value) == "scalar/top" && wir.Op(kind) == wir.OpBinOp {
		if message, refuted := orderedComparisonUnionDiagnostic(wir.Operator(op), by["left"], by["right"], partition); refuted {
			diagnostics = append(diagnostics, equation.Fact{
				Key:   "type.operator.comparison_operand/" + operation.Target.Name,
				Value: []byte(message),
			})
		}
	}
	// Exact scalar evaluation deliberately leaves broad, sealed type witnesses
	// at Top.  Before publishing that loss of precision, let the existing type
	// operator derive the result from those already-published witnesses.  This
	// keeps provider results such as integer(any) useful through arithmetic and
	// concatenation without inventing a concrete runtime value.
	if string(value) == "scalar/top" && len(diagnostics) == 0 {
		// A sealed contiguous literal has a proven sequence cardinality. Other
		// sealed tables retain the broad number result: Lua length is not a
		// member count for records, holes, or non-sequence shapes.
		if wir.Op(kind) == wir.OpUnOp && wir.Operator(op) == wir.UnLen {
			if operand, found := by["value"]; found {
				table, resolved := resolvedSealedTable(operand, partition)
				sealedReceiver := resolved && table.Closed
				// Lua's type predicate certifies the runtime kind, and that is
				// the whole receiver contract the length operator needs: a
				// validated table or string receiver has a length whatever its
				// members turn out to be.
				lengthReceiver := sealedReceiver ||
					runtimeTypeProven(operand, "table", partition) ||
					runtimeTypeProven(operand, "string", partition)
				if lengthReceiver {
					result := typ.Type(typ.Integer)
					if sealedReceiver {
						result = typ.Number
						if length, sequence := sealedSequenceLength(table); sequence {
							result = typ.LiteralInt(length)
						}
					}
					if encoded, encodedOK := shapefact.EncodeTarget(result); encodedOK {
						value = encoded
					}
				}
			}
		}
	}
	if string(value) == "scalar/top" && len(diagnostics) == 0 {
		if typed, ok := typedExpressionResult(wir.Op(kind), wir.Operator(op), by, partition); ok {
			value = typed
		}
	}
	values := []equation.Fact{{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: value}}
	values = append(values, originFacts...)
	if wir.Op(kind) == wir.OpLogical {
		for _, source := range boundarySources {
			if boundary, ok := gradualAnyBoundaryFact(string(result), source, operation.Target.Name, partition.Values()); ok {
				values = append(values, boundary)
				break
			}
		}
	}
	if wir.Op(kind) == wir.OpUnOp && wir.Operator(op) == wir.UnNot {
		if boundary, ok := gradualAnyBoundaryFact(string(result), by["value"], operation.Target.Name, partition.Values()); ok {
			values = append(values, boundary)
		}
	}
	if wir.Op(kind) == wir.OpBinOp && (wir.Operator(op) == wir.BinEq || wir.Operator(op) == wir.BinNe) {
		// Equality against an explicit-any boundary has no concrete boolean
		// result. Carry only the already-published boundary to the comparison
		// result so branch evaluation can retain the possible arm; the boundary
		// is not a type proof for either operand.
		for _, role := range []string{"left", "right"} {
			if root, _, found := explicitAnySourceFact(by[role], partition.Values()); found {
				values = append(values, equation.Fact{Key: "gradual-any/" + string(result) + "/" + operation.Target.Name, Value: root})
				break
			}
		}
		closedComparison := closedPlacementIdentityComparison(by["left"], by["right"], partition)
		for _, role := range []string{"left", "right"} {
			if allocation, found := placementAllocationForTerm(by[role], partition); found {
				if !closedComparison {
					values = append(values, placementBlockerFact(allocation.Identity, operation.Target.Name, "identity-compare"))
				} else {
					values = append(values, placementContractFact(allocation.Identity, "identity", operation.Target.Name))
				}
			}
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values, Diagnostics: diagnostics}}, nil
}

func closedPlacementIdentityComparison(left, right []byte, partition equation.Partition) bool {
	for _, term := range [][]byte{left, right} {
		allocation, found := placementAllocationForTerm(term, partition)
		if !found || !placementClosedAllocation(allocation, partition) {
			return false
		}
	}
	return true
}

func projectTypePredicateWriteRelation(target string, source []byte, operation string, partition equation.Partition) []equation.Fact {
	if !strings.HasPrefix(target, "path/") {
		return nil
	}
	encodedSource := base64.RawURLEncoding.EncodeToString(source)
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, typePredicatePairPrefix) {
			parts := strings.Split(strings.TrimPrefix(fact.Key, typePredicatePairPrefix), "/")
			if len(parts) == 2 && parts[0] == encodedSource {
				return []equation.Fact{{Key: typePredicateValuePrefix + base64.RawURLEncoding.EncodeToString([]byte(target)) + "/" + parts[1] + "/" + operation, Value: append([]byte(nil), fact.Value...)}}
			}
		}
		if strings.HasPrefix(fact.Key, typePredicateValuePrefix) {
			parts := strings.Split(strings.TrimPrefix(fact.Key, typePredicateValuePrefix), "/")
			if len(parts) == 3 && parts[1] == encodedSource {
				return []equation.Fact{{Key: typePredicatePairPrefix + parts[0] + "/" + base64.RawURLEncoding.EncodeToString([]byte(target)), Value: append([]byte(nil), fact.Value...)}}
			}
		}
	}
	return nil
}

// resolvedSealedTable accepts only the current closed literal fact for an
// expression operand. It is intentionally narrower than a declared table
// type: a declaration can be nil or widened, whereas the sealed value is the
// concrete table-kind evidence required by the length operator.
func resolvedSealedTable(term []byte, partition equation.Partition) (shapefact.Table, bool) {
	value, err := resolveCurrentValue(term, partition)
	if err != nil {
		return shapefact.Table{}, false
	}
	table, ok := shapefact.DecodeTable(value)
	return table, ok && table.Closed
}

// sealedSequenceLength proves an exact Lua length only for a closed, present,
// contiguous top-level integer sequence. A named key, nested member, hole, or
// zero index keeps the result broad because it has no border-free sequence
// proof.
func sealedSequenceLength(table shapefact.Table) (int64, bool) {
	if !table.Closed || len(table.Members) == 0 {
		return 0, false
	}
	seen := make(map[int]bool, len(table.Members))
	for _, member := range table.Members {
		if !member.Present {
			return 0, false
		}
		segments, parsed := segment.ParseFormattedSegments(member.Suffix)
		if !parsed || len(segments) != 1 || segments[0].Kind != segment.SegmentIndexInt {
			return 0, false
		}
		index := segments[0].Index
		if index < 1 || index > len(table.Members) || seen[index] {
			return 0, false
		}
		seen[index] = true
	}
	return int64(len(table.Members)), true
}

func typedExpressionResult(kind wir.Op, operator wir.Operator, operands map[string][]byte, partition equation.Partition) ([]byte, bool) {
	// Exact scalar arithmetic already has a concrete evaluator. This fallback
	// exists only to carry a sealed type witness across the expression seam;
	// applying it to plain scalar coercions would replace their intentionally
	// conservative Top result with an inferred static contract.
	hasWitness := func(role string) bool {
		term, found := operands[role]
		if !found {
			return false
		}
		value, err := resolveCurrentValue(term, partition)
		if err != nil {
			return false
		}
		_, witnessed := shapefact.DecodeTarget(value)
		return witnessed
	}
	witnessed := false
	switch kind {
	case wir.OpBinOp:
		witnessed = hasWitness("left") || hasWitness("right")
	case wir.OpUnOp:
		witnessed = hasWitness("value")
	case wir.OpConcat:
		for index := 0; ; index++ {
			role := fmt.Sprintf("value-%08d", index)
			if _, found := operands[role]; !found {
				break
			}
			witnessed = witnessed || hasWitness(role)
		}
	}
	if !witnessed {
		return nil, false
	}
	resolveType := func(role string) (typ.Type, bool) {
		term, found := operands[role]
		if !found {
			return nil, false
		}
		value, err := resolveCurrentValue(term, partition)
		if err != nil {
			return nil, false
		}
		return expressionValueType(value)
	}
	var result typ.Type
	var ok bool
	switch kind {
	case wir.OpBinOp:
		operatorText, operatorOK := expressionOperatorText(operator)
		if !operatorOK {
			return nil, false
		}
		left, leftOK := resolveType("left")
		right, rightOK := resolveType("right")
		if !leftOK || !rightOK {
			return nil, false
		}
		result, ok = typeoperator.BinaryOp(left, operatorText, right)
	case wir.OpUnOp:
		operatorText, operatorOK := expressionOperatorText(operator)
		if !operatorOK {
			return nil, false
		}
		value, valueOK := resolveType("value")
		if !valueOK {
			return nil, false
		}
		result, ok = typeoperator.UnaryOp(operatorText, value)
	case wir.OpConcat:
		for index := 0; ; index++ {
			value, valueOK := resolveType(fmt.Sprintf("value-%08d", index))
			if !valueOK {
				return nil, false
			}
			if index == 0 {
				result = value
				continue
			}
			result, ok = typeoperator.BinaryOp(result, "..", value)
			if !ok {
				return nil, false
			}
			if _, next := operands[fmt.Sprintf("value-%08d", index+1)]; !next {
				break
			}
		}
	default:
		return nil, false
	}
	if !ok || result == nil {
		return nil, false
	}
	return shapefact.EncodeTarget(result)
}

func expressionValueType(value []byte) (typ.Type, bool) {
	if value, ok := shapefact.DecodeTarget(value); ok {
		return value, true
	}
	switch {
	case string(value) == "scalar/nil":
		return typ.Nil, true
	case strings.HasPrefix(string(value), "scalar/bool/"):
		parsed, err := strconv.ParseBool(strings.TrimPrefix(string(value), "scalar/bool/"))
		return typ.LiteralBool(parsed), err == nil
	case strings.HasPrefix(string(value), "scalar/string/"):
		parsed, err := strconv.Unquote(strings.TrimPrefix(string(value), "scalar/string/"))
		return typ.LiteralString(parsed), err == nil
	case strings.HasPrefix(string(value), "scalar/number/"):
		parsed := strings.TrimPrefix(string(value), "scalar/number/")
		if integer, err := strconv.ParseInt(parsed, 10, 64); err == nil {
			return typ.LiteralInt(integer), true
		}
		if number, err := strconv.ParseFloat(parsed, 64); err == nil {
			return typ.LiteralNumber(number), true
		}
	}
	return nil, false
}

// orderedComparisonUnionDiagnostic publishes only a runtime failure already
// established by closed operand witnesses. A broad scalar or gradual type has
// no such proof and remains silent.
func orderedComparisonUnionDiagnostic(operator wir.Operator, leftTerm, rightTerm []byte, partition equation.Partition) (string, bool) {
	operatorText, ordered := orderedComparisonOperator(operator)
	if !ordered {
		return "", false
	}
	leftValue, leftErr := resolveCurrentValue(leftTerm, partition)
	rightValue, rightErr := resolveCurrentValue(rightTerm, partition)
	if leftErr != nil || rightErr != nil {
		return "", false
	}
	left, leftTyped := expressionValueType(leftValue)
	right, rightTyped := expressionValueType(rightValue)
	if !leftTyped || !rightTyped {
		return "", false
	}
	if _, valid := typeoperator.BinaryOp(left, operatorText, right); valid {
		return "", false
	}
	if unionAdmitsProvenNonNumber(left) || unionAdmitsProvenNonNumber(right) {
		return fmt.Sprintf("operator %s cannot compare %s with %s", operatorText, typeformat.Short(left), typeformat.Short(right)), true
	}
	return "", false
}

func orderedComparisonOperator(operator wir.Operator) (string, bool) {
	switch operator {
	case wir.BinLt:
		return "<", true
	case wir.BinLe:
		return "<=", true
	case wir.BinGt:
		return ">", true
	case wir.BinGe:
		return ">=", true
	default:
		return "", false
	}
}

func unionAdmitsProvenNonNumber(value typ.Type) bool {
	value = unwrap.Alias(unwrap.Annotations(value))
	union, ok := value.(*typ.Union)
	if !ok || union == nil || len(union.Members) == 0 || typ.ContainsAny(value) || typ.ContainsTypeParam(value) || inspect.ContainsUnknown(value) {
		return false
	}
	for _, member := range union.Members {
		if member != nil && !subtype.IsSubtype(member, typ.Number) {
			return true
		}
	}
	return false
}

// sealedShapeReceiverType reconstructs a diagnostic-only receiver type from
// an already sealed literal table. It accepts no open members, aliases, or
// inferred paths: every retained field must be a direct, present member with
// an independently concrete value. The result therefore explains an absent
// member without turning an untracked Lua table access into a type proof.
func sealedShapeReceiverType(value []byte) (typ.Type, bool) {
	if valueType, known := expressionValueType(value); known {
		return valueType, true
	}
	table, sealed := shapefact.DecodeTable(value)
	if !sealed || !table.Closed {
		return nil, false
	}
	builder := typetable.NewRecord()
	for _, member := range table.Members {
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid || len(segments) != 1 || !member.Present {
			return nil, false
		}
		memberType, concrete := sealedShapeReceiverType([]byte(member.Value))
		if !concrete {
			return nil, false
		}
		switch item := segments[0]; item.Kind {
		case segment.SegmentField:
			builder.Field(item.Name, memberType)
		case segment.SegmentIndexString:
			builder.StaticStringIndex(item.Name, memberType)
		case segment.SegmentIndexInt:
			builder.StaticIntIndex(int64(item.Index), memberType)
		default:
			return nil, false
		}
	}
	return builder.Build(), true
}

func expressionOperatorText(operator wir.Operator) (string, bool) {
	switch operator {
	case wir.BinAdd:
		return "+", true
	case wir.BinSub:
		return "-", true
	case wir.BinMul:
		return "*", true
	case wir.BinDiv:
		return "/", true
	case wir.BinIDiv:
		return "//", true
	case wir.BinMod:
		return "%", true
	case wir.BinPow:
		return "^", true
	case wir.BinBAnd:
		return "&", true
	case wir.BinBOr:
		return "|", true
	case wir.BinBXor:
		return "~", true
	case wir.BinShl:
		return "<<", true
	case wir.BinShr:
		return ">>", true
	case wir.BinEq:
		return "==", true
	case wir.BinNe:
		return "~=", true
	case wir.BinLt:
		return "<", true
	case wir.BinLe:
		return "<=", true
	case wir.BinGt:
		return ">", true
	case wir.BinGe:
		return ">=", true
	case wir.UnNeg:
		return "-", true
	case wir.UnNot:
		return "not", true
	case wir.UnLen:
		return "#", true
	case wir.UnBNot:
		return "~", true
	}
	return "", false
}

// concatOperandMayBeNil accepts only a closed nil value or an explicit
// optional annotation fact. Top and other gradual values remain silent: they
// have no proof that a nil can reach the operand.
func concatOperandMayBeNil(value []byte) bool {
	if string(value) == "scalar/nil" {
		return true
	}
	// A finite provider result can retain nilability as a sealed type witness
	// instead of a textual annotation refinement. It is still the exact value
	// reaching this concat operand, so preserve its established optional proof.
	if optionalConcreteWitness(value) {
		return true
	}
	const marker = "claim-type/"
	index := strings.LastIndex(string(value), marker)
	if index < 0 {
		return false
	}
	declared, err := strconv.Unquote(string(value)[index+len(marker):])
	return err == nil && strings.HasSuffix(declared, "?")
}

// concatOperandOriginFacts classifies why the operand this transaction just
// refuted can be nil. The classification reads only facts already closed in
// this partition: the operand's own declared optional field, or the optional
// result contract published by the call that produced it. Publication renders
// the classification; it never re-derives the cause from source.
func concatOperandOriginFacts(operation string, index int, term []byte, partition equation.Partition) []equation.Fact {
	key := fmt.Sprintf("%s%s/value-%08d", concatOperandOriginPrefix, operation, index)
	if subject, found := currentEpochFact(optionalResultOriginPrefix, term, partition); found && len(subject) != 0 {
		return []equation.Fact{{Key: key, Value: []byte(concatOriginOptionalResult + string(subject))}}
	}
	if concatOperandOptionalField(term, partition) {
		return []equation.Fact{{Key: key, Value: []byte(concatOriginOptionalField)}}
	}
	return nil
}

// concatOperandOptionalField accepts only a member read whose ancestor's
// current published type projects an optional field at that exact suffix. A
// root path or an unprojectable ancestor has no field origin to report.
func concatOperandOptionalField(term []byte, partition equation.Partition) bool {
	path := strings.TrimPrefix(string(term), "path/")
	if path == string(term) {
		return false
	}
	for cut := len(path); cut > 0; {
		cut = strings.LastIndexAny(path[:cut], ".[")
		if cut < 0 {
			return false
		}
		segments, valid := segment.ParseFormattedSegments(path[cut:])
		if !valid {
			return false
		}
		declared, found := declaredTypeForTerm([]byte("path/"+path[:cut]), partition)
		if !found || declared == nil {
			continue
		}
		projected, ok := luatypeprojection.ApplySegments(declared, segments)
		return ok && projected != nil && proof.OptionalTypeHasConcreteValue(projected)
	}
	return false
}

// optionalProviderConcatWitness reads a provider's optional result only from
// the same call-results operation that owns the current value term. The value
// itself remains Top: this fact is limited to the concat consumer's nilability
// obligation and cannot be reused as an assignment or member-access proof.
func optionalProviderConcatWitness(term []byte, partition equation.Partition) ([]byte, bool) {
	prefix := "value/" + string(term) + "/"
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && fact.Key > latest {
			latest = fact.Key
		}
	}
	if latest == "" {
		return nil, false
	}
	operation := strings.TrimPrefix(latest, prefix)
	want := "optional-provider-result/" + string(term) + "/" + operation
	for _, fact := range partition.Values() {
		if fact.Key == want {
			return append([]byte(nil), fact.Value...), true
		}
	}
	return nil, false
}

func numberValue(v float64) []byte {
	return []byte("scalar/number/" + strconv.FormatFloat(v, 'g', -1, 64))
}
func basicBinary(op wir.Operator, a, b []byte) ([]byte, error) {
	switch op {
	case wir.BinEq:
		if (optionalConcreteWitness(a) && string(b) == "scalar/nil") || (optionalConcreteWitness(b) && string(a) == "scalar/nil") {
			return []byte(optionalNilComparison), nil
		}
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

func optionalConcreteWitness(value []byte) bool {
	witness, ok := shapefact.DecodeTarget(value)
	return ok && optionalConcreteWitnessType(witness)
}

const optionalNilComparison = "scalar/bool/optional-nil-comparison"

func optionalConcreteWitnessType(witness typ.Type) bool {
	return witness != nil && proof.OptionalTypeHasConcreteValue(witness)
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
	var declaredContract []byte
	for _, operand := range operation.Operands {
		if operand.Role != "declared-type" {
			continue
		}
		if declared, ok := shapefact.DecodeTarget(operand.Value); ok && declared != nil {
			declaredContract = append([]byte(nil), operand.Value...)
		}
		break
	}
	result, err := frozenMutationDiagnostic(operation, partition, "assignment")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if diagnostic, witness, refuted := optionalWriteContainerDiagnostic(operation, string(operands["display"]), partition); refuted {
		result.Closure.Diagnostics = append(result.Closure.Diagnostics, diagnostic)
		result.Closure.Values = append(result.Closure.Values, witness)
	}
	// A typed dotted function definition has already published an exact
	// callable contract at this path. A later static write may refute that
	// contract only with a concrete non-callable value; opaque callables and
	// unknown values remain unreported rather than being treated as proof.
	if existing, found := declaredTypeForTerm(operands["target"], partition); found && functionContractWriteRefuted(value, existing) {
		result.Closure.Diagnostics = append(result.Closure.Diagnostics, equation.Fact{
			Key:   "type.assignment/" + operation.Target.Name,
			Value: []byte(fmt.Sprintf("cannot assign %s because assigned value is %s, not %s", operands["display"], assignmentEvidenceValue(value), functionContractDisplay(existing))),
		})
		result.Closure.Values = append(result.Closure.Values, equation.Fact{Key: "assignment-function-contract/" + operation.Target.Name, Value: []byte("refuted")})
	}
	if len(declaredContract) != 0 {
		result.Closure.Values = append(result.Closure.Values, equation.Fact{
			Key:   "declared-type/" + string(operands["target"]) + "/" + operation.Target.Name,
			Value: declaredContract,
		})
	}
	result.Closure.Values = append(result.Closure.Values, equation.Fact{
		Key: "value/" + string(operands["target"]) + "/" + operation.Target.Name, Value: value,
	})
	// Every replacement advances the target's current version.  Nested member
	// cells resolve their parent through this epoch, so omitting it would leave
	// a replacement table reachable only by its stale predecessor identity.
	result.Closure.Values = append(result.Closure.Values, equation.Fact{
		Key: epochFactPrefix + string(operands["target"]) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name),
	})
	memberValues, memberValueErr := projectSealedTableMemberValues(string(operands["target"]), value, operation.Target.Name)
	if memberValueErr != nil {
		return equation.TransactionResult{}, memberValueErr
	}
	result.Closure.Values = append(result.Closure.Values, memberValues...)
	// A static member write can be the first publication of a local closure
	// capability (for example, `function module.f() ... end`).  The value fact
	// alone proves that the member is callable, but it does not authorize the
	// lexical evaluator to demand that particular closed body.  Forward only
	// the already-published handle from the written value; an opaque callable
	// remains a value-only fact and cannot acquire local-body authority here.
	if handle, found := closureHandleFor(operands["value"], partition); found {
		encoded, marshalErr := json.Marshal(handle)
		if marshalErr != nil {
			return equation.TransactionResult{}, marshalErr
		}
		result.Closure.Values = append(result.Closure.Values, equation.Fact{
			Key: "closure/" + string(operands["target"]) + "/" + operation.Target.Name, Value: encoded,
		})
	}
	// Replacing an indexed path with a sealed table preserves the table value
	// and its exact callable-member capabilities together. A later method call
	// may demand only those capabilities that were already published by the
	// literal; an opaque or subsequently replaced member remains unavailable.
	memberClosures, memberErr := projectMemberClosures(string(operands["target"]), operands["value"], operation.Target.Name, partition)
	if memberErr != nil {
		return equation.TransactionResult{}, memberErr
	}
	result.Closure.Values = append(result.Closure.Values, memberClosures...)
	literalMemberClosures, literalErr := projectSealedTableMemberClosures(string(operands["target"]), operands["value"], operation.Target.Name, partition)
	if literalErr != nil {
		return equation.TransactionResult{}, literalErr
	}
	result.Closure.Values = append(result.Closure.Values, literalMemberClosures...)
	if root, suffix, ok := heapTableAddress(operands["target"]); ok && suffix != "" {
		if identity, found := tableIdentityForTerm(root, partition); found {
			writeIdentity := identity
			// Lua dispatches a write to a missing key through a table-valued
			// __newindex metamethod. The destination can itself carry another
			// metatable, so retain only Top at the routed member: proving the
			// direct table write would be unsound, while a concrete forwarded value
			// would overstate the incomplete metamethod model.
			if _, present := heapMemberCurrent(heapMemberPrefix, identity, suffix, partition); !present && heapTableClosed(identity, partition) {
				if routed, found := heapMetaNewIndexCurrent(identity, partition); found {
					writeIdentity = routed
					value = []byte("scalar/top")
				}
			}
			result.Closure.Values = append(result.Closure.Values, heapMemberFact(writeIdentity, suffix, operation.Target.Name, value))
			memberIdentity, _ := tableIdentityForTerm(operands["value"], partition)
			if len(memberIdentity) != 0 {
				result.Closure.Values = append(result.Closure.Values, heapIdentityFact(string(operands["target"]), operation.Target.Name, memberIdentity))
			}
			if cell, published, cellErr := memberCellFactWithIdentity(writeIdentity, suffix, operation.Target.Name, value, memberIdentity, partition); cellErr != nil {
				return equation.TransactionResult{}, cellErr
			} else if published {
				result.Closure.Values = append(result.Closure.Values, cell)
			}
			if writeIdentity == nil || string(writeIdentity) == string(identity) {
				if len(memberIdentity) != 0 {
					result.Closure.Values = append(result.Closure.Values, heapMemberIdentityFact(identity, suffix, operation.Target.Name, memberIdentity))
				}
			}
			// A complete static path can cross an already-published member identity
			// before it reaches its final slot. Mirror the exact replacement at
			// that nested identity so an equivalent alias observes the same write.
			// The walk accepts only existing heap links; an unresolved prefix keeps
			// the replacement local to its original source path.
			if nestedIdentity, nestedSuffix, nested := nestedHeapMemberAddress(identity, suffix, partition); nested {
				result.Closure.Values = append(result.Closure.Values, heapMemberFact(nestedIdentity, nestedSuffix, operation.Target.Name, value))
				if len(memberIdentity) != 0 {
					result.Closure.Values = append(result.Closure.Values, heapMemberIdentityFact(nestedIdentity, nestedSuffix, operation.Target.Name, memberIdentity))
					result.Closure.Values = append(result.Closure.Values, heapStaticReplacementFact(memberIdentity, operation.Target.Name))
				}
			}
		}
		parent, hasParent := placementAllocationForTerm(root, partition)
		child, hasChild := placementAllocationForTerm(operands["value"], partition)
		if hasChild {
			result.Closure.Values = append(result.Closure.Values, placementBindingFact(string(operands["target"]), operation.Target.Name, child.Identity))
		}
		switch {
		case hasParent && hasChild && parent.Identity != child.Identity:
			result.Closure.Values = append(result.Closure.Values, placementContainmentFact(parent.Identity, child.Identity, operation.Target.Name))
		case hasChild && !hasParent:
			// The destination container has no allocation in this frame: it is a
			// boundary cell, an import, or a global. The stored allocation therefore
			// outlives this frame and cannot keep a stack lifetime. Ownership follows
			// the container, so record the retain rather than a lifetime blocker.
			result.Closure.Values = append(result.Closure.Values, placementEventFact(child.Identity, operation.Target.Name, placementEventOwned))
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

// functionContractWriteRefuted accepts only an exact declared callable and a
// concrete incompatible replacement. A bare scalar/function transport is not
// a signature witness, so it stays unknown rather than being rejected.
func functionContractWriteRefuted(value []byte, declared typ.Type) bool {
	function, ok := unwrap.Alias(subst.ExpandInstantiated(declared)).(*typ.Function)
	if !ok || function == nil {
		return false
	}
	if actual, sealed := sealedFunctionType(value); sealed {
		return !subtype.IsSubtype(actual, function)
	}
	if strings.HasPrefix(string(value), "scalar/function") {
		return false
	}
	return knownScalarRelation(value, false) == shapeRefuted
}

func functionContractDisplay(declared typ.Type) string {
	function, ok := unwrap.Alias(subst.ExpandInstantiated(declared)).(*typ.Function)
	if !ok || function == nil {
		return typeformat.Short(declared)
	}
	params := make([]string, 0, len(function.Params))
	for _, param := range function.Params {
		if param.Type == nil {
			return typeformat.Short(declared)
		}
		params = append(params, typeformat.Short(param.Type))
	}
	returns := make([]string, 0, len(function.Returns))
	for _, returned := range function.Returns {
		if returned == nil {
			return typeformat.Short(declared)
		}
		returns = append(returns, typeformat.Short(returned))
	}
	return "fun(" + strings.Join(params, ", ") + ") -> " + strings.Join(returns, ", ")
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
	values := []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: value}, {Key: epochFactPrefix + target + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)}}
	if identity, found := tableIdentityForTerm(operands["container"], partition); found {
		key, keyErr := resolveCurrentValue(operands["key"], partition)
		if suffix, exact := tableMemberSuffix(key, []byte("suffix/")); keyErr == nil && exact {
			if member, found := heapMemberCurrent(heapMemberPrefix, identity, suffix, partition); found {
				values[0].Value = member
			} else if heapTableClosed(identity, partition) && !heapMetaAttached(identity, partition) && !heapHasExternalCallback(identity, partition) {
				values[0].Value = []byte("scalar/nil")
			}
			if memberIdentity, found := heapMemberCurrent(heapMemberIdentityPrefix, identity, suffix, partition); found {
				values = append(values, heapIdentityFact(target, operation.Target.Name, memberIdentity))
			}
		}
	}
	// A sealed type witness is a separate, already-published authority from a
	// heap identity. It can describe a cast or provider result whose members
	// have not been materialized locally. RuntimeIndex preserves Lua's missing
	// slot nilability, and providerReturnTypeValue rejects open, generic, and
	// any-shaped answers before they can reach this result slot.
	if string(values[0].Value) == "scalar/top" {
		if projected, display, scalar, optional, ok := typedRuntimeIndexResult(operands["container"], operands["key"], partition); ok {
			values[0].Value = projected
			values = append(values, equation.Fact{Key: indexReadDisplayPrefix + target + "/" + operation.Target.Name, Value: []byte(display)})
			if scalar {
				values = append(values, equation.Fact{Key: indexReadScalarPrefix + target + "/" + operation.Target.Name, Value: []byte("scalar")})
			}
			if optional {
				values = append(values, equation.Fact{Key: typedOptionalReadPrefix + target + "/" + operation.Target.Name, Value: []byte("typed")})
			}
		}
	}
	// Reading through an explicit-any boundary cannot validate the selected
	// member, but it does preserve the already-published boundary itself. The
	// downstream consumer may therefore reject a typed use instead of treating
	// this exact unvalidated read as an opaque Top value.
	if root, _, explicit := explicitAnySourceFact(operands["container"], partition.Values()); explicit {
		values = append(values, equation.Fact{Key: "gradual-any/" + target + "/" + operation.Target.Name, Value: root})
	}
	if allocation, found := placementAllocationForTerm(operands["container"], partition); found {
		key, keyErr := resolveCurrentValue(operands["key"], partition)
		_, exactKey := tableMemberSuffix(key, []byte("suffix/"))
		if keyErr != nil || !exactKey || !placementClosedAllocation(allocation, partition) {
			values = append(values, placementBlockerFact(allocation.Identity, operation.Target.Name, "dynamic-index"))
		} else {
			values = append(values, placementContractFact(allocation.Identity, "dynamic-index", operation.Target.Name))
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

func typedRuntimeIndexResult(container, key []byte, partition equation.Partition) ([]byte, string, bool, bool, bool) {
	containerValue, err := resolveCurrentValue(container, partition)
	if err != nil {
		return nil, "", false, false, false
	}
	containerType, ok := shapefact.DecodeTarget(containerValue)
	if !ok && isUnknownScalar(containerValue) {
		// A local declared union return remains Top as a runtime value until a
		// guard selects an arm. Its guarded, epoch-current summary is still an
		// existing type publication: RuntimeIndex can use it to retain Lua's
		// nilability for an unguarded member read, without fabricating a table
		// shape or granting either union arm as a value proof.
		if encoded, found := currentEpochFact(summaryTypePrefix, container, partition); found {
			containerType, err = typ.DecodeCanonical(context.Background(), encoded)
			ok = err == nil && containerType != nil
		}
	}
	if !ok && strings.HasPrefix(string(containerValue), "scalar/claim/claim-kind/1/") {
		containerType, ok = castTargetWitness(container, partition)
	}
	// A conservative current scalar is not a structural refutation. When the
	// exact path already has a published type summary, retain that independent
	// authority for this one RuntimeIndex projection. The summary is produced
	// by the normal import/call transport; absent such a publication, the read
	// remains Top.
	if !ok {
		containerType, ok = typedPathType(container, partition)
	}
	// A published type witness may have unrelated open members. RuntimeIndex
	// still derives only the selected member, so an exact selected member stays
	// available without treating the container itself as a closed return value.
	if !ok || containerType == nil {
		return nil, "", false, false, false
	}
	keyValue, err := resolveCurrentValue(key, partition)
	if err != nil {
		return nil, "", false, false, false
	}
	keyType, ok := expressionValueType(keyValue)
	if !ok || keyType == nil {
		return nil, "", false, false, false
	}
	result, ok := access.RuntimeIndex(containerType, keyType)
	if !ok && optionalConcreteWitnessType(containerType) {
		result, ok = access.RuntimeIndex(proof.ProjectionWithoutNil(containerType), keyType)
		if ok {
			result = typ.MaterializeOptional(result)
		}
	}
	if !ok {
		return nil, "", false, false, false
	}
	if indexPresenceProven(container, key, partition) {
		if present := typetable.PresentReadonlyEntryValue(result); present != nil {
			result = present
		}
	}
	encoded, ok := finiteReturnWitnessValue(result)
	if !ok {
		return nil, "", false, false, false
	}
	_, scalar := optionalEvidenceDisplay(result)
	return encoded, typeformat.Short(proof.ProjectionWithoutNil(result)), scalar, optionalConcreteWitnessType(result), true
}

func castTargetWitness(term []byte, partition equation.Partition) (typ.Type, bool) {
	prefix := "cast-target/" + string(term) + "/"
	var witness []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (witness == nil || fact.Key > latest) {
			witness, latest = fact.Value, fact.Key
		}
	}
	if witness == nil {
		return nil, false
	}
	return shapefact.DecodeTarget(witness)
}

// heapIndexSubject binds an index proof to the sealed table identity when one
// exists. Falling back to the exact term remains conservative: no unproven
// alias can inherit a path-local proof.
func heapIndexSubject(container []byte, partition equation.Partition) string {
	if identity, found := tableIdentityForTerm(container, partition); found {
		return "identity/" + base64.RawURLEncoding.EncodeToString(identity)
	}
	return "term/" + base64.RawURLEncoding.EncodeToString(container)
}

// indexPresenceProven accepts only a current guarded proof for this container
// and key. Any later dynamic write through the same heap identity revokes it.
func indexPresenceProven(container, index []byte, partition equation.Partition) bool {
	subject := heapIndexSubject(container, partition)
	prefix := heapIndexPresencePrefix + subject + "/" + base64.RawURLEncoding.EncodeToString(index) + "/"
	proof := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "proven" && fact.Key > proof {
			proof = fact.Key
		}
	}
	if proof == "" {
		return false
	}
	revokePrefix := heapIndexRevokePrefix + subject + "/"
	revocation := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, revokePrefix) && string(fact.Value) == "revoked" && fact.Key > revocation {
			revocation = fact.Key
		}
	}
	return revocation == "" || factOperation(proof) > factOperation(revocation)
}

func factOperation(key string) string {
	_, operation, found := strings.Cut(key, "/op-")
	if !found {
		return ""
	}
	return "op-" + operation
}

// claimKernel makes user claims explicit checked refinements. An unproven
// claim remains a downstream assumption but never becomes reusable proof.
func claimKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition, imported map[string]bool) (equation.TransactionResult, error) {
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
		}
	}
	if (!strings.HasPrefix(target, "path/") && !strings.HasPrefix(target, "temp/")) || !validClaimKind(kind) || !validClaimType(kind, targetType) {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed claim")
	}
	throwTemplate := claimAssertThrowTemplate(operation, kind)
	value, available, err := resolveClaimValue(source, partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	shapeRelation := shapeUnknown
	var (
		memberSurface []byte
		shapeTarget   []byte
	)
	for _, operand := range operation.Operands {
		if operand.Role == "shape-target" {
			shapeTarget = operand.Value
			shapeRelation = assignmentShapeRelation(lexical, source, value, operand.Value, partition)
			if shapeRelation == shapeRefuted {
				if target, ok := shapefact.DecodeTarget(operand.Value); ok {
					memberSurface, _ = lexicalMemberCallableSurface(lexical, source, target, partition)
				}
			}
			break
		}
	}
	// A channel cast from nil is an explicit static boundary used by select
	// expressions.  Keep nil as the runtime value, but publish the exact
	// already-lowered Channel<T> contract so the select consumer can establish
	// its symbolic arm identity.  This admits neither a broad claim nor an
	// arbitrary interface cast.
	channelCastFromNil := false
	if kind == "claim-kind/1" && string(value) == "scalar/nil" {
		if target, ok := shapefact.DecodeTarget(shapeTarget); ok {
			_, channelCastFromNil = ambient.ChannelPayloadType(target)
		}
	}
	// A sealed table whose declared record includes callable members retains a
	// concrete dispatch witness even when function-variance proof is outside
	// this scalar relation. Do not turn that missing compatibility proof into a
	// diagnostic; apply can still emit only a proven call-contract violation.
	callableRecordShape := shapefact.IsTable(value) && strings.Contains(targetType, "fun(")
	// A concrete member fact may describe the literal that happened to cross
	// an explicit-any boundary, but it cannot discharge a later declared
	// assignment contract. The boundary itself is already-published evidence
	// and remains authoritative until validation supplies a separate proof.
	anySource := isUnvalidatedAnyValue(value) || sourceHasAnyBoundary(source, partition.Values()) || providerAnyResultBoundary(source, partition)
	// An explicit-any declaration remains a boundary, but an exact sealed
	// member may still be the concrete counterexample for its assignment
	// diagnostic. Keep that existing heap publication separate from the
	// boundary's proof status: it explains a refutation without validating the
	// surrounding aggregate.
	boundaryShape := []byte(nil)
	if anySource {
		if shapefact.IsTable(value) {
			boundaryShape = append([]byte(nil), value...)
		} else if sealed, found := heapMemberValue(source, partition); found && shapefact.IsTable(sealed) {
			boundaryShape = sealed
		}
	}
	// A runtime type test is the boundary's own validator. Its proof certifies
	// exactly one runtime kind, and runtimeTypeProofAdmitsTarget already keeps
	// a structural target (which shares the "table" kind without satisfying its
	// member contract) out of that proof. What remains is a target the test
	// decides outright, so the validated boundary proves the annotation instead
	// of merely being exempt from demanding a proof.
	boundaryValidated := kind == "claim-kind/3" && anySource && assignmentTargetRequiresProof(targetType) &&
		runtimeTypeValidationProves(source, targetType, shapeTarget, partition)
	boundaryRequiresProof := kind == "claim-kind/3" && anySource && assignmentTargetRequiresProof(targetType) && !boundaryValidated
	castFromAnyBoundary := kind == "claim-kind/1" && sourceHasAnyBoundary(source, partition.Values())
	if available && !boundaryRequiresProof && !castFromAnyBoundary && (claimProven(value, kind, targetType) || shapeRelation == shapeProven || channelCastFromNil || boundaryValidated) {
		closure := equation.OutputClosure{Values: []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: value}}}
		if throwTemplate.Key != "" {
			closure.Values = append(closure.Values, throwTemplate)
		}
		// A successfully checked annotation is a closed type publication for
		// this exact binding. Later aggregate literals may consume that
		// publication through their recorded member origins; an unproven claim
		// deliberately publishes no type authority.
		if kind == "claim-kind/3" && len(shapeTarget) != 0 {
			closure.Values = append(closure.Values, equation.Fact{Key: "type/" + target + "/" + operation.Target.Name, Value: append([]byte(nil), shapeTarget...)})
		}
		// A cast becomes a reusable type witness only when the cast's own
		// structural relation has already been proven. The assertion target on
		// an unchecked cast remains non-authoritative, so it cannot make a
		// later aggregate annotation pass by itself.
		if kind == "claim-kind/1" && (shapeRelation == shapeProven || channelCastFromNil) && len(shapeTarget) != 0 {
			closure.Values = append(closure.Values, equation.Fact{Key: "type/" + target + "/" + operation.Target.Name, Value: append([]byte(nil), shapeTarget...)})
		}
		if channelCastFromNil {
			closure.Values = append(closure.Values, equation.Fact{Key: epochFactPrefix + target + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)})
		}
		if kind == "claim-kind/3" && claimTypeIsAny(targetType) {
			closure.Values = append(closure.Values, explicitAnyBoundaryFact(target, operation.Target.Name))
		}
		if kind == "claim-kind/1" {
			// A successful cast has already established its structural target
			// from the source value. Preserve that checked witness for the exact
			// result slot; casts fed by an any boundary never enter this branch.
			if witness, ok := shapefact.DecodeTarget(shapeTarget); ok && witness != nil {
				closure.Values = append(closure.Values, equation.Fact{Key: "cast-target/" + target + "/" + operation.Target.Name, Value: append([]byte(nil), shapeTarget...)})
			}
			if !channelCastFromNil {
				closure.Diagnostics = []equation.Fact{{Key: "advice.redundant_claim/" + operation.Target.Name, Value: []byte("proven runtime claim")}}
			}
		}
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	refined := []byte("scalar/claim/" + kind + "/" + targetType)
	closure := equation.OutputClosure{Values: []equation.Fact{{Key: "value/" + target + "/" + operation.Target.Name, Value: refined}}}
	if throwTemplate.Key != "" {
		closure.Values = append(closure.Values, throwTemplate)
	}
	if kind == "claim-kind/1" {
		for _, operand := range operation.Operands {
			if operand.Role != "shape-target" {
				continue
			}
			if witness, ok := shapefact.DecodeTarget(operand.Value); ok && finiteReturnWitness(witness, make(map[typ.Type]bool)) {
				// The cast witness belongs only to this exact result term. It may
				// drive a direct consumer (such as an indexed read), but must not
				// become member-origin authority for a later aggregate claim.
				closure.Values = append(closure.Values, equation.Fact{Key: "cast-target/" + target + "/" + operation.Target.Name, Value: append([]byte(nil), operand.Value...)})
			}
			break
		}
	}
	if kind == "claim-kind/3" && claimTypeIsAny(targetType) {
		closure.Values = append(closure.Values, explicitAnyBoundaryFact(target, operation.Target.Name))
	}
	// A claim can refine a separate path without erasing the explicit boundary
	// value it consumed. Preserve that exact existing fact for a later branch
	// or assignment on a member path; Top values and ordinary refinements are
	// deliberately not forwarded as boundary evidence. An in-place annotation
	// reads and writes one cell, so its refinement is the only value this
	// operation may publish for that cell.
	if string(source) != target && strings.HasPrefix(string(source), "path/") && isExplicitAnyValue(value) {
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(source) + "/" + operation.Target.Name, Value: append([]byte(nil), value...)})
	}
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
	mapReadMissing := declaredOptionalMapReadMissingSlot(source, partition)
	if kind == "claim-kind/3" && available && memberMissing(value) {
		closure.Diagnostics = []equation.Fact{{
			Key:   "type.member.missing/" + operation.Target.Name,
			Value: []byte(memberMissingMessage(sourceDisplay, value)),
		}}
		// A declaration without an initializer reads its own Lua nil slot. That
		// slot establishes the declared local's downstream contract; it is not an
		// assignment of nil to the declaration type.
	} else if kind == "claim-kind/3" && (string(source) != target || string(value) != "scalar/nil") && available && (anySource && assignmentTargetRequiresProof(targetType) || assignmentMismatchProven(value, targetType) || shapeRelation == shapeRefuted || publishedOptionalAssignmentWitness(source, shapeTarget, partition) || mapReadMissing) {
		message := assignmentMismatchMessage(sourceDisplay, value, targetType)
		optionalSource := optionalAssignmentSource(value, targetType) || closedLiteralDeclaredOptionalMemberSource(source, value, shapeTarget, partition) || publishedOptionalAssignmentWitness(source, shapeTarget, partition)
		if declared := boundClaimDeclaredDisplay(operation, targetType); declared != "" {
			actual := assignmentValueType(value)
			if literal, found := literalDiagnosticValue(source, partition); found {
				actual = assignmentEvidenceValue(literal)
			}
			message = "cannot assign " + sourceDisplay + " because it is " + actual + ", not " + declared
		}
		for _, operand := range operation.Operands {
			if operand.Role != "shape-target" {
				continue
			}
			if declared, ok := shapefact.DecodeTarget(operand.Value); ok {
				if mismatch, found := firstAssignmentMismatch(value, declared); found {
					message = "cannot assign " + sourceDisplay + mismatch.Suffix + " because it is " + assignmentEvidenceValue(mismatch.Value) + ", not " + typeformat.Short(mismatch.Expected)
				} else if field, _, found := missingRequiredField(value, declared); found {
					message = fmt.Sprintf("object literal is missing required field %q", field)
				}
			}
			break
		}
		if anySource {
			message = assignmentAnyMismatchMessage(sourceDisplay, targetType, shapeTarget)
			if len(boundaryShape) != 0 {
				if declared, decoded := shapefact.DecodeTarget(shapeTarget); decoded && declared != nil && valueAgainstType(boundaryShape, declared) == shapeRefuted {
					message = "cannot assign " + sourceDisplay + " because it is " + boundaryShapeEvidenceValue(boundaryShape) + ", not " + typeformat.Short(declared)
				}
			}
		} else if memberSurface != nil {
			if _, ok := lexicalMemberCallableDisplay(lexical, operation); ok {
				message = "cannot assign " + sourceDisplay + " because it is fun() -> " + assignmentEvidenceValue(memberSurface) + ", not " + callableClaimDisplay(lexical, operation)
			}
		}
		if optionalSource {
			message = "cannot assign " + sourceDisplay + " because it may be nil"
		}
		if !anySource && optionalAssignmentWitness(source, value, shapeTarget, partition) {
			message = "cannot assign " + sourceDisplay + " because it may be nil"
		}
		if mapReadMissing {
			message = assignmentMapReadMissingPrefix + message
		}
		closure.Diagnostics = []equation.Fact{{
			Key:   "type.assignment/" + operation.Target.Name,
			Value: []byte(message),
		}}
		if memberSurface != nil {
			closure.Values = append(closure.Values, equation.Fact{Key: "assignment-member-surface/" + operation.Target.Name, Value: memberSurface})
		}
	} else if (!isUnknownScalar(value) || !importedResultPath(string(source), imported) || targetType != "claim-type/\"string\"") && !claimTypeIsAny(targetType) && !callableRecordShape && !(kind == "claim-kind/3" && string(source) == target && string(value) == "scalar/nil") && !guardedLocalCallResultClaim(lexical, operation, source) {
		// The closure keys facts by identity, so separate unproven claims must
		// retain their operation identity.
		closure.Diagnostics = []equation.Fact{{Key: "claim/unproven/" + operation.Target.Name, Value: []byte("claim " + strings.TrimPrefix(targetType, "claim-type/") + " is not proven")}}
	}
	return equation.TransactionResult{Complete: true, Closure: closure}, nil
}

// claimAssertThrowTemplate publishes the terminal behavior that the existing
// claim occurrence proves structurally. It is not a value refinement: an
// unchecked assertion still leaves its result claimed, while the operation's
// nil throw arm remains a proven property of the lowered ClaimAssert itself.
func claimAssertThrowTemplate(operation equation.BoundEquation, kind string) equation.Fact {
	if kind != claimAssertKind {
		return equation.Fact{}
	}
	return equation.Fact{
		Key:   "throw_template/" + operation.Target.Name,
		Value: []byte(claimAssertThrowTemplateValue),
	}
}

const (
	// claimAssertKind is the claim occurrence encoding of a non-nil assertion.
	claimAssertKind = "claim-kind/2"
	// claimAssertThrowTemplateValue is the single published spelling of that
	// assertion's terminal contract, shared with the native projection of a
	// frozen body so one occurrence never carries two vocabularies.
	claimAssertThrowTemplateValue = "allocates=false;false_arm=passes;kind=claim_assert;nil_arm=throws;preserves_word_on_success=true"
)

// evalNodeKernel publishes only the structural operation named by a NodeEval
// coordinate. It deliberately carries no type, dispatch, lifetime, or
// reachability conclusion.
func evalNodeKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "operation")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	name := string(operands["operation"])
	if name != "closure" && name != "length" {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed evaluation node")
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
		Key:   "eval_node/" + operation.Target.Name,
		Value: []byte("operation=" + name),
	}}}}, nil
}

// guardedLocalCallResultClaim recognizes an annotation immediately consuming a
// result from a local closure beneath an existing branch guard. The compiled
// apply -> call-results -> write chain is the only authority: an imported or
// unknown call keeps the ordinary fail-closed unproven-claim lint.
func guardedLocalCallResultClaim(lexical *lexicalEvaluator, operation equation.BoundEquation, source []byte) bool {
	if lexical == nil || len(operation.Guards) == 0 || (!strings.HasPrefix(string(source), "path/") && !strings.HasPrefix(string(source), "temp/")) {
		return false
	}
	compilation, found := lexical.byBody[operation.Target.Body]
	if !found {
		return false
	}
	result := string(source)
	if strings.HasPrefix(result, "path/") {
		result = ""
		for _, candidate := range compilation.Artifact.Equations {
			if candidate.Occurrence.Kind != "environment-write" {
				continue
			}
			target, hasTarget := artifactOperand(candidate.Operands, "target")
			value, hasValue := artifactOperand(candidate.Operands, "value")
			if hasTarget && hasValue && string(target) == string(source) && strings.HasPrefix(string(value), "temp/") {
				result = string(value)
				break
			}
		}
	}
	if result == "" {
		return false
	}
	for _, candidate := range compilation.Artifact.Equations {
		if candidate.Occurrence.Kind != "call-results" {
			continue
		}
		if _, external := artifactOperand(candidate.Operands, "provider"); external {
			continue
		}
		for _, operand := range candidate.Operands {
			if strings.HasPrefix(operand.Role, "result-") && string(operand.Term.Encoding) == result {
				return !guardWindowHasMutation(compilation.Artifact, operation, candidate.Target.Name)
			}
		}
	}
	return false
}

// guardWindowHasMutation rejects suppression when any concrete write lies
// between this annotation's active guard and the local call that produced its
// source. The order comes from sealed operation coordinates; no path or value
// is reconstructed here.
func guardWindowHasMutation(artifact equation.Artifact, claim equation.BoundEquation, resultOperation string) bool {
	resultIndex, valid := operationIndex(resultOperation)
	if !valid {
		return true
	}
	guardIndex := -1
	for _, guard := range claim.Guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" {
			return true
		}
		index, valid := operationIndex(parts[2])
		if !valid {
			return true
		}
		if guardIndex < 0 || index < guardIndex {
			guardIndex = index
		}
	}
	if guardIndex < 0 || guardIndex >= resultIndex {
		return true
	}
	for _, candidate := range artifact.Equations {
		index, valid := operationIndex(candidate.Target.Name)
		if !valid || index <= guardIndex || index >= resultIndex {
			continue
		}
		switch candidate.Occurrence.Kind {
		case "environment-write", "path-replacement", "index-mutation", "path-invalidation":
			return true
		}
	}
	return false
}

func importedResultPath(source string, paths map[string]bool) bool {
	for source != "" {
		if paths[source] {
			return true
		}
		cut := strings.LastIndexAny(source, ".[ ")
		if cut < 0 {
			return false
		}
		source = source[:cut]
	}
	return false
}

// importedResultPaths follows only closed artifact edges from a resolved module
// provider result. This prevents a provisional Top from becoming a permanent
// lint before its imported summary reaches a later write or indexed read.
func importedResultPaths(artifact equation.Artifact) map[string]bool {
	paths := make(map[string]bool)
	for changed := true; changed; {
		changed = false
		for _, operation := range artifact.Equations {
			var provider, result, target, value, container string
			for _, operand := range operation.Operands {
				switch operand.Role {
				case "provider":
					provider = string(operand.Term.Encoding)
				case "target":
					target = string(operand.Term.Encoding)
				case "value":
					value = string(operand.Term.Encoding)
				case "container":
					container = string(operand.Term.Encoding)
				default:
					if operand.Role != "result-display" && strings.HasPrefix(operand.Role, "result-") {
						result = string(operand.Term.Encoding)
					}
				}
			}
			switch operation.Occurrence.Kind {
			case "call-results":
				if _, _, _, ok := importedProviderTarget([]byte(provider)); ok && result != "" && !paths[result] {
					paths[result], changed = true, true
				}
			case "environment-write":
				if paths[value] && target != "" && !paths[target] {
					paths[target], changed = true, true
				}
			case "dynamic-index-read":
				if paths[container] && target != "" && !paths[target] {
					paths[target], changed = true, true
				}
			}
		}
	}
	return paths
}

type shapeRelation uint8

const (
	shapeUnknown shapeRelation = iota
	shapeProven
	shapeRefuted
)

// assignmentShapeRelation is deliberately proof-oriented: a malformed or
// unsupported target/type member is unknown, never a compatibility result.
func assignmentShapeRelation(lexical *lexicalEvaluator, source, value, encodedTarget []byte, partition equation.Partition) shapeRelation {
	target, ok := shapefact.DecodeTarget(encodedTarget)
	if !ok {
		return shapeUnknown
	}
	if authoritative, ok := lexical.importedAuthority(string(source)); ok {
		if resolved, found := lexical.typeOrigin(encodedTarget); found {
			target = resolved
		}
		// The imported declaration is this path's contract, not its current
		// content. A published witness that lies inside that contract is a
		// refinement established after the import -- a guard narrowing it, for
		// instance -- and it is the type the assignment actually sees.
		if witness, known := scalarWitnessType(value); known && subtype.IsSubtype(witness, authoritative) {
			authoritative = witness
		}
		if subtype.IsSubtype(authoritative, target) {
			return shapeProven
		}
		return shapeRefuted
	}
	if relation := valueAgainstType(value, target); relation == shapeProven {
		return relation
	} else if relation == shapeUnknown {
		if relation := publishedContainerOriginRelation(source, value, target, partition); relation != shapeUnknown {
			return relation
		}
	} else if member := lexicalMemberCallableRelation(lexical, source, target, partition); member != shapeUnknown {
		return member
	} else {
		return relation
	}
	return lexicalMemberCallableRelation(lexical, source, target, partition)
}

// publishedContainerOriginRelation transports an element relation only from
// the literal's existing member-origin publications. A sealed container may
// retain opaque imported element shapes, but its direct members still point to
// the exact typed values from which it was built. Those already-published
// types can prove the homogeneous element contract without inspecting source
// spelling or assuming a type for an untracked table entry.
func publishedContainerOriginRelation(source, value []byte, target typ.Type, partition equation.Partition) shapeRelation {
	if !strings.HasPrefix(string(source), "path/") {
		return shapeUnknown
	}
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return shapeUnknown
	}
	resolved := unwrap.Alias(subst.ExpandInstantiated(target))
	var element, key typ.Type
	switch typed := resolved.(type) {
	case *typ.Array:
		element = typed.Element
	case *typ.Map:
		element, key = typed.Value, typed.Key
	case *typ.ReadonlyMap:
		element, key = typed.Value, typed.Key
	default:
		return shapeUnknown
	}
	if element == nil {
		return shapeUnknown
	}
	for _, member := range table.Members {
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid {
			return shapeUnknown
		}
		// Nested members describe the already-sealed shape of one direct
		// element. Their origin belongs to that descendant rather than to a
		// separate array/map slot, so only direct container entries take part
		// in this homogeneous-element proof.
		if len(segments) != 1 {
			continue
		}
		entry := segments[0]
		if key == nil {
			if entry.Kind != segment.SegmentIndexInt {
				return shapeRefuted
			}
		} else if !acceptsEveryValue(key) {
			keyValue, encoded := containerKeyValue(entry)
			if !encoded {
				return shapeRefuted
			}
			if relation := valueAgainstType(keyValue, key); relation != shapeProven {
				return relation
			}
		}
		if !member.Present {
			continue
		}
		origin, found := heapMemberOriginCurrent(source, member.Suffix, partition)
		if !found {
			return shapeUnknown
		}
		originType, found := typedPathType(origin, partition)
		if !found {
			originType, found = declaredTypeForTerm(origin, partition)
		}
		if !found || originType == nil {
			return shapeUnknown
		}
		if !subtype.IsSubtype(originType, element) {
			return shapeRefuted
		}
	}
	return shapeProven
}

// lexicalMemberCallableRelation is the narrow callable-surface projection for
// a local table member.  It consumes an existing closure capability and the
// current capture cells, then compares the child publication with the claimed
// zero-argument function result.  It never derives a callable contract from a
// member name, table shape, or annotation alone.
func lexicalMemberCallableRelation(lexical *lexicalEvaluator, source []byte, target typ.Type, partition equation.Partition) shapeRelation {
	_, relation := lexicalMemberCallableSurface(lexical, source, target, partition)
	return relation
}

func lexicalMemberCallableDisplay(lexical *lexicalEvaluator, operation equation.BoundEquation) (string, bool) {
	for _, operand := range operation.Operands {
		if operand.Role != "shape-target" {
			continue
		}
		target, ok := shapefact.DecodeTarget(operand.Value)
		if !ok {
			return "", false
		}
		function, ok := unwrap.Alias(subst.ExpandInstantiated(target)).(*typ.Function)
		if !ok || function == nil || len(function.Params) != 0 || len(function.Returns) != 1 || function.Returns[0] == nil {
			return "", false
		}
		return "fun() -> " + callableReturnDisplay(lexical, function.Returns[0]), true
	}
	return "", false
}

func callableClaimDisplay(lexical *lexicalEvaluator, operation equation.BoundEquation) string {
	if display, ok := lexicalMemberCallableDisplay(lexical, operation); ok {
		return display
	}
	return boundClaimDeclaredDisplay(operation, "")
}

func callableReturnDisplay(lexical *lexicalEvaluator, returned typ.Type) string {
	if lexical != nil {
		names := make([]string, 0, len(lexical.typeDefinitions))
		for name := range lexical.typeDefinitions {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if typ.TypeEquals(lexical.typeDefinitions[name], returned) {
				return name
			}
		}
	}
	return typeformat.Short(returned)
}

// lexicalMemberCallableSurface evaluates a locally published member closure
// against its current captured cells. The resulting slot is an existing child
// return publication, not a signature inferred from a member spelling.
func lexicalMemberCallableSurface(lexical *lexicalEvaluator, source []byte, target typ.Type, partition equation.Partition) ([]byte, shapeRelation) {
	if lexical == nil || !strings.HasPrefix(string(source), "path/") {
		return nil, shapeUnknown
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(target)).(*typ.Function)
	if !ok || function == nil || len(function.TypeParams) != 0 || len(function.Params) != 0 || function.Variadic != nil || len(function.Returns) != 1 || function.Returns[0] == nil {
		return nil, shapeUnknown
	}
	handle, found := closureHandleFor(source, partition)
	if !found {
		if cut := strings.LastIndex(string(source), "."); cut > len("path/") && cut < len(source)-1 {
			handle, found = methodClosureHandleFor(source[:cut], string(source[cut+1:]), partition)
		}
	}
	if !found {
		return nil, shapeUnknown
	}
	child, found := lexical.byPrototype[handle.Prototype]
	if !found || child.Cyclic != nil || len(child.Boundary.Parameters) != 0 || len(handle.Captures) != len(child.Boundary.Captures) {
		return nil, shapeUnknown
	}
	operation := equation.BoundEquation{Target: equation.Coordinate{Name: "member-surface"}}
	projected, err := lexical.applyKnown(operation, directCallOperands{callee: source, resultArity: 1}, handle, partition)
	if err != nil {
		return nil, shapeUnknown
	}
	for _, fact := range projected.Closure.Values {
		if fact.Key == "call-result/member-surface/00000000" {
			relation := valueAgainstType(fact.Value, function.Returns[0])
			if relation == shapeUnknown && string(fact.Value) == "scalar/nil" && !unwrap.IsOptionalLike(function.Returns[0]) {
				relation = shapeRefuted
			}
			return append([]byte(nil), fact.Value...), relation
		}
	}
	return nil, shapeUnknown
}

// optionalAssignmentWitness accepts only a sealed source type that explicitly
// admits nil and a concrete declared target that excludes it. Indexed reads
// retain their established missing-slot witness; a non-indexed field needs the
// narrower exact local-call result or exact method-return publication below.
// An annotation, Top, or an unsealed scalar remains unproven and follows the
// ordinary path.
func optionalAssignmentWitness(path, value, encodedTarget []byte, partition equation.Partition) bool {
	if !derivedIndexedPath(path) && !localCallableResultAncestor(path, partition) && !methodReturnSummaryAncestor(path, partition) {
		if _, optionalRead := currentEpochFact(typedOptionalReadPrefix, path, partition); !optionalRead {
			return false
		}
	}
	witness, ok := shapefact.DecodeTarget(value)
	if !ok || witness == nil || !proof.OptionalTypeHasConcreteValue(witness) {
		return false
	}
	declared, ok := shapefact.DecodeTarget(encodedTarget)
	if !ok || declared == nil {
		return false
	}
	return !subtype.IsSubtype(typ.Nil, declared)
}

// localCallableResultAncestor follows a static result descendant back to the
// exact local callable result that published it. A result marker is propagated
// only by the ordinary call-results and local-write ownership chain, so an
// imported summary, annotation, or unrelated optional field cannot claim this
// diagnostic boundary.
func localCallableResultAncestor(term []byte, partition equation.Partition) bool {
	for {
		if _, found := currentEpochFact("local-call-result/", term, partition); found {
			return true
		}
		root, suffix, member := tableAddress(term)
		if !member || suffix == "" {
			return false
		}
		term = root
	}
}

// methodReturnSummaryAncestor follows a static result descendant only to the
// current method-return summary that owns it. The summary is emitted at the
// call-results boundary and carried by the immediately following write, so a
// method declaration, stale write, or arbitrary optional annotation cannot
// become a consumer-side nilability witness. Restricting this bridge to an
// optional record keeps it aligned with the existing root-method guard rule:
// a scalar or open result has no closed member path to republish.
func methodReturnSummaryAncestor(term []byte, partition equation.Partition) bool {
	for {
		if encoded, current := currentEpochFact(methodReturnSummaryPrefix, term, partition); current {
			summary, err := typ.DecodeCanonical(context.Background(), encoded)
			if err == nil && rootOptionalClosedSummary(summary) {
				return true
			}
		}
		root, suffix, _, found := typedAncestor(term, partition)
		if !found || len(suffix) == 0 {
			return false
		}
		term = root
	}
}

func valueAgainstType(value []byte, target typ.Type) shapeRelation {
	return valueAgainstTypeSeen(value, target, newShapeComparison())
}

// shapeComparison holds one structural claim comparison. Recursive aliases are
// regular type graphs, so comparing a published value shape against one must
// close a repeated value/type obligation coinductively. The relation is local
// to a single claim: no result is published or reused outside the fact that
// supplied the value shape.
type shapeComparison struct {
	active map[shapeRecursivePair]bool
	memo   map[shapeRecursivePair]shapeRelation
}

type shapeRecursivePair struct {
	value  string
	target *typ.Recursive
}

func newShapeComparison() *shapeComparison {
	return &shapeComparison{
		active: make(map[shapeRecursivePair]bool),
		memo:   make(map[shapeRecursivePair]shapeRelation),
	}
}

func valueAgainstTypeSeen(value []byte, target typ.Type, comparison *shapeComparison) shapeRelation {
	if comparison == nil {
		return shapeUnknown
	}
	if target == nil || isUnknownScalar(value) {
		return shapeUnknown
	}
	// A literal table carries its sealed member evidence alongside its broad
	// table type. Prefer that finite evidence for structural targets: reducing
	// it to the broad type first would refute an empty map or an all-optional
	// record even though the closed literal proves the assignment.
	resolvedTarget := unwrap.Alias(subst.ExpandInstantiated(target))
	if resolvedTarget == nil {
		return shapeUnknown
	}
	if shapefact.IsTable(value) && resolvedTarget.Kind() == kind.Record {
		return tableAgainstRecord(value, resolvedTarget, comparison)
	}
	if resolvedTarget.Kind() == kind.Map {
		if table, ok := shapefact.DecodeTable(value); ok && table.Closed && len(table.Members) == 0 {
			return tableAgainstMap(value, resolvedTarget)
		}
	}
	if source, ok := shapefact.DecodeTarget(value); ok {
		if subtype.IsSubtype(source, target) {
			return shapeProven
		}
		return shapeRefuted
	}
	// Function literals carry their canonical function type inside the sealed
	// callable value.  That is an ordinary closed fact emitted by the front,
	// not an annotation-derived assumption, so it can discharge a later
	// annotation claim (and, symmetrically, refute an incompatible contract).
	// An unsealed callable has no such witness and remains unknown.
	if source, ok := sealedFunctionType(value); ok {
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
	target = resolvedTarget
	if recursive, ok := target.(*typ.Recursive); ok {
		return comparison.againstRecursive(value, recursive)
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
		return valueAgainstTypeSeen(value, optional.Inner, comparison)
	case kind.Union:
		union, ok := target.(*typ.Union)
		if !ok || len(union.Members) == 0 {
			return shapeUnknown
		}
		unknown := false
		for _, member := range union.Members {
			relation := valueAgainstTypeSeen(value, member, comparison)
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
			relation := valueAgainstTypeSeen(value, member, comparison)
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
		return tableAgainstRecord(value, target, comparison)
	case kind.Array:
		array, ok := target.(*typ.Array)
		if !ok || array.Element == nil {
			return shapeUnknown
		}
		return tableAgainstContainer(value, array.Element, nil, comparison)
	case kind.Map:
		mapping, ok := target.(*typ.Map)
		if !ok || mapping.Key == nil || mapping.Value == nil {
			return shapeUnknown
		}
		return tableAgainstContainer(value, mapping.Value, mapping.Key, comparison)
	case kind.ReadonlyMap:
		mapping, ok := target.(*typ.ReadonlyMap)
		if !ok || mapping.Key == nil || mapping.Value == nil {
			return shapeUnknown
		}
		return tableAgainstContainer(value, mapping.Value, mapping.Key, comparison)
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

func (comparison *shapeComparison) againstRecursive(value []byte, target *typ.Recursive) shapeRelation {
	if target == nil || target.Body == nil || target.Body == target {
		return shapeUnknown
	}
	pair := shapeRecursivePair{value: string(value), target: target}
	if result, ok := comparison.memo[pair]; ok {
		return result
	}
	if comparison.active[pair] {
		// This is the equirecursive assumption for the current structural
		// comparison. The enclosing product/union obligations still decide
		// whether the surrounding value is compatible.
		return shapeProven
	}
	comparison.active[pair] = true
	result := valueAgainstTypeSeen(value, target.Body, comparison)
	delete(comparison.active, pair)
	comparison.memo[pair] = result
	return result
}

// sealedFunctionType decodes only the canonical witness produced by
// front.functionValue.  The signature text is intentionally not used as a
// type parser: absent or malformed canonical data is not proof.
func sealedFunctionType(value []byte) (typ.Type, bool) {
	encoded := strings.TrimPrefix(string(value), "scalar/function/")
	if encoded == string(value) || encoded == "" {
		return nil, false
	}
	wire, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}
	var payload struct {
		Canonical string `json:"canonical,omitempty"`
	}
	if err := json.Unmarshal(wire, &payload); err != nil || payload.Canonical == "" {
		return nil, false
	}
	canonical, err := base64.RawURLEncoding.DecodeString(payload.Canonical)
	if err != nil {
		return nil, false
	}
	// A front function value is a closed publication. Its canonical payload
	// may contain recursive aliases from its declared signature; structural
	// decoding restores their graph without pretending to recreate source
	// declaration identity.
	function, err := typ.DecodeCanonicalStructural(context.Background(), canonical)
	if err != nil || function == nil {
		return nil, false
	}
	if _, ok := unwrap.Alias(function).(*typ.Function); !ok {
		return nil, false
	}
	return function, true
}

// tableAgainstMap proves the one case that needs no inferred key or value
// relation: a sealed empty literal has no map entries that could violate the
// declared homogeneous contract. Non-empty literals remain unknown until an
// exact entry projection supplies both sides of that relation.
func tableAgainstMap(value []byte, target typ.Type) shapeRelation {
	if _, ok := unwrap.Alias(target).(*typ.Map); !ok {
		return shapeUnknown
	}
	table, ok := shapefact.DecodeTable(value)
	if !ok {
		return knownScalarRelation(value, false)
	}
	if table.Closed && len(table.Members) == 0 {
		return shapeProven
	}
	return shapeUnknown
}

func tableAgainstRecord(value []byte, target typ.Type, comparison *shapeComparison) shapeRelation {
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
			if field.Optional || unwrap.IsOptionalLike(field.Type) {
				continue
			}
			if table.Closed {
				return shapeRefuted
			}
			unknown = true
			continue
		}
		memberValue := tableDescendantEvidence(table, member.Suffix, []byte(member.Value))
		relation := valueAgainstTypeSeen(memberValue, field.Type, comparison)
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

// tableDescendantEvidence reconciles a nested literal member with the direct
// descendants sealed by its containing literal.  Lowering emits both forms so
// later path writes can retain precise locations; the outer fact is the newer
// publication when an imported static member was unavailable during the
// nested object's first materialization.  Only members already sealed in the
// same closed table participate, and malformed or non-table members remain
// unchanged.
func tableDescendantEvidence(table shapefact.Table, prefix string, value []byte) []byte {
	nested, ok := shapefact.DecodeTable(value)
	if !ok || prefix == "" {
		return value
	}
	positions := make(map[string]int, len(nested.Members))
	for index, member := range nested.Members {
		positions[member.Suffix] = index
	}
	changed := false
	for _, member := range table.Members {
		if !strings.HasPrefix(member.Suffix, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(member.Suffix, prefix)
		if suffix == "" || (suffix[0] != '.' && suffix[0] != '[') {
			continue
		}
		if index, found := positions[suffix]; found {
			nested.Members[index] = shapefact.Member{Suffix: suffix, Present: member.Present, Value: member.Value}
		} else {
			nested.Members = append(nested.Members, shapefact.Member{Suffix: suffix, Present: member.Present, Value: member.Value})
		}
		changed = true
	}
	if !changed {
		return value
	}
	encoded, ok := shapefact.EncodeTable(nested)
	if !ok {
		return value
	}
	return encoded
}

// assignmentMismatch identifies the first closed record member that refutes a
// declared structural assignment. It is a diagnostic projection of the same
// finite shape relation used by claimKernel: unknown/open members never appear
// here, and recursive declaration graphs remain bounded by the existing
// comparison when deciding whether a member is refuted.
type assignmentMismatch struct {
	Suffix   string
	Value    []byte
	Expected typ.Type
}

// uniqueNamedArm selects the single union arm whose member surface admits every
// member a closed literal actually names. When exactly one arm can name them
// all, that arm is the only contract the literal could have satisfied, so an
// already-refuted assignment reports the member that refutes it rather than the
// whole union. Several admitting arms leave arm selection undecided and the
// whole-value message stands.
func uniqueNamedArm(union *typ.Union, value []byte) (typ.Type, bool) {
	if union == nil || len(union.Members) == 0 {
		return nil, false
	}
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return nil, false
	}
	names := make([]string, 0, len(table.Members))
	for _, member := range table.Members {
		if !member.Present {
			continue
		}
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid || len(segments) != 1 || segments[0].Kind != segment.SegmentField {
			return nil, false
		}
		names = append(names, segments[0].Name)
	}
	if len(names) == 0 {
		return nil, false
	}
	var selected typ.Type
	for _, arm := range union.Members {
		record, isRecord := unwrap.Alias(subst.ExpandInstantiated(arm)).(*typ.Record)
		if !isRecord || record == nil || record.Open {
			continue
		}
		admits := true
		for _, name := range names {
			if record.GetField(name) == nil {
				admits = false
				break
			}
		}
		if !admits {
			continue
		}
		if selected != nil {
			return nil, false
		}
		selected = arm
	}
	return selected, selected != nil
}

// missingRequiredField reports the first declared field that a sealed object
// literal does not provide. Only a closed constructor can prove absence, and
// only a field that excludes nil carries the obligation: an optional or
// nil-admitting field is satisfied by its own absence.
func missingRequiredField(value []byte, target typ.Type) (string, typ.Type, bool) {
	record, ok := assignmentRecordTarget(target)
	if !ok {
		return "", nil, false
	}
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return "", nil, false
	}
	for _, field := range record.Fields {
		if field.Name == "" || field.Type == nil || field.Optional || subtype.IsSubtype(typ.Nil, field.Type) {
			continue
		}
		if member, found := table.Lookup("." + field.Name); found && member.Present {
			continue
		}
		return field.Name, field.Type, true
	}
	return "", nil, false
}

// resolvedAssignmentTarget expands a declared assignment target to the shape
// that carries its obligations, following aliases, instantiations, and one
// recursive unrolling. It is the single entry every assignment projection uses
// so they all read the same contract.
func resolvedAssignmentTarget(target typ.Type) typ.Type {
	if target == nil {
		return nil
	}
	resolved := unwrap.Alias(subst.ExpandInstantiated(target))
	if recursive, ok := resolved.(*typ.Recursive); ok && recursive.Body != nil && recursive.Body != recursive {
		resolved = unwrap.Alias(subst.ExpandInstantiated(recursive.Body))
	}
	return resolved
}

// assignmentRecordTarget resolves a declared assignment target to the record
// contract it imposes. Any other target shape has no field obligations.
func assignmentRecordTarget(target typ.Type) (*typ.Record, bool) {
	record, ok := resolvedAssignmentTarget(target).(*typ.Record)
	return record, ok && record != nil
}

func firstAssignmentMismatch(value []byte, target typ.Type) (assignmentMismatch, bool) {
	if target == nil {
		return assignmentMismatch{}, false
	}
	resolved := resolvedAssignmentTarget(target)
	if union, ok := resolved.(*typ.Union); ok {
		arm, selected := uniqueNamedArm(union, value)
		if !selected {
			return assignmentMismatch{}, false
		}
		return firstAssignmentMismatch(value, arm)
	}
	record, ok := resolved.(*typ.Record)
	if !ok {
		return assignmentMismatch{}, false
	}
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return assignmentMismatch{}, false
	}
	for _, field := range record.Fields {
		member, found := table.Lookup("." + field.Name)
		if !found || !member.Present || valueAgainstType([]byte(member.Value), field.Type) != shapeRefuted {
			continue
		}
		if nested, found := firstAssignmentMismatch([]byte(member.Value), field.Type); found {
			nested.Suffix = "." + field.Name + nested.Suffix
			return nested, true
		}
		return assignmentMismatch{Suffix: "." + field.Name, Value: []byte(member.Value), Expected: field.Type}, true
	}
	return assignmentMismatch{}, false
}

// tableAgainstContainer proves a homogeneous array or map only from every
// direct member in a sealed literal.  Nested members describe the member's
// own shape, not another container entry.  A field key is a string key; an
// integer index is the sole array-key form.  Unknown member/key relations
// stay unknown, so an open or partially described shape never becomes proof.
func tableAgainstContainer(value []byte, element, key typ.Type, comparison *shapeComparison) shapeRelation {
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed || element == nil {
		return shapeUnknown
	}
	unknown := false
	for _, member := range table.Members {
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid || len(segments) == 0 {
			return shapeUnknown
		}
		if len(segments) != 1 {
			continue
		}
		entry := segments[0]
		if key == nil {
			if entry.Kind != segment.SegmentIndexInt {
				return shapeRefuted
			}
		} else {
			keyValue, encoded := containerKeyValue(entry)
			if !encoded {
				return shapeRefuted
			}
			relation := shapeProven
			if !acceptsEveryValue(key) {
				relation = valueAgainstTypeSeen(keyValue, key, comparison)
			}
			if relation == shapeRefuted {
				return shapeRefuted
			}
			unknown = unknown || relation == shapeUnknown
		}
		// A nil constructor member is an absent entry, which carries no
		// element obligation for a homogeneous container.
		if !member.Present {
			continue
		}
		relation := shapeProven
		if !acceptsEveryValue(element) {
			relation = valueAgainstTypeSeen([]byte(member.Value), element, comparison)
		}
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

// acceptsEveryValue recognizes only an already-resolved any target. It is
// used within a sealed container comparison: each concrete member is present,
// and any imposes no additional value or key obligation. It intentionally
// does not make a top-level `any` claim proven, because that declaration is an
// explicit precision boundary that claimKernel must retain.
func acceptsEveryValue(target typ.Type) bool {
	if target == nil {
		return false
	}
	resolved := unwrap.Alias(subst.ExpandInstantiated(target))
	return resolved != nil && resolved.Kind() == kind.Any
}

func containerKeyValue(entry segment.Segment) ([]byte, bool) {
	switch entry.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return []byte("scalar/string/" + strconv.Quote(entry.Name)), true
	case segment.SegmentIndexInt:
		return []byte("scalar/number/" + strconv.Itoa(entry.Index)), true
	default:
		return nil, false
	}
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
	target, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil {
		return false
	}
	return provenValueNotSubtype(value, target)
}

// optionalAssignmentSource renders recursive optional values by their decisive
// nilability refutation, before recursive member comparison can report an
// incidental callable-shape mismatch. Both sides are already-published type
// facts; ordinary optionals and nil-accepting targets stay on the existing
// assignment path.
func optionalAssignmentSource(value []byte, targetType string) bool {
	source, ok := shapefact.DecodeTarget(value)
	if !ok || source == nil {
		return false
	}
	optional, ok := unwrap.Alias(subst.ExpandInstantiated(source)).(*typ.Optional)
	if !ok || optional == nil || optional.Inner == nil {
		return false
	}
	if _, recursive := unwrap.Alias(subst.ExpandInstantiated(optional.Inner)).(*typ.Recursive); !recursive {
		return false
	}
	name, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil || name == "" {
		return false
	}
	return !strings.HasSuffix(name, "?") && name != "any" && name != "nil"
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

func isUnvalidatedAnyValue(value []byte) bool {
	return isExplicitAnyValue(value) || string(value) == "scalar/external-callback-any"
}

func sourceHasExplicitAny(source []byte, values []equation.Fact) bool {
	_, _, found := explicitAnySourceFact(source, values)
	return found
}

// sourceHasAnyBoundary reads only the solved type facts for the exact source
// path and its structural ancestors. A concrete value at such a path is not a
// validation proof: the declared any boundary remains authoritative until a
// runtime guard publishes a proof for that same path.
func sourceHasAnyBoundary(source []byte, values []equation.Fact) bool {
	if sourceHasExplicitAny(source, values) {
		return true
	}
	path := strings.TrimPrefix(string(source), "path/")
	if path == string(source) || path == "" {
		return false
	}
	for {
		prefix := "type/path/" + path + "/"
		for _, fact := range values {
			if !strings.HasPrefix(fact.Key, prefix) {
				continue
			}
			declared, ok := shapefact.DecodeTarget(fact.Value)
			if ok && declared != nil && unwrap.Alias(subst.ExpandInstantiated(declared)).Kind() == kind.Any {
				return true
			}
		}
		if summaryTypeIsAny([]byte("path/"+path), values) {
			return true
		}
		cut := strings.LastIndexAny(path, ".[")
		if cut < 0 {
			return false
		}
		path = path[:cut]
	}
}

// summaryTypeIsAny recognizes the current, project-published summary at a
// path.  Imported and joined values carry this canonical fact rather than a
// local declaration; matching it to the term's current epoch prevents an old
// Any summary from surviving a later write.
func summaryTypeIsAny(term []byte, values []equation.Fact) bool {
	if !strings.HasPrefix(string(term), "path/") {
		return false
	}
	epochPrefix := epochFactPrefix + string(term) + "/"
	latest := ""
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, epochPrefix) && fact.Key > latest {
			latest = fact.Key
		}
	}
	if latest == "" {
		return false
	}
	operation := strings.TrimPrefix(latest, epochPrefix)
	for _, fact := range values {
		if fact.Key != summaryTypePrefix+string(term)+"/"+operation {
			continue
		}
		summary, err := typ.DecodeCanonical(context.Background(), fact.Value)
		return err == nil && summary != nil && unwrap.Alias(subst.ExpandInstantiated(summary)).Kind() == kind.Any
	}
	return false
}

func hasSummaryAnyArgument(arguments [][]byte, values []equation.Fact) bool {
	for _, argument := range arguments {
		if summaryTypeIsAny(argument, values) {
			return true
		}
	}
	return false
}

func gradualAnyBoundaryFact(target string, source []byte, operation string, values []equation.Fact) (equation.Fact, bool) {
	root, found := gradualAnySourceFact(source, values)
	if !found {
		return equation.Fact{}, false
	}
	return equation.Fact{Key: "gradual-any/" + target + "/" + operation, Value: root}, true
}

func gradualAnySourceFact(source []byte, values []equation.Fact) ([]byte, bool) {
	term := string(source)
	path := strings.TrimPrefix(term, "path/")
	if (path == term || path == "") && !strings.HasPrefix(term, "temp/") {
		return nil, false
	}
	for {
		candidate := term
		if strings.HasPrefix(term, "path/") {
			candidate = "path/" + path
		}
		prefix := "gradual-any/" + candidate + "/"
		var root []byte
		latest := ""
		for _, fact := range values {
			if strings.HasPrefix(fact.Key, prefix) && strings.HasPrefix(string(fact.Value), "path/") && (root == nil || fact.Key > latest) {
				root, latest = append([]byte(nil), fact.Value...), fact.Key
			}
		}
		if root != nil {
			return root, true
		}
		if !strings.HasPrefix(term, "path/") {
			return nil, false
		}
		cut := strings.LastIndexAny(path, ".[")
		if cut < 0 {
			return nil, false
		}
		path = path[:cut]
	}
}

// sourceHasGradualLogicalBoundary identifies the exact untyped formal whose
// published gradual boundary reached an argument through a logical result.
// The marker is attached only by the guarded uncalled admission above; a
// general explicit-any boundary keeps its established diagnostic projection.
func sourceHasGradualLogicalBoundary(source []byte, values []equation.Fact) bool {
	root, found := gradualAnySourceFact(source, values)
	if !found {
		return false
	}
	prefix := "gradual-logical/" + string(root) + "/"
	for _, fact := range values {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == string(root) {
			return true
		}
	}
	return false
}

// explicitAnySourceFact returns only an already-published exact source or
// ancestor fact. It is used to retain a precision boundary through a closed
// equation handoff, never to infer a value for an unrecorded member read.
func explicitAnySourceFact(source []byte, values []equation.Fact) ([]byte, []byte, bool) {
	if root, found := gradualAnySourceFact(source, values); found {
		return root, []byte("scalar/claim/claim-kind/3/\"any\""), true
	}
	path := strings.TrimPrefix(string(source), "path/")
	if path == string(source) || path == "" {
		return nil, nil, false
	}
	for {
		declaredPrefix := "declared-type/path/" + path + "/"
		for _, fact := range values {
			if !strings.HasPrefix(fact.Key, declaredPrefix) {
				continue
			}
			declared, ok := shapefact.DecodeTarget(fact.Value)
			if ok && declared != nil && unwrap.Alias(subst.ExpandInstantiated(declared)).Kind() == kind.Any {
				return []byte("path/" + path), []byte("scalar/claim/claim-kind/3/\"any\""), true
			}
		}
		prefix := "value/path/" + path + "/"
		var value []byte
		latest := ""
		for _, fact := range values {
			if strings.HasPrefix(fact.Key, prefix) && isExplicitAnyValue(fact.Value) && (value == nil || fact.Key > latest) {
				value, latest = fact.Value, fact.Key
			}
		}
		if value != nil {
			return []byte("path/" + path), append([]byte(nil), value...), true
		}
		cut := strings.LastIndexAny(path, ".[")
		if cut < 0 {
			return nil, nil, false
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

func assignmentAnyMismatchMessage(source string, targetType string, shapeTarget []byte) string {
	declared, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil {
		declared = strings.TrimPrefix(targetType, "claim-type/")
	}
	if declared == "" || structuralAssignmentTarget(shapeTarget) {
		return "cannot assign " + source + " because " + source + " comes from any/unknown; no proof shows it satisfies the declared type"
	}
	return "cannot assign " + source + " because it is any, not " + declared
}

// structuralAssignmentTarget recognizes a declared target whose contract is a
// field structure rather than a single named contract. Such a target is
// satisfied member by member, so an unvalidated any source is reported as a
// missing boundary proof instead of a scalar type mismatch. The decision reads
// the resolved target type, never its rendered spelling.
func structuralAssignmentTarget(shapeTarget []byte) bool {
	target, ok := shapefact.DecodeTarget(shapeTarget)
	if !ok {
		return false
	}
	_, record := assignmentRecordTarget(target)
	return record
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
	if value, found := declaredOptionalMapReadValue(term, partition); found {
		return value, true, nil
	}
	value, err := resolveCurrentValue(term, partition)
	if err != nil {
		return nil, false, nil
	}
	// A method-result summary can prove an optional static member only at the
	// consuming claim that owns both that member path and its declared target.
	// Keeping this projection out of general reads prevents an optional receiver
	// from changing iteration, branch, or intermediate expression semantics.
	if projected, found := methodReturnOptionalClaimValue(term, partition); found {
		value = projected
	}
	return value, true, nil
}

func methodReturnOptionalClaimValue(term []byte, partition equation.Partition) ([]byte, bool) {
	root, suffix, source, ok := typedAncestor(term, partition)
	if !ok || len(suffix) == 0 || !methodReturnSummaryAncestor(root, partition) || !optionalConcreteWitnessType(source) {
		return nil, false
	}
	projected, found := typedPathSegments(source, suffix)
	if !found {
		return nil, false
	}
	return shapefact.EncodeTarget(projected)
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
	operands, err := requiredOperandsByRole(operation.Operands, "container", "key", "suffix")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	// A dynamic write may address the element that justified an earlier
	// in-range read. Revoke that publication by the resolved heap identity so
	// aliases observe the same transition; without an identity, the exact
	// container path is still the only available conservative subject.
	subject := heapIndexSubject(operands["container"], partition)
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
		Key: heapIndexRevokePrefix + subject + "/" + operation.Target.Name, Value: []byte("revoked"),
	}}}}, nil
}

// frozenMutationDiagnostic is deliberately fact-only: a prior freeze epoch,
// or the true edge of table.isfrozen, is the complete proof. It never turns an
// unknown heap value or a merely reachable freeze into a violation.
func frozenMutationDiagnostic(operation equation.BoundEquation, partition equation.Partition, action string) (equation.TransactionResult, error) {
	var subject, display []byte
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "write-container":
			subject = operand.Value
		case "write-container-display":
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

// optionalWriteContainerDiagnostic refutes a member write whose container is
// still proven to admit nil at this point. The container's current value is the
// whole proof: a guard that narrows it publishes a later non-optional value at
// the same path, so a narrowed container reaches this write with nothing to
// report. An unknown container remains unreported.
func optionalWriteContainerDiagnostic(operation equation.BoundEquation, display string, partition equation.Partition) (equation.Fact, equation.Fact, bool) {
	var container, containerDisplay []byte
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "write-container":
			container = operand.Value
		case "write-container-display":
			containerDisplay = operand.Value
		}
	}
	if !strings.HasPrefix(string(container), "path/") || len(containerDisplay) == 0 || display == "" {
		return equation.Fact{}, equation.Fact{}, false
	}
	value, err := resolveCurrentValue(container, partition)
	if err != nil || !optionalConcreteWitness(value) {
		return equation.Fact{}, equation.Fact{}, false
	}
	return equation.Fact{
			Key:   "type.assignment.optional_target/" + operation.Target.Name,
			Value: []byte(fmt.Sprintf("cannot assign through optional %s without nil check", containerDisplay)),
		}, equation.Fact{
			Key:   optionalWriteContainerPrefix + operation.Target.Name,
			Value: append([]byte(nil), value...),
		}, true
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
	if diagnostic, found := closedDynamicWriteDiagnostic(operation, operands, partition); found {
		result.Closure.Diagnostics = append(result.Closure.Diagnostics, diagnostic)
	} else if diagnostic, found := declaredElementWriteDiagnostic(operation, operands, partition); found {
		result.Closure.Diagnostics = append(result.Closure.Diagnostics, diagnostic)
	}
	if identity, found := tableIdentityForTerm(operands["container"], partition); found {
		key, keyErr := resolveCurrentValue(operands["key"], partition)
		if suffix, exact := tableMemberSuffix(key, operands["suffix"]); keyErr == nil && exact {
			value, valueErr := resolveCurrentValue(operands["value"], partition)
			if valueErr == nil {
				result.Closure.Values = append(result.Closure.Values, heapMemberFact(identity, suffix, operation.Target.Name, value))
				memberIdentity, hasMemberIdentity := tableIdentityForTerm(operands["value"], partition)
				if hasMemberIdentity {
					result.Closure.Values = append(result.Closure.Values, heapMemberIdentityFact(identity, suffix, operation.Target.Name, memberIdentity))
				}
				// An exact dynamic key can address a nested member whose enclosing
				// table is already reachable by an alias. Publish the same write at
				// that existing member identity, so later reads through the alias
				// observe the replacement. The identity walk is bounded entirely by
				// pre-existing heap facts; an unresolved prefix stays fail-closed.
				if nestedIdentity, nestedSuffix, nested := nestedHeapMemberAddress(identity, suffix, partition); nested {
					result.Closure.Values = append(result.Closure.Values, heapMemberFact(nestedIdentity, nestedSuffix, operation.Target.Name, value))
					if hasMemberIdentity {
						result.Closure.Values = append(result.Closure.Values, heapMemberIdentityFact(nestedIdentity, nestedSuffix, operation.Target.Name, memberIdentity))
					}
				}
			}
		}
	}
	return result, nil
}

// declaredElementWriteDiagnostic checks a dynamic write against the element
// contract already published for its exact container. A literal shape is not
// required: the declaration itself is the authority. Unknown values remain
// unreported, while an explicit-any boundary and a concrete refutation cannot
// silently enter the homogeneous container.
func declaredElementWriteDiagnostic(operation equation.BoundEquation, operands map[string][]byte, partition equation.Partition) (equation.Fact, bool) {
	// A non-empty suffix writes through the selected element (for example
	// slots[key].value), so the container's element contract does not describe
	// the assigned leaf. That case is owned by the existing path/heap projection.
	if string(operands["suffix"]) != "suffix/" {
		return equation.Fact{}, false
	}
	declared, found := declaredTypeForTerm(operands["container"], partition)
	if !found {
		return equation.Fact{}, false
	}
	element, ok := declaredElementContract(declared)
	if !ok {
		return equation.Fact{}, false
	}
	value, err := resolveCurrentValue(operands["value"], partition)
	if err != nil {
		return equation.Fact{}, false
	}
	// Storing nil under a key removes that entry instead of placing a value in
	// it, so the element contract does not describe the store. Reads already
	// carry the resulting absence: an element is optional until an in-range
	// proof discharges it.
	if string(value) == "scalar/nil" {
		return equation.Fact{}, false
	}
	anySource := isExplicitAnyValue(value) || sourceHasAnyBoundary(operands["value"], partition.Values())
	if !anySource && valueAgainstType(value, element) != shapeRefuted {
		return equation.Fact{}, false
	}
	source := "value"
	for _, operand := range operation.Operands {
		if operand.Role == "source-display" && len(operand.Value) != 0 {
			source = string(operand.Value)
			break
		}
	}
	expected := typeformat.Short(element)
	message := "cannot assign " + source + " because it is " + assignmentEvidenceValue(value) + ", not " + expected
	if anySource {
		message = "cannot assign " + source + " because it is any, not " + expected
	}
	return equation.Fact{Key: "type.assignment/" + operation.Target.Name, Value: []byte(message)}, true
}

// declaredElementContract returns the type a declared homogeneous container
// admits at any one key. Both spellings a Lua declaration can carry are the
// same contract: a map's value type and a list's element type describe every
// entry, so an unproven key writes against exactly that type.
func declaredElementContract(declared typ.Type) (typ.Type, bool) {
	if declared == nil {
		return nil, false
	}
	switch container := unwrap.Alias(subst.ExpandInstantiated(declared)).(type) {
	case *typ.Map:
		if container == nil || container.Value == nil {
			return nil, false
		}
		return container.Value, true
	case *typ.Array:
		if container == nil || container.Element == nil {
			return nil, false
		}
		return container.Element, true
	default:
		return nil, false
	}
}

// closedDynamicWriteDiagnostic rejects a broad write only when the exact
// table shape, every member value, and the assigned value have already been
// published. A prior dynamic mutation invalidates this narrow literal-shape
// authority, so later writes deliberately receive no inferred contract.
//
// A declared container has no inferred contract either. The constructor's
// members describe what the table currently holds, while the declaration states
// what every key admits; deriving the obligation from the members instead would
// reject writes the declared type allows.
func closedDynamicWriteDiagnostic(operation equation.BoundEquation, operands map[string][]byte, partition equation.Partition) (equation.Fact, bool) {
	identity, found := tableIdentityForTerm(operands["container"], partition)
	if !found || !heapTableClosed(identity, partition) || heapIndexWasMutated(identity, operation.Target.Name, partition) {
		return equation.Fact{}, false
	}
	if declared, hasDeclaration := declaredTypeForTerm(operands["container"], partition); hasDeclaration {
		if _, homogeneous := declaredElementContract(declared); homogeneous {
			return equation.Fact{}, false
		}
	}
	container, containerErr := resolveCurrentValue(operands["container"], partition)
	table, sealed := shapefact.DecodeTable(container)
	if containerErr != nil || !sealed || !table.Closed || len(table.Members) == 0 {
		return equation.Fact{}, false
	}
	key, keyErr := resolveCurrentValue(operands["key"], partition)
	if keyErr != nil || dynamicKeyIsExact(key) {
		return equation.Fact{}, false
	}
	value, valueErr := resolveCurrentValue(operands["value"], partition)
	if valueErr != nil || isUnknownScalar(value) {
		return equation.Fact{}, false
	}
	mismatch := false
	contracts := make([]string, 0, len(table.Members))
	for _, member := range table.Members {
		if !member.Present {
			return equation.Fact{}, false
		}
		memberValue, current := heapMemberCurrent(heapMemberPrefix, identity, member.Suffix, partition)
		if !current {
			return equation.Fact{}, false
		}
		contract, exact := literalValueContract(memberValue)
		if !exact {
			return equation.Fact{}, false
		}
		contracts = append(contracts, contract)
		if !literalValueSatisfies(value, memberValue) {
			mismatch = true
		}
	}
	if !mismatch {
		return equation.Fact{}, false
	}
	contracts = uniqueOrderedStrings(contracts)
	source := "value"
	for _, operand := range operation.Operands {
		if operand.Role == "source-display" {
			source = string(operand.Value)
		}
	}
	return equation.Fact{
		Key:   "type.assignment/" + operation.Target.Name,
		Value: []byte("cannot assign " + source + " because it is " + assignmentValueType(value) + ", not " + strings.Join(contracts, " & ")),
	}, true
}

func uniqueOrderedStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" && !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	return out
}

func heapIndexWasMutated(identity []byte, before string, partition equation.Partition) bool {
	prefix := heapIndexRevokePrefix + "identity/" + base64.RawURLEncoding.EncodeToString(identity) + "/"
	beforeIndex, valid := operationIndex(before)
	if !valid {
		return true
	}
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) || string(fact.Value) != "revoked" {
			continue
		}
		if mutationIndex, valid := operationIndex(factOperation(fact.Key)); valid && mutationIndex+1 < beforeIndex {
			return true
		}
	}
	return false
}

func operationIndex(operation string) (int, bool) {
	value, found := strings.CutPrefix(operation, "op-")
	if !found || value == "" {
		return 0, false
	}
	index, err := strconv.Atoi(value)
	return index, err == nil
}

func dynamicKeyIsExact(value []byte) bool {
	return strings.HasPrefix(string(value), "scalar/string/") || strings.HasPrefix(string(value), "scalar/number/")
}

func literalValueContract(value []byte) (string, bool) {
	switch {
	case strings.HasPrefix(string(value), "scalar/string/"):
		raw := strings.TrimPrefix(string(value), "scalar/string/")
		text, err := strconv.Unquote(raw)
		if err != nil {
			return "", false
		}
		return strconv.Quote(text), true
	case strings.HasPrefix(string(value), "scalar/number/"):
		return strings.TrimPrefix(string(value), "scalar/number/"), true
	case string(value) == "scalar/bool/true":
		return "true", true
	case string(value) == "scalar/bool/false":
		return "false", true
	case string(value) == "scalar/nil":
		return "nil", true
	default:
		return "", false
	}
}

func literalValueSatisfies(value, contract []byte) bool {
	if string(value) == string(contract) {
		return true
	}
	// A non-literal scalar cannot establish equality with a literal member.
	return false
}

func enrichClosedDynamicWriteDiagnostic(item PublishedDiagnostic, operation equation.Equation, targetSpan wir.Span) PublishedDiagnostic {
	source, target := "value", "value"
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "source-display":
			source = string(operand.Term.Encoding)
		case "display":
			target = string(operand.Term.Encoding)
		}
	}
	valueType, contract, found := strings.Cut(strings.TrimPrefix(item.Message, "cannot assign "+source+" because it is "), ", not ")
	if !found || valueType == "" || contract == "" {
		return item
	}
	if !targetSpan.Valid() {
		targetSpan = item.Span
	}
	item.Evidence = []DiagnosticEvidence{
		{Span: item.Span, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("%s has type %s", source, valueType)},
		{Span: targetSpan, Kind: "abstract fact", Trust: "proven", Message: fmt.Sprintf("assignment target %s requires %s", target, contract)},
		{Span: item.Span, Kind: "missing proof", Trust: "unknown", Reason: "boundary validation missing", Message: fmt.Sprintf("no proof on this path shows %s is %s", source, contract)},
	}
	item.Labels = []DiagnosticLabel{{Span: item.Span, Message: "assigned value " + valueType}, {Span: targetSpan, Message: "assignment target " + target}}
	item.Help = "Use a value compatible with the expected type, or change the target type if `" + source + "` is valid."
	return item
}

func genericForKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "iteration-kind", "iterator", "state", "control")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	value := []byte("scalar/top")
	numericInduction := false
	// The front carries the iteration form as part of the same closed loop
	// carrier as the control triple. A numeric loop counter is therefore typed
	// only from its already-published start, limit, and step; a generic iterator
	// with numerically shaped state/control cannot borrow this induction proof.
	if string(operands["iteration-kind"]) == "iteration-kind/numeric" {
		numeric, integral := true, true
		for _, role := range []string{"iterator", "state", "control"} {
			bound, boundErr := resolveCurrentValue(operands[role], partition)
			if boundErr != nil || valueAgainstType(bound, typ.Number) != shapeProven {
				numeric = false
				break
			}
			integral = integral && valueAgainstType(bound, typ.Integer) == shapeProven
		}
		if numeric {
			numericInduction = true
			inductionType := typ.Number
			if integral {
				inductionType = typ.Integer
			}
			var encoded bool
			value, encoded = shapefact.EncodeTarget(inductionType)
			if !encoded {
				return equation.TransactionResult{}, fmt.Errorf("engine: encode numeric loop witness")
			}
		}
	}
	iteratorElement, indexedIterator := currentEpochFact(iteratorElementPrefix, operands["iterator"], partition)
	iteratorKey, keyedIterator := currentEpochFact(iteratorKeyPrefix, operands["iterator"], partition)
	values := make([]equation.Fact, 0)
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "result-") {
			continue
		}
		result := string(operand.Value)
		if !strings.HasPrefix(result, "path/") {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed generic-for result %q", operand.Role)
		}
		resultValue := value
		if keyedIterator {
			index, indexErr := strconv.Atoi(strings.TrimPrefix(operand.Role, "result-"))
			if indexErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed generic-for result %q", operand.Role)
			}
			if index == 0 {
				resultValue = iteratorKey
			} else if index == 1 {
				resultValue = iteratorElement
			}
		} else if indexedIterator {
			index, indexErr := strconv.Atoi(strings.TrimPrefix(operand.Role, "result-"))
			if indexErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed generic-for result %q", operand.Role)
			}
			if index == 0 {
				var encoded bool
				resultValue, encoded = shapefact.EncodeTarget(typ.Integer)
				if !encoded {
					return equation.TransactionResult{}, fmt.Errorf("engine: encode indexed iterator key witness")
				}
			} else {
				resultValue = iteratorElement
			}
		}
		values = append(values, equation.Fact{Key: "value/" + result + "/" + operation.Target.Name, Value: append([]byte(nil), resultValue...)})
		if numericInduction {
			values = append(values, equation.Fact{Key: numericForInductionPrefix + result + "/" + operation.Target.Name, Value: append([]byte(nil), resultValue...)})
		}
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

// iteratorElementWitness reifies an iterator's declared result relation. The
// iterator contract comes from the existing standard-library signature and
// the key/value types come from an existing closed source or typed binding.
func iteratorElementWitness(provider []byte, arguments map[int][]byte, partition equation.Partition) ([]byte, []byte, bool) {
	signature, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(providerName(provider))
	if !found {
		return nil, nil, false
	}
	iterator, found := iteration.ActiveIterator(signature.Effect.Labels)
	if !found {
		return nil, nil, false
	}
	argument, found := arguments[iterator.Source.Index]
	if !found {
		return nil, nil, false
	}
	value, err := resolveCurrentValue(argument, partition)
	if err != nil {
		return nil, nil, false
	}
	// An unannotated, sealed array literal is already a published finite value
	// shape.  Its indexed entries may carry an explicit-any boundary, which
	// must remain authoritative after ipairs transports that exact element into
	// the loop body.  Admit an element only when every direct integer entry is
	// present and has the identical published value; mixed or open shapes stay
	// unmodeled rather than receiving an invented array type.
	if iterator.Kind == iteration.IterateIndexed {
		if element, found := sealedIndexedIteratorElement(value); found {
			return nil, element, true
		}
	}
	source, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		source, decoded = typedPathType(argument, partition)
	}
	if !decoded {
		source, decoded = declaredTypeForTerm(argument, partition)
	}
	if !decoded || source == nil {
		return nil, nil, false
	}
	switch iterator.Kind {
	case iteration.IterateIndexed:
		array, ok := unwrap.Alias(subst.ExpandInstantiated(source)).(*typ.Array)
		if !ok || array.Element == nil {
			return nil, nil, false
		}
		element, ok := shapefact.EncodeTarget(array.Element)
		return nil, element, ok
	case iteration.IterateKeyed:
		mapping, ok := unwrap.Alias(subst.ExpandInstantiated(source)).(*typ.Map)
		if !ok || mapping.Key == nil || mapping.Value == nil {
			return nil, nil, false
		}
		key, keyOK := shapefact.EncodeTarget(mapping.Key)
		element, elementOK := shapefact.EncodeTarget(mapping.Value)
		return key, element, keyOK && elementOK
	default:
		return nil, nil, false
	}
}

func sealedIndexedIteratorElement(value []byte) ([]byte, bool) {
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return nil, false
	}
	var element []byte
	for _, member := range table.Members {
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid {
			return nil, false
		}
		if len(segments) != 1 || segments[0].Kind != segment.SegmentIndexInt {
			continue
		}
		if segments[0].Index < 1 || !member.Present {
			return nil, false
		}
		if element == nil {
			element = []byte(member.Value)
			continue
		}
		if string(element) != member.Value {
			return nil, false
		}
	}
	return element, element != nil
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
	// A select writes its result on every path it can take, so the destination is
	// written even when no case yields a payload this transaction can prove.
	// Withholding the refined result type is a precision decision; withholding
	// the write itself would leave a read with no completed producer, which is an
	// artifact malformation rather than an absent fact.
	unrefined := equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{
		{Key: "value/" + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte("scalar/top")},
		{Key: epochFactPrefix + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
	}}}
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
				return unrefined, nil
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
		if item == nil || item.display == "" {
			return unrefined, nil
		}
		if item.payload == nil {
			// The front carries a payload operand when the WIR root has a
			// declaration. Imported summaries instead make the channel type
			// available only after the member path is closed. Recover that exact
			// witness here; an absent or non-channel type still produces no
			// select fact.
			payload, known := typedChannelPayload([]byte(item.term), partition)
			if !known {
				return unrefined, nil
			}
			item.payload = payload
		}
		identity, ok := resolveCurrentIdentity([]byte(item.term), partition)
		if !ok || !isChannelIdentity(identity) {
			return unrefined, nil
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
		return unrefined, nil
	}
	encodedResult, ok := shapefact.EncodeTarget(resultType)
	if !ok {
		return unrefined, nil
	}
	meta, marshalErr := json.Marshal(selectMetaWire{Cases: len(ordered), HasDefault: string(operands["default"]) == "select/default/true"})
	if marshalErr != nil {
		return equation.TransactionResult{}, marshalErr
	}
	values := []equation.Fact{
		{Key: "value/" + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte("scalar/top")},
		{Key: epochFactPrefix + string(operands["result"]) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
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
	if closure, recognized := typePredicateBranchClosure(operation, partition); recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := correlationBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		closure = appendClosedGuardAdvice(closure, operation, partition)
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := returnTupleTrueBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := typedNilBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		closure = appendClosedGuardAdvice(closure, operation, partition)
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := selectBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		closure = appendClosedGuardAdvice(closure, operation, partition)
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := typedLiteralBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		closure = appendClosedGuardAdvice(closure, operation, partition)
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := typedIndexBranchClosure(operation, partition); err != nil {
		return equation.TransactionResult{}, err
	} else if recognized {
		closure = appendClosedGuardAdvice(closure, operation, partition)
		return equation.TransactionResult{Complete: true, Closure: closure}, nil
	}
	if closure, recognized, err := typeWitnessBranchClosure(operation, partition); err != nil {
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
	boundaryPossible := false
	unknownCondition := false
	var boundarySource, boundaryValue, boundaryConsumer []byte
	acceptBoundary := func(source []byte) bool {
		term, value, found := explicitAnySourceFact(source, partition.Values())
		if !found {
			return false
		}
		boundaryPossible, boundarySource, boundaryValue, boundaryConsumer = true, term, value, append([]byte(nil), source...)
		return true
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
			// An explicit any ancestor is a published precision boundary, so a
			// truthy arm remains a possible execution path. Select that arm to
			// check its contracts, but do not publish an always-true suggestion:
			// the boundary provides no truth proof.
			if acceptBoundary(operand.Value) {
				continue
			}
			// A compound condition can carry its exact source path only in
			// normalized branch evidence. Defer the fail-closed decision until
			// that closed evidence has been inspected below.
			unknownCondition = true
		}
	}
	if !boundaryPossible {
		for _, operand := range operation.Operands {
			predicate, trueEdge, recognized := branchEvidencePredicate(operand)
			if !recognized || !trueEdge || predicate.Path == "" {
				continue
			}
			acceptBoundary([]byte("path/" + predicate.Path))
		}
	}
	if unknownCondition && !boundaryPossible {
		if closure, recognized, compoundErr := compoundTypeEqualityBranchClosure(operation, partition); compoundErr != nil {
			return equation.TransactionResult{}, compoundErr
		} else if recognized {
			return equation.TransactionResult{Complete: true, Closure: closure}, nil
		}
		return equation.TransactionResult{Complete: true, Closure: undecidedBranchOutcome(operation, partition)}, nil
	}
	truth, frozenGuard := true, false
	var err error
	if !boundaryPossible {
		truth, frozenGuard, err = branchTruth(operation.Operands, partition)
	}
	if errors.Is(err, errUnknownScalar) {
		return equation.TransactionResult{Complete: true, Closure: undecidedBranchOutcome(operation, partition)}, nil
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
	closure.Values = append(closure.Values, equation.Fact{
		Key:   "branch-proof/" + fmt.Sprintf("%x", operation.Target.Body) + "/" + operation.Target.Name + "/" + edge,
		Value: []byte("proven"),
	})
	if boundaryPossible {
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(boundarySource) + "/" + operation.Target.Name, Value: append([]byte(nil), boundaryValue...)})
		if strings.HasPrefix(string(boundaryConsumer), "path/") {
			closure.Values = append(closure.Values, equation.Fact{Key: "gradual-any/" + string(boundaryConsumer) + "/" + operation.Target.Name, Value: append([]byte(nil), boundarySource...)})
		}
	}
	if frozenGuard && truth {
		closure.Outcomes = append(closure.Outcomes, equation.Fact{Key: "frozen-branch/" + operation.Target.Name, Value: []byte("proven")})
	}
	if truth {
		for _, proof := range runtimeTypeBranchProofs(operation) {
			guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
			closure.Values = append(closure.Values, equation.Fact{Key: runtimeTypeProofKey(proof.path, proof.typeName), Value: []byte("proven"), Guards: []equation.Guard{guard}})
		}
	}
	if truth && !boundaryPossible {
		closure.Diagnostics = []equation.Fact{{Key: "advice.always_true_guard/" + operation.Target.Name, Value: []byte("proven constant guard")}}
	}
	if len(operation.Guards) != 0 && !boundaryPossible {
		closure.Diagnostics = append(closure.Diagnostics, equation.Fact{Key: "lint.condition.redundant/" + operation.Target.Name, Value: []byte(strconv.FormatBool(truth))})
	}
	return equation.TransactionResult{Complete: true, Closure: closure}, nil
}

// compoundTypeEqualityBranchClosure closes a short-circuit branch only when
// every front-published true-edge predicate is decidable from current facts.
// The logical result temp is deliberately Top (the type() call has no ordinary
// scalar producer), so this existing compound evidence is the authority for
// reaching the guarded body. A false conjunct proves the false edge; an
// unknown conjunct leaves both edges unselected.
func compoundTypeEqualityBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	hasTypeEquality, hasUnknown, predicates := false, false, 0
	typePredicates := make([]branchPredicateWire, 0)
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "implied-") {
			continue
		}
		predicate, trueEdge, recognized := branchEvidencePredicate(operand)
		if !recognized || !trueEdge {
			continue
		}
		predicates++
		hasTypeEquality = hasTypeEquality || predicate.Kind == "type-equal"
		if predicate.Kind == "type-equal" {
			typePredicates = append(typePredicates, predicate)
		}
		truth, err := evaluateBranchPredicateWire(predicate, partition)
		if errors.Is(err, errUnknownScalar) {
			hasUnknown = true
			continue
		}
		if err != nil {
			return equation.OutputClosure{}, false, err
		}
		if !truth {
			return compoundBranchOutcome(operation, false), true, nil
		}
	}
	if !hasTypeEquality || predicates == 0 || hasUnknown {
		return equation.OutputClosure{}, false, nil
	}
	closure := compoundBranchOutcome(operation, true)
	guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
	for _, predicate := range typePredicates {
		typeName, err := compoundTypeName(predicate, partition)
		if err != nil {
			return equation.OutputClosure{}, false, err
		}
		closure.Values = append(closure.Values, equation.Fact{
			Key:    runtimeTypeProofKey(predicate.Path, typeName),
			Value:  []byte("proven"),
			Guards: []equation.Guard{guard},
		})
	}
	return closure, true, nil
}

func compoundTypeName(predicate branchPredicateWire, partition equation.Partition) (string, error) {
	if predicate.TypeName != "" {
		return predicate.TypeName, nil
	}
	if predicate.OtherPath == "" {
		return "", errUnknownScalar
	}
	other, err := branchPathValue(predicate.OtherPath, partition)
	if err != nil {
		return "", err
	}
	return scalarString(other)
}

func compoundBranchOutcome(operation equation.BoundEquation, truth bool) equation.OutputClosure {
	edge, narrowing := "false", "falsy"
	if truth {
		edge, narrowing = "true", "truthy"
	}
	return equation.OutputClosure{
		Outcomes: []equation.Fact{
			{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge)},
			{Key: "narrowing/" + operation.Target.Name, Value: []byte(narrowing)},
		},
		Values: []equation.Fact{{
			Key:   "branch-proof/" + fmt.Sprintf("%x", operation.Target.Body) + "/" + operation.Target.Name + "/" + edge,
			Value: []byte("proven"),
		}},
	}
}

// undecidedBranchOutcome selects both edges of a branch whose selector the
// engine cannot decide. A predicate it does not understand refutes neither arm,
// so both remain reachable and every obligation they carry is still owed: an
// arm left unselected is an arm never checked, which turns an un-understood
// guard into a proof of nothing at all.
//
// It publishes no branch proof and no narrowing marker beyond the undecided
// edge itself. The two arms stay mutually exclusive guard partitions and
// nothing either writes becomes visible past the join.
//
// The refinement it does carry is the branch's own true-edge evidence: a
// front-published check tagged as holding on the true edge holds wherever that
// edge is taken, whatever decides the selector as a whole. Both the runtime
// type proof and the projected path value are guarded to the true edge, so
// they refine only the arm they belong to and the false edge stays unrefined:
// a conjunct that fails proves nothing about any individual conjunct.
func undecidedBranchOutcome(operation equation.BoundEquation, partition equation.Partition) equation.OutputClosure {
	closure := equation.OutputClosure{}
	trueGuard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
	for _, edge := range [2]string{"true", "false"} {
		guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/" + edge)}
		closure.Outcomes = append(closure.Outcomes,
			equation.Fact{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge), Guards: []equation.Guard{guard}},
			equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("undecided/" + edge), Guards: []equation.Guard{guard}},
		)
	}
	for _, item := range runtimeTypeBranchProofs(operation) {
		closure.Values = append(closure.Values, equation.Fact{
			Key: runtimeTypeProofKey(item.path, item.typeName), Value: []byte("proven"), Guards: []equation.Guard{trueGuard},
		})
	}
	for _, item := range lengthFloorBranchProofs(operation) {
		closure.Values = append(closure.Values, equation.Fact{
			Key:   heapLengthFloorPrefix + heapIndexSubject([]byte("path/"+item.path), partition) + "/" + operation.Target.Name,
			Value: []byte(strconv.FormatInt(item.floor, 10)), Guards: []equation.Guard{trueGuard},
		})
	}
	for _, item := range impliedTrueEdgeNarrowings(operation, partition) {
		encoded, ok := shapefact.EncodeTarget(item.narrowed)
		if !ok {
			continue
		}
		closure.Values = append(closure.Values, equation.Fact{
			Key: "value/" + item.term + "/" + operation.Target.Name, Value: encoded, Guards: []equation.Guard{trueGuard},
		})
	}
	return closure
}

type impliedNarrowing struct {
	term     string
	narrowed typ.Type
}

// impliedTrueEdgeNarrowings folds every true-edge check into the type its path
// carries on that edge. The checks are applied in publication order and each
// one reads the result of the ones before it, so a conjunction narrows exactly
// as its arms compose: `p and p.kind == "x"` first removes nil from p, then
// selects p's "x" arm.
//
// Only an already-published type witness is refined. An un-understood selector
// must not invent a value for a path nothing was proven about, and an empty
// projection means the check refutes its own edge rather than narrowing it.
func impliedTrueEdgeNarrowings(operation equation.BoundEquation, partition equation.Partition) []impliedNarrowing {
	narrowed := make(map[string]typ.Type)
	order := make([]string, 0, len(operation.Operands))
	record := func(term string, value typ.Type) {
		if _, seen := narrowed[term]; !seen {
			order = append(order, term)
		}
		narrowed[term] = value
	}
	current := func(term []byte) (typ.Type, bool) {
		if value, seen := narrowed[string(term)]; seen {
			return value, true
		}
		value, err := resolveCurrentValue(term, partition)
		if err != nil {
			return nil, false
		}
		return shapefact.DecodeTarget(value)
	}
	for _, operand := range operation.Operands {
		predicate, trueEdge, recognized := branchEvidencePredicate(operand)
		if !recognized || !trueEdge || predicate.Path == "" {
			continue
		}
		switch predicate.Kind {
		case "truthy", "falsy", "not-nil", "nil":
			term := "path/" + predicate.Path
			// A truthiness check on a member path is a discriminant over the
			// root's arms: an arm whose member cannot hold that truthiness -
			// including one that does not declare the member at all, because a
			// missing member reads nil - is refuted on this edge.
			if predicate.Kind == "truthy" || predicate.Kind == "falsy" {
				if root, suffix, source, found := typedAncestor([]byte(term), partition); found && len(suffix) != 0 {
					if value, seen := narrowed[string(root)]; seen {
						source = value
					}
					if selected, ok := memberTruthinessNarrowing(source, suffix, (predicate.Kind == "truthy") != predicate.Negated); ok {
						record(string(root), selected)
					}
				}
			}
			witness, known := current([]byte(term))
			if !known {
				continue
			}
			selected, ok := nilabilityProjection(predicate, witness)
			if !ok {
				continue
			}
			record(term, selected)
		case "literal-equal", "literal-not":
			literal, ok := literalType(predicate.Literal)
			if !ok {
				continue
			}
			root, suffix, source, found := typedAncestor([]byte("path/"+predicate.Path), partition)
			if !found || len(suffix) == 0 {
				continue
			}
			if value, seen := narrowed[string(root)]; seen {
				source = value
			}
			selected := typ.Type(nil)
			if (predicate.Kind == "literal-equal") != predicate.Negated {
				selected, ok = variant.NarrowByPathLiteral(source, suffix, literal)
			} else {
				selected, ok = variant.NarrowByPathLiteralNot(source, suffix, literal)
			}
			if !ok || selected == nil || typ.IsNever(selected) {
				continue
			}
			record(string(root), selected)
		case "path-equal":
			// An equality relates two typed paths on its true edge: each side's
			// value set is the intersection of the two. When a side is a member of
			// a tagged union, its receiver keeps only the arms whose member can
			// equal the peer's value set and refutes the arms proven disjoint. The
			// relation is symmetric, so both receivers narrow the same way; a peer
			// with no member ancestor simply supplies the constraint type.
			if predicate.Negated || predicate.OtherPath == "" {
				continue
			}
			for _, pair := range [2][2]string{{predicate.Path, predicate.OtherPath}, {predicate.OtherPath, predicate.Path}} {
				memberPath, peerPath := pair[0], pair[1]
				root, suffix, source, found := typedAncestor([]byte("path/"+memberPath), partition)
				if !found || len(suffix) == 0 {
					continue
				}
				if value, seen := narrowed[string(root)]; seen {
					source = value
				}
				peer, peerKnown := current([]byte("path/" + peerPath))
				if !peerKnown || peer == nil {
					continue
				}
				selected, ok := variant.NarrowByPathType(source, suffix, peer)
				if !ok || selected == nil || typ.IsNever(selected) {
					continue
				}
				record(string(root), selected)
			}
		}
	}
	out := make([]impliedNarrowing, 0, len(order))
	for _, term := range order {
		out = append(out, impliedNarrowing{term: term, narrowed: narrowed[term]})
	}
	return out
}

// nilabilityProjection selects the side of witness that a truthiness or nil
// check keeps on the edge where it holds.
func nilabilityProjection(predicate branchPredicateWire, witness typ.Type) (typ.Type, bool) {
	holds := !predicate.Negated
	var selected typ.Type
	switch predicate.Kind {
	case "truthy", "falsy":
		truthy, falsy, split := proof.TruthinessSplit(witness)
		if !split {
			return nil, false
		}
		selected = truthy
		if (predicate.Kind == "falsy") == holds {
			selected = falsy
		}
	case "not-nil", "nil":
		selected = proof.ProjectionWithoutNil(witness)
		if (predicate.Kind == "nil") == holds {
			selected = typ.Nil
		}
	default:
		return nil, false
	}
	if selected == nil || typ.IsNever(selected) {
		return nil, false
	}
	return selected, true
}

// memberTruthinessNarrowing selects the arms of a receiver that can hold the
// requested truthiness at a member path. A missing member reads nil, so an arm
// that does not declare the member is refuted on the truthy edge and retained
// on the falsy one; this is the ordinary tagged-union discriminant.
func memberTruthinessNarrowing(source typ.Type, suffix []segment.Segment, wantTruthy bool) (typ.Type, bool) {
	if source == nil || len(suffix) == 0 {
		return nil, false
	}
	selected, ok := typ.Type(nil), false
	if wantTruthy {
		selected, ok = variant.NarrowByPathTruthy(source, suffix)
	} else {
		selected, ok = variant.NarrowByPathFalsy(source, suffix)
	}
	if !ok || selected == nil || typ.IsNever(selected) {
		return nil, false
	}
	return selected, true
}

// typeWitnessBranchClosure refines a falsiness branch whose selector resolves to
// a published type witness instead of a decided value. A type is not a value: it
// cannot select an arm, so both arms stay reachable and each publishes the
// projection of the guarded path under its own edge guard. When one projection
// is empty the witness does decide the branch, and the ordinary scalar rule owns
// that case together with the constant-guard advice it publishes.
func typeWitnessBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	predicate, found, err := soleBranchPredicate(operation)
	if err != nil || !found || predicate.Path == "" {
		return equation.OutputClosure{}, false, err
	}
	switch predicate.Kind {
	case "truthy", "falsy":
	default:
		return equation.OutputClosure{}, false, nil
	}
	term := []byte("path/" + predicate.Path)
	value, err := resolveCurrentValue(term, partition)
	if err != nil {
		return equation.OutputClosure{}, false, err
	}
	witness, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		return equation.OutputClosure{}, false, nil
	}
	selected, rejected, split := proof.TruthinessSplit(witness)
	if !split || selected == nil || rejected == nil || typ.IsNever(selected) || typ.IsNever(rejected) {
		return equation.OutputClosure{}, false, nil
	}
	if predicate.Kind == "falsy" {
		selected, rejected = rejected, selected
	}
	if predicate.Negated {
		selected, rejected = rejected, selected
	}
	// The same check is a discriminant over the receiver of a member path: the
	// arms that cannot hold this truthiness at that member are refuted on the
	// edge where it holds. Both edges keep their own projection, so neither
	// selection escapes its guard.
	rootTerm, rootSelected, rootRejected := []byte(nil), typ.Type(nil), typ.Type(nil)
	if root, suffix, source, found := typedAncestor(term, partition); found && len(suffix) != 0 {
		wantTruthy := (predicate.Kind == "truthy") != predicate.Negated
		trueSide, trueOK := memberTruthinessNarrowing(source, suffix, wantTruthy)
		falseSide, falseOK := memberTruthinessNarrowing(source, suffix, !wantTruthy)
		if trueOK || falseOK {
			rootTerm, rootSelected, rootRejected = root, trueSide, falseSide
		}
	}
	closure := equation.OutputClosure{}
	for _, edge := range [2]struct {
		name  string
		type_ typ.Type
		root  typ.Type
	}{{"true", selected, rootSelected}, {"false", rejected, rootRejected}} {
		encoded, ok := shapefact.EncodeTarget(edge.type_)
		if !ok {
			return equation.OutputClosure{}, false, nil
		}
		guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/" + edge.name)}
		closure.Outcomes = append(closure.Outcomes,
			equation.Fact{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge.name), Guards: []equation.Guard{guard}},
			equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("type-witness/" + edge.name), Guards: []equation.Guard{guard}},
		)
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(term) + "/" + operation.Target.Name, Value: encoded, Guards: []equation.Guard{guard}})
		if len(rootTerm) == 0 || edge.root == nil {
			continue
		}
		rootEncoded, rootOK := shapefact.EncodeTarget(edge.root)
		if !rootOK {
			continue
		}
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(rootTerm) + "/" + operation.Target.Name, Value: rootEncoded, Guards: []equation.Guard{guard}})
	}
	return closure, true, nil
}

// soleBranchPredicate returns the branch's single predicate operand. A branch
// carrying more than one predicate has no single selector to project.
func soleBranchPredicate(operation equation.BoundEquation) (branchPredicateWire, bool, error) {
	var predicate branchPredicateWire
	found := false
	for _, operand := range operation.Operands {
		if operand.Role != "predicate" {
			continue
		}
		if found || !strings.HasPrefix(string(operand.Value), branchPredicatePrefix) {
			return branchPredicateWire{}, false, nil
		}
		if err := json.Unmarshal(operand.Value[len(branchPredicatePrefix):], &predicate); err != nil {
			return branchPredicateWire{}, false, fmt.Errorf("engine: decode branch predicate: %w", err)
		}
		found = true
	}
	return predicate, found, nil
}

// typePredicateBranchClosure projects the `T:is(value)` witness through the
// exact polarity of a guard over its error slot. Both polarities are the two
// halves of the same contract: the error slot is non-nil exactly when the value
// slot is nil, so a not-nil error edge proves the value nil, and a nil error
// edge proves the value the checked type T. Only the true edge carries a
// projection; the false edge negates a compound guard and proves nothing here.
func typePredicateBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool) {
	var errorPath string
	errorAbsent := false
	for _, operand := range operation.Operands {
		predicate, trueEdge, ok := branchEvidencePredicate(operand)
		if !ok || !trueEdge || predicate.Negated || predicate.Path == "" {
			continue
		}
		if predicate.Kind == "not-nil" || predicate.Kind == "nil" {
			errorPath = "path/" + predicate.Path
			errorAbsent = predicate.Kind == "nil"
			break
		}
	}
	if errorPath == "" {
		return equation.OutputClosure{}, false
	}
	encodedError := base64.RawURLEncoding.EncodeToString([]byte(errorPath))
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, typePredicatePairPrefix) || fact.Value == nil {
			continue
		}
		if _, valid := shapefact.DecodeTarget(fact.Value); !valid {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(fact.Key, typePredicatePairPrefix), "/")
		if len(parts) != 2 || parts[1] != encodedError {
			continue
		}
		valuePath, err := base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil || !strings.HasPrefix(string(valuePath), "path/") {
			continue
		}
		// A nil error edge validates the value: it holds the checked type T. A
		// non-nil error edge invalidates it: the value slot is nil.
		trueValue := []byte("scalar/nil")
		if errorAbsent {
			trueValue = append([]byte(nil), fact.Value...)
		}
		trueGuard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
		falseGuard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/false")}
		return equation.OutputClosure{
			Outcomes: []equation.Fact{
				{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/true"), Guards: []equation.Guard{trueGuard}},
				{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/false"), Guards: []equation.Guard{falseGuard}},
				{Key: "narrowing/" + operation.Target.Name, Value: []byte("type-predicate/true"), Guards: []equation.Guard{trueGuard}},
				{Key: "narrowing/" + operation.Target.Name, Value: []byte("type-predicate/false"), Guards: []equation.Guard{falseGuard}},
			},
			Values: []equation.Fact{{Key: "value/" + string(valuePath) + "/" + operation.Target.Name, Value: trueValue, Guards: []equation.Guard{trueGuard}}},
		}, true
	}
	return equation.OutputClosure{}, false
}

// appendClosedGuardAdvice preserves an existing branch closure while allowing
// a dominated selector to publish the same constant-guard fact as the scalar
// branch path.  Specialized closures own their narrowing facts and therefore
// return before branchKernel's ordinary diagnostic tail.  This adapter never
// trusts the dominance guard by itself: it asks the current partition for the
// exact selector truth, so a write between guards (or an opaque value) stays
// silent.
func appendClosedGuardAdvice(closure equation.OutputClosure, operation equation.BoundEquation, partition equation.Partition) equation.OutputClosure {
	if len(operation.Guards) == 0 {
		return closure
	}
	truth, _, err := branchTruth(operation.Operands, partition)
	if err != nil || !truth {
		return closure
	}
	closure.Diagnostics = append(closure.Diagnostics, equation.Fact{
		Key: "advice.always_true_guard/" + operation.Target.Name, Value: []byte("proven constant guard"),
	})
	return closure
}

// typedNilBranchClosure projects an existing optional member witness through
// the exact nil predicate that selects it.  It does not infer a member type:
// correlationMemberType accepts either the current closed value or the
// declaration already published for that exact path.  Both edge values stay
// under the front-owned branch guard, so a write on another epoch cannot leak
// the refinement past this branch.
func typedNilBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	var predicate branchPredicateWire
	found := false
	for _, operand := range operation.Operands {
		if operand.Role != "predicate" {
			continue
		}
		if found || !strings.HasPrefix(string(operand.Value), branchPredicatePrefix) {
			return equation.OutputClosure{}, false, nil
		}
		if err := json.Unmarshal(operand.Value[len(branchPredicatePrefix):], &predicate); err != nil {
			return equation.OutputClosure{}, false, fmt.Errorf("engine: decode nil branch predicate: %w", err)
		}
		found = true
	}
	if !found || predicate.Path == "" || predicate.Negated || (predicate.Kind != "nil" && predicate.Kind != "not-nil") {
		return equation.OutputClosure{}, false, nil
	}
	term := []byte("path/" + predicate.Path)
	if !derivedPathTerm(term) {
		return equation.OutputClosure{}, false, nil
	}
	source, known := correlationMemberType(term, partition)
	if !known || !optionalConcreteWitnessType(source) {
		return equation.OutputClosure{}, false, nil
	}
	nonNil := proof.ProjectionWithoutNil(source)
	if nonNil == nil || typ.IsNever(nonNil) {
		return equation.OutputClosure{}, false, nil
	}
	trueType, falseType := nonNil, typ.Type(typ.Nil)
	if predicate.Kind == "nil" {
		trueType, falseType = typ.Nil, nonNil
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
			equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("typed-nil/" + edge.name), Guards: []equation.Guard{guard}},
		)
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(term) + "/" + operation.Target.Name, Value: value, Guards: []equation.Guard{guard}})
	}
	return closure, true, nil
}

type correlationConeEpoch struct {
	Term  string `json:"term"`
	Epoch string `json:"epoch,omitempty"`
}

// correlationConeValue is a guarded branch projection, not a replacement
// value.  Its epoch rows are the complete proof cone: the guarded member, its
// equal peer, and every enclosing path segment of each.  An absent epoch is
// meaningful--a later write creates one and therefore revokes the projection.
type correlationConeValue struct {
	Value  []byte                 `json:"value"`
	Epochs []correlationConeEpoch `json:"epochs"`
}

// correlationBranchClosure publishes the one existing fact that a true
// equality/non-nil branch proves: an equal member has the same non-nil type as
// the guarded member.  It accepts only closed front evidence, requires equal
// current member surfaces, and keeps the projection under the branch's true
// guard.  No alias is inferred from source spelling or from a declaration.
func correlationBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	equal := make(map[string]map[string]bool)
	nonNil := make(map[string]bool)
	for _, operand := range operation.Operands {
		predicate, trueEdge, ok := branchEvidencePredicate(operand)
		if !ok || !trueEdge || predicate.Negated {
			continue
		}
		switch predicate.Kind {
		case "path-equal":
			if predicate.Path == "" || predicate.OtherPath == "" {
				continue
			}
			if equal[predicate.Path] == nil {
				equal[predicate.Path] = make(map[string]bool)
			}
			if equal[predicate.OtherPath] == nil {
				equal[predicate.OtherPath] = make(map[string]bool)
			}
			equal[predicate.Path][predicate.OtherPath] = true
			equal[predicate.OtherPath][predicate.Path] = true
		case "not-nil":
			if predicate.Path != "" {
				nonNil[predicate.Path] = true
			}
		}
	}
	if len(equal) == 0 || len(nonNil) == 0 {
		return equation.OutputClosure{}, false, nil
	}

	guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
	closure := equation.OutputClosure{Outcomes: []equation.Fact{
		{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/true"), Guards: []equation.Guard{guard}},
		{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/false"), Guards: []equation.Guard{{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/false")}}},
		{Key: "narrowing/" + operation.Target.Name, Value: []byte("correlation/true"), Guards: []equation.Guard{guard}},
		{Key: "narrowing/" + operation.Target.Name, Value: []byte("correlation/false"), Guards: []equation.Guard{{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/false")}}},
	}}
	sourcePaths := make([]string, 0, len(nonNil))
	for sourcePath := range nonNil {
		sourcePaths = append(sourcePaths, sourcePath)
	}
	sort.Strings(sourcePaths)
	for _, sourcePath := range sourcePaths {
		source := []byte("path/" + sourcePath)
		if !derivedPathTerm(source) {
			continue
		}
		sourceType, sourceKnown := correlationMemberType(source, partition)
		if !sourceKnown || sourceType == nil || !subtype.IsSubtype(typ.Nil, sourceType) {
			continue
		}
		for _, targetPath := range correlatedPathTargets(sourcePath, equal) {
			if targetPath == sourcePath {
				continue
			}
			target := []byte("path/" + targetPath)
			if !derivedPathTerm(target) {
				continue
			}
			targetType, targetKnown := correlationMemberType(target, partition)
			if !targetKnown || targetType == nil || !typ.TypeEquals(sourceType, targetType) {
				continue
			}
			narrowed := proof.ProjectionWithoutNil(targetType)
			if narrowed == nil || typ.IsNever(narrowed) {
				continue
			}
			value, encoded := shapefact.EncodeTarget(narrowed)
			if !encoded {
				continue
			}
			epochs, complete := correlationConeEpochs(sourcePath, targetPath, partition)
			if !complete {
				continue
			}
			wire, marshalErr := json.Marshal(correlationConeValue{Value: value, Epochs: epochs})
			if marshalErr != nil {
				return equation.OutputClosure{}, false, marshalErr
			}
			closure.Values = append(closure.Values, equation.Fact{
				Key:   correlationConeValuePrefix + base64.RawURLEncoding.EncodeToString(target) + "/" + operation.Target.Name,
				Value: wire, Guards: []equation.Guard{guard},
			})
		}
	}
	if len(closure.Values) == 0 {
		return equation.OutputClosure{}, false, nil
	}
	return closure, true, nil
}

// returnTupleTrueBranchClosure applies the one directional fact carried by an
// imported finite return catalog: when its exact boolean result slot is true,
// the paired result slot is present. Both branch outcomes remain guarded; a
// catalog with no true tuple, a nil true companion, or a stale/untyped target
// is deliberately unavailable.
func returnTupleTrueBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	predicate, found, err := soleBranchPredicate(operation)
	if err != nil || !found || predicate.Kind != "truthy" || predicate.Negated || predicate.Path == "" {
		return equation.OutputClosure{}, false, err
	}
	trigger := []byte("path/" + predicate.Path)
	prefix := returnTupleTruePrefix + base64.RawURLEncoding.EncodeToString(trigger) + "/"
	guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
	closure := equation.OutputClosure{}
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) || string(fact.Value) != "proven" {
			continue
		}
		encoded := strings.TrimPrefix(fact.Key, prefix)
		target, decodeErr := base64.RawURLEncoding.DecodeString(encoded)
		if decodeErr != nil || !strings.HasPrefix(string(target), "path/") {
			continue
		}
		valueType, known := correlationMemberType(target, partition)
		if !known || !optionalConcreteWitnessType(valueType) {
			continue
		}
		narrowed := proof.ProjectionWithoutNil(valueType)
		value, valueEncoded := shapefact.EncodeTarget(narrowed)
		if !valueEncoded || narrowed == nil || typ.IsNever(narrowed) {
			continue
		}
		closure.Values = append(closure.Values, equation.Fact{Key: "value/" + string(target) + "/" + operation.Target.Name, Value: value, Guards: []equation.Guard{guard}})
	}
	if len(closure.Values) == 0 {
		return equation.OutputClosure{}, false, nil
	}
	falseGuard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/false")}
	closure.Outcomes = append(closure.Outcomes,
		equation.Fact{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/true"), Guards: []equation.Guard{guard}},
		equation.Fact{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/false"), Guards: []equation.Guard{falseGuard}},
		equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("return-tuple/true"), Guards: []equation.Guard{guard}},
		equation.Fact{Key: "narrowing/" + operation.Target.Name, Value: []byte("return-tuple/false"), Guards: []equation.Guard{falseGuard}},
	)
	return closure, true, nil
}

// correlatedPathTargets extends an equality over the exact suffix already
// guarded by a non-nil predicate. Thus `p == q` relates `p.f` and `q.f`, while
// `p.inner == q.inner` relates `p.inner.x` and `q.inner.x`; a textual prefix
// that cuts through a path segment is never an alias.
func correlatedPathTargets(source string, equal map[string]map[string]bool) []string {
	targets := make(map[string]bool)
	for base := range equal {
		suffix, matches := correlationPathSuffix(source, base)
		if !matches {
			continue
		}
		for _, peer := range equalityCone(base, equal) {
			if peer != base {
				targets[peer+suffix] = true
			}
		}
	}
	out := make([]string, 0, len(targets))
	for target := range targets {
		out = append(out, target)
	}
	sort.Strings(out)
	return out
}

func correlationPathSuffix(path, prefix string) (string, bool) {
	if prefix == "" || !strings.HasPrefix(path, prefix) {
		return "", false
	}
	suffix := strings.TrimPrefix(path, prefix)
	if suffix == "" || (suffix[0] != '.' && suffix[0] != '[') || !segment.ValidFormattedSegments(suffix) {
		return "", false
	}
	return suffix, true
}

// correlationMemberType reads an existing current value witness first.  A
// declared root is the only fallback: it is a front-published contract, and
// the cone snapshot below binds that contract to the exact current path
// versions.  No local source annotation or synthesized alias can enter here.
func correlationMemberType(term []byte, partition equation.Partition) (typ.Type, bool) {
	if value, err := resolveCurrentValue(term, partition); err == nil {
		if valueType, known := expressionValueType(value); known && valueType != nil {
			return valueType, true
		}
	}
	if valueType, known := typedPathType(term, partition); known && valueType != nil {
		return valueType, true
	}
	root, suffix, valid := tableAddress(term)
	if !valid || suffix == "" {
		return nil, false
	}
	declared, found := declaredTypeForTerm(root, partition)
	if !found || declared == nil {
		return nil, false
	}
	segments, parsed := segment.ParseFormattedSegments(suffix)
	if !parsed || len(segments) == 0 {
		return nil, false
	}
	projected, projectedOK := luatypeprojection.ApplySegments(declared, segments)
	return projected, projectedOK && projected != nil
}

func equalityCone(source string, equal map[string]map[string]bool) []string {
	seen := map[string]bool{source: true}
	queue := []string{source}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for next := range equal[current] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func correlationConeEpochs(source, target string, partition equation.Partition) ([]correlationConeEpoch, bool) {
	terms := make(map[string]bool)
	for _, path := range []string{source, target} {
		root, suffix, ok := tableAddress([]byte("path/" + path))
		if !ok || suffix == "" {
			return nil, false
		}
		segments, valid := segment.ParseFormattedSegments(suffix)
		if !valid || len(segments) == 0 {
			return nil, false
		}
		terms[string(root)] = true
		for index := range segments {
			terms[string(root)+segment.FormatSegments(segments[:index+1])] = true
		}
	}
	ordered := make([]string, 0, len(terms))
	for term := range terms {
		ordered = append(ordered, term)
	}
	sort.Strings(ordered)
	epochs := make([]correlationConeEpoch, 0, len(ordered))
	for _, term := range ordered {
		epoch, _ := currentEpoch([]byte(term), partition)
		epochs = append(epochs, correlationConeEpoch{Term: term, Epoch: epoch})
	}
	return epochs, true
}

type runtimeTypeBranchProof struct{ path, typeName string }

func runtimeTypeBranchProofs(operation equation.BoundEquation) []runtimeTypeBranchProof {
	seen := make(map[string]bool)
	proofs := make([]runtimeTypeBranchProof, 0)
	for _, operand := range operation.Operands {
		predicate, trueEdge, recognized := branchEvidencePredicate(operand)
		if !recognized || !trueEdge {
			continue
		}
		if predicate.Kind != "type-equal" || predicate.Path == "" || predicate.TypeName == "" {
			continue
		}
		switch predicate.TypeName {
		case "nil", "boolean", "number", "string", "table", "function":
			key := predicate.Path + "\x00" + predicate.TypeName
			if !seen[key] {
				seen[key] = true
				proofs = append(proofs, runtimeTypeBranchProof{path: predicate.Path, typeName: predicate.TypeName})
			}
		}
	}
	return proofs
}

func runtimeTypeProofKey(path, typeName string) string {
	return "runtime-type-proof/" + base64.RawURLEncoding.EncodeToString([]byte("path/"+path)) + "/" + typeName
}

type lengthFloorBranchProof struct {
	path  string
	floor int64
}

// lengthFloorBranchProofs returns the sequence length lower bounds a branch's
// true edge establishes. A normalized `#xs >= k` check is the only source: it
// is already the front's canonical form for every non-empty guard spelling, so
// no comparison is re-read here.
func lengthFloorBranchProofs(operation equation.BoundEquation) []lengthFloorBranchProof {
	proofs := make([]lengthFloorBranchProof, 0)
	for _, operand := range operation.Operands {
		predicate, trueEdge, recognized := branchEvidencePredicate(operand)
		if !recognized || !trueEdge || predicate.Negated || predicate.Kind != "len-ge" || predicate.Path == "" || predicate.LenFloor < 1 {
			continue
		}
		index := -1
		for position, existing := range proofs {
			if existing.path == predicate.Path {
				index = position
				break
			}
		}
		if index < 0 {
			proofs = append(proofs, lengthFloorBranchProof{path: predicate.Path, floor: predicate.LenFloor})
			continue
		}
		if predicate.LenFloor > proofs[index].floor {
			proofs[index].floor = predicate.LenFloor
		}
	}
	return proofs
}

// lengthFloorProven returns the largest currently proven length lower bound for
// container. A dynamic write through the same heap identity revokes the bound
// exactly as it revokes an index presence proof: a write can move the sequence
// border, so a bound established before it proves nothing after it.
func lengthFloorProven(container []byte, partition equation.Partition) int64 {
	subject := heapIndexSubject(container, partition)
	prefix := heapLengthFloorPrefix + subject + "/"
	floor, proofPoint := int64(0), ""
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		bound, err := strconv.ParseInt(string(fact.Value), 10, 64)
		if err != nil || bound <= floor {
			continue
		}
		floor, proofPoint = bound, fact.Key
	}
	if floor == 0 {
		return 0
	}
	revokePrefix := heapIndexRevokePrefix + subject + "/"
	revocation := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, revokePrefix) && string(fact.Value) == "revoked" && fact.Key > revocation {
			revocation = fact.Key
		}
	}
	if revocation != "" && factOperation(proofPoint) <= factOperation(revocation) {
		return 0
	}
	return floor
}

// provenSequenceIndexValue reads a constant sequence index that a proven length
// lower bound puts in range. The declared element type is the value of such a
// read: the optional the ordinary projection returns describes the slot's
// possible absence, and the bound is exactly the proof that it is present. An
// element type that is itself optional keeps its own nil.
func provenSequenceIndexValue(term []byte, partition equation.Partition) ([]byte, bool) {
	root, suffix, valid := tableAddress(term)
	if !valid || suffix == "" {
		return nil, false
	}
	segments, parsed := segment.ParseFormattedSegments(suffix)
	if !parsed || len(segments) == 0 {
		return nil, false
	}
	last := segments[len(segments)-1]
	if last.Kind != segment.SegmentIndexInt || last.Index < 1 {
		return nil, false
	}
	container := append(append([]byte(nil), root...), []byte(segment.FormatSegments(segments[:len(segments)-1]))...)
	if int64(last.Index) > lengthFloorProven(container, partition) {
		return nil, false
	}
	value, err := resolveCurrentValue(container, partition)
	if err != nil {
		return nil, false
	}
	witness, decoded := shapefact.DecodeTarget(value)
	if !decoded {
		return nil, false
	}
	array, ok := unwrap.Alias(subst.ExpandInstantiated(witness)).(*typ.Array)
	if !ok || array == nil || array.Element == nil {
		return nil, false
	}
	return shapefact.EncodeTarget(array.Element)
}

// runtimeTypeValidationProves consumes only the fact emitted by a true edge
// of `type(path) == name`. It never treats a literal value carried through an
// any boundary as proof: sibling paths remain unvalidated.
func runtimeTypeValidationProves(source []byte, targetType string, shapeTarget []byte, partition equation.Partition) bool {
	name, err := strconv.Unquote(strings.TrimPrefix(targetType, "claim-type/"))
	if err != nil || source == nil || !strings.HasPrefix(string(source), "path/") {
		return false
	}
	// Lua's type predicate certifies only the runtime kind. A record, array,
	// map, tuple, or other structural target can share the "table" runtime
	// kind without satisfying its member contract, so it must retain the
	// explicit-any boundary until an existing structural validator proves it.
	if target, decoded := shapefact.DecodeTarget(shapeTarget); decoded && target != nil && !runtimeTypeProofAdmitsTarget(name, target) {
		return false
	}
	return runtimeTypeProven(source, name, partition)
}

// runtimeTypeProven reports whether a true edge of `type(term) == name` is
// currently visible to this consumer. It is the single reader of the proof
// fact family: a guard partition that does not include the proving edge never
// sees the fact at all.
func runtimeTypeProven(term []byte, name string, partition equation.Partition) bool {
	if !strings.HasPrefix(string(term), "path/") {
		return false
	}
	prefix := runtimeTypeProofKey(strings.TrimPrefix(string(term), "path/"), name)
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "proven" {
			return true
		}
	}
	return false
}

func runtimeTypeProofAdmitsTarget(name string, target typ.Type) bool {
	target = unwrap.Alias(subst.ExpandInstantiated(target))
	if target == nil {
		return false
	}
	switch name {
	case "nil":
		return target.Kind() == kind.Nil
	case "boolean":
		return target.Kind() == kind.Boolean
	case "number":
		return target.Kind() == kind.Number || target.Kind() == kind.Integer
	case "string":
		return target.Kind() == kind.String
	case "function":
		return target.Kind() == kind.Function
	default:
		return false
	}
}

// typedIndexBranchClosure carries normalized numeric/index relations through
// an unknown runtime branch as guarded facts. The branch remains bifurcated;
// only the true edge receives a presence fact, and that fact is tied to the
// existing table identity (or, when no identity is available, its exact path).
func typedIndexBranchClosure(operation equation.BoundEquation, partition equation.Partition) (equation.OutputClosure, bool, error) {
	hasConsumer := false
	predicates := make([]branchPredicateWire, 0, len(operation.Operands))
	for _, operand := range operation.Operands {
		if operand.Role == "index-presence-consumer" && string(operand.Value) == "scalar/bool/true" {
			hasConsumer = true
		}
		if operand.Role == "predicate" {
			predicate, _, ok := branchEvidencePredicate(operand)
			if !ok {
				return equation.OutputClosure{}, false, nil
			}
			// A direct runtime-type predicate must remain on the ordinary branch
			// path, which owns explicit-any validation. Compound numeric guards,
			// however, carry their individual bounds as normalized evidence rather
			// than as this direct predicate and must retain the established index
			// narrowing path.
			if predicate.Kind == "type-equal" {
				return equation.OutputClosure{}, false, nil
			}
		}
		predicate, trueEdge, ok := branchEvidencePredicate(operand)
		if !ok || !trueEdge {
			continue
		}
		switch predicate.Kind {
		case "num-ge", "index-in-range":
			predicates = append(predicates, predicate)
		}
	}
	hasIndexBound := false
	for _, predicate := range predicates {
		hasIndexBound = hasIndexBound || predicate.Kind == "index-in-range"
	}
	if !hasIndexBound && !hasConsumer {
		return equation.OutputClosure{}, false, nil
	}
	guard := equation.Guard{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/true")}
	closure := equation.OutputClosure{Outcomes: []equation.Fact{
		{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/true"), Guards: []equation.Guard{guard}},
		{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/false"), Guards: []equation.Guard{{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/false")}}},
		{Key: "narrowing/" + operation.Target.Name, Value: []byte("index/true"), Guards: []equation.Guard{guard}},
		{Key: "narrowing/" + operation.Target.Name, Value: []byte("index/false"), Guards: []equation.Guard{{Body: operation.Target.Body, Encoding: []byte("front/branch/" + operation.Target.Name + "/false")}}},
	}}
	lower, upper := publishedIndexRelations(partition)
	for _, predicate := range predicates {
		if predicate.Negated || predicate.Path == "" {
			continue
		}
		index := []byte("path/" + predicate.Path)
		encodedIndex := base64.RawURLEncoding.EncodeToString(index)
		switch predicate.Kind {
		case "num-ge":
			if predicate.NumFloor < 1 {
				continue
			}
			lower[encodedIndex] = index
			closure.Values = append(closure.Values, equation.Fact{Key: heapIndexLowerPrefix + encodedIndex + "/" + operation.Target.Name, Value: []byte("proven"), Guards: []equation.Guard{guard}})
		case "index-in-range":
			if predicate.OtherPath == "" {
				continue
			}
			container := []byte("path/" + predicate.OtherPath)
			encodedContainer := base64.RawURLEncoding.EncodeToString(container)
			upper[encodedIndex+"/"+encodedContainer] = struct{ index, container []byte }{index, container}
			closure.Values = append(closure.Values, equation.Fact{Key: heapIndexUpperPrefix + encodedIndex + "/" + encodedContainer + "/" + operation.Target.Name, Value: []byte("proven"), Guards: []equation.Guard{guard}})
		}
	}
	for relation, pair := range upper {
		encodedIndex, _, _ := strings.Cut(relation, "/")
		if _, found := lower[encodedIndex]; !found {
			continue
		}
		closure.Values = append(closure.Values, equation.Fact{
			Key:   heapIndexPresencePrefix + heapIndexSubject(pair.container, partition) + "/" + encodedIndex + "/" + operation.Target.Name,
			Value: []byte("proven"), Guards: []equation.Guard{guard},
		})
	}
	// Index evidence can be one conjunct of a broader condition such as
	// `entry and entry.next_id`. This specialized branch owns the index proof,
	// but the true edge still owns the ordinary non-nil refinements emitted by
	// the same front evidence.
	for _, item := range impliedTrueEdgeNarrowings(operation, partition) {
		encoded, ok := shapefact.EncodeTarget(item.narrowed)
		if !ok {
			continue
		}
		closure.Values = append(closure.Values, equation.Fact{
			Key: "value/" + item.term + "/" + operation.Target.Name, Value: encoded, Guards: []equation.Guard{guard},
		})
	}
	return closure, true, nil
}

func publishedIndexRelations(partition equation.Partition) (map[string][]byte, map[string]struct{ index, container []byte }) {
	lower := make(map[string][]byte)
	upper := make(map[string]struct{ index, container []byte })
	for _, fact := range partition.Values() {
		if string(fact.Value) != "proven" {
			continue
		}
		if rest, found := strings.CutPrefix(fact.Key, heapIndexLowerPrefix); found {
			index, _, valid := strings.Cut(rest, "/")
			if decoded, err := base64.RawURLEncoding.DecodeString(index); valid && err == nil {
				lower[index] = decoded
			}
			continue
		}
		rest, found := strings.CutPrefix(fact.Key, heapIndexUpperPrefix)
		if !found {
			continue
		}
		parts := strings.Split(rest, "/")
		if len(parts) < 3 {
			continue
		}
		index, indexErr := base64.RawURLEncoding.DecodeString(parts[0])
		container, containerErr := base64.RawURLEncoding.DecodeString(parts[1])
		if indexErr == nil && containerErr == nil {
			upper[parts[0]+"/"+parts[1]] = struct{ index, container []byte }{index, container}
		}
	}
	return lower, upper
}

func branchEvidencePredicate(operand equation.BoundOperand) (branchPredicateWire, bool, bool) {
	encoded := operand.Value
	if strings.HasPrefix(string(encoded), branchEvidencePrefix) {
		rest := strings.TrimPrefix(string(encoded), branchEvidencePrefix)
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) != 3 || parts[0] != "true" || parts[1] != "true" {
			return branchPredicateWire{}, false, false
		}
		encoded = []byte(parts[2])
	} else if operand.Role != "predicate" {
		return branchPredicateWire{}, false, false
	}
	if !strings.HasPrefix(string(encoded), branchPredicatePrefix) {
		return branchPredicateWire{}, false, false
	}
	var predicate branchPredicateWire
	if json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate) != nil || predicate.Kind == "" {
		return branchPredicateWire{}, false, false
	}
	return predicate, true, true
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
	if predicate.Path == "" || predicate.Negated {
		return equation.OutputClosure{}, false, nil
	}
	literal := typ.Type(nil)
	switch predicate.Kind {
	case "literal-equal", "literal-not":
		if predicate.Literal == "" {
			return equation.OutputClosure{}, false, nil
		}
		var literalOK bool
		literal, literalOK = literalType(predicate.Literal)
		if !literalOK {
			return equation.OutputClosure{}, false, nil
		}
	case "truthy", "falsy":
		literal = typ.LiteralBool(true)
	default:
		return equation.OutputClosure{}, false, nil
	}
	term := []byte("path/" + predicate.Path)
	root, suffix, source, ok := typedAncestor(term, partition)
	trueType, falseType := typ.Type(nil), typ.Type(nil)
	if ok && len(suffix) != 0 {
		var trueOK, falseOK bool
		trueType, trueOK = variant.NarrowByPathLiteral(source, suffix, literal)
		falseType, falseOK = variant.NarrowByPathLiteralNot(source, suffix, literal)
		if predicate.Kind == "literal-not" || predicate.Kind == "falsy" {
			trueType, falseType = falseType, trueType
			trueOK, falseOK = falseOK, trueOK
		}
		if !trueOK || !falseOK {
			return equation.OutputClosure{}, false, nil
		}
	} else {
		// A root truthiness guard has no member suffix for typedAncestor to
		// traverse. It may still narrow an exact current call-result summary.
		// The summary must have been published at this path's current epoch;
		// annotations and historical writes remain unavailable. The false edge
		// conservatively retains the broader source type, so false-like concrete
		// values cannot be lost.
		if predicate.Kind != "truthy" {
			return equation.OutputClosure{}, false, nil
		}
		encoded, methodResult := currentEpochFact(methodReturnSummaryPrefix, term, partition)
		found := methodResult
		if !found {
			encoded, found = currentEpochFact(summaryTypePrefix, term, partition)
		}
		if !found {
			return equation.OutputClosure{}, false, nil
		}
		var decodeErr error
		source, decodeErr = typ.DecodeCanonical(context.Background(), encoded)
		if decodeErr != nil || !rootOptionalRecordSummary(source) {
			return equation.OutputClosure{}, false, nil
		}
		trueType, falseType, root = proof.ProjectionWithoutNil(source), source, term
		if trueType == nil || typ.IsNever(trueType) {
			return equation.OutputClosure{}, false, nil
		}
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

// rootOptionalClosedSummary recognizes the finite imported-result surface that
// can cross a root truthiness guard without source reconstruction. Scalars and
// open values retain the ordinary branch path; the non-nil projection must be
// an existing closed record surface (including a finite record union).
func rootOptionalClosedSummary(source typ.Type) bool {
	if source == nil || !proof.OptionalTypeHasConcreteValue(source) {
		return false
	}
	return closedMemberSurface(proof.ProjectionWithoutNil(source))
}

func rootOptionalRecordSummary(source typ.Type) bool {
	if source == nil || !proof.OptionalTypeHasConcreteValue(source) {
		return false
	}
	_, record := unwrap.Alias(subst.ExpandInstantiated(proof.ProjectionWithoutNil(source))).(*typ.Record)
	return record
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
	var metatableFacts []equation.Fact
	var argumentFacts []equation.Fact
	var assertionFacts []equation.Fact
	// captureInvalidations revokes the caller proofs that a callee this
	// application does not evaluate would otherwise keep refuting. They publish
	// with the call's other completed outputs so the revocation shares its epoch.
	var captureInvalidations []equation.Fact
	defer func() {
		if err == nil && result.Complete {
			result.Closure.Values = append(result.Closure.Values, placementFacts...)
			result.Closure.Values = append(result.Closure.Values, metatableFacts...)
			result.Closure.Values = append(result.Closure.Values, argumentFacts...)
			result.Closure.Values = append(result.Closure.Values, assertionFacts...)
			result.Closure.Values = append(result.Closure.Values, captureInvalidations...)
		}
	}()
	if closure, recognized, freezeErr := freezeCallEpoch(operation, partition); freezeErr != nil {
		return equation.TransactionResult{}, freezeErr
	} else if recognized {
		// Freeze has its own epoch ordering: retain that established kernel
		// boundary, and publish the independently closed placement event from
		// the same sealed call operands before returning.
		if guardsHold(operation.Guards, partition) {
			operands, operandErr := callOperands(operation.Operands)
			if operandErr != nil {
				return equation.TransactionResult{}, operandErr
			}
			closure.Values = append(closure.Values, placementApplyFacts(operation, operands, partition)...)
		}
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
	assertionFacts = assertedPathNarrowingFacts(operation, operands, partition)
	argumentFacts = callArgumentFacts(operation.Target.Name, operands.arguments)
	placementFacts = placementApplyFacts(operation, operands, partition)
	placementFacts = append(placementFacts, placementSuspensionFacts(operation, operands, partition)...)
	if operands.display == "setmetatable" && !operands.spread && len(operands.arguments) == 2 {
		if object, found := tableIdentityForTerm(operands.arguments[0], partition); found {
			metatableFacts = append(metatableFacts, heapMetaAttachedFact(object, operation.Target.Name))
			if metatable, found := tableIdentityForTerm(operands.arguments[1], partition); found {
				if target, found := heapMemberCurrent(heapMemberIdentityPrefix, metatable, ".__newindex", partition); found {
					metatableFacts = append(metatableFacts, heapMetaNewIndexFact(object, operation.Target.Name, target))
				}
			}
		}
	}
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
	if lexical != nil && localCallable {
		placementFacts = append(placementFacts, placementInvokedClosureCaptureFacts(operation, operands, handle, partition)...)
		placementFacts = append(placementFacts, placementClosedLocalSummaryFacts(lexical, operation, operands, handle, partition)...)
		if result, refuted := lexical.boundaryArgumentRefutation(operation, operands, handle, partition); refuted {
			return result, nil
		}
		if result, refuted := lexical.boundaryArithmeticOperandRefutation(operation, operands, handle, partition); refuted {
			return result, nil
		}
	}
	applyLocal := func() (equation.TransactionResult, bool, error) {
		recursiveDemand := lexical != nil && lexical.closureDemandRecurses(handle, partition)
		child, childKnown := front.Compilation{}, false
		if lexical != nil {
			child, childKnown = lexical.byPrototype[handle.Prototype]
		}
		// A declared one-slot, acyclic child result is the only table return that
		// can be projected without losing tuple order or crossing a recursive
		// summary boundary. Its sealed allocation is the existing publication; a
		// declaration alone remains insufficient.
		tableProjectionUnsafe := !childKnown || !hasProjectableTableResult(child)
		if lexical == nil || !localCallable || (operands.resultArity == 0 && !lexical.requiresBody[handle.Prototype]) ||
			(operands.resultArity != 0 && !lexical.requiresBody[handle.Prototype] &&
				((lexical.hasClaim(handle.Prototype) && !lexical.hasRuntimeCastClaim(handle.Prototype)) || lexical.hasTableAllocation(handle.Prototype) && tableProjectionUnsafe && !recursiveDemand) && !childHasSelect(child)) {
			if localCallable {
				captureInvalidations = lexical.capturedMemberWriteInvalidations(handle, operation.Target.Name, partition)
			}
			if lexical != nil && childKnown {
				if declared, published, declaredErr := lexical.declaredBranchAssignmentDiagnostics(child); declaredErr != nil {
					return equation.TransactionResult{}, false, declaredErr
				} else if declared {
					return equation.TransactionResult{Complete: true, Closure: published}, true, nil
				}
			}
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
			captureInvalidations = lexical.capturedMemberWriteInvalidations(handle, operation.Target.Name, partition)
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
	typedMethodSignature := callableShape{}
	typedMethodContract := false
	if hasCallee && (!known || isUnknownScalar(callee)) {
		// A static member read is a derived use of an already-published declared
		// receiver, rather than a direct value cell. Preserve the ordinary
		// fail-closed call lookup, but recognize the one concrete outcome that
		// cannot become callable: the closed receiver has no such member.
		if resolved, resolveErr := resolveCurrentValue(operands.callee, partition); resolveErr == nil && memberMissing(resolved) {
			callee, known = resolved, true
		}
	}
	if hasCallee && known && memberMissing(callee) {
		return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
			Key:   "type.member.missing/" + operation.Target.Name,
			Value: []byte(memberMissingMessage(operands.display, callee)),
		}}}}, nil
	}
	if hasCallee && (!known || isUnknownScalar(callee)) {
		if receiver, method, static := staticMemberCall(operands.callee); static {
			if signature, available := typedMethodCallableSignature(receiver, method, partition); available && callArgumentsNeedPublishedContract(operands.arguments, partition) {
				typedMethodSignature, typedMethodContract = signature, true
			}
		}
	}
	parameterOffset := 0
	if !hasCallee {
		receiver, receiverKnown := resolveKnownCurrentValue(operands.receiver, partition)
		if !receiverKnown {
			return equation.TransactionResult{Complete: true}, nil
		}
		if (strings.HasPrefix(string(operands.receiver), "temp/") || len(operation.Guards) == 0) && optionalMethodReceiverAtCall(operands.receiver, receiver, operands.method, partition) {
			return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
				Key:   "type.call.optional_receiver/" + operation.Target.Name,
				Value: []byte(fmt.Sprintf("cannot call method on an optional value without a nil check: %s may be nil", operands.display)),
			}}}}, nil
		}
		if payload, channel := typedChannelPayload(operands.receiver, partition); channel && operands.method == "send" && !operands.spread && len(operands.arguments) == 1 {
			if argument, available := resolveKnownCurrentValue(operands.arguments[0], partition); available {
				if subject, actual, expected, mismatch := firstChannelPayloadMismatch(argument, payload, "argument-00000000"); mismatch {
					return callDiagnostic(operation, "argument_type", subject, fmt.Sprintf("%s is %s, not %s", channelPayloadDisplay(subject), actual, expected)), nil
				}
			}
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
			// A typed receiver's member contract is a published value witness,
			// even when the receiver is an opaque interface rather than a sealed
			// table.  Reuse that canonical member contract for argument checking;
			// do not materialize a member value or inspect a provider body.
			if signature, available := typedMethodCallableSignature(operands.receiver, operands.method, partition); available && callArgumentsNeedPublishedContract(operands.arguments, partition) {
				typedMethodSignature, typedMethodContract = signature, true
			} else {
				if receiverType, declared := declaredTypeForTerm(operands.receiver, partition); declared && declaredMethodMissing(receiverType, operands.method) {
					if missing, encoded := memberMissingValue(receiverType); encoded {
						return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Diagnostics: []equation.Fact{{
							Key:   "type.member.missing/" + operation.Target.Name,
							Value: []byte(memberMissingMessage(operands.display, missing)),
						}}}}, nil
					}
				}
				return equation.TransactionResult{Complete: true}, nil
			}
		}
	}
	if !known && !typedMethodContract {
		if outcome, projected, err := applyLocal(); err != nil {
			return equation.TransactionResult{}, err
		} else if projected {
			return outcome, nil
		}
		return equation.TransactionResult{Complete: true}, nil
	}
	if hasCallee && !typedMethodContract && optionalCallableValue(callee) {
		return callDiagnostic(operation, "not_callable", "callee", fmt.Sprintf("cannot call %s because it may be nil", operands.display)), nil
	}
	if !typedMethodContract && !isUnknownScalar(callee) && !isCallableValue(callee) {
		return callDiagnostic(operation, "not_callable", "callee", fmt.Sprintf("%s is %s, not callable", operands.display, callDisplayValue(callee))), nil
	}
	signature, signatureKnown := callableSignature(callee)
	if typedMethodContract {
		signature, signatureKnown = typedMethodSignature, true
	}
	if !signatureKnown || operands.spread {
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
			parameterOffset = 1
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
		expected, accepts := callableParameterAt(signature, index)
		if !accepts {
			break
		}
		// A method contract from a published receiver keeps its canonical
		// parameter type even when the selected argument's runtime value is
		// intentionally Top.  Reuse that existing pair of publications to
		// reject only the proven nilability conflict; neither an annotation nor
		// an unknown scalar is treated as evidence for this boundary.
		if callableParameterRejectsNil(expected) && ((!hasCallee && optionalArgumentMayBeNil(term, partition)) ||
			(declaredEntryBoundary(operation.Target.Body, partition) && optionalProviderArgumentMayBeNil(term, partition))) {
			return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d may be nil, not %s", index+1, callableParameterType(expected))), nil
		}
		argument, known := resolveKnownCurrentValue(term, partition)
		// An explicit any is a published precision boundary, not a proof of a
		// typed parameter contract.  The argument may retain a concrete shape
		// from its allocation, but that shape crossed the boundary without
		// validation and cannot discharge this call's declared requirement.
		if known && (isExplicitAnyValue(argument) || sourceHasAnyBoundary(term, partition.Values()) || declaredExplicitAny(term, partition)) && callableParameterRequiresProof(expected) {
			expectedType := callableParameterType(expected)
			return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is any, not %s", index+1, expectedType)), nil
		}
		if known && genericConstraintRefuted(argument, expected) {
			return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValueForTerm(term, argument, partition), strings.TrimSpace(strings.SplitN(expected, ":", 2)[1]))), nil
		}
		if !known || numericForInductionSatisfies(term, argument, expected, partition) {
			continue
		}
		if !provenValueNotSubtype(argument, expected) {
			if contract, published := callableParameterContract(signature, index+parameterOffset); published && !genericCallableSignature(signature) &&
				!refinement.ContainsFreeTypeParam(contract) && structuralArgumentWitness(argument) && valueAgainstType(argument, contract) == shapeRefuted {
				return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValueForTerm(term, argument, partition), typeformat.Short(contract))), nil
			}
			continue
		}
		return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValueForTerm(term, argument, partition), expected)), nil
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

// staticMemberCall recovers the receiver and member only from the front's
// canonical single-segment direct-callee path. Dynamic and nested paths have
// no equivalent member authority and therefore remain unavailable.
func staticMemberCall(callee []byte) (receiver []byte, method string, ok bool) {
	root, suffix, member := tableAddress(callee)
	segments, static := segment.ParseFormattedSegments(suffix)
	if !member || !static || len(segments) != 1 || segments[0].Kind != segment.SegmentField {
		return nil, "", false
	}
	return root, segments[0].Name, true
}

func callArgumentsNeedPublishedContract(arguments [][]byte, partition equation.Partition) bool {
	if hasSummaryAnyArgument(arguments, partition.Values()) || hasPublishedOptionalArgument(arguments, partition) {
		return true
	}
	for _, argument := range arguments {
		if sourceHasAnyBoundary(argument, partition.Values()) {
			return true
		}
	}
	return false
}

func typeRequiresBoundaryProof(value typ.Type) bool {
	if value == nil {
		return false
	}
	switch unwrap.Alias(subst.ExpandInstantiated(value)).Kind() {
	case kind.Any, kind.Unknown:
		return false
	default:
		return true
	}
}

// boundaryArgumentRefutation compares a known caller value with the closed
// declared type at a local function boundary.  It consumes the same sealed
// function witness used by assignment claims; a body may be ineligible for
// result projection, but that must not erase a directly refuted parameter
// contract.  Unknown values and malformed/inexact boundaries remain silent.
func (l *lexicalEvaluator) boundaryArgumentRefutation(operation equation.BoundEquation, operands directCallOperands, handle closureHandle, partition equation.Partition) (equation.TransactionResult, bool) {
	child, exists := l.byPrototype[handle.Prototype]
	if !exists || child.WIR == nil || operands.spread {
		return equation.TransactionResult{}, false
	}
	arguments := operands.arguments
	if operands.receiver != nil {
		arguments = append([][]byte{operands.receiver}, arguments...)
	}
	if len(arguments) != len(child.Boundary.Parameters) {
		return equation.TransactionResult{}, false
	}
	for index, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type == 0 {
			continue
		}
		argument, known := resolveKnownCurrentValue(arguments[index], partition)
		if !known || isUnknownScalar(argument) || (!declaredExplicitAny(arguments[index], partition) && !l.memberRelayEntry(operation, operands, partition)) {
			continue
		}
		expected := child.WIR.Type(parameter.Type)
		if valueAgainstType(argument, expected) != shapeRefuted {
			continue
		}
		return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not %s", index+1, callDisplayValueForTerm(arguments[index], argument, partition), typeformat.Short(expected))), true
	}
	return equation.TransactionResult{}, false
}

// boundaryArithmeticOperandRefutation carries a local body's direct numeric
// operand obligation back to its exact caller. An unannotated formal normally
// has no boundary contract, but an already-lowered binary expression with a
// proven numeric counterpart is an executable requirement of that formal.
// The check remains fail-closed: it accepts only a known, statically refuted
// argument at a complete lexical boundary and never infers a type from an
// opaque expression or unresolved capture.
func (l *lexicalEvaluator) boundaryArithmeticOperandRefutation(operation equation.BoundEquation, operands directCallOperands, handle closureHandle, partition equation.Partition) (equation.TransactionResult, bool) {
	child, exists := l.byPrototype[handle.Prototype]
	if !exists || child.WIR == nil || operands.spread {
		return equation.TransactionResult{}, false
	}
	arguments := operands.arguments
	if operands.receiver != nil {
		arguments = append([][]byte{operands.receiver}, arguments...)
	}
	if len(arguments) != len(child.Boundary.Parameters) {
		return equation.TransactionResult{}, false
	}
	for index, parameter := range child.Boundary.Parameters {
		if parameter.Vararg || parameter.Type != 0 {
			continue
		}
		argument, known := resolveKnownCurrentValue(arguments[index], partition)
		if !known || isUnknownScalar(argument) || !arithmeticOperandRefuted(argument) {
			continue
		}
		formal := []byte(boundaryTerm(parameter.Symbol))
		for _, expression := range child.Artifact.Equations {
			if expression.Occurrence.Kind != "expression" {
				continue
			}
			expressionKind, found := artifactOperand(expression.Operands, "kind")
			if !found || string(expressionKind) != strconv.Itoa(int(wir.OpBinOp)) {
				continue
			}
			operator, found := artifactOperand(expression.Operands, "operator")
			if !found {
				continue
			}
			operatorID, err := strconv.Atoi(string(operator))
			if err != nil {
				continue
			}
			operatorText, supported := expressionOperatorText(wir.Operator(operatorID))
			if !supported {
				continue
			}
			left, leftFound := artifactOperand(expression.Operands, "left")
			right, rightFound := artifactOperand(expression.Operands, "right")
			if !leftFound || !rightFound {
				continue
			}
			counterpart := right
			if bytes.Equal(right, formal) {
				counterpart = left
			} else if !bytes.Equal(left, formal) {
				continue
			}
			counterpartValue, resolveErr := resolveCurrentValue(counterpart, partition)
			if resolveErr != nil || isUnknownScalar(counterpartValue) {
				continue
			}
			counterpartType, typed := expressionValueType(counterpartValue)
			if !typed {
				continue
			}
			result, numeric := typeoperator.BinaryOp(typ.Number, operatorText, counterpartType)
			if !numeric || result == nil || (result.Kind() != kind.Number && result.Kind() != kind.Integer) {
				continue
			}
			return callDiagnostic(operation, "argument_type", indexedCallSubject("argument", index), fmt.Sprintf("argument %d is %s, not number", index+1, callDisplayValue(argument))), true
		}
	}
	return equation.TransactionResult{}, false
}

func arithmeticOperandRefuted(value []byte) bool {
	// A closed table is an existing runtime-kind publication. Unlike an open
	// table annotation, it conclusively cannot satisfy Lua's numeric operand
	// requirement even when the general structural comparator has no scalar
	// relation for tables.
	return shapefact.IsTable(value) || valueAgainstType(value, typ.Number) == shapeRefuted
}

// memberRelayEntry proves that this is the narrowly admitted member-summary
// path: the current body has the relay template and the callable member was
// seeded at this exact child entry.  It prevents ordinary local calls from
// gaining a new boundary diagnostic merely because their values happen to be
// closed.
func (l *lexicalEvaluator) memberRelayEntry(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) bool {
	if l == nil || operands.resultArity != 0 {
		return false
	}
	compilation, found := l.byBody[operation.Target.Body]
	if !found || !compilationRequiresDiagnosticPublication(compilation) {
		return false
	}
	receiver, method := []byte(nil), ""
	if operands.callee != nil {
		cut := strings.LastIndex(string(operands.callee), ".")
		if cut <= len("path/") {
			return false
		}
		receiver, method = operands.callee[:cut], string(operands.callee[cut+1:])
	} else {
		receiver, method = operands.receiver, operands.method
	}
	if len(receiver) == 0 || method == "" {
		return false
	}
	prefix := "member-closure/" + string(receiver) + "/entry/"
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		var wire memberClosureWire
		if json.Unmarshal(fact.Value, &wire) == nil && wire.Suffix == "."+method && validClosureHandle(wire.Handle) {
			return true
		}
	}
	return false
}

// declaredExplicitAny reads only the descriptive entry fact published from a
// front declaration. Unlike a shape or inferred value, it identifies a
// deliberate precision boundary and never manufactures compatibility proof.
func declaredExplicitAny(term []byte, partition equation.Partition) bool {
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return false
	}
	boundaryPrefix := "explicit-any/" + string(term) + "/"
	prefix := "declared-type/" + string(term) + "/"
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, boundaryPrefix) && string(fact.Value) == "declared" {
			return true
		}
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		declared, ok := shapefact.DecodeTarget(fact.Value)
		if ok && declared != nil && unwrap.Alias(subst.ExpandInstantiated(declared)).Kind() == kind.Any {
			return true
		}
	}
	return false
}

func explicitAnyBoundaryFact(term, operation string) equation.Fact {
	return equation.Fact{Key: "explicit-any/" + term + "/" + operation, Value: []byte("declared")}
}

func genericCallableSignature(signature callableShape) bool {
	for _, parameter := range signature.Params {
		if len(parameter) > 0 && parameter[0] >= 'A' && parameter[0] <= 'Z' && (len(parameter) == 1 || strings.HasPrefix(parameter[1:], " :")) {
			return true
		}
	}
	return false
}

// callableParameterRequiresProof recognizes only a concrete declared
// parameter type.  Unconstrained generic and top-like parameters impose no
// boundary-validation obligation, so they retain the existing call behavior.
func callableParameterRequiresProof(parameter string) bool {
	expected := callableParameterType(parameter)
	return expected != "" && expected != "any" && expected != "unknown"
}

func callableParameterType(parameter string) string {
	if _, expected, found := strings.Cut(parameter, ":"); found {
		return strings.TrimSpace(expected)
	}
	return strings.TrimSpace(parameter)
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
	fields, record := closedRecordConstraintFields(constraint)
	if !record {
		return false
	}
	table, tableValue := shapefact.DecodeTable(value)
	if !tableValue {
		return true
	}
	// A sealed literal records both present fields and proved absences.  Only
	// that existing shape witness can refute a record constraint; an open table
	// remains unclassified rather than being rejected from its source spelling.
	if !table.Closed {
		return false
	}
	for _, field := range fields {
		member, found := table.Lookup("." + field)
		if !found || !member.Present {
			return true
		}
	}
	return false
}

// closedRecordConstraintFields accepts the finite record surface emitted in a
// callable type-parameter constraint.  It returns names only after validating
// every field declaration, so malformed or open constraint text cannot become
// a structural rejection authority.
func closedRecordConstraintFields(constraint string) ([]string, bool) {
	if len(constraint) < 3 || constraint[0] != '{' || constraint[len(constraint)-1] != '}' {
		return nil, false
	}
	items := strings.Split(strings.TrimSpace(constraint[1:len(constraint)-1]), ",")
	fields := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		name, fieldType, found := strings.Cut(strings.TrimSpace(item), ":")
		name, fieldType = strings.TrimSpace(name), strings.TrimSpace(fieldType)
		if !found || name == "" || fieldType == "" || seen[name] {
			return nil, false
		}
		for index, runeValue := range name {
			if !(runeValue == '_' || runeValue >= 'a' && runeValue <= 'z' || runeValue >= 'A' && runeValue <= 'Z' || index != 0 && runeValue >= '0' && runeValue <= '9') {
				return nil, false
			}
		}
		seen[name] = true
		fields = append(fields, name)
	}
	return fields, len(fields) != 0
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
	values := []equation.Fact{heapMemberFact(identity, suffix, operation.Target.Name, value)}
	// The heap cell is the identity-backed mutation authority.  Its matching
	// source-path cell is needed when this closed table subsequently crosses a
	// lexical return boundary: heapMemberSurface deliberately rebuilds a
	// returned literal only from member paths published by the producing body.
	// Keep the bridge exact -- a dynamic receiver has no canonical source cell
	// to publish, even when its current heap identity happens to be known.
	memberTerm := append(append([]byte(nil), operands.arguments[0]...), []byte(suffix)...)
	if _, _, exact := heapTableAddress(memberTerm); exact {
		values = append(values,
			equation.Fact{Key: "value/" + string(memberTerm) + "/" + operation.Target.Name, Value: append([]byte(nil), value...)},
			equation.Fact{Key: epochFactPrefix + string(memberTerm) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
		)
	}
	if memberIdentity, found := tableIdentityForTerm(operands.arguments[1], partition); found {
		values = append(values, heapMemberIdentityFact(identity, suffix, operation.Target.Name, memberIdentity))
	}
	return equation.OutputClosure{Values: values}, true
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
	return strings.HasPrefix(string(value), "scalar/channel/op-") || strings.HasPrefix(string(value), "scalar/channel-entry/") || strings.HasPrefix(string(value), "scalar/channel-summary/")
}

// resolveCurrentIdentity uses the same operation-key epoch discipline as
// normal value reads.  A symbolic entry identity is intentionally accepted
// only when it was published by entryKernel for an exact Channel<T> root.
func resolveCurrentIdentity(term []byte, partition equation.Partition) ([]byte, bool) {
	if identity, ok := currentEpochFact("identity/", term, partition); ok {
		return identity, true
	}
	value, known := resolveKnownCurrentValue(term, partition)
	if known && isChannelIdentity(value) {
		return value, true
	}
	if _, known := currentChannelPayloadFact(term, partition); known {
		return []byte("scalar/channel-summary/" + base64.RawURLEncoding.EncodeToString(term)), true
	}
	// A typed module summary can prove that a derived field is a Channel<T>,
	// but it has no heap allocation fact in this local partition. Preserve a
	// stable identity witness tied to that already-published path so a select
	// can compose its payload summary without inventing a channel for an
	// untyped or unresolved member.
	if _, channelOK := typedChannelPayload(term, partition); !channelOK {
		return nil, false
	}
	return []byte("scalar/channel-summary/" + base64.RawURLEncoding.EncodeToString(term)), true
}

// typedChannelPayload keeps an instantiated Channel<T> intact while walking
// a summary record. shapefact's value encoding deliberately erases generic
// arguments from a runtime-shaped interface, so this projection starts from
// the closed ancestor type instead of decoding the projected value again.
func typedChannelPayload(term []byte, partition equation.Partition) (typ.Type, bool) {
	if encoded, ok := currentChannelPayloadFact(term, partition); ok {
		payload, decoded := shapefact.DecodeTarget(encoded)
		return payload, decoded && payload != nil
	}
	if encoded, ok := currentEpochFact("type/", term, partition); ok {
		if channel, decoded := shapefact.DecodeTarget(encoded); decoded {
			if payload, ok := ambient.ChannelPayloadType(channel); ok && payload != nil {
				return payload, true
			}
		}
	}
	channel, ok := typedPathType(term, partition)
	if !ok || channel == nil {
		return nil, false
	}
	payload, ok := ambient.ChannelPayloadType(channel)
	return payload, ok && payload != nil
}

// currentChannelPayloadFact reads a payload witness at the current epoch of
// its owning path. Channel payloads belong to a static descendant while the
// ordinary epoch belongs to the root value, so requiring both coordinates
// keeps a superseded wrapper result from contributing a stale select arm.
func currentChannelPayloadFact(term []byte, partition equation.Partition) ([]byte, bool) {
	if encoded, ok := currentEpochFact(channelPayloadPrefix, term, partition); ok {
		return encoded, true
	}
	path := strings.TrimPrefix(string(term), "path/")
	if path == string(term) {
		return nil, false
	}
	for cut := len(path); cut > 0; {
		cut = strings.LastIndexAny(path[:cut], ".[")
		if cut < 0 {
			return nil, false
		}
		root, suffix := path[:cut], path[cut:]
		if !segment.ValidFormattedSegments(suffix) {
			return nil, false
		}
		rootTerm := []byte("path/" + root)
		operation, current := currentEpoch(rootTerm, partition)
		if !current {
			continue
		}
		if fact, found := partition.Value(channelPayloadPrefix + string(rootTerm) + suffix + "/" + operation); found {
			return fact.Value, true
		}
	}
	return nil, false
}

// firstChannelPayloadMismatch walks only a sealed argument shape against the
// declared channel payload. Unknown or open values remain unavailable rather
// than becoming a speculative send diagnostic.
func firstChannelPayloadMismatch(value []byte, target typ.Type, subject string) (string, string, string, bool) {
	target = unwrap.Alias(subst.ExpandInstantiated(target))
	if target == nil || isUnknownScalar(value) {
		return "", "", "", false
	}
	if union, ok := target.(*typ.Union); ok {
		for _, member := range union.Members {
			if valueAgainstType(value, member) == shapeProven {
				return "", "", "", false
			}
		}
		if valueAgainstType(value, target) == shapeRefuted {
			return subject, assignmentEvidenceValue(value), typeformat.Short(target), true
		}
		return "", "", "", false
	}
	if record, ok := target.(*typ.Record); ok {
		table, sealed := shapefact.DecodeTable(value)
		if !sealed || !table.Closed {
			return "", "", "", false
		}
		for _, field := range record.Fields {
			member, found := table.Lookup("." + field.Name)
			if !found || !member.Present {
				if field.Optional {
					continue
				}
				return subject + "." + field.Name, "nil", typeformat.Short(field.Type), true
			}
			if nested, actual, expected, mismatch := firstChannelPayloadMismatch([]byte(member.Value), field.Type, subject+"."+field.Name); mismatch {
				return nested, actual, expected, true
			}
		}
		return "", "", "", false
	}
	if valueAgainstType(value, target) == shapeRefuted {
		return subject, assignmentEvidenceValue(value), typeformat.Short(target), true
	}
	return "", "", "", false
}

func channelPayloadDisplay(subject string) string {
	base, suffix, found := strings.Cut(subject, ".")
	if !found {
		return strings.Replace(base, "argument-00000000", "argument 1", 1)
	}
	return strings.Replace(base, "argument-00000000", "argument 1", 1) + "." + suffix
}

func typedPathType(term []byte, partition equation.Partition) (typ.Type, bool) {
	_, suffix, source, ok := typedAncestor(term, partition)
	if !ok || len(suffix) == 0 || source == nil {
		return nil, false
	}
	projected, ok := luatypeprojection.ApplySegments(source, suffix)
	return projected, ok && projected != nil
}

func declaredTypeForTerm(term []byte, partition equation.Partition) (typ.Type, bool) {
	var encoded []byte
	latest := ""
	for _, fact := range partition.Values() {
		if (strings.HasPrefix(fact.Key, "type/"+string(term)+"/") || strings.HasPrefix(fact.Key, "declared-type/"+string(term)+"/")) && fact.Key > latest {
			encoded, latest = fact.Value, fact.Key
		}
	}
	if len(encoded) == 0 {
		return nil, false
	}
	return shapefact.DecodeTarget(encoded)
}

func channelPayloadSummaryFacts(root, operation string, value typ.Type) []equation.Fact {
	var facts []equation.Fact
	var walk func(typ.Type, []segment.Segment)
	walk = func(current typ.Type, suffix []segment.Segment) {
		if current == nil {
			return
		}
		if payload, ok := ambient.ChannelPayloadType(current); ok && payload != nil {
			if encoded, ok := shapefact.EncodeTarget(payload); ok {
				facts = append(facts, equation.Fact{Key: channelPayloadPrefix + root + segment.FormatSegments(suffix) + "/" + operation, Value: encoded})
			}
			return
		}
		current = unwrap.Annotations(current)
		if alias, ok := current.(*typ.Alias); ok && alias != nil {
			walk(alias.UnaliasedTarget(), suffix)
			return
		}
		record, ok := current.(*typ.Record)
		if !ok || record == nil {
			return
		}
		for _, field := range record.Fields {
			walk(field.Type, append(append([]segment.Segment(nil), suffix...), segment.Segment{Kind: segment.SegmentField, Name: field.Name}))
		}
	}
	walk(value, nil)
	return facts
}

// channelPayloadRelationFacts carries a Channel<T> payload only through an
// already-published finite import return relation. Each emitted path comes
// from the relation's exact member suffix and each payload comes from the
// corresponding call argument's current type publication; unresolved leaves
// simply contribute no fact.
func channelPayloadRelationFacts(template exportrelation.Value, result, operation string, arguments map[int][]byte, partition equation.Partition) []equation.Fact {
	if result == "" || operation == "" || !strings.HasPrefix(result, "temp/") {
		return nil
	}
	facts := make([]equation.Fact, 0)
	var walk func(exportrelation.Value, string)
	walk = func(value exportrelation.Value, suffix string) {
		if value.Parameter != nil {
			argument, found := arguments[*value.Parameter]
			if !found {
				return
			}
			payload, found := typedChannelPayload(argument, partition)
			if !found || payload == nil {
				return
			}
			encoded, ok := shapefact.EncodeTarget(payload)
			if !ok {
				return
			}
			facts = append(facts, equation.Fact{Key: channelPayloadPrefix + result + suffix + "/" + operation, Value: encoded})
			return
		}
		for _, member := range value.Table {
			if !segment.ValidFormattedSegments(member.Suffix) {
				return
			}
			walk(member.Value, suffix+member.Suffix)
		}
	}
	walk(template, "")
	return facts
}

func rebaseChannelPayloadFacts(source []byte, target, operation string, partition equation.Partition) []equation.Fact {
	prefix := channelPayloadPrefix + string(source)
	var facts []equation.Fact
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(fact.Key, prefix)
		cut := strings.LastIndexByte(rest, '/')
		if cut < 0 {
			continue
		}
		facts = append(facts, equation.Fact{Key: channelPayloadPrefix + target + rest[:cut] + "/" + operation, Value: append([]byte(nil), fact.Value...)})
	}
	return facts
}

func currentEpochFact(prefix string, term []byte, partition equation.Partition) ([]byte, bool) {
	operation, ok := currentEpoch(term, partition)
	if !ok {
		return nil, false
	}
	key := prefix + string(term) + "/" + operation
	if fact, found := partition.Value(key); found {
		return fact.Value, true
	}
	return nil, false
}

// currentEpoch is the sole current-version lookup for a path.  Consumers that
// publish guarded predicate facts must compare against this existing
// publication rather than retaining evaluator-local mutation state.
func currentEpoch(term []byte, partition equation.Partition) (string, bool) {
	epochPrefix := epochFactPrefix + string(term) + "/"
	latest, found := partition.LatestValuePrefix(epochPrefix)
	if !found {
		return "", false
	}
	return strings.TrimPrefix(latest.Key, epochPrefix), true
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

func operationOperand(operands []equation.BoundOperand, role string) ([]byte, bool) {
	for _, operand := range operands {
		if operand.Role == role {
			return append([]byte(nil), operand.Value...), true
		}
	}
	return nil, false
}

func heapClosedFact(identity []byte, operation string) equation.Fact {
	return equation.Fact{Key: heapTableClosedPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation, Value: []byte("closed")}
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

func memberCellFactKey(identity []byte, suffix, operation string) string {
	return heapFactKey(memberCellPrefix, identity, suffix, operation)
}

func memberCellFact(identity []byte, suffix, operation string, value []byte, partition equation.Partition) (equation.Fact, bool, error) {
	return memberCellFactWithSource(identity, suffix, operation, value, value, nil, partition)
}

func memberCellFactWithIdentity(identity []byte, suffix, operation string, value, memberIdentity []byte, partition equation.Partition) (equation.Fact, bool, error) {
	return memberCellFactWithSource(identity, suffix, operation, value, value, memberIdentity, partition)
}

func memberCellFactWithSource(identity []byte, suffix, operation string, value, source, memberIdentity []byte, partition equation.Partition) (equation.Fact, bool, error) {
	if len(identity) == 0 || suffix == "" || !segment.ValidFormattedSegments(suffix) || len(value) == 0 {
		return equation.Fact{}, false, nil
	}
	cell := memberCellWire{Value: append([]byte(nil), value...)}
	if handle, found := closureHandleFor(source, partition); found {
		cell.Handle = &handle
	}
	if len(memberIdentity) != 0 {
		cell.MemberIdentity = append([]byte(nil), memberIdentity...)
	} else if resolvedIdentity, found := tableIdentityForTerm(source, partition); found {
		cell.MemberIdentity = resolvedIdentity
	}
	encoded, err := json.Marshal(cell)
	if err != nil {
		return equation.Fact{}, false, err
	}
	return equation.Fact{Key: memberCellFactKey(identity, suffix, operation), Value: encoded}, true, nil
}

func currentMemberCell(identity []byte, suffix string, partition equation.Partition) (memberCellWire, bool) {
	prefix := memberCellPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix)) + "/"
	var encoded []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (encoded == nil || fact.Key > latest) {
			encoded, latest = fact.Value, fact.Key
		}
	}
	if encoded == nil {
		return memberCellWire{}, false
	}
	var cell memberCellWire
	if json.Unmarshal(encoded, &cell) != nil || len(cell.Value) == 0 || (cell.Handle != nil && !validClosureHandle(*cell.Handle)) {
		return memberCellWire{}, false
	}
	return cell, true
}

// memberCellSeedsForEntry follows only identities already reachable from an
// exact entry seed.  It is intentionally a closed heap snapshot: no source
// path or declared table type is converted into a callable capability.
func memberCellSeedsForEntry(seeds []entrySeed, partition equation.Partition) []entryMemberCellSeed {
	queue := make([][]byte, 0, len(seeds))
	seenIdentity := make(map[string]bool)
	for _, seed := range seeds {
		if identity, found := tableIdentityForTerm([]byte(seed.Term), partition); found {
			queue = append(queue, identity)
		}
	}
	byCell := make(map[string]entryMemberCellSeed)
	for len(queue) != 0 {
		identity := queue[0]
		queue = queue[1:]
		if seenIdentity[string(identity)] {
			continue
		}
		seenIdentity[string(identity)] = true
		prefix := memberCellPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
		for _, fact := range partition.Values() {
			if !strings.HasPrefix(fact.Key, prefix) {
				continue
			}
			rest := strings.TrimPrefix(fact.Key, prefix)
			cut := strings.LastIndexByte(rest, '/')
			if cut <= 0 || cut == len(rest)-1 {
				continue
			}
			suffixBytes, err := base64.RawURLEncoding.DecodeString(rest[:cut])
			if err != nil || !segment.ValidFormattedSegments(string(suffixBytes)) {
				continue
			}
			cell, found := currentMemberCell(identity, string(suffixBytes), partition)
			if !found {
				continue
			}
			key := string(identity) + "\x00" + string(suffixBytes)
			byCell[key] = entryMemberCellSeed{Identity: append([]byte(nil), identity...), Suffix: string(suffixBytes), Wire: cell}
			if len(cell.MemberIdentity) != 0 {
				queue = append(queue, cell.MemberIdentity)
			}
		}
	}
	keys := make([]string, 0, len(byCell))
	for key := range byCell {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]entryMemberCellSeed, 0, len(keys))
	for _, key := range keys {
		out = append(out, byCell[key])
	}
	return out
}

func tableIdentitySeedsForEntry(seeds []entrySeed, partition equation.Partition) []entryTableIdentitySeed {
	byTerm := make(map[string]entryTableIdentitySeed, len(seeds))
	for _, seed := range seeds {
		if identity, found := tableIdentityForTerm([]byte(seed.Term), partition); found {
			byTerm[seed.Term] = entryTableIdentitySeed{Term: seed.Term, Identity: append([]byte(nil), identity...)}
		}
	}
	terms := make([]string, 0, len(byTerm))
	for term := range byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	out := make([]entryTableIdentitySeed, 0, len(terms))
	for _, term := range terms {
		out = append(out, byTerm[term])
	}
	return out
}

func placementSeedsForEntry(seeds []entrySeed, partition equation.Partition) []entryPlacementSeed {
	byTerm := make(map[string]entryPlacementSeed, len(seeds))
	for _, seed := range seeds {
		allocation, found := placementAllocationForTerm([]byte(seed.Term), partition)
		if !found || !allocation.Complete || allocation.Identity == "" || allocation.Result == "" || allocation.Kind == "" {
			continue
		}
		byTerm[seed.Term] = entryPlacementSeed{Term: seed.Term, Allocation: allocation}
	}
	terms := make([]string, 0, len(byTerm))
	for term := range byTerm {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	out := make([]entryPlacementSeed, 0, len(terms))
	for _, term := range terms {
		out = append(out, byTerm[term])
	}
	return out
}

func materializedMemberOrigin(value []byte) (string, []byte, bool) {
	const prefix = "member/"
	rest := strings.TrimPrefix(string(value), prefix)
	if rest == string(value) {
		return "", nil, false
	}
	for _, marker := range []string{"/path/", "/temp/"} {
		index := strings.Index(rest, marker)
		if index <= 0 {
			continue
		}
		suffix, source := rest[:index], []byte(rest[index+1:])
		if !segment.ValidFormattedSegments(suffix) {
			return "", nil, false
		}
		return suffix, source, true
	}
	return "", nil, false
}

func heapMemberOriginFact(term, suffix, operation string, source []byte) equation.Fact {
	return equation.Fact{Key: heapMemberOriginPrefix + term + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix)) + "/" + operation, Value: append([]byte(nil), source...)}
}

func heapMemberOriginCurrent(term []byte, suffix string, partition equation.Partition) ([]byte, bool) {
	prefix := heapMemberOriginPrefix + string(term) + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix)) + "/"
	var value []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latest) {
			value, latest = fact.Value, fact.Key
		}
	}
	return append([]byte(nil), value...), value != nil
}

func heapMetaNewIndexFact(identity []byte, operation string, target []byte) equation.Fact {
	return equation.Fact{Key: heapMetaNewIndexPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation, Value: append([]byte(nil), target...)}
}

func heapMetaAttachedFact(identity []byte, operation string) equation.Fact {
	return equation.Fact{Key: heapMetaAttachedPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation, Value: []byte("attached")}
}

func heapMetaAttached(identity []byte, partition equation.Partition) bool {
	prefix := heapMetaAttachedPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "attached" {
			return true
		}
	}
	return false
}

// heapExternalCallbackFact records that an opaque provider received a local
// callback which may mutate this captured table after the call returns. It
// invalidates only the closed-literal absence proof; it never invents a member
// value or an execution order.
func heapExternalCallbackFact(identity []byte, operation string) equation.Fact {
	return equation.Fact{Key: heapExternalCallbackPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation, Value: []byte("may-mutate")}
}

func heapHasExternalCallback(identity []byte, partition equation.Partition) bool {
	prefix := heapExternalCallbackPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range partition.ValuesPrefix(prefix) {
		if string(fact.Value) == "may-mutate" {
			return true
		}
	}
	return false
}

func heapMetaNewIndexCurrent(identity []byte, partition equation.Partition) ([]byte, bool) {
	prefix := heapMetaNewIndexPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	var value []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latest) {
			value, latest = fact.Value, fact.Key
		}
	}
	return append([]byte(nil), value...), value != nil
}

// heapMemberCurrent reads one heap member publication.  It reconverges for the
// same reason ordinary values do: a member written on a branch edge is the
// current member of that edge, and the point both edges reach holds their join.
// The member-identity lane joins by agreement only -- an allocation identity is
// a name, not a lattice, so edges naming different objects publish no identity
// at all rather than a widened one.
func heapMemberCurrent(prefix string, identity []byte, suffix string, partition equation.Partition) ([]byte, bool) {
	want := prefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + base64.RawURLEncoding.EncodeToString([]byte(suffix)) + "/"
	join := joinPublishedValues
	if prefix == heapMemberIdentityPrefix {
		join = joinAgreedValues
	}
	fact, found := partition.Reconverged(want, equation.Reconvergence{Current: latestPublication, Join: join})
	if !found {
		return nil, false
	}
	return fact.Value, true
}

// latestPublication is the current row inside one fully decided guard cube.
// Engine value keys end in the publishing operation coordinate, so the latest
// key is the last completed write that cube performed.
func latestPublication(candidates []equation.Fact) (equation.Fact, bool) {
	var latest equation.Fact
	selected := false
	for _, candidate := range candidates {
		if !selected || candidate.Key > latest.Key {
			latest, selected = candidate, true
		}
	}
	return latest, selected
}

// joinAgreedValues is the lattice for a payload that names something rather
// than describing it.  Agreement survives the join; disagreement withholds,
// because no widened name exists.
func joinAgreedValues(left, right []byte) ([]byte, bool) {
	if !bytes.Equal(left, right) {
		return nil, false
	}
	return append([]byte(nil), left...), true
}

func heapTableClosed(identity []byte, partition equation.Partition) bool {
	prefix := heapTableClosedPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "closed" {
			return true
		}
	}
	return false
}

func heapStaticReplacementFact(identity []byte, operation string) equation.Fact {
	return equation.Fact{Key: heapStaticReplacePrefix + base64.RawURLEncoding.EncodeToString(identity) + "/" + operation, Value: []byte("static")}
}

func heapStaticReplacement(identity []byte, partition equation.Partition) bool {
	prefix := heapStaticReplacePrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "static" {
			return true
		}
	}
	return false
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

func staticBracketMemberPath(term []byte) bool {
	_, suffix, member := tableAddress(term)
	if !member || suffix == "" {
		return false
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid {
		return false
	}
	for _, item := range segments {
		if item.Kind == segment.SegmentIndexString {
			return true
		}
	}
	return false
}

func scalarLiteralDiagnosticValue(value []byte) bool {
	return strings.HasPrefix(string(value), "scalar/string/") || strings.HasPrefix(string(value), "scalar/number/") || strings.HasPrefix(string(value), "scalar/bool/")
}

func literalDiagnosticValue(term []byte, partition equation.Partition) ([]byte, bool) {
	prefix := literalDiagnosticPrefix + string(term) + "/"
	var value []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && fact.Key > latest && scalarLiteralDiagnosticValue(fact.Value) {
			value, latest = append([]byte(nil), fact.Value...), fact.Key
		}
	}
	return value, value != nil
}

// heapTableAddress is the auxiliary heap lens for Lua's equivalent static
// member spellings. Source paths retain their structural bracket segments for
// type projection and diagnostics; only identity-backed heap reads and writes
// canonicalize valid bracket-string keys to the same slot as dot access.
func heapTableAddress(term []byte) ([]byte, string, bool) {
	root, suffix, ok := tableAddress(term)
	if !ok || suffix == "" {
		return root, suffix, ok
	}
	return root, fieldCanonicalTableSuffix(suffix), true
}

// fieldCanonicalTableSuffix gives Lua's equivalent static member spellings a
// shared heap lens. A bracket string key that is a valid field name addresses
// the same table slot as its dotted spelling, so it may consume facts already
// published for that exact slot. Non-identifier keys and dynamic reads retain
// their distinct, fail-closed paths.
func fieldCanonicalTableSuffix(suffix string) string {
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid {
		return suffix
	}
	changed := false
	for index, item := range segments {
		if item.Kind == segment.SegmentIndexString && tableFieldName(item.Name) {
			segments[index] = segment.Segment{Kind: segment.SegmentField, Name: item.Name}
			changed = true
		}
	}
	if !changed {
		return suffix
	}
	return segment.FormatSegments(segments)
}

func tableIdentityForTerm(term []byte, partition equation.Partition) ([]byte, bool) {
	if identity, ok := currentEpochFact(heapTableIdentityPrefix, term, partition); ok {
		return identity, true
	}
	root, suffix, ok := heapTableAddress(term)
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
		for count := 1; count <= len(segments); count++ {
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
	root, suffix, ok := heapTableAddress(term)
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
	var sealedReceiver typ.Type
	indexedPath := false
	for len(segments) != 0 {
		matched := false
		for count := 1; count <= len(segments); count++ {
			prefix := segment.FormatSegments(segments[:count])
			next, found := heapMemberCurrent(heapMemberIdentityPrefix, identity, prefix, partition)
			if !found {
				continue
			}
			for _, item := range segments[:count] {
				if item.Kind == segment.SegmentIndexString || item.Kind == segment.SegmentIndexInt {
					indexedPath = true
					break
				}
			}
			// The identity edge is usable only alongside its already-published
			// concrete member value. Retain that sealed receiver so a later
			// absent member can be reported as a missing-member fact, rather
			// than being conflated with Lua's untyped nil fallback.
			if member, present := heapMemberCurrent(heapMemberPrefix, identity, prefix, partition); present && !indexedPath {
				if receiver, decoded := sealedShapeReceiverType(member); decoded {
					sealedReceiver = receiver
				} else {
					sealedReceiver = nil
				}
			} else {
				sealedReceiver = nil
			}
			identity, segments, matched = next, segments[count:], true
			break
		}
		if matched {
			continue
		}
		whole := segment.FormatSegments(segments)
		if value, found := heapMemberCurrent(heapMemberPrefix, identity, whole, partition); found {
			return value, true
		}
		if heapTableClosed(identity, partition) && !heapMetaAttached(identity, partition) && !heapHasExternalCallback(identity, partition) {
			// A closed literal establishes that this member is absent at runtime,
			// but an existing declaration on its owning path can still describe a
			// wider optional member surface. Preserve that published nilability
			// when it is the only fact available. An explicit member write above
			// remains authoritative, so this cannot hide a concrete nil assignment.
			if value, optional := declaredOptionalMemberValue(term, partition); optional {
				return value, true
			}
			if sealedReceiver != nil && heapStaticReplacement(identity, partition) {
				if missing, encoded := memberMissingValue(sealedReceiver); encoded {
					return missing, true
				}
			}
			return []byte("scalar/nil"), true
		}
		return nil, false
	}
	return nil, false
}

// declaredOptionalMemberValue projects a static member only from the owning
// path's already-published declared type. It is used solely for a closed-literal
// absence: dynamic/open objects and explicit heap writes keep their ordinary
// value facts. A malformed path or a non-optional projection stays unavailable.
func declaredOptionalMemberValue(term []byte, partition equation.Partition) ([]byte, bool) {
	root, suffix, ok := tableAddress(term)
	if !ok || suffix == "" {
		return nil, false
	}
	segments, ok := segment.ParseFormattedSegments(suffix)
	if !ok || len(segments) == 0 {
		return nil, false
	}
	// An indexed path carries independent runtime absence: a declared array
	// element type must not turn a missing entry into a merely optional field.
	// This bridge is only for static record-member projections.
	if hasIndexSegment(segments) {
		return nil, false
	}
	declared, ok := declaredTypeForTerm(root, partition)
	if !ok || declared == nil || !closedMemberSurface(declared) {
		return nil, false
	}
	member, ok := luatypeprojection.ApplySegments(declared, segments)
	if !ok || !optionalConcreteWitnessType(member) {
		return nil, false
	}
	value, ok := shapefact.EncodeTarget(member)
	return value, ok
}

// declaredOptionalMapReadValue carries the existing typed entry witness for an
// exact static read from a declared map. A concrete heap cell can describe one
// retained write, but it is not a presence proof for a later lookup: the
// declared map contract still makes the selected element optional. This admits
// only a final bracket index over a declared map and an already-optional
// element witness, so dynamic/open reads and ordinary child publication remain
// on their existing fail-closed paths.
func declaredOptionalMapReadValue(term []byte, partition equation.Partition) ([]byte, bool) {
	if closedUnmutatedHeapRead(term, partition) {
		return nil, false
	}
	value, found, missingSlot := declaredOptionalMapReadWitness(term, partition)
	if !found || !missingSlot {
		return nil, false
	}
	return shapefact.EncodeTarget(value)
}

// declaredOptionalMapReadMissingSlot reports whether an exact declared map
// supplied nilability solely because this selected slot can be absent. The
// claim carries that closed distinction to its publisher so diagnostic text
// need not guess from a rendered structural type.
func declaredOptionalMapReadMissingSlot(term []byte, partition equation.Partition) bool {
	if closedUnmutatedHeapRead(term, partition) {
		return false
	}
	value, found, missingSlot := declaredOptionalMapReadWitness(term, partition)
	return found && missingSlot && mapReadNeedsNilWitness(value)
}

// mapReadNeedsNilWitness is deliberately limited to non-scalar map elements.
// Scalars already have an existing exact index-read transaction that retains
// their declared display and handles their missing-slot diagnostic. Aggregate
// and callable elements need the type witness here because their heap fallback
// is only nil and loses the published declared member contract.
func mapReadNeedsNilWitness(value typ.Type) bool {
	base := unwrap.Alias(subst.ExpandInstantiated(proof.ProjectionWithoutNil(value)))
	if base == nil {
		return false
	}
	switch base.Kind() {
	case kind.Boolean, kind.Number, kind.Integer, kind.String:
		return false
	default:
		return true
	}
}

// closedUnmutatedHeapRead preserves an exact selected member of a sealed
// literal until an indexed mutation revokes that fact. A declaration describes
// fallback absence only; it cannot displace a closed, unmutated concrete slot.
func closedUnmutatedHeapRead(term []byte, partition equation.Partition) bool {
	root, suffix, member := tableAddress(term)
	if !member || suffix == "" {
		return false
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid || len(segments) == 0 {
		return false
	}
	last := segment.FormatSegments(segments[len(segments)-1:])
	container := append(append([]byte(nil), root...), []byte(segment.FormatSegments(segments[:len(segments)-1]))...)
	identity, found := tableIdentityForTerm(container, partition)
	if !found || !heapTableClosed(identity, partition) || heapMetaAttached(identity, partition) {
		return false
	}
	prefix := heapIndexRevokePrefix + "identity/" + base64.RawURLEncoding.EncodeToString(identity) + "/"
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && string(fact.Value) == "revoked" {
			return false
		}
	}
	value, found := heapMemberCurrent(heapMemberPrefix, identity, last, partition)
	return found && string(value) != "scalar/nil" && !isUnknownScalar(value)
}

func declaredOptionalMapReadWitness(term []byte, partition equation.Partition) (typ.Type, bool, bool) {
	_, segments, source, ok := declaredMapReadAncestor(term, partition)
	if !ok || source == nil || len(segments) == 0 {
		return nil, false, false
	}
	// Lua's `t.k` is `t["k"]`, so the syntax of the final selector decides
	// nothing here. The container projection below is the authority: only a
	// declared map reaches the optional element witness.
	container, ok := luatypeprojection.ApplySegments(source, segments[:len(segments)-1])
	if !ok || container == nil {
		return nil, false, false
	}
	base := unwrap.Alias(subst.ExpandInstantiated(proof.ProjectionWithoutNil(container)))
	if base == nil || (base.Kind() != kind.Map && base.Kind() != kind.ReadonlyMap) {
		return nil, false, false
	}
	var declaredElement typ.Type
	switch mapping := base.(type) {
	case *typ.Map:
		declaredElement = mapping.Value
	case *typ.ReadonlyMap:
		declaredElement = mapping.Value
	}
	if declaredElement == nil {
		return nil, false, false
	}
	if _, union := unwrap.Alias(subst.ExpandInstantiated(declaredElement)).(*typ.Union); union {
		return nil, false, false
	}
	value, ok := luatypeprojection.ApplySegments(source, segments)
	if !ok || value == nil {
		return nil, false, false
	}
	missingSlot := !optionalConcreteWitnessType(container) && !optionalConcreteWitnessType(declaredElement)
	if !optionalConcreteWitnessType(value) {
		value = typ.MaterializeUnion([]typ.Type{value, typ.Nil})
	}
	if !optionalConcreteWitnessType(value) {
		return nil, false, false
	}
	return value, true, missingSlot
}

// declaredMapReadAncestor finds the nearest existing type publication for an
// indexed map read. Unlike typedAncestor, its root may be non-optional: the
// optionality being diagnosed belongs to the selected map element, not to the
// map container. The caller still admits only a typed final map index.
func declaredMapReadAncestor(term []byte, partition equation.Partition) ([]byte, []segment.Segment, typ.Type, bool) {
	if root, suffix, source, ok := typedAncestor(term, partition); ok {
		return root, suffix, source, true
	}
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
		segments, valid := segment.ParseFormattedSegments(suffix)
		if !valid || len(segments) == 0 {
			return nil, nil, nil, false
		}
		rootTerm := []byte("path/" + root)
		if source, declared := declaredTypeForTerm(rootTerm, partition); declared && source != nil {
			return rootTerm, segments, source, true
		}
	}
	return nil, nil, nil, false
}

// closedLiteralDeclaredOptionalMemberSource identifies the one optional value
// produced by declaredOptionalMemberValue. Its diagnostic is a nilability
// failure, while other optional values retain their established diagnostics.
func closedLiteralDeclaredOptionalMemberSource(source, value, encodedTarget []byte, partition equation.Partition) bool {
	declared, ok := declaredOptionalMemberValue(source, partition)
	if !ok || string(declared) != string(value) {
		return false
	}
	target, ok := shapefact.DecodeTarget(encodedTarget)
	return ok && target != nil && !subtype.IsSubtype(typ.Nil, target)
}

// nestedHeapMemberAddress follows an already-published member-identity prefix
// of a complete static suffix. It returns the deepest existing object and its
// remaining member suffix, never constructing identity from a source path.
// Dynamic writes use this to update aliases of an exact selected table member.
func nestedHeapMemberAddress(identity []byte, suffix string, partition equation.Partition) ([]byte, string, bool) {
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid || len(segments) < 2 {
		return nil, "", false
	}
	for count := 1; count < len(segments); count++ {
		prefix := segment.FormatSegments(segments[:count])
		next, found := heapMemberCurrent(heapMemberIdentityPrefix, identity, prefix, partition)
		if !found || len(next) == 0 {
			return nil, "", false
		}
		identity = next
	}
	return identity, segment.FormatSegments(segments[len(segments)-1:]), true
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
		value, known := resolveKnownCurrentValue(payload, partition)
		switch {
		case isolationStatePresent(partition, isolationEscapedPrefix, payload):
			return sendSafetyResult(operation, sendSafetyEscaped), true
		case known && carriesClosureIdentity(value):
			return sendSafetyResult(operation, sendSafetyCaptured), true
		case isolationStatePresent(partition, isolationFrozenPrefix, payload):
			return sendSafetyResult(operation, sendSafetyImmutable), true
		case strings.HasPrefix(string(payload), "temp/") && known && isIsolatedLiteral(value):
			return sendSafetyResult(operation, sendSafetyIsolated), true
		default:
			return sendSafetyResult(operation, sendSafetyUnproven), true
		}
	default:
		return equation.TransactionResult{}, false
	}
}

// sendSafetyVerdict pairs the source-facing hint with the structural row a
// native transfer consumer reads. The row names the transfer the runtime
// performs and the deopt event classes the verdict is bound to, so admission
// never depends on matching the message text.
type sendSafetyVerdict struct {
	message string
	content string
	events  string
}

var (
	sendSafetyIsolated = sendSafetyVerdict{
		message: "send payload is proven isolated for zero-copy transfer",
		content: "copy_required=false transfer=move verdict=isolated",
		events:  "escape,write.field",
	}
	sendSafetyImmutable = sendSafetyVerdict{
		message: "send payload is proven immutable for zero-copy sharing",
		content: "copy_required=false transfer=share verdict=immutable",
	}
	// The escape is what ended the payload's isolation, so the refutation names
	// that class as the point the isolation was lost.
	sendSafetyEscaped = sendSafetyVerdict{
		message: "send payload has a proven escaping alias; zero-copy transfer is rejected",
		content: "basis=owner_store copy_required=true verdict=escaped_refuted",
		events:  "escape",
	}
	// A captured environment refutes the transfer by proof rather than leaving
	// it unproven, so the row is a refutation while the hint stays the copy
	// fallback the source-facing rule already reports.
	sendSafetyCaptured = sendSafetyVerdict{
		message: "send payload is not proven isolated or immutable; runtime will copy",
		content: "basis=closure_capture copy_required=true verdict=escaped_refuted",
	}
	sendSafetyUnproven = sendSafetyVerdict{
		message: "send payload is not proven isolated or immutable; runtime will copy",
		content: "copy_required=true verdict=unproven_copy",
	}
)

func sendSafetyResult(operation equation.BoundEquation, verdict sendSafetyVerdict) equation.TransactionResult {
	key := "send_safety/" + operation.Target.Name
	if verdict.events != "" {
		key += "/contract-revocation/" + verdict.events
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{
		Values:      []equation.Fact{{Key: key, Value: []byte(verdict.content)}},
		Diagnostics: []equation.Fact{{Key: "send.isolation/" + operation.Target.Name, Value: []byte(verdict.message)}},
	}}
}

// carriesClosureIdentity is a refutation, not an absence of proof: a closed
// payload whose member is a function transports that function's captured
// environment across the actor boundary, and no transfer mode admits it.
func carriesClosureIdentity(value []byte) bool {
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed {
		return false
	}
	for _, member := range table.Members {
		if member.Present && strings.HasPrefix(member.Value, "scalar/function/") {
			return true
		}
	}
	return false
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
	callee       []byte
	receiver     []byte
	method       string
	display      string
	arguments    [][]byte
	assertedPath []byte
	spread       bool
	resultArity  int
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
		case operand.Role == "receiver-display":
			if result.display != "target" || len(operand.Value) == 0 {
				return directCallOperands{}, fmt.Errorf("engine: malformed receiver display")
			}
			result.display = string(operand.Value) + "." + result.method
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
		case operand.Role == "asserted-path":
			if result.assertedPath != nil || !strings.HasPrefix(string(operand.Value), "path/") || len(operand.Value) == len("path/") {
				return directCallOperands{}, fmt.Errorf("engine: malformed asserted call path")
			}
			result.assertedPath = append([]byte(nil), operand.Value...)
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

// assertedPathNarrowingFacts publishes the postcondition of a direct global
// assert. The front emits asserted-path only for a normalized truthy/not-nil
// check on that runtime-validating call. The value itself is never invented:
// it is the non-nil projection of the exact current or declared witness
// already available for the asserted path.
func assertedPathNarrowingFacts(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) []equation.Fact {
	if len(operands.assertedPath) == 0 {
		return nil
	}
	source, known := assertionPathType(operands.assertedPath, partition)
	if !known || source == nil {
		return nil
	}
	narrowed := proof.ProjectionWithoutNil(source)
	if narrowed == nil || typ.IsNever(narrowed) {
		return nil
	}
	value, encoded := shapefact.EncodeTarget(narrowed)
	if !encoded {
		return nil
	}
	return []equation.Fact{{
		Key:   "assertion-value/" + string(operands.assertedPath) + "/" + operation.Target.Name,
		Value: value,
	}}
}

func assertionPathType(term []byte, partition equation.Partition) (typ.Type, bool) {
	if value, err := resolveCurrentValue(term, partition); err == nil {
		if source, known := expressionValueType(value); known && source != nil {
			return source, true
		}
	}
	if source, known := correlationMemberType(term, partition); known && source != nil {
		return source, true
	}
	return declaredTypeForTerm(term, partition)
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
	if identity, found := tableIdentityForTerm(receiverTerm, partition); found && heapHasExternalCallback(identity, partition) {
		return nil, false
	}
	if strings.HasPrefix(string(receiverTerm), "path/") {
		memberTerm := []byte(string(receiverTerm) + "." + method)
		if value, known := resolveKnownCurrentValue(memberTerm, partition); known {
			return value, true
		}
	}
	return methodCallable(receiver, method)
}

// optionalMethodReceiverAtCall admits the receiver's own declared contract as
// the optional witness when the current value is the nil member of that same
// declaration. A proven-nil receiver satisfies the optional obligation even
// more strongly than an unnarrowed one, and the declaration is what names the
// callable member the call selects.
func optionalMethodReceiverAtCall(receiverTerm, receiver []byte, method string, partition equation.Partition) bool {
	if optionalMethodReceiver(receiverTerm, receiver, method) {
		return true
	}
	if string(receiver) != "scalar/nil" {
		return false
	}
	declared, found := declaredTypeForTerm(receiverTerm, partition)
	if !found || declared == nil {
		return false
	}
	encoded, ok := shapefact.EncodeTarget(declared)
	return ok && optionalMethodReceiver(receiverTerm, encoded, method)
}

// optionalMethodReceiver proves only the failure that follows from an
// already-published optional receiver type: a method exists on its non-nil
// projection, but this call has not established that projection.
func optionalMethodReceiver(receiverTerm, receiver []byte, method string) bool {
	// The caller restricts named paths to unguarded calls. A temporary is an
	// immediate expression receiver; either form is an exact source boundary.
	if !strings.HasPrefix(string(receiverTerm), "temp/") && !strings.HasPrefix(string(receiverTerm), "path/") {
		return false
	}
	receiverType, ok := shapefact.DecodeTarget(receiver)
	if !ok || !optionalConcreteWitnessType(receiverType) || method == "" {
		return false
	}
	projected := proof.ProjectionWithoutNil(receiverType)
	if projected == nil {
		return false
	}
	segments := []segment.Segment{{Kind: segment.SegmentField, Name: method}}
	callee, found := variant.FieldAtPath(projected, segments)
	if !found {
		callee, found = access.Field(projected, method)
	}
	if !found || callee == nil {
		return false
	}
	_, callable := unwrap.Alias(subst.ExpandInstantiated(callee)).(*typ.Function)
	return callable
}

// optionalCallableValue accepts a direct call target only when its existing
// value publication is an optional type whose non-nil projection is callable.
// It is a diagnostic predicate, not a dispatch fallback: the call remains
// rejected until a guard publishes the non-nil projection.
func optionalCallableValue(value []byte) bool {
	witness, ok := shapefact.DecodeTarget(value)
	if !ok || !optionalConcreteWitnessType(witness) {
		return false
	}
	_, callable := unwrap.Alias(subst.ExpandInstantiated(proof.ProjectionWithoutNil(witness))).(*typ.Function)
	return callable
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
	Params       []string            `json:"params"`
	Returns      []string            `json:"returns"`
	TypeParams   []callableTypeParam `json:"type_params"`
	Required     int                 `json:"required"`
	Variadic     bool                `json:"variadic"`
	VariadicType string              `json:"variadic_type,omitempty"`
	// Canonical is the front-published function type. The spelling fields
	// remain responsible for arity and display; Canonical carries structural
	// parameter contracts for sealed argument witnesses.
	Canonical string `json:"canonical,omitempty"`
}

func structuralArgumentWitness(value []byte) bool {
	if isCallableValue(value) {
		return false
	}
	if shapefact.IsTable(value) {
		return true
	}
	_, complete := scalarWitnessType(value)
	return complete
}

func callableParameterContract(signature callableShape, index int) (typ.Type, bool) {
	if signature.Canonical == "" || index < 0 {
		return nil, false
	}
	wire, err := base64.RawURLEncoding.DecodeString(signature.Canonical)
	if err != nil || len(wire) == 0 {
		return nil, false
	}
	decoded, err := typ.DecodeCanonical(context.Background(), wire)
	if err != nil || decoded == nil {
		return nil, false
	}
	function, ok := unwrap.Alias(decoded).(*typ.Function)
	if !ok || function == nil || index >= len(function.Params) || function.Params[index].Type == nil {
		return nil, false
	}
	return function.Params[index].Type, true
}

type callableTypeParam struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint,omitempty"`
}

func isCallableValue(value []byte) bool {
	if string(value) == "scalar/function" || strings.HasPrefix(string(value), "scalar/function/") {
		return true
	}
	callee, ok := shapefact.DecodeTarget(value)
	_, callable := unwrap.Alias(subst.ExpandInstantiated(callee)).(*typ.Function)
	return ok && callable
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
	if err := json.Unmarshal(wire, &signature); err != nil || signature.Required < 0 || signature.Required > len(signature.Params) || signature.Variadic != (signature.VariadicType != "") {
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

// callableParameterAt returns the already-published contract accepted at one
// source argument position. A variadic signature uses its explicit element
// contract for every tail slot; it never treats the arity marker itself as a
// type witness.
func callableParameterAt(signature callableShape, index int) (string, bool) {
	if index < 0 {
		return "", false
	}
	if index < len(signature.Params) {
		return signature.Params[index], true
	}
	if signature.Variadic && signature.VariadicType != "" {
		return signature.VariadicType, true
	}
	return "", false
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
	if signature.Variadic {
		signature.VariadicType = substituteCallableTypes([]string{signature.VariadicType}, bindings)[0]
	}
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
	if value, found := latestValue(term, partition); found {
		return value, true
	}
	// A member path can be a formal-derived read with no independent write.
	// resolveCurrentValue already projects it from the same sealed ancestor
	// shape. Dynamic/indexed paths retain their dedicated heap-current readers;
	// they cannot reuse an allocation-time table shape after a mutation.
	_, suffix, member := tableAddress(term)
	segments, static := segment.ParseFormattedSegments(suffix)
	if !member || !static || len(segments) == 0 {
		return nil, false
	}
	for _, item := range segments {
		if item.Kind != segment.SegmentField {
			return nil, false
		}
	}
	return shapeMemberValue(term, partition)
}

func numericForInductionSatisfies(term, value []byte, expected string, partition equation.Partition) bool {
	if strings.HasSuffix(expected, "?") {
		return string(value) != "scalar/nil" && numericForInductionSatisfies(term, value, strings.TrimSuffix(expected, "?"), partition)
	}
	valuePrefix := "value/" + string(term) + "/"
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, valuePrefix) && fact.Key > latest {
			latest = fact.Key
		}
	}
	if latest == "" {
		return false
	}
	operation := strings.TrimPrefix(latest, valuePrefix)
	witness, current := partition.Value(numericForInductionPrefix + string(term) + "/" + operation)
	if !current || !bytes.Equal(witness.Value, value) {
		return false
	}
	source, encoded := shapefact.DecodeTarget(value)
	if !encoded || source == nil {
		return false
	}
	source = unwrap.Alias(subst.ExpandInstantiated(source))
	if source == nil {
		return false
	}
	switch expected {
	case "nil":
		return source.Kind() == kind.Nil
	case "boolean":
		return source.Kind() == kind.Boolean
	case "string":
		return source.Kind() == kind.String
	case "number":
		return source.Kind() == kind.Number || source.Kind() == kind.Integer
	case "integer":
		return source.Kind() == kind.Integer
	default:
		return false
	}
}

// provenValueNotSubtype refutes a declared primitive contract from a published
// value. The value is decoded into the type whose values it witnesses and
// judged by the subtype relation, so a published type witness is measured by
// what it holds rather than by how it is encoded. A spelling that is not a
// closed primitive contract refutes nothing, and neither does a value this
// engine cannot place in the type domain.
func provenValueNotSubtype(value []byte, spelling string) bool {
	if isUnknownScalar(value) {
		return false
	}
	declared, resolved := primitiveContractType(spelling)
	if !resolved {
		return false
	}
	if witness, known := scalarWitnessType(value); known {
		return !subtype.IsSubtype(witness, declared)
	}
	// A table or a callable carries no scalar witness type, and no primitive
	// contract admits either of them.
	return shapefact.IsTable(value) || string(value) == "scalar/table" ||
		string(value) == "scalar/function" || strings.HasPrefix(string(value), "scalar/function/")
}

// primitiveContractType resolves the closed primitive spellings a published
// value can be judged against, including their optional forms. Any other
// spelling names a declaration whose contract this comparison does not own.
func primitiveContractType(spelling string) (typ.Type, bool) {
	spelling = strings.TrimSpace(spelling)
	if inner := strings.TrimSuffix(spelling, "?"); inner != spelling {
		resolved, ok := primitiveContractType(inner)
		if !ok {
			return nil, false
		}
		return normalize.Optional(resolved), true
	}
	switch spelling {
	case "nil", "boolean", "string", "number", "integer":
		return typ.BuiltinPrimitiveType(spelling)
	default:
		return nil, false
	}
}

func callDisplayValue(value []byte) string {
	display, err := displayValue(value)
	if err != nil {
		// A closed literal shape is an existing call-site value witness.  It is
		// presentation only here: the constraint kernel has already established
		// the refutation, and an open shape still renders as unknown.
		return boundaryShapeEvidenceValue(value)
	}
	return string(display)
}

// callDisplayValueForTerm preserves a runtime type witness when a guarded
// value is reported at a local function boundary. The kernel has already
// established the refutation; this only avoids presenting a pre-guard literal
// value in place of the currently published runtime type.
func callDisplayValueForTerm(term, value []byte, partition equation.Partition) string {
	if typeName, proven := runtimeTypeProofDisplay(term, partition); proven {
		return typeName
	}
	return callDisplayValue(value)
}

func runtimeTypeProofDisplay(term []byte, partition equation.Partition) (string, bool) {
	if !strings.HasPrefix(string(term), "path/") {
		return "", false
	}
	prefix := "runtime-type-proof/" + base64.RawURLEncoding.EncodeToString(term) + "/"
	var typeName string
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) || string(fact.Value) != "proven" {
			continue
		}
		candidate := strings.TrimPrefix(fact.Key, prefix)
		if candidate == "" || strings.Contains(candidate, "/") {
			return "", false
		}
		if typeName != "" && typeName != candidate {
			return "", false
		}
		typeName = candidate
	}
	return typeName, typeName != ""
}

// externalCallKernel is a sealed provider-boundary factor.  It intentionally
// owns no result term: call-results remains the sole result-slot owner for
// every call, whether the callee is local or external.
func externalCallKernel(lexical *lexicalEvaluator, operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
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
	arguments, complete := placementExternalArguments(operation)
	if !complete {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external call arguments")
	}
	values := placementExternalOwnershipFacts(operation, operands["provider"], arguments, partition)
	values = append(values, placementImportedStoreFacts(lexical, operation, operands["provider"], arguments, partition)...)
	values = append(values, opaqueCallbackCaptureEffects(lexical, operands["provider"], arguments, operation.Target.Name, partition)...)
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{
		Values: values,
	}}, nil
}

// opaqueCallbackCaptureEffects derives a may-mutate fact only from a callback
// handle already published in this partition and an unresolved provider. The
// callback body's own lowered write is the authority; no source name or
// synthetic callback behavior is consulted.
func opaqueCallbackCaptureEffects(lexical *lexicalEvaluator, provider []byte, arguments [][]byte, operation string, partition equation.Partition) []equation.Fact {
	if lexical == nil || providerName(provider) == "" {
		return nil
	}
	if _, known := (signaturelookup.Source{IncludeStdlib: true}).LookupView(providerName(provider)); known {
		return nil
	}
	facts := make([]equation.Fact, 0)
	seen := make(map[string]bool)
	for _, argument := range arguments {
		handle, known := closureHandleFor(argument, partition)
		if !known {
			continue
		}
		child, found := lexical.byPrototype[handle.Prototype]
		if !found || len(handle.Captures) != len(child.Boundary.Captures) {
			continue
		}
		for index, capture := range child.Boundary.Captures {
			if !childWritesCapture(child, boundaryTerm(capture.Symbol)) {
				continue
			}
			identity, found := tableIdentityForTerm([]byte(handle.Captures[index]), partition)
			if !found || seen[string(identity)] {
				continue
			}
			seen[string(identity)] = true
			facts = append(facts, heapExternalCallbackFact(identity, operation))
		}
	}
	return facts
}

func childWritesCapture(child front.Compilation, capture string) bool {
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "environment-write", "path-replacement", "index-mutation", "path-invalidation":
			target, found := artifactOperand(operation.Operands, "target")
			if found && (string(target) == capture || strings.HasPrefix(string(target), capture+".") || strings.HasPrefix(string(target), capture+"[")) {
				return true
			}
		}
	}
	return false
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
	var callee []byte
	var receiver []byte
	var method []byte
	var typePredicateErrorTarget []byte
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
		case operand.Role == "callee":
			if callee != nil || (!strings.HasPrefix(string(operand.Value), "path/") && !strings.HasPrefix(string(operand.Value), "temp/") && !strings.HasPrefix(string(operand.Value), "scalar/")) {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result callee")
			}
			callee = operand.Value
		case strings.HasPrefix(operand.Role, "argument-"):
			index, err := callArgumentIndex(operand.Role)
			if err != nil || argumentTerms[index] != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result argument %q", operand.Role)
			}
			argumentTerms[index] = operand.Value
		case operand.Role == "receiver":
			if receiver != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: duplicate call result receiver")
			}
			receiver = operand.Value
		case operand.Role == "method":
			if method != nil || !strings.HasPrefix(string(operand.Value), "method/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result method")
			}
			method = operand.Value
		case operand.Role == "type-predicate-error-target":
			if _, ok := shapefact.DecodeTarget(operand.Value); typePredicateErrorTarget != nil || !ok {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed type predicate error target")
			}
			typePredicateErrorTarget = operand.Value
		case operand.Role == "result-display":
			// Source display is descriptive only. It is emitted by the front
			// alongside this exact call-result publication and is never used as
			// a provider or value authority.
		case strings.HasPrefix(operand.Role, "result-"):
			resultTerms[strings.TrimPrefix(operand.Role, "result-")] = operand.Value
		case strings.HasPrefix(operand.Role, "target-"):
			targetTerms[strings.TrimPrefix(operand.Role, "target-")] = operand.Value
		default:
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result role %q", operand.Role)
		}
	}
	if !hasApplication || (receiver == nil) != (method == nil) || (len(targetTerms) != 0 && len(resultTerms) != len(targetTerms)) {
		return equation.TransactionResult{}, fmt.Errorf("engine: incomplete call result transaction")
	}
	if err := consumeCallArgumentFacts(application, argumentTerms, partition); err != nil {
		return equation.TransactionResult{}, err
	}
	methodProvider, hasMethodProvider := stdlibMethodProvider(receiver, method, partition)
	values := make([]equation.Fact, 0, len(resultTerms))
	for key, result := range resultTerms {
		if len(result) == 0 || !strings.HasPrefix(string(result), "temp/") || (len(targetTerms) != 0 && len(targetTerms[key]) == 0) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result %q", key)
		}
		value := []byte("scalar/top")
		localCallableResult := false
		projectedLocalResult := false
		importedRelation := false
		receiverResult := false
		receiverResultTerm := receiver
		setMetatableReceiver := []byte(nil)
		var importedSummary typ.Type
		var methodSummary typ.Type
		var localSummary typ.Type
		localUnionSummary, localUnion := inferredLocalCallableResultType(lexical, callee, mustCallResultIndex(key), partition)
		retainLocalUnion := localUnion && requiresLocalUnionProof(localUnionSummary) && !closedScalarCallArguments(argumentTerms, partition)
		// A known lexical apply seals its child outcome under the same
		// application coordinate. call-results is the sole owner of caller
		// result terms, so it consumes that private projection rather than
		// falling through to Top.
		projectedKey := "call-result/" + strings.TrimPrefix(string(application), "call/") + "/" + key
		if !retainLocalUnion {
			for _, fact := range partition.Values() {
				if fact.Key == projectedKey {
					value = append([]byte(nil), fact.Value...)
					projectedLocalResult = true
					break
				}
			}
		}
		if key == "00000000" && hasValue(partition, "effect.call-bool/"+strings.TrimPrefix(string(application), "call/")) {
			value = []byte("scalar/boolean")
		}
		if index, err := strconv.Atoi(key); err == nil {
			// A demanded local child may already have published the returned table
			// value. If its closed self-return contract identifies that table with
			// the receiver, restore the receiver's identity and member capabilities
			// below instead of treating the materialized child table as opaque.
			if shapefact.IsTable(value) {
				if contract, ok := sealedMethodReceiverResultValue(lexical, receiver, method, index, partition); ok {
					value, receiverResult = contract, true
				}
			}
			if string(value) == "scalar/top" && receiver != nil && externalCallbackReceiverMayMutate(receiver, provider, partition) {
				value = []byte("scalar/external-callback-any")
			}
			// A local child result is the most precise existing publication. Only
			// when it is absent may the result owner use the direct callee's sealed
			// function contract; an opaque callable has no such witness.
			if string(value) == "scalar/top" {
				if contract, ok := sealedCallableResultValue(lexical, callee, index, argumentTerms, partition); ok {
					value = contract
					localCallableResult = true
				}
			}
			if string(value) == "scalar/top" {
				if summary, ok := sealedCallableResultType(lexical, callee, index, argumentTerms, partition); ok && requiresLocalUnionProof(summary) {
					localSummary = summary
				}
				if localSummary == nil && localUnion && requiresLocalUnionProof(localUnionSummary) {
					localSummary = localUnionSummary
				}
			}
			if string(value) == "scalar/top" {
				if contract, ok := typedCallableResultValue(callee, index, argumentTerms, partition); ok {
					value = contract
				}
			}
			if string(value) == "scalar/top" {
				if contract, ok := sealedMethodResultValue(lexical, receiver, method, index, partition); ok {
					value = contract
				}
			}
			if string(value) == "scalar/top" {
				if contract, ok := sealedMethodReceiverResultValue(lexical, receiver, method, index, partition); ok {
					value, receiverResult = contract, true
				}
			}
			if string(value) == "scalar/top" {
				if contract, receiverTerm, ok := sealedStaticMemberReceiverResultValue(lexical, callee, index, partition); ok {
					value, receiverResult, receiverResultTerm = contract, true, receiverTerm
				}
			}
			if string(value) == "scalar/top" {
				if summary, ok := typedMethodReturnType(receiver, method, index, partition); ok {
					methodSummary = summary
				}
				if contract, ok := typedMethodResultValue(receiver, method, index, partition); ok {
					value = contract
				}
			}
			if string(value) == "scalar/top" {
				if contract, ok := ambientChannelMethodResultValue(receiver, method, index, partition); ok {
					value = contract
				}
			}
			if contract, ok := stdlibMethodResultValue(receiver, method, index, partition); ok {
				value = contract
			}
			if provider != nil {
				if contract, ok := providerResultValue(provider, index, argumentTerms, partition); ok {
					value = contract
				}
				if host, ok := hostGlobalProviderResultType(lexical, provider, index, argumentTerms, partition); ok {
					value, _ = providerReturnTypeValue(host)
					importedSummary = host
					lexical.setImportedAuthority(string(result), host)
				}
				if imported, ok := importedProviderResultValue(lexical, provider, index, argumentTerms, partition); ok {
					value = imported
				}
				if relation, template, parameterized, ok := importedProviderRelationValue(lexical, provider, application, index, argumentTerms, partition); ok {
					value = relation
					importedRelation = parameterized
					values = append(values, channelPayloadRelationFacts(template, string(result), operation.Target.Name, argumentTerms, partition)...)
					if template.Parameter != nil {
						if argument, found := argumentTerms[*template.Parameter]; found {
							receiverResult, receiverResultTerm = true, argument
						}
					}
					values = append(values, placementImportedReturnFacts(template, string(result), strings.TrimPrefix(string(application), "call/"))...)
				}
				if summary, ok := importedProviderResultType(lexical, provider, index, argumentTerms, partition); ok {
					// A parameterized export relation has already materialized the
					// caller's sealed value. Its broad declared return (for example
					// `table`) remains summary metadata, but cannot displace that
					// concrete relation as the assignment authority.
					if !importedRelation {
						importedSummary = summary
						lexical.setImportedAuthority(string(result), summary)
					}
				}
			}
			// setmetatable returns its first argument unchanged. Preserve only the
			// existing value and heap identity; the metatable's effects are modeled
			// independently by applyKernel and never fabricated from its shape.
			if string(value) == "scalar/top" && providerName(provider) == "setmetatable" {
				if receiver, found := argumentTerms[0]; found {
					setMetatableReceiver = receiver
				}
			}
		}
		values = append(values,
			equation.Fact{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: value},
			equation.Fact{Key: epochFactPrefix + string(result) + "/" + operation.Target.Name, Value: []byte(operation.Target.Name)},
		)
		if typePredicateErrorTarget != nil && key == "00000000" {
			values = append(values, equation.Fact{Key: typePredicateTargetPrefix + base64.RawURLEncoding.EncodeToString(result) + "/" + operation.Target.Name, Value: append([]byte(nil), typePredicateErrorTarget...)})
		}
		if localCallableResult {
			// The value came from this exact local callable's sealed contract.
			// Later static reads may preserve only its explicit nilability after
			// the owning environment write transports this marker.
			values = append(values, equation.Fact{Key: "local-call-result/" + string(result) + "/" + operation.Target.Name, Value: []byte("sealed")})
		}
		if importedRelation {
			values = append(values, equation.Fact{
				Key:   "imported-relation-result/" + base64.RawURLEncoding.EncodeToString(result) + "/" + operation.Target.Name,
				Value: []byte("scalar/bool/true"),
			})
		}
		if methodSummary != nil {
			encoded, encodeErr := typ.EncodeCanonical(context.Background(), methodSummary)
			if encodeErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: encode typed method result summary: %w", encodeErr)
			}
			values = append(values, equation.Fact{Key: summaryTypePrefix + string(result) + "/" + operation.Target.Name, Value: encoded})
			values = append(values, equation.Fact{Key: methodReturnSummaryPrefix + string(result) + "/" + operation.Target.Name, Value: encoded})
		}
		if localSummary != nil {
			encoded, encodeErr := typ.EncodeCanonical(context.Background(), localSummary)
			if encodeErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: encode local union result summary: %w", encodeErr)
			}
			values = append(values, equation.Fact{Key: summaryTypePrefix + string(result) + "/" + operation.Target.Name, Value: encoded})
		}
		if receiverResult {
			if identity, found := tableIdentityForTerm(receiverResultTerm, partition); found {
				values = append(values, heapIdentityFact(string(result), operation.Target.Name, identity))
			}
			members, memberErr := projectMemberClosures(string(result), receiverResultTerm, operation.Target.Name, partition)
			if memberErr != nil {
				return equation.TransactionResult{}, memberErr
			}
			values = append(values, members...)
		}
		if setMetatableReceiver != nil {
			if identity, found := tableIdentityForTerm(setMetatableReceiver, partition); found {
				values = append(values, heapIdentityFact(string(result), operation.Target.Name, identity))
			}
		}
		// A discriminated-record union is published as a summary, never as a
		// value: the summary is the contract the caller's own guard narrows,
		// while the value lattice keeps no unselected arm. Withholding it
		// entirely would leave the local guard nothing to select from.
		if importedSummary != nil {
			encoded, encodeErr := typ.EncodeCanonical(context.Background(), importedSummary)
			if encodeErr != nil {
				return equation.TransactionResult{}, fmt.Errorf("engine: encode imported result summary: %w", encodeErr)
			}
			values = append(values, equation.Fact{Key: summaryTypePrefix + string(result) + "/" + operation.Target.Name, Value: encoded})
			values = append(values, channelPayloadSummaryFacts(string(result), operation.Target.Name, importedSummary)...)
		}
		if resultIndex, err := strconv.Atoi(key); err == nil {
			optionalProvider := providerName(provider)
			if optionalProvider == "" && hasMethodProvider {
				optionalProvider = methodProvider
			}
			// A completed local child has already published this exact result slot.
			// A same-named stdlib provider is only a fallback for unresolved calls;
			// its any boundary cannot taint the local publication.
			if !projectedLocalResult && providerAnyResult(optionalProvider, resultIndex, len(resultTerms)) {
				values = append(values, equation.Fact{
					Key:   "provider-any-result/" + string(result) + "/" + operation.Target.Name,
					Value: []byte("unvalidated"),
				})
			}
			optional, hasOptional := optionalProviderResultValue(optionalProvider, resultIndex)
			if !hasOptional {
				optional, hasOptional = optionalCallableResultValue(callee, resultIndex, partition)
			}
			if hasOptional {
				values = append(values, equation.Fact{
					Key:   "optional-provider-result/" + string(result) + "/" + operation.Target.Name,
					Value: optional,
				})
				if origin, named := optionalResultOrigin(operation, resultIndex); named {
					values = append(values, equation.Fact{
						Key:   optionalResultOriginPrefix + string(result) + "/" + operation.Target.Name,
						Value: []byte(origin),
					})
				}
			}
		}
		if key, element, ok := iteratorElementWitness(provider, argumentTerms, partition); ok {
			values = append(values, equation.Fact{Key: iteratorElementPrefix + string(result) + "/" + operation.Target.Name, Value: element})
			if len(key) != 0 {
				values = append(values, equation.Fact{Key: iteratorKeyPrefix + string(result) + "/" + operation.Target.Name, Value: key})
			}
		}
		if source, found := iteratorSourceTerm(provider, argumentTerms); found {
			// The iterator source is selected by the published provider effect,
			// not by a callee name or source spelling. It discharges only this
			// same call's temporary opaque boundary.
			if allocation, found := placementAllocationForTerm(source, partition); found && placementClosedAllocation(allocation, partition) {
				applicationID := strings.TrimPrefix(string(application), "call/")
				if applicationID != "" {
					values = append(values, placementContractFact(allocation.Identity, "iterator", applicationID))
				}
			}
		}
		// A type predicate's result is the closed control witness for its
		// validated value. When the exact predicate argument is explicitly any,
		// retain that boundary on the control result so a possible arm is
		// evaluated without turning the value result into a type witness.
		if string(value) == "scalar/top" && typePredicateErrorTarget != nil && (key == "00000001" || (key == "00000000" && len(resultTerms) == 1)) {
			if argument, found := argumentTerms[0]; found {
				if root, _, ok := explicitAnySourceFact(argument, partition.Values()); ok {
					values = append(values, equation.Fact{Key: "gradual-any/" + string(result) + "/" + operation.Target.Name, Value: root})
				}
			}
		}
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
	// A `T:is(value)` call owns the paired `(value, error)` witness. When the
	// call is evaluated as a lexical child the target rides the projected
	// call-type-predicate fact; a direct builtin invocation carries the same
	// resolved target on its own front operand. Both are the identical closed
	// type T, so either source seals the pair.
	target, found := currentCallTypePredicateTarget(application, partition)
	if !found && typePredicateErrorTarget != nil {
		target, found = typePredicateErrorTarget, true
	}
	if found && len(resultTerms) == 2 {
		if value, okValue := resultTerms["00000000"]; okValue {
			if errValue, okErr := resultTerms["00000001"]; okErr {
				values = append(values, equation.Fact{Key: typePredicatePairPrefix + base64.RawURLEncoding.EncodeToString(value) + "/" + base64.RawURLEncoding.EncodeToString(errValue), Value: target})
			}
		}
	}
	values = append(values, importedReturnTupleFacts(lexical, provider, resultTerms, argumentTerms)...)
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

// importedReturnTupleFacts transports only the already-exported finite tuple
// catalog to the exact local result targets of this call. A tuple is useful
// here solely as a branch implication; it never replaces a call result value
// or infers a relation from a provider name, declaration, or source spelling.
func importedReturnTupleFacts(lexical *lexicalEvaluator, provider []byte, targets map[string][]byte, arguments map[int][]byte) []equation.Fact {
	if lexical == nil || provider == nil || len(targets) < 2 {
		return nil
	}
	module, suffix, load, ok := importedProviderTarget(provider)
	if !ok || load || suffix == "" {
		return nil
	}
	lexical.importedAuthorityMu.RLock()
	summary, found := lexical.importedRelations[module]
	lexical.importedAuthorityMu.RUnlock()
	if !found {
		return nil
	}
	function, found := summary.Function(strings.TrimPrefix(suffix, "."), len(arguments))
	if !found || len(function.ReturnTuples) == 0 {
		return nil
	}
	keys := make([]string, 0, len(targets))
	for index := range targets {
		keys = append(keys, index)
	}
	sort.Strings(keys)
	facts := make([]equation.Fact, 0)
	for _, triggerIndex := range keys {
		trigger := targets[triggerIndex]
		triggerSlot, err := strconv.Atoi(triggerIndex)
		if err != nil || !strings.HasPrefix(string(trigger), "temp/") {
			continue
		}
		for _, targetIndex := range keys {
			target := targets[targetIndex]
			targetSlot, err := strconv.Atoi(targetIndex)
			if err != nil || triggerSlot == targetSlot || !strings.HasPrefix(string(target), "temp/") {
				continue
			}
			valid, witnessed := true, false
			for _, tuple := range function.ReturnTuples {
				if triggerSlot >= len(tuple.Values) || targetSlot >= len(tuple.Values) {
					valid = false
					break
				}
				if tuple.Values[triggerSlot].Scalar != "scalar/bool/true" {
					continue
				}
				witnessed = true
				if tuple.Values[targetSlot].Scalar == "scalar/nil" || tuple.Values[targetSlot].Scalar == "" {
					valid = false
					break
				}
			}
			if valid && witnessed {
				facts = append(facts, equation.Fact{Key: returnTupleTruePrefix + base64.RawURLEncoding.EncodeToString(trigger) + "/" + base64.RawURLEncoding.EncodeToString(target), Value: []byte("proven")})
			}
		}
	}
	return facts
}

func mustCallResultIndex(key string) int {
	index, err := strconv.Atoi(key)
	if err != nil || index < 0 {
		return -1
	}
	return index
}

// closedScalarCallArguments reports whether every argument at this exact
// application boundary is an immutable scalar fact. A missing or broad
// argument cannot choose one member of a finite local return union.
func closedScalarCallArguments(arguments map[int][]byte, partition equation.Partition) bool {
	if len(arguments) == 0 {
		return true
	}
	for _, argument := range arguments {
		if !exactRelationScalar(argument) {
			return false
		}
	}
	return true
}

func currentCallTypePredicateTarget(application []byte, partition equation.Partition) ([]byte, bool) {
	prefix := callTypePredicatePrefix + strings.TrimPrefix(string(application), "call/") + "/"
	var target []byte
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && fact.Key > latest {
			if _, ok := shapefact.DecodeTarget(fact.Value); ok {
				target, latest = append([]byte(nil), fact.Value...), fact.Key
			}
		}
	}
	return target, target != nil
}

func iteratorSourceTerm(provider []byte, arguments map[int][]byte) ([]byte, bool) {
	signature, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(providerName(provider))
	if !found {
		return nil, false
	}
	iterator, found := iteration.ActiveIterator(signature.Effect.Labels)
	if !found {
		return nil, false
	}
	source, found := arguments[iterator.Source.Index]
	return source, found
}

func placementArgumentsPresent(operands directCallOperands, partition equation.Partition) bool {
	for _, argument := range operands.arguments {
		if allocation, found := placementAllocationForTerm(argument, partition); found && placementClosedAllocation(allocation, partition) {
			return true
		}
	}
	return false
}

// placementVerifiedLocalCallFacts closes the provisional opaque-call blocker
// only after applyKnown has completed the exact lexical child with the same
// caller partition.  The child projector carries any observed escape or
// opaque boundary back alongside this contract, so a completed local call is
// transparent only to the extent its published child facts prove it is.
func placementVerifiedLocalCallFacts(operation equation.BoundEquation, operands directCallOperands, partition equation.Partition) []equation.Fact {
	var facts []equation.Fact
	for _, argument := range operands.arguments {
		allocation, found := placementAllocationForTerm(argument, partition)
		if !found || !placementClosedAllocation(allocation, partition) {
			continue
		}
		facts = append(facts, placementContractFact(allocation.Identity, "local", operation.Target.Name))
	}
	return facts
}

func placementClosedLocalSummaryFacts(lexical *lexicalEvaluator, operation equation.BoundEquation, operands directCallOperands, handle closureHandle, partition equation.Partition) []equation.Fact {
	child, found := lexical.byPrototype[handle.Prototype]
	if !found || child.Cyclic != nil || len(handle.Captures) != 0 || len(child.Boundary.Captures) != 0 || !closedLocalParameterSummary(child) {
		return nil
	}
	return placementVerifiedLocalCallFacts(operation, operands, partition)
}

// closedLocalParameterSummary admits only a child that has no operation able
// to retain, return, or mutate a formal. Its finite body artifact is the
// authority; calls, writes, index operations, and publications of a formal
// all keep the caller-side opaque boundary intact.
func closedLocalParameterSummary(child front.Compilation) bool {
	formals := make(map[string]bool, len(child.Boundary.Parameters))
	for _, parameter := range child.Boundary.Parameters {
		formals[boundaryTerm(parameter.Symbol)] = true
	}
	for _, operation := range child.Artifact.Equations {
		switch operation.Occurrence.Kind {
		case "apply", "external-call", "call-results", "environment-write", "path-replacement", "index-mutation":
			return false
		case "publication":
			for _, operand := range operation.Operands {
				if strings.HasPrefix(operand.Role, "return-value-") && formals[string(operand.Term.Encoding)] {
					return false
				}
			}
		}
	}
	return true
}

// callArgumentFacts preserves the exact argument terms selected by an apply
// operation for its paired call-results consumer. The apply operation is the
// sole owner of call-site operands; call-results receives only its application
// coordinate, so it can consume these already-published references without
// reconstructing arguments from source syntax or declaration types.
func callArgumentFacts(application string, arguments [][]byte) []equation.Fact {
	if application == "" || len(arguments) == 0 {
		return nil
	}
	values := make([]equation.Fact, 0, len(arguments))
	for index, argument := range arguments {
		if len(argument) == 0 {
			return nil
		}
		values = append(values, equation.Fact{
			Key:   fmt.Sprintf("call-argument/%s/%08d", application, index),
			Value: append([]byte(nil), argument...),
		})
	}
	return values
}

// consumeCallArgumentFacts restores only the exact argument references
// published by the matching apply operation. A malformed or conflicting fact
// is an engine invariant failure; an absent fact simply leaves the result
// bridge without that argument and therefore fail-closed.
func consumeCallArgumentFacts(application []byte, arguments map[int][]byte, partition equation.Partition) error {
	callID, found := strings.CutPrefix(string(application), "call/")
	if !found || callID == "" {
		return fmt.Errorf("engine: malformed call result application")
	}
	prefix := "call-argument/" + callID + "/"
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		index, err := strconv.Atoi(strings.TrimPrefix(fact.Key, prefix))
		if err != nil || index < 0 || len(fact.Value) == 0 ||
			(!strings.HasPrefix(string(fact.Value), "path/") && !strings.HasPrefix(string(fact.Value), "temp/") && !strings.HasPrefix(string(fact.Value), "scalar/")) {
			return fmt.Errorf("engine: malformed call argument transport")
		}
		if existing, present := arguments[index]; present && !bytes.Equal(existing, fact.Value) {
			return fmt.Errorf("engine: conflicting call argument transport")
		}
		arguments[index] = append([]byte(nil), fact.Value...)
	}
	return nil
}

// sealedCallableResultValue bridges a local boundary whose body projection is
// unavailable. The closure capability and canonical function type are both
// already published by the allocation transaction. Requiring both means an
// imported declaration, an open callable, and a bare source annotation cannot
// manufacture a result fact here. This bridge transports only runtime scalar
// and callable facts; records and containers retain their child projection so
// a declared structural contract never becomes a synthetic shape witness.
// Generic closures likewise remain with their ordinary child projection until
// their exact call arguments have crossed the paired apply publication. An
// incomplete instantiation leaves the result slot Top.
func sealedCallableResultValue(lexical *lexicalEvaluator, callee []byte, index int, arguments map[int][]byte, partition equation.Partition) ([]byte, bool) {
	if lexical == nil || callee == nil || index < 0 {
		return nil, false
	}
	if _, local := closureHandleFor(callee, partition); !local {
		return nil, false
	}
	value, err := resolveCurrentValue(callee, partition)
	if err != nil {
		return nil, false
	}
	if functionType, ok := sealedFunctionType(value); ok {
		if function, ok := unwrap.Alias(subst.ExpandInstantiated(functionType)).(*typ.Function); ok && function != nil && len(function.TypeParams) != 0 {
			result, instantiated := instantiateProviderReturn(function, arguments, partition, index)
			returnValue, materialized := providerReturnTypeValue(result)
			return returnValue, instantiated && materialized
		}
	}
	return sealedFunctionResultValue(value, index)
}

// sealedCallableResultType exposes a local declared return only as an
// epoch-current summary. The same closure handle and sealed function witness
// required by sealedCallableResultValue keep a declaration or annotation from
// becoming an independent source of facts. Callers must still decide whether
// that summary is safe to materialize as a runtime value.
func sealedCallableResultType(lexical *lexicalEvaluator, callee []byte, index int, arguments map[int][]byte, partition equation.Partition) (typ.Type, bool) {
	if lexical == nil || callee == nil || index < 0 {
		return nil, false
	}
	if _, local := closureHandleFor(callee, partition); !local {
		return nil, false
	}
	value, err := resolveCurrentValue(callee, partition)
	if err != nil {
		return nil, false
	}
	functionType, ok := sealedFunctionType(value)
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(functionType)).(*typ.Function)
	if !ok || function == nil || index >= len(function.Returns) || function.Returns[index] == nil {
		return nil, false
	}
	return instantiateProviderReturn(function, arguments, partition, index)
}

// inferredLocalCallableResultType retains a finite union only when every
// return candidate is already a sealed literal shape in the callee artifact.
// This is summary metadata, not a runtime table value: callers still need a
// branch or a literal-correlated boundary relation before one arm can become
// concrete.
func inferredLocalCallableResultType(lexical *lexicalEvaluator, callee []byte, index int, partition equation.Partition) (typ.Type, bool) {
	if lexical == nil || callee == nil || index < 0 {
		return nil, false
	}
	handle, local := closureHandleFor(callee, partition)
	if !local {
		return nil, false
	}
	return inferredClosureResultAtIndex(lexical, handle, index)
}

// inferredClosureResultAtIndex retains a finite union only when every return
// candidate at this slot is already a sealed literal shape in the callee's
// child artifact. It is the handle-keyed core shared by the direct-call result
// owner and the module export projection; both hold an already-resolved local
// closure handle rather than a call-site callee spelling.
func inferredClosureResultAtIndex(lexical *lexicalEvaluator, handle closureHandle, index int) (typ.Type, bool) {
	if lexical == nil || index < 0 || len(handle.Captures) != 0 {
		return nil, false
	}
	child, found := lexical.byPrototype[handle.Prototype]
	if !found || child.Cyclic != nil {
		return nil, false
	}
	values := make(map[string][]byte)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		target, hasTarget := artifactOperand(operation.Operands, "target")
		value, hasValue := artifactOperand(operation.Operands, "value")
		if !hasTarget || !hasValue || !strings.HasPrefix(string(target), "temp/") || values[string(target)] != nil {
			continue
		}
		if table, sealed := shapefact.DecodeTable(value); sealed && table.Closed {
			values[string(target)] = append([]byte(nil), value...)
		}
	}
	returns := make([]typ.Type, 0)
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		term, found := artifactOperand(operation.Operands, fmt.Sprintf("return-value-%08d", index))
		if !found {
			continue
		}
		value, found := values[string(term)]
		if !found {
			return nil, false
		}
		result, concrete := sealedShapeReceiverType(value)
		if !concrete || result == nil {
			return nil, false
		}
		returns = append(returns, result)
	}
	if len(returns) == 0 {
		return nil, false
	}
	return typ.MaterializeUnion(returns), true
}

// inferredClosureSingleReturn publishes a module-exported local closure's
// inferred first return only when the closure returns a single value at every
// return site. A multi-value return is fail-closed here: the export function
// slot vocabulary this feeds carries one summary per member, so a partial
// arity would misstate the exported signature. Declared returns never reach
// this path; they already ride the closure's canonical signature.
func inferredClosureSingleReturn(lexical *lexicalEvaluator, handle closureHandle) (typ.Type, bool) {
	child, found := lexical.byPrototype[handle.Prototype]
	if lexical == nil || len(handle.Captures) != 0 || !found || child.Cyclic != nil {
		return nil, false
	}
	for _, operation := range child.Artifact.Equations {
		if operation.Occurrence.Kind != "publication" {
			continue
		}
		if _, multi := artifactOperand(operation.Operands, "return-value-00000001"); multi {
			return nil, false
		}
	}
	return inferredClosureResultAtIndex(lexical, handle, 0)
}

// inferredReturnMemberSummaries projects the inferred first-return type of each
// returned member closure. The return kernel already published the member
// suffix and its resolved local closure handle; this reuses the direct-call
// inference so a module export can carry an undeclared but statically finite
// return union to its importers. It is fail-closed: a member without an
// inferable single return publishes nothing.
func inferredReturnMemberSummaries(lexical *lexicalEvaluator, values []equation.Fact) []equation.Fact {
	if lexical == nil {
		return nil
	}
	type summaryEntry struct {
		encoded  []byte
		conflict bool
	}
	bySuffix := make(map[string]*summaryEntry)
	order := make([]string, 0)
	poison := func(suffix string) {
		if entry, seen := bySuffix[suffix]; seen {
			entry.conflict = true
			return
		}
		bySuffix[suffix] = &summaryEntry{conflict: true}
		order = append(order, suffix)
	}
	for _, fact := range values {
		if !strings.HasPrefix(fact.Key, "return-member-closure/") {
			continue
		}
		var wire memberClosureWire
		if json.Unmarshal(fact.Value, &wire) != nil || wire.Suffix == "" || !validClosureHandle(wire.Handle) {
			continue
		}
		returnType, ok := inferredClosureSingleReturn(lexical, wire.Handle)
		if !ok || returnType == nil {
			// A member that cannot be inferred at one return site must never be
			// asserted from another site's arm.
			poison(wire.Suffix)
			continue
		}
		encoded, err := typ.EncodeCanonical(context.Background(), returnType)
		if err != nil {
			poison(wire.Suffix)
			continue
		}
		if entry, seen := bySuffix[wire.Suffix]; seen {
			if entry.conflict || !bytes.Equal(entry.encoded, encoded) {
				entry.conflict = true
			}
			continue
		}
		bySuffix[wire.Suffix] = &summaryEntry{encoded: encoded}
		order = append(order, wire.Suffix)
	}
	var out []equation.Fact
	for _, suffix := range order {
		entry := bySuffix[suffix]
		if entry.conflict || len(entry.encoded) == 0 {
			continue
		}
		out = append(out, equation.Fact{Key: returnMemberSummaryPrefix + suffix, Value: entry.encoded})
	}
	return out
}

// typedCallableResultValue consumes a direct callee's exact canonical
// function surface. The surface must already be published at the callee path
// (including a typed member projection); declarations and source spelling are
// not alternate authorities, and unresolved/generic return slots stay Top.
func typedCallableResultValue(callee []byte, index int, arguments map[int][]byte, partition equation.Partition) ([]byte, bool) {
	if callee == nil || index < 0 {
		return nil, false
	}
	value, err := resolveCurrentValue(callee, partition)
	if err != nil {
		return nil, false
	}
	functionType, ok := shapefact.DecodeTarget(value)
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(functionType)).(*typ.Function)
	if !ok || function == nil || index >= len(function.Returns) || function.Returns[index] == nil {
		return nil, false
	}
	result, instantiated := instantiateProviderReturn(function, arguments, partition, index)
	returnValue, materialized := providerReturnTypeValue(result)
	return returnValue, instantiated && materialized
}

// sealedMethodResultValue is the method analogue of the direct-callee bridge.
// Both the exact member value and the local closure capability must already be
// published at the receiver/member path.  A declared receiver type, an open
// callable, or a stale table member has no authority to produce a result.
func sealedMethodResultValue(lexical *lexicalEvaluator, receiver, method []byte, index int, partition equation.Partition) ([]byte, bool) {
	if lexical == nil || receiver == nil || index < 0 {
		return nil, false
	}
	name, ok := callMethodName(method)
	if !ok {
		return nil, false
	}
	handle, local := methodClosureHandleFor(receiver, name, partition)
	if !local {
		return nil, false
	}
	if _, available := lexical.byPrototype[handle.Prototype]; !available {
		return nil, false
	}
	receiverValue, err := resolveCurrentValue(receiver, partition)
	if err != nil {
		return nil, false
	}
	callee, found := currentMethodCallable(receiver, receiverValue, name, partition)
	if !found && isClaimRefinement(receiverValue) {
		// A claim is not a callable witness. It may only expose the immediately
		// preceding sealed table through the same contiguous refinement chain
		// used by applyKernel; a real write ends that chain.
		if sealed, sealedFound := priorSealedTableValue(receiver, partition); sealedFound {
			callee, found = currentMethodCallable(receiver, sealed, name, partition)
		}
	}
	if !found {
		return nil, false
	}
	return sealedFunctionResultValue(callee, index)
}

// sealedMethodReceiverResultValue transports an exact sealed receiver through
// a local method only when its already-published callable contract accepts
// that receiver as the sole result. This preserves the table identity and
// member capabilities required by fluent recursive method chains; it does not
// construct a record from an annotation or apply to an open/imported method.
func sealedMethodReceiverResultValue(lexical *lexicalEvaluator, receiver, method []byte, index int, partition equation.Partition) ([]byte, bool) {
	if lexical == nil || receiver == nil || index != 0 {
		return nil, false
	}
	name, ok := callMethodName(method)
	if !ok {
		return nil, false
	}
	handle, local := methodClosureHandleFor(receiver, name, partition)
	if !local {
		return nil, false
	}
	if _, available := lexical.byPrototype[handle.Prototype]; !available {
		return nil, false
	}
	receiverValue, err := resolveCurrentValue(receiver, partition)
	if err != nil {
		return nil, false
	}
	if isClaimRefinement(receiverValue) {
		var found bool
		receiverValue, found = priorSealedTableValue(receiver, partition)
		if !found {
			return nil, false
		}
	}
	if !shapefact.IsTable(receiverValue) {
		return nil, false
	}
	callee, found := currentMethodCallable(receiver, receiverValue, name, partition)
	if !found {
		return nil, false
	}
	functionType, ok := sealedFunctionType(callee)
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(functionType).(*typ.Function)
	if !ok || function == nil || len(function.TypeParams) != 0 || len(function.Params) == 0 || function.Params[0].Type == nil || len(function.Returns) != 1 || function.Returns[0] == nil {
		return nil, false
	}
	// The canonical self parameter and sole return slot are the closed method
	// contract. Mutual subtype proof admits separately decoded recursive nodes
	// while still rejecting an arbitrary record-returning method.
	return receiverValue, subtype.IsSubtype(function.Params[0].Type, function.Returns[0]) && subtype.IsSubtype(function.Returns[0], function.Params[0].Type)
}

// sealedStaticMemberReceiverResultValue is the normalized-call counterpart of
// sealedMethodReceiverResultValue. The front may encode obj:method() as a
// direct call to path/obj.method, but the receiver path remains an existing
// closed fact rather than source-derived dispatch information.
func sealedStaticMemberReceiverResultValue(lexical *lexicalEvaluator, callee []byte, index int, partition equation.Partition) ([]byte, []byte, bool) {
	if !strings.HasPrefix(string(callee), "path/") {
		return nil, nil, false
	}
	cut := strings.LastIndex(string(callee), ".")
	if cut <= len("path/") || cut == len(callee)-1 {
		return nil, nil, false
	}
	receiver := append([]byte(nil), callee[:cut]...)
	method := []byte("method/" + strconv.Quote(string(callee[cut+1:])))
	value, ok := sealedMethodReceiverResultValue(lexical, receiver, method, index, partition)
	return value, receiver, ok
}

// typedMethodResultValue transports a scalar result contract from an already
// published typed receiver surface. Such surfaces are produced by closed
// module/import or control-flow publications; an annotation claim is encoded
// as a claim refinement and therefore cannot enter this path. The field must
// resolve through the canonical type graph and its return slot must still be a
// concrete provider value, so optional, union, and generic slots remain Top.
func typedMethodResultValue(receiver, method []byte, index int, partition equation.Partition) ([]byte, bool) {
	result, ok := typedMethodReturnType(receiver, method, index, partition)
	if !ok {
		return nil, false
	}
	return providerReturnTypeValue(result)
}

// ambientChannelMethodResultValue projects a channel method return from the
// receiver's already-published payload witness. A channel's methods are an
// ambient contract rather than record members, so the ordinary member
// projection never reaches them and the result slot would stay Top even where
// the payload is fully proven. The payload fact and the ambient signature are
// both existing publications; nothing here is derived from the method name
// alone, and a receiver without a proven payload keeps its Top result.
func ambientChannelMethodResultValue(receiver, method []byte, index int, partition equation.Partition) ([]byte, bool) {
	if receiver == nil || method == nil || index < 0 {
		return nil, false
	}
	name, ok := callMethodName(method)
	if !ok {
		return nil, false
	}
	payload, ok := typedChannelPayload(receiver, partition)
	if !ok || payload == nil {
		return nil, false
	}
	contract, status := typecall.MemberCall(typ.Instantiate(ambient.ChannelGeneric(), payload), name)
	if status != typecall.MemberCallOK {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(contract)).(*typ.Function)
	if !ok || function == nil || index >= len(function.Returns) || function.Returns[index] == nil {
		return nil, false
	}
	return providerReturnTypeValue(function.Returns[index])
}

// typedMethodReturnType projects a method return from an existing published
// receiver witness. The caller decides whether the type is materializable as a
// value or must remain a diagnostic-only summary; a union is never fabricated
// as a runtime shape merely to retain its member contracts.
func typedMethodReturnType(receiver, method []byte, index int, partition equation.Partition) (typ.Type, bool) {
	if receiver == nil || method == nil || index < 0 {
		return nil, false
	}
	name, ok := callMethodName(method)
	if !ok {
		return nil, false
	}
	receiverType, ok := currentTypedReceiverType(receiver, partition)
	if !ok {
		return nil, false
	}
	// Keep the existing record/union projection unchanged. Interfaces have no
	// record field, so only that unavailable case falls back to access.Field,
	// which substitutes Self through the published interface method contract.
	callee, ok := variant.FieldAtPath(receiverType, []segment.Segment{{Kind: segment.SegmentField, Name: name}})
	if !ok {
		callee, ok = access.Field(receiverType, name)
	}
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(callee)).(*typ.Function)
	if !ok || function == nil || index >= len(function.Returns) || function.Returns[index] == nil {
		return nil, false
	}
	return function.Returns[index], true
}

// currentTypedReceiverType reads only the value witness or canonical summary
// published at the receiver's current epoch. A method result may itself be an
// imported typed record, whose runtime value intentionally remains Top while
// its sealed summary is the sole authority for the next method lookup.
func currentTypedReceiverType(receiver []byte, partition equation.Partition) (typ.Type, bool) {
	if value, err := resolveCurrentValue(receiver, partition); err == nil {
		if receiverType, ok := shapefact.DecodeTarget(value); ok {
			return receiverType, true
		}
	}
	encoded, found := currentEpochFact(summaryTypePrefix, receiver, partition)
	if !found {
		return nil, false
	}
	receiverType, err := typ.DecodeCanonical(context.Background(), encoded)
	return receiverType, err == nil && methodSummaryComposable(receiverType, make(map[typ.Type]bool))
}

// methodSummaryComposable admits current summaries that can be consumed by
// static member and map-read composers. Sequences stay at their established
// iterator boundary: projecting an imported method summary through an array
// would otherwise create a second element authority alongside the iterator's
// own guarded publication.
func methodSummaryComposable(value typ.Type, seen map[typ.Type]bool) bool {
	if value == nil {
		return false
	}
	value = unwrap.Alias(subst.ExpandInstantiated(value))
	if value == nil || seen[value] {
		return value != nil
	}
	seen[value] = true
	switch value.Kind() {
	case kind.Any, kind.Unknown, kind.Never, kind.TypeParam, kind.Generic, kind.Ref, kind.Array, kind.Tuple:
		return false
	}
	return !typ.WalkChildren(value, func(child typ.Type) bool {
		return !methodSummaryComposable(child, seen)
	})
}

// typedMethodCallableSignature exposes a callable contract only from the
// receiver's already-published canonical type witness. It is deliberately
// transient: unlike a sealed table member, an interface member is not a new
// runtime value that can be published into the partition.
func typedMethodCallableSignature(receiver []byte, method string, partition equation.Partition) (callableShape, bool) {
	if receiver == nil || method == "" {
		return callableShape{}, false
	}
	value, err := resolveCurrentValue(receiver, partition)
	if err != nil {
		return callableShape{}, false
	}
	receiverType, ok := shapefact.DecodeTarget(value)
	if !ok {
		return callableShape{}, false
	}
	callee, ok := variant.FieldAtPath(receiverType, []segment.Segment{{Kind: segment.SegmentField, Name: method}})
	if !ok {
		callee, ok = access.Field(receiverType, method)
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(callee)).(*typ.Function)
	if !ok || function == nil {
		return callableShape{}, false
	}
	signature := callableShape{
		Params:   make([]string, len(function.Params)),
		Returns:  make([]string, len(function.Returns)),
		Variadic: function.Variadic != nil,
	}
	for index, parameter := range function.Params {
		if parameter.Type == nil {
			return callableShape{}, false
		}
		signature.Params[index] = parameter.Type.String()
		if bound, ok := unwrap.Annotations(parameter.Type).(*typ.TypeParam); ok && bound.Name != "" {
			found := false
			for _, existing := range signature.TypeParams {
				found = found || existing.Name == bound.Name
			}
			if !found {
				item := callableTypeParam{Name: bound.Name}
				if bound.Constraint != nil {
					item.Constraint = bound.Constraint.String()
				}
				signature.TypeParams = append(signature.TypeParams, item)
			}
		}
		if !parameter.Optional && !strings.HasSuffix(signature.Params[index], "?") {
			signature.Required++
		}
	}
	for index, result := range function.Returns {
		if result == nil {
			return callableShape{}, false
		}
		signature.Returns[index] = result.String()
	}
	if function.Variadic != nil {
		signature.VariadicType = function.Variadic.String()
		if signature.VariadicType == "" {
			return callableShape{}, false
		}
	}
	for _, parameter := range function.TypeParams {
		if parameter == nil || parameter.Name == "" {
			return callableShape{}, false
		}
		duplicate := false
		for _, existing := range signature.TypeParams {
			duplicate = duplicate || existing.Name == parameter.Name
		}
		if duplicate {
			continue
		}
		item := callableTypeParam{Name: parameter.Name}
		if parameter.Constraint != nil {
			item.Constraint = parameter.Constraint.String()
		}
		signature.TypeParams = append(signature.TypeParams, item)
	}
	return signature, true
}

// callableParameterRejectsNil reads the closed signature spelling already
// attached to this call. Optional parameters are normalized with a trailing
// question mark by the type formatter; nil and top-like parameters impose no
// non-nil proof obligation.
func callableParameterRejectsNil(parameter string) bool {
	expected := callableParameterType(parameter)
	return expected != "" && expected != "any" && expected != "unknown" && expected != "nil" && !strings.HasSuffix(expected, "?")
}

// optionalArgumentMayBeNil accepts only a current published path type whose
// concrete type includes nil. It never treats scalar Top or a local claim as
// evidence, preserving the engine's ordinary fail-closed gradual boundary.
func optionalArgumentMayBeNil(argument []byte, partition equation.Partition) bool {
	actual, available := typedPathType(argument, partition)
	if !available || actual == nil || !proof.OptionalTypeHasConcreteValue(actual) {
		return false
	}
	// An unselected discriminated union has no member surface of its own: a
	// member declared on some arms only reads nil on the others, and that nil
	// describes the unselected arm rather than a proven nilability of this
	// argument. The branch machinery publishes the selected arm before such a
	// read, so this boundary waits for it instead of refuting the call.
	if _, suffix, source, ok := typedAncestor(argument, partition); ok && len(suffix) != 0 && requiresLocalUnionProof(source) {
		return false
	}
	return true
}

func declaredEntryBoundaryKey(body equation.BodyID) string {
	return "declared-entry-boundary/" + fmt.Sprintf("%x", body)
}

// declaredEntryBoundary is present only in a versioned private entry for a
// capture-free, declaration-owned body. It does not establish nilability; it
// merely scopes consumption of the separate optional provider result fact.
func declaredEntryBoundary(body equation.BodyID, partition equation.Partition) bool {
	for _, fact := range partition.Values() {
		if fact.Key == declaredEntryBoundaryKey(body) && string(fact.Value) == "declared" {
			return true
		}
	}
	return false
}

func optionalProviderArgumentMayBeNil(argument []byte, partition equation.Partition) bool {
	_, found := currentEpochFact("optional-provider-result/", argument, partition)
	return found
}

func hasPublishedOptionalArgument(arguments [][]byte, partition equation.Partition) bool {
	for _, argument := range arguments {
		if optionalArgumentMayBeNil(argument, partition) {
			return true
		}
	}
	return false
}

// publishedOptionalAssignmentWitness is the assignment counterpart of the
// call boundary above. The source and target are both existing canonical type
// publications: an optional source can refute only a target that excludes
// nil, while untyped and annotation-only paths remain unavailable.
func publishedOptionalAssignmentWitness(source, encodedTarget []byte, partition equation.Partition) bool {
	actual, available := typedPathType(source, partition)
	if !available || !proof.OptionalTypeHasConcreteValue(actual) {
		return false
	}
	target, decoded := shapefact.DecodeTarget(encodedTarget)
	return decoded && target != nil && !subtype.IsSubtype(typ.Nil, target)
}

func sealedFunctionResultValue(value []byte, index int) ([]byte, bool) {
	decoded, ok := sealedFunctionType(value)
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(decoded).(*typ.Function)
	if !ok || function == nil || len(function.TypeParams) != 0 || index < 0 || index >= len(function.Returns) {
		return nil, false
	}
	return finiteReturnWitnessValue(function.Returns[index])
}

// stdlibMethodResultValue crosses only a sealed receiver fact into the
// existing standard-library signature registry.  It neither recognizes a
// method by source spelling nor trusts the call's annotation: an unknown or
// non-string receiver leaves the result at Top.
func stdlibMethodResultValue(receiver, method []byte, index int, partition equation.Partition) ([]byte, bool) {
	provider, ok := stdlibMethodProvider(receiver, method, partition)
	if !ok {
		return nil, false
	}
	result, ok := signaturelookup.StdlibResultSlot(provider, index)
	if !ok {
		return nil, false
	}
	return providerReturnTypeValue(result)
}

// stdlibMethodProvider resolves a method only through its current sealed
// receiver fact and the canonical standard-library registry. Keeping this
// lookup shared makes value and optional-result publications agree on the
// exact same authority.
func stdlibMethodProvider(receiver, method []byte, partition equation.Partition) (string, bool) {
	if receiver == nil || method == nil {
		return "", false
	}
	name, ok := callMethodName(method)
	if !ok {
		return "", false
	}
	value, err := resolveCurrentValue(receiver, partition)
	if err != nil {
		return "", false
	}
	receiverType := typ.Type(nil)
	if strings.HasPrefix(string(value), "scalar/string/") {
		receiverType = typ.String
	} else if decoded, decodedOK := shapefact.DecodeTarget(value); decodedOK {
		receiverType = decoded
	}
	if receiverType == nil {
		// Formal and declared locals publish their exact type at their entry or
		// write boundary. This is descriptive evidence only, but it is the
		// established authority for resolving a standard-library method contract
		// when the runtime value remains Top.
		receiverType, _ = declaredTypeForTerm(receiver, partition)
	}
	if receiverType == nil {
		return "", false
	}
	return signaturelookup.StdlibMethodProvider(receiverType, name)
}

// providerResultValue turns a finite, declared stdlib result slot into a
// canonical type fact.  A malformed provider, unknown dynamic tail, or any
// result type intentionally leaves the call-result owner at Top.
func providerResultValue(provider []byte, index int, arguments map[int][]byte, partition equation.Partition) ([]byte, bool) {
	name := providerName(provider)
	if name == "" {
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
	// A global provider's declared function is the same closed authority used
	// for its finite result slots.  Instantiate it only from exact argument
	// facts already published at this call boundary.  This preserves optional
	// results (for example table.remove) without promoting an unresolved
	// generic parameter or an open call into a proof.
	if signature, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(name); found && signature.Type != nil {
		if result, instantiated := instantiateProviderReturn(signature.Type, arguments, partition, index); instantiated {
			return providerReturnTypeValue(result)
		}
	}
	result, ok := signaturelookup.StdlibResultSlot(name, index)
	if !ok {
		return nil, false
	}
	return providerReturnTypeValue(result)
}

func providerName(provider []byte) string {
	encoded := strings.TrimPrefix(string(provider), "provider/global/")
	if encoded == string(provider) || encoded == "" {
		return ""
	}
	name, err := strconv.Unquote(encoded)
	if err != nil {
		return ""
	}
	return name
}

func externalCallbackReceiverMayMutate(receiver, provider []byte, partition equation.Partition) bool {
	name := providerName(provider)
	if name == "" {
		return false
	}
	if _, known := (signaturelookup.Source{IncludeStdlib: true}).LookupView(name); known {
		return false
	}
	identity, found := tableIdentityForTerm(receiver, partition)
	return found && heapHasExternalCallback(identity, partition)
}

// requiresLocalUnionProof keeps a declared discriminated-record union from
// becoming a value proof at an import boundary. Its selected arm is a runtime
// fact, so member publication must wait for the caller's local guard.
func requiresLocalUnionProof(value typ.Type) bool {
	union, ok := unwrap.Alias(unwrap.Annotations(value)).(*typ.Union)
	if !ok || union == nil {
		return false
	}
	records := 0
	for _, member := range union.Members {
		if _, ok := unwrap.Alias(unwrap.Annotations(member)).(*typ.Record); ok {
			records++
		}
	}
	return records > 1
}

// importedProviderResultValue projects the export seeded at the consumer
// entry into either require's result or a statically selected require-member
// call result. The provider identity is emitted by the front from the exact
// local-require binding, so a source spelling or global lookup cannot select
// an import fact.
func importedProviderResultValue(lexical *lexicalEvaluator, provider []byte, index int, arguments map[int][]byte, partition equation.Partition) ([]byte, bool) {
	result, ok := importedProviderResultType(lexical, provider, index, arguments, partition)
	if !ok {
		return nil, false
	}
	return importedReturnValue(result)
}

// hostGlobalProviderResultType resolves a host-installed global only from the
// project-selected GlobalTypes publication. The provider name is emitted by
// the front from the global symbol/path, while the callable and its result are
// still read exclusively from the host's exact manifest type.
func hostGlobalProviderResultType(lexical *lexicalEvaluator, provider []byte, index int, arguments map[int][]byte, partition equation.Partition) (typ.Type, bool) {
	name := providerName(provider)
	root, suffix, found := strings.Cut(name, ".")
	if !found || root == "" || suffix == "" {
		return nil, false
	}
	global, ok := lexical.globalType(root)
	if !ok {
		return nil, false
	}
	segments, valid := segment.ParseFormattedSegments("." + suffix)
	if !valid || len(segments) == 0 {
		return nil, false
	}
	callee, found := variant.FieldAtPath(global, segments)
	if !found {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(callee)).(*typ.Function)
	if !ok || function == nil || index < 0 || index >= len(function.Returns) {
		return nil, false
	}
	return instantiateProviderReturn(function, arguments, partition, index)
}

// importedProviderResultType is the exact resolved return slot of a module
// provider. It is derived solely from the require-seeded entry export and
// closed call arguments, so callers may carry it as summary metadata without
// turning an absent provider result into a type witness.
func importedProviderRelationValue(lexical *lexicalEvaluator, provider, application []byte, index int, arguments map[int][]byte, partition equation.Partition) ([]byte, exportrelation.Value, bool, bool) {
	if lexical == nil || index != 0 {
		return nil, exportrelation.Value{}, false, false
	}
	module, suffix, load, ok := importedProviderTarget(provider)
	if !ok || load || suffix == "" {
		return nil, exportrelation.Value{}, false, false
	}
	suffix = strings.TrimPrefix(suffix, ".")
	lexical.importedAuthorityMu.RLock()
	summary, found := lexical.importedRelations[module]
	lexical.importedAuthorityMu.RUnlock()
	if !found {
		return nil, exportrelation.Value{}, false, false
	}
	function, found := summary.Function(suffix, len(arguments))
	if !found {
		return nil, exportrelation.Value{}, false, false
	}
	declared, found := importedProviderResultType(lexical, provider, index, arguments, partition)
	if found && !relationReturnTypeSafe(declared, make(map[typ.Type]bool)) {
		return nil, exportrelation.Value{}, false, false
	}
	template, selected := importedReturnTemplate(function, arguments, partition)
	if !selected {
		return nil, exportrelation.Value{}, false, false
	}
	value, ok := materializeImportedReturn(template, application, arguments, partition)
	return value, template, templateUsesParameter(template), ok
}

// importedReturnTemplate selects only a closed relation that the provider
// already published. Conditional relations require the exact scalar argument
// at their recorded formal position; broad or missing arguments keep the
// declared summary rather than choosing a return arm.
func importedReturnTemplate(function exportrelation.Function, arguments map[int][]byte, partition equation.Partition) (exportrelation.Value, bool) {
	if function.Return.Valid(function.Arity) {
		return function.Return, true
	}
	conditional := function.Conditional
	if !conditional.Valid(function.Arity) {
		return exportrelation.Value{}, false
	}
	argument, found := arguments[conditional.Parameter]
	if !found {
		return exportrelation.Value{}, false
	}
	value, found := resolveKnownCurrentValue(argument, partition)
	if !found || !exactRelationScalar(value) {
		return exportrelation.Value{}, false
	}
	if string(value) == conditional.Literal {
		return conditional.Match, true
	}
	return conditional.Otherwise, true
}

func exactRelationScalar(value []byte) bool {
	if string(value) == "scalar/bool/true" || string(value) == "scalar/bool/false" || string(value) == "scalar/nil" {
		return true
	}
	if strings.HasPrefix(string(value), "scalar/string/") {
		_, err := strconv.Unquote(strings.TrimPrefix(string(value), "scalar/string/"))
		return err == nil
	}
	if strings.HasPrefix(string(value), "scalar/number/") {
		_, err := strconv.ParseFloat(strings.TrimPrefix(string(value), "scalar/number/"), 64)
		return err == nil
	}
	return false
}

func templateUsesParameter(template exportrelation.Value) bool {
	if template.Parameter != nil {
		return true
	}
	for _, member := range template.Table {
		if templateUsesParameter(member.Value) {
			return true
		}
	}
	return false
}

// A literal template has no dynamic-key lane.  Keep declared map components
// authoritative instead of replacing them with a finite sample of entries.
func relationReturnTypeSafe(value typ.Type, seen map[typ.Type]bool) bool {
	value = unwrap.Alias(value)
	if value == nil || seen[value] {
		return value != nil
	}
	seen[value] = true
	switch item := value.(type) {
	case *typ.Map, *typ.ReadonlyMap:
		return false
	case *typ.Array:
		return relationReturnTypeSafe(item.Element, seen)
	case *typ.Tuple:
		for _, element := range item.Elements {
			if !relationReturnTypeSafe(element, seen) {
				return false
			}
		}
		return true
	case *typ.Optional:
		return relationReturnTypeSafe(item.Inner, seen)
	case *typ.Record:
		if item.MapKey != nil || item.MapValue != nil {
			return false
		}
		for _, field := range item.Fields {
			if !relationReturnTypeSafe(field.Type, seen) {
				return false
			}
		}
	}
	return true
}

func materializeImportedReturn(template exportrelation.Value, application []byte, arguments map[int][]byte, partition equation.Partition) ([]byte, bool) {
	if template.Parameter != nil {
		argumentKey := "call-argument/" + strings.TrimPrefix(string(application), "call/") + "/" + fmt.Sprintf("%08d", *template.Parameter)
		for _, fact := range partition.Values() {
			if fact.Key == argumentKey {
				// The transport publication names the exact caller term. Resolve
				// that already-published term at its current epoch so a parameter
				// return carries the caller's sealed value rather than an opaque
				// path token. If the term has no current fact, the relation remains
				// unavailable instead of manufacturing a result value.
				value, found := resolveKnownCurrentValue(fact.Value, partition)
				if !found {
					return nil, false
				}
				return importedParameterSurface(fact.Value, value, partition), true
			}
		}
		term, found := arguments[*template.Parameter]
		if !found {
			return nil, false
		}
		value, found := resolveKnownCurrentValue(term, partition)
		if !found {
			return nil, false
		}
		return importedParameterSurface(term, value, partition), true
	}
	if template.Scalar != "" {
		return []byte(template.Scalar), true
	}
	if len(template.Table) == 0 {
		return nil, false
	}
	table := shapefact.Table{Closed: true, Members: make([]shapefact.Member, 0, len(template.Table))}
	for _, member := range template.Table {
		value, ok := materializeImportedReturn(member.Value, application, arguments, partition)
		if !ok {
			return nil, false
		}
		table.Members = append(table.Members, shapefact.Member{Suffix: member.Suffix, Present: string(value) != "scalar/nil", Value: string(value)})
		if string(value) == "scalar/nil" {
			table.Members[len(table.Members)-1].Value = ""
		}
		// A nested finite member is already part of the same validated return
		// relation. Publish its exact path as well as the parent table so a
		// later dotted read consumes the relation witness instead of treating
		// the nested cell as absent.
		if nested, nestedOK := shapefact.DecodeTable(value); nestedOK && nested.Closed {
			for _, child := range nested.Members {
				if child.Present && child.Suffix != "" {
					table.Members = append(table.Members, shapefact.Member{Suffix: member.Suffix + child.Suffix, Present: true, Value: child.Value})
				}
			}
		}
	}
	return shapefact.EncodeTable(table)
}

// importedParameterSurface snapshots the current direct members of a sealed
// caller-owned table for an exported identity relation. Both the relation and
// every member fact are existing publications. An open or callback-exposed
// heap remains at its original broad value, so this helper never grants a
// structural proof from an unresolved or mutable boundary.
func importedParameterSurface(term, value []byte, partition equation.Partition) []byte {
	table, sealed := shapefact.DecodeTable(value)
	if !sealed || !table.Closed || len(table.Members) != 0 {
		return value
	}
	identity, found := tableIdentityForTerm(term, partition)
	if !found || !heapTableClosed(identity, partition) {
		return value
	}
	return heapMemberSurface(term, value, partition)
}

// heapMemberSurface republishes a sealed table value with the current member
// publications of its heap identity. A static member write advances the member
// cell rather than the aggregate value fact, so a read inside the writing
// partition consumes the write while the aggregate still carries the
// allocation-time shape. A consumer that transports the aggregate out of that
// partition must consume the same member authority; otherwise the closed table
// would prove the absence of a member it demonstrably has. An externally exposed
// heap keeps its original broad value.
func heapMemberSurface(term, value []byte, partition equation.Partition) []byte {
	table, sealed := shapefact.DecodeTable(value)
	if !sealed || !table.Closed {
		return value
	}
	// A static member write publishes the written cell at its exact member path.
	// The absence of any such publication proves this partition performed no
	// static member write on this term, so the aggregate is already current and
	// the heap walk is skipped.
	if !hasStaticMemberPathWrite(term, partition) {
		return value
	}
	identity, found := tableIdentityForTerm(term, partition)
	if !found || heapHasExternalCallback(identity, partition) {
		return value
	}
	prefix := heapMemberPrefix + base64.RawURLEncoding.EncodeToString(identity) + "/"
	latest := make(map[string]equation.Fact)
	for _, fact := range partition.ValuesPrefix(prefix) {
		rest := strings.TrimPrefix(fact.Key, prefix)
		encodedSuffix, _, ok := strings.Cut(rest, "/")
		if !ok {
			continue
		}
		suffixBytes, err := base64.RawURLEncoding.DecodeString(encodedSuffix)
		if err != nil || !segment.ValidFormattedSegments(string(suffixBytes)) {
			continue
		}
		suffix := string(suffixBytes)
		if prior, exists := latest[suffix]; !exists || fact.Key > prior.Key {
			latest[suffix] = fact
		}
	}
	if len(latest) == 0 {
		return value
	}
	members := make(map[string]shapefact.Member, len(table.Members)+len(latest))
	for _, member := range table.Members {
		members[member.Suffix] = member
	}
	for suffix, fact := range latest {
		members[suffix] = shapefact.Member{Suffix: suffix, Present: string(fact.Value) != "scalar/nil", Value: string(fact.Value)}
	}
	table.Members = table.Members[:0]
	for _, member := range members {
		table.Members = append(table.Members, member)
	}
	sort.Slice(table.Members, func(i, j int) bool { return table.Members[i].Suffix < table.Members[j].Suffix })
	if surfaced, ok := shapefact.EncodeTable(table); ok {
		return surfaced
	}
	return value
}

// hasStaticMemberPathWrite recognizes the exact path-value counterpart of a
// heap member write.  Both dot and integer/string bracket paths are static
// WIR lenses; dynamic writes do not carry such a path fact and therefore
// cannot make a returned aggregate more precise.
func hasStaticMemberPathWrite(term []byte, partition equation.Partition) bool {
	prefix := "value/" + string(term)
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(fact.Key, prefix)
		if len(rest) == 0 || (rest[0] != '.' && rest[0] != '[') {
			continue
		}
		cut := strings.LastIndexByte(rest, '/')
		if cut <= 0 || !segment.ValidFormattedSegments(rest[:cut]) {
			continue
		}
		return true
	}
	return false
}

func importedProviderResultType(lexical *lexicalEvaluator, provider []byte, index int, arguments map[int][]byte, partition equation.Partition) (typ.Type, bool) {
	modulePath, suffix, requireResult, ok := importedProviderTarget(provider)
	if !ok {
		return nil, false
	}
	imported, ok := lexical.importedType(modulePath)
	if !ok {
		imported, ok = importedEntryType(modulePath, partition)
	}
	if !ok {
		return nil, false
	}
	if requireResult {
		if index != 0 {
			return nil, false
		}
		return imported, true
	}
	segments, valid := segment.ParseFormattedSegments(suffix)
	if !valid && suffix != "" {
		return nil, false
	}
	callee := imported
	if len(segments) != 0 {
		var found bool
		callee, found = variant.FieldAtPath(imported, segments)
		if !found {
			return nil, false
		}
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(callee)).(*typ.Function)
	if !ok || function == nil || index < 0 || index >= len(function.Returns) || function.Returns[index] == nil {
		return nil, false
	}
	returnType, ok := instantiateProviderReturn(function, arguments, partition, index)
	if !ok {
		return nil, false
	}
	return returnType, true
}

// importedReturnValue keeps an explicit any only when it is the sealed result
// type of an already-resolved module export.  Unlike an absent provider result
// (which remains Top), this is a concrete precision boundary published by the
// manifest and must remain visible to each later assignment contract.
func importedReturnValue(result typ.Type) ([]byte, bool) {
	if result != nil && unwrap.Alias(result).Kind() == kind.Any {
		return []byte("scalar/claim/claim-kind/3/\"any\""), true
	}
	return providerReturnTypeValue(result)
}

// instantiateProviderReturn unifies only closed argument types already
// published at this call boundary. It is deliberately structural and
// fail-closed: an incomplete generic match leaves the provider result Top.
// It serves both resolved module exports and standard-library providers.
func instantiateProviderReturn(function *typ.Function, arguments map[int][]byte, partition equation.Partition, index int) (typ.Type, bool) {
	if function == nil || index < 0 || index >= len(function.Returns) || function.Returns[index] == nil {
		return nil, false
	}
	if len(function.TypeParams) == 0 {
		return function.Returns[index], true
	}
	params := make(map[string]bool, len(function.TypeParams))
	for _, parameter := range function.TypeParams {
		if parameter == nil || parameter.Name == "" {
			return nil, false
		}
		params[parameter.Name] = true
	}
	bindings := make(map[string]typ.Type, len(params))
	for position, expected := range function.Params {
		argument, present := arguments[position]
		if !present || expected.Type == nil {
			continue
		}
		actual, decoded := publishedProviderArgumentType(argument, partition)
		if !decoded || !inferImportedTypeArgs(expected.Type, actual, params, bindings) {
			return nil, false
		}
	}
	for name := range params {
		if bindings[name] == nil {
			// A zero-argument generic constructor has no call-site evidence for
			// this parameter. Preserve the provider's exact return graph while
			// making only that unbound slot unknown; parameter-independent
			// members (for example Collection<T>.count) remain available, while
			// T-dependent members still cannot prove a concrete claim.
			bindings[name] = typ.Unknown
		}
	}
	return subst.Substitute(function.Returns[index], bindings), true
}

// publishedProviderArgumentType retains a type argument only when its exact
// term already owns a closed publication. Imported call results carry their
// instantiated summary through summaryTypePrefix, while checked annotations
// carry their target through the ordinary type publication. Both are guarded
// facts tied to this term's current epoch; an untyped literal still uses the
// existing sealed-shape decoder below. This lets a module generic result feed
// the next call without treating source annotations or an older write as a
// fresh witness.
func publishedProviderArgumentType(term []byte, partition equation.Partition) (typ.Type, bool) {
	if encoded, found := currentPublishedTermFact(summaryTypePrefix, term, partition); found {
		if summary, err := typ.DecodeCanonical(context.Background(), encoded); err == nil && closedProviderArgumentType(summary, 0) {
			return summary, true
		}
	}
	if encoded, found := currentPublishedTermFact("type/", term, partition); found {
		if checked, decoded := shapefact.DecodeTarget(encoded); decoded && closedProviderArgumentType(checked, 0) {
			return checked, true
		}
	}
	value, found := resolveKnownCurrentValue(term, partition)
	if !found {
		return nil, false
	}
	return providerArgumentType(value)
}

// currentPublishedTermFact returns a type fact only when it was published by
// the same operation as the term's newest value. A historical annotation must
// not survive a later reassignment, and an entry declaration is metadata, not
// a checked value witness.
func currentPublishedTermFact(prefix string, term []byte, partition equation.Partition) ([]byte, bool) {
	valuePrefix := "value/" + string(term) + "/"
	latest := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, valuePrefix) && fact.Key > latest {
			latest = fact.Key
		}
	}
	if latest == "" {
		return nil, false
	}
	operation := strings.TrimPrefix(latest, valuePrefix)
	if operation == "entry" {
		return nil, false
	}
	want := prefix + string(term) + "/" + operation
	for _, fact := range partition.Values() {
		if fact.Key == want {
			return append([]byte(nil), fact.Value...), true
		}
	}
	return nil, false
}

// closedProviderArgumentType refuses precision boundaries and unresolved
// binders anywhere in the carried graph. A recursive graph is likewise not a
// finite generic-call witness: the bounded walk fails closed rather than
// attempting to reconstitute a recursive type argument from an import edge.
func closedProviderArgumentType(value typ.Type, depth int) bool {
	if value == nil || depth > 64 {
		return false
	}
	value = unwrap.Alias(subst.ExpandInstantiated(value))
	if value == nil {
		return false
	}
	switch value.Kind() {
	case kind.Any, kind.Unknown, kind.Never, kind.TypeParam, kind.Generic, kind.Ref:
		return false
	}
	return !typ.WalkChildren(value, func(child typ.Type) bool {
		return !closedProviderArgumentType(child, depth+1)
	})
}

// providerArgumentType recovers a finite array witness from a sealed literal
// only when every present top-level member is an integer-indexed, exact value.
// This is the same evidence accepted by tableAgainstContainer; an open table,
// object field, nested member, absent entry, or unknown element fails closed.
func providerArgumentType(value []byte) (typ.Type, bool) {
	if target, ok := shapefact.DecodeTarget(value); ok {
		return target, true
	}
	if function, ok := sealedFunctionType(value); ok {
		return function, true
	}
	if scalar, ok := expressionValueType(value); ok {
		if literal, literalOK := scalar.(*typ.Literal); literalOK {
			switch literal.Base {
			case kind.Boolean:
				return typ.Boolean, true
			case kind.String:
				return typ.String, true
			case kind.Integer:
				return typ.Integer, true
			case kind.Number:
				return typ.Number, true
			}
		}
		return scalar, true
	}
	table, ok := shapefact.DecodeTable(value)
	if !ok || !table.Closed || len(table.Members) == 0 {
		return nil, false
	}
	elements := make([]typ.Type, 0, len(table.Members))
	for _, member := range table.Members {
		segments, valid := segment.ParseFormattedSegments(member.Suffix)
		if !valid || len(segments) != 1 || segments[0].Kind != segment.SegmentIndexInt || !member.Present {
			return nil, false
		}
		element, known := expressionValueType([]byte(member.Value))
		if !known {
			return nil, false
		}
		elements = append(elements, element)
	}
	if base, uniform := uniformLiteralBase(elements); uniform {
		return typ.NewArray(base), true
	}
	return typ.NewArray(typ.MaterializeUnion(elements)), true
}

// uniformLiteralBase widens only a homogeneous finite literal family. The
// literal shape remains the authority for the inference; widening to its
// common primitive merely expresses the array contract expected by a generic
// provider. Mixed or non-literal entries retain their exact union instead.
func uniformLiteralBase(values []typ.Type) (typ.Type, bool) {
	if len(values) == 0 {
		return nil, false
	}
	var base kind.Kind
	set := false
	for _, value := range values {
		literal, ok := value.(*typ.Literal)
		if !ok {
			return nil, false
		}
		if !set {
			base = literal.Base
			set = true
		} else if base != literal.Base {
			return nil, false
		}
	}
	switch base {
	case kind.Boolean:
		return typ.Boolean, true
	case kind.String:
		return typ.String, true
	case kind.Integer:
		return typ.Integer, true
	case kind.Number:
		return typ.Number, true
	default:
		return nil, false
	}
}

func inferImportedTypeArgs(expected, actual typ.Type, params map[string]bool, bindings map[string]typ.Type) bool {
	expected = unwrap.Alias(subst.ExpandInstantiated(expected))
	actual = unwrap.Alias(subst.ExpandInstantiated(actual))
	if expected == nil || actual == nil {
		return false
	}
	if parameter, ok := expected.(*typ.TypeParam); ok && params[parameter.Name] {
		if prior := bindings[parameter.Name]; prior != nil {
			// A later concrete argument may be a narrower inhabitant of an
			// already-bound generic parameter.  The binding itself remains the
			// published contract: accepting only an actual subtype cannot widen
			// it or manufacture a result witness.  This is needed for ordinary
			// reducers whose callback fixes A as number while their literal seed
			// is an integer.
			return typ.TypeEquals(prior, actual) || subtype.IsSubtype(actual, prior)
		}
		bindings[parameter.Name] = actual
		return true
	}
	switch want := expected.(type) {
	case *typ.Record:
		got, ok := actual.(*typ.Record)
		if !ok {
			return false
		}
		for _, field := range want.Fields {
			actualField := got.GetField(field.Name)
			if actualField == nil || field.Type == nil || !inferImportedTypeArgs(field.Type, actualField.Type, params, bindings) {
				return false
			}
		}
		return true
	case *typ.Function:
		got, ok := actual.(*typ.Function)
		if !ok || len(want.Params) != len(got.Params) || len(want.Returns) != len(got.Returns) {
			return false
		}
		for i := range want.Params {
			if parameter, parameterOK := unwrap.Alias(subst.ExpandInstantiated(want.Params[i].Type)).(*typ.TypeParam); parameterOK && params[parameter.Name] {
				if bound := bindings[parameter.Name]; bound != nil {
					// A callback parameter is contravariant: once an earlier
					// argument has fixed T, its callback may accept T itself or
					// any established supertype, but never a narrower type.
					if !subtype.IsSubtype(bound, got.Params[i].Type) {
						return false
					}
					continue
				}
			}
			if !inferImportedTypeArgs(want.Params[i].Type, got.Params[i].Type, params, bindings) {
				return false
			}
		}
		for i := range want.Returns {
			if !inferImportedTypeArgs(want.Returns[i], got.Returns[i], params, bindings) {
				return false
			}
		}
		return true
	case *typ.Array:
		got, ok := actual.(*typ.Array)
		return ok && inferImportedTypeArgs(want.Element, got.Element, params, bindings)
	case *typ.Optional:
		got, ok := actual.(*typ.Optional)
		return ok && inferImportedTypeArgs(want.Inner, got.Inner, params, bindings)
	default:
		return typ.TypeEquals(expected, actual)
	}
}

type moduleProviderWire struct {
	Module string `json:"module"`
	Suffix string `json:"suffix,omitempty"`
}

func importedProviderTarget(provider []byte) (modulePath, suffix string, requireResult, ok bool) {
	encoded := strings.TrimPrefix(string(provider), "provider/module-load/")
	if encoded != string(provider) && encoded != "" {
		modulePath, err := strconv.Unquote(encoded)
		return modulePath, "", true, err == nil && modulePath != ""
	}
	encoded = strings.TrimPrefix(string(provider), "provider/module/v1/")
	if encoded == string(provider) || encoded == "" {
		return "", "", false, false
	}
	wired, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false, false
	}
	var wire moduleProviderWire
	if json.Unmarshal(wired, &wire) != nil || wire.Module == "" {
		return "", "", false, false
	}
	canonical, marshalErr := json.Marshal(wire)
	if marshalErr != nil || string(canonical) != string(wired) {
		return "", "", false, false
	}
	return wire.Module, wire.Suffix, false, true
}

func importedEntryType(modulePath string, partition equation.Partition) (typ.Type, bool) {
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
	return shapefact.DecodeTarget(value)
}

func providerReturnTypeValue(result typ.Type) ([]byte, bool) {
	if result == nil {
		return nil, false
	}
	// The value lattice can carry an exact declared shape, including an
	// optional result. The optional remains a value-level union with nil, so it
	// cannot prove a non-optional claim. Unresolved generic parameters still
	// have no concrete runtime witness and remain Top.
	switch unwrap.Alias(result).Kind() {
	case kind.Any, kind.Unknown, kind.Never, kind.Union, kind.TypeParam:
		return nil, false
	}
	return shapefact.EncodeTarget(result)
}

// optionalCallableResultValue reads a declared optional result slot from the
// callee's own current sealed function value. The call result itself remains
// Top: this witness records only that the callee's published contract admits
// nil at that slot, which is exactly what a consumer's nil obligation needs.
// A captured callable carries this contract into a child body even where the
// closure capability that would materialize its body was not transported.
func optionalCallableResultValue(callee []byte, index int, partition equation.Partition) ([]byte, bool) {
	if callee == nil || index < 0 {
		return nil, false
	}
	value, err := resolveCurrentValue(callee, partition)
	if err != nil {
		return nil, false
	}
	functionType, ok := sealedFunctionType(value)
	if !ok {
		return nil, false
	}
	function, ok := unwrap.Alias(subst.ExpandInstantiated(functionType)).(*typ.Function)
	if !ok || function == nil || len(function.TypeParams) != 0 || index >= len(function.Returns) {
		return nil, false
	}
	result := function.Returns[index]
	if !finiteReturnWitness(result, make(map[typ.Type]bool)) || !unwrap.IsOptionalLike(result) {
		return nil, false
	}
	return shapefact.EncodeTarget(result)
}

// optionalResultOrigin names the callee slot that published an optional
// result. The name comes from the call-result operation's own source display,
// so a temporary result term can still be explained by the call that produced
// it. A call with no authored display has no origin to report.
func optionalResultOrigin(operation equation.BoundEquation, index int) (string, bool) {
	for _, operand := range operation.Operands {
		if operand.Role != "result-display" || len(operand.Value) == 0 {
			continue
		}
		callee := strings.TrimSuffix(string(operand.Value), "(...)")
		if callee == "" || callee == string(operand.Value) {
			return "", false
		}
		return fmt.Sprintf("%s return %d", callee, index+1), true
	}
	return "", false
}

func optionalProviderResultValue(provider string, index int) ([]byte, bool) {
	if provider == "" || index < 0 {
		return nil, false
	}
	result, ok := signaturelookup.StdlibResultSlot(provider, index)
	if !ok || !finiteReturnWitness(result, make(map[typ.Type]bool)) || !unwrap.IsOptionalLike(result) {
		return nil, false
	}
	return shapefact.EncodeTarget(result)
}

// providerAnyResult identifies a declared finite result slot whose canonical
// standard-library contract is an explicit any boundary. The call-result value
// remains Top, while this separate fact preserves the registry's requirement
// that a later typed assignment validate the boundary.
func providerAnyResult(provider string, index, resultArity int) bool {
	result, found := signaturelookup.StdlibResultSlot(provider, index)
	if found {
		return result != nil && unwrap.Alias(result).Kind() == kind.Any
	}
	// A one-slot any declaration is the registry's conservative description of
	// an open Lua result tail. Only an expanded call result may carry that tail
	// to later slots; a fixed one-slot call remains governed by the exact slot.
	if resultArity <= 1 || index <= 0 {
		return false
	}
	first, firstFound := signaturelookup.StdlibResultSlot(provider, 0)
	_, secondFound := signaturelookup.StdlibResultSlot(provider, 1)
	return firstFound && !secondFound && first != nil && unwrap.Alias(first).Kind() == kind.Any
}

func providerAnyResultBoundary(term []byte, partition equation.Partition) bool {
	_, found := currentEpochFact("provider-any-result/", term, partition)
	return found
}

func finiteReturnWitnessValue(result typ.Type) ([]byte, bool) {
	if result == nil || !finiteReturnWitness(result, make(map[typ.Type]bool)) {
		return nil, false
	}
	return shapefact.EncodeTarget(result)
}

// finiteReturnWitness admits only a declared, finite slot that can retain
// nilability without standing in for an unbounded union, generic parameter, or
// explicit any result. A union is accepted solely when it is an optional
// witness: every member is concrete and one member is nil.
func finiteReturnWitness(result typ.Type, seen map[typ.Type]bool) bool {
	if typ.ContainsAny(result) || typ.ContainsTypeParam(result) {
		return false
	}
	result = unwrap.Alias(subst.ExpandInstantiated(result))
	if result == nil || seen[result] {
		return result != nil
	}
	seen[result] = true
	switch value := result.(type) {
	case *typ.Optional:
		return value != nil && finiteReturnWitness(value.Inner, seen)
	case *typ.Union:
		if value == nil || len(value.Members) == 0 {
			return false
		}
		hasNil := false
		for _, member := range value.Members {
			if member == nil {
				return false
			}
			hasNil = hasNil || unwrap.Alias(member).Kind() == kind.Nil
			if !finiteReturnWitness(member, seen) {
				return false
			}
		}
		return hasNil
	}
	switch result.Kind() {
	case kind.Any, kind.Unknown, kind.Never, kind.TypeParam, kind.Union:
		return false
	default:
		return true
	}
}

// returnValueSubject names one return slot. The authored spelling is added
// when the returned expression has one, so the reader sees both the slot and
// the expression that filled it.
func returnValueSubject(index int, display string) string {
	if display == "" {
		return fmt.Sprintf("returned value %d", index+1)
	}
	return fmt.Sprintf("returned value %d (%s)", index+1, display)
}

// indexedRoleValue decodes an operand whose role carries a slot index.
func indexedRoleValue(role, prefix string, value []byte) (int, string, bool) {
	suffix, ok := strings.CutPrefix(role, prefix)
	if !ok {
		return 0, "", false
	}
	index, err := strconv.Atoi(suffix)
	if err != nil || index < 0 {
		return 0, "", false
	}
	return index, string(value), true
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
	values := make([][]byte, 0, len(operation.Operands))
	declared := make(map[int]typ.Type)
	displays := make(map[int]string)
	for _, operand := range operation.Operands {
		const prefix = "return-value-"
		if index, display, ok := indexedRoleValue(operand.Role, "return-display-", operand.Value); ok {
			displays[index] = display
			continue
		}
		if strings.HasPrefix(operand.Role, "declared-return-") {
			indexText := strings.TrimPrefix(operand.Role, "declared-return-")
			index, err := strconv.Atoi(indexText)
			if err != nil || index < 0 {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed declared return operand role %q", operand.Role)
			}
			declaredType, ok := shapefact.DecodeTarget(operand.Value)
			if !ok || declaredType == nil {
				// A generic declaration may have a canonical type-parameter edge
				// whose binder is intentionally unavailable to a structural value
				// fact. The declaration cannot prove or refute a runtime return in
				// that case, but it must not invalidate the independently closed
				// return tuple. Omit only that optional contract check.
				continue
			}
			declared[index] = declaredType
			continue
		}
		if !strings.HasPrefix(operand.Role, prefix) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		indexText := strings.TrimPrefix(operand.Role, prefix)
		if len(indexText) != 8 {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		index, err := strconv.Atoi(indexText)
		if err != nil || index < 0 || (index < len(values) && values[index] != nil) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		for len(values) <= index {
			values = append(values, nil)
		}
		value, err := resolveCurrentValue(operand.Value, partition)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		// A return leaves the partition that owns this body's member writes, so
		// the returned value consumes the heap member authority here rather than
		// handing the caller the allocation-time template.
		values[index] = heapMemberSurface(operand.Value, value, partition)
	}
	for index, value := range values {
		if value == nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: missing return value %d", index)
		}
	}
	diagnostics := make([]equation.Fact, 0)
	for index, expected := range declared {
		if index >= len(values) {
			continue
		}
		if valueAgainstType(values[index], expected) != shapeRefuted {
			continue
		}
		subject := returnValueSubject(index, displays[index])
		message := fmt.Sprintf("%s is %s, not %s", subject, assignmentEvidenceValue(values[index]), typeformat.Short(expected))
		if optionalConcreteWitness(values[index]) && valueAgainstType([]byte("scalar/nil"), expected) == shapeRefuted {
			message = fmt.Sprintf("%s may be nil, not %s", subject, typeformat.Short(expected))
		}
		diagnostics = append(diagnostics, equation.Fact{
			Key:   fmt.Sprintf("type.return.contract/%s/%08d", operation.Target.Name, index),
			Value: []byte(message),
		})
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
		source := boundOperandValue(operation.Operands, fmt.Sprintf("return-value-%08d", index))
		for memberIndex, wire := range returnMemberClosures(source, partition) {
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
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: projected, Outcomes: outcomes, Diagnostics: diagnostics}}, nil
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
			if operand.Role != "predicate-display" && operand.Role != "index-presence-consumer" && !strings.HasPrefix(operand.Role, "implied-") && !strings.HasPrefix(operand.Role, "sufficient-") && !strings.HasPrefix(operand.Role, "difference-") {
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
	return evaluateBranchPredicateWire(predicate, partition)
}

func evaluateBranchPredicateWire(predicate branchPredicateWire, partition equation.Partition) (bool, error) {
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
	case "nil", "not-nil":
		result, err = scalarIsNil(value)
		if predicate.Kind == "not-nil" {
			result = !result
		}
	case "literal-equal", "literal-not":
		if predicate.Literal == "" {
			return false, fmt.Errorf("engine: literal predicate has no literal")
		}
		result, err = scalarEqualsEncoding(value, []byte(predicate.Literal))
		if predicate.Kind == "literal-not" {
			result = !result
		}
	case "path-equal", "path-not":
		other, valueErr := branchPathValue(predicate.OtherPath, partition)
		if valueErr != nil {
			return false, valueErr
		}
		result, err = scalarEqualsEncoding(value, other)
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

// scalarIsNil answers a nil/not-nil predicate from a current value. A published
// type witness is not a runtime value: it answers the predicate only when its
// whole value set is nil or its whole value set excludes nil. A type that
// admits both leaves the predicate unavailable, so the branch keeps both edges
// and the narrowing lane owns the refinement.
func scalarIsNil(value []byte) (bool, error) {
	if isUnknownScalar(value) {
		return false, errUnknownScalar
	}
	if target, ok := shapefact.DecodeTarget(value); ok {
		if target == nil || typ.IsTopLike(target) || typ.IsNever(target) {
			return false, errUnknownScalar
		}
		present := proof.ProjectionWithoutNil(target)
		hasValue := present != nil && !typ.IsNever(present)
		switch {
		case !typevalue.ProjectionHasNil(target) && hasValue:
			return false, nil
		case typevalue.ProjectionHasNil(target) && !hasValue:
			return true, nil
		default:
			return false, errUnknownScalar
		}
	}
	return string(value) == "scalar/nil", nil
}

// scalarEqualsEncoding compares two current values for runtime equality. Two
// scalar encodings denote single values, so byte equality is value equality. A
// type witness denotes a set: it proves equality only when both sides are the
// same single-value literal, and proves nothing otherwise.
func scalarEqualsEncoding(value, other []byte) (bool, error) {
	if isUnknownScalar(value) || isUnknownScalar(other) {
		return false, errUnknownScalar
	}
	valueTarget, valueIsTarget := shapefact.DecodeTarget(value)
	otherTarget, otherIsTarget := shapefact.DecodeTarget(other)
	if !valueIsTarget && !otherIsTarget {
		return string(value) == string(other), nil
	}
	left, leftOK := singleValueWitness(value, valueTarget, valueIsTarget)
	right, rightOK := singleValueWitness(other, otherTarget, otherIsTarget)
	if !leftOK || !rightOK {
		return false, errUnknownScalar
	}
	return left == right, nil
}

// singleValueWitness reduces a current value to the one runtime value it
// denotes. A scalar literal encoding and a literal type both denote exactly one
// value; every wider type denotes more than one and has no single witness.
func singleValueWitness(value []byte, target typ.Type, isTarget bool) (string, bool) {
	if !isTarget {
		if strings.HasPrefix(string(value), "scalar/") && !isUnknownScalar(value) {
			return string(value), true
		}
		return "", false
	}
	resolved := unwrap.Alias(target)
	if resolved == nil {
		return "", false
	}
	if resolved.Kind() == kind.Nil {
		return "scalar/nil", true
	}
	literal, ok := resolved.(*typ.Literal)
	if !ok {
		return "", false
	}
	switch text := literal.Value.(type) {
	case string:
		return "scalar/string/" + strconv.Quote(text), true
	case bool:
		return "scalar/bool/" + strconv.FormatBool(text), true
	case int64:
		return "scalar/number/" + strconv.FormatInt(text, 10), true
	case float64:
		return "scalar/number/" + strconv.FormatFloat(text, 'g', -1, 64), true
	default:
		return "", false
	}
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
	// A closed summary can prove that a selected member is absent, but that
	// absence marker is not a runtime scalar. Predicates must leave it
	// unselected rather than turning an unavailable proof into an evaluator
	// failure.
	if memberMissing(value) {
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
		// Lua's length operator applies to tables as well as strings.  The
		// scalar domain has no table-cardinality value, so an array length is
		// unavailable here rather than a malformed string.  Keep the branch
		// fail-closed: callers turn this into an unselected, unproven edge.
		if !strings.HasPrefix(string(value), "scalar/string/") {
			return 0, errUnknownScalar
		}
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

// boundOperandValue selects one bound operand by role. Operand order is
// presentation-dependent, so every slot lookup goes through its role.
func boundOperandValue(operands []equation.BoundOperand, role string) []byte {
	for _, operand := range operands {
		if operand.Role == role {
			return operand.Value
		}
	}
	return nil
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
	if value, found := declaredOptionalMapReadValue(term, partition); found {
		return value, nil
	}
	if value, found := heapMemberValue(term, partition); found {
		return value, nil
	}
	if value, found := assertionNarrowedValue(term, cutoff, partition); found {
		return value, nil
	}
	if value, found := provenSequenceIndexValue(term, partition); found {
		return value, nil
	}
	if value, found := typedPathValue(term, partition); found {
		return value, nil
	}
	if value, found := reconvergedValue(term, cutoff, partition); found {
		return value, nil
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

// assertionNarrowedValue reads a completed assert postcondition. Its operation
// coordinate is compared with ordinary value publications, so the proof lasts
// only until a later write to the same path and only through the caller's
// explicit read boundary. Partition.Values keeps a guarded postcondition
// private to the guard partition that established it.
func assertionNarrowedValue(term []byte, before string, partition equation.Partition) ([]byte, bool) {
	assertionPrefix := "assertion-value/" + string(term) + "/"
	valuePrefix := "value/" + string(term) + "/"
	assertion, assertionPoint := []byte(nil), ""
	latestValuePoint := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, assertionPrefix) {
			point := strings.TrimPrefix(fact.Key, assertionPrefix)
			if point != "" && (before == "" || point <= before) && point > assertionPoint {
				assertion, assertionPoint = fact.Value, point
			}
			continue
		}
		if strings.HasPrefix(fact.Key, valuePrefix) {
			point := strings.TrimPrefix(fact.Key, valuePrefix)
			if point != "" && (before == "" || point <= before) && point > latestValuePoint {
				latestValuePoint = point
			}
		}
	}
	if assertion == nil || latestValuePoint > assertionPoint {
		return nil, false
	}
	return append([]byte(nil), assertion...), true
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
	if value, found := correlationConeCurrentValue(term, partition); found {
		return value, nil
	}
	if value, found := declaredOptionalMapReadValue(term, partition); found {
		return value, nil
	}
	if value, found := heapMemberValue(term, partition); found {
		return value, nil
	}
	if value, found := assertionNarrowedValue(term, "", partition); found {
		return value, nil
	}
	if value, found := provenSequenceIndexValue(term, partition); found {
		return value, nil
	}
	if value, found := reconvergedValue(term, "", partition); found {
		return value, nil
	}
	if value, found := typedPathValue(term, partition); found {
		return value, nil
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

// correlationConeCurrentValue selects only a currently valid guarded
// correlation projection.  A malformed row and a stale row are both ignored:
// neither can become an ordinary value witness.  This is intentionally a read
// filter rather than a broad invalidation pass, so a write to one path revokes
// only cones that recorded that exact path in their published proof boundary.
func correlationConeCurrentValue(term []byte, partition equation.Partition) ([]byte, bool) {
	if !derivedPathTerm(term) {
		return nil, false
	}
	prefix := correlationConeValuePrefix + base64.RawURLEncoding.EncodeToString(term) + "/"
	var value []byte
	latest := ""
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, prefix) || fact.Key <= latest {
			continue
		}
		var wire correlationConeValue
		if json.Unmarshal(fact.Value, &wire) != nil || len(wire.Value) == 0 || len(wire.Epochs) == 0 || !correlationConeEpochsCurrent(wire.Epochs, partition) {
			continue
		}
		if _, decoded := shapefact.DecodeTarget(wire.Value); !decoded {
			continue
		}
		value, latest = append([]byte(nil), wire.Value...), fact.Key
	}
	return value, value != nil
}

func correlationConeEpochsCurrent(epochs []correlationConeEpoch, partition equation.Partition) bool {
	seen := make(map[string]bool, len(epochs))
	for _, item := range epochs {
		if item.Term == "" || seen[item.Term] || (!strings.HasPrefix(item.Term, "path/") && !strings.HasPrefix(item.Term, "temp/")) {
			return false
		}
		seen[item.Term] = true
		current, _ := currentEpoch([]byte(item.Term), partition)
		if current != item.Epoch {
			return false
		}
	}
	return true
}

// reconvergedValue is the single value-lane read for a term.  Inside a
// partition whose guards already decide every branch the latest write depends
// on it is the ordinary latest publication.  Past such a branch it is the
// control-flow join the evaluator computes over that decision's edges, so an
// arm write reaches its post-dominator as a union with the value the other edge
// carries rather than staying private to the arm.
//
// cutoff is the caller's read boundary: an empty boundary reads the current
// value, and a non-empty one admits only publications at or before it.  The
// boundary is applied inside every edge, so each edge resolves its own current
// value with the same timing rule.
func reconvergedValue(term []byte, cutoff string, partition equation.Partition) ([]byte, bool) {
	prefix := "value/" + string(term) + "/"
	fact, found := partition.Reconverged(prefix, equation.Reconvergence{
		Current: func(candidates []equation.Fact) (equation.Fact, bool) {
			if cutoff == "" {
				return latestPublication(candidates)
			}
			bounded := make([]equation.Fact, 0, len(candidates))
			for _, candidate := range candidates {
				if strings.TrimPrefix(candidate.Key, prefix) <= cutoff {
					bounded = append(bounded, candidate)
				}
			}
			return latestPublication(bounded)
		},
		Join: joinPublishedValues,
	})
	if !found {
		return nil, false
	}
	return fact.Value, true
}

// joinPublishedValues is the value lattice at a reconvergence point.  Edges
// that published the same value keep it exactly.  Otherwise the point holds the
// union of the edge witnesses, and a payload that contributes no witness widens
// to the unknown scalar: one edge must never speak for a point both edges
// reach.
func joinPublishedValues(left, right []byte) ([]byte, bool) {
	if bytes.Equal(left, right) {
		return append([]byte(nil), left...), true
	}
	if isUnknownScalar(left) || isUnknownScalar(right) {
		return []byte("scalar/top"), true
	}
	leftWitness, leftKnown := joinedValueWitness(left)
	rightWitness, rightKnown := joinedValueWitness(right)
	if !leftKnown || !rightKnown {
		return []byte("scalar/top"), true
	}
	encoded, ok := shapefact.EncodeTarget(normalize.UnionForEvidence(leftWitness, rightWitness))
	if !ok {
		return []byte("scalar/top"), true
	}
	return encoded, true
}

// joinedValueWitness is the type a published payload contributes to a join. A
// sealed literal table contributes its recorded structure, so joining two arm
// tables keeps their members instead of collapsing the point to a broad table.
func joinedValueWitness(value []byte) (typ.Type, bool) {
	if witness, ok := scalarWitnessType(value); ok {
		return witness, true
	}
	return sealedShapeReceiverType(value)
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
		if identity, tracked := tableIdentityForTerm([]byte("path/"+ancestor), partition); tracked && heapHasExternalCallback(identity, partition) {
			return nil, false
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

// selectConstrainedArms composes every select constraint that holds in this
// partition. Each constraint states that the winning arm lies inside its own
// arm set, so successive guarded edges compose by intersection: an outer
// `result.channel ~= a` false edge and an inner `result.channel == b` true edge
// together prove arm `b`, not their union. Composition is what carries the
// proof across nested selects and child bodies, where several edges of the same
// select are live at once. An empty intersection is a contradictory partition
// and yields no payload rather than a widened one.
func selectConstrainedArms(selectID []byte, cases int, partition equation.Partition) (map[int]bool, bool) {
	arms := make(map[int]bool, cases)
	for arm := 0; arm < cases; arm++ {
		arms[arm] = true
	}
	for _, fact := range partition.Values() {
		if !strings.HasPrefix(fact.Key, "select/constraint/") {
			continue
		}
		var constraint selectConstraintWire
		if json.Unmarshal(fact.Value, &constraint) != nil || constraint.Select != string(selectID) || constraint.Default {
			continue
		}
		allowed := make(map[int]bool, len(constraint.Arms))
		for _, arm := range constraint.Arms {
			if arm >= 0 && arm < cases {
				allowed[arm] = true
			}
		}
		for arm := range arms {
			if !allowed[arm] {
				delete(arms, arm)
			}
		}
	}
	if len(arms) == 0 {
		return nil, false
	}
	return arms, true
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
	arms, ok := selectConstrainedArms(selectID, meta.Cases, partition)
	if !ok {
		return nil, false
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
		// The arm payload is the select kernel's own closed publication of a
		// declared Channel<T> argument, so it is read back with the structural
		// decoder that the rest of the shape-fact family uses. A recursive
		// payload type carries no declaration identity through canonical bytes;
		// requiring one here would silently drop the arm and widen the read.
		payload, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
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
	// An optional receiver reached inside the path is named by no term, so no
	// publication can carry its presence: the member surface walk looks through
	// it while the segment walk keeps its nilability, and the nilable answer is
	// the one this static projection may publish. The ancestor root is excluded
	// because its own current value is the authority every guard, assertion, and
	// correlation publishes against, and this whole projection is consulted only
	// when no such current publication exists.
	if found && !optionalConcreteWitnessType(source) && !optionalConcreteWitnessType(field) {
		if projected, ok := typedPathSegments(source, suffix); ok && optionalConcreteWitnessType(projected) {
			field = projected
		}
	}
	if !found && methodReturnSummaryAncestor(root, partition) {
		// A closed union can omit a field on some arms. The projection helper
		// retains that absence as nil, whereas FieldAtPath intentionally refuses
		// partial members for branch-dispatch decisions. The method summary is
		// the existing current fact that authorizes this consumer projection.
		field, found = luatypeprojection.ApplySegments(source, suffix)
	}
	// Preserve an optional receiver across a static projection only when its
	// exact current call-result publication carries the sealed type summary.
	// This includes local callable results and resolved imported/method results;
	// arbitrary annotations and stale summaries remain unavailable.
	if optionalConcreteWitnessType(source) && currentCallResultSummary(root, partition) {
		if projected, projectedFound := typedPathSegments(source, suffix); projectedFound && methodSummaryComposable(projected, make(map[typ.Type]bool)) {
			field, found = projected, true
		}
	}
	if !found && hasIndexSegment(suffix) {
		if indexed, indexedFound := typedPathSegments(source, suffix); indexedFound && optionalConcreteWitnessType(indexed) {
			field, found = indexed, true
		}
	}
	if !found {
		// A summary union is a contract over several possible method results,
		// not proof that every member path exists at runtime. Leave an
		// unselected arm Top so only the established branch machinery can make
		// it concrete; emitting a missing-member fact here would reject a valid
		// discriminant-guarded consumer.
		if _, summary := currentEpochFact(summaryTypePrefix, root, partition); summary {
			if _, union := unwrap.Alias(subst.ExpandInstantiated(source)).(*typ.Union); union {
				if projected, projectedFound := luatypeprojection.ApplySegments(source, suffix); projectedFound {
					return shapefact.EncodeTarget(projected)
				}
				return []byte("scalar/top"), true
			}
		}
		if !closedMemberSurface(source) {
			return []byte("scalar/top"), true
		}
		return memberMissingValue(source)
	}
	value, ok := shapefact.EncodeTarget(field)
	return value, ok
}

func currentCallResultSummary(term []byte, partition equation.Partition) bool {
	return localCallableResultAncestor(term, partition) || methodReturnSummaryAncestor(term, partition)
}

// hasCurrentSummaryFact is the rich-diagnostic counterpart of
// currentEpochFact. Diagnostic projection receives a closed value slice rather
// than a partition, but it must use the same current-epoch rule before
// replacing a generated temporary with its call-site display.
func hasCurrentSummaryFact(prefix string, term []byte, facts []equation.Fact) bool {
	valuePrefix := "value/" + string(term) + "/"
	latest := ""
	for _, fact := range facts {
		if strings.HasPrefix(fact.Key, valuePrefix) && fact.Key > latest {
			latest = fact.Key
		}
	}
	if latest == "" {
		return false
	}
	operation := strings.TrimPrefix(latest, valuePrefix)
	for _, fact := range facts {
		if fact.Key == prefix+string(term)+"/"+operation {
			return true
		}
	}
	return false
}

// typedPathSegments walks a sealed static path one segment at a time so an
// indexed read can use the existing RuntimeIndex relation. Any optional
// receiver is projected only for that step and then restored in the result;
// no path access manufactures presence proof.
func typedPathSegments(source typ.Type, suffix []segment.Segment) (typ.Type, bool) {
	current := source
	optional := false
	for _, item := range suffix {
		if optionalConcreteWitnessType(current) {
			current = proof.ProjectionWithoutNil(current)
			optional = true
		}
		if current == nil {
			return nil, false
		}
		var next typ.Type
		var found bool
		switch item.Kind {
		case segment.SegmentField:
			next, found = variant.FieldAtPath(current, []segment.Segment{item})
		case segment.SegmentIndexString:
			if !closedSequence(current) {
				return nil, false
			}
			next, found = access.RuntimeIndex(current, typ.LiteralString(item.Name))
		case segment.SegmentIndexInt:
			if !closedSequence(current) {
				return nil, false
			}
			next, found = access.RuntimeIndex(current, typ.LiteralNumber(float64(item.Index)))
		default:
			return nil, false
		}
		if !found || next == nil {
			return nil, false
		}
		current = next
	}
	if optional {
		current = typ.MaterializeOptional(current)
	}
	return current, true
}

func closedSequence(value typ.Type) bool {
	value = unwrap.Alias(subst.ExpandInstantiated(value))
	switch value.(type) {
	case *typ.Array, *typ.Tuple:
		return true
	default:
		return false
	}
}

func hasIndexSegment(items []segment.Segment) bool {
	for _, item := range items {
		switch item.Kind {
		case segment.SegmentIndexString, segment.SegmentIndexInt:
			return true
		}
	}
	return false
}

func derivedIndexedPath(term []byte) bool {
	return strings.HasPrefix(string(term), "path/") && strings.Contains(string(term), "[")
}

func closedMemberSurface(value typ.Type) bool {
	value = unwrap.Alias(subst.ExpandInstantiated(value))
	switch item := value.(type) {
	case *typ.Record:
		// A map component types every key in its domain, so those keys are
		// entries the record admits rather than members it refutes. Only the
		// named surface is closed, and a record carrying one is not.
		return item != nil && !item.Open && !item.HasMapComponent()
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

// declaredMethodMissing admits a missing-method diagnostic only when the
// declared receiver is a closed surface and neither the ordinary member graph
// nor the standard-library contract publishes the requested capability.  This
// keeps open, interface, and unknown receivers fail-closed while treating a
// mixed primitive union (such as string | number) as unavailable whenever its
// members do not share the same published method provider.
func declaredMethodMissing(receiver typ.Type, method string) bool {
	if receiver == nil || method == "" {
		return false
	}
	if _, available := signaturelookup.StdlibMethodProvider(receiver, method); available {
		return false
	}
	if _, available := variant.FieldAtPath(receiver, []segment.Segment{{Kind: segment.SegmentField, Name: method}}); available {
		return false
	}
	return closedMethodSurface(receiver)
}

func closedMethodSurface(value typ.Type) bool {
	value = unwrap.Alias(subst.ExpandInstantiated(value))
	switch item := value.(type) {
	case *typ.Record:
		return item != nil && !item.Open
	case *typ.Union:
		if item == nil || len(item.Members) == 0 {
			return false
		}
		for _, member := range item.Members {
			if !closedMethodSurface(member) {
				return false
			}
		}
		return true
	default:
		switch value.Kind() {
		case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
			return true
		default:
			return false
		}
	}
}

// decodeSummaryType reconstructs a published summary type. A summary is a
// closed structural narrowing hint, not a declaration-identity authority, so a
// recursive graph (for example an imported recursive union alias) is restored
// through the structural decoder. The strict decoder is tried first: it keeps
// the exact-representative round-trip guarantee for every non-recursive
// summary, and only its explicit recursive-identity signal falls through.
func decodeSummaryType(encoded []byte) (typ.Type, error) {
	decoded, err := typ.DecodeCanonical(context.Background(), encoded)
	if err != nil && errors.Is(err, typ.ErrCanonicalRecursiveIdentityUnavailable) {
		return typ.DecodeCanonicalStructural(context.Background(), encoded)
	}
	return decoded, err
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
		rootTerm := []byte("path/" + root)
		// A guarded branch publication is the current value on that edge and
		// therefore takes precedence over the broader module summary. This is
		// what lets an existing discriminant proof select one union arm without
		// discarding the summary on the opposite edge.
		if value, found := latestValue(rootTerm, partition); found {
			if typeValue, decoded := shapefact.DecodeTarget(value); decoded {
				return rootTerm, segs, typeValue, true
			}
		}
		// `assert` publishes the exact non-nil postcondition under the current
		// partition. It is a presence proof for this root just like a guarded
		// branch value, and is revoked by a later write in assertionNarrowedValue.
		if value, found := assertionNarrowedValue(rootTerm, "", partition); found {
			if typeValue, decoded := shapefact.DecodeTarget(value); decoded {
				return rootTerm, segs, typeValue, true
			}
		}
		if encoded, found := currentEpochFact(summaryTypePrefix, rootTerm, partition); found {
			typeValue, decodeErr := decodeSummaryType(encoded)
			if decodeErr == nil && typeValue != nil {
				return rootTerm, segs, typeValue, true
			}
		}
		if typeValue, declared := declaredTypeForTerm(rootTerm, partition); declared && optionalConcreteWitnessType(typeValue) {
			// A sealed table publication is this root's current runtime value and
			// proves its presence, so the declaration's nilability is not what a
			// member read descends through.
			if value, found := latestValue(rootTerm, partition); !found || !shapefact.IsTable(value) {
				return rootTerm, segs, typeValue, true
			}
		}
		value, found := latestValue(rootTerm, partition)
		if !found {
			value, found = selectPayloadValue(rootTerm, partition)
			if !found {
				continue
			}
		}
		typeValue, decoded := shapefact.DecodeTarget(value)
		if !decoded {
			continue
		}
		return rootTerm, segs, typeValue, true
	}
	return nil, nil, nil, false
}

// summaryPartialProjection recognizes an import summary that cannot prove a
// requested member on every arm. It is an explicit absence of a boundary
// proof, not a missing-member fact: a local guard may still establish the arm
// before a later read. Keeping it unpublished preserves the ordinary value
// path's fail-closed semantics.

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
	if bracket := strings.LastIndex(member, "["); bracket >= 0 {
		member = strings.Trim(strings.TrimSuffix(member[bracket+1:], "]"), "\"")
	}
	return fmt.Sprintf("%s has no member %q", typeformat.Short(receiver), member)
}

func latestValue(term []byte, partition equation.Partition) ([]byte, bool) {
	return reconvergedValue(term, "", partition)
}

func luaTruthy(value []byte) (bool, error) {
	if isUnknownScalar(value) {
		return false, errUnknownScalar
	}
	if memberMissing(value) {
		return false, errUnknownScalar
	}
	if target, ok := shapefact.DecodeTarget(value); ok {
		// A published type is not a value. It decides a condition only when one
		// side of Lua's falsy partition is empty; a type that holds both truthy
		// and falsy values -- boolean above all -- keeps both edges reachable.
		truthy, falsy, split := proof.TruthinessSplit(target)
		if !split {
			return false, errUnknownScalar
		}
		if typ.IsNever(falsy) {
			return true, nil
		}
		if typ.IsNever(truthy) {
			return false, nil
		}
		return false, errUnknownScalar
	}
	if shapefact.IsTable(value) {
		return true, nil
	}
	switch string(value) {
	case "scalar/boolean", optionalNilComparison:
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

// scalarWitnessType resolves a published engine value to the type whose value
// set it witnesses. It is the single decoder used wherever a stored value has
// to be compared in the type domain instead of by its encoding. An unknown
// scalar, a local claim, and a table shape carry no such type.
func scalarWitnessType(value []byte) (typ.Type, bool) {
	if target, ok := shapefact.DecodeTarget(value); ok && target != nil {
		return target, true
	}
	encoded := string(value)
	switch encoded {
	case "scalar/nil":
		return typ.Nil, true
	case "scalar/boolean":
		return typ.Boolean, true
	case "scalar/bool/true":
		return typ.True, true
	case "scalar/bool/false":
		return typ.False, true
	}
	switch {
	case strings.HasPrefix(encoded, "scalar/number/"):
		text := strings.TrimPrefix(encoded, "scalar/number/")
		if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
			return typ.LiteralInt(integer), true
		}
		if number, err := strconv.ParseFloat(text, 64); err == nil {
			return typ.LiteralNumber(number), true
		}
		return nil, false
	case strings.HasPrefix(encoded, "scalar/string/"):
		text, err := strconv.Unquote(strings.TrimPrefix(encoded, "scalar/string/"))
		if err != nil {
			return nil, false
		}
		return typ.LiteralString(text), true
	}
	return nil, false
}

// undecidedLogicalValue joins the two outcomes of a short-circuit whose left
// operand's truth is not decided: the surviving projection of the left operand
// and the whole right operand. A side with no type witness leaves the result
// unknown rather than committing the expression to one arm.
func undecidedLogicalValue(left, right []byte, operator wir.Operator) []byte {
	leftType, leftKnown := scalarWitnessType(left)
	rightType, rightKnown := scalarWitnessType(right)
	if !leftKnown || !rightKnown {
		return []byte("scalar/top")
	}
	truthy, falsy, split := proof.TruthinessSplit(leftType)
	if !split {
		return []byte("scalar/top")
	}
	survivor := falsy
	if operator == wir.LogOr {
		survivor = truthy
	}
	members := []typ.Type{rightType}
	if !typ.IsNever(survivor) {
		members = append([]typ.Type{survivor}, members...)
	}
	encoded, ok := shapefact.EncodeTarget(normalize.UnionForEvidence(members...))
	if !ok {
		return []byte("scalar/top")
	}
	return encoded
}

func isUnknownScalar(value []byte) bool {
	return string(value) == "scalar/top" || strings.HasPrefix(string(value), "scalar/claim/")
}

func derivedPathTerm(term []byte) bool {
	path := strings.TrimPrefix(string(term), "path/")
	return strings.Contains(path, ".") || strings.Contains(path, "[")
}

// joinVisibleFacts presents the module surface the way any other consumer sees
// it: a publication guarded by a branch edge belongs to that arm, and only a
// published branch proof makes it visible past the join. The partition owns
// that visibility rule, so this projection reuses it rather than repeating it.
func joinVisibleFacts(stored []equation.Fact) []equation.Fact {
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: stored})
	if err != nil {
		return stored
	}
	return partition.Values()
}

func publishedValues(artifact equation.Artifact, stored []equation.Fact) []equation.Fact {
	stored = joinVisibleFacts(stored)
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
	artifactDisplayBindings(artifact, func(target, display []byte, coordinate equation.Coordinate) {
		key := "value/" + string(target) + "/" + coordinate.Name
		value, found := storedByKey[key]
		if !found {
			return
		}
		decoded, err := displayValue(value)
		if err != nil {
			return
		}
		byIdentity[key] = candidate{fact: equation.Fact{Key: key, Value: decoded}, display: string(display), coordinate: coordinate}
	})

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

// joinVisibleOutcomes applies the same join rule to the outcome lane: an edge
// published under a branch guard belongs to that arm, so only a published
// branch proof carries it past the join. The value lane is the evidence that
// resolves such a proof, exactly as it is for any other consumer.
func joinVisibleOutcomes(stored, evidence []equation.Fact) []equation.Fact {
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: evidence, Outcomes: stored})
	if err != nil {
		return stored
	}
	return partition.Outcomes()
}

func publishedOutcomes(stored, evidence []equation.Fact) []equation.Fact {
	stored = joinVisibleOutcomes(stored, evidence)
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
