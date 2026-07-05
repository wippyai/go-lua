package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFactsNodeTransferWIRAssignmentTargetShadowAcceptsMatchingPathWrite(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1401)
	target := symbol.ID(1401)
	targetPath := path.NewPath(target, "box").Field("value")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1401), HasExpr: true}
	resolver := resolverForWIRAssignmentShadow(point, target, "box")
	body := wirBodyWithPathWrite(point, wir.OpStaticMemberWrite, targetPath)
	var issues []WIRAssignmentTargetIssue

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources:             sourceValuesForWIRAssignmentShadow(reg, source),
		Visibility:          resolver,
		WIR:                 body,
		WIRAssignmentTarget: func(issue WIRAssignmentTargetIssue) { issues = append(issues, issue) },
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if len(issues) != 0 {
		t.Fatalf("shadow issues = %#v, want none", issues)
	}
	assertPathValue(t, reg, resolver.KeySpace(), got, path.PathKey("sym1401@1.value"), presentValue(reg))
}

func TestFactsNodeTransferWIRAssignmentTargetShadowReportsMismatchWithoutChangingTransfer(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1402)
	target := symbol.ID(1402)
	targetPath := path.NewPath(target, "box").Field("value")
	wirPath := path.NewPath(target, "box").Field("other")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1402), HasExpr: true}
	resolver := resolverForWIRAssignmentShadow(point, target, "box")
	body := wirBodyWithPathWrite(point, wir.OpStaticMemberWrite, wirPath)
	var issues []WIRAssignmentTargetIssue

	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources:             sourceValuesForWIRAssignmentShadow(reg, source),
		Visibility:          resolver,
		WIR:                 body,
		WIRAssignmentTarget: func(issue WIRAssignmentTargetIssue) { issues = append(issues, issue) },
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if len(issues) != 1 {
		t.Fatalf("shadow issues = %#v, want one mismatch", issues)
	}
	if issues[0].Kind != WIRAssignmentTargetMismatch {
		t.Fatalf("issue kind = %v, want mismatch", issues[0].Kind)
	}
	wantFactKey, _ := visibility.AddressAt(resolver, point, targetPath).VisibleLocalKeyspaceKey()
	wantWIRKey, _ := visibility.AddressAt(resolver, point, wirPath).VisibleLocalKeyspaceKey()
	if issues[0].FactKey != wantFactKey || issues[0].WIRKey != wantWIRKey || issues[0].WIROp != wir.OpStaticMemberWrite {
		t.Fatalf("issue = %#v, want fact %v wir %v op %v", issues[0], wantFactKey, wantWIRKey, wir.OpStaticMemberWrite)
	}
	assertPathValue(t, reg, resolver.KeySpace(), got, path.PathKey("sym1402@1.value"), presentValue(reg))
}

func TestFactsNodeTransferWIRAssignmentTargetShadowReportsMissingTarget(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1403)
	target := symbol.ID(1403)
	targetPath := path.NewPath(target, "box").Field("value")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1403), HasExpr: true}
	resolver := resolverForWIRAssignmentShadow(point, target, "box")
	body := wir.NewBody("empty-assignment-shadow")
	var issues []WIRAssignmentTargetIssue

	NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{
				point: factflow.NewPathAssignment(targetPath, source),
			},
		}),
		Sources:             sourceValuesForWIRAssignmentShadow(reg, source),
		Visibility:          resolver,
		WIR:                 body,
		WIRAssignmentTarget: func(issue WIRAssignmentTargetIssue) { issues = append(issues, issue) },
	})(transfer.NodeContext{
		Registry: reg,
		Point:    point,
	}, state.State{})

	if len(issues) != 1 {
		t.Fatalf("shadow issues = %#v, want one missing-target issue", issues)
	}
	if issues[0].Kind != WIRAssignmentTargetMissing {
		t.Fatalf("issue kind = %v, want missing", issues[0].Kind)
	}
}

func resolverForWIRAssignmentShadow(point cfg.Point, target symbol.ID, name string) *visibility.Resolver {
	builder := visibility.NewBuilder()
	builder.Define(point, target, name)
	return visibility.NewResolver(builder.Build())
}

func sourceValuesForWIRAssignmentShadow(reg *axis.Registry, source factflow.ValueSource) *recordingSourceValues {
	return &recordingSourceValues{
		values: map[factflow.ValueSource]product.Value{source: presentValue(reg)},
	}
}

func wirBodyWithPathWrite(point cfg.Point, op wir.Op, p path.Path) *wir.Body {
	body := wir.NewBody("assignment-shadow")
	dst := wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(p))}
	start := body.Len()
	body.Emit(wir.Instruction{Op: op, Point: point, Dst: dst})
	body.SetPointRange(point, start, body.Len())
	return body
}
