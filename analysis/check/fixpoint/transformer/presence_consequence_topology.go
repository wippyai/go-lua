package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// presenceConsequenceBlockInventory is the transformer-side view of one
// producer-sealed consequence SCC. predecessors are indexes in the same
// barrier stage's deterministic condensation order. They describe topology
// only; the factapply block remains the sole semantic operation.
type presenceConsequenceBlockInventory struct {
	block            factapply.PresenceImplicationDependencyBlock
	coordinateWrites []state.CoordinateSlot
	valueWrites      []statekey.Value
	predecessors     []int
	pathMutation     bool
	mayContradict    bool
	requiresFeedback bool
}

type presenceConsequenceStageInventory struct {
	stage           factapply.PresenceImplicationDependencyStage
	reducerWrites   []state.CoordinateSlot
	reducerSkeleton bool
	blocks          []presenceConsequenceBlockInventory
}

type presenceConsequenceInventory struct {
	plan   factapply.PresenceImplicationDependencyPlan
	slots  []state.CoordinateSlot
	stages []presenceConsequenceStageInventory
}

// presenceConsequencePayloadLayout is the exact typed carrier owned by one
// consequence SCC. A feedback carrier contains only monotone consequence
// payload. It is never a descending State snapshot.
type presenceConsequencePayloadLayout struct {
	coordinateWrites []state.CoordinateSlot
	coordinateOffset int
	coordinateCount  int
	valueWrites      []statekey.Value
	valueOffset      int
	valueCount       int
	laneWrites       []state.ProductLane
	laneOffset       int
	laneCount        int
	feasibility      int
}

type presenceConsequenceBlockTopology struct {
	stage, block int
	operation    factapply.PresenceImplicationDependencyBlock
	predecessors []int
	level        int
	feedbackNode int
	payload      presenceConsequencePayloadLayout
}

type presenceConsequenceStageTopology struct {
	operation       factapply.PresenceImplicationDependencyStage
	reducerWrites   []state.CoordinateSlot
	reducerSkeleton bool
	blocks          []presenceConsequenceBlockTopology
}

// presenceConsequenceTopology is frozen before guarded arenas or relation
// edges are allocated. segmentCount is the exact number of ordinary branch
// segments contributed by publication reducers and consequence blocks.
type presenceConsequenceTopology struct {
	plan             factapply.PresenceImplicationDependencyPlan
	slots            []state.CoordinateSlot
	stages           []presenceConsequenceStageTopology
	segmentCount     int
	feedbackNodes    int
	coordinateValues int
	valueValues      int
	laneValues       int
	feasibilityBits  int
}

// freezeBranchPresenceConsequenceTopology imports the exact producer-sealed
// dependency plan owned by one branch factor. This is the only adapter from a
// branch factor into consequence topology; branch syntax and State execution
// remain inaccessible here.
func freezeBranchPresenceConsequenceTopology(
	domain state.ProductDomain,
	factors factapply.BranchRelationFactors,
	factor int,
) (presenceConsequenceTopology, bool, error) {
	plan, present := factors.PresenceImplicationDependencyPlan(factor)
	if !present {
		return presenceConsequenceTopology{}, false, nil
	}
	topology, err := freezePresenceImplicationPlanTopology(domain, plan)
	return topology, true, err
}

func freezePresenceImplicationPlanTopology(domain state.ProductDomain, plan factapply.PresenceImplicationDependencyPlan) (presenceConsequenceTopology, error) {
	inventory := presenceConsequenceInventory{plan: plan, slots: plan.Slots()}
	for _, dependencyStage := range plan.Stages() {
		stage := presenceConsequenceStageInventory{
			stage: dependencyStage, reducerWrites: dependencyStage.ReducerWrites(),
			reducerSkeleton: dependencyStage.ReducerWritesSkeleton(),
		}
		for _, dependencyBlock := range dependencyStage.Blocks() {
			stage.blocks = append(stage.blocks, presenceConsequenceBlockInventory{
				block:            dependencyBlock,
				coordinateWrites: dependencyBlock.CoordinateWrites(),
				valueWrites:      dependencyBlock.ValueWrites(),
				predecessors:     dependencyBlock.Predecessors(),
				pathMutation:     dependencyBlock.PathMutation(),
				mayContradict:    dependencyBlock.MayContradict(),
				requiresFeedback: dependencyBlock.RequiresFeedback(),
			})
		}
		inventory.stages = append(inventory.stages, stage)
	}
	topology, err := freezePresenceConsequenceTopology(domain, inventory)
	return topology, err
}

