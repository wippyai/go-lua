package functionfact

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractEnvironmentReturns extracts environment-return facts for a function
// from solved abstract-interpreter evidence.
func ExtractEnvironmentReturns(result *api.FuncResult, fnSym cfg.SymbolID, observer observation.Projector) []contract.EnvReturnSpec {
	if fnSym == 0 || result == nil || result.Graph == nil {
		return nil
	}
	root, ok := exportedFunctionRoot(result.Graph.NameOf(fnSym))
	if !ok {
		return nil
	}
	var specs []contract.EnvReturnSpec
	result.Graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		info.EachSourceCall(func(exprIndex int, call *cfg.CallInfo) {
			if call == nil || call.Call == nil {
				return
			}
			path := call.CalleePath
			if envPathRootName(result.Graph, path) != root {
				return
			}
			spec := contract.EnvReturnSpec{
				Method: call.Method,
				Path:   cloneConstraintSegments(path.Segments),
				Args:   envReturnArgTypes(observer, p, call.Args),
			}
			if when := envReturnCondition(result, p); when.IsFalse() {
				return
			} else {
				spec.When = when
			}
			resultCount := 1
			if exprIndex == len(info.Exprs)-1 {
				if returns := observer.MultiTypeOf(call.Call, p); len(returns) > 0 {
					resultCount = len(returns)
				}
			}
			for resultIndex := 0; resultIndex < resultCount; resultIndex++ {
				next := spec
				next.ReturnIndex = exprIndex + resultIndex
				next.ResultIndex = resultIndex
				specs = append(specs, next)
			}
		})
	})
	return NormalizeEnvReturns(specs)
}

func envReturnCondition(result *api.FuncResult, p cfg.Point) constraint.Condition {
	if result == nil || result.Graph == nil {
		return constraint.TrueCondition()
	}
	proofs := result.ConditionProofFacts()
	if proofs == nil {
		return constraint.TrueCondition()
	}
	return flow.ParameterCondition(proofs.ConditionAt(p), envReturnParams(result.Graph))
}

func envReturnParams(graph *cfg.Graph) []flow.ParamInfo {
	slots := graph.ParamSlotsReadOnly()
	if len(slots) == 0 {
		return nil
	}
	params := make([]flow.ParamInfo, 0, len(slots))
	for _, slot := range slots {
		params = append(params, flow.ParamInfo{
			Name:   slot.Name,
			Symbol: slot.Symbol,
		})
	}
	return params
}

func exportedFunctionRoot(name string) (string, bool) {
	root, _, ok := strings.Cut(name, ".")
	if !ok || root == "" {
		return "", false
	}
	return root, true
}

func envPathRootName(graph *cfg.Graph, path constraint.Path) string {
	if graph != nil && path.Symbol != 0 {
		if name := graph.NameOf(path.Symbol); name != "" {
			return name
		}
	}
	return path.Root
}

func envReturnArgTypes(observer observation.Projector, p cfg.Point, args []ast.Expr) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		if arg != nil {
			out[i] = observer.TypeOf(arg, p)
		}
	}
	return out
}

func cloneConstraintSegments(segments []constraint.Segment) []constraint.Segment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(segments))
	copy(out, segments)
	return out
}

// NormalizeEnvReturns canonicalizes environment-return product facts.
func NormalizeEnvReturns(specs []contract.EnvReturnSpec) []contract.EnvReturnSpec {
	return mergeEnvReturns(nil, specs, func(_, candidate typ.Type) typ.Type {
		return value.NormalizeFactType(candidate)
	})
}

// JoinEnvReturns precisely merges environment-return observations in one
// analysis round.
func JoinEnvReturns(existing, candidate []contract.EnvReturnSpec) []contract.EnvReturnSpec {
	return mergeEnvReturns(existing, candidate, value.JoinPrecise)
}

// WidenEnvReturns merges environment-return observations at a recursive
// fixpoint boundary.
func WidenEnvReturns(existing, candidate []contract.EnvReturnSpec) []contract.EnvReturnSpec {
	return mergeEnvReturns(existing, candidate, value.MergeForConvergence)
}

