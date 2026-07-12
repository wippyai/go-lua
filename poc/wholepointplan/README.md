# Whole-point semantic plan POC

This POC compiles every `cfg.Point`-keyed `factflow.FactsInput` field into one
immutable, ordered row. A reflection exhaustiveness test makes a newly-added
point fact fail until it receives an explicit classification. Randomized tests
then prove that mixed whole-point rows contain every fact occurrence exactly
once and preserve the current applicator order.

Execution deliberately delegates to `factapply.NewFactsNodeTransfer` and
`NewFactsEdgeTransfer` over the same immutable snapshot. This gives exact State
parity with the production oracle while establishing a single compilation seam;
it does **not** claim an independent symbolic interpreter yet.

## Fail-closed boundary

Generic/third-party transfer extensions have no operation ABI in this stage.
Any such extension rejects the complete compile and returns a zero Plan, so no
partial row can execute. Expression-keyed maps are source dependencies rather
than point-local operations and remain inside the immutable `Facts` snapshot.

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
