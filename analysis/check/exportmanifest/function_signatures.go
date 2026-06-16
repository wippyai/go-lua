package exportmanifest

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/module/signature"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func publishFunctionSignatures(m *manifest.Manifest, modulePath string, result program.Result) {
	root := result.RootResult()
	if m == nil || modulePath == "" || root == nil || root.Graph() == nil {
		return
	}
	for _, exportRoot := range returnedSourcePaths(root) {
		publishFunctionDefinitionSignatures(m, modulePath, result, root, exportRoot)
	}
}

func returnedSourcePaths(result *body.Result) []pathdom.Path {
	var out []pathdom.Path
	seen := make(map[pathdom.PathKey]struct{})
	for _, point := range result.ReturnPoints() {
		fact, ok := result.ReturnFact(point)
		if !ok {
			continue
		}
		for _, source := range fact.Sources {
			if source.Kind != sourceprovenance.SourceExpression || source.Expr == nil {
				continue
			}
			p, ok := result.ExpressionPath(source.Expr)
			if !ok || p.IsEmpty() {
				continue
			}
			key := p.Key()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func publishFunctionDefinitionSignatures(
	m *manifest.Manifest,
	modulePath string,
	prog program.Result,
	root *body.Result,
	exportRoot pathdom.Path,
) {
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || fact.Func == nil || fact.Name == nil {
			continue
		}
		member, ok := functionDefinitionExportMember(root, exportRoot, fact.Name)
		if !ok {
			continue
		}
		name, ok := functionSignatureName(modulePath, member)
		if !ok {
			continue
		}
		fn, ok := functionSignatureType(root, fact.Func)
		if !ok {
			continue
		}
		sig := signature.Function{Type: fn}
		if summary, ok := functionSummary(prog, root, fact.Func); ok {
			sig.Effect = functionSummaryEffect(summary, fn)
		}
		m.DefineFunctionSignature(name, sig)
	}
}

func functionDefinitionExportMember(result *body.Result, root pathdom.Path, name *ast.FuncName) (segment.Segment, bool) {
	if name == nil {
		return segment.Segment{}, false
	}
	if name.Method != "" {
		receiver, ok := result.ExpressionPath(name.Receiver)
		if !ok || !receiver.Equal(root) {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentField, Name: name.Method}, true
	}
	target, ok := result.ExpressionPath(name.Func)
	if !ok || target.Symbol != root.Symbol {
		return segment.Segment{}, false
	}
	return directMemberSegment(root.Segments, target.Segments)
}

func functionSignatureName(modulePath string, member segment.Segment) (string, bool) {
	var memberName string
	switch member.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		memberName = member.Name
	default:
		return "", false
	}
	if modulePath == "" || memberName == "" {
		return "", false
	}
	return modulePath + "." + memberName, true
}

func functionSignatureType(result *body.Result, fn *ast.FunctionExpr) (*typ.Function, bool) {
	t, ok := functionDefinitionMemberType(result, fn)
	if !ok {
		return nil, false
	}
	fnType, ok := t.(*typ.Function)
	return fnType, ok && fnType != nil
}

func functionSummary(prog program.Result, result *body.Result, fn *ast.FunctionExpr) (summary.Summary, bool) {
	id, ok := result.FunctionSymbol(fn)
	if !ok || id == 0 {
		return summary.Summary{}, false
	}
	key, ok := prog.FunctionKey(id)
	if !ok {
		return summary.Summary{}, false
	}
	return prog.Snapshot().Read(key)
}

func functionSummaryEffect(s summary.Summary, fn *typ.Function) effect.Row {
	if fn == nil {
		return effect.Empty
	}
	labels := errorReturnLabels(s.ReturnPresenceRelations, len(fn.Returns))
	labels = append(labels, normalReturnParamRefinementLabels(s.NormalReturnParams, len(fn.Params))...)
	if len(labels) == 0 {
		return effect.Empty
	}
	row := effect.Empty
	for _, label := range labels {
		row = row.With(label)
	}
	return row
}

func normalReturnParamRefinementLabels(values []product.Value, arity int) []effect.Label {
	if arity <= 0 || len(values) == 0 {
		return nil
	}
	limit := arity
	if len(values) < limit {
		limit = len(values)
	}
	var out []effect.Label
	for i := range limit {
		p := product.PresenceOf(values[i])
		switch {
		case presence.Equal(p, presence.Absent()):
			out = append(out, postcondition.NormalReturnRefinement{
				Target:     effect.ParamRef{Index: i},
				Refinement: postcondition.Absent{},
			})
		case presence.Equal(p, presence.Present()):
			out = append(out, postcondition.NormalReturnRefinement{
				Target:     effect.ParamRef{Index: i},
				Refinement: postcondition.Present{},
			})
		}
	}
	return out
}

func errorReturnLabels(relations []summary.ReturnPresenceRelation, arity int) []effect.Label {
	if arity < 2 {
		return nil
	}
	byKey := make(map[returnPresenceKey]struct{}, len(relations))
	for _, relation := range relations {
		byKey[returnPresenceKeyOf(relation)] = struct{}{}
	}
	var out []effect.Label
	for _, relation := range relations {
		if !presence.Equal(relation.TriggerPresence, presence.Present()) ||
			!presence.Equal(relation.TargetPresence, presence.Absent()) {
			continue
		}
		errorIndex := relation.TriggerIndex
		valueIndex := relation.TargetIndex
		if valueIndex < 0 || errorIndex < 0 || valueIndex >= arity || errorIndex >= arity || valueIndex >= errorIndex {
			continue
		}
		complement := returnPresenceKey{
			triggerIndex:    errorIndex,
			triggerPresence: presence.Absent(),
			targetIndex:     valueIndex,
			targetPresence:  presence.Present(),
		}
		if _, ok := byKey[complement]; !ok {
			continue
		}
		out = append(out, returns.ErrorReturn{ValueIndex: valueIndex, ErrorIndex: errorIndex})
	}
	return out
}

type returnPresenceKey struct {
	triggerIndex    int
	triggerPresence presence.Value
	targetIndex     int
	targetPresence  presence.Value
}

func returnPresenceKeyOf(relation summary.ReturnPresenceRelation) returnPresenceKey {
	return returnPresenceKey{
		triggerIndex:    relation.TriggerIndex,
		triggerPresence: relation.TriggerPresence,
		targetIndex:     relation.TargetIndex,
		targetPresence:  relation.TargetPresence,
	}
}
