package front

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
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
	pathInvalidationKernel      = "front/path-invalidation/v1"
	indexMutationKernel         = "front/index-mutation/v1"
	branchKernel                = "front/branch-relations/v1"
	applyKernel                 = "front/apply/v1"
	resultsKernel               = "front/call-results/v1"
	genericForKernel            = "front/generic-for/v1"
	selectKernel                = "front/channel-select/v1"
	entryName                   = "entry"
)

// CompileBody parses source and lowers its chunk through bind, cfgbuild, and
// wirlower before compiling the resulting complete equation source. The
// walking skeleton admits only the structural entry operation; later families
// are added explicitly rather than being skipped.
func CompileBody(source string) (equation.Artifact, error) {
	stmts, err := parse.ParseString(source, "<front>")
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: parse body: %w", err)
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
		return equation.Artifact{}, fmt.Errorf("front: build CFG")
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	if body == nil {
		return equation.Artifact{}, fmt.Errorf("front: lower WIR")
	}
	return compileWIR(source, body, built.Graph, assignmentSnapshotStarts(stmts, built))
}

type operation struct {
	instruction    wir.Instruction
	target         equation.Coordinate
	family         string
	allocationSite string
	callResults    bool
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
	if graphHasCycle(graph) && len(loopBindings) == 0 {
		return equation.Artifact{}, fmt.Errorf("front: cyclic CFG is outside the generic-for walking slice")
	}
	loopBindingPoints := make(map[cfg.Point]bool)
	for _, bindings := range loopBindings {
		for _, binding := range bindings {
			loopBindingPoints[binding.point] = true
		}
	}
	operations := make([]operation, 0, body.Len())
	entries := 0
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpEntry, wir.OpStaticMemberWrite, wir.OpDynamicIndexRead, wir.OpBranch, wir.OpSelect:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
			if instruction.Op == wir.OpEntry {
				entries++
			}
		case wir.OpAssign:
			if instruction.A.Kind == wir.OperandNone {
				if loopBindingPoints[instruction.Point] {
					continue
				}
				return equation.Artifact{}, fmt.Errorf("front: assignment at point %d has no value source", instruction.Point)
			}
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
		case wir.OpIterate:
			if instruction.Iter != wir.IterGeneric {
				return equation.Artifact{}, fmt.Errorf("front: iterate at point %d has kind %d, want generic", instruction.Point, instruction.Iter)
			}
			if len(loopBindings[instruction.Point]) == 0 {
				return equation.Artifact{}, fmt.Errorf("front: generic-for at point %d has no bound variables", instruction.Point)
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
			)
		case wir.OpCall:
			// Application and result materialization are an inseparable ordered
			// pair: a partial application cannot expose an unowned result.
			operations = append(operations,
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}},
				operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations) + 1)}, callResults: true},
			)
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
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
			if operation.family == "allocation-template" {
				draft.Operands = terms.template
			} else {
				draft.Operands = terms.materialization
			}
		case operation.family == "path-invalidation" || operation.family == "index-mutation":
			container, _, err := pathTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: dynamic index write %s: %w", operation.target.Name, err)
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
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
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
			target, display, err := pathTerm(body, instruction.Dst)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			value, err := scalarTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("environment-write")
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
			readBefore, err := readBeforeTerm(operation, operations, snapshots)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: assignment %s: %w", operation.target.Name, err)
			}
			absence := "front/absence/error"
			if instruction.A.Kind == wir.OperandPath && implicitGlobalPath(body, instruction.A) {
				// Lua resolves an unread, implicit global to nil.  This is an
				// explicit source rule, not a fallback for a missing local fact.
				absence = "front/absence/nil"
			}
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
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "value", Term: value},
			}
		case instruction.Op == wir.OpDynamicIndexRead:
			target, display, err := pathTerm(body, instruction.Dst)
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
			draft.Occurrence = occurrence("path-replacement")
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "container", Term: container},
				{Role: "key", Term: key},
			}
		case instruction.Op == wir.OpBranch:
			draft.Occurrence = occurrence("branch-relations")
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
			operands, err := branchOperands(body, instruction)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: branch %s: %w", operation.target.Name, err)
			}
			draft.Operands = operands
		case instruction.Op == wir.OpCall:
			if !operation.callResults {
				operands, err := applyOperands(body, instruction)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: call %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("apply")
				draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			} else {
				apply := operations[index-1]
				operands, err := callResultOperands(body, instruction, apply.target)
				if err != nil {
					return equation.Artifact{}, fmt.Errorf("front: call results %s: %w", operation.target.Name, err)
				}
				draft.Occurrence = occurrence("call-results")
				draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
				draft.Operands = operands
			}
		case instruction.Op == wir.OpIterate:
			operands, err := genericForOperands(body, instruction, loopBindings[instruction.Point])
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: generic-for %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("generic-for")
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
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
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
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
	compiler, err = compiler.With("generic-for", equation.BindExistingKernel(genericForKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure generic-for compiler: %w", err)
	}
	compiler, err = compiler.With("channel-select", equation.BindExistingKernel(selectKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure channel-select compiler: %w", err)
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
		return equation.Term{}, fmt.Errorf("missing assignment snapshot boundary at CFG point %d", current.instruction.Point)
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
	case "entry", "environment-write", "allocation-template", "object-materialization", "path-replacement", "path-invalidation", "index-mutation", "branch-relations", "apply", "call-results", "generic-for", "channel-select":
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
	case "generic-for":
		return genericForKernel, true
	case "channel-select":
		return selectKernel, true
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

func genericForOperands(body *wir.Body, instruction wir.Instruction, bindings []loopBinding) ([]equation.Operand, error) {
	if instruction.Iter != wir.IterGeneric {
		return nil, fmt.Errorf("iterator kind %d is not generic", instruction.Iter)
	}
	if instruction.ListSpread {
		return nil, fmt.Errorf("open iterator result tail has no closed generic-for tuple")
	}
	sources := body.Operands(instruction.List)
	if len(sources) == 0 {
		return nil, fmt.Errorf("iterator has no source values")
	}
	roles := []string{"iterator", "state", "control"}
	operands := make([]equation.Operand, 0, len(roles)+2*len(bindings))
	for index, role := range roles {
		term := equation.ClosedTerm([]byte("scalar/nil"))
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
// therefore an error.  A temp is a closed body-local reference, not a guessed
// fallback value.
func pathStoreTerm(body *wir.Body, operand wir.Operand) (equation.Term, error) {
	switch operand.Kind {
	case wir.OperandTemp:
		if operand.Ref == 0 {
			return equation.Term{}, fmt.Errorf("zero temporary operand")
		}
		return equation.ClosedTerm([]byte("temporary/" + strconv.FormatUint(uint64(operand.Ref), 10))), nil
	case wir.OperandVararg:
		return equation.ClosedTerm([]byte("vararg")), nil
	default:
		return scalarTerm(body, operand)
	}
}

func suffixTerm(body *wir.Body, suffix wir.SegmentRange) equation.Term {
	return equation.ClosedTerm([]byte("suffix/" + segment.FormatSegments(body.Segments(suffix))))
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
	}
	for index, argument := range body.Operands(instruction.List) {
		term, err := scalarTerm(body, argument)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		operands = append(operands, equation.Operand{Role: indexedRole("argument", index), Term: term})
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

func callResultOperands(body *wir.Body, instruction wir.Instruction, apply equation.Coordinate) ([]equation.Operand, error) {
	results := body.Operands(instruction.Results)
	targets := body.CallResultTargets(instruction.Point)
	if len(targets) != len(results) {
		return nil, fmt.Errorf("result target count %d, want %d", len(targets), len(results))
	}
	operands := make([]equation.Operand, 1, 1+len(results)*2)
	operands[0] = equation.Operand{Role: "application", Term: equation.ClosedTerm([]byte("call/" + apply.Name))}
	for index, result := range results {
		term, err := scalarTerm(body, result)
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", index, err)
		}
		target, err := callResultTargetTerm(targets[index])
		if err != nil {
			return nil, fmt.Errorf("result target %d: %w", index, err)
		}
		operands = append(operands,
			equation.Operand{Role: indexedRole("result", index), Term: term},
			equation.Operand{Role: indexedRole("target", index), Term: target},
		)
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
// equation occurrences is emitted.  In particular, an open table tail has no
// exact finite object graph, so it is rejected instead of being silently
// represented as an absent field, nil, Bottom, or an invented unknown value.
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
		if instruction.ListSpread {
			return allocationOperandSets{}, fmt.Errorf("table constructor has an open final value tail")
		}
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

func guardsForPoint(graph cfg.Graph, point cfg.Point, body equation.BodyID, branches map[cfg.Point]equation.Coordinate) []equation.Guard {
	guards := make([]equation.Guard, 0, len(branches))
	for branch, target := range branches {
		if branch == point {
			continue
		}
		trueReach, falseReach := false, false
		for _, successor := range graph.Successors(branch) {
			condition, isBranchEdge := graph.EdgeCond(branch, successor)
			if !isBranchEdge || !reachable(graph, successor, point) {
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

func reachable(graph cfg.Graph, from, target cfg.Point) bool {
	seen := make(map[cfg.Point]bool, graph.Size())
	stack := []cfg.Point{from}
	for len(stack) != 0 {
		point := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if point == target {
			return true
		}
		if seen[point] {
			continue
		}
		seen[point] = true
		stack = append(stack, graph.Successors(point)...)
	}
	return false
}
