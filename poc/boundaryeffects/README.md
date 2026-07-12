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
solver over 128 caller bindings, 4096 value pairs, and each of the 17 State
lanes independently. This validates the admitted slice, not general function
summarization. In particular, heap and correlated variant semantics require
lane-specific symbolic adapters before this mechanism can cover real bodies.

Run:

```sh
go test ./poc/boundaryeffects
go test ./poc/boundaryeffects -run '^$' -bench . -benchmem -count 5
```
