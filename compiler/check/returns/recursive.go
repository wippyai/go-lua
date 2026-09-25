package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

// recursionVariableName names the recursion variables of recursive returns.
const recursionVariableName = "rec"

// CallsItself reports whether the function bound to sym can call itself: its
// body, or the body of a local function it reaches through the local
// functions it captures, references sym.
func CallsItself(store api.StoreView, sym cfg.SymbolID) bool {
	if store == nil || sym == 0 {
		return false
	}
	graphs := store.Graphs()
	visited := map[cfg.SymbolID]bool{sym: true}
	queue := []cfg.SymbolID{sym}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ref := store.FunctionRefBySym(current)
		if ref == nil || ref.Func == nil {
			continue
		}
		bindings := store.ModuleBindings()
		if g := graphs[ref.GraphID]; g != nil && g.Bindings() != nil {
			bindings = g.Bindings()
		}
		if bindings == nil {
			continue
		}
		for _, captured := range bindings.CapturedSymbols(ref.Func) {
			if captured == sym {
				return true
			}
			if !visited[captured] {
				visited[captured] = true
				queue = append(queue, captured)
			}
		}
	}
	return false
}

// RecursionVariables returns one fresh recursion variable per return slot. A
// recursive function without a summary yet types its recursive calls with
// them; TieRecursiveReturns then binds each to the slot inferred with it.
func RecursionVariables(arity int) []typ.Type {
	if arity < 1 {
		arity = 1
	}
	out := make([]typ.Type, arity)
	for i := range out {
		out[i] = typ.NewRecursivePlaceholder(recursionVariableName)
	}
	return out
}

// TieRecursiveReturns closes the returns next of a recursive function over
// self, the returns its recursive calls were typed with.
//
// A slot of self that is a recursion variable is bound to the slot of next,
// without the variable itself as a top-level member: a recursive call whose
// result is returned as is adds nothing to the least fixpoint. Any other slot
// of self is an earlier estimate; every occurrence of it nested in next is a
// recursive call's result and becomes the recursion variable of
// mu X. next[self := X]. Its body is the slot, so a converged estimate
// reproduces itself.
func TieRecursiveReturns(self, next []typ.Type) []typ.Type {
	if len(self) == 0 {
		return next
	}
	out := append([]typ.Type(nil), next...)
	for i, s := range self {
		if s == nil {
			continue
		}
		if variable, ok := s.(*typ.Recursive); ok && variable.Name == recursionVariableName && variable.Body == nil {
			body := typ.Type(typ.Nil)
			if i < len(out) && out[i] != nil {
				body = withoutMember(out[i], variable)
			}
			variable.SetBody(body)
			if i < len(out) {
				out[i] = body
			}
			continue
		}
		if i >= len(out) || out[i] == nil || typ.IsUnknown(s) || typ.TypeEquals(s, out[i]) {
			continue
		}
		folded := typ.FoldApproximations(recursionVariableName, out[i], func(node typ.Type) bool {
			return typ.TypeEquals(node, s)
		})
		if rec, ok := folded.(*typ.Recursive); ok && folded != out[i] {
			out[i] = rec.Body
		}
	}
	return out
}

// withoutMember returns t without member at its top level.
func withoutMember(t, member typ.Type) typ.Type {
	if t == member {
		return typ.Never
	}
	if opt, ok := t.(*typ.Optional); ok && opt.Inner == member {
		return typ.Nil
	}
	u, ok := t.(*typ.Union)
	if !ok {
		return t
	}
	kept := make([]typ.Type, 0, len(u.Members))
	for _, m := range u.Members {
		if m != member {
			kept = append(kept, m)
		}
	}
	if len(kept) == len(u.Members) {
		return t
	}
	return typ.NewUnion(kept...)
}
