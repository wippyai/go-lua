# Prior analyzer reference

This tree contains the analyzer implementation removed by the canonical
Program flash cut on 2026-07-30.

It is inert reference material:

- no live package may import it;
- no test or gate may execute it;
- no compatibility adapter may bridge it to the canonical analyzer;
- failures in former consumers are migration-ledger entries, not reasons to
  restore code from this tree.

Semantics may be re-derived from its scenarios, but implementation is written
against the canonical `Program -> Rules -> Solver -> State -> Queries`
pipeline.
