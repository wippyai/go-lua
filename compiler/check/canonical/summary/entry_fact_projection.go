package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

// directCallEntryFactInput is the call-boundary projection for point-local path
// facts. It consumes normalized runtime arguments plus caller-side argument
// paths and emits parameter-relative boundary facts for the callee entry key.
type directCallEntryFactInput struct {
	Call   *ast.FuncCallExpr
	Callee FuncRef

	ParamSlot EntryValueParamSlot
	ArgPath   EntryReferenceArgPath

	KeyPresence flow.KeyPresenceFacts
	Num         *numeric.State
	IndexWrites flow.IndexWriteAdmissionFacts
}

// directCallEntryFacts projects caller point-local path facts into callee
// parameter-relative entry facts. Facts are emitted only when every referenced
// path is structurally owned by one of the runtime argument paths.
func directCallEntryFacts(in directCallEntryFactInput) flow.BoundaryFacts {
	if in.Call == nil || in.ParamSlot == nil || in.ArgPath == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	keyFacts := flow.ProjectKeyPresenceBoundaryFacts(
		in.KeyPresence,
		entryBoundaryProjector{in: in},
		flow.KeyPresenceBoundaryProjection{},
	)
	var lenLower []flow.BoundaryLengthLowerBound
	if in.Num != nil && !in.Num.IsUnsat() {
		in.Num.ForEachLenBound(func(key constraint.PathKey, lower, _ int64) bool {
			if lower <= 0 {
				return true
			}
			target, ok := entryBoundaryPathForCallerKey(in, key)
			if ok {
				lenLower = append(lenLower, flow.BoundaryLengthLowerBound{Target: target, Lower: lower})
			}
			return true
		})
	}
	var indexWrites []flow.BoundaryIndexWriteFact
	if !in.IndexWrites.IsBottom() {
		in.IndexWrites.ForEachAddress(func(fact flow.IndexWriteAdmissionAddressFact) bool {
			if !fact.HasKeyPath || fact.Value.IsZero() {
				return true
			}
			table, ok := entryBoundaryPathForCallerAddress(in, fact.Target)
			if !ok {
				return true
			}
			key, ok := entryBoundaryPathForCallerAddress(in, fact.KeyPath)
			if !ok {
				return true
			}
			indexWrites = append(indexWrites, flow.BoundaryIndexWriteFact{
				Table: table,
				Key:   key,
				Value: fact.Value,
			})
			return true
		})
	}
	return flow.BoundaryFactsOf(
		keyFacts.KeyPresence(),
		keyFacts.KeyArrays(),
		keyFacts.KeyArrayValues(),
		keyFacts.AppendKeys(),
		lenLower,
		indexWrites,
	).WithAppendElementFieldOrigins(keyFacts.AppendElementFieldOrigins())
}

func entryBoundaryPathForCallerKey(in directCallEntryFactInput, key constraint.PathKey) (flow.BoundaryPath, bool) {
	addr, ok := flow.StableAddressFromCanonicalKey(key)
	if !ok {
		return flow.BoundaryPath{}, false
	}
	return entryBoundaryPathForCallerAddress(in, addr)
}

func entryBoundaryPathForCallerAddress(in directCallEntryFactInput, addr flow.StableAddress) (flow.BoundaryPath, bool) {
	paths := (entryBoundaryProjector{in: in}).PathsFromAddress(addr)
	if len(paths) == 0 {
		return flow.BoundaryPath{}, false
	}
	return paths[0], true
}

type entryBoundaryProjector struct {
	in directCallEntryFactInput
}

func (p entryBoundaryProjector) PathsFromAddress(addr flow.StableAddress) []flow.BoundaryPath {
	target, ok := addr.Path()
	if !ok {
		return nil
	}
	for _, arg := range entryRuntimeArgs(p.in.Callee, p.in.Call, p.in.ParamSlot) {
		source, ok := p.in.ArgPath(arg.RuntimeIdx, arg.Expr)
		if !ok || source.IsEmpty() {
			continue
		}
		if path, ok := flow.BoundaryParamPathFromPath(target, source, arg.Slot); ok {
			return []flow.BoundaryPath{path}
		}
	}
	return nil
}
