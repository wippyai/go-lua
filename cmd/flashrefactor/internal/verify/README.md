# Structural verifier

`verify` is the pure post-cut structural boundary. The executor supplies two
complete source snapshots and the flattened reviewed `cutplan.Import` routes.
It parses those exact bytes, verifies the exact import-spec delta separately
for every consumer file, then projects and checks the resulting package import
graph for acyclicity. There is no caller-supplied aggregate graph authority.

A successful report has a deterministic digest, a complete post-source digest,
and one digest-bearing evidence entry for each requested gate. Semantic gates
retain their resolver evidence digest; structural gates retain the post-source
digest they examined.

Diagnostics and resolved-object residue are typed successful evidence from
`semantic`; this package neither loads files nor invokes a resolver.  Every
requested `cutplan.Gate` has exactly one disposition, and a disposition for an
unrequested gate is rejected.  This prevents structural verification from
quietly becoming a second semantic authority.