func (t presenceConsequenceTopology) intermediateCoordinateCount() int {
	if t.segmentCount <= 1 {
		return 0
	}
	return t.segmentCount - 1
}

// freezePresenceConsequenceTopology validates the producer's already-sealed
// SCC condensation and assigns deterministic typed offsets. It deliberately
// does not inspect branch syntax, execute a consequence round, or discover an
// SCC. The global dirty scheduler later consumes this finite layout.
func freezePresenceConsequenceTopology(
	domain state.ProductDomain,
	inventory presenceConsequenceInventory,
) (presenceConsequenceTopology, error) {
	if !domain.Valid() {
		return presenceConsequenceTopology{}, fmt.Errorf("transformer: presence consequence topology has no product authority")
	}
	out := presenceConsequenceTopology{
		plan:  inventory.plan,
		slots: append([]state.CoordinateSlot(nil), inventory.slots...),
	}
	if err := validatePresenceConsequenceSlots(domain, out.slots); err != nil {
		return presenceConsequenceTopology{}, err
	}
	mutationLanes := domain.PathDescendantMutationParticipantLanes()
	for stageIndex, sourceStage := range inventory.stages {
		stage := presenceConsequenceStageTopology{
			operation:       sourceStage.stage,
			reducerWrites:   append([]state.CoordinateSlot(nil), sourceStage.reducerWrites...),
			reducerSkeleton: sourceStage.reducerSkeleton,
		}
		if err := validatePresenceConsequenceSubset(domain, out.slots, stage.reducerWrites); err != nil {
			return presenceConsequenceTopology{}, fmt.Errorf("transformer: presence consequence stage %d reducer: %w", stageIndex, err)
		}
		if stage.reducerSkeleton || len(stage.reducerWrites) != 0 {
			out.segmentCount++
		}
		for blockIndex, sourceBlock := range sourceStage.blocks {
			predecessors, err := freezePresenceConsequencePredecessors(blockIndex, sourceBlock.predecessors)
			if err != nil {
				return presenceConsequenceTopology{}, fmt.Errorf("transformer: presence consequence stage %d block %d: %w", stageIndex, blockIndex, err)
			}
			coordinateWrites := append([]state.CoordinateSlot(nil), sourceBlock.coordinateWrites...)
			if err := validatePresenceConsequenceSlots(domain, coordinateWrites); err != nil {
				return presenceConsequenceTopology{}, fmt.Errorf("transformer: presence consequence stage %d block %d writes: %w", stageIndex, blockIndex, err)
			}
			if err := validatePresenceConsequenceSubset(domain, out.slots, coordinateWrites); err != nil {
				return presenceConsequenceTopology{}, fmt.Errorf("transformer: presence consequence stage %d block %d: %w", stageIndex, blockIndex, err)
			}
			valueWrites, err := freezePresenceConsequenceValues(sourceBlock.valueWrites)
			if err != nil {
				return presenceConsequenceTopology{}, fmt.Errorf("transformer: presence consequence stage %d block %d: %w", stageIndex, blockIndex, err)
			}
			layout := presenceConsequencePayloadLayout{
				coordinateWrites: coordinateWrites,
				coordinateOffset: out.coordinateValues,
				coordinateCount:  len(coordinateWrites),
				valueWrites:      valueWrites,
				valueOffset:      out.valueValues,
				valueCount:       len(valueWrites),
				laneOffset:       out.laneValues,
				feasibility:      -1,
			}
			out.coordinateValues += len(coordinateWrites)
			out.valueValues += len(valueWrites)
			if sourceBlock.pathMutation {
				layout.laneWrites = append([]state.ProductLane(nil), mutationLanes...)
				layout.laneCount = len(mutationLanes)
				out.laneValues += len(mutationLanes)
			}
			if sourceBlock.mayContradict {
				layout.feasibility = out.feasibilityBits
				out.feasibilityBits++
			}
			feedbackNode := -1
			if sourceBlock.requiresFeedback {
				feedbackNode = out.feedbackNodes
				out.feedbackNodes++
			}
			level := 0
			for _, predecessor := range predecessors {
				predecessorLevel := stage.blocks[predecessor].level + 1
				if predecessorLevel > level {
					level = predecessorLevel
				}
			}
			stage.blocks = append(stage.blocks, presenceConsequenceBlockTopology{
				stage: stageIndex, block: blockIndex,
				operation: sourceBlock.block, predecessors: predecessors,
				level: level, feedbackNode: feedbackNode, payload: layout,
			})
			out.segmentCount++
		}
		out.stages = append(out.stages, stage)
	}
	return out, nil
}

