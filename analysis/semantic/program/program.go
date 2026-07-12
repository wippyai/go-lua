// Package program defines the backend-neutral canonical program consumed by
// concrete and circuit region executors. It owns no scheduler or source model.
package program

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

const Schema = "go-lua.semantic.program/v1"

var ErrInvalid = errors.New("semantic program: invalid")

type BlockID uint32
type RegionID uint32
type ObservationID uint32
type MemberID string
type GuardID string
type TransactionDigest [sha256.Size]byte

type TransactionRef struct{ Digest TransactionDigest }

func Ref(frozen transaction.FrozenTransaction) TransactionRef {
	return TransactionRef{Digest: TransactionDigest(frozen.Digest())}
}

type Block struct {
	ID           BlockID
	Member       MemberID
	Transactions []TransactionRef
}

type Edge struct {
	From, To BlockID
	Guard    GuardID
}

type ObservationKind uint8

const (
	ObserveBoundary ObservationKind = iota + 1
	ObserveNode
	ObserveEdge
	ObserveResult
)

type ObservationSlot struct {
	ID     ObservationID
	At     BlockID
	Kind   ObservationKind
	Schema string
}

// LoopMu is an intraprocedural least-fixed-point region. Parent is zero for a
// top-level loop; nested loops must be strict block subsets of their parent.
type LoopMu struct {
	ID     RegionID
	SCC    RegionID
	Parent RegionID
	Owner  MemberID
	Entry  BlockID
	Blocks []BlockID
}

// CallSCCMu owns the interprocedural fixed point for the complete lexical
// member set represented by this Program.
type CallSCCMu struct {
	ID      RegionID
	Members []MemberID
}

type Completeness uint8

const (
	TargetsOpen Completeness = iota + 1
	TargetsComplete
)

type KnownTarget struct {
	Guard  GuardID
	Member MemberID
}

type TargetResidue struct {
	Unknown bool
	Native  bool
}

type MixedTargetRoute struct {
	At           BlockID
	Known        []KnownTarget
	Residue      TargetResidue
	Completeness Completeness
	Proof        [sha256.Size]byte
}

type Spec struct {
	Entry        BlockID
	Members      []MemberID
	Transactions []TransactionRef
	Blocks       []Block
	Edges        []Edge
	Observations []ObservationSlot
	CallSCC      CallSCCMu
	Loops        []LoopMu
	Routes       []MixedTargetRoute
}

type Program struct {
	entry        BlockID
	members      []MemberID
	transactions []TransactionRef
	blocks       []Block
	edges        []Edge
	observations []ObservationSlot
	callSCC      CallSCCMu
	loops        []LoopMu
	routes       []MixedTargetRoute
	canonical    []byte
	digest       [sha256.Size]byte
}

func Freeze(spec Spec) (Program, error) {
	p := Program{
		entry: spec.Entry, members: append([]MemberID(nil), spec.Members...),
		transactions: append([]TransactionRef(nil), spec.Transactions...),
		blocks:       cloneBlocks(spec.Blocks), edges: append([]Edge(nil), spec.Edges...),
		observations: append([]ObservationSlot(nil), spec.Observations...),
		callSCC:      cloneCallSCC(spec.CallSCC), loops: cloneLoops(spec.Loops),
		routes: cloneRoutes(spec.Routes),
	}
	p.canonicalize()
	if err := p.validate(); err != nil {
		return Program{}, err
	}
	encoded, err := encodeCanonical(p)
	if err != nil {
		return Program{}, err
	}
	p.canonical = encoded
	p.digest = sha256.Sum256(encoded)
	return p, nil
}

func (p *Program) canonicalize() {
	slices.Sort(p.members)
	sort.Slice(p.transactions, func(i, j int) bool {
		return bytes.Compare(p.transactions[i].Digest[:], p.transactions[j].Digest[:]) < 0
	})
	sort.Slice(p.blocks, func(i, j int) bool { return p.blocks[i].ID < p.blocks[j].ID })
	sort.Slice(p.edges, func(i, j int) bool {
		a, b := p.edges[i], p.edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Guard < b.Guard
	})
	sort.Slice(p.observations, func(i, j int) bool { return p.observations[i].ID < p.observations[j].ID })
	slices.Sort(p.callSCC.Members)
	sort.Slice(p.loops, func(i, j int) bool { return p.loops[i].ID < p.loops[j].ID })
	for i := range p.loops {
		slices.Sort(p.loops[i].Blocks)
	}
	sort.Slice(p.routes, func(i, j int) bool { return p.routes[i].At < p.routes[j].At })
	for i := range p.routes {
		sort.Slice(p.routes[i].Known, func(a, b int) bool {
			x, y := p.routes[i].Known[a], p.routes[i].Known[b]
			if x.Member != y.Member {
				return x.Member < y.Member
			}
			return x.Guard < y.Guard
		})
	}
}

