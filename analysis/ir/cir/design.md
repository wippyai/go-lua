# cir — checker instruction IR (scaffold + prototype)

cir replaces the point-keyed half of the fact pipeline with a small, closed
instruction set attached per `cfg.Point`. Lowering translates syntax and
resolves bindings/types only; every value derivation (refinements, narrowing,
type conclusions) moves into the transfer interpreter, keyed on instruction
kind. cir does not replace the CFG — topology stays in `analysis/ir/cfg`.

## Instruction set

| Op | Operands | Meaning |
|----|----------|---------|
| `Noop` | — | structural point, no value effect |
| `Entry` / `Exit` | — | function boundary points |
| `Assign` | Dst, A | copy value A into path/temp Dst |
| `StaticMemberWrite` | Dst(path), A | `container.field = A` (member path in Dst) |
| `DynamicIndexWrite` | Dst(container), A(key), B(val) | `container[key] = val` |
| `MakeTable` | Dst, List(values), Type | table literal into Dst |
| `BinOp` | Dst, A, B, Operator | `Dst = A op B` |
| `UnOp` | Dst, A, Operator | `Dst = op A` |
| `Concat` | Dst, List | flattened n-ary `..` |
| `Call` | Results, Call{Callee\|Receiver+Method}, List(args), ListSpread, ResultSpread | direct/method call |
| `Return` | List(values), ListSpread | function return |
| `Branch` | Check (branchcond.Check), A(when Check=None) | edge selection; topology in CFG |
| `Iterate` | Results(vars), List(sources), Iter{numeric\|generic}, ListSpread | for-loop header |
| `Claim` | Dst, A, Claim{cast\|assert\|annotation\|asserts}, Type | value-fact assertion |
| `Select` | Dst, List(cases), SelectDefault | recognized channel select |
| `Logical` | Dst, A, B, Operator{and\|or} | short-circuit and/or (value form) |
| `Closure` | Dst, Func(proto), List(captures) | function literal + upvalue capture |

18 opcodes. The type sublanguage costs zero instructions: `TypeDefStmt` /
`InterfaceDefStmt` never enter the CFG; type exprs resolve at bind time.
`branchcond.Check` (closed 14-kind descriptor) is reused verbatim as the Branch
operand — no re-derivation.

## Operand encoding decision

Operands are scalar handles, never pointers: `Operand{Kind, Ref uint32}`.

- Paths, consts, and type refs are interned per `Body` into dense 1-based pools
  (`InternPath`/`InternConst`/`InternType`); index 0 is a reserved "none"
  sentinel, so a zero ref is unambiguously absent.
- Path identity keys on `path.PathKey` (structural, version-sensitive). The
  canonical structural `keyspace.Key` is the intended production key; the
  prototype keys on `path.Key()` strings for simplicity (see open questions).
- Variadic operands (call args, return values, table entries, iterator sources,
  n-ary concat, results) live in one shared `operandPool` referenced by
  `OperandRange{Start,Len}` — no per-instruction slices.
- Temps (`OperandTemp`) are body-local dense ids for expression results; varargs
  are their own operand kind (`OperandVararg`).

Instructions are flat value structs in `Body.instrs`; iteration allocates
nothing. Const literals keep the raw numeric source spelling (no lossy float
round-trip).

## Multi-value / multret encoding (the hard design point)

Lua calls and varargs produce a dynamic number of values. Losing that arity is
unacceptable for both consumers. cir encodes it with two orthogonal, explicit
markers rather than flattening:

- `ResultSpread` (on `Call`): the call's produced result count is open (multret,
  not truncated to `Results.Len`). `Results` still names the explicitly bound
  head destinations.
- `ListSpread` (on `Call` args / `Return` / `Iterate`): the final `List` operand
  expands to all runtime values it produces. Preceding operands are exact static
  arity; only the tail is open.

Worked example — `local a, b = f(); return g(h())`:

```
a, b = call f()              ; fixed arity 2, no spread
%1 = call h() multret        ; h() in tail arg position -> open results
%0 = call g(%1...) multret   ; g gets h's open tail; g in return tail -> open results
return %0...                 ; return forwards g's open tail
```

