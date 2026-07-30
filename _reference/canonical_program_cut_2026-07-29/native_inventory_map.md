# Lane NAT native-family inventory (N0)

Baseline oracle verdict MD5: `31b85ae829bb93a7815c2282b60e2910`

Scope: the 48 families asserted by the 173 native fixtures, including ordinary
closure families that `Result.Native` serializes unchanged. “Source” names the
current producer and, after the arrow, the fixpoint publication that already
states the same semantic information. “Gap” means the information is owned by
lowering but is not yet carried through an equation/kernel publication.

| Family | Current computation / scan | Fixpoint source or required publication | N0 class |
|---|---|---|---|
| `alias_disjoint` | `native_alias.go`: two body walks identify fresh table/closure allocations, aliases, and later invalidators | Allocation identity/rekey, escape, call, member-write, and epoch rows already state identity and invalidation; add an allocation-kernel disjointness row so the guarded allocation coordinate owns the conclusion | gap |
| `branch-proof` | Already emitted by `branchKernel`; native only serializes it | `branch-proof/<body>/<occurrence>/<edge>` | sourced |
| `branch_partition` | `native_branch.go` walks `OpBranch`, folds constants/types, queries CFG; `native.go` also translates branch-proof rows | `branch-proof`, current `value`, declared type, residue-window, and recurrence/guard rows; publish the dynamic/always partition in `branchKernel` | sourced |
| `builtin_call` | `native_numeric.go` walks calls, resolves implicit globals, checks argument/result shape | Apply/call-result facts plus published global binding and result value; add the native descriptor to the apply/call-result kernel at that guarded call coordinate | gap |
| `call-argument` | Ordinary apply-kernel publication | `call-argument/<occurrence>/<position>` | sourced |
| `call-result` | Ordinary call-results-kernel publication | `call-result/...` | sourced |
| `call_scc` | AST `native_contracts.go` call graph/SCC recognizer | No equivalent partition row. WIR call targets and closure identities must lower an SCC descriptor to a guarded publication equation | gap |
| `callee_set` | AST `native_contracts.go` binding/call recognizer | Call target, member identity, write epochs, import boundary, and sealed-member inventory exist; publish completeness in the apply kernel, with unknown as absence | gap |
| `capture_epoch_root` | `native_structural.go` walks closure operands and scans the body for first capture use | Allocation-template capture inventory plus environment-write/call/epoch rows; allocation/object-materialization kernel needs a capture-root publication anchored at the guarded closure coordinate | gap |
| `capture_transport` | AST `native_operations.go` and `native_structural.go` independently recognize captured table initialization/mutation | Allocation member inventory, capture inventory, element values, writes, and epochs; publish transport from object materialization after those inputs are visible | gap |
| `closure` | Ordinary allocation/object-materialization value publication | `value`, `heap/table-identity`, and closure allocation rows | sourced |
| `concat_site` | `native_numeric.go` walks concat instructions and locally classifies every operand | Expression-kernel operand values/types already decide primitive dispatch and formatting; publish the site descriptor from the guarded expression kernel | gap |
| `constant_value` | Post-solve `publishedConstantValues` plus `native_numeric.go` duplicate WIR constant folding | Exact scalar `value/<term>/<occurrence>` from the equation constant lattice; expression/write kernels can publish the descriptor at the same guarded coordinate | sourced |
| `discriminant_select` | WIR `native_wir_contracts.go` scans branch comparisons and closed literal variants | Branch operands, branch proofs, declared discriminant type, and variant facts; branch kernel needs an exhaustive/select descriptor publication | gap |
| `divisor_property` | `native_numeric.go` locally folds divisor constants | Expression operand `value` and numeric relation/residue facts | sourced |
| `effect_row` | AST `native_operations.go` recognizes calls/selects/coroutine operations | Apply/select publications, resolved callee identity, callback/escape rows, and control outcomes; publish the row in apply/select kernels | gap |
| `epoch` | Ordinary environment-write/path-replacement/index-mutation publication | `epoch/...` | sourced |
| `eval_node` | Root rows from `evalNodeKernel`; `native_frozen.go` replays nested artifacts | `eval_node` is already a kernel publication for evaluated bodies; child admission must carry nested rows through the same kernel rather than frozen post-projection | sourced |
| `explicit-any` | Ordinary claim/expression publication | `explicit-any/...` | sourced |
| `function_entry` | AST `native_contracts.go` scans function syntax/result/vararg shape | Entry operands and declared return schema are already lowering-owned; entry kernel needs a function-entry descriptor publication for each admitted body | gap |
| `heap` | Ordinary allocation/member/index/meta kernels; `native.go` serializes | `heap/...` families through factkey | sourced |
| `host_global_binding` | `native_structural.go` walks calls and then searches projected value rows | Apply/call-result value plus resolved implicit-global identity; publish managed binding from call-results when the result is proven | gap |
| `interproc_summary` | `native_summary.go` walks nested WIR declarations/captures/types | Child entry declared-return schema, type parameters, and capture mutability must be published by child entry/summary kernel | gap |
| `list_construction` | `native_table.go` scans constructors, capacities, spreads, duplicate operands | Allocation-template member inventory and allocation entries state contents; constructor capacity/spread/duplicate metadata needs an allocation-kernel publication | gap |
| `metatable_seal` | `native_metatable.go` makes two module-wide WIR passes over construction, writes, calls, and reads | Heap identity/member/meta-attached/meta-identity and call rows already carry the topology; publish absent/installed seals from heap/meta kernels after guarded facts reconverge | sourced |
| `nilability` | `native_nilability.go` performs four scans plus CFG reachability over checks, writes, calls, loops | Current value/type, branch-proof/guards, epochs, escape/call rows, and recurrence joins already carry the full proof; publish point-local nilability from branch/write/call kernels | sourced |
| `numeric_branch` | `native_numeric.go` scans comparisons and independently resolves/folds carriers | Branch operands, current value/type, branch proofs, and numeric relation facts | sourced |
| `numeric_loop_carrier` | `native_numeric.go` scans numeric loops then quadratically rescans the full body for carrier writes | Generic-for numeric induction, recurrence, current value/type, branch and write facts; publish carrier disposition from generic-for/recurrence kernel | sourced |
| `placement` | Ordinary placement facts serialized by native | `placement/...` | sourced |
| `publication_identity` | Post-solve `native_publication.go` linearly scans all source-spanned WIR instructions | Every admitted equation already owns stable body + coordinate + occurrence; add a mechanical publication from the executing kernel/equation coordinate | gap |
| `record_construction` | WIR `native_wir_contracts.go` scans complete table constructors | Allocation-template member inventory, identities, escape rows, and table type/shape; publish record construction from allocation/object-materialization kernel | gap |
| `record_entry_ownership` | WIR `native_wir_contracts.go` classifies constructor entries | Allocation member/member-identity inventory and escape/ownership rows; object-materialization kernel publication | gap |
| `recursive_type_identity` | WIR `native_wir_contracts.go` traverses declared recursive types | Declared shape/type graph is lowering input but not a value fact; entry/claim kernel needs a canonical recursive-identity publication | gap |
| `representation` | `native_numeric.go` scans literals/operators/joins and runs a duplicate constant folder | Exact scalar `value`, declared/runtime type, expression operator operands/results, and recurrence join facts | sourced |
| `runtime-type-proof` | Ordinary validation/claim publication | `runtime-type-proof/...` | sourced |
| `scalar_operator` | `native_numeric.go` scans numeric operators and duplicates result/overflow classification | Expression-kernel operand/result values and declared types | sourced |
| `sealed_table` | `native.go` reparses heap keys and performs reachability/member-set analysis after solve | Heap table identity/closed/member/member-identity/meta/index/freeze rows state the same graph; publish seal/frozen closure in the heap/freeze kernels or a factkey-driven guarded kernel | sourced |
| `send_safety` | AST `native_operations.go` recognizes sends and table/closure expressions | Channel-send outcome, escape/isolation/freeze/member graph, and closure capability facts; channel/apply kernel needs the native disposition publication | gap |
| `shape_identity` | WIR `native_wir_contracts.go` plus engine `native_shape_epoch.go` scan declared records, reads, and writes | Declared shape facts and heap shape/epoch/write/meta/call rows; publish module and receiver epochs from entry/claim/path-replacement kernels | gap |
| `shape_transition` | AST and WIR native contract recognizers both scan record writes | Heap member inventory, static replace/path replacement, and shape facts; path-replacement kernel publication | gap |
| `table_construction_bound` | `native_structural.go` scans constructors, loop membership, and reachability | Allocation coordinates plus CFG recurrence/guard facts; allocation kernel needs bounded occurrence metadata lowered with the constructor | gap |
| `table_element` | `native_element.go` has eleven scans (origins, reads, guards, bounds, escapes, stability, producers) and `native.go` also projects index-presence | Heap member/index-presence/length-floor/index relations, value/type, escape, call, epoch, and guard facts already state element class/presence; publish the consumer descriptor from dynamic-index/allocation kernels | sourced |
| `table_growth` | `native_table.go` scans writes, preallocation, loops, aliases, escapes | Allocation capacity/member inventory, index mutation, escape/alias, recurrence and length facts; publish growth disposition in allocation/index-mutation kernels | sourced |
| `table_length` | `native_table.go` scans length ops, dense writes, holes, metatable calls | Length-term, length-floor, heap index/member, meta, escape, and recurrence facts; publish length disposition in expression/index/meta kernels | sourced |
| `throw_template` | Claim kernel publishes root claim-assert template; `native_frozen.go` replays nested artifacts | Existing claim-assert kernel row; nested body admission must evaluate/publish it instead of post-projecting frozen equations | sourced |
| `truthiness_class` | `native_branch.go` walks branches and runs its own constant/type classifier | Current value/type plus branch proof/guard rows | sourced |
| `typed_producer` | `native_structural.go` scans every instruction destination and declared type | Claim/expression value, declared/runtime type and trust facts; publish runtime-relevant producer in the owning guarded kernel | sourced |
| `value` | Ordinary equation value publication | `value/...` and related value families | sourced |

