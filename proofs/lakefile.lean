import Lake
open Lake DSL

-- Lean 4 mechanization pilot for the two smallest-ranked obligations in
-- analysis/architecture/soundness_obligations.md: the placement lane lattice
-- laws (A1, instantiated for the placement axis) and the depth-exhaustion
-- polarity lemma (B4 / invariants.md Rule 1). No external dependencies
-- (no Mathlib): both proofs are finite/structural and go through with core
-- Lean tactics only, keeping the build fast and self-contained.
package «GoLuaProofs» where

@[default_target]
lean_lib «GoLua» where
  globs := #[.submodules `GoLua]
