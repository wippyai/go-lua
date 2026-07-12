package transferfacts

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	semanticprogram "github.com/wippyai/go-lua/analysis/semantic/program"
)

var ErrSemanticProgramTopology = errors.New("transferfacts: invalid semantic program topology")

// ProgramMember is one WIR/CFG body and its already-derived neutral semantic
// attachments. This adapter does not derive fact meaning or execute a Program.
type ProgramMember struct {
	ID           semanticprogram.MemberID
	Graph        cfg.Graph
	WIR          *wir.Body
	Transactions []PointTransactionRef
	Observations []PointObservationSpec
	Routes       []PointRouteSpec
}

type PointTransactionRef struct {
	Point cfg.Point
	Ref   semanticprogram.TransactionRef
}

type PointObservationSpec struct {
	Point  cfg.Point
	Kind   semanticprogram.ObservationKind
	Schema string
}

type PointRouteSpec struct {
	Point        cfg.Point
	Known        []semanticprogram.KnownTarget
	Residue      semanticprogram.TargetResidue
	Completeness semanticprogram.Completeness
	Proof        [32]byte
}

type ProgramInput struct{ Members []ProgramMember }

type PointBlock struct {
	Member semanticprogram.MemberID
	Point  cfg.Point
	Block  semanticprogram.BlockID
}

// ProgramTopology is the exact detached RPO point-to-block correspondence used
// by callers to attach later products without relying on cfg.Point numerics.
type ProgramTopology struct{ points []PointBlock }

func (t ProgramTopology) Points() []PointBlock { return append([]PointBlock(nil), t.points...) }

