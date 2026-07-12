package circuit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/region"
	"github.com/wippyai/go-lua/analysis/semantic/primitive"
	"github.com/wippyai/go-lua/analysis/semantic/program"
)

var ErrInvalidCircuit = errors.New("circuit: invalid program reification")

type BlockTransactionNode struct {
	Block        program.BlockID
	Member       program.MemberID
	Transactions []program.TransactionRef
}

// BlockTransactionNode is reified-only in this POC: evaluation proves circuit
// scheduling and guarded composition but deliberately does not execute slot
// effects until the shared transaction backend is connected.
type ApplyNode struct {
	Block        program.BlockID
	Known        []program.KnownTarget
	Residue      program.TargetResidue
	Completeness program.Completeness
}
type LoopMuNode struct{ Region program.LoopMu }
type CallSCCMuNode struct{ Region program.CallSCCMu }
type IntrinsicNode struct {
	ProgramID string
	Call      primitive.IntrinsicCall
}

// Circuit is the immutable structural reification of one canonical semantic
// program. Intrinsics remain references; evaluation never invokes native code.
type Circuit struct {
	program    program.Program
	blocks     []BlockTransactionNode
	applies    []ApplyNode
	loops      []LoopMuNode
	callSCC    CallSCCMuNode
	intrinsics []IntrinsicNode
	canonical  []byte
	digest     [sha256.Size]byte
}

func Reify(source program.Program, primitives primitive.Registry) (Circuit, error) {
	available := make(map[program.TransactionDigest]struct{})
	var intrinsics []IntrinsicNode
	for _, descriptor := range primitives.Programs() {
		for _, step := range descriptor.Steps {
			if transaction, ok := step.Transaction(); ok {
				available[program.TransactionDigest(transaction.Digest())] = struct{}{}
			}
			if call, ok := step.IntrinsicCall(); ok {
				intrinsics = append(intrinsics, IntrinsicNode{ProgramID: descriptor.ID, Call: call})
			}
		}
	}
	blocks := make([]BlockTransactionNode, 0, len(source.Blocks()))
	for _, block := range source.Blocks() {
		for _, ref := range block.Transactions {
			if _, ok := available[ref.Digest]; !ok {
				return Circuit{}, fmt.Errorf("%w: block %d transaction is absent from primitive registry", ErrInvalidCircuit, block.ID)
			}
		}
		blocks = append(blocks, BlockTransactionNode{Block: block.ID, Member: block.Member, Transactions: append([]program.TransactionRef(nil), block.Transactions...)})
	}
	applies := make([]ApplyNode, 0, len(source.Routes()))
	for _, route := range source.Routes() {
		applies = append(applies, ApplyNode{Block: route.At, Known: append([]program.KnownTarget(nil), route.Known...), Residue: route.Residue, Completeness: route.Completeness})
	}
	loops := make([]LoopMuNode, 0, len(source.Loops()))
	for _, loop := range source.Loops() {
		loops = append(loops, LoopMuNode{Region: loop})
	}
	sort.Slice(intrinsics, func(i, j int) bool {
		if intrinsics[i].ProgramID != intrinsics[j].ProgramID {
			return intrinsics[i].ProgramID < intrinsics[j].ProgramID
		}
		if intrinsics[i].Call.ID != intrinsics[j].Call.ID {
			return intrinsics[i].Call.ID < intrinsics[j].Call.ID
		}
		return intrinsics[i].Call.SchemaVersion < intrinsics[j].Call.SchemaVersion
	})
	wire := struct {
		Program    [sha256.Size]byte
		Primitive  [sha256.Size]byte
		Blocks     []BlockTransactionNode
		Applies    []ApplyNode
		Loops      []LoopMuNode
		CallSCC    CallSCCMuNode
		Intrinsics []IntrinsicNode
	}{source.Digest(), primitives.Digest(), blocks, applies, loops, CallSCCMuNode{source.CallSCC()}, intrinsics}
	canonical, err := json.Marshal(wire)
	if err != nil {
		return Circuit{}, fmt.Errorf("%w: codec: %v", ErrInvalidCircuit, err)
	}
	return Circuit{program: source, blocks: blocks, applies: applies, loops: loops, callSCC: CallSCCMuNode{source.CallSCC()}, intrinsics: intrinsics, canonical: canonical, digest: sha256.Sum256(canonical)}, nil
}

