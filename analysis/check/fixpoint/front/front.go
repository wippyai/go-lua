package front

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// ErrUnsupportedInstruction reports a WIR operation outside the front's
// admitted family. CompileBody never omits such an operation.
var ErrUnsupportedInstruction = errors.New("front: unsupported WIR instruction")

const (
	entryKernel                 = "front/entry/v1"
	writeKernel                 = "front/environment-write/v1"
	allocationTemplateKernel    = "front/allocation-template/v1"
	objectMaterializationKernel = "front/object-materialization/v1"
	pathReplacementKernel       = "front/path-replacement/v1"
	dynamicIndexReadKernel      = "front/dynamic-index-read/v1"
	pathInvalidationKernel      = "front/path-invalidation/v1"
	indexMutationKernel         = "front/index-mutation/v1"
	branchKernel                = "front/branch-relations/v1"
	applyKernel                 = "front/apply/v1"
	resultsKernel               = "front/call-results/v1"
	externalCallKernel          = "front/external-call/v1"
	genericForKernel            = "front/generic-for/v1"
	selectKernel                = "front/channel-select/v1"
	publicationKernel           = "front/publication/v1"
	claimKernel                 = "front/claim/v1"
	expressionKernel            = "front/expression/v1"
	entryName                   = "entry"
)

// CompileBody parses source and lowers its chunk through bind, cfgbuild, and
// wirlower before compiling the resulting complete equation source. The
// walking skeleton admits only the structural entry operation; later families
// are added explicitly rather than being skipped.
// Compilation is the front's complete admission result.  Artifact is always
// present; Cyclic is present exactly when the source CFG has a recurrence and
// carries the source-frozen WTO certificate for that same artifact.
type Compilation struct {
	Artifact equation.Artifact
	Cyclic   *equation.CyclicArtifact
	// Nested holds the independently admitted lexical bodies owned by closure
	// allocations in Artifact. They retain the same WIR-derived equation form
	// and publication path as the enclosing body; a caller decides which body
	// entries are available to evaluate.
	Nested           []Compilation
	ClaimSpans       map[string]wir.Span
	ClaimTargetSpans map[string]wir.Span
	CallSpans        map[string]wir.Span
	BranchSpans      map[string]wir.Span
}

// Compile parses and lowers one complete body, retaining cyclic control-flow
// as a frozen equation certificate rather than rejecting it at the front door.
func Compile(source string) (Compilation, error) {
	stmts, err := parse.ParseString(source, "<front>")
	if err != nil {
		return Compilation{}, fmt.Errorf("front: parse body: %w", err)
	}
	// channel is the ambient runtime module whose select form has a dedicated
	// WIR operation.  A local binding still shadows it, so arbitrary
	// user-authored .select methods remain ordinary calls.
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	// The sealed call-order view keeps recognized channel.select case calls
	// inside the select operation rather than emitting independent calls before
	// the select transaction.
	built := cfgbuild.BuildChunkWithOptions(stmts, bindings, cfgbuild.Options{SealedLuaTypeChecks: true})
	if built == nil || built.Graph == nil {
		return Compilation{}, fmt.Errorf("front: build CFG")
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	if body == nil {
		return Compilation{}, fmt.Errorf("front: lower WIR")
	}
	artifact, err := compileWIR(source, body, built.Graph, assignmentSnapshotStarts(stmts, built))
	if err != nil {
		return Compilation{}, err
	}
	claimSpans, claimTargetSpans := claimSpans(body, artifact)
	compilation := Compilation{Artifact: artifact, ClaimSpans: claimSpans, ClaimTargetSpans: claimTargetSpans, CallSpans: callSpans(body, artifact), BranchSpans: branchSpans(body, artifact)}
	if graphHasCycle(built.Graph) {
		cyclic, err := freezeCyclicArtifact(artifact, body, built.Graph)
		if err != nil {
			return Compilation{}, err
		}
		compilation.Cyclic = &cyclic
	}
	nested, err := compileNestedBodies(body)
	if err != nil {
		return Compilation{}, err
	}
	compilation.Nested = nested
	return compilation, nil
}

// compileNestedBodies admits every complete WIR lexical child through the
// ordinary equation front. The parent WIR already owns the child body and CFG,
// so this is neither a second source traversal nor a child evaluator.
func compileNestedBodies(parent *wir.Body) ([]Compilation, error) {
	if parent == nil {
		return nil, nil
	}
	protos := parent.Protos()
	children := make([]Compilation, 0, len(protos))
	for _, proto := range protos {
		if proto.Body == nil || proto.Graph == nil || proto.Name == "" {
			return nil, fmt.Errorf("front: nested prototype is incomplete")
		}
		artifact, err := compileWIR(proto.Name, proto.Body, proto.Graph, nil)
		if err != nil {
			return nil, fmt.Errorf("front: nested body %q: %w", proto.Name, err)
		}
		claimSpans, claimTargetSpans := claimSpans(proto.Body, artifact)
		child := Compilation{
			Artifact:         artifact,
			ClaimSpans:       claimSpans,
			ClaimTargetSpans: claimTargetSpans,
			CallSpans:        callSpans(proto.Body, artifact),
			BranchSpans:      branchSpans(proto.Body, artifact),
		}
		if graphHasCycle(proto.Graph) {
			cyclic, err := freezeCyclicArtifact(artifact, proto.Body, proto.Graph)
			if err != nil {
				return nil, fmt.Errorf("front: nested body %q: %w", proto.Name, err)
			}
			child.Cyclic = &cyclic
		}
		child.Nested, err = compileNestedBodies(proto.Body)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return children, nil
}

// callSpans binds source call anchors to their apply operations.  The apply
// occurrence is the equation point that proves call-contract violations; WIR
// remains the sole source authority for its position.
func callSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	calls := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpCall {
			calls = append(calls, instruction)
		}
	}
	out := make(map[string]wir.Span, len(calls))
	callIndex := 0
	for _, item := range artifact.Equations {
		if item.Occurrence.Kind != "apply" || callIndex >= len(calls) {
			continue
		}
		call := calls[callIndex]
		span := call.CallSpan
		if !span.Valid() {
			span = call.CalleeSpan
		}
		if span.Valid() {
			out[item.Target.Name+"/call"] = span
		}
		if call.CalleeSpan.Valid() {
			out[item.Target.Name+"/callee"] = call.CalleeSpan
		}
		for index, argument := range body.CallArgumentMeta(call.CallArgs) {
			if argument.Span.Valid() {
				out[item.Target.Name+"/"+indexedRole("argument", index)] = argument.Span
			}
		}
		callIndex++
	}
	return out
}

// branchSpans retains the whole-condition anchor for a branch-owned fact.
func branchSpans(body *wir.Body, artifact equation.Artifact) map[string]wir.Span {
	if body == nil {
		return nil
	}
	branches := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		if instruction := body.Instr(index); instruction.Op == wir.OpBranch {
			branches = append(branches, instruction)
		}
	}
	out := make(map[string]wir.Span, len(branches))
	branchIndex := 0
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" || branchIndex >= len(branches) {
			continue
		}
		if span := branches[branchIndex].ExprSpan; span.Valid() {
			out[operation.Target.Name] = span
		}
		branchIndex++
	}
	return out
}

// claimSpans retains the source anchors needed to render claim failures after
// equation evaluation. Equation facts name their owning operation, while WIR
// remains the authority for source coordinates.
func claimSpans(body *wir.Body, artifact equation.Artifact) (map[string]wir.Span, map[string]wir.Span) {
	if body == nil {
		return nil, nil
	}
	claims := make([]wir.Instruction, 0)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpClaim {
			claims = append(claims, instruction)
		}
	}
	spans := make(map[string]wir.Span, len(claims))
	targets := make(map[string]wir.Span, len(claims))
	claimIndex := 0
	for _, item := range artifact.Equations {
		if item.Occurrence.Kind != "claim" || claimIndex >= len(claims) {
			continue
		}
		claim := claims[claimIndex]
		span := claim.TargetSpan
		if (claim.Claim == wir.ClaimAnnotation || claim.Claim == wir.ClaimCast) && claim.ExprSpan.Valid() {
			span = claim.ExprSpan
		}
		if !span.Valid() {
			span = claim.ExprSpan
		}
		if !span.Valid() {
			span = claim.CallSpan
		}
		if span.Valid() {
			spans[item.Target.Name] = span
		}
		if claim.DeclaredSpan.Valid() {
			targets[item.Target.Name] = claim.DeclaredSpan
		} else if claim.TargetSpan.Valid() {
			targets[item.Target.Name] = claim.TargetSpan
		}
		claimIndex++
	}
	return spans, targets
}

// CompileBody is retained for consumers that only need the equation artifact.
// Check uses Compile so it can select the acyclic or cyclic execution path.
func CompileBody(source string) (equation.Artifact, error) {
	compilation, err := Compile(source)
	if err != nil {
		return equation.Artifact{}, err
	}
	return compilation.Artifact, nil
}

type operation struct {
	instruction     wir.Instruction
	target          equation.Coordinate
	family          string
	allocationSite  string
	allocationEntry *wir.TableEntry
	callResults     bool
	callApply       equation.Coordinate
	external        equation.Term
}