Static arity is preserved everywhere it is known; the single open tail is marked
exactly once at both producer (ResultSpread) and consumer (ListSpread), so no
information is lost.

Alternatives considered and rejected:
1. Flatten multret into a fixed count at lowering — loses arity, unsound for
   codegen and for callee-summary result counting.
2. Separate `ProjectResult i` instructions (`t=call; a=proj t 0`) — more
   instructions, and still needs an open-tail marker for the unbounded case.
3. Tuple-typed values — pushes arity into the value domain; heavier, and the
   transfer/codegen split wants it structural.

Chosen: explicit dual markers (above). **Flagged for Codex review** — this is the
encoding most expensive to change later.

## Consumer 2: bytecode/JIT backend (codegen)

cir + CFG, annotated with solved checker facts (types, ShapeIDs, placement,
nilability) and judgments (JIR), is the input to a future arena-VM bytecode
backend that emits specialized bytecode with judgment-proven guard elimination.
Encoding impact, per decision:

- **Scalar interned operands** map directly to VM registers/slots; dense 1-based
  refs are a natural slot-allocation domain. No symbolic path strings at use
  sites — only in the printer.
- **Multret markers** give codegen exact `CALL`/`RETURN` operand counts where
  static, and the `C=0` (multret) form where `ResultSpread`/`ListSpread` is set,
  without re-deriving arity. (Contrast go-lua-arena `compiler/bytecode` +
  `value.Proto`, which cir must not lose parity with; cir does not mirror its
  layout.)
- **Explicit receiver binding** in `Call` (`Receiver` + `Method`, never a folded
  member-call blob) decomposes to a `SELF`-style op directly.
- **Const literals keep raw spelling** so the backend chooses int vs float
  encoding.
- **Guard-elimination sites** (where JIR judgments authorize dropping a runtime
  guard): `StaticMemberWrite`/`MakeTable` field access, `DynamicIndexWrite` bounds,
  `Call` callee/arg-type guards, and `Claim` (cast/assert narrowings). These
  instructions are where a proven judgment lets codegen emit the unchecked op.

## Extended dialect decisions (Stage 4)

Full-coverage lowering resolves three design points the prototype deferred. Each
translates syntax and resolves bindings only; every value conclusion stays in
transfer.

### Short-circuit `and` / `or` — purity split (`OpLogical` + branch topology)

Locked by decision **D3**. `and` / `or` lowering is chosen by the conservative
purity of the short-circuited right operand, because the right operand is only
conditionally evaluated and its *effects* must not materialize when the guard
short-circuits (`x and f()` must not run `f` on falsy `x`). Purity classification:

- **Conservatively pure** = literals and plain identifier reads only. A member
  read (`t.f`) is impure — `__index` can be a metamethod with arbitrary effects.
  Index reads, calls, nested logicals, and any compound expression are impure.
- **Pure right operand** keeps the single value form `OpLogical{Dst, A, B, op}`:
  eager evaluation of a pure `B` is observationally free, so no branch is needed
  and the closed instruction set stays small. The short-circuit *result*
  selection plus the guard narrowing (truthy/falsy `A`) are still derived by
  transfer, not concluded in lowering.
- **Impure right operand** lowers to branch topology: the bypass edge assigns
  `Dst = A`; the taken edge evaluates `B` then assigns `Dst = B`; the two edges
  merge at the CFG join. `and` takes the RHS edge when `A` is truthy, `or` when
  `A` is falsy. This models effect gating correctly with no new instruction —
  the branch is ordinary CFG topology and the two assigns are ordinary `OpAssign`.

- **Consumer-2 codegen impact.** `OpLogical` expands to a TEST + conditional
  jump; the branch-topology form is already the TEST + jump in the CFG, so a
  transfer-proven guard (`A` known truthy/falsy) folds either form to one side.

