package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// frozenMemberUseClosure is forest-owned evidence that one method value's
// complete runtime use set consists of the exact prepared call sites below.
// It is not inferred from a binder count: complete call surfaces, typed
// metatable attachment operations, exact member writes, and every binder read
// jointly participate in the certificate.
type frozenMemberUseClosure struct {
	callSites int
	complete  bool
}

type frozenMethodTarget struct {
	function symbol.ID
	name     string
}

func freezeMemberUseClosures(prepared preparedBodies, proof metatableMethodProof) map[symbol.ID]frozenMemberUseClosure {
	if prepared.bindings == nil || proof.empty() || !prepared.callTopology.Complete() {
		return nil
	}
	targetsByTable := make(map[symbol.ID]map[string]frozenMethodTarget)
	methodTables := make(map[symbol.ID]symbol.ID)
	definitionReads := make(map[symbol.ID]map[*ast.IdentExpr]struct{})
	prepared.bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.Kind != bind.FunctionOriginMethod || origin.Symbol == 0 || origin.Method == "" {
			return true
		}
		table, ok := prepared.bindings.MethodOriginReceiverSymbol(origin)
		if !ok || table == 0 {
			return true
		}
		if targetsByTable[table] == nil {
			targetsByTable[table] = make(map[string]frozenMethodTarget)
		}
		// Duplicate definitions of one member are mutable topology, so neither
		// definition receives a closed-use certificate.
		if _, duplicate := targetsByTable[table][origin.Method]; duplicate {
			targetsByTable[table][origin.Method] = frozenMethodTarget{}
			return true
		}
		targetsByTable[table][origin.Method] = frozenMethodTarget{function: origin.Symbol, name: origin.Method}
		methodTables[origin.Symbol] = table
		if stmt, ok := origin.Stmt.(*ast.FuncDefStmt); ok && stmt != nil && stmt.Name != nil {
			if ident := receiverRootIdent(stmt.Name.Receiver); ident != nil {
				if definitionReads[table] == nil {
					definitionReads[table] = make(map[*ast.IdentExpr]struct{})
				}
				definitionReads[table][ident] = struct{}{}
			}
		}
		return true
	})
	if len(methodTables) == 0 {
		return nil
	}

	attachedTables := attachedMethodTables(prepared, proof)
	callCounts := make(map[symbol.ID]int)
	callStarts := make(map[symbol.ID]map[sourceStart]struct{})
	validTables := make(map[symbol.ID]bool, len(targetsByTable))
	for table := range targetsByTable {
		validTables[table] = attachedTables[table] && !prepared.bindings.HasWrite(table)
	}

	for _, static := range prepared.allStatics() {
		plan := static.OperationPlan()
		graph := static.Graph()
		if plan == nil || graph == nil {
			for table := range validTables {
				validTables[table] = false
			}
			continue
		}
		facts := plan.Facts()
		surface, surfaceOK := plan.CallSurface()
		if !surfaceOK {
			for table := range validTables {
				validTables[table] = false
			}
			continue
		}
		for point := cfg.Point(0); int(point) < graph.Size(); point++ {
			if site, ok := facts.CallSiteView(point); ok {
				if _, present := surface.Site(point); !present {
					for table := range validTables {
						validTables[table] = false
					}
					continue
				}
				table, member, matched := memberCallTarget(site)
				if matched {
					target := targetsByTable[table][member]
					if target.function != 0 {
						callCounts[target.function]++
						if callStarts[table] == nil {
							callStarts[table] = make(map[sourceStart]struct{})
						}
						span := site.CalleeSpan()
						callStarts[table][sourceStart{line: span.StartLine, column: span.StartCol}] = struct{}{}
					}
				}
			}
			if assignment, ok := facts.PathAssignment(point); ok {
				validateMethodTableWrite(validTables, targetsByTable, facts, assignment.TargetPathRef(), assignment.Source())
			}
			if assignment, ok := facts.PathStaticMemberWrite(point); ok {
				validateMethodTableWrite(validTables, targetsByTable, facts, assignment.TargetPathRef(), assignment.Source())
			}
			if write, ok := facts.DynamicIndexWrite(point); ok {
				if table := write.TablePathRef().Symbol; validTables[table] {
					validTables[table] = false
				}
			}
		}
		facts.ForEachDynamicIndexExpression(func(_ factflow.ExprRef, expression factflow.DynamicIndexExpression) bool {
			if table := expression.TablePathRef().Symbol; validTables[table] {
				validTables[table] = false
			}
			return true
		})
	}

	for table := range validTables {
		if !validTables[table] {
			continue
		}
		for _, read := range prepared.bindings.ReadIdents(table) {
			if _, ok := definitionReads[table][read]; ok {
				continue
			}
			if _, ok := proof.metaIndexReads[table][read]; ok {
				continue
			}
			if _, ok := callStarts[table][sourceStart{line: read.Line(), column: read.Column()}]; ok {
				continue
			}
			validTables[table] = false
			break
		}
	}

	out := make(map[symbol.ID]frozenMemberUseClosure)
	for function, table := range methodTables {
		if validTables[table] && callCounts[function] > 0 {
			out[function] = frozenMemberUseClosure{callSites: callCounts[function], complete: true}
		}
	}
	return out
}

