# W29 ledger

## Scope

`analysis/check/engine/engine.go` only. `__legacy` and frozen fixture data were
not modified.

## Closed facts attempted

- A child-entry wire lane carries a declared gradual-any boundary separately
  from its concrete caller seed.
- A boundary lookup follows only published entry, write, and branch facts.
- Branch handling republishes the fact at the consumer path rather than
  deriving a type from syntax.

## Outcome

The implementation builds and passes vet and Stage 1, but the full oracle
remains `495/673`; in particular,
`regression/gradual-field-incomplete-guard-rejected`,
`regression/non-cast-call-leaves-argument-gradual`, and
`modules/active-session-any-time-sub-soundness` still fail. No claim of a
count rise or zero-regression set difference is made.
