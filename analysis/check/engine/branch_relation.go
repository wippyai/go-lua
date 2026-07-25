package engine

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	"github.com/wippyai/go-lua/analysis/domain/constraint/solver"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

const branchDiffPrefix = "front/branch-diff/v1/"

// branchDiffWire mirrors the front's normalized difference-logic branch
// descriptor
//
//	coHi*hi + coHi2*hi2 - lo <= c
//
// carried on the branch edge named by Edge. The engine reads the same closed
// encoding the front publishes; it never re-derives the relation from source.
type branchDiffWire struct {
	CoHi     int64  `json:"co_hi"`
	HiPath   string `json:"hi_path"`
	HiIsLen  bool   `json:"hi_is_len,omitempty"`
	CoHi2    int64  `json:"co_hi2,omitempty"`
	Hi2Path  string `json:"hi2_path,omitempty"`
	Hi2IsLen bool   `json:"hi2_is_len,omitempty"`
	HasHi2   bool   `json:"has_hi2,omitempty"`
	LoPath   string `json:"lo_path"`
	LoIsLen  bool   `json:"lo_is_len,omitempty"`
	C        int64  `json:"c,omitempty"`
	Edge     bool   `json:"edge,omitempty"`
}

// decodeBranchDiff reads one published difference descriptor. A descriptor with
// an empty operand names no relation and is dropped.
func decodeBranchDiff(encoded []byte) (branchDiffWire, bool) {
	if !strings.HasPrefix(string(encoded), branchDiffPrefix) {
		return branchDiffWire{}, false
	}
	var wire branchDiffWire
	if json.Unmarshal(encoded[len(branchDiffPrefix):], &wire) != nil {
		return branchDiffWire{}, false
	}
	if wire.HiPath == "" || wire.LoPath == "" || (wire.HasHi2 && wire.Hi2Path == "") {
		return branchDiffWire{}, false
	}
	return wire, true
}

// trueEdgeBranchDifferences collects the difference descriptors this branch
// proves on its true edge.
func trueEdgeBranchDifferences(operation equation.BoundEquation) []branchDiffWire {
	var out []branchDiffWire
	for _, operand := range operation.Operands {
		if !strings.HasPrefix(operand.Role, "difference-") {
			continue
		}
		if wire, ok := decodeBranchDiff(operand.Value); ok && wire.Edge {
			out = append(out, wire)
		}
	}
	return out
}

// artifactTrueEdgeLengthRelation reports a published difference descriptor that
// relates an operand to an array length on the branch's true edge. It reads the
// artifact form of the operand, before the partition binds it.
func artifactTrueEdgeLengthRelation(role string, encoding []byte) bool {
	if !strings.HasPrefix(role, "difference-") {
		return false
	}
	wire, ok := decodeBranchDiff(encoding)
	return ok && wire.Edge && (wire.LoIsLen || wire.HiIsLen || wire.Hi2IsLen)
}

// relationVariable names the solver variable for one operand of a normalized
// relation. The numeric IR treats every variable as an opaque key, so the
// value/length distinction has to survive in the key itself.
func relationVariable(pathKey string, isLength bool) pathdom.PathKey {
	if isLength {
		return pathdom.PathKey("len/" + pathKey)
	}
	return pathdom.PathKey("value/" + pathKey)
}

// relationAssertions lowers the bound evidence a branch proves on its true edge
// into the numeric constraint IR the solver portfolio consumes: a normalized
// floor becomes GeConst, an index-in-range predicate becomes value <= len, and
// each difference descriptor becomes its Le or bounded-sum form.
func relationAssertions(predicates []branchPredicateWire, differences []branchDiffWire) []numeric.NumericConstraint {
	asserted := make([]numeric.NumericConstraint, 0, len(predicates)+len(differences))
	for _, predicate := range predicates {
		if predicate.Negated || predicate.Path == "" {
			continue
		}
		switch predicate.Kind {
		case "num-ge":
			asserted = append(asserted, numeric.GeConst{X: relationVariable(predicate.Path, false), C: predicate.NumFloor})
		case "index-in-range":
			if predicate.OtherPath == "" {
				continue
			}
			asserted = append(asserted, numeric.Le{
				X: relationVariable(predicate.Path, false),
				Y: relationVariable(predicate.OtherPath, true),
				C: 0,
			})
		}
	}
	for _, difference := range differences {
		low := relationVariable(difference.LoPath, difference.LoIsLen)
		high := relationVariable(difference.HiPath, difference.HiIsLen)
		if !difference.HasHi2 && difference.CoHi == 1 {
			asserted = append(asserted, numeric.Le{X: high, Y: low, C: difference.C})
			continue
		}
		second := pathdom.PathKey("")
		coefficient := int64(0)
		if difference.HasHi2 {
			second = relationVariable(difference.Hi2Path, difference.Hi2IsLen)
			coefficient = difference.CoHi2
		}
		asserted = append(asserted, numeric.NewScaledLe(difference.CoHi, high, coefficient, second, low, difference.C))
	}
	return asserted
}

// relationContainers lists the arrays whose length the branch's relations bound
// something against. A container that never appears as a length operand cannot
// be the subject of an in-range proof, so the candidate set is complete.
func relationContainers(predicates []branchPredicateWire, differences []branchDiffWire) []string {
	seen := make(map[string]bool)
	add := func(name string, isLength bool) {
		if isLength && name != "" {
			seen[name] = true
		}
	}
	for _, predicate := range predicates {
		if predicate.Kind == "index-in-range" && !predicate.Negated {
			add(predicate.OtherPath, true)
		}
	}
	for _, difference := range differences {
		add(difference.HiPath, difference.HiIsLen)
		add(difference.Hi2Path, difference.Hi2IsLen)
		add(difference.LoPath, difference.LoIsLen)
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// relationalIndexPair is one in-range conclusion the solver derived from the
// branch's relations: index is proven no greater than the length of container.
type relationalIndexPair struct{ index, container string }

// relationalIndexUpperBounds asks the wired solver portfolio which index/array
// pairs the branch's own relations put in range. Only an index that already
// carries a positive floor is a candidate: the upper bound alone never proves
// presence, and asking for it elsewhere would only cost solver time.
//
// The portfolio is the single constraint path. Difference logic answers the
// transitive and length-equality goals; the exact linear backend answers the
// bounded-sum residue difference logic cannot express.
func relationalIndexUpperBounds(predicates []branchPredicateWire, differences []branchDiffWire, indexes []string) []relationalIndexPair {
	if len(differences) == 0 || len(indexes) == 0 {
		return nil
	}
	containers := relationContainers(predicates, differences)
	if len(containers) == 0 {
		return nil
	}
	asserted := relationAssertions(predicates, differences)
	if len(asserted) == 0 {
		return nil
	}
	portfolio := solver.DefaultPortfolio()
	var proven []relationalIndexPair
	for _, index := range indexes {
		for _, container := range containers {
			goal := numeric.Le{X: relationVariable(index, false), Y: relationVariable(container, true), C: 0}
			if portfolio.Entails(asserted, goal) == decision.Valid {
				proven = append(proven, relationalIndexPair{index: index, container: container})
			}
		}
	}
	return proven
}

// provenFloorPaths lists the index paths that currently carry a positive floor,
// read back from the encoded lower-bound relations the branch lane maintains.
func provenFloorPaths(lower map[string][]byte) []string {
	out := make([]string, 0, len(lower))
	for _, index := range lower {
		if name, found := strings.CutPrefix(string(index), "path/"); found && name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
