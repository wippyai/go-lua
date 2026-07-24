# W41 narrowing7 ledger

## Fixed

- `narrowing/else-branch-wrong-type`

## Publication change

The declaration-owned uncalled-child boundary now admits an annotation
assignment only when its source is a static member path of a fully declared
formal. The member already has an existing declaration witness, so evaluating
the closed branch may publish its ordinary `type.assignment` fact. Publication
continues to reject arbitrary locals, branch-only refinements, calls, dynamic
indexing, channel selection, mutable/captured boundaries, and all diagnostic
families except the established assignment fact.

`TestCheckPublishesUncalledDeclaredBranchAssignmentContract` pins the
discriminated-union else-edge diagnostic and its source span.

## Guardrails

- No file under `testdata/fixtures` changed.
- No `__legacy` file changed or supplied implementation authority.
- The full-oracle diff is one removed failure and zero added failures.
