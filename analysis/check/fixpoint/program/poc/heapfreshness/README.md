# Returned-allocation freshness audit

This regression pins the production summary-boundary allocation semantics.

A table literal is keyed by its callee CFG allocation site. The summary-backed
call provider records callee-local returned allocations as templates and
instantiates them at the caller's static call site.

Concrete inline semantics require a two-level identity:

```
(callee allocation site, caller static call site)
```

Distinct call sites then receive distinct abstract objects.  Repeated execution
of one call site deliberately reuses its abstract identity, but its object must
be weakly joined so the finite allocation-site abstraction converges without
discarding facts from an earlier iteration.

The production seam is summary outcome instantiation in
`program/internal/callresult`: before `outcomeFromSummary`, one deterministic
call-site substitution is applied to return values, the complete reachable
heap-object graph (keys, roots, and member values), and identity-bearing return
facts. Outcome application weakly joins repeated materialization of the same
caller allocation site.
