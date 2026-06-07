package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
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
	return flow.ProjectBoundaryFacts(
		flow.BoundaryFactProjectionInput{
			KeyPresence: in.KeyPresence,
			Num:         in.Num,
			IndexWrites: in.IndexWrites,
		},
		entryBoundaryProjector{in: in},
		flow.BoundaryFactProjectionPolicy{},
	)
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
