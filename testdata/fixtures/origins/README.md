# origins — source-ordered witness-trace fixtures

These fixtures encode diagnostics that cite ORIGIN CHAINS: a value (nil) is born at
some line, survives one or more control-flow joins, and reaches a use where it causes
an error. The diagnostic is expected to render an ORIGIN-ORDERED evidence trace that
walks the causal chain from birth, through the joins where the value survived, to the
use site.

## Status

Refuted judgments render these fixtures as source-ordered witness traces without a
per-fixture renderer flag. The `run.skip = "diagnostic-only fixture"` setting remains:
the fixtures verify checker diagnostics rather than Lua runtime output.

Two halves of the oracle:

- The `diagnostics[]` blocks in each manifest are the MACHINE-CHECKABLE half: exact
  codes, spans, evidence entries (kind + trust), and `render_ordered_contains` trace
  lines that the renderer must reproduce in origin order.
- The narratives below are the HUMAN half: the born/survives-join/use sequence with
  line references, and the expected evidence entries.

Evidence vocabulary (matching the semantic evidence-chain schema):
- `kind`: `"abstract fact"` (inferred by the checker) or `"user assertion"` (from an
  annotation the user wrote).
- `trust`: `"proven"` (checker-derived fact) or `"claimed"` (asserted by user
  annotation, e.g. an optional type declaration).

---

## 1. nil-born-survives-join-reaches-use

Expected diagnostic code: `type.nil.unsafe_use` at `main.lua:10:12`.

`x` is declared `string?` and initialized nil (L6). It is assigned only on the `if flag`
arm (L8); there is no `else`, so after the join it may still be nil. The method call
`x:upper()` (L10) is a nil-unsafe use.

Source-ordered witness trace (born -> declaration -> survives join -> use):

1. proven: x born nil at main.lua:6 (else branch had no assignment)
2. claimed: x declared with optional type string?
3. proven: x survives the if/else join at main.lua:9 (no else assignment)
4. proven: x reaches use at main.lua:10 (method call on possibly-nil value)

Expected evidence entries:
- abstract fact / proven — birth at L6:11 ("x born nil ... else branch had no assignment")
- abstract fact / proven — survival at L9:5 ("x survives the if/else join at main.lua:9")
- user assertion / claimed — optional type at L6:14 ("x declared with optional type string?")

---

## 2. optional-field-origin

Expected diagnostic code: `type.nil.unsafe_use` at `main.lua:4:12`.

`Cfg.hook` is declared optional (`string?`) at the type definition (L1). `run` reads
`c.hook:len()` (L4) with no guard. The nil possibility is BORN at the field's optional
declaration and flows to the read.

Origin-ordered witness trace (optional declaration origin -> use):

1. claimed: field hook declared optional at main.lua:1 (type string?)
2. proven: c.hook reaches use at main.lua:4 (method call on possibly-nil field)

Expected evidence entries:
- user assertion / claimed — optional field declaration at L1:28 ("field hook declared optional ... type string?")
- abstract fact / proven — use at L4:12 ("c.hook reaches use ... method call on possibly-nil field")

---

## 3. nil-through-two-joins

Expected diagnostic code: `type.nil.unsafe_use` at `main.lua:17:12`.

`x` is declared `string?` and initialized nil (L10). A first `if p` may assign (L12),
then a second `if q` may assign (L15). Neither branch is exhaustive, so the nil survives
BOTH joins before the method call `x:upper()` (L17). The trace has two "survives join"
hops in origin order.

Source-ordered witness trace (born -> declaration -> survives join #1 -> survives join #2 -> use):

1. proven: x born nil at main.lua:10 (else branch had no assignment)
2. claimed: x declared with optional type string?
3. proven: x survives the if/else join at main.lua:13 (no else assignment)
4. proven: x survives the if/else join at main.lua:16 (no else assignment)
5. proven: x reaches use at main.lua:17 (method call on possibly-nil value)

Expected evidence entries:
- abstract fact / proven — birth at L10:11
- abstract fact / proven — survival at first join L13:5
- abstract fact / proven — survival at second join L16:5
- user assertion / claimed — optional type at L10:14

---

## 4. nil-through-three-joins

Expected diagnostic code: `type.nil.unsafe_use` at `main.lua:21:12`.

The nil born at L10 crosses three non-exhaustive joins at L13, L16, and L19
before reaching the method call at L21. This locks a trace with origins on more
than three distinct source lines, including the optional declaration at L10.
