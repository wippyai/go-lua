# W39 seam-1 sealed-evidence ledger

## Fixed

- `realworld/trait-registry`
  - The expected indexed optional-access assignment diagnostic was already
    present, but the enclosing `TraitRegistryEntry` literal also emitted an
    unexpected unproven claim.
  - A nested literal can retain an earlier provisional member while its
    containing sealed table has the later direct descendant publication. The
    record comparison now reconciles that nested member only with descendants
    already sealed by the same table before checking the declared field.
  - This permits the imported `types.TRAIT_TYPE` publication to prove the
    `meta.type: string?` member. It neither creates a fact from a member name
    nor uses source-specific fixture knowledge; unresolved or malformed nested
    values remain unknown.

## Regression coverage

- `TestCheckProjectProvesImportedSealedNestedRecordAssignment` exercises the
  same imported static-member and nested-record boundary without fixture data.
- The fixture continues to assert its intended `spec.tools[1].id` optional
  access diagnostic unchanged.

## Scope

- `testdata/fixtures` unchanged.
- `__legacy` unchanged.
- No skip, manifest relaxation, or expectation changes.
