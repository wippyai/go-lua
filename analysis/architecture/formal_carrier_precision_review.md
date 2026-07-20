# Formal carrier precision review

Status: **blocking verdict**, 2026-07-18. This is a read-only design review of
the proposed doubled-vocabulary registered-product carrier. It makes no
production change.

## Verdict

Do **not** make `ValueFactor[FormalSlot]` plus the existing registered State
factors the whole function-summary carrier. That carrier is a product of
independent abstract states, not an abstraction of an input/output relation.
It cannot represent the identity function for arbitrary callers, and it also
cannot retain guarded output correlation or a store whose value is copied from
an input. Building or wiring that schema would be a sound but materially lossy
summary path.

The doubled `IN`/`MID`/`OUT` vocabulary, full-width `formal.Root`, formal
identity terms, exact `Meet`, projection, and registered substitution laws are
all useful and should remain. They are the address and residual-fact algebra
for composition. They are not, by themselves, the missing functional
dependency algebra.

The minimal lawful carrier is a guarded **symbolic transformer** over that
formal address vocabulary:

1. scalar and value-bearing writes retain the existing `ValueTerm` DAG;
2. path-bearing writes retain the existing `PathTerm` DAG;
3. identity-bearing residual facts use the existing canonical
   `identity.Term` and registered substitution;
4. guarded alternatives retain the existing `Guard`/ROBDD partition; and
5. address-only residual must-facts may use the registered formal product
   directly.

`relationCode` is the sole instruction IR. Its existing `ValueTerm`,
`PathTerm`, `Guard`, `EffectTerm`, outcome, choice, sequence, and lexical-mu
nodes must be interpreted into the transformer lattice by the frozen WTO.
There must be no new expression IR and no concrete-`State` transducer. At a
call, composition substitutes caller terms/roots into the already-stabilized
callee transformer; only final root application evaluates `ValueTerm`s to
`product.Value` and materializes registered State factors.

## Why the proposed product is not relational enough

The generic Values factor is exactly a finite map from an address to an
ordinary `product.Value` (`state/value_lane_factor.go:11-27`). Its lifted map
operations compare, join, meet, widen, and narrow each map value independently
(`state/value_lane_factor.go:28-90`). Changing the key from concrete
`key.Value` to `FormalSlot` prevents namespace collisions, but it does not add
a predicate relating two slots.

The apparently relational State lanes do not fill this gap:

- `StoreRelation` means that one path is stored into another for ownership
  evidence (`state/store_relation.go:9-15`). It is not equality of arbitrary
  values.
- `RelConstraint` is a restricted affine numeric/length relation over path
  operands (`state/diff_relation_lane.go:10-40`). It cannot express equality of
  the complete 17-axis `product.Value`, object identity, functions, strings,
  typestate, presence, or user lattices.
- Path evidence has a finite ground congruence for path addresses, but Values
  explicitly declares path-equality quotient participation **independent**
  (`state/lane_spec_values.go:7-19`). Therefore a proof that two paths are
  equal is not a universal scalar copy constraint, and it does not turn two
  independent Values coordinates into the graph of the identity function.