func (p Program) validate() error {
	if p.entry == 0 {
		return invalid("entry", "zero block ID")
	}
	memberSet := map[MemberID]struct{}{}
	for _, member := range p.members {
		if !validName(string(member)) {
			return invalid("member", fmt.Sprintf("invalid ID %q", member))
		}
		if _, ok := memberSet[member]; ok {
			return invalid("member", fmt.Sprintf("duplicate ID %q", member))
		}
		memberSet[member] = struct{}{}
	}
	if len(memberSet) == 0 {
		return invalid("members", "empty")
	}
	txSet := map[TransactionDigest]struct{}{}
	for _, ref := range p.transactions {
		if ref.Digest == (TransactionDigest{}) {
			return invalid("transaction", "zero digest")
		}
		if _, ok := txSet[ref.Digest]; ok {
			return invalid("transaction", "duplicate digest")
		}
		txSet[ref.Digest] = struct{}{}
	}
	blocks := map[BlockID]Block{}
	for _, block := range p.blocks {
		if block.ID == 0 {
			return invalid("block", "zero ID")
		}
		if _, ok := blocks[block.ID]; ok {
			return invalid("block", fmt.Sprintf("duplicate ID %d", block.ID))
		}
		if _, ok := memberSet[block.Member]; !ok {
			return invalid("block", fmt.Sprintf("%d has unknown member %q", block.ID, block.Member))
		}
		for _, ref := range block.Transactions {
			if _, ok := txSet[ref.Digest]; !ok {
				return invalid("block", fmt.Sprintf("%d has undeclared transaction", block.ID))
			}
		}
		blocks[block.ID] = block
	}
	if _, ok := blocks[p.entry]; !ok {
		return invalid("entry", "references unknown block")
	}
	seenEdge := map[string]struct{}{}
	for _, edge := range p.edges {
		if _, ok := blocks[edge.From]; !ok {
			return invalid("edge", fmt.Sprintf("unknown source %d", edge.From))
		}
		if _, ok := blocks[edge.To]; !ok {
			return invalid("edge", fmt.Sprintf("unknown target %d", edge.To))
		}
		if !validStableID(string(edge.Guard)) {
			return invalid("edge", fmt.Sprintf("invalid guard ID %q", edge.Guard))
		}
		key := fmt.Sprintf("%d\x00%d\x00%s", edge.From, edge.To, edge.Guard)
		if _, ok := seenEdge[key]; ok {
			return invalid("edge", "duplicate")
		}
		seenEdge[key] = struct{}{}
	}
	seenObs := map[ObservationID]struct{}{}
	for _, observation := range p.observations {
		if observation.ID == 0 || observation.Kind < ObserveBoundary || observation.Kind > ObserveResult || !validName(observation.Schema) {
			return invalid("observation", fmt.Sprintf("invalid slot %d", observation.ID))
		}
		if _, ok := seenObs[observation.ID]; ok {
			return invalid("observation", fmt.Sprintf("duplicate ID %d", observation.ID))
		}
		if _, ok := blocks[observation.At]; !ok {
			return invalid("observation", fmt.Sprintf("%d references unknown block", observation.ID))
		}
		seenObs[observation.ID] = struct{}{}
	}
	if p.callSCC.ID == 0 {
		return invalid("call SCC", "zero region ID")
	}
	if !slices.Equal(p.callSCC.Members, p.members) {
		return invalid("call SCC", "members do not equal program members")
	}
	regions := map[RegionID]struct{}{p.callSCC.ID: {}}
	loopByID := map[RegionID]LoopMu{}
	for _, loop := range p.loops {
		if loop.ID == 0 || loop.SCC != p.callSCC.ID {
			return invalid("loop", fmt.Sprintf("%d has invalid SCC owner", loop.ID))
		}
		if _, ok := regions[loop.ID]; ok {
			return invalid("region", fmt.Sprintf("duplicate ID %d", loop.ID))
		}
		regions[loop.ID] = struct{}{}
		loopByID[loop.ID] = loop
		if _, ok := memberSet[loop.Owner]; !ok {
			return invalid("loop", fmt.Sprintf("%d has unknown owner", loop.ID))
		}
		if len(loop.Blocks) == 0 || !containsBlock(loop.Blocks, loop.Entry) {
			return invalid("loop", fmt.Sprintf("%d does not own its entry", loop.ID))
		}
		prior := BlockID(0)
		for _, id := range loop.Blocks {
			block, ok := blocks[id]
			if !ok || block.Member != loop.Owner {
				return invalid("loop", fmt.Sprintf("%d owns foreign block %d", loop.ID, id))
			}
			if id == prior {
				return invalid("loop", fmt.Sprintf("%d repeats block %d", loop.ID, id))
			}
			prior = id
		}
	}
	for _, loop := range p.loops {
		if loop.Parent == 0 {
			continue
		}
		parent, ok := loopByID[loop.Parent]
		if !ok || parent.Owner != loop.Owner || !strictSubset(loop.Blocks, parent.Blocks) {
			return invalid("loop", fmt.Sprintf("%d has invalid parent %d", loop.ID, loop.Parent))
		}
		for cursor := parent; cursor.Parent != 0; {
			if cursor.Parent == loop.ID {
				return invalid("loop", "parent cycle")
			}
			next, ok := loopByID[cursor.Parent]
			if !ok {
				break
			}
			cursor = next
		}
	}
	seenRoute := map[BlockID]struct{}{}
	for _, route := range p.routes {
		if _, ok := blocks[route.At]; !ok {
			return invalid("route", fmt.Sprintf("unknown block %d", route.At))
		}
		if _, ok := seenRoute[route.At]; ok {
			return invalid("route", fmt.Sprintf("duplicate block %d", route.At))
		}
		seenRoute[route.At] = struct{}{}
		if route.Completeness != TargetsOpen && route.Completeness != TargetsComplete {
			return invalid("route", "invalid completeness")
		}
		if route.Completeness == TargetsComplete && route.Proof == ([sha256.Size]byte{}) {
			return invalid("route", "complete targets require proof")
		}
		if route.Completeness == TargetsOpen && route.Proof != ([sha256.Size]byte{}) {
			return invalid("route", "open targets cannot claim completeness proof")
		}
		seen := map[string]struct{}{}
		for _, target := range route.Known {
			if _, ok := memberSet[target.Member]; !ok || !validStableID(string(target.Guard)) {
				return invalid("route", "invalid known target")
			}
			key := string(target.Member) + "\x00" + string(target.Guard)
			if _, ok := seen[key]; ok {
				return invalid("route", "duplicate known target")
			}
			seen[key] = struct{}{}
		}
		if len(route.Known) == 0 && !route.Residue.Unknown && !route.Residue.Native {
			return invalid("route", "has no targets or residue")
		}
	}
	return nil
}

