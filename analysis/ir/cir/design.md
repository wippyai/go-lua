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
| `Select` | Dst, List(cases) | recognized channel select |

16 opcodes. The type sublanguage costs zero instructions: `TypeDefStmt` /
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

## Open questions for the Codex design round

1. **Operand interning strategy.** Adopt `keyspace.Key` as the canonical path ref
   (structural, no string hashing, already the state-key currency) vs the
   prototype's `path.PathKey` strings. `keyspace` ids are per-KeySpace and must
   not be serialized — does a `Body` carry its `KeySpace`, or does interning run
   against a shared per-function space? Const/type interning likewise.
2. **Multret/vararg encoding** (above) — confirm dual-marker over projection
   instructions; confirm it satisfies both summary result-counting and codegen.
3. **Select recognition-then-lowering.** Channel select is a structural pattern
   over `Call` today. Does lowering run structural recognition and emit `Select`
   directly, or emit `Call` and let a recognition pass rewrite to `Select`? The
   boundary rule says lowering may recognize syntax but not conclude values — a
   recognized select shape is syntax, so it belongs in lowering, but it needs the
   channel-case sub-call operands modeled (not yet prototyped).
4. **Migration seam vs factflow.** How does the transfer interpreter consume cir
   in parallel with the existing `factflow.Facts` path during construct-by-
   construct migration (shadow diff, byte-identical oracle)? Which lane is the
   source of truth per construct, and where is the switch?
5. **Type ref identity.** Prototype stores a syntactic spelling. Production should
   carry the resolved `typ.Type` / ShapeID ref (attached post-bind), keyed for
   both transfer and codegen. Where is that resolution attached?
6. **Logical `and`/`or` short-circuit.** Not modeled yet (lowered opaque). Needs
   a control-flow lowering (two edges + a value merge) or a dedicated value form.

## Prototype scope

Covered: local/ordinary assignment (incl. static member + dynamic index writes),
if/elseif/else, numeric + generic for, while, return, direct + method calls,
table literals, binary/unary/concat, cast, non-nil assert, annotation claims,
varargs, multret. Golden tests in `lower/lower_test.go`. Out of scope: short-
circuit value lowering, full channel-select operands, function-literal values,
goto/label topology beyond fall-through. cir is consumed by nothing yet.
