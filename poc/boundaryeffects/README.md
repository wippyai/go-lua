# Guarded boundary-effect proof

This isolated POC reduces the same acyclic diamond used by
`poc/functiontransformer` to two guarded outcomes and a joined tail. Execution
does not walk the CFG and does not call `factapply` kernels. It applies compact
meet, bulk path kill/gen, assignment-equality, and user-lattice propagation
effects directly, while retaining every point-input observation.

The admission boundary is intentionally narrow and exact:

- structural path copies and ordinary equality/inequality guards are admitted;
- caller roots use fixed-size packed bindings (no maps and zero allocations);
- finite pre-existing branch aliases, heap objects, variant-origin values, and
  non-fresh output roots fail closed because their effects depend on input
  state and cannot be represented by this fixed delta;
- unsupported syntax/fact families are outside this POC rather than silently
  approximated.

The randomized differential checks every CFG point and exit against the current
solver over 128 caller bindings and 4096 value pairs. A second differential
runs both the production solver and this POC separately under each of the 17
single-lane domains, including the production entry reachability and
`NormalizeForDomain` rules. This validates the admitted slice, not general
function summarization. In particular, heap and correlated variant semantics
require lane-specific symbolic adapters before this mechanism can cover real
bodies.

There are three result surfaces:

- `Execute` builds the legacy all-point result map;
- `ExecuteExit` computes only the function boundary used by summary solving;
- `ExecuteObserved` writes only requested point snapshots to caller-owned,
  fixed-size storage. The observation plan and storage add no allocations.

The path refinement plus canonical-key writes within each assignment already
use one `PathEvidenceEdit` transaction. Equality-proof publication,
typestate canonicalization, and user-lattice propagation deliberately remain in
production order and cannot be folded into that edit through the current public
State transaction without changing semantics.

Representative Ryzen 9 7950X3D results:

| path | ns/op | B/op | allocs/op | speedup |
|---|---:|---:|---:|---:|
| current body solve | 58,400 | 46,024 | 472 | 1.0x |
| all-point compatibility map | 11,400 | 6,216 | 33 | 5.1x |
| boundary-only exit | 10,620 | 3,112 | 24 | 5.5x |
| sparse join + exit | 10,680 | 3,112 | 24 | 5.5x |
| packed root binding | 32 | 0 | 0 | — |

The first implementation eagerly inserted evolving States into a result map;
that forced persistent representations to escape and measured only 2.8x.
Fixed observation storage removes that artifact. The admitted summary path does
clear 4x; semantic State edits, rather than observation selection, now dominate.

Run:

```sh
go test ./poc/boundaryeffects
go test ./poc/boundaryeffects -run '^$' -bench . -benchmem -count 5
```
