# Guarded summary adapter POC

This POC proves the smallest reusable boundary for transformer execution:

1. bind caller parameter values into compact guarded rows;
2. retain every row whose guard may hold;
3. emit only the existing `summary.Summary` schema (`Returns`,
   `ParamObligations`, and normal-return `PathRefinements`,
   `PathInvalidations`, and `EffectDeltas`);
4. combine alternatives with `summary.Join` and canonicalize with
   `summary.NormalizeOwned`; and
5. pass the result through the production summary-to-call-outcome adapter and
   normal call fact application.

The package intentionally lives below `analysis/check/fixpoint/program`. The
production summary-to-outcome implementation is correctly internal to that
tree; placing this isolated POC here exercises the real adapter without adding
a public API solely for an experiment or duplicating its lowering rules.

The differential tests cover:

- return rows and an unconditional parameter obligation against a real Lua
  body solve;
- exact `Summary` lattice composition;
- exact `CallOutcome` lowering;
- exact post-call `State` for each of the 17 default state lanes independently;
  and
- atomic fallback when an obligation is conditional. `Summary` cannot encode
  a guarded entry requirement, so no partial result is published.

This is not production wiring and does not claim that arbitrary body semantics
have already been compiled into rows. It establishes that once a supported
transformer slice has produced these boundary rows, no new summary or state
domain is needed to consume them.
