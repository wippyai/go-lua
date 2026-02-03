package sibling

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// ConstraintsForIdent returns sibling constraints for error-return patterns using bindings.
// If wantNil is true, siblings are constrained to nil (error path).
// If wantNil is false, siblings are constrained to not-nil (success path).
func ConstraintsForIdent(ident *ast.IdentExpr, p cfg.Point, inputs *flow.Inputs, wantNil bool) []constraint.Constraint {
	if ident == nil || inputs == nil || inputs.SiblingAssignments == nil || inputs.Graph == nil {
		return nil
	}
	g, ok := inputs.Graph.(*cfg.Graph)
	if !ok {
		return nil
	}
	bindings := g.Bindings()
	if bindings == nil {
		return nil
	}
	sym, found := bindings.SymbolOf(ident)
	if !found || sym == 0 {
		return nil
	}
	ver := g.VisibleVersion(p, sym)
	if ver.ID == 0 {
		return nil
	}
	return ConstraintsForSymbol(sym, ver.ID, inputs, wantNil, bindings)
}

// ConstraintsForSymbol returns sibling constraints given a resolved symbol.
func ConstraintsForSymbol(sym cfg.SymbolID, versionID int, inputs *flow.Inputs, wantNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	if sym == 0 || versionID == 0 || inputs == nil || inputs.SiblingAssignments == nil {
		return nil
	}
	key := flow.SiblingKey{Symbol: sym, VersionID: versionID}
	sibling, found := inputs.SiblingAssignments[key]
	if !found || sibling == nil || len(sibling.Names) < 2 {
		return nil
	}
	pos := -1
	for i, s := range sibling.Symbols {
		if s == sym {
			pos = i
			break
		}
	}
	if pos < 0 {
		return nil
	}

	if len(sibling.Correlations) > 0 || len(sibling.CoCorrelations) > 0 {
		var result []constraint.Constraint
		result = append(result, correlatedSiblingConstraints(sibling, pos, wantNil, bindings)...)
		result = append(result, coCorrelatedSiblingConstraints(sibling, pos, wantNil, bindings)...)
		if len(result) > 0 {
			return result
		}
	}
	return nil
}

// correlatedSiblingConstraints emits constraints using spec-driven ErrorReturn correlations.
// Bidirectional: checking error narrows value, and checking value narrows error.
func correlatedSiblingConstraints(sibling *flow.SiblingAssignment, pos int, wantNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	var result []constraint.Constraint
	for _, cor := range sibling.Correlations {
		partnerIdx := -1
		switch pos {
		case cor.ErrorIndex:
			partnerIdx = cor.ValueIndex
		case cor.ValueIndex:
			partnerIdx = cor.ErrorIndex
		}
		if partnerIdx < 0 || partnerIdx >= len(sibling.Names) {
			continue
		}
		sibName := sibling.Names[partnerIdx]
		if sibName == "" {
			continue
		}
		var sibSym cfg.SymbolID
		if partnerIdx < len(sibling.Symbols) {
			sibSym = sibling.Symbols[partnerIdx]
		}
		root := sibName
		if sibSym != 0 && bindings != nil {
			if name := bindings.Name(sibSym); name != "" {
				root = name
			}
		}
		path := constraint.Path{Root: root, Symbol: sibSym}
		if wantNil {
			result = append(result, constraint.IsNil{Path: path})
		} else {
			result = append(result, constraint.NotNil{Path: path})
		}
	}
	return result
}

// coCorrelatedSiblingConstraints emits constraints using spec-driven CorrelatedReturn correlations.
// Same-direction: when one index is non-nil, the partner is also non-nil (and vice versa).
// The direction is flipped relative to correlatedSiblingConstraints because the call site
// applies inverse semantics for ErrorReturn; co-correlation compensates by flipping again.
func coCorrelatedSiblingConstraints(sibling *flow.SiblingAssignment, pos int, wantNil bool, bindings *bind.BindingTable) []constraint.Constraint {
	var result []constraint.Constraint
	for _, cor := range sibling.CoCorrelations {
		partnerIdx := -1
		switch pos {
		case cor.ValueIndex:
			partnerIdx = cor.ErrorIndex
		case cor.ErrorIndex:
			partnerIdx = cor.ValueIndex
		}
		if partnerIdx < 0 || partnerIdx >= len(sibling.Names) {
			continue
		}
		sibName := sibling.Names[partnerIdx]
		if sibName == "" {
			continue
		}
		var sibSym cfg.SymbolID
		if partnerIdx < len(sibling.Symbols) {
			sibSym = sibling.Symbols[partnerIdx]
		}
		root := sibName
		if sibSym != 0 && bindings != nil {
			if name := bindings.Name(sibSym); name != "" {
				root = name
			}
		}
		path := constraint.Path{Root: root, Symbol: sibSym}
		if wantNil {
			result = append(result, constraint.NotNil{Path: path})
		} else {
			result = append(result, constraint.IsNil{Path: path})
		}
	}
	return result
}