func compileWIR(source string, body *wir.Body, graph cfg.Graph, snapshots map[cfg.Point]cfg.Point) (equation.Artifact, error) {
	if body == nil || graph == nil {
		return equation.Artifact{}, fmt.Errorf("front: nil WIR body")
	}
	bodyID := bodyID(source)
	entry := equation.EntryParameter{Body: bodyID, Name: entryName}
	loopBindings, err := genericForBindings(body, graph)
	if err != nil {
		return equation.Artifact{}, err
	}
	loopBindingPoints := make(map[cfg.Point]bool)
	for _, bindings := range loopBindings {
		for _, binding := range bindings {
			loopBindingPoints[binding.point] = true
		}
	}
	for point := range numericForBindingPoints(body) {
		loopBindingPoints[point] = true
	}
	operations := make([]operation, 0, body.Len())
	entries := 0
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpEntry, wir.OpStaticMemberWrite, wir.OpBranch, wir.OpClaim, wir.OpSelect, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpLogical:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
			if instruction.Op == wir.OpEntry {
				entries++
			}
		case wir.OpDynamicIndexRead:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, family: "dynamic-index-read"})
		case wir.OpAssign:
			if instruction.A.Kind == wir.OperandNone {
				if loopBindingPoints[instruction.Point] {
					continue
				}
				return equation.Artifact{}, fmt.Errorf("front: assignment at point %d has no value source", instruction.Point)
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpIterate:
			if instruction.Iter == wir.IterGeneric && len(loopBindings[instruction.Point]) == 0 {
				return equation.Artifact{}, fmt.Errorf("front: generic-for at point %d has no bound variables", instruction.Point)
			}
			if instruction.Iter != wir.IterGeneric && instruction.Iter != wir.IterNumeric {
				return equation.Artifact{}, fmt.Errorf("front: iterate at point %d has unknown kind %d", instruction.Point, instruction.Iter)
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpDynamicIndexWrite:
			// A dynamic store has two inseparable semantic occurrences: the
			// mutation itself and the invalidation of every path below the
			// dynamically addressed container.  Keep both occurrences or fail
			// the whole body; emitting only either half would be unsound.
			operations = append(operations,
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, family: "path-invalidation"},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 1)}, family: "index-mutation"},
			)
		case wir.OpMakeTable, wir.OpClosure:
			site := operationName(len(operations))
			operations = append(operations,
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: site}, family: "allocation-template", allocationSite: site},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 1)}, family: "object-materialization", allocationSite: site},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 2)}, family: "allocation-write"},
			)
			// Constructors publish a completed value just as assignments do. The
			// allocation pair records topology; these writes close value chains for
			// later reads and returns.
			if instruction.Op == wir.OpMakeTable && instruction.Dst.Kind == wir.OperandPath {
				for _, entry := range body.TableEntries(instruction.TableEntries) {
					entry := entry
					operations = append(operations, operation{
						instruction:     instruction,
						target:          equation.Coordinate{Body: bodyID, Name: operationName(len(operations))},
						family:          "allocation-entry-write",
						allocationEntry: &entry,
					})
				}
			}
		case wir.OpCall:
			// Calls exclusively own their application/result pair.  An external
			// provider contributes one sealed boundary factor between those two
			// occurrences; it never manufactures or owns result slots.
			apply := equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}
			operations = append(operations, operation{instruction: instruction, target: apply})
			if provider, ok := externalProvider(body, instruction); ok {
				operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, callApply: apply, external: provider})
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}, callResults: true, callApply: apply})
		case wir.OpReturn:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpExit, wir.OpNoop:
			// These WIR operations carry no transfer occurrence. They are CFG
			// structure, so retaining them as equations would invent semantics.
		default:
			return equation.Artifact{}, fmt.Errorf("%w: %d at instruction %d", ErrUnsupportedInstruction, instruction.Op, index)
		}
	}
	if entries != 1 {
		return equation.Artifact{}, fmt.Errorf("front: WIR body has %d entry operations, want one", entries)
	}
	drafts := make([]equation.Draft, 0, len(operations))
	branchTargets := make(map[cfg.Point]equation.Coordinate)
	for _, operation := range operations {
		if operation.instruction.Op == wir.OpBranch {
			if _, duplicate := branchTargets[operation.instruction.Point]; duplicate {
				return equation.Artifact{}, fmt.Errorf("front: multiple branches at CFG point %d", operation.instruction.Point)
			}
			branchTargets[operation.instruction.Point] = operation.target
		}
	}
	guardReachability := newReachabilityCache(graph)
	for index, operation := range operations {
		instruction := operation.instruction
		draft := equation.Draft{Target: operation.target, Entry: entry}
		if index != 0 {
			draft.Dependencies = []equation.Coordinate{operations[index-1].target}
		}
		switch {
		case operation.family == "allocation-template" || operation.family == "object-materialization":
			terms, err := allocationOperands(body, instruction, operation.allocationSite)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: %s %s: %w", operation.family, operation.target.Name, err)
			}
			draft.Occurrence = occurrence(operation.family)
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			if operation.family == "allocation-template" {
				draft.Operands = terms.template
			} else {
				draft.Operands = terms.materialization
			}
		case operation.family == "allocation-write":
			operands, err := allocationWriteOperands(body, instruction, operation, operations)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: allocation write %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case operation.family == "allocation-entry-write":
			operands, err := allocationEntryWriteOperands(body, instruction, operation, operations)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: allocation entry write %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case operation.family == "path-invalidation" || operation.family == "index-mutation":
			container := equation.ClosedTerm([]byte("scalar/top"))
			if instruction.Dst.Kind != wir.OperandNone {
				var err error
				container, err = pathStoreTerm(body, instruction.Dst)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: %w", operation.target.Name, err)
				}
			}
			key, err := pathStoreTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: key: %w", operation.target.Name, err)
			}
			value, err := pathStoreTerm(body, instruction.B)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: value: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence(operation.family)
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			if operation.family == "path-invalidation" {
				draft.Operands = []equation.Operand{
					{Role: "container", Term: container},
					{Role: "key", Term: key},
					{Role: "suffix", Term: suffixTerm(body, instruction.DynamicSuffix)},
				}
			} else {
				draft.Operands = []equation.Operand{
					{Role: "container", Term: container},
					{Role: "key", Term: key},
					{Role: "suffix", Term: suffixTerm(body, instruction.DynamicSuffix)},
					{Role: "value", Term: value},
				}
			}
		case instruction.Op == wir.OpEntry:
			draft.Occurrence = occurrence("entry")
			draft.Operands = []equation.Operand{{Role: "entry", Term: equation.EntryTerm(entry)}}
		case instruction.Op == wir.OpAssign:
			target, err := scalarTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			display := string(target.Encoding)
			if instruction.Dst.Kind == wir.OperandPath {
				_, display, err = pathTerm(body, instruction.Dst)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
				}
			}
			value, err := scalarTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			readBefore, err := readBeforeTerm(operation, operations, snapshots)
			if instruction.Dst.Kind == wir.OperandTemp {
				// Temporary assignments are expression-internal steps, not Lua
				// statement targets. They must read the immediately preceding
				// operation rather than demand a statement snapshot they cannot own.
				readBefore, err = precedingReadBoundary(operation, operations)
			}
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			absence := assignmentAbsencePolicy(body, instruction.A)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "value", Term: value},
				{Role: "read-before", Term: readBefore},
				{Role: "absence", Term: equation.ClosedTerm([]byte(absence))},
			}
		case instruction.Op == wir.OpStaticMemberWrite:
			target, display, err := memberPathTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: static member write %s: %w", operation.target.Name, err)
			}
			value, err := pathStoreTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: static member write %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("path-replacement")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "value", Term: value},
			}
		case instruction.Op == wir.OpDynamicIndexRead:
			target, err := scalarTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index read %s: %w", operation.target.Name, err)
			}
			container, err := pathStoreTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index read %s: container: %w", operation.target.Name, err)
			}
			key, err := pathStoreTerm(body, instruction.B)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index read %s: key: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("dynamic-index-read")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "container", Term: container},
				{Role: "key", Term: key},
			}
		case instruction.Op == wir.OpClaim:
			target, err := scalarTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: claim %s: target: %w", operation.target.Name, err)
			}
			value, err := scalarTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: claim %s: value: %w", operation.target.Name, err)
			}
			claimType, err := claimTypeTerm(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: claim %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("claim")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "value", Term: value},
				{Role: "kind", Term: equation.ClosedTerm([]byte("claim-kind/" + strconv.Itoa(int(instruction.Claim))))},
				{Role: "type", Term: claimType},
			}
			if target, ok := shapefact.EncodeTarget(body.Type(instruction.Type)); ok {
				draft.Operands = append(draft.Operands, equation.Operand{Role: "shape-target", Term: equation.ClosedTerm(target)})
			}
			if instruction.Dst.Kind == wir.OperandPath {
				display := body.Path(wir.PathRef(instruction.Dst.Ref)).String()
				if display == "" {
					return equation.Artifact{}, fmt.Errorf("front: claim %s: empty path target", operation.target.Name)
				}
				draft.Operands = append(draft.Operands, equation.Operand{Role: "display", Term: equation.ClosedTerm([]byte(display))})
			}
			if instruction.A.Kind == wir.OperandPath {
				sourceDisplay := body.Path(wir.PathRef(instruction.A.Ref)).String()
				if sourceDisplay == "" {
					return equation.Artifact{}, fmt.Errorf("front: claim %s: empty path source", operation.target.Name)
				}
				draft.Operands = append(draft.Operands, equation.Operand{Role: "source-display", Term: equation.ClosedTerm([]byte(sourceDisplay))})
			}
		case instruction.Op == wir.OpBinOp, instruction.Op == wir.OpUnOp, instruction.Op == wir.OpConcat, instruction.Op == wir.OpLogical:
			operands, err := expressionOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: expression %s: %w", operation.target.Name, err)
			}
			draft.Occurrence, draft.Guards, draft.Operands = occurrence("expression"), guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets), operands
		case instruction.Op == wir.OpBranch:
			draft.Occurrence = occurrence("branch-relations")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			operands, err := branchOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: branch %s: %w", operation.target.Name, err)
			}
			draft.Operands = operands
		case instruction.Op == wir.OpCall:
			if operation.external.Encoding != nil {
				operands, err := externalCallOperands(body, instruction, operation.callApply, operation.external)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: external call %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("external-call")
				draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			} else if !operation.callResults {
				operands, err := applyOperands(body, instruction)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: call %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("apply")
				draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			} else {
				operands, err := callResultOperands(body, instruction, operation.callApply)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: call results %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("call-results")
				draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			}
		case instruction.Op == wir.OpReturn:
			operands, err := publicationOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: return %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("publication")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case instruction.Op == wir.OpIterate:
			var operands []equation.Operand
			var err error
			if instruction.Iter == wir.IterNumeric {
				operands, err = numericForOperands(body, instruction)
			} else {
				operands, err = genericForOperands(body, instruction, loopBindings[instruction.Point])
			}
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: generic-for %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("generic-for")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		case instruction.Op == wir.OpSelect:
			result, err := selectResultTerm(instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: channel select %s: %w", operation.target.Name, err)
			}
			operands := make([]equation.Operand, 0, 2+instruction.List.Len)
			operands = append(operands,
				equation.Operand{Role: "result", Term: result},
				equation.Operand{Role: "default", Term: equation.ClosedTerm([]byte("select/default/" + strconv.FormatBool(instruction.SelectDefault)))},
			)
			for caseIndex, candidate := range body.Operands(instruction.List) {
				channel, err := selectCaseTerm(body, candidate)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: channel select %s case %d: %w", operation.target.Name, caseIndex, err)
				}
				operands = append(operands, equation.Operand{Role: fmt.Sprintf("case-%08d", caseIndex), Term: channel})
			}
			draft.Occurrence = occurrence("channel-select")
			draft.Guards = guardsForPoint(graph, guardReachability, instruction.Point, bodyID, branchTargets)
			draft.Operands = operands
		default:
			return equation.Artifact{}, fmt.Errorf("%w: %d", ErrUnsupportedInstruction, instruction.Op)
		}
		drafts = append(drafts, draft)
	}
	compiler, err := equation.Skeleton().With("entry", equation.BindExistingKernel(entryKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure entry compiler: %w", err)
	}
	compiler, err = compiler.With("environment-write", equation.BindExistingKernel(writeKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure assignment compiler: %w", err)
	}
	compiler, err = compiler.With("allocation-template", equation.BindExistingKernel(allocationTemplateKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure allocation template compiler: %w", err)
	}
	compiler, err = compiler.With("object-materialization", equation.BindExistingKernel(objectMaterializationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure object materialization compiler: %w", err)
	}
	compiler, err = compiler.With("path-replacement", equation.BindExistingKernel(pathReplacementKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure path replacement compiler: %w", err)
	}
	compiler, err = compiler.With("dynamic-index-read", equation.BindExistingKernel(dynamicIndexReadKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure dynamic index read compiler: %w", err)
	}
	compiler, err = compiler.With("path-invalidation", equation.BindExistingKernel(pathInvalidationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure path invalidation compiler: %w", err)
	}
	compiler, err = compiler.With("index-mutation", equation.BindExistingKernel(indexMutationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure index mutation compiler: %w", err)
	}
	compiler, err = compiler.With("branch-relations", equation.BindExistingKernel(branchKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure branch compiler: %w", err)
	}
	compiler, err = compiler.With("apply", equation.BindExistingKernel(applyKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure apply compiler: %w", err)
	}
	compiler, err = compiler.With("call-results", equation.BindExistingKernel(resultsKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure call-results compiler: %w", err)
	}
	compiler, err = compiler.With("external-call", equation.BindExistingKernel(externalCallKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure external call compiler: %w", err)
	}
	compiler, err = compiler.With("generic-for", equation.BindExistingKernel(genericForKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure generic-for compiler: %w", err)
	}
	compiler, err = compiler.With("channel-select", equation.BindExistingKernel(selectKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure channel-select compiler: %w", err)
	}
	compiler, err = compiler.With("publication", equation.BindExistingKernel(publicationKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure publication compiler: %w", err)
	}
	compiler, err = compiler.With("claim", equation.BindExistingKernel(claimKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure claim compiler: %w", err)
	}
	compiler, err = compiler.With("expression", equation.BindExistingKernel(expressionKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure expression compiler: %w", err)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: drafts})
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: compile equations: %w", err)
	}
	return artifact, nil
}

// assignmentSnapshotStarts maps each ordinary/local assignment point to the
// first point in its source statement. Lua evaluates every right-hand side
// before writing any left-hand target, so all targets in one statement must
// resolve path operands at that common pre-write boundary.
func assignmentSnapshotStarts(stmts []ast.Stmt, built *cfgbuild.Result) map[cfg.Point]cfg.Point {
	starts := make(map[cfg.Point]cfg.Point)
	if built == nil {
		return starts
	}
	var visit func([]ast.Stmt)
	mark := func(stmt ast.Stmt, targets int) {
		points := built.StmtPoints.PointsFor(stmt)
		if targets == 0 || len(points) < targets {
			return
		}
		assignmentPoints := points[len(points)-targets:]
		for _, point := range assignmentPoints {
			starts[point] = assignmentPoints[0]
		}
	}
	visit = func(items []ast.Stmt) {
		for _, stmt := range items {
			switch node := stmt.(type) {
			case *ast.LocalAssignStmt:
				mark(node, len(node.Names))
			case *ast.AssignStmt:
				mark(node, len(node.Lhs))
			case *ast.IfStmt:
				visit(node.Then)
				visit(node.Else)
			case *ast.DoBlockStmt:
				visit(node.Stmts)
			case *ast.WhileStmt:
				visit(node.Stmts)
			case *ast.RepeatStmt:
				visit(node.Stmts)
			case *ast.NumberForStmt:
				visit(node.Stmts)
			case *ast.GenericForStmt:
				visit(node.Stmts)
			}
		}
	}
	visit(stmts)
	return starts
}

func readBeforeTerm(current operation, operations []operation, snapshots map[cfg.Point]cfg.Point) (equation.Term, error) {
	start, found := snapshots[current.instruction.Point]
	if !found {
		// Nested WIR bodies are already source-normalized but intentionally do
		// not retain an AST statement-point sidecar. Their assignment point is
		// therefore the exact snapshot boundary; its predecessor remains the
		// same admitted operation-order seam used by root-body assignments.
		for index, candidate := range operations {
			if candidate.target == current.target {
				if index == 0 {
					return equation.Term{}, fmt.Errorf("assignment snapshot has no predecessor")
				}
				return equation.ClosedTerm([]byte("front/read-before/" + operations[index-1].target.Name)), nil
			}
		}
		return equation.Term{}, fmt.Errorf("assignment operation %s is absent", current.target.Name)
	}
	for index, candidate := range operations {
		if candidate.instruction.Point != start {
			continue
		}
		if index == 0 {
			return equation.Term{}, fmt.Errorf("assignment snapshot has no predecessor")
		}
		return equation.ClosedTerm([]byte("front/read-before/" + operations[index-1].target.Name)), nil
	}
	return equation.Term{}, fmt.Errorf("assignment snapshot boundary %d has no operation", start)
}

func implicitGlobalPath(body *wir.Body, operand wir.Operand) bool {
	if body == nil || operand.Kind != wir.OperandPath {
		return false
	}
	return body.IsImplicitGlobalSymbol(body.Path(wir.PathRef(operand.Ref)).Symbol)
}

// assignmentAbsencePolicy makes the source-level distinction between an
// unread implicit global and a path whose producing heap write is outside the
// current scalar model. Lua reads the former as nil; the latter is an unknown
// value and must not be turned into false or rejected as an incomplete fact.
func assignmentAbsencePolicy(body *wir.Body, operand wir.Operand) string {
	if operand.Kind != wir.OperandPath {
		return "front/absence/error"
	}
	if implicitGlobalPath(body, operand) {
		return "front/absence/nil"
	}
	return "front/absence/top"
}

func bodyID(source string) equation.BodyID {
	return equation.BodyID(sha256.Sum256(append([]byte("front/lua-body/v1\x00"), []byte(source)...)))
}

func occurrence(kind string) equation.Occurrence {
	contract, _ := ContractID(kind)
	return equation.Occurrence{Kind: kind, ContractID: contract}
}

func operationName(index int) string { return fmt.Sprintf("op-%08d", index) }

// ContractID returns the contract identity admitted by this front for kind.
// The engine uses this exact content identity when registering its canonical
// kernels; unknown kinds deliberately have no binding.
func ContractID(kind string) (equation.ContentID, bool) {
	switch kind {
	case "entry", "environment-write", "allocation-template", "object-materialization", "path-replacement", "dynamic-index-read", "path-invalidation", "index-mutation", "branch-relations", "apply", "call-results", "external-call", "generic-for", "channel-select", "publication", "claim", "expression":
		return equation.ContentID(sha256.Sum256([]byte("front/contract/v1/" + kind))), true
	default:
		return equation.ContentID{}, false
	}
}

// KernelID returns the canonical kernel identity admitted by this walking
// front for kind. It has no fallback for unsupported equation families.
func KernelID(kind string) (string, bool) {
	switch kind {
	case "entry":
		return entryKernel, true
	case "environment-write":
		return writeKernel, true
	case "allocation-template":
		return allocationTemplateKernel, true
	case "object-materialization":
		return objectMaterializationKernel, true
	case "path-replacement":
		return pathReplacementKernel, true
	case "dynamic-index-read":
		return dynamicIndexReadKernel, true
	case "path-invalidation":
		return pathInvalidationKernel, true
	case "index-mutation":
		return indexMutationKernel, true
	case "branch-relations":
		return branchKernel, true
	case "apply":
		return applyKernel, true
	case "call-results":
		return resultsKernel, true
	case "external-call":
		return externalCallKernel, true
	case "generic-for":
		return genericForKernel, true
	case "channel-select":
		return selectKernel, true
	case "publication":
		return publicationKernel, true
	case "claim":
		return claimKernel, true
	case "expression":
		return expressionKernel, true
	default:
		return "", false
	}
}

type loopBinding struct {
	point   cfg.Point
	term    equation.Term
	display string
}

func genericForBindings(body *wir.Body, graph cfg.Graph) (map[cfg.Point][]loopBinding, error) {
	bindings := make(map[cfg.Point][]loopBinding)
	for index := 0; index < body.Len(); index++ {
		header := body.Instr(index)
		if header.Op != wir.OpIterate || header.Iter != wir.IterGeneric {
			continue
		}
		var next cfg.Point
		found := false
		for _, successor := range graph.Successors(header.Point) {
			condition, branchEdge := graph.EdgeCond(header.Point, successor)
			if branchEdge && condition {
				if found {
					return nil, fmt.Errorf("front: generic-for at point %d has multiple true successors", header.Point)
				}
				next, found = successor, true
			}
		}
		if !found {
			return nil, fmt.Errorf("front: generic-for at point %d has no true successor", header.Point)
		}
		seen := map[cfg.Point]bool{}
		for {
			if seen[next] {
				return nil, fmt.Errorf("front: generic-for at point %d has cyclic binding path", header.Point)
			}
			seen[next] = true
			instructions := body.PointInstructions(next)
			if len(instructions) != 1 || instructions[0].Op != wir.OpAssign || instructions[0].A.Kind != wir.OperandNone {
				break
			}
			term, display, err := pathTerm(body, instructions[0].Dst)
			if err != nil {
				return nil, fmt.Errorf("front: generic-for at point %d: %w", header.Point, err)
			}
			bindings[header.Point] = append(bindings[header.Point], loopBinding{point: next, term: term, display: display})
			successors := graph.Successors(next)
			if len(successors) != 1 {
				break
			}
			next = successors[0]
		}
		if len(bindings[header.Point]) == 0 {
			return nil, fmt.Errorf("front: generic-for at point %d has no binding assignments", header.Point)
		}
	}
	return bindings, nil
}

func numericForBindingPoints(body *wir.Body) map[cfg.Point]bool {
	points := make(map[cfg.Point]bool)
	if body == nil {
		return points
	}
	bindings := make(map[wir.Operand]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpIterate || instruction.Iter != wir.IterNumeric {
			continue
		}
		for _, result := range body.Operands(instruction.Results) {
			bindings[result] = true
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpAssign && instruction.A.Kind == wir.OperandNone && bindings[instruction.Dst] {
			points[instruction.Point] = true
		}
	}
	return points
}

func genericForOperands(body *wir.Body, instruction wir.Instruction, bindings []loopBinding) ([]equation.Operand, error) {
	if instruction.Iter != wir.IterGeneric {
		return nil, fmt.Errorf("iterator kind %d is not generic", instruction.Iter)
	}
	sources := body.Operands(instruction.List)
	if len(sources) == 0 {
		return nil, fmt.Errorf("iterator has no source values")
	}
	roles := []string{"iterator", "state", "control"}
	operands := make([]equation.Operand, 0, len(roles)+2*len(bindings))
	for index, role := range roles {
		term := equation.ClosedTerm([]byte("scalar/nil"))
		// An open iterator tail carries no closed state/control coordinates. It
		// is not nil: retain Top so the loop cannot manufacture a finite tuple.
		if instruction.ListSpread {
			term = equation.ClosedTerm([]byte("scalar/top"))
		}
		if index < len(sources) {
			resolved, err := scalarTerm(body, sources[index])
			if err != nil {
				return nil, fmt.Errorf("%s source: %w", role, err)
			}
			term = resolved
		}
		operands = append(operands, equation.Operand{Role: role, Term: term})
	}
	for index, binding := range bindings {
		name := fmt.Sprintf("%08d", index)
		operands = append(operands,
			equation.Operand{Role: "result-" + name, Term: binding.term},
			equation.Operand{Role: "display-" + name, Term: equation.ClosedTerm([]byte(binding.display))},
		)
	}
	return operands, nil
}

func numericForOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	if instruction.Iter != wir.IterNumeric {
		return nil, fmt.Errorf("iterator kind %d is not numeric", instruction.Iter)
	}
	sources := body.Operands(instruction.List)
	results := body.Operands(instruction.Results)
	if len(sources) != 3 || len(results) != 1 {
		return nil, fmt.Errorf("numeric-for has %d bounds and %d bindings, want 3 and 1", len(sources), len(results))
	}
	operands := make([]equation.Operand, 0, 5)
	for index, role := range []string{"iterator", "state", "control"} {
		term, err := scalarTerm(body, sources[index])
		if err != nil {
			return nil, fmt.Errorf("%s bound: %w", role, err)
		}
		operands = append(operands, equation.Operand{Role: role, Term: term})
	}
	result, display, err := pathTerm(body, results[0])
	if err != nil {
		return nil, fmt.Errorf("numeric binding: %w", err)
	}
	return append(operands,
		equation.Operand{Role: "result-00000000", Term: result},
		equation.Operand{Role: "display-00000000", Term: equation.ClosedTerm([]byte(display))},
	), nil
}

func selectResultTerm(operand wir.Operand) (equation.Term, error) {
	if operand.Kind != wir.OperandTemp {
		return equation.Term{}, fmt.Errorf("result is operand kind %d, want temporary", operand.Kind)
	}
	return equation.ClosedTerm([]byte("value/temp/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
}

func selectCaseTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	if operand.Kind != wir.OperandPath {
		return equation.Term{}, fmt.Errorf("case is operand kind %d, want path", operand.Kind)
	}
	return scalarTerm(body, operand)
}

func pathTerm(body *wir.Body, operand wir.Operand) (equation.Term, string, error) {
	if operand.Kind != wir.OperandPath {
		return equation.Term{}, "", fmt.Errorf("assignment target is operand kind %d, want path", operand.Kind)
	}
	path := body.Path(wir.PathRef(operand.Ref))
	if path.IsEmpty() || path.Key() == "" || path.String() == "" {
		return equation.Term{}, "", fmt.Errorf("empty assignment target path")
	}
	return equation.ClosedTerm([]byte("path/" + path.Key())), path.String(), nil
}

// memberPathTerm rejects a root target: a static member write is never a
// disguised environment write.  Nil remains a valid value operand elsewhere;
// an absent target is always rejected.
func memberPathTerm(body *wir.Body, operand wir.Operand) (equation.Term, string, error) {
	term, display, err := pathTerm(body, operand)
	if err != nil {
		return equation.Term{}, "", err
	}
	path := body.Path(wir.PathRef(operand.Ref))
	if len(path.Segments) == 0 {
		return equation.Term{}, "", fmt.Errorf("static member target has no member path")
	}
	return term, display, nil
}

// pathStoreTerm preserves every operand shape this family can consume.  In
// particular, scalar/nil is a real Lua value, while OperandNone is absence and
// therefore an error. A temp uses the same body-local value namespace as every
// other consumer: temp zero is the first valid temporary, not a sentinel.
func pathStoreTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	return scalarTerm(body, operand)
}

func suffixTerm(body *wir.Body, suffix wir.SegmentRange) equation.Term {
	return equation.ClosedTerm([]byte("suffix/" + segment.FormatSegments(body.Segments(suffix))))
}

func expressionOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	result, display, err := pathTerm(body, instruction.Dst)
	if instruction.Dst.Kind != wir.OperandPath {
		result, err = scalarTerm(body, instruction.Dst)
		display = ""
	}
	if err != nil {
		return nil, fmt.Errorf("result: %w", err)
	}
	if instruction.Op != wir.OpConcat && instruction.Operator == wir.OperatorNone {
		return nil, fmt.Errorf("missing operator")
	}
	operands := []equation.Operand{{Role: "result", Term: result}, {Role: "kind", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Op))))}, {Role: "operator", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Operator))))}}
	if display != "" {
		operands = append(operands, equation.Operand{Role: "display", Term: equation.ClosedTerm([]byte(display))})
	}
	appendOperand := func(role string, value wir.Operand) error {
		if value.Kind == wir.OperandNone {
			// This is an unrepresentable source operand, not Lua nil. Keep the
			// expression transaction complete with Top so it cannot invent a
			// concrete value or decide a branch from missing syntax evidence.
			operands = append(operands, equation.Operand{Role: role, Term: equation.ClosedTerm([]byte("scalar/top"))})
			return nil
		}
		term, err := scalarTerm(body, value)
		if err != nil {
			return fmt.Errorf("%s: %w", role, err)
		}
		operands = append(operands, equation.Operand{Role: role, Term: term})
		return nil
	}
	switch instruction.Op {
	case wir.OpBinOp, wir.OpLogical:
		if err := appendOperand("left", instruction.A); err != nil {
			return nil, err
		}
		if err := appendOperand("right", instruction.B); err != nil {
			return nil, err
		}
	case wir.OpUnOp:
		if err := appendOperand("value", instruction.A); err != nil {
			return nil, err
		}
	case wir.OpConcat:
		values := body.Operands(instruction.List)
		if len(values) < 2 {
			return nil, fmt.Errorf("concat has %d operands", len(values))
		}
		for i, value := range values {
			if err := appendOperand(indexedRole("value", i), value); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("not expression")
	}
	return operands, nil
}

func scalarTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	switch operand.Kind {
	case wir.OperandPath:
		path := body.Path(wir.PathRef(operand.Ref))
		if path.IsEmpty() || path.Key() == "" {
			return equation.Term{}, fmt.Errorf("empty path operand")
		}
		return equation.ClosedTerm([]byte("path/" + path.Key())), nil
	case wir.OperandConst:
		constant := body.Const(wir.ConstRef(operand.Ref))
		switch constant.Kind {
		case wir.ConstNil:
			return equation.ClosedTerm([]byte("scalar/nil")), nil
		case wir.ConstBool:
			return equation.ClosedTerm([]byte("scalar/bool/" + strconv.FormatBool(constant.Bool))), nil
		case wir.ConstNumber:
			return equation.ClosedTerm([]byte("scalar/number/" + constant.Number)), nil
		case wir.ConstString:
			return equation.ClosedTerm([]byte("scalar/string/" + strconv.Quote(constant.Str))), nil
		default:
			return equation.Term{}, fmt.Errorf("unknown constant kind %d", constant.Kind)
		}
	case wir.OperandTemp:
		return equation.ClosedTerm([]byte("temp/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
	case wir.OperandVararg:
		return equation.ClosedTerm([]byte("vararg")), nil
	default:
		return equation.Term{}, fmt.Errorf("operand kind %d is outside the scalar slice", operand.Kind)
	}
}

// claimTypeTerm seals the only type information an OpClaim may carry.  A
// non-nil assertion has no type target; every type-bearing claim must resolve
// to an interned WIR type instead of falling back to source spelling.
func claimTypeTerm(body *wir.Body, instruction wir.Instruction) (equation.Term, error) {
	if instruction.Claim == wir.ClaimAssert {
		if instruction.Type != 0 {
			return equation.Term{}, fmt.Errorf("non-nil assertion has a type target")
		}
		return equation.ClosedTerm([]byte("claim-type/non-nil")), nil
	}
	if instruction.Claim != wir.ClaimCast && instruction.Claim != wir.ClaimAnnotation && instruction.Claim != wir.ClaimAssertsPredicate {
		return equation.Term{}, fmt.Errorf("unknown claim kind %d", instruction.Claim)
	}
	if instruction.Type == 0 || body.Type(instruction.Type) == nil || body.TypeDisplay(instruction.Type) == "" {
		return equation.Term{}, fmt.Errorf("type-bearing claim has no resolved target type")
	}
	return equation.ClosedTerm([]byte("claim-type/" + strconv.Quote(body.TypeDisplay(instruction.Type)))), nil
}

// applyOperands preserves the complete source-side call shape. The kernel,
// rather than this front, owns dispatch and outcome semantics.
func applyOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	operands := make([]equation.Operand, 0, 12+int(instruction.List.Len)+int(instruction.CallTypeArgs.Len))
	if instruction.Call.Method != 0 {
		if instruction.Call.Callee.Kind != wir.OperandNone || instruction.Call.Receiver.Kind == wir.OperandNone {
			return nil, fmt.Errorf("malformed method call shape")
		}
		receiver, err := scalarTerm(body, instruction.Call.Receiver)
		if err != nil {
			return nil, fmt.Errorf("receiver: %w", err)
		}
		method := body.Const(instruction.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, fmt.Errorf("malformed method name")
		}
		operands = append(operands,
			equation.Operand{Role: "receiver", Term: receiver},
			equation.Operand{Role: "method", Term: equation.ClosedTerm([]byte("method/" + strconv.Quote(method.Str)))},
		)
	} else {
		if instruction.Call.Callee.Kind == wir.OperandNone || instruction.Call.Receiver.Kind != wir.OperandNone {
			return nil, fmt.Errorf("malformed direct call shape")
		}
		callee, err := scalarTerm(body, instruction.Call.Callee)
		if err != nil {
			return nil, fmt.Errorf("callee: %w", err)
		}
		operands = append(operands, equation.Operand{Role: "callee", Term: callee})
		if instruction.Call.Callee.Kind == wir.OperandPath {
			calleePath := body.Path(wir.PathRef(instruction.Call.Callee.Ref)).String()
			if calleePath != "" {
				operands = append(operands, equation.Operand{Role: "callee-display", Term: equation.ClosedTerm([]byte(calleePath))})
			}
		}
	}
	for index, argument := range body.Operands(instruction.List) {
		term, err := scalarTerm(body, argument)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("argument", index), Term: term})
		if argument.Kind == wir.OperandPath {
			if display := body.Path(wir.PathRef(argument.Ref)).String(); display != "" {
				operands = append(operands, equation.Operand{Role: indexedRole("argument-display", index), Term: equation.ClosedTerm([]byte(display))})
			}
		}
	}
	if instruction.Type != 0 {
		typeName := body.TypeDisplay(instruction.Type)
		if typeName == "" {
			return nil, fmt.Errorf("empty callee type")
		}
		operands = append(operands, equation.Operand{Role: "callee-type", Term: equation.ClosedTerm([]byte("type/" + strconv.Quote(typeName)))})
	}
	for index, ref := range body.TypeRefs(instruction.CallTypeArgs) {
		typeName := body.TypeDisplay(ref)
		if typeName == "" {
			return nil, fmt.Errorf("empty type argument %d", index)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("type-argument", index), Term: equation.ClosedTerm([]byte("type/" + strconv.Quote(typeName)))})
	}
	if instruction.Check != 0 {
		check, err := callCheckTerm(body.Check(instruction.Check))
		if err != nil {
			return nil, err
		}
		operands = append(operands, equation.Operand{Role: "check", Term: check})
	}
	operands = append(operands,
		equation.Operand{Role: "context", Term: equation.ClosedTerm([]byte("call-context/" + strconv.FormatUint(uint64(instruction.CallContext), 10)))},
		equation.Operand{Role: "list-spread", Term: boolTerm(instruction.ListSpread)},
		equation.Operand{Role: "result-spread", Term: boolTerm(instruction.ResultSpread)},
		equation.Operand{Role: "final", Term: boolTerm(instruction.CallFinal)},
		equation.Operand{Role: "expanded", Term: boolTerm(instruction.CallExpanded)},
		equation.Operand{Role: "adjusted", Term: boolTerm(instruction.CallAdjusted)},
		equation.Operand{Role: "open-tail", Term: boolTerm(instruction.CallOpenTail)},
		equation.Operand{Role: "condition-negated", Term: boolTerm(instruction.CallConditionNegated)},
	)
	return operands, nil
}

// externalProvider recognizes providers whose callable identity lives outside
// this body.  Local closures deliberately remain ordinary calls: converting
// them into an external boundary would erase their body-local identity.
func externalProvider(body *wir.Body, instruction wir.Instruction) (equation.Term, bool) {
	if body == nil || instruction.Op != wir.OpCall {
		return equation.Term{}, false
	}
	operand := instruction.Call.Callee
	if instruction.Call.Method != 0 {
		operand = instruction.Call.Receiver
	}
	if operand.Kind != wir.OperandPath {
		return equation.Term{}, false
	}
	path := body.Path(wir.PathRef(operand.Ref))
	if path.IsEmpty() || path.Symbol == 0 {
		return equation.Term{}, false
	}
	root := path.RootOnly()
	if module, ok := body.SymbolRequireModulePath(root.Symbol); ok {
		return equation.ClosedTerm([]byte("provider/module/" + strconv.Quote(module))), true
	}
	kind, global := body.SymbolKind(root.Symbol)
	if !global && kind != wir.SymbolGlobal && !body.IsImplicitGlobalSymbol(root.Symbol) {
		return equation.Term{}, false
	}
	name := path.String()
	if name == "" {
		return equation.Term{}, false
	}
	return equation.ClosedTerm([]byte("provider/global/" + strconv.Quote(name))), true
}

// externalCallOperands closes the external boundary's entire source-side
// input.  Result ownership stays with call-results; this factor proves the
// provider boundary and preserves argument/result-shape distinctions for its
// eventual provider implementation.
func externalCallOperands(body *wir.Body, instruction wir.Instruction, apply equation.Coordinate, provider equation.Term) ([]equation.Operand, error) {
	if provider.Entry || len(provider.Encoding) == 0 || apply.Name == "" {
		return nil, fmt.Errorf("incomplete external call boundary")
	}
	operands := []equation.Operand{
		{Role: "application", Term: equation.ClosedTerm([]byte("call/" + apply.Name))},
		{Role: "provider", Term: provider},
		{Role: "argument-spread", Term: boolTerm(instruction.ListSpread)},
		{Role: "result-arity", Term: equation.ClosedTerm([]byte(strconv.Itoa(int(instruction.Results.Len))))},
		{Role: "result-spread", Term: boolTerm(instruction.ResultSpread)},
		{Role: "context", Term: equation.ClosedTerm([]byte("call-context/" + strconv.FormatUint(uint64(instruction.CallContext), 10)))},
	}
	for index, argument := range body.Operands(instruction.List) {
		term, err := scalarTerm(body, argument)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("argument", index), Term: term})
	}
	if instruction.Call.Method != 0 {
		receiver, err := scalarTerm(body, instruction.Call.Receiver)
		if err != nil {
			return nil, fmt.Errorf("receiver: %w", err)
		}
		method := body.Const(instruction.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, fmt.Errorf("malformed method selector")
		}
		operands = append(operands,
			equation.Operand{Role: "receiver", Term: receiver},
			equation.Operand{Role: "method", Term: equation.ClosedTerm([]byte("method/" + strconv.Quote(method.Str)))},
		)
	}
	return operands, nil
}

// publicationOperands resolves the normalized return inventory to stable
// slots. ListSpread records how the final producer was evaluated; the List
// already carries the adjusted result coordinates consumed by publication.
// In particular, the head result of an open tail is a real, exact slot rather
// than a reason to discard the entire return operation.
func publicationOperands(body *wir.Body, instruction wir.Instruction) ([]equation.Operand, error) {
	values := body.Operands(instruction.List)
	operands := make([]equation.Operand, 0, len(values))
	for index, value := range values {
		term, err := scalarTerm(body, value)
		if err != nil {
			return nil, fmt.Errorf("value %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("return-value", index), Term: term})
	}
	return operands, nil
}

func callResultOperands(body *wir.Body, instruction wir.Instruction, apply equation.Coordinate) ([]equation.Operand, error) {
	results := body.Operands(instruction.Results)
	targets := make([]wir.CallResultTarget, len(results))
	completeTargets := len(results) != 0
	for index := range results {
		target, ok := body.CallResultTarget(instruction.Point, index)
		if !ok {
			completeTargets = false
			continue
		}
		targets[index] = target
	}
	// A call result carrier is useful even when syntax has no representable
	// consumer (for example, generic-for iterator tuple setup).  Preserve every
	// result as Top, but only emit target metadata when it is complete: a partial
	// target tuple would incorrectly certify a selective result flow.
	operands := make([]equation.Operand, 1, 1+len(results)*2)
	operands[0] = equation.Operand{Role: "application", Term: equation.ClosedTerm([]byte("call/" + apply.Name))}
	for index, result := range results {
		term, err := scalarTerm(body, result)
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("result", index), Term: term})
		if completeTargets {
			target, err := callResultTargetTerm(targets[index])
			if err != nil {
				return nil, fmt.Errorf("result target %d: %w", index, err)
			}
			operands = append(operands, equation.Operand{Role: indexedRole("target", index), Term: target})
		}
	}
	return operands, nil
}

func indexedRole(prefix string, index int) string { return fmt.Sprintf("%s-%08d", prefix, index) }

func boolTerm(value bool) equation.Term {
	return equation.ClosedTerm([]byte("scalar/bool/" + strconv.FormatBool(value)))
}

func callCheckTerm(check wir.Check) (equation.Term, error) {
	if check.Kind == wir.CheckNone {
		return equation.Term{}, fmt.Errorf("empty normalized call check")
	}
	return equation.ClosedTerm([]byte(fmt.Sprintf("check/%d/path/%s/other/%s/type/%q/literal/%q/string/%q/len/%d/floor/%d/ceil/%d/has-ceil/%t/ceil-negated/%t/negated/%t/producer/%d/has-producer/%t",
		check.Kind, check.Path.Key(), check.OtherPath.Key(), check.TypeName, fmt.Sprint(check.Literal), check.LiteralString, check.LenFloor, check.NumFloor, check.NumCeil, check.HasNumCeil, check.NumCeilNegated, check.Negated, check.ProducerPoint, check.HasProducerPoint))), nil
}

func callResultTargetTerm(target wir.CallResultTarget) (equation.Term, error) {
	if target.Index < 0 || target.ResultIndex < 0 {
		return equation.Term{}, fmt.Errorf("negative result index")
	}
	base := fmt.Sprintf("result-target/%d/index/%d/result/%d", target.Kind, target.Index, target.ResultIndex)
	if !target.Path.IsEmpty() {
		if target.Path.Key() == "" {
			return equation.Term{}, fmt.Errorf("empty target path key")
		}
		base += "/path/" + string(target.Path.Key())
	}
	switch target.Kind {
	case wir.CallResultTargetLocalAssignment, wir.CallResultTargetOrdinaryAssignment, wir.CallResultTargetReturn, wir.CallResultTargetExpression:
		return equation.ClosedTerm([]byte(base)), nil
	default:
		return equation.Term{}, fmt.Errorf("unknown result target kind %d", target.Kind)
	}
}

type allocationOperandSets struct {
	template        []equation.Operand
	materialization []equation.Operand
}

// allocationOperands seals the whole syntactic allocation before either of its
// equation occurrences is emitted. An open table tail is admitted as its
// source-owned final producer, but deliberately cannot certify a finite object
// graph: materialization receives its exact open-tail marker and the front
// withholds closed-table shape facts.
func allocationOperands(body *wir.Body, instruction wir.Instruction, allocationSite string) (allocationOperandSets, error) {
	result, err := allocationValueTerm(body, instruction.Dst)
	if err != nil {
		return allocationOperandSets{}, fmt.Errorf("destination: %w", err)
	}
	if allocationSite == "" {
		return allocationOperandSets{}, fmt.Errorf("missing allocation site")
	}
	site := equation.ClosedTerm([]byte("allocation-site/" + allocationSite))
	sets := allocationOperandSets{
		template: []equation.Operand{
			{Role: "site", Term: site},
			{Role: "result", Term: result},
		},
		materialization: []equation.Operand{
			{Role: "site", Term: site},
			{Role: "result", Term: result},
		},
	}
	switch instruction.Op {
	case wir.OpMakeTable:
		if !instruction.StaticStringKeysComplete {
			return allocationOperandSets{}, fmt.Errorf("table constructor has a non-exact key")
		}
		typeTerm, err := allocationTypeTerm(body, instruction.Type)
		if err != nil {
			return allocationOperandSets{}, err
		}
		sets.template = append(sets.template,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("allocation-kind/table"))},
			equation.Operand{Role: "type", Term: typeTerm},
		)
		sets.materialization = append(sets.materialization,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("object-kind/table"))},
			equation.Operand{Role: "list-floor", Term: listFloorTerm(body, instruction)},
		)
		if instruction.ListSpread {
			values := body.Operands(instruction.List)
			if len(values) == 0 {
				return allocationOperandSets{}, fmt.Errorf("table constructor has an empty open final value tail")
			}
			tail, err := allocationValueTerm(body, values[len(values)-1])
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("table constructor open tail: %w", err)
			}
			// The marker is part of both frozen allocation occurrences. The
			// materializer can retain the exact source producer without treating
			// unknown arity as an absent member or a closed array bound.
			sets.template = append(sets.template,
				equation.Operand{Role: "open-tail", Term: boolTerm(true)},
				equation.Operand{Role: "tail", Term: tail},
			)
			sets.materialization = append(sets.materialization,
				equation.Operand{Role: "open-tail", Term: boolTerm(true)},
				equation.Operand{Role: "tail", Term: tail},
			)
		} else {
			sets.template = append(sets.template, equation.Operand{Role: "open-tail", Term: boolTerm(false)})
			sets.materialization = append(sets.materialization, equation.Operand{Role: "open-tail", Term: boolTerm(false)})
		}
		for index, entry := range body.TableEntries(instruction.TableEntries) {
			if len(entry.Suffix.Segments) == 0 {
				return allocationOperandSets{}, fmt.Errorf("table entry %d has no exact suffix", index)
			}
			value, err := allocationValueTerm(body, entry.Value)
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("table entry %d: %w", index, err)
			}
			if isNilConstant(body, entry.Value) {
				// Lua removes a key assigned nil.  Do not encode an object member
				// for it: absence is a distinct state, not a Bottom value.
				continue
			}
			sets.materialization = append(sets.materialization, equation.Operand{
				Role: fmt.Sprintf("member-%08d", index),
				Term: equation.ClosedTerm([]byte("member/" + segment.FormatSegments(entry.Suffix.Segments) + "/" + string(value.Encoding))),
			})
		}
		for index, valueOperand := range body.Operands(instruction.List) {
			value, err := allocationValueTerm(body, valueOperand)
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("table value %d: %w", index, err)
			}
			sets.template = append(sets.template, equation.Operand{
				Role: fmt.Sprintf("value-%08d", index), Term: value,
			})
		}
	case wir.OpClosure:
		proto := body.Proto(instruction.Func)
		if instruction.Func == 0 || proto.Body == nil || proto.Graph == nil || proto.Name == "" {
			return allocationOperandSets{}, fmt.Errorf("closure has no complete nested prototype")
		}
		sets.template = append(sets.template,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("allocation-kind/closure"))},
			equation.Operand{Role: "prototype", Term: equation.ClosedTerm([]byte("prototype/" + proto.Name))},
		)
		sets.materialization = append(sets.materialization,
			equation.Operand{Role: "kind", Term: equation.ClosedTerm([]byte("object-kind/closure"))},
			equation.Operand{Role: "prototype", Term: equation.ClosedTerm([]byte("prototype/" + proto.Name))},
		)
		for index, capture := range body.Operands(instruction.List) {
			value, err := allocationValueTerm(body, capture)
			if err != nil {
				return allocationOperandSets{}, fmt.Errorf("closure capture %d: %w", index, err)
			}
			sets.materialization = append(sets.materialization, equation.Operand{
				Role: fmt.Sprintf("capture-%08d", index), Term: value,
			})
		}
	default:
		return allocationOperandSets{}, fmt.Errorf("instruction %d does not allocate an object", instruction.Op)
	}
	return sets, nil
}

func allocationValueTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	if term, err := scalarTerm(body, operand); err == nil {
		return term, nil
	}
	switch operand.Kind {
	case wir.OperandTemp:
		return equation.ClosedTerm([]byte(fmt.Sprintf("temp/%08d", operand.Ref))), nil
	case wir.OperandVararg:
		return equation.ClosedTerm([]byte("vararg")), nil
	default:
		return equation.Term{}, fmt.Errorf("operand kind %d is not a sealed value", operand.Kind)
	}
}

// allocationWriteOperands closes the value produced by every constructor.
func allocationWriteOperands(body *wir.Body, instruction wir.Instruction, current operation, operations []operation) ([]equation.Operand, error) {
	target, err := allocationTargetTerm(body, instruction.Dst)
	if err != nil {
		return nil, err
	}
	value := "scalar/table"
	if instruction.Op == wir.OpClosure {
		proto := body.Proto(instruction.Func)
		value = functionValue(proto.Type)
	} else if shape, ok, err := tableShapeTerm(body, instruction); err != nil {
		return nil, err
	} else if ok {
		value = string(shape)
	}
	readBefore, err := precedingReadBoundary(current, operations)
	if err != nil {
		return nil, err
	}
	return []equation.Operand{
		{Role: "target", Term: target},
		{Role: "display", Term: hiddenAllocationDisplay(current.target)},
		{Role: "value", Term: equation.ClosedTerm([]byte(value))},
		{Role: "read-before", Term: readBefore},
		{Role: "absence", Term: equation.ClosedTerm([]byte("front/absence/error"))},
	}, nil
}

