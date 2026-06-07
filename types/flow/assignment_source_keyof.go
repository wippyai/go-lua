package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
)

// PathVersioner maps a static source path into the point-relative path that
// condition facts should publish.
type PathVersioner func(constraint.Path) constraint.Path

// TruthyAssignmentValueKeyOfQuery asks which key-presence facts become true
// when ValuePath, assigned from an AssignmentSource, is known truthy.
type TruthyAssignmentValueKeyOfQuery struct {
	Assignments []UnifiedAssignment
	ValuePath   constraint.Path
	VersionPath PathVersioner
}

// TruthyAssignmentValueKeyOfConstraints projects source-owned assignment
// evidence into branch constraints. A truthy value assigned from map[key] proves
// the runtime key is present in that map; flow owns this source-evidence law so
// condition extraction does not inspect AssignmentSource internals.
func TruthyAssignmentValueKeyOfConstraints(q TruthyAssignmentValueKeyOfQuery) []constraint.Constraint {
	if q.ValuePath.IsEmpty() || q.ValuePath.Symbol == 0 {
		return nil
	}
	var out []constraint.Constraint
	for _, assign := range q.Assignments {
		if assign.TargetPath.IsEmpty() {
			continue
		}
		targetPath := versionAssignmentSourcePath(assign.TargetPath, q.VersionPath)
		if !targetPath.Equal(q.ValuePath) {
			continue
		}
		if keyOf, ok := AssignmentSourceKeyOfConstraint(assign.Source, q.VersionPath); ok {
			out = append(out, keyOf)
		}
	}
	return out
}

// AssignmentSourceKeyOfConstraint returns the key-presence constraint implied by
// a map-element assignment source.
func AssignmentSourceKeyOfConstraint(source AssignmentSource, version PathVersioner) (constraint.KeyOf, bool) {
	if source.Kind != AssignmentSourceMapElement || source.MapPath.IsEmpty() || source.KeySymbol == 0 {
		return constraint.KeyOf{}, false
	}
	tablePath := versionAssignmentSourcePath(source.MapPath, version)
	if tablePath.IsEmpty() {
		return constraint.KeyOf{}, false
	}
	keyRoot := source.KeyVar
	if keyRoot == "" {
		keyRoot = source.MapPath.Root
	}
	keyPath := versionAssignmentSourcePath(constraint.Path{Root: keyRoot, Symbol: source.KeySymbol}, version)
	if keyPath.IsEmpty() {
		return constraint.KeyOf{}, false
	}
	return constraint.KeyOf{Table: tablePath, Key: keyPath}, true
}

func versionAssignmentSourcePath(path constraint.Path, version PathVersioner) constraint.Path {
	if version == nil {
		return path
	}
	return version(path)
}
