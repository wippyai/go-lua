package transformer

import (
	"fmt"
	"sort"

	key "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// coordinateSelectionContract is the producer-owned certificate which lets
// the guarded executor align one registered coordinate family without
// materializing every scalar in that family. The executor only asks the
// registered family for its closure; it never dispatches on an operation or
// effect kind.
type coordinateSelectionContract interface {
	Family(state.ProductDomain) (state.CoordinateFamily, bool)
	Select(state.ProductDomain, []state.CoordinateSlot) ([]int, error)
	Identity() string
}

type pathCoordinateSelection struct {
	seeds    []keyspace.Key
	identity string
}

func (s pathCoordinateSelection) Family(domain state.ProductDomain) (state.CoordinateFamily, bool) {
	return domain.PathValueFamily()
}

func (s pathCoordinateSelection) Select(domain state.ProductDomain, slots []state.CoordinateSlot) ([]int, error) {
	return domain.PathCoordinateMutationClosure(slots, s.seeds, nil)
}

func (s pathCoordinateSelection) Identity() string { return s.identity }

type rootAssignmentCoordinateSelection struct {
	plan interface {
		CoordinateClosure(state.ProductDomain, []state.CoordinateSlot) ([]int, error)
	}
}

func (s rootAssignmentCoordinateSelection) Family(domain state.ProductDomain) (state.CoordinateFamily, bool) {
	return domain.PathValueFamily()
}

func (s rootAssignmentCoordinateSelection) Select(domain state.ProductDomain, slots []state.CoordinateSlot) ([]int, error) {
	if s.plan == nil {
		return nil, fmt.Errorf("transformer: root-assignment coordinate selection has no plan")
	}
	return s.plan.CoordinateClosure(domain, slots)
}

func (rootAssignmentCoordinateSelection) Identity() string { return "root-assignment" }

// structuralPathKey freezes a PathTerm against this relation's sealed root
// carrier. It is deliberately independent of a DD leaf BindingCursor: every
// invocation of the relation has the same structural coordinate address.
func (b *relationProgramBody) structuralPathKey(term PathTerm) (keyspace.Key, error) {
	if b == nil || b.relation.arena == nil || b.keys == nil || !b.keys.Valid() || term == 0 || int(term) >= len(b.relation.arena.paths) {
		return keyspace.Key{}, fmt.Errorf("transformer: coordinate selection has a foreign path term")
	}
	node := b.relation.arena.paths[term]
	var symbolPath pathdom.Path
	if node.environment != 0 {
		symbolPath = pathdom.Path{Symbol: node.environment, Segments: append([]segment.Segment(nil), node.segments...)}
	} else {
		slot, exact := b.rootValueSlot(node.root)
		id, symbolSlot := key.ParseSymbolValue(slot)
		if !exact || !symbolSlot {
			return keyspace.Key{}, fmt.Errorf("transformer: coordinate selection path has an unbound formal symbol root")
		}
		symbolPath = pathdom.Path{Symbol: id, Segments: append([]segment.Segment(nil), node.segments...)}
	}
	key := b.keys.FromPath(symbolPath)
	if b.keys.FormatReadOnly(key) == "" {
		return keyspace.Key{}, fmt.Errorf("transformer: coordinate selection path has no structural spelling")
	}
	return key, nil
}

func freezePathCoordinateSelection(body *relationProgramBody, identity string, terms ...PathTerm) (coordinateSelectionContract, error) {
	seeds := make([]keyspace.Key, 0, len(terms))
	for _, term := range terms {
		if term == 0 {
			continue
		}
		seed, err := body.structuralPathKey(term)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}
	return freezePathCoordinateKeys(body, identity, seeds)
}

// freezeSemanticCoordinateSelection compiles the complete structural
// coordinate dependency of an effect's ValueTerm DAG and its explicit path
// operands into one family certificate. Dynamic reads contribute their table,
// key and range addresses; Select contributes its guard atoms; frame results
// contribute their sealed path targets. No product value is evaluated here.
func freezeSemanticCoordinateSelection(body *relationProgramBody, identity string, values []ValueTerm, paths []PathTerm) (coordinateSelectionContract, error) {
	if body == nil || body.relation.arena == nil {
		return nil, fmt.Errorf("transformer: semantic coordinate selection has no frozen arena")
	}
	seeds := make([]keyspace.Key, 0, len(paths)+len(values))
	for _, term := range paths {
		if term == 0 {
			continue
		}
		seed, err := body.structuralPathKey(term)
		if err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}
	arena := body.relation.arena
	seen := make(map[ValueTerm]struct{}, len(values))
	stack := append([]ValueTerm(nil), values...)
	appendPath := func(term PathTerm) error {
		if term == 0 {
			return nil
		}
		seed, err := body.structuralPathKey(term)
		if err != nil {
			return err
		}
		seeds = append(seeds, seed)
		return nil
	}
	for len(stack) != 0 {
		term := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if term == 0 || int(term) >= len(arena.values) {
			return nil, fmt.Errorf("transformer: semantic coordinate selection contains a foreign value term")
		}
		if _, present := seen[term]; present {
			continue
		}
		seen[term] = struct{}{}
		node := arena.values[term]
		stack = append(stack, node.args...)
		if node.integerProof != 0 {
			stack = append(stack, node.integerProof)
		}
		switch node.op {
		case valueDynamicRead, valueDynamicTableRead:
			for _, path := range []PathTerm{node.path, node.keyPath, node.rangePath} {
				if err := appendPath(path); err != nil {
					return nil, err
				}
			}
		case valueFrameResult:
			if node.frame == 0 || int(node.frame) >= len(body.frames) || node.resultIndex < 0 {
				return nil, fmt.Errorf("transformer: semantic coordinate selection has a foreign frame result")
			}
			frame := body.frames[node.frame]
			if !frame.valid() || node.resultIndex >= len(frame.resultSelectors) {
				return nil, fmt.Errorf("transformer: semantic coordinate selection has an invalid frame result")
			}
			for _, target := range frame.resultSelectors[node.resultIndex].targets {
				if target.stateTarget && target.slot == 0 && target.path.Kind != keyspace.KindInvalid {
					if body.keys == nil || body.keys.FormatReadOnly(target.path) == "" {
						return nil, fmt.Errorf("transformer: semantic coordinate selection has a foreign frame path")
					}
					seeds = append(seeds, target.path)
				}
			}
		case valueSelect:
			atoms := make(map[ValueTerm]struct{})
			if err := collectRelationGuardAtoms(arena, node.guard, atoms, make(map[Guard]uint8)); err != nil {
				return nil, err
			}
			for atom := range atoms {
				stack = append(stack, atom)
			}
		}
	}
	return freezePathCoordinateKeys(body, identity, seeds)
}

func freezePathCoordinateKeys(body *relationProgramBody, identity string, seeds []keyspace.Key) (coordinateSelectionContract, error) {
	if len(seeds) == 0 {
		return nil, nil
	}
	if body == nil || body.keys == nil || !body.keys.Valid() {
		return nil, fmt.Errorf("transformer: coordinate selection has no keyspace")
	}
	for index, seed := range seeds {
		if body.keys.FormatReadOnly(seed) == "" {
			return nil, fmt.Errorf("transformer: coordinate selection seed %d belongs to a foreign keyspace", index)
		}
	}
	sort.Slice(seeds, func(i, j int) bool { return body.keys.Less(seeds[i], seeds[j]) })
	dedup := seeds[:0]
	for _, seed := range seeds {
		if len(dedup) == 0 || dedup[len(dedup)-1] != seed {
			dedup = append(dedup, seed)
		}
	}
	return pathCoordinateSelection{seeds: append([]keyspace.Key(nil), dedup...), identity: identity}, nil
}