// AssembleSemanticProgram is the sole WIR/CFG -> semantic/program topology
// seam. It validates total WIR point correspondence, assigns deterministic IDs,
// derives cyclic CFG SCCs as LoopMu nodes and freezes the neutral Program.
func AssembleSemanticProgram(input ProgramInput) (semanticprogram.Program, ProgramTopology, error) {
	members := append([]ProgramMember(nil), input.Members...)
	if len(members) == 0 {
		return semanticprogram.Program{}, ProgramTopology{}, topologyError("members are empty")
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	seenMembers := map[semanticprogram.MemberID]struct{}{}
	var spec semanticprogram.Spec
	pointBlocks := make(map[memberPoint]semanticprogram.BlockID)
	memberRPO := make(map[semanticprogram.MemberID][]cfg.Point)
	for _, member := range members {
		if _, duplicate := seenMembers[member.ID]; duplicate {
			return semanticprogram.Program{}, ProgramTopology{}, topologyError("duplicate member %q", member.ID)
		}
		seenMembers[member.ID] = struct{}{}
		if err := validateMemberTopology(member); err != nil {
			return semanticprogram.Program{}, ProgramTopology{}, err
		}
		rpo := append([]cfg.Point(nil), member.Graph.RPO()...)
		memberRPO[member.ID] = rpo
		spec.Members = append(spec.Members, member.ID)
		for _, point := range rpo {
			if len(spec.Blocks) == int(^uint32(0)) {
				return semanticprogram.Program{}, ProgramTopology{}, topologyError("block ID space exhausted")
			}
			block := semanticprogram.BlockID(len(spec.Blocks) + 1)
			pointBlocks[memberPoint{member.ID, point}] = block
			spec.Blocks = append(spec.Blocks, semanticprogram.Block{ID: block, Member: member.ID})
		}
	}
	first := members[0]
	spec.Entry = pointBlocks[memberPoint{first.ID, first.Graph.Entry()}]
	spec.CallSCC = semanticprogram.CallSCCMu{ID: 1, Members: append([]semanticprogram.MemberID(nil), spec.Members...)}

	txCatalog := map[semanticprogram.TransactionDigest]semanticprogram.TransactionRef{}
	var observations []orderedObservation
	var routes []orderedRoute
	for _, member := range members {
		blockIndex := make(map[semanticprogram.BlockID]int, len(memberRPO[member.ID]))
		for _, point := range memberRPO[member.ID] {
			block := pointBlocks[memberPoint{member.ID, point}]
			blockIndex[block] = int(block) - 1
		}
		for _, binding := range member.Transactions {
			block, ok := pointBlocks[memberPoint{member.ID, binding.Point}]
			if !ok {
				return semanticprogram.Program{}, ProgramTopology{}, topologyError("member %q transaction references uncovered point %d", member.ID, binding.Point)
			}
			spec.Blocks[blockIndex[block]].Transactions = append(spec.Blocks[blockIndex[block]].Transactions, binding.Ref)
			txCatalog[binding.Ref.Digest] = binding.Ref
		}
		for ordinal, observation := range member.Observations {
			block, ok := pointBlocks[memberPoint{member.ID, observation.Point}]
			if !ok {
				return semanticprogram.Program{}, ProgramTopology{}, topologyError("member %q observation references uncovered point %d", member.ID, observation.Point)
			}
			observations = append(observations, orderedObservation{member: member.ID, block: block, ordinal: ordinal, kind: observation.Kind, schema: observation.Schema})
		}
		for _, route := range member.Routes {
			block, ok := pointBlocks[memberPoint{member.ID, route.Point}]
			if !ok {
				return semanticprogram.Program{}, ProgramTopology{}, topologyError("member %q route references uncovered point %d", member.ID, route.Point)
			}
			routes = append(routes, orderedRoute{member: member.ID, block: block, spec: route})
		}
		for _, edge := range member.Graph.Edges() {
			from, fromOK := pointBlocks[memberPoint{member.ID, edge.From}]
			to, toOK := pointBlocks[memberPoint{member.ID, edge.To}]
			if !fromOK || !toOK {
				return semanticprogram.Program{}, ProgramTopology{}, topologyError("member %q edge %d->%d escapes RPO coverage", member.ID, edge.From, edge.To)
			}
			guard := semanticprogram.GuardID("flow")
			if member.Graph.IsBranch(edge.From) {
				if edge.Cond {
					guard = "branch.true"
				} else {
					guard = "branch.false"
				}
			}
			spec.Edges = append(spec.Edges, semanticprogram.Edge{From: from, To: to, Guard: guard})
		}
	}
	for _, ref := range txCatalog {
		spec.Transactions = append(spec.Transactions, ref)
	}
	sort.Slice(observations, func(i, j int) bool {
		a, b := observations[i], observations[j]
		if a.member != b.member {
			return a.member < b.member
		}
		if a.block != b.block {
			return a.block < b.block
		}
		return a.ordinal < b.ordinal
	})
	for index, observation := range observations {
		spec.Observations = append(spec.Observations, semanticprogram.ObservationSlot{ID: semanticprogram.ObservationID(index + 1), At: observation.block, Kind: observation.kind, Schema: observation.schema})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].member != routes[j].member {
			return routes[i].member < routes[j].member
		}
		return routes[i].block < routes[j].block
	})
	for _, route := range routes {
		spec.Routes = append(spec.Routes, semanticprogram.MixedTargetRoute{At: route.block, Known: append([]semanticprogram.KnownTarget(nil), route.spec.Known...), Residue: route.spec.Residue, Completeness: route.spec.Completeness, Proof: route.spec.Proof})
	}

	nextRegion := semanticprogram.RegionID(2)
	for _, member := range members {
		rpo := memberRPO[member.ID]
		rank := make(map[cfg.Point]int, len(rpo))
		for i, point := range rpo {
			rank[point] = i
		}
		plan := solve.NewWTOPlan(rpo, func(point cfg.Point) []cfg.Point {
			successors := append([]cfg.Point(nil), cfg.SuccessorsReadOnly(member.Graph, point)...)
			sort.Slice(successors, func(i, j int) bool { return rank[successors[i]] < rank[successors[j]] })
			return successors
		})
		appendWTOLoops(&spec.Loops, plan.Elements(), 0, spec.CallSCC.ID, member.ID, pointBlocks, &nextRegion)
	}
	frozen, err := semanticprogram.Freeze(spec)
	if err != nil {
		return semanticprogram.Program{}, ProgramTopology{}, fmt.Errorf("%w: freeze: %v", ErrSemanticProgramTopology, err)
	}
	topology := ProgramTopology{points: make([]PointBlock, 0, len(pointBlocks))}
	for _, member := range members {
		for _, point := range memberRPO[member.ID] {
			topology.points = append(topology.points, PointBlock{Member: member.ID, Point: point, Block: pointBlocks[memberPoint{member.ID, point}]})
		}
	}
	return frozen, topology, nil
}