func (c Circuit) BlockNodes() []BlockTransactionNode {
	out := append([]BlockTransactionNode(nil), c.blocks...)
	for i := range out {
		out[i].Transactions = append([]program.TransactionRef(nil), out[i].Transactions...)
	}
	return out
}
func (c Circuit) ApplyNodes() []ApplyNode {
	out := append([]ApplyNode(nil), c.applies...)
	for i := range out {
		out[i].Known = append([]program.KnownTarget(nil), out[i].Known...)
	}
	return out
}
func (c Circuit) LoopNodes() []LoopMuNode {
	out := append([]LoopMuNode(nil), c.loops...)
	for i := range out {
		out[i].Region.Blocks = append([]program.BlockID(nil), out[i].Region.Blocks...)
	}
	return out
}
func (c Circuit) CallSCCNode() CallSCCMuNode {
	out := c.callSCC
	out.Region.Members = append([]program.MemberID(nil), out.Region.Members...)
	return out
}
func (c Circuit) IntrinsicNodes() []IntrinsicNode {
	out := append([]IntrinsicNode(nil), c.intrinsics...)
	for i := range out {
		out[i].Call.Payload = append([]byte(nil), out[i].Call.Payload...)
	}
	return out
}
func (c Circuit) CanonicalBytes() []byte    { return append([]byte(nil), c.canonical...) }
func (c Circuit) Digest() [sha256.Size]byte { return c.digest }

type EvaluationConfig struct {
	Key          CellKey
	Entry        Disjunct
	RouteBinding func(program.KnownTarget) (Disjunct, bool)
	VisitBlock   func(program.BlockID)
}
type EvaluationStats struct{ Cells, Disjuncts, Widens, Transfers, PrecisionLossCells, IntrinsicCalls int }
type EvaluationResult struct {
	Cells     map[program.BlockID]Cell
	Revisions map[program.BlockID]uint64
	Stats     EvaluationStats
}

type evalKind uint8

const (
	evalBottom evalKind = iota
	evalConcrete
	evalTop
)

type evalValue struct {
	kind evalKind
	cell Cell
}

