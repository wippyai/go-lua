# Dense direct WTO proof

This isolated package executes one representative reducible Lua body directly
over dense point arrays. It reuses `operationplan`'s canonical semantic barrier
catalog and the production `factapply` point transactions; it does not create a
generic equation map or recursively solve a callee body.

The fixture contains a guarded loop, root and member-path writes, implication
closure, and a call hole resolved through the current `CallOutcomeProvider`.
Tests compare every CFG point, exit, and normalized summary under the full
State product and each of its 17 lanes, through widening/narrowing and two
dependency revisions.

This is deliberately not a general WTO compiler. `Compile` admits exactly one
declared natural-loop partition and fails closed when any CFG point is outside
it. Heap/effect semantics stay in the shared production transaction until an
independent provenance-complete direct handler exists.

On the representative 58-point body (Ryzen 9 7950X3D, `-count=5`) the direct
executor is stable at about 243 us/op, 106.6 KB, and 432 allocations versus the
current transfer WTO at about 609 us/op, 213.5 KB, and 1,052 allocations: 2.5x
faster, 50% fewer allocated bytes, and 2.4x fewer allocations. The decisive
optimization is semantic-plan-certified fusion of unique-predecessor identity
rows, while retaining every point snapshot for diagnostics.