type memberPoint struct {
	member semanticprogram.MemberID
	point  cfg.Point
}
type orderedObservation struct {
	member  semanticprogram.MemberID
	block   semanticprogram.BlockID
	ordinal int
	kind    semanticprogram.ObservationKind
	schema  string
}
type orderedRoute struct {
	member semanticprogram.MemberID
	block  semanticprogram.BlockID
	spec   PointRouteSpec
}

func validateMemberTopology(member ProgramMember) error {
	if member.ID == "" || member.Graph == nil || member.WIR == nil {
		return topologyError("member %q requires ID, CFG and WIR", member.ID)
	}
	rpo := member.Graph.RPO()
	if len(rpo) != member.Graph.Size() {
		return topologyError("member %q RPO covers %d/%d points", member.ID, len(rpo), member.Graph.Size())
	}
	debug := member.WIR.DebugPoints()
	if len(debug) != len(rpo) {
		return topologyError("member %q WIR debug coverage is %d/%d", member.ID, len(debug), len(rpo))
	}
	seen := map[cfg.Point]struct{}{}
	for index, point := range rpo {
		if _, duplicate := seen[point]; duplicate || member.Graph.Node(point) == nil {
			return topologyError("member %q has invalid RPO point %d", member.ID, point)
		}
		seen[point] = struct{}{}
		if !member.WIR.HasPoint(point) {
			return topologyError("member %q WIR has no point window for %d", member.ID, point)
		}
		if debug[index].Point != point || debug[index].Ordinal != uint32(index+1) {
			return topologyError("member %q WIR debug order diverges at RPO index %d", member.ID, index)
		}
	}
	return nil
}

func topologyError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSemanticProgramTopology, fmt.Sprintf(format, args...))
}

func appendWTOLoops(out *[]semanticprogram.LoopMu, elements []solve.WTOElement[cfg.Point], parent, scc semanticprogram.RegionID, owner semanticprogram.MemberID, blocks map[memberPoint]semanticprogram.BlockID, next *semanticprogram.RegionID) {
	for _, element := range elements {
		if !element.IsComponent() {
			continue
		}
		id := *next
		*next++
		ownedPoints := []cfg.Point{element.Vertex}
		collectWTOBodyPoints(element.Body, &ownedPoints)
		ownedBlocks := make([]semanticprogram.BlockID, len(ownedPoints))
		for i, point := range ownedPoints {
			ownedBlocks[i] = blocks[memberPoint{owner, point}]
		}
		slices.Sort(ownedBlocks)
		*out = append(*out, semanticprogram.LoopMu{ID: id, SCC: scc, Parent: parent, Owner: owner, Entry: blocks[memberPoint{owner, element.Vertex}], Blocks: ownedBlocks})
		appendWTOLoops(out, element.Body, id, scc, owner, blocks, next)
	}
}

func collectWTOBodyPoints(elements []solve.WTOElement[cfg.Point], out *[]cfg.Point) {
	for _, element := range elements {
		*out = append(*out, element.Vertex)
		collectWTOBodyPoints(element.Body, out)
	}
}
