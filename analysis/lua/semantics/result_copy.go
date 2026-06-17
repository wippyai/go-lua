package semantics

import (
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func copyExprs(in []ast.Expr) []ast.Expr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.Expr, len(in))
	copy(out, in)
	return out
}

func copyTypeExprs(in []ast.TypeExpr) []ast.TypeExpr {
	if len(in) == 0 {
		return nil
	}
	out := make([]ast.TypeExpr, len(in))
	copy(out, in)
	return out
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func copySymbols(in []symbol.ID) []symbol.ID {
	if len(in) == 0 {
		return nil
	}
	out := make([]symbol.ID, len(in))
	copy(out, in)
	return out
}

func completeSymbols(symbols []symbol.ID, want int) bool {
	if len(symbols) != want {
		return false
	}
	for _, id := range symbols {
		if id == 0 {
			return false
		}
	}
	return true
}

func copyLocalAssignmentFact(fact LocalAssignmentFact) LocalAssignmentFact {
	fact.Exprs = copyExprs(fact.Exprs)
	fact.Types = copyTypeExprs(fact.Types)
	return fact
}

func copyOrdinaryAssignmentFact(fact OrdinaryAssignmentFact) OrdinaryAssignmentFact {
	fact.Path = fact.Path.Clone()
	fact.ContainerPath = fact.ContainerPath.Clone()
	fact.Lhs = copyExprs(fact.Lhs)
	fact.Rhs = copyExprs(fact.Rhs)
	return fact
}

func copyCallFact(fact CallFact) CallFact {
	fact.Args = copyExprs(fact.Args)
	fact.TypeArgs = copyTypeExprs(fact.TypeArgs)
	fact.ArgumentSources = copyValueSources(fact.ArgumentSources)
	fact.CalleePath = fact.CalleePath.Clone()
	fact.ReceiverPath = fact.ReceiverPath.Clone()
	fact.MethodPath = fact.MethodPath.Clone()
	fact.ResultTargets = copyResultTargets(fact.ResultTargets)
	fact.ChannelSelect = copyChannelSelectFact(fact.ChannelSelect)
	return fact
}

func copyReturnFact(fact ReturnFact) ReturnFact {
	fact.Exprs = copyExprs(fact.Exprs)
	fact.Sources = copyValueSources(fact.Sources)
	return fact
}

func copyObjectLiteralFact(fact ObjectLiteralFact) ObjectLiteralFact {
	fact.Entries = copyObjectEntries(fact.Entries)
	return fact
}

func copyObjectEntries(in []ObjectEntryFact) []ObjectEntryFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ObjectEntryFact, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Suffix = in[i].Suffix.Clone()
	}
	return out
}

func copyChannelSelectFact(fact ChannelSelectFact) ChannelSelectFact {
	fact.ResultTarget = copyResultTarget(fact.ResultTarget)
	fact.Cases = copyChannelSelectCases(fact.Cases)
	return fact
}

func copyChannelSelectCases(in []ChannelSelectCaseFact) []ChannelSelectCaseFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]ChannelSelectCaseFact, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].ChannelPath = in[i].ChannelPath.Clone()
	}
	return out
}

func copyValueSources(in []sourceprovenance.ASTSource) []sourceprovenance.ASTSource {
	if len(in) == 0 {
		return nil
	}
	out := make([]sourceprovenance.ASTSource, len(in))
	copy(out, in)
	return out
}

func copyResultTargets(in []CallResultTarget) []CallResultTarget {
	if len(in) == 0 {
		return nil
	}
	out := make([]CallResultTarget, len(in))
	for i := range in {
		out[i] = copyResultTarget(in[i])
	}
	return out
}

func copyResultTarget(target CallResultTarget) CallResultTarget {
	target.Path = target.Path.Clone()
	return target
}