Under same-CFG lowering (D1a) the impure branch topology is **not synthesized by
cirlower** — cfgbuild already materializes it. `cfgbuild.appendShortCircuitValueCalls`
emits the guard branch, the RHS-eval point, and the join, and records them in
`cfgfacts.Metadata` (`ShortCircuitGuard(point) -> {Stmt, Condition}`,
`ExpressionEvaluation(point) -> {Stmt, Expr}`). cirlower maps `OpBranch` onto the
guard point and the RHS assigns onto the eval point; the pure case has no eval
point and carries `OpLogical` on the enclosing statement point.

### Closures / function definitions — `OpClosure` + nested protos

`FunctionExpr` (anonymous, `local function`, and `FuncDefStmt` sugar including
methods) lowers to `OpClosure{Dst, Func, List(captures)}`. The nested function is
lowered as its own `FuncProto` (a child `Body` + `CFG`) exactly like a top-level
function; the parent references it by `FuncRef`. Captures come from
`bind.DirectCaptures` in first-use order and are emitted as path operands (the
upvalue identities). `FuncDefStmt` desugars to a closure written to its resolved
name path: a bare name binds directly; a member/method target (`a.b`, `a:m`)
emits the closure into a temp then a `StaticMemberWrite`.

- **Consumer-2 codegen impact.** `Func` is the proto/prototype index the backend
  emits a `CLOSURE`-style op against; `List(captures)` is the exact upvalue slot
  vector (scalar path refs → VM slots) with no free-variable re-analysis. Method
  definitions reuse the `SELF`-style receiver decomposition already used by
  `Call`.

### Channel `select` — recognition in lowering (`OpSelect`)

`channel.select { ch:case_receive(), default = ... }` is recognized structurally
during lowering (via `channelruntime.IsSelectCall` / `IsReceiveCaseCall`, read
only) and emitted as `OpSelect{Dst, List(channel paths), SelectDefault}`. A
recognized select shape is *syntax*, so recognition belongs in lowering per the
boundary rule; the payload/refinement of the selected value is left to transfer.
A call not matching the shape falls through to an ordinary `OpCall`.

- **Consumer-2 codegen impact.** `List` is the ordered channel-operand vector for
  a `SELECT` opcode; `SelectDefault` picks the blocking vs non-blocking form. No
  re-recognition at codegen — the shape is settled at lowering.

## Coverage harness (`CIR_SHADOW`)

`shadow_test.go` (`package cir_test`, gated on `CIR_SHADOW=1`) runs both the
point-keyed semantics extraction and cir lowering over every `testdata/fixtures`
`main.lua` and compares per-category construct coverage by operand identity
(path `Key()`), since the two pipelines build independent CFGs. It proves
lowering *completeness* (full point-state equality is a migration concern). Last
run: 574/574 fixtures, TOTAL 99.13% — assign 99.97%, call 99.53%, branch 91.15%,
return 100%. Residuals: the branch gap is the `OpLogical` short-circuit decision
above; a handful of call misses are computed/first-class callees keyed as
`callexpr`; one assign miss is a computed assignment target with no static path.

## Locked decisions (Stage 4, journal #1392)

All six design-round questions are resolved. D-labels reference the locked
journal decision.

- **D1 — same-CFG lowering.** cir attaches to the same `cfg.Graph` `body.Run`
  solves. cirlower's independent CFG build is replaced by consuming the graph
  cfgbuild already produced; the state-equality oracle is per-point on that
  shared graph. See "Same-CFG point mapping" below.
- **D2 — operand vs state-cell identity.** cir operands stay source path refs
  (the `PathKey` pool). Transfer-time address resolution runs through a cached
  `AddressResolver` keyed `(point, operand, mode)` producing `keyspace.Key` state
  keys. Operand identity is deliberately not state-cell identity. Interface +
  fake live in `resolver.go`; the production binding to `visibility.Resolver` is
  factapply's, not cir's.
