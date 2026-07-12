# Returned-allocation freshness audit

This POC pins a defect in the current summary boundary; it does not change
production semantics.

A table literal is keyed by its callee CFG allocation site.  The summary-backed
call provider currently copies that identity and its heap object unchanged to
every caller.  Consequently two distinct static calls to the same function are
treated as returning the same runtime object.  Applying the second outcome can
also replace heap facts written through the first result.

Concrete inline semantics require a two-level identity:

```
(callee allocation site, caller static call site)
```

Distinct call sites then receive distinct abstract objects.  Repeated execution
of one call site deliberately reuses its abstract identity, but its object must
be weakly joined so the finite allocation-site abstraction converges without
discarding facts from an earlier iteration.

The smallest production fix seam is summary outcome instantiation in
`program/internal/callresult`: before `outcomeFromSummary`, build one deterministic
call-site allocation substitution and apply it consistently to return values,
the complete reachable heap-object graph (keys, roots, and member values), and
placement keys.  Outcome application must distinguish fresh first materialization
from repeated materialization of the same caller allocation site; the latter is
a heap-object join, not an unconditional `WriteHeapTableObject` replacement.