## Remainder summary

- Existing fixpoint source: **25 families**.
- Missing kernel publication: **23 families**.
- Irreducible scan-based families: **0**. The gaps are lowering metadata or
  descriptors over already-published facts; none requires a post-fixpoint WIR
  analysis.
- Baseline engine-native scan sites counted syntactically: **52** (`Len()` body
  walks, including the nested quadratic rescan; helper range walks over
  artifacts/contracts are not counted).
- Post-solve unguarded publishers to remove: native contracts, publication
  identities, and constant values.

The ordered implementation seam is: (1) project the 25 sourced families only
from closure facts; (2) make each of the 23 gaps an equation draft and a
guard-preserving kernel publication; (3) serialize the resulting visible
closure using factkey declarations, with no WIR access in `Result.Native`.

## N3 AST/WIR corpus divergence

The two lists are not redundant. A descriptor multiset diff across all 177 Lua
files in `testdata/fixtures/native` found **526 AST-only rows** and **184
WIR-only rows**:

- AST-only: `function_entry` (155), `effect_row` (212), `callee_set` (102),
  `call_scc` (2), `capture_transport` (2), legacy `shape_identity` (43), and
  legacy `shape_transition` (10).
- WIR-only: `record_construction` (70), `record_entry_ownership` (14),
  `recursive_type_identity` (6), `discriminant_select` (3), canonical
  `shape_identity` (88), and canonical `shape_transition` (3).

