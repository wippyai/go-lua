package cfg

import "github.com/wippyai/go-lua/compiler/ast"

func buildLocalFunctionAssignments(infoByPoint []NodeInfo, assignPoints []Point) []LocalFunctionAssignment {
	if len(infoByPoint) == 0 || len(assignPoints) == 0 {
		return nil
	}

	var out []LocalFunctionAssignment
	for _, p := range assignPoints {
		idx := int(p)
		if idx < 0 || idx >= len(infoByPoint) {
			continue
		}
		info, ok := infoByPoint[idx].(*AssignInfo)
		if !ok || info == nil || !info.IsLocal || len(info.Targets) == 0 {
			continue
		}
		info.EachTargetSource(func(_ int, target AssignTarget, source ast.Expr) {
			if target.Kind != TargetIdent || target.Symbol == 0 {
				return
			}
			fn, ok := source.(*ast.FunctionExpr)
			if !ok || fn == nil {
				return
			}
			out = append(out, LocalFunctionAssignment{
				Symbol: target.Symbol,
				Name:   target.Name,
				Func:   fn,
			})
		})
	}
	return out
}
