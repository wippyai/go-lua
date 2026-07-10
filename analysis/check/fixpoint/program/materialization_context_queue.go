package program

import "github.com/wippyai/go-lua/analysis/check/fixpoint/summary"

// materializationContextQueue is a one-way cursor over contexts discovered while
// rendering already-converged summaries. It deliberately has no dirty/dependent
// API: materialization may append newly discovered contexts, but it cannot
// reschedule a key it already solved.
type materializationContextQueue struct {
	contexts *contextIndex
	cursor   int
	seen     map[summary.SummaryKey]struct{}
}

func newMaterializationContextQueue(keys *programKeys) *materializationContextQueue {
	if keys == nil {
		return &materializationContextQueue{}
	}
	return &materializationContextQueue{contexts: &keys.contexts}
}

func (q *materializationContextQueue) Next() (keyedFunction, bool) {
	if q == nil || q.contexts == nil {
		return keyedFunction{}, false
	}
	for q.cursor < q.contexts.Len() {
		context := q.contexts.Entry(q.cursor)
		q.cursor++
		if context.funcExpr == nil {
			continue
		}
		if q.seen == nil {
			q.seen = make(map[summary.SummaryKey]struct{})
		}
		if _, ok := q.seen[context.key]; ok {
			continue
		}
		q.seen[context.key] = struct{}{}
		return context, true
	}
	return keyedFunction{}, false
}