// EnvReturnsEqual compares canonical environment-return fact slices.
func EnvReturnsEqual(a, b []contract.EnvReturnSpec) bool {
	a = NormalizeEnvReturns(a)
	b = NormalizeEnvReturns(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !envReturnSpecEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func mergeEnvReturns(
	existing []contract.EnvReturnSpec,
	candidate []contract.EnvReturnSpec,
	mergeType func(typ.Type, typ.Type) typ.Type,
) []contract.EnvReturnSpec {
	if len(existing) == 0 && len(candidate) == 0 {
		return nil
	}
	merged := make([]contract.EnvReturnSpec, 0, len(existing)+len(candidate))
	add := func(spec contract.EnvReturnSpec) {
		normalized, ok := normalizeEnvReturn(spec, mergeType)
		if !ok {
			return
		}
		for i, current := range merged {
			if envReturnMergeIdentityEqual(current, normalized) {
				normalized.Args = mergeEnvReturnArgs(current.Args, normalized.Args, mergeType)
				merged[i] = normalized
				return
			}
		}
		merged = append(merged, normalized)
	}
	for _, spec := range existing {
		add(spec)
	}
	for _, spec := range candidate {
		add(spec)
	}
	if len(merged) == 0 {
		return nil
	}
	slices.SortFunc(merged, compareEnvReturnIdentity)
	return merged
}

func normalizeEnvReturn(
	spec contract.EnvReturnSpec,
	normalizeType func(typ.Type, typ.Type) typ.Type,
) (contract.EnvReturnSpec, bool) {
	if spec.ReturnIndex < 0 || spec.ResultIndex < 0 {
		return contract.EnvReturnSpec{}, false
	}
	out := contract.EnvReturnSpec{
		When:        spec.When,
		ReturnIndex: spec.ReturnIndex,
		ResultIndex: spec.ResultIndex,
		Method:      spec.Method,
	}
	if len(out.When.Disjuncts) == 0 {
		out.When = constraint.TrueCondition()
	}
	if len(spec.Path) > 0 {
		out.Path = make([]constraint.Segment, len(spec.Path))
		copy(out.Path, spec.Path)
	}
	if len(spec.Args) > 0 {
		out.Args = make([]typ.Type, len(spec.Args))
		for i, arg := range spec.Args {
			out.Args[i] = normalizeType(nil, arg)
		}
	}
	return out, true
}

func mergeEnvReturnArgs(existing, candidate []typ.Type, mergeType func(typ.Type, typ.Type) typ.Type) []typ.Type {
	if len(existing) == 0 && len(candidate) == 0 {
		return nil
	}
	n := len(existing)
	if len(candidate) > n {
		n = len(candidate)
	}
	out := make([]typ.Type, n)
	for i := 0; i < n; i++ {
		var left, right typ.Type
		if i < len(existing) {
			left = existing[i]
		}
		if i < len(candidate) {
			right = candidate[i]
		}
		out[i] = mergeType(left, right)
	}
	return out
}

func envReturnSpecEqual(a, b contract.EnvReturnSpec) bool {
	if !a.When.Equals(b.When) {
		return false
	}
	if a.ReturnIndex != b.ReturnIndex || a.ResultIndex != b.ResultIndex || a.Method != b.Method {
		return false
	}
	if len(a.Path) != len(b.Path) || len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Path {
		if a.Path[i] != b.Path[i] {
			return false
		}
	}
	for i := range a.Args {
		if !value.FactTypeEqual(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

func envReturnMergeIdentityEqual(a, b contract.EnvReturnSpec) bool {
	if !a.When.Equals(b.When) {
		return false
	}
	if a.ReturnIndex != b.ReturnIndex || a.ResultIndex != b.ResultIndex || a.Method != b.Method {
		return false
	}
	if !segmentsEqual(a.Path, b.Path) || len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if !typ.TypeEquals(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

func compareEnvReturnIdentity(a, b contract.EnvReturnSpec) int {
	if c := cmp.Compare(a.ReturnIndex, b.ReturnIndex); c != 0 {
		return c
	}
	if c := cmp.Compare(a.ResultIndex, b.ResultIndex); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Method, b.Method); c != 0 {
		return c
	}
	if c := compareSegments(a.Path, b.Path); c != 0 {
		return c
	}
	if c := cmp.Compare(a.When.Hash(), b.When.Hash()); c != 0 {
		return c
	}
	return compareEnvReturnArgsIdentity(a.Args, b.Args)
}

func compareSegments(a, b []constraint.Segment) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if c := cmp.Compare(a[i].Kind, b[i].Kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Name, b[i].Name); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Index, b[i].Index); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}

func segmentsEqual(a, b []constraint.Segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func compareEnvReturnArgsIdentity(a, b []typ.Type) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		switch {
		case a[i] == nil && b[i] == nil:
			continue
		case a[i] == nil:
			return -1
		case b[i] == nil:
			return 1
		}
		if c := cmp.Compare(a[i].Kind(), b[i].Kind()); c != 0 {
			return c
		}
		if c := cmp.Compare(a[i].Hash(), b[i].Hash()); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(a), len(b))
}
