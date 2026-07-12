# Real-body semantic program acceptance

This POC compiles the immutable `operationplan` rows plus body-owned generic
iteration, expression-source dependencies, and observation consumers into one
syntax-free program. Payloads remain in their existing immutable owners and the
program stores only `(store, key)` references.

The acceptance fixture is `compiler.validate_graph` from
`regression/deadlock-compiler-lua` (368 CFG points). Its current program has 57
generic-for declarations, 172 expression dependency references, and 469
observation declarations. The full program deliberately fails closed on one
missing executable family:

`body.generic-for-variable`

The generic-for check declarations are non-executable sidecars. All current
factflow node/edge transactions, source dependencies, and observation consumers
are represented. This is why the existing dense WTO executor cannot honestly
admit this pathological body yet: `body.prepare` rejects any body containing a
generic-for transfer, even though the factflow operation rows themselves are
complete.

On the local Ryzen 9 7950X3D, compiling and rejecting the 368-point program is
about 73 us / 233 KB / 339 allocations. A direct base-entry solve of the body is
about 39 ms / 9.6 MB / 74k allocations. These are seam measurements, not the
seven real interprocedural phase/context solves; the latter remain the actual
performance acceptance target once the generic-for transaction is extracted.

