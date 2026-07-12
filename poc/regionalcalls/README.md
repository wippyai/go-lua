# Exact regional call-contribution replacement

This isolated POC exercises the production retained WTO transaction at the
reuse boundary that survived the canonical-cell census.

When a callee summary changes, `pointSummaryDependencyTracker` identifies the
CFG equations that read it. Regional recomputation must:

1. seed those changed equation owners;
2. close the region over dynamic reads, emissions, and WTO/SCC influence;
3. retract every invalidated owner's old output bag;
4. reset affected cells from preserved outside-region initial/contributions;
5. run the region with the replacement transfer binding;
6. expand or fall back if a new edge escapes the retained plan;
7. publish and commit the complete generation transactionally.

Retraction is the essential difference from resume. If a call contribution
changes from unknown/missing to known, the local result can decrease. Joining a
new known value into the old unknown value remains unknown forever. Replacing
the old owner's contribution and recomputing its downstream cone produces the
same result as a clean solve.

The tests include that former precision hole and randomized cyclic graphs with
arbitrary increasing or decreasing transfer replacements. Every regional result
is differential-checked against a clean production WTO solve. No checker path
imports this package.

Run:

```sh
go test -race ./poc/regionalcalls
go vet ./poc/regionalcalls
```
