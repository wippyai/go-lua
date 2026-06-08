# Semantic Model Refactor

The type engine should be organized around normalized semantic programs, not
around whichever path, AST node, or fact row a caller happens to hold.

## Review Gate

New helper layers are acceptable only when they are part of one of the canonical
semantic programs below and delete duplicated active surfaces. A change that
only routes an old helper through a new helper is not progress.

Every migration slice must answer:

- Which semantic program owns this operation?
- Where is source syntax lowered exactly once?
- Which old path/address/call reconstruction APIs are deleted or made private?
- What stable query key would a salsa cache use?
- Which fixture or unit law proves the old behavior is preserved?

## Canonical Programs

### Read Relation Program

Owns backward reads over provenance, alias, append-field, and key-presence
relations.

The core object is an address-native relation index over point state:

`target address -> source address + relation kind + suffix + replay policy`

Consumers ask semantic questions such as "what sources can prove this path?" or
"what append-field sources reach this element field?" They must not reopen
`ValueOriginFacts`, `PathAliasFacts`, or key-presence rows independently.

Deletion targets include carrier-local route APIs, identity alias projections,
append-origin source/destination traversals, and map-write readback queues that
reconstruct alias closure manually.

### Write Transaction Program

Owns the full lifecycle of a write:

`source intent -> normalized access footprint -> pre-state evidence selection -> invalidation -> product/static/reference effects -> derived proof replay`

Transfer recognizes source syntax and emits write transactions. Flow executes the
reduced-product law. Consumers must not manually lower paths, invalidate rows,
and replay proofs in separate call-site code.

Deletion targets include `Apply*PathTransaction` wrappers, boundary append-key
plans, transfer-local alias replay walks, and split dynamic-index proof entry
points that each redo part of the write lifecycle.

### Call Edge Program

Owns one call boundary as a stable semantic edge.

The call edge exposes selected targets, call outcomes, expected argument types,
callback refs, callback expected signatures, entry context, receiver/self facts,
return values, references, relations, effects, and param narrows from one
projection keyed by the call edge identity.

Transfer, observation, nested inference, callback environment, and parameter
evidence consume that projection instead of realigning AST calls, `cfg.CallInfo`,
bindings, expected-argument arrays, product frames, and summary readers.

Deletion targets include duplicate expression-to-path lowering in call target and
type resolvers, nested callback expected-function reconstruction, and
observation call-return special reads from driver state.

## Substrate Boundaries

`types/typ`, `types/domain/value/product`, and `types/constraint` are substrate
layers. They should not absorb checker orchestration. If a fix wants to encode
call, write, or provenance policy there, it is probably compensating for a
missing semantic program in `types/flow` or `compiler/check`.

## Caching Rule

A semantic program must have an obvious finite cache key:

- read relation: point-state identity, target address, relation mask, direction,
  and replay policy
- write transaction: write kind, normalized access footprint, pre-state epoch,
  and policy bits
- call edge: graph/call identity, point-state identity, selected target context,
  and expected type context

If the key depends on ad hoc AST traversal or string-form path reconstruction in
multiple consumers, the abstraction is not canonical enough.
