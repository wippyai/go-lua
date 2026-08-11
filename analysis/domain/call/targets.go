package call

import (
	"github.com/wippyai/go-lua/program/keyspace"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

type targetKind uint8

const (
	targetBody targetKind = iota + 1
	targetSeed
)

type targetKey struct {
	kind  targetKind
	shard linkproject.Shard
	body  keyspace.Term
	seed  linkboundary.Seed
}

type functionTargetKey struct {
	shard    linkproject.Shard
	function keyspace.Term
}

type targetRow struct {
	key      targetKey
	function keyspace.Term // exact canonical Program function
}

func (algebra *Algebra) buildTargets() bool {
	project := algebra.source.Project()
	if project == nil {
		return false
	}
	mounts := project.Mounts()
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, present := mounts.At(shardIndex)
		p, available := mounts.Program(shard)
		if !present || !available || p == nil {
			return false
		}
		flow := p.Flow()
		functions := flow.Authored().Functions()
		executable := flow.Executable()
		var previousBody keyspace.Term
		for functionIndex := 0; functionIndex < functions.Count(); functionIndex++ {
			function, present := functions.At(functionIndex)
			if !present || function == 0 {
				return false
			}
			if !executable.Contains(function) {
				continue
			}
			_, body, _, related := functions.Get(function)
			if !related || body == 0 || !executable.Contains(body) || previousBody != 0 && body <= previousBody {
				return false
			}
			key := targetKey{kind: targetBody, shard: shard, body: body}
			if algebra.targetIndex[key].valid() || !algebra.appendTarget(targetRow{key: key, function: function}) {
				return false
			}
			previousBody = body
		}
	}
	// Executable function bodies are the sole body-target prefix.  Retaining
	// its width lets the public Bodies cursor reuse Call's canonical target
	// order without a second list, map, or per-Application projection.
	algebra.bodyTargetCount = len(algebra.targets)
	boundary := algebra.source.Boundary()
	if boundary == nil {
		return false
	}
	seeds := boundary.Seeds()
	for seedIndex := 0; seedIndex < seeds.Count(); seedIndex++ {
		seed, ok := seeds.At(seedIndex)
		if !ok {
			return false
		}
		// Boundary is the sole external-value classifier.  Operation covers
		// both direct Target operations and nominal provider endpoints; Loader
		// covers the scoped require ingress.  Denied bootstrap values satisfy
		// neither query and cannot enter Call's target universe.
		_, operation := seeds.Operation(seed)
		_, loader := seeds.Loader(seed)
		if !operation && !loader {
			continue
		}
		if !algebra.appendTarget(targetRow{key: targetKey{kind: targetSeed, seed: seed}}) {
			return false
		}
	}
	return true
}

func (algebra *Algebra) appendTarget(row targetRow) bool {
	if algebra == nil || len(algebra.targets) == int(^selector(0)) || algebra.targetIndex[row.key].valid() {
		return false
	}
	functionKey := functionTargetKey{}
	if row.key.kind == targetBody {
		functionKey = functionTargetKey{shard: row.key.shard, function: row.function}
		if row.function == 0 || algebra.functionIndex[functionKey].valid() {
			return false
		}
	}
	selector := selector(len(algebra.targets) + 1)
	algebra.targets = append(algebra.targets, row)
	algebra.targetIndex[row.key] = selector
	if row.key.kind == targetBody {
		algebra.functionIndex[functionKey] = selector
	}
	return true
}