func invalid(field, detail string) error { return fmt.Errorf("%w: %s: %s", ErrInvalid, field, detail) }
func validName(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}
func validStableID(value string) bool {
	if value == "" {
		return false
	}
	separator := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' {
			separator = false
			continue
		}
		if index > 0 && index < len(value)-1 && !separator && (character == '-' || character == '.' || character == '/' || character == ':') {
			separator = true
			continue
		}
		return false
	}
	return true
}
func containsBlock(ids []BlockID, want BlockID) bool {
	_, ok := slices.BinarySearch(ids, want)
	return ok
}
func strictSubset(child, parent []BlockID) bool {
	if len(child) >= len(parent) {
		return false
	}
	for _, id := range child {
		if !containsBlock(parent, id) {
			return false
		}
	}
	return true
}

func cloneBlocks(in []Block) []Block {
	out := append([]Block(nil), in...)
	for i := range out {
		out[i].Transactions = append([]TransactionRef(nil), out[i].Transactions...)
	}
	return out
}
func cloneCallSCC(in CallSCCMu) CallSCCMu {
	in.Members = append([]MemberID(nil), in.Members...)
	return in
}
func cloneLoops(in []LoopMu) []LoopMu {
	out := append([]LoopMu(nil), in...)
	for i := range out {
		out[i].Blocks = append([]BlockID(nil), out[i].Blocks...)
	}
	return out
}
func cloneRoutes(in []MixedTargetRoute) []MixedTargetRoute {
	out := append([]MixedTargetRoute(nil), in...)
	for i := range out {
		out[i].Known = append([]KnownTarget(nil), out[i].Known...)
	}
	return out
}

func (p Program) Entry() BlockID      { return p.entry }
func (p Program) Members() []MemberID { return append([]MemberID(nil), p.members...) }
func (p Program) Transactions() []TransactionRef {
	return append([]TransactionRef(nil), p.transactions...)
}
func (p Program) Blocks() []Block { return cloneBlocks(p.blocks) }
func (p Program) Edges() []Edge   { return append([]Edge(nil), p.edges...) }
func (p Program) Observations() []ObservationSlot {
	return append([]ObservationSlot(nil), p.observations...)
}
func (p Program) CallSCC() CallSCCMu         { return cloneCallSCC(p.callSCC) }
func (p Program) Loops() []LoopMu            { return cloneLoops(p.loops) }
func (p Program) Routes() []MixedTargetRoute { return cloneRoutes(p.routes) }
func (p Program) CanonicalBytes() []byte     { return append([]byte(nil), p.canonical...) }
func (p Program) Digest() [sha256.Size]byte  { return p.digest }