// tableShapeTerm turns the WIR constructor inventory into a closed, finite
// value fact. It deliberately declines an open tail or unclassified key: those
// shapes have no complete member-presence proof.
func tableShapeTerm(body *wir.Body, instruction wir.Instruction) ([]byte, bool, error) {
	if instruction.Op != wir.OpMakeTable || !instruction.StaticStringKeysComplete || instruction.ListSpread {
		return nil, false, nil
	}
	bySuffix := make(map[string]shapefact.Member)
	for _, entry := range body.TableEntries(instruction.TableEntries) {
		suffix := segment.FormatSegments(entry.Suffix.Segments)
		if suffix == "" {
			return nil, false, fmt.Errorf("table member has no suffix")
		}
		member := shapefact.Member{Suffix: suffix}
		if !isNilConstant(body, entry.Value) {
			value, err := allocationValueTerm(body, entry.Value)
			if err != nil {
				return nil, false, err
			}
			member.Present, member.Value = true, string(value.Encoding)
		}
		// Lua constructor writes are ordered; the final duplicate key wins.
		bySuffix[suffix] = member
	}
	members := make([]shapefact.Member, 0, len(bySuffix))
	for _, member := range bySuffix {
		members = append(members, member)
	}
	shape, ok := shapefact.EncodeTable(shapefact.Table{Closed: true, Members: members})
	return shape, ok, nil
}