This is the standard distinction between a Cartesian product abstraction and
an input/output relational abstraction. Boutonnet and Halbwachs define a
procedure summary over pairs of initial and current states, with relational
composition and existential elimination of locals; their worked identity
constraints explicitly keep initial and current variables equal. They also
show that a disjunction/partition is needed when a single relation cannot keep
the desired cases separately. See [Disjunctive Relational Abstract
Interpretation, §§3.2-4.2](https://www-verimag.imag.fr/~halbwach/vmcai2019.pdf).
Cousot and Cousot classify this approach as symbolic relational separate
analysis in [Compositional Separate Modular Static Analysis of Programs by
Abstract Interpretation](https://www.di.ens.fr/~cousot/COUSOTpapers/SSGRR-01-PC-RC.shtml).

## Mandatory witness 1: identity

Consider:

```lua
local function f(p)
    return p
end
```

The concrete relation is the graph `OUT.ret0 = IN.p`. Let two call sites pass
distinct abstract values `A` and `B` (for example two different exact object
identities, or exact string and number literals).

A caller-independent plain product summary has only independent bindings such
as:

```text
Values[IN.p]   = A join B       -- or Top
Values[OUT.r0] = A join B       -- or Top
```

Meeting the summary with `IN.p = A` changes only the `IN.p` map entry.
Existentially projecting `IN` cannot manufacture `OUT.r0 = A`; `OUT.r0`
remains `A join B`. A summary specialized to `A` would answer the first caller
but would not be reusable for `B`. Thus the proposed product must either replay
the body per caller or lose the exact copy.

The existing syntax already carries the exact answer. `valueRoot` is a
first-class `ValueTerm` (`transformer/terms.go:52-80,135-160`), and a return
operation already stores a `ValueTerm` (`transformer/relation.go:23-29`). The
summary should retain `OUT.r0 := Root(IN.p)`. Applying it with `A` evaluates to
`A`; applying the same frozen term with `B` evaluates to `B`. No callee body
cell or equation is created.

This is not merely a Lua-specific corner. A relational abstraction of a set of
functions must be able to preserve equal outputs for related inputs; a plain
product loses those relationships. See [A Relational Abstraction for
Functions, especially Example 10 and §5](https://research.cs.wisc.edu/wpis/papers/sas05.pdf).

## Mandatory witness 2: guarded correlated return

Consider:

```lua
local function f(p)
    if p then
        return p
    end
    return 0
end
```

The required summary is not merely
`OUT.r0 = IN.p join 0`. It is:

```text
Truthy(IN.p) => OUT.r0 = IN.p
Falsy(IN.p)  => OUT.r0 = 0
```

The distinction matters when a caller has already refined `p`, and when return
facts from the same branch (presence, typestate, obligations, heap effects, or
multiple tuple results) must remain correlated. A single product leaf joins
the alternatives and irreversibly forgets the implication.

Again, the one existing syntax already has the required semantics:
`SelectValue` retains the condition and both arms and evaluates only the live
arm when the guard is decided (`transformer/terms.go:537-557`). The
factor-native guarded evaluator maps `valueRoot`, constants, and `valueSelect`
without a concrete-State evaluator (`transformer/guarded_value_decision.go:
130-195`). The formal carrier must retain this `Guard`/decision structure;
it must not flatten it into one registered-product terminal before Apply.

The ROBDD is a representation of guarded alternatives, not a second semantic
IR. Its variables are canonical `Guard` atoms from the same `ValueTerm` arena,
and its terminals are transformer components. No semantic node may be invented
only by the ROBDD layer.

## Mandatory witness 3: heap alias and copied effect

Consider:

```lua
local function f(p, v)
    p.x = v
    return p
end

-- q may alias p
local r = f(p, v)
use(q.x, r.x)
```

An exact reusable summary needs all of the following at once:

```text
OUT.r0                    := Root(IN.p)
store Path(IN.p).x        := Root(IN.v)
heap receiver identity   := FormalIdentity(IN.p)
allocation/effect residue := registered formal factors
```

The formal identity and path-root work correctly make alias/effect addresses
substitutable. They do not supply the value copied by the store. A
path-evidence or heap factor whose payload is an ordinary `product.Value`
cannot store `Root(IN.v)` for all callers. Writing `Top` is sound but loses the
same information that concrete replay currently preserves.

The lawful representation is the already-existing `EffectTerm`/`PathTerm`/
`ValueTerm` write in `relationCode`, plus formal identity/path residue. During
composition, `IN.p` and `IN.v` are simultaneously substituted with caller
terms, and identity substitution is applied once through all registered
identity-bearing factors. During final materialization, the canonical effect
transaction updates the sole State product. If `q` aliases `p`, the existing
heap/path identity laws expose the write through `q`; no call-specific heap
implementation is needed.

## Minimal carrier contract

The smallest carrier that passes all three witnesses is:

```text
Transformer = ROBDD<TransformerLeaf>

TransformerLeaf =
    Reachability
  × OutputBindings<FormalSlot, ValueTerm>
  × OrderedEffects<EffectTerm>                 // ValueTerm/PathTerm payloads
  × RegisteredFormalResidualFactors            // address/identity facts only
  × CorrelatedOutcomeMetadata
```

This notation describes semantic roles, not permission to create five new
public object families. In code, the roles should be views over the existing
sealed `relationCode` arenas and outcome/effect tables:

- `relationCode` already contains `Sequence`, `Choice`, `LoopMu`, outcomes,
  `boundaryStepApply`, effect references, and value terms
  (`transformer/reduced_relation.go:19-102`).
- the arena already hash-conses `ValueTerm`, `PathTerm`, and `Guard` nodes
  (`transformer/terms.go:197-210`);
- `JoinValue` already provides the canonical symbolic join and `SelectValue`
  the correlated symbolic choice (`transformer/terms.go:513-557`); and
- the formal region inventory already freezes one caller-independent lexical
  cell universe and WTO from `relationCode`
  (`transformer/formal_relation_region_inventory.go:84-143`).

The new semantic operation is therefore not another parser, bytecode, or
effect vocabulary. It is a formal evaluator for the existing node kinds whose
cell values retain terms/effects instead of materializing concrete State.

### Composition

At `boundaryStepApply`, do not allocate a callee route. Compose the stabilized
callee transformer into the caller by structural substitution:

1. bind callee `IN` roots to caller `ValueTerm`/`PathTerm` arguments;
2. alpha-rename callee allocation templates and callee-local guard atoms;
3. substitute output bindings and effect terms simultaneously;
4. combine guarded alternatives in the shared decision kernel;
5. `Meet` only the registered formal residual factors;
6. eliminate callee `MID` and locals; and
7. retain a DAG reference to the canonical substituted terms rather than
   copying or evaluating them.

Only Apply count and cheap substitution/hash-cons lookup may scale with caller
count. Callee lexical cells, WTO evaluations, and effect normalization must not.

### Join, widening, and narrowing

- Symbolic scalar join is `JoinValue`, not `product.Join` on prematurely
  evaluated caller values.
- Guarded join is ROBDD union with leafwise transformer join.
- Registered residual factors use their current exact product laws.
- Widening applies at the frozen WTO heads. A lane that widens a value payload
  receives the symbolic term only at final evaluation; it must not invent a
  second symbolic axis implementation.
- Narrowing refines the same carrier after ascent equality. It does not replay
  a concrete body and does not publish into ascent.

If an operation cannot be represented by the existing `ValueTerm`/
`PathTerm`/`EffectTerm` vocabulary, compilation must fail as an incomplete
canonical operation and the vocabulary must be extended at its foundational
operation definition. Falling back to concrete State, storing `Top`, or adding
a call-special case is forbidden.

## Required correction to the reconciliation contract

The current reconciliation document says “A relation is the registered
product over physically disjoint `IN`, `MID`, and `OUT` boundary
vocabularies.” Read literally, that is false for the scalar and value-bearing
parts. It should say:

> A relation is a guarded symbolic transformer over disjoint formal boundary
> vocabularies. Its address/identity residual is the registered formal product;
> its value dependencies and value-bearing effects retain the canonical
> `ValueTerm`/`PathTerm`/`EffectTerm` DAG until final application.

The rest of that document's composition, one-WTO, atomic-cut, no-fallback, and
performance requirements remain valid.

## Stop conditions for implementation

Do not wire the formal schema unless focused tests prove, without callee replay:

1. one frozen identity transformer returns two distinct caller abstract values
   exactly;
2. a guard-refined caller selects the correlated return/effect row, while an
   ambiguous caller receives the lawful join;
3. a parameter-to-field store preserves the caller value and alias-visible
   heap effect; and
4. 1, 10, and 100 callers have identical callee cell/equation/evaluation counts.

Any implementation based only on `ValueFactor[FormalSlot]` with
`product.Value` payloads fails condition 1 by construction and must remain
unwired.
