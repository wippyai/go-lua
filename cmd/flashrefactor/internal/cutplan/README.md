# cutplan

`cutplan` is the reviewed input language for one atomic ownership cut. It is
not a script and has no compatibility, adapter, or command vocabulary.

The entire authored surface is:

```text
Intent(schema, name, operations)
Operation(id, after, authority, edits, bindings, imports, footprint, verify)
```

`authority` states the sole old and new semantic owner. `edits` is the only
authored mutation sum:

- `relocate`: exact source file, target `{path, package}` clause, and one
  resolved `{from, to}` declaration mapping per subject;
- `retire`: exact source file and resolved `SymbolRef`s; or
- `generate`: exact declared inputs, destination, and a symbolic provider key.

The executor owns an immutable provider registry. Generic `cutplan` only
accepts a safe symbolic key, never a command or shell fragment; preflight must
reject a key absent from that executor registry.

`SymbolRef` has one unambiguous grammar, so a package object cannot be confused
with a named-type member:

```text
import/path#package:Name
import/path#type:Receiver/field:Name
import/path#type:Receiver/method:Name
```

`bindings` and `imports` are exact per consumer. A binding contains resolved
from/to objects and only direct field/view receiver steps; an import contains a
consumer, exact old/new import endpoints (`path`, declared package `name`, and
local `alias`), and the resolved objects using it. `name` is the imported
package clause; `alias` is the exact explicit Go spelling and is empty for an
ordinary unaliased import. The effective identifier is `alias` when present,
otherwise `name`; dot and blank aliases are rejected. Package name is never
inferred from an import-path basename.
`nil` old/new import endpoints mean add/remove, respectively; this is the
actual import relation, not a `no_*` marker.

`footprint.read` is exactly what preflight fingerprints. `footprint.write` is
exactly what dry-run/apply hashes or deletes. Every write outside `read` must be
absent before mutation. No output path, closure list, or inferred file set is
authored elsewhere.

`verify` contains only exact named laws and registered structural gates. A law
has an ID, package, and one top-level `Test...` name. Residue paths are derived
from relocation/retirement sources and resolver sites, not authored. The
executor runs laws through the repository bounded runner; no intent field can
carry a shell command.

```json
{
  "schema": 3,
  "name": "link-flow-cut",
  "operations": [{
    "id": "link-flow",
    "authority": {"from": "link", "to": "flow"},
    "edits": [{
      "kind": "relocate",
      "relocate": {
        "source": "program/link/link.go",
        "destination": {"path": "program/link/flow/flow.go", "package": "flow"},
        "subjects": [
          {
            "from": {"object": "github.com/wippyai/go-lua/program/link#type:Link/field:computationScalars"},
            "to": {"object": "github.com/wippyai/go-lua/program/link/flow#package:Scalars"}
          }
        ],
        "containment": {
          "parent": {"object": "github.com/wippyai/go-lua/program/link#package:Link"},
          "child": {"object": "github.com/wippyai/go-lua/program/link/flow#package:Flow"},
          "through": {"object": "github.com/wippyai/go-lua/program/link#type:Link/field:flow"}
        }
      }
    }],
    "bindings": [{
      "consumer": "program/link/link.go",
      "from": {"object": "github.com/wippyai/go-lua/program/link#type:Link/field:computationScalars"},
      "to": {"object": "github.com/wippyai/go-lua/program/link/flow#package:Scalars"},
      "form": "field",
      "receiver": [{
        "kind": "field",
        "object": {"object": "github.com/wippyai/go-lua/program/link#type:Link/field:flow"}
      }]
    }],
    "imports": [{
      "consumer": "program/link/link.go",
      "from": {"path": "github.com/wippyai/go-lua/program/link", "name": "link", "alias": "link"},
      "to": {"path": "github.com/wippyai/go-lua/program/link/flow", "name": "flow", "alias": "flow"},
      "symbols": [{"object": "github.com/wippyai/go-lua/program/link/flow#package:Scalars"}]
    }],
    "footprint": {
      "read": ["program/link/link.go"],
      "write": ["program/link/link.go", "program/link/flow/flow.go"]
    },
    "verify": {
      "laws": [{"id": "link22", "package": "./program/link", "test": "TestPackTransferCompleteIncidenceDispositionReplayForeignArtifactLaws"}],
      "gates": ["diagnostics", "import-dag", "object-residue"]
    }
  }]
}
```

An intent is strict JSON and canonicalized before SHA-256 digesting. Operation
dependencies form a DAG; independent operations can share only reads. A
read/write overlap requires an explicit dependency and an authority chain. A
write/write overlap is always rejected: it must be one operation, not a hidden
second action. Resolver definitions/references are classified `source` or
`target` in `Lock`: subject sources must resolve in the declared source file,
and subject targets must resolve in the declared target file and package. A
pre-cut source identity therefore cannot stand in for a staged target
declaration. Byte fingerprints, provider registry identities, hazards, output
hashes, and the exact diff are generated only in `Lock`.

`ResolutionRequirements(intent)` is the deterministic generated resolver
denominator. It supplies every object’s source/target class and any forced
declaration path/package; both the semantic resolver and lock validator use
this same projection rather than re-deriving classification. For containment,
the parent and inserted `through` field are forced to the relocation source;
the child container is forced to the relocation destination and package. Any
competing forced location rejects the intent.

`ReferenceRouteRequirements(intent)` derives one generated route per
relocation subject. A `Lock` records every source declaration/use and its one
post-cut target declaration/use; both source and target site sets must be
complete and globally unique. `GateRequirements(intent)` similarly derives the
set-union of requested structural gates, and the lock records one normalized
result digest per gate. Neither evidence type is an authored action language.

The lock also binds the helper build and hash, Go version and executable hash,
structured resolver identity, and normalized build-environment and module-graph
digests. A replay must recreate these generated commitments; it cannot replay
under a merely similar toolchain.

The command layer builds a `Lock`, verifies it before mutation, and verifies
its exact output and diff after dry run/apply. Any changed input, unknown JSON
field, unresolved object, blocking hazard, undeclared file, partial output, or
unbounded test rejects the whole cut.
