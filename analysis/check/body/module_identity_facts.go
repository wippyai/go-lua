package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
)

type moduleIdentityFacts struct {
	facts factflow.Facts
}

func (m moduleIdentityFacts) LocalAssignment(point cfg.Point) (moduleidentity.Assignment, bool) {
	fact, ok := m.facts.LocalAssignment(point)
	if !ok {
		return moduleidentity.Assignment{}, false
	}
	return moduleIdentityAssignment(fact), true
}

func (m moduleIdentityFacts) OrdinaryAssignment(point cfg.Point) (moduleidentity.Assignment, bool) {
	fact, ok := m.facts.OrdinaryAssignment(point)
	if !ok {
		return moduleidentity.Assignment{}, false
	}
	return moduleIdentityAssignment(fact), true
}

func (m moduleIdentityFacts) PathAssignment(point cfg.Point) (moduleidentity.Assignment, bool) {
	fact, ok := m.facts.PathAssignment(point)
	if !ok {
		return moduleidentity.Assignment{}, false
	}
	target := fact.TargetPathRef()
	return moduleidentity.Assignment{
		Target:       target.Clone(),
		TargetSymbol: target.Symbol,
		Source:       moduleIdentitySource(fact.Source()),
	}, true
}

func (m moduleIdentityFacts) PathDescendantInvalidation(point cfg.Point) (pathdom.Path, bool) {
	fact, ok := m.facts.PathDescendantInvalidation(point)
	if !ok {
		return pathdom.Path{}, false
	}
	return fact.ContainerPath(), true
}

func (m moduleIdentityFacts) CallSite(point cfg.Point) (moduleidentity.CallSite, bool) {
	site, ok := m.facts.CallSiteView(point)
	if !ok {
		return moduleidentity.CallSite{}, false
	}
	args := site.ArgumentSources()
	outArgs := make([]moduleidentity.Source, 0, len(args))
	for _, arg := range args {
		outArgs = append(outArgs, moduleIdentitySource(arg))
	}
	return moduleidentity.CallSite{
		Callee:       site.CalleePath(),
		Args:         outArgs,
		TypeArgCount: len(site.TypeArgs()),
		MethodName:   site.MethodName(),
	}, true
}

func (m moduleIdentityFacts) ObjectLiteral(expr moduleidentity.SourceRef) ([]moduleidentity.ObjectEntry, bool) {
	lit, ok := m.facts.ObjectLiteralView(factflow.ExprRef(expr))
	if !ok {
		return nil, false
	}
	var out []moduleidentity.ObjectEntry
	lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		out = append(out, moduleidentity.ObjectEntry{
			Suffix: entry.Suffix(),
			Source: moduleIdentitySource(entry.Source()),
		})
		return true
	})
	return out, len(out) != 0
}

func (m moduleIdentityFacts) ExpressionPath(expr moduleidentity.SourceRef) (pathdom.Path, bool) {
	return m.facts.ExpressionPath(factflow.ExprRef(expr))
}

func moduleIdentityAssignment(fact factflow.RootAssignment) moduleidentity.Assignment {
	return moduleidentity.Assignment{
		Target:       fact.TargetPath(),
		TargetSymbol: fact.TargetSymbol(),
		Source:       moduleIdentitySource(fact.Source()),
	}
}

func moduleIdentitySource(source factflow.ValueSource) moduleidentity.Source {
	out := moduleidentity.Source{
		Expr:        moduleidentity.SourceRef(source.ExprRef),
		HasExpr:     source.HasExpr,
		CallPoint:   source.CallPoint,
		ResultIndex: source.ResultIndex,
		PathKey:     source.PathKey,
		String:      source.String,
	}
	switch source.Kind {
	case factflow.ValueSourceExpression:
		out.Kind = moduleidentity.SourceExpression
	case factflow.ValueSourceCall:
		out.Kind = moduleidentity.SourceCall
	case factflow.ValueSourcePath:
		out.Kind = moduleidentity.SourcePath
	case factflow.ValueSourceLiteral:
		if source.LiteralKind == factflow.ValueSourceLiteralString {
			out.Kind = moduleidentity.SourceStringLiteral
		}
	default:
		out.Kind = moduleidentity.SourceUnknown
	}
	return out
}
