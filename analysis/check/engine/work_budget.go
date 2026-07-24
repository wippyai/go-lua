package engine

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// defaultWorkBudget is the per-file evaluation budget in fact visits. It is
// the analysis bound: a program whose evaluation demands more work than this
// is published conservatively instead of being analysed further. The value is
// a fixed property of this engine build, so a given source always receives the
// same verdict on any machine under any load.
//
// It is calibrated against the corpus: the heaviest fixture, the deliberate
// stress program semantic/type-engine-edge-matrix, spends about 2.5M visits,
// so this limit leaves roughly 3x headroom above the heaviest program the
// checker is expected to analyse in full.
const defaultWorkBudget uint64 = 8_000_000

// workBudget bounds one file's evaluation in units the input alone
// determines. Every equation any VM executes -- the root acyclic artifact, the
// root cyclic fixpoint, and every nested lexical body reached through a child
// application or a recursive summary -- draws the size of the closed partition
// it consumes, because a kernel filters and clones that whole fact lane before
// it can read anything. Transaction count alone is not a work measure: a
// single transaction over a large closure costs orders of magnitude more than
// one over a small closure, so the closure size is what the fixpoint's cost
// actually tracks.
//
// The count is therefore the evaluation's own step counter rather than an
// observation of the host: it depends only on the program, the artifact the
// front lowered from it, and the fixpoint's iteration order.
//
// The budget belongs to exactly one file evaluation and is owned, like the rest
// of lexicalEvaluator, by that evaluation's single goroutine.
type workBudget struct {
	limit     uint64
	spent     uint64
	exhausted bool
}

func newWorkBudget(limit uint64) *workBudget { return &workBudget{limit: limit} }

// workBudgetError reports an exhausted deterministic budget. Both numbers it
// carries are counts rather than elapsed measurements, so the message a given
// source produces is the same on every machine.
type workBudgetError struct {
	Limit uint64
	Spent uint64
}

func (e *workBudgetError) Error() string {
	return fmt.Sprintf("engine: evaluation exceeded its work budget of %d fact visits after %d", e.Limit, e.Spent)
}

// charge draws units of work. Once the budget is exhausted it stays
// exhausted: a later caller that swallows this error cannot convert an
// unfinished evaluation into a published one, because overspent() still holds
// at the file boundary.
func (b *workBudget) charge(units uint64) error {
	if b == nil {
		return nil
	}
	if b.exhausted {
		return &workBudgetError{Limit: b.limit, Spent: b.spent}
	}
	b.spent += units
	if b.spent > b.limit {
		b.exhausted = true
		return &workBudgetError{Limit: b.limit, Spent: b.spent}
	}
	return nil
}

// overspent reports whether the budget was exhausted anywhere in the file's
// evaluation, including inside a nested body whose caller discarded the error.
func (b *workBudget) overspent() bool { return b != nil && b.exhausted }

func (b *workBudget) err() error {
	if !b.overspent() {
		return nil
	}
	return &workBudgetError{Limit: b.limit, Spent: b.spent}
}

// meterKernel charges the file budget for the partition a transaction is about
// to read, plus the transaction itself so an empty partition still costs
// something. Every acyclic and cyclic kernel binding in this engine is
// constructed through registry and cyclicRegistry, so metering there covers
// every VM at every lexical depth from a single accounting point.
func (l *lexicalEvaluator) meterKernel(kernel equation.Kernel) equation.Kernel {
	// A nil kernel stays nil so the registry keeps rejecting an unbound
	// contract rather than accepting a wrapper that panics on execution.
	if kernel == nil || l == nil || l.budget == nil {
		return kernel
	}
	budget := l.budget
	return equation.KernelFunc(func(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
		if err := budget.charge(uint64(partition.FactCount()) + 1); err != nil {
			return equation.TransactionResult{}, err
		}
		return kernel.Execute(operation, partition)
	})
}