type sourceStart struct {
	line   int
	column int
}

func receiverRootIdent(expr ast.Expr) *ast.IdentExpr {
	for {
		switch current := expr.(type) {
		case *ast.IdentExpr:
			return current
		case *ast.AttrGetExpr:
			expr = current.Object
		default:
			return nil
		}
	}
}

func memberCallTarget(site factflow.CallSiteView) (symbol.ID, string, bool) {
	if site.MethodName() != "" {
		receiver, ok := site.ReceiverPath()
		return receiver.Symbol, site.MethodName(), ok && receiver.Symbol != 0 && len(receiver.Segments) == 0
	}
	receiver, member, ok := site.CalleeMemberAccessPath()
	if !ok || receiver.Symbol == 0 || len(receiver.Segments) != 0 {
		return 0, "", false
	}
	name, ok := staticMemberName(member)
	return receiver.Symbol, name, ok
}

func staticMemberName(member segment.Segment) (string, bool) {
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return member.Name, member.Name != ""
	default:
		return "", false
	}
}

func validateMethodTableWrite(valid map[symbol.ID]bool, targets map[symbol.ID]map[string]frozenMethodTarget, facts factflow.Facts, targetPath pathdom.Path, source factflow.ValueSource) {
	table := targetPath.Symbol
	if !valid[table] {
		return
	}
	if len(targetPath.Segments) != 1 {
		valid[table] = false
		return
	}
	name, ok := staticMemberName(targetPath.Segments[0])
	target := targets[table][name]
	if !ok || target.function == 0 || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		valid[table] = false
		return
	}
	function, ok := facts.ExpressionFunction(source.ExprRef)
	if !ok || function != target.function {
		valid[table] = false
	}
}

func attachedMethodTables(prepared preparedBodies, proof metatableMethodProof) map[symbol.ID]bool {
	out := make(map[symbol.ID]bool)
	for _, static := range prepared.allStatics() {
		plan := static.OperationPlan()
		graph := static.Graph()
		if plan == nil || graph == nil {
			continue
		}
		facts := plan.Facts()
		for point := cfg.Point(0); int(point) < graph.Size(); point++ {
			op, ok := plan.AttachMetatableOperation(point)
			if !ok {
				continue
			}
			meta := op.Metatable()
			if meta.Kind != factflow.ValueSourceExpression || !meta.HasExpr {
				continue
			}
			p, ok := facts.ExpressionPathRef(meta.ExprRef)
			if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
				continue
			}
			if methods := proof.metaIndexes[p.Symbol]; methods != 0 {
				out[methods] = true
			}
		}
	}
	return out
}

func (p preparedBodies) allStatics() []*body.Static {
	out := make([]*body.Static, 0, len(p.functions)+1)
	if p.root != nil {
		out = append(out, p.root)
	}
	for _, fn := range p.bindings.Functions() {
		if static := p.functions[fn]; static != nil && static != p.root {
			out = append(out, static)
		}
	}
	return out
}
