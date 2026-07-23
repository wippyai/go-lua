package front

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/compiler/parse"
)

// ErrUnsupportedInstruction reports a WIR operation outside the front's
// admitted family. CompileBody never omits such an operation.
var ErrUnsupportedInstruction = errors.New("front: unsupported WIR instruction")

const (
	entryKernel  = "front/entry/v1"
	writeKernel  = "front/environment-write/v1"
	branchKernel = "front/branch-relations/v1"
	entryName    = "entry"
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
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	if built == nil || built.Graph == nil {
		return equation.Artifact{}, fmt.Errorf("front: build CFG")
	}
	body := wirlower.Lower("chunk", stmts, bindings, built)
	if body == nil {
		return equation.Artifact{}, fmt.Errorf("front: lower WIR")
	}
	return compileWIR(source, body, built.Graph)
}

type operation struct {
	instruction wir.Instruction
	target      equation.Coordinate
}

func compileWIR(source string, body *wir.Body, graph cfg.Graph) (equation.Artifact, error) {
	if body == nil || graph == nil {
		return equation.Artifact{}, fmt.Errorf("front: nil WIR body")
	}
	if graphHasCycle(graph) {
		return equation.Artifact{}, fmt.Errorf("front: cyclic CFG is outside the acyclic walking slice")
	}
	bodyID := bodyID(source)
	entry := equation.EntryParameter{Body: bodyID, Name: entryName}
	operations := make([]operation, 0, body.Len())
	entries := 0
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpEntry, wir.OpAssign, wir.OpBranch:
			operations = append(operations, operation{instruction: instruction, target: equation.Coordinate{Body: bodyID, Name: operationName(len(operations))}})
			if instruction.Op == wir.OpEntry {
				entries++
			}
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
		switch instruction.Op {
		case wir.OpEntry:
			draft.Occurrence = occurrence("entry")
			draft.Operands = []equation.Operand{{Role: "entry", Term: equation.EntryTerm(entry)}}
		case wir.OpAssign:
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
			draft.Operands = []equation.Operand{
				{Role: "target", Term: target},
				{Role: "display", Term: equation.ClosedTerm([]byte(display))},
				{Role: "value", Term: value},
			}
		case wir.OpBranch:
			if body.Check(instruction.Check).Kind != wir.CheckNone {
				return equation.Artifact{}, fmt.Errorf("front: branch %s: normalized check kind %d is outside the scalar slice", operation.target.Name, body.Check(instruction.Check).Kind)
			}
			condition, err := scalarTerm(body, instruction.A)
			if err != nil {
				return equation.Artifact{}, fmt.Errorf("front: branch %s: %w", operation.target.Name, err)
			}
			draft.Occurrence = occurrence("branch-relations")
			draft.Guards = guardsForPoint(graph, instruction.Point, bodyID, branchTargets)
			draft.Operands = []equation.Operand{{Role: "condition", Term: condition}}
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
	compiler, err = compiler.With("branch-relations", equation.BindExistingKernel(branchKernel))
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: configure branch compiler: %w", err)
	}
	artifact, err := compiler.Compile(equation.Source{Drafts: drafts})
	if err != nil {
		return equation.Artifact{}, fmt.Errorf("front: compile equations: %w", err)
	}
	return artifact, nil
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
	case "entry", "environment-write", "branch-relations":
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
	case "branch-relations":
		return branchKernel, true
	default:
		return "", false
	}
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
	default:
		return equation.Term{}, fmt.Errorf("operand kind %d is outside the scalar slice", operand.Kind)
	}
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