// functionValue seals the callable shape into the constructor's ordinary
// value fact.  It is deliberately a closed transport term: apply later reads
// that fact through the equation partition, rather than consulting WIR or
// re-analysing source.
func functionValue(t typ.Type) string {
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil {
		return "scalar/function"
	}
	type signature struct {
		Params   []string `json:"params"`
		Required int      `json:"required"`
		Variadic bool     `json:"variadic"`
	}
	wire := signature{Params: make([]string, len(fn.Params)), Variadic: fn.Variadic != nil}
	for index, param := range fn.Params {
		if param.Type == nil {
			return "scalar/function"
		}
		wire.Params[index] = param.Type.String()
		// Lua's annotated optional parameter surface (T?) is callable with an
		// omitted trailing argument even when the parser has no default-value
		// marker on the parameter slot.
		if !param.Optional && !strings.HasSuffix(wire.Params[index], "?") {
			wire.Required++
		}
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return "scalar/function"
	}
	return "scalar/function/" + base64.RawURLEncoding.EncodeToString(encoded)
}

// allocationEntryWriteOperands projects a closed constructor entry onto its
// root path. The lowering layer already flattens nested static table entries.
func allocationEntryWriteOperands(body *wir.Body, instruction wir.Instruction, current operation, operations []operation) ([]equation.Operand, error) {
	if instruction.Dst.Kind != wir.OperandPath || current.allocationEntry == nil {
		return nil, fmt.Errorf("missing static table entry target")
	}
	root := body.Path(wir.PathRef(instruction.Dst.Ref))
	targetPath := root.AppendPathSuffix(current.allocationEntry.Suffix)
	target, display, err := closedPathTerm(targetPath)
	if err != nil {
		return nil, err
	}
	value, err := allocationValueTerm(body, current.allocationEntry.Value)
	if err != nil {
		return nil, fmt.Errorf("entry value: %w", err)
	}
	readBefore, err := precedingReadBoundary(current, operations)
	if err != nil {
		return nil, err
	}
	return []equation.Operand{
		{Role: "target", Term: target},
		{Role: "display", Term: equation.ClosedTerm([]byte("front/hidden/allocation/" + display))},
		{Role: "value", Term: value},
		{Role: "read-before", Term: readBefore},
		{Role: "absence", Term: equation.ClosedTerm([]byte("front/absence/error"))},
	}, nil
}

func allocationTargetTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	switch operand.Kind {
	case wir.OperandPath:
		term, _, err := pathTerm(body, operand)
		return term, err
	case wir.OperandTemp:
		return equation.ClosedTerm([]byte("temp/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
	default:
		return equation.Term{}, fmt.Errorf("destination is operand kind %d", operand.Kind)
	}
}

func closedPathTerm(value path.Path) (equation.Term, string, error) {
	if value.IsEmpty() || value.Key() == "" || value.String() == "" {
		return equation.Term{}, "", fmt.Errorf("empty static table entry path")
	}
	return equation.ClosedTerm([]byte("path/" + value.Key())), value.String(), nil
}

func precedingReadBoundary(current operation, operations []operation) (equation.Term, error) {
	for index, candidate := range operations {
		if candidate.target != current.target {
			continue
		}
		if index == 0 {
			return equation.Term{}, fmt.Errorf("write has no predecessor")
		}
		return equation.ClosedTerm([]byte("front/read-before/" + operations[index-1].target.Name)), nil
	}
	return equation.Term{}, fmt.Errorf("write operation is absent")
}

func hiddenAllocationDisplay(target equation.Coordinate) equation.Term {
	return equation.ClosedTerm([]byte("front/hidden/allocation/" + target.Name))
}

func allocationTypeTerm(body *wir.Body, ref wir.TypeRef) (equation.Term, error) {
	if ref == 0 {
		return equation.ClosedTerm([]byte("type/none")), nil
	}
	display := body.TypeDisplay(ref)
	if display == "" {
		return equation.Term{}, fmt.Errorf("unknown table type")
	}
	return equation.ClosedTerm([]byte("type/" + display)), nil
}

func isNilConstant(body *wir.Body, operand wir.Operand) bool {
	return operand.Kind == wir.OperandConst && body.Const(wir.ConstRef(operand.Ref)).Kind == wir.ConstNil
}

// listFloorTerm reports only a proven contiguous prefix.  It never treats a
// missing, nil, or non-positive element as a list member, preserving the
// distinction between an absent key and a present nil value.
func listFloorTerm(body *wir.Body, instruction wir.Instruction) equation.Term {
	floor := 0
	entries := body.TableEntries(instruction.TableEntries)
	for {
		found := false
		for _, entry := range entries {
			if !exactPositiveIndex(entry, floor+1) || isNilConstant(body, entry.Value) {
				continue
			}
			found = true
			break
		}
		if !found {
			break
		}
		floor++
	}
	return equation.ClosedTerm([]byte(fmt.Sprintf("list-floor/%d", floor)))
}

func exactPositiveIndex(entry wir.TableEntry, index int) bool {
	return len(entry.Suffix.Segments) == 1 &&
		entry.Suffix.Segments[0].Kind == segment.SegmentIndexInt &&
		entry.Suffix.Segments[0].Index == index
}

func guardsForPoint(graph cfg.Graph, reachability *reachabilityCache, point cfg.Point, body equation.BodyID, branches map[cfg.Point]equation.Coordinate) []equation.Guard {
	guards := make([]equation.Guard, 0, len(branches))
	for branch, target := range branches {
		if branch == point {
			continue
		}
		trueReach, falseReach := false, false
		for _, successor := range graph.Successors(branch) {
			condition, isBranchEdge := graph.EdgeCond(branch, successor)
			if !isBranchEdge || !reachability.reaches(successor, point) {
				continue
			}
			if condition {
				trueReach = true
			} else {
				falseReach = true
			}
		}
		if trueReach == falseReach {
			continue
		}
		edge := "false"
		if trueReach {
			edge = "true"
		}
		guards = append(guards, equation.Guard{Body: body, Encoding: []byte("front/branch/" + target.Name + "/" + edge)})
	}
	return guards
}

func graphHasCycle(graph cfg.Graph) bool {
	visiting := make(map[cfg.Point]bool, graph.Size())
	visited := make(map[cfg.Point]bool, graph.Size())
	var visit func(cfg.Point) bool
	visit = func(point cfg.Point) bool {
		if visiting[point] {
			return true
		}
		if visited[point] {
			return false
		}
		visiting[point] = true
		for _, next := range graph.Successors(point) {
			if visit(next) {
				return true
			}
		}
		visiting[point] = false
		visited[point] = true
		return false
	}
	return visit(graph.Entry())
}

// freezeCyclicArtifact translates the already-admitted equation stream and
// CFG topology into a closed cyclic certificate. The resulting WTO is
// computed once here and retained verbatim by the evaluator -- execution
// never discovers or rebuilds a schedule.
func freezeCyclicArtifact(artifact equation.Artifact, body *wir.Body, graph cfg.Graph) (equation.CyclicArtifact, error) {
	if len(artifact.Equations) == 0 {
		return equation.CyclicArtifact{}, fmt.Errorf("front: cannot freeze an empty cyclic artifact")
	}
	cells := make([]equation.CellID, 0, len(artifact.Equations))
	byTarget := make(map[equation.Coordinate]equation.CellID, len(artifact.Equations))
	for _, operation := range artifact.Equations {
		cell := equation.CellID("front/" + operation.Target.Name)
		cells = append(cells, cell)
		byTarget[operation.Target] = cell
	}
	pointCells, err := cyclicOperationCells(artifact, body, graph, byTarget)
	if err != nil {
		return equation.CyclicArtifact{}, err
	}
	edges := make(map[equation.CellID][]equation.CellID, len(cells))
	dependencies := make([]equation.SemanticDependency, 0, len(cells))
	for _, operation := range artifact.Equations {
		to := byTarget[operation.Target]
		for _, target := range operation.Dependencies {
			from, ok := byTarget[target]
			if !ok {
				return equation.CyclicArtifact{}, fmt.Errorf("front: cyclic dependency %s has no cell", target.Name)
			}
			edges[from] = append(edges[from], to)
			dependencies = append(dependencies, equation.SemanticDependency{From: from, To: to, Reason: equation.EdgeContractRead, Evidence: "front/operation-order"})
		}
	}
	for point, sources := range pointCells {
		from := sources[len(sources)-1]
		for _, next := range graph.Successors(point) {
			for _, to := range cyclicReachableOperationCells(graph, next, pointCells) {
				edges[from] = append(edges[from], to)
				dependencies = append(dependencies, equation.SemanticDependency{From: from, To: to, Reason: equation.EdgeContractAdvance, Evidence: "front/cfg-edge"})
			}
		}
	}
	plan := solve.NewWTOPlan(cells, func(cell equation.CellID) []equation.CellID {
		return append([]equation.CellID(nil), edges[cell]...)
	})
	cyclic, err := equation.NewCyclicArtifact(artifact, byTarget, plan, dependencies,
		[]equation.OutputSelector{{ID: "published", Cells: append([]equation.CellID(nil), cells...)}},
		[]equation.CellID{cells[0]}, append([]equation.CellID(nil), cells...))
	if err != nil {
		return equation.CyclicArtifact{}, fmt.Errorf("front: freeze cyclic artifact: %w", err)
	}
	return cyclic, nil
}

// cyclicOperationCells repeats only the front's operation cardinality pass.
// The produced coordinates are the already-compiled operation names, so this
// cannot create a second lowering or infer an alternate equation topology.
func cyclicOperationCells(artifact equation.Artifact, body *wir.Body, graph cfg.Graph, byTarget map[equation.Coordinate]equation.CellID) (map[cfg.Point][]equation.CellID, error) {
	if body == nil || graph == nil || len(artifact.Equations) == 0 {
		return nil, fmt.Errorf("front: cyclic operation map has no body")
	}
	loopBindings, err := genericForBindings(body, graph)
	if err != nil {
		return nil, err
	}
	loopBindingPoints := make(map[cfg.Point]bool)
	for _, bindings := range loopBindings {
		for _, binding := range bindings {
			loopBindingPoints[binding.point] = true
		}
	}
	for point := range numericForBindingPoints(body) {
		loopBindingPoints[point] = true
	}
	bodyID := artifact.Equations[0].Target.Body
	points := make(map[cfg.Point][]equation.CellID)
	index := 0
	appendAt := func(point cfg.Point, count int) error {
		for offset := 0; offset < count; offset++ {
			target := equation.Coordinate{Body: bodyID, Name: operationName(index)}
			cell, ok := byTarget[target]
			if !ok {
				return fmt.Errorf("front: cyclic operation %s has no compiled cell", target.Name)
			}
			points[point] = append(points[point], cell)
			index++
		}
		return nil
	}
	for instructionIndex := 0; instructionIndex < body.Len(); instructionIndex++ {
		instruction := body.Instr(instructionIndex)
		count := 0
		switch instruction.Op {
		case wir.OpEntry, wir.OpStaticMemberWrite, wir.OpDynamicIndexRead, wir.OpBranch, wir.OpClaim, wir.OpSelect, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpLogical, wir.OpAssign, wir.OpIterate, wir.OpReturn:
			count = 1
			if instruction.Op == wir.OpAssign && instruction.A.Kind == wir.OperandNone && loopBindingPoints[instruction.Point] {
				count = 0
			}
		case wir.OpDynamicIndexWrite:
			count = 2
		case wir.OpMakeTable, wir.OpClosure:
			count = 3
			if instruction.Op == wir.OpMakeTable && instruction.Dst.Kind == wir.OperandPath {
				count += len(body.TableEntries(instruction.TableEntries))
			}
		case wir.OpCall:
			count = 2
			if _, external := externalProvider(body, instruction); external {
				count++
			}
		case wir.OpExit, wir.OpNoop:
		default:
			return nil, fmt.Errorf("front: cyclic operation map: %w: %d", ErrUnsupportedInstruction, instruction.Op)
		}
		if err := appendAt(instruction.Point, count); err != nil {
			return nil, err
		}
	}
	if index != len(artifact.Equations) {
		return nil, fmt.Errorf("front: cyclic operation map has %d cells, want %d", index, len(artifact.Equations))
	}
	return points, nil
}

func cyclicReachableOperationCells(graph cfg.Graph, start cfg.Point, points map[cfg.Point][]equation.CellID) []equation.CellID {
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{start}
	var cells []equation.CellID
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[point] {
			continue
		}
		seen[point] = true
		if atPoint := points[point]; len(atPoint) != 0 {
			cells = append(cells, atPoint[0])
			continue
		}
		stack = append(stack, graph.Successors(point)...)
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i] < cells[j] })
	return cells
}

// reachabilityCache shares each successor's graph walk across every operation
// that needs branch guards. Large straight-line fixtures otherwise repeat the
// same O(branches*points) traversal for every draft.
type reachabilityCache struct {
	graph cfg.Graph
	from  map[cfg.Point]map[cfg.Point]bool
}

func newReachabilityCache(graph cfg.Graph) *reachabilityCache {
	return &reachabilityCache{graph: graph, from: make(map[cfg.Point]map[cfg.Point]bool)}
}

func (cache *reachabilityCache) reaches(from, target cfg.Point) bool {
	reachable, found := cache.from[from]
	if !found {
		reachable = make(map[cfg.Point]bool, cache.graph.Size())
		stack := []cfg.Point{from}
		for len(stack) != 0 {
			point := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if reachable[point] {
				continue
			}
			reachable[point] = true
			stack = append(stack, cache.graph.Successors(point)...)
		}
		cache.from[from] = reachable
	}
	return reachable[target]
}
