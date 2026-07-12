# Whole-point semantic plan POC

This POC compiles every `cfg.Point`-keyed `factflow.FactsInput` field into one
immutable, ordered row. A reflection exhaustiveness test makes a newly-added
point fact fail until it receives an explicit classification. Randomized tests
then prove that mixed whole-point rows contain every fact occurrence exactly
once and preserve the current applicator barriers.

The canonical node barriers are N0 call/channel materialization, N1
no-normal-return, N2 implication publication/closure, N3 descendant and
postcondition/channel effects, N4 dynamic/root/path/static writes, N5 return,
and N6 covariant finalization. Edge barriers are E0 reachability, E1 guard
refinements, E2 implication closure, E3 scalar/presence/path relations, E4
evidence, and E5 call-edge effects. `Cursor` orders by this registry rather
than by Go declaration order.

Composite operations carry an exact barrier set. A call site owns N0 and E5;
a channel select owns N0 and N3. Auxiliary declarations such as fixed call
results and return-presence relations are marked as sidecars with an explicit
owner, so a future executor cannot apply them as independent transitions.

Execution deliberately delegates to `factapply.NewFactsNodeTransfer` and
`NewFactsEdgeTransfer` over the same immutable snapshot. This gives exact State
parity with the production oracle while establishing a single compilation seam;
it does **not** claim an independent symbolic interpreter yet.

## Fail-closed boundary

Generic/third-party transfer extensions have no operation ABI in this stage.
Any such extension rejects the complete compile and returns a zero Plan, so no
partial row can execute. Expression-keyed maps are source dependencies owned
by the relevant materialization/write transaction rather than point-local
operations; they remain inside the immutable `Facts` snapshot.

## Remaining gaps

- The concrete executor is still the whole production kernel, not per-operation
  kernels. The next stage must extract shared kernels without changing order,
  atomic rollback, lazy call reads, or cancellation behavior.
- Channel-select and call facts participate in multiple internal production
  stages. They occur once in the fact row; their internal staging remains in the
  concrete kernel until those kernels are made explicit.
- `BranchSufficientLiteralCases` is classified for completeness but currently
  has no direct `factapply` consumer.
- No symbolic State/summary execution, rebasing, guard composition, or lane
  adapters are claimed here.