The exact spellings differ materially, not just by ordering: AST shape rows
omit canonical `shape_id`; WIR record rows carry subjects, ownership and escape
revocations absent from the AST stream. Therefore deleting the AST stream now
would remove five independently asserted families and 526 corpus publications.
The required unification work is to lower the five AST-only semantic families
from WIR/kernel inputs, then delete `native_contracts.go` and
`native_operations.go`; the legacy AST shape families should be dropped once
their fixture expectations are moved to the canonical WIR shape rows.

## Sound stopping-point status

This patch completes the first publication seam, not the whole lane:

- `publication_identity` is now a lowering-owned, in-fixpoint publication.
  Its post-solve engine WIR scan and `native_publication.go` are deleted.
- Root-body `constant_value` rows now read the equation partition's exact
  `value/<term>/...` lattice result in `publicationKernel`; the kernel does no
  arithmetic folding. Nested lexical bodies are not evaluated into the root
  partition today, so their byte-contract rows remain in the single marked
  residual `publishedNestedConstantValues`. This is one residual subset of one
  otherwise sourced family, not a new family.
- All front `NativeContract` rows now travel through one ordinary publication
  equation and closure join. The old post-solve native-contract injection is
  gone. The batching is transport only: the AST/WIR semantic divergence above
  remains and N3 therefore cannot delete the AST recognizers yet.
- Generic projection parsing and branch-proof parsing route through
  `factkey.Project` and `factkey.ParseBranchProof`. The two new coordinate
  families (`constant_value` and `publication_identity`) are complete
  `factkey.Family` records and their producers use `BuildKey`; family-specific
  native key decoders remain, so N4 is partial.
- Engine-native syntactic WIR scans are **50** at this checkpoint, down from
  **52**. Eight production native files still import WIR/CFG, so the N5 source
  guard is not yet satisfied.

Checkpoint counts:

- N0 family classification: **25 sourced / 23 gaps / 0 irreducible**.
- Closed gap in this patch: **1 family** (`publication_identity`).
- Marked residual: **1 family subset** (nested `constant_value`); the root
  subset is partition-sourced.
- Remaining N0 gaps: **22 families**, plus the unresolved AST/WIR unification.
