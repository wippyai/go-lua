# Real-body semantic program acceptance

This POC compiles the immutable `operationplan` rows plus body-owned generic
iteration, expression-source dependencies, and observation consumers into one
syntax-free program. Payloads remain in their existing immutable owners and the
program stores only `(store, key)` references.

The acceptance fixture is `compiler.validate_graph` from
`regression/deadlock-compiler-lua` (368 CFG points). Its current program has 57
generic-for declarations, 172 expression dependency references, and 469
observation declarations. Without a registered body extension transaction,
the program deliberately fails closed on one missing executable family:

`body.generic-for-variable`

The generic-for check declarations are non-executable sidecars. The extracted
canonical generic-for transaction now satisfies that family without retaining
the AST. All current factflow node/edge transactions, source dependencies, and
observation consumers are represented.

The admitted program is exercised over all seven recorded prepass, summary,
and materialization entries. Concrete-program WTO is compared to canonical
generic WTO at every point and across each of the 17 State lanes before
publication; normalized summary and output surfaces are then materialized and
queried.

On the local Ryzen 9 7950X3D (`-benchtime=5x`), compiling the 368-point program
is about 96 us / 233 KB / 339 allocations. A direct base-entry solve is about
37.1 ms / 9.49 MB / 72.1k allocations through current WTO versus 35.3 ms /
9.87 MB / 70.2k allocations through the concrete semantic program. This is a
real but modest ~5% CPU reduction, not the interprocedural phase-collapse win.