// freezePresenceConsequenceBlockLevels turns the producer-sealed condensation
// edges into deterministic parallel levels. Independent blocks retain the
// same level; every RAW predecessor is an explicit earlier sparse/global
// stage. This is dependency algebra, not an execution-time ordering choice.
func freezePresenceConsequenceBlockLevels(blocks []factapply.PresenceImplicationDependencyBlock) ([]int, error) {
	levels := make([]int, len(blocks))
	for blockIndex, block := range blocks {
		predecessors, err := freezePresenceConsequencePredecessors(blockIndex, block.Predecessors())
		if err != nil {
			return nil, fmt.Errorf("presence consequence block %d: %w", blockIndex, err)
		}
		for _, predecessor := range predecessors {
			candidate := levels[predecessor] + 1
			if candidate > levels[blockIndex] {
				levels[blockIndex] = candidate
			}
		}
	}
	return levels, nil
}

func freezePresenceConsequencePredecessors(block int, values []int) ([]int, error) {
	out := append([]int(nil), values...)
	sort.Ints(out)
	for index, predecessor := range out {
		if predecessor < 0 || predecessor >= block {
			return nil, fmt.Errorf("condensation predecessor %d is not earlier than block %d", predecessor, block)
		}
		if index != 0 && out[index-1] == predecessor {
			return nil, fmt.Errorf("condensation predecessor %d is duplicated", predecessor)
		}
	}
	return out, nil
}

func freezePresenceConsequenceValues(values []statekey.Value) ([]statekey.Value, error) {
	out := append([]statekey.Value(nil), values...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for index, value := range out {
		if value == 0 {
			return nil, fmt.Errorf("presence consequence payload contains an invalid Values slot")
		}
		if index != 0 && out[index-1] == value {
			return nil, fmt.Errorf("presence consequence payload repeats Values slot %d", value)
		}
	}
	return out, nil
}

func validatePresenceConsequenceSlots(domain state.ProductDomain, slots []state.CoordinateSlot) error {
	for index, slot := range slots {
		if index != 0 {
			less, err := domain.CoordinateSlotLess(slots[index-1], slot)
			if err != nil {
				return err
			}
			if !less {
				equal, equalErr := domain.CoordinateSlotEqual(slots[index-1], slot)
				if equalErr != nil {
					return equalErr
				}
				if equal {
					return fmt.Errorf("presence consequence coordinate inventory repeats a slot")
				}
				return fmt.Errorf("presence consequence coordinate inventory is not canonical")
			}
		}
	}
	return nil
}

func validatePresenceConsequenceSubset(domain state.ProductDomain, inventory, subset []state.CoordinateSlot) error {
	for _, wanted := range subset {
		found := false
		for _, candidate := range inventory {
			equal, err := domain.CoordinateSlotEqual(candidate, wanted)
			if err != nil {
				return err
			}
			if equal {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("presence consequence write is outside the frozen coordinate inventory")
		}
	}
	return nil
}