func Evaluate(ctx context.Context, circuit Circuit, bindings *Domain, config EvaluationConfig) (EvaluationResult, error) {
	if bindings == nil || len(circuit.canonical) == 0 || config.RouteBinding == nil {
		return EvaluationResult{}, ErrInvalidCircuit
	}
	entry, err := bindings.Singleton(config.Key, config.Entry)
	if err != nil {
		return EvaluationResult{}, err
	}
	blocks := circuit.program.Blocks()
	cells := make([]program.BlockID, len(blocks))
	for i, b := range blocks {
		cells[i] = b.ID
	}
	successors := make(map[program.BlockID][]program.BlockID, len(cells))
	for _, edge := range circuit.program.Edges() {
		if !containsBlockID(successors[edge.From], edge.To) {
			successors[edge.From] = append(successors[edge.From], edge.To)
		}
	}
	routes := make(map[program.BlockID][]program.KnownTarget, len(circuit.applies))
	for _, route := range circuit.applies {
		routes[route.Block] = append([]program.KnownTarget(nil), route.Known...)
	}
	widenAt := make(map[program.BlockID]bool)
	for _, loop := range circuit.loops {
		widenAt[loop.Region.Entry] = true
	}
	guardedWidens := 0
	var evalErr error
	domain := newEvalLattice(bindings, &guardedWidens, &evalErr)
	join := domain.Join
	result, err := region.Run(ctx, region.System[program.BlockID, evalValue]{Lattice: domain, Cells: cells, Successors: func(cell program.BlockID) []program.BlockID { return successors[cell] }, InitialSparse: func(cell program.BlockID) (evalValue, bool) {
		return evalValue{kind: evalConcrete, cell: entry}, cell == circuit.program.Entry()
	}, WidenAt: func(cell program.BlockID) bool { return widenAt[cell] }, Transfer: func(block program.BlockID, read func(program.BlockID) evalValue, emit func(program.BlockID, evalValue)) {
		if config.VisitBlock != nil {
			config.VisitBlock(block)
		}
		value := read(block)
		if value.kind == evalBottom {
			return
		}
		if value.kind == evalTop {
			evalErr = fmt.Errorf("%w: unexpected evaluator Top at block %d", ErrInvalidCircuit, block)
			return
		}
		for _, target := range routes[block] {
			disjunct, ok := config.RouteBinding(target)
			if !ok {
				evalErr = fmt.Errorf("%w: route %s/%s has no binding", ErrInvalidCircuit, target.Guard, target.Member)
				return
			}
			singleton, e := bindings.Singleton(config.Key, disjunct)
			if e != nil {
				evalErr = e
				return
			}
			value = join(value, evalValue{kind: evalConcrete, cell: singleton})
		}
		for _, successor := range successors[block] {
			emit(successor, value)
		}
	}})
	if err != nil {
		return EvaluationResult{}, err
	}
	if evalErr != nil {
		return EvaluationResult{}, evalErr
	}
	out := EvaluationResult{Cells: make(map[program.BlockID]Cell, len(cells)), Revisions: result.Revisions}
	out.Stats.Cells = len(cells)
	out.Stats.Transfers = result.Stats.TransferCalls
	out.Stats.Widens = result.Stats.WidenCalls + guardedWidens
	out.Stats.IntrinsicCalls = len(circuit.intrinsics)
	for block, value := range result.Values {
		if value.kind == evalBottom {
			continue
		}
		if value.kind != evalConcrete {
			return EvaluationResult{}, fmt.Errorf("%w: non-concrete final cell %d", ErrInvalidCircuit, block)
		}
		out.Cells[block] = value.cell
		out.Stats.Disjuncts += len(value.cell.disjuncts)
		if value.cell.loss {
			out.Stats.PrecisionLossCells++
		}
	}
	return out, nil
}

func newEvalLattice(bindings *Domain, guardedWidens *int, evalErr *error) lattice.Lattice[evalValue] {
	join := func(left, right evalValue) evalValue {
		if left.kind == evalBottom {
			return right
		}
		if right.kind == evalBottom {
			return left
		}
		if left.kind == evalTop || right.kind == evalTop {
			return evalValue{kind: evalTop}
		}
		joined, _, err := bindings.Join(left.cell, right.cell)
		if errors.Is(err, ErrWidenRequired) {
			joined, _, err = bindings.Widen(left.cell, right.cell)
			*guardedWidens++
		}
		if err != nil {
			*evalErr = err
			return evalValue{kind: evalTop}
		}
		return evalValue{kind: evalConcrete, cell: joined}
	}
	widen := func(left, right evalValue) evalValue {
		if left.kind == evalBottom {
			return right
		}
		if right.kind == evalBottom {
			return left
		}
		if left.kind == evalTop || right.kind == evalTop {
			return evalValue{kind: evalTop}
		}
		joined, _, err := bindings.Widen(left.cell, right.cell)
		if err != nil {
			*evalErr = err
			return evalValue{kind: evalTop}
		}
		return evalValue{kind: evalConcrete, cell: joined}
	}
	return lattice.Lattice[evalValue]{Bottom: func() evalValue { return evalValue{kind: evalBottom} }, Top: func() evalValue { return evalValue{kind: evalTop} }, Equal: func(a, b evalValue) bool {
		if a.kind != b.kind {
			return false
		}
		if a.kind != evalConcrete {
			return true
		}
		return bindings.Equal(a.cell, b.cell)
	}, LessOrEq: func(a, b evalValue) bool {
		if a.kind == evalBottom || b.kind == evalTop {
			return true
		}
		if a.kind == evalTop || b.kind == evalBottom {
			return false
		}
		return bindings.LessOrEq(a.cell, b.cell)
	}, Join: join, Widen: widen}
}
func containsBlockID(values []program.BlockID, want program.BlockID) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
