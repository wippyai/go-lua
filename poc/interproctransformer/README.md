# Interprocedural guarded transformer POC

This isolated package proves the actual call-boundary substitution, not merely
a cheaper callee CFG replay. A nonrecursive equality callee is compiled once to
a guarded boundary. Each caller binding specializes that boundary to the
existing `summary.Summary`, lowers it to the existing `callpayload.CallOutcome`,
then applies it through the provider-independent
`ApplyResolvedCallOutcomeOrdinaryEffects` and
`ApplyResolvedCallOutcomeEdge` seams.

The differential oracle re-solves the callee in the caller's exact context,
projects the same Summary, and applies it to the same caller. It compares all
callee and caller requested points, both exits, and the Summary independently
under every one of the 17 State lane domains. Heap identities, placements,
recursion, and state-sensitive sidecars fail closed. `Lower` reflectively
rejects every nonempty Summary field outside its exact whitelist, including any
future field. After construction, specialization has an asserted zero callee
body-solve count.

On the local Ryzen 9 7950X3D, repeated resolution plus caller application is
195–196 us for exact-context body solving and 44.5–45.1 us for guarded
specialization/application: **4.33–4.40x**, with callee body solves reduced
from one per binding to zero. The 80 identity CFG points represent ordinary
syntax scaffolding in a modest function; they have no expensive callback and
compile away because they carry no boundary effect.

Run:

```sh
go test -race ./poc/interproctransformer
go test -run '^$' -bench BenchmarkRepeatedCallerBindings -benchmem -count=3 ./poc/interproctransformer
```
