package facts

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
)

// computeNoReturn computes the least fixed point of functions that never return
// normally. The set grows monotonically and is bounded by Program.Refs.
func computeNoReturn(p Program) []ref.FuncRef {
	noReturn := make(map[ref.FuncRef]bool)
	for _, ref := range p.Refs {
		g := graphOf(p, ref)
		if g == nil {
			continue
		}
		if !graphReachesExit(g, g.Entry()) {
			noReturn[ref] = true
		}
	}
	if p.ResolveCallee == nil {
		if len(noReturn) == 0 {
			return nil
		}
		return noReturnEntries(noReturn)
	}
	for changed := true; changed; {
		changed = false
		for _, ref := range p.Refs {
			if noReturn[ref] {
				continue
			}
			g := graphOf(p, ref)
			if g == nil {
				continue
			}
			if bodyAlwaysRaises(p, g, noReturn) {
				noReturn[ref] = true
				changed = true
			}
		}
	}
	if len(noReturn) == 0 {
		return nil
	}
	return noReturnEntries(noReturn)
}

func noReturnEntries(noReturn map[ref.FuncRef]bool) []ref.FuncRef {
	out := make([]ref.FuncRef, 0, len(noReturn))
	for ref, ok := range noReturn {
		if ok {
			out = append(out, ref)
		}
	}
	return out
}

func bodyAlwaysRaises(p Program, g *cfg.Graph, noReturn map[ref.FuncRef]bool) bool {
	raises := false
	g.EachCall(func(point cfg.Point, info *cfg.CallInfo) {
		if raises || info == nil || info.Call == nil {
			return
		}
		if !graphDominatesExit(g, point) {
			return
		}
		ref, ok := p.ResolveCallee(g, info.Call)
		if ok && noReturn[ref] {
			raises = true
		}
	})
	return raises
}

// graphReachesExit reports whether g's exit is reachable from p by a forward CFG
// walk. A path that terminates with error() has no successor to the exit.
func graphReachesExit(g *cfg.Graph, p cfg.Point) bool {
	if g == nil {
		return false
	}
	exit := g.Exit()
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{p}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == exit {
			return true
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return false
}

// graphDominatesExit reports whether q is on every entry-to-exit path: the exit
// is unreachable from the entry once q is removed.
func graphDominatesExit(g *cfg.Graph, q cfg.Point) bool {
	if g == nil {
		return false
	}
	entry := g.Entry()
	exit := g.Exit()
	if q == entry {
		return true
	}
	seen := make(map[cfg.Point]bool)
	stack := []cfg.Point{entry}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == q || seen[cur] {
			continue
		}
		if cur == exit {
			return false
		}
		seen[cur] = true
		stack = append(stack, g.Successors(cur)...)
	}
	return true
}