- **D3 — logical purity split.** See the short-circuit section above.
- **D5 — resolved type refs.** `Type` interns the resolved `typ.Type` by
  `typ.EqualityHash`; the display spelling is `t.String()`, kept only for
  printing. cirlower resolves type expressions through `typeresolve.Resolver`,
  the same path the engine uses. The `<type>`/`<lit>` syntactic fallbacks are
  deleted; an unresolved type expression interns to the none ref. ShapeID stays
  deferred to codegen consumer work.
- **Multret/vararg** and **select recognition** stay as the prototype resolved
  them (dual spread markers; `OpSelect` at lowering).

## Same-CFG point mapping (D1a)

cfgbuild is the point authority. Its point granularity differs from the
prototype's one-point-per-statement model, so cirlower maps constructs onto the
pre-existing points rather than allocating its own:

- **Per-call points.** cfgbuild emits one `NodeCall` point per call in Lua
  evaluation order (`appendValueExprCalls`) before the owning statement's own
  point. cir lowers one `OpCall` (into temps) per call and places it on the
  matching call point.
- **Per-target points.** each assignment target and each `local` symbol gets its
  own `NodeAssign` point. Multret result binding therefore splits: `OpCall` into
  head temps at the call point, then one `OpAssign temp -> target` per target
  point. The prototype's folded `Results` window on `OpCall` is a self-CFG-only
  encoding.
- **Loop headers split.** numeric-for is `NodeAssign` (loop-var preheader) +
  `NodeBranch`; generic-for is `NodeBranch` + one `NodeAssign` per loop var.
  `OpIterate` maps to the branch point; loop-var binding maps to the assign
  point(s).
- **Joins carry no instruction.** `NodeJoin` points print as `noop`; cir attaches
  nothing to them.
- **Construct discovery.** cirlower reads `cfgbuild.Result.StmtPoints`
  (stmt -> points, in creation order) plus `cfgfacts.Metadata`
  (`Assignment`/`Loop`/`NumericFor`/`GenericFor`/`ShortCircuitGuard`/
  `ExpressionEvaluation`/`Label`/`Goto`). Together these expose every construct's
  point identity; **no cfgbuild change is required.** D1a is a large but fully
  in-lane rewrite, not a cfgbuild handoff.

## Operand address resolution (D2)

`resolver.go` defines `AddressResolver.Resolve(point, op, mode) -> (keyspace.Key,
bool)` with the four access modes `ReadBefore` / `WriteLocal` / `RootOrVisible` /
`Evidence`. Resolution is a pure function of `(point, op, mode)` for a fixed
Body; `CachingResolver` memoizes it. The real implementation (factapply, later
step) closes over the Body to decode an operand's interned path and delegates to
`visibility.Resolver`; cir ships only the contract and a test fake.

## Skeleton step 1 status

Landed in cir/cirlower lanes: **D2** (resolver interface + caching + fake),
**D5** (resolved TypeRef). **D1a** same-CFG rewrite and **D3** effectful-logical
branch topology are specified above and remain the next slice; the prototype
self-CFG `Chunk` entry stays as the transition path until `Lower(chunk, bindings,
cfgResult)` replaces it and the goldens migrate to the shared-graph form.

## Lowering scope (Stage 4 — full dialect)

Covered: local/ordinary assignment (incl. static member + dynamic index writes,
multret tail expansion across multiple targets), if/elseif/else, numeric +
generic for, while, repeat/until, break, goto/label topology, return, direct +
method calls, table constructors (array + hash + trailing spread), binary /
unary / n-ary concat, short-circuit and/or (`OpLogical`), closures and function
definitions (`OpClosure` + nested protos, methods), channel-select (`OpSelect`),
cast / non-nil assert / annotation claims, varargs, multret (call-in-middle vs
tail). Golden tests in `lower/lower_test.go` (incl. adversarial multret);
completeness measured by the `CIR_SHADOW` harness (99.13% over 574 fixtures).

Residual gaps (explicit): short-circuit and/or is a value op, not branch topology
(≈9% branch-category divergence — intentional, see Stage 4 decision); a small
number of computed/first-class callees and computed assignment targets carry no
static path and key as `callexpr`/`target`. cir is consumed by nothing yet.
