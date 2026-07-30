package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/state"
)

// PathDependencySchedule is one sealed operation-owned schedule over the
// family-owned coordinate dependency certificate. The coordinate plan keeps
// coordinate and normalized Values locations atomic; stages retain semantic
// barriers that may not be crossed by a sparse executor.
type PathDependencySchedule struct {
	plan   state.CoordinateDependencyPlan
	stages []PathDependencyStage
	seal   *pathDependencyScheduleSeal
}

// PathDependencyStage is one ordered barrier interval. IDs retain their
// operation-owned identity and are never interpreted as coordinate indexes.
type PathDependencyStage struct {
	ids  []state.CoordinateDependencyID
	seal *pathDependencyScheduleSeal
}

type pathDependencyScheduleSeal struct{}

func sealPathDependencySchedule(plan state.CoordinateDependencyPlan, stages [][]state.CoordinateDependencyID) (PathDependencySchedule, error) {
	ids := plan.IDs()
	if len(ids) == 0 || len(stages) == 0 {
		return PathDependencySchedule{}, fmt.Errorf("factapply: empty path dependency schedule")
	}
	want := make(map[state.CoordinateDependencyID]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			return PathDependencySchedule{}, fmt.Errorf("factapply: invalid path dependency identity")
		}
		want[id] = struct{}{}
	}
	seen := make(map[state.CoordinateDependencyID]struct{}, len(ids))
	seal := new(pathDependencyScheduleSeal)
	out := PathDependencySchedule{plan: plan, seal: seal, stages: make([]PathDependencyStage, 0, len(stages))}
	for _, stageIDs := range stages {
		if len(stageIDs) == 0 {
			return PathDependencySchedule{}, fmt.Errorf("factapply: empty path dependency stage")
		}
		stage := PathDependencyStage{ids: append([]state.CoordinateDependencyID(nil), stageIDs...), seal: seal}
		for _, id := range stage.ids {
			if _, valid := want[id]; !valid {
				return PathDependencySchedule{}, fmt.Errorf("factapply: foreign path dependency identity %d", id)
			}
			if _, duplicate := seen[id]; duplicate {
				return PathDependencySchedule{}, fmt.Errorf("factapply: duplicate path dependency identity %d", id)
			}
			seen[id] = struct{}{}
		}
		out.stages = append(out.stages, stage)
	}
	if len(seen) != len(want) {
		return PathDependencySchedule{}, fmt.Errorf("factapply: incomplete path dependency schedule")
	}
	return out, nil
}

// CoordinatePlan returns the immutable family-owned certificate. Its public
// accessors detach every slice and never expose internal maps.
func (s PathDependencySchedule) CoordinatePlan() (state.CoordinateDependencyPlan, bool) {
	if s.seal == nil || len(s.stages) == 0 {
		return state.CoordinateDependencyPlan{}, false
	}
	return s.plan, true
}

func (s PathDependencySchedule) Len() int {
	if s.seal == nil {
		return 0
	}
	return len(s.stages)
}

func (s PathDependencySchedule) Stage(index int) (PathDependencyStage, bool) {
	if s.seal == nil || index < 0 || index >= len(s.stages) || s.stages[index].seal != s.seal {
		return PathDependencyStage{}, false
	}
	stage := s.stages[index]
	stage.ids = append([]state.CoordinateDependencyID(nil), stage.ids...)
	return stage, true
}

func (s PathDependencyStage) IDs() []state.CoordinateDependencyID {
	if s.seal == nil {
		return nil
	}
	return append([]state.CoordinateDependencyID(nil), s.ids...)
}
