package product

import (
	"strconv"
	"testing"
	"time"

	"github.com/wippyai/go-lua/types/domain/value/axis/effectrows"
	"github.com/wippyai/go-lua/types/domain/value/axis/escape"
	"github.com/wippyai/go-lua/types/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/types/domain/value/axis/identityrecursion"
	"github.com/wippyai/go-lua/types/domain/value/axis/numeric"
	"github.com/wippyai/go-lua/types/domain/value/axis/ownership"
	"github.com/wippyai/go-lua/types/domain/value/axis/presence"
	"github.com/wippyai/go-lua/types/domain/value/axis/shapevalue"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// numericRangeValue isolates the numeric axis with the given interval; every
// other axis sits at its Top so a numeric-only chain stresses only that axis.
func numericRangeValue(lo, hi int64) AbstractValue {
	return New(
		shapevalue.Top(),
		presence.Top(),
		numeric.Range(lo, hi),
		effectrows.Top(),
		ownership.Top(),
		escape.Top(),
		identityrecursion.Top(),
		evidence.Top(),
	)
}

// accGrowth is one adversarial growth family that stresses a single infinite-
// height growth pattern. stages[i] is the i-th observation of a value the fixed-
// point solver re-derives with progressively larger structure. The successive
// stages are not required to be comparable: a deepening array (number[],
// number[][], ...) or a fresh recursive family is an antichain of observations,
// and ACC is exactly the guarantee that folding such an unbounded stream through
// Widen still reaches a fixed point.
type accGrowth struct {
	name   string
	stages []AbstractValue
}

// accAdversarialGrowth builds one growth family per infinite-height axis. Each
// isolates the pattern the locked design must bound with a true widening: deep
// self-embedding record nesting, growing record field-sets, growing union and
// literal member-sets, widening numeric intervals, deepening optional/presence,
// growing tuple arity, deepening array nesting, and distinct recursive families.
func accAdversarialGrowth() []accGrowth {
	var families []accGrowth

	staged := func(name string, build func(i int) typ.Type, n int) accGrowth {
		stages := make([]AbstractValue, 0, n)
		for i := 0; i < n; i++ {
			stages = append(stages, FromType(build(i)))
		}
		return accGrowth{name: name, stages: stages}
	}

	// Deep self-embedding record nesting: {v}, {v, next:{v}}, {v, next:{v, next:{v}}},
	// ... Each stage embeds the previous (same-layout) record one level deeper, the
	// canonical self-similar tower. A widening that folds the tower to a leaf-
	// dropping mu (one that covers nothing) under-approximates; one that keeps every
	// depth distinct never stabilizes.
	families = append(families, staged("self_embedding_record", func(i int) typ.Type {
		t := typ.NewRecord().Field("v", typ.Number).Build()
		for j := 0; j < i; j++ {
			t = typ.NewRecord().Field("v", typ.Number).Field("next", t).Build()
		}
		return t
	}, 8))

	// Growing record field-set: {f0}, {f0,f1}, ... Each stage adds a field.
	families = append(families, staged("growing_record_fields", func(i int) typ.Type {
		b := typ.NewRecord()
		for j := 0; j <= i; j++ {
			b = b.Field("f"+strconv.Itoa(j), typ.Number)
		}
		return b.Build()
	}, 8))

	// Growing union member-set over distinct record variants: each stage adds one
	// variant. A widening that did not bound union growth would accumulate members.
	families = append(families, staged("growing_union_members", func(i int) typ.Type {
		members := make([]typ.Type, 0, i+2)
		for j := 0; j <= i+1; j++ {
			members = append(members, typ.NewRecord().Field("tag"+strconv.Itoa(j), typ.Number).Build())
		}
		return typ.NewUnion(members...)
	}, 8))

	// Sequence-wrapped self-embedding record union: string | (msg), where msg is
	// {text: string | tuple[1](msg) | ..., type: "text"} — the realistic claude-
	// message mapper accumulator. The recursive position re-introduces the message
	// record through a tuple[1] indirection (a content-array literal), and carries a
	// discriminant literal that must survive. Each stage adds one deeper member to
	// the text union, so the union grows heterogeneously while the family stays the
	// same. A widening that cannot fold a record-rooted recursion reached through a
	// sequence wrapper accumulates an unbounded {text:{text:...}} tower (this is the
	// loop-built heterogeneous union-array non-convergence). Widen must fold it to a
	// finite recursive product and absorb the finite unfoldings.
	msg := func(text typ.Type) typ.Type {
		return typ.NewTuple(typ.NewRecord().
			Field("text", text).
			Field("type", typ.LiteralString("text")).
			Build())
	}
	families = append(families, staged("seq_wrapped_self_embedding_record_union", func(i int) typ.Type {
		members := []typ.Type{typ.String}
		var inner typ.Type = typ.String
		for j := 0; j <= i; j++ {
			members = append(members, msg(inner))
			inner = typ.NewUnion(append([]typ.Type(nil), members...)...)
		}
		return typ.NewUnion(members...)
	}, 8))

	// Growing heterogeneous record-shape union over the same message family: each
	// stage adds one more content block variant discriminated on its type literal
	// ({type:"text",...}, {type:"tool_use",...}, ...) and nests one level deeper, so
	// the message record's content array element is itself a growing union of
	// differently-shaped blocks that can re-embed the message. This is the loop-built
	// heterogeneous union-array the mapper accumulates. The widening must bound both
	// the heterogeneous block member growth and the embedded depth into a finite
	// recursive product while keeping the message family coherent.
	families = append(families, staged("growing_heterogeneous_record_union", func(i int) typ.Type {
		blockTypes := []string{"text", "tool_use", "tool_result", "image"}
		message := func(content typ.Type) typ.Type {
			return typ.NewRecord().
				Field("role", typ.LiteralString("user")).
				Field("content", typ.NewArray(content)).
				Build()
		}
		blocks := make([]typ.Type, 0, i+2)
		blocks = append(blocks, typ.NewRecord().
			Field("type", typ.LiteralString(blockTypes[0])).
			Field("text", typ.String).
			Build())
		var msg typ.Type = message(typ.NewUnion(append([]typ.Type(nil), blocks...)...))
		for j := 1; j <= i; j++ {
			blocks = append(blocks, typ.NewRecord().
				Field("type", typ.LiteralString(blockTypes[j%len(blockTypes)])).
				Field("nested", msg).
				Build())
			msg = message(typ.NewUnion(append([]typ.Type(nil), blocks...)...))
		}
		return msg
	}, 8))

	// Growing literal set: "k0", {"k0","k1"}, ... Singleton literals stay exact
	// until the chain forces a widen toward the base primitive.
	families = append(families, staged("growing_literal_set", func(i int) typ.Type {
		members := make([]typ.Type, 0, i+1)
		for j := 0; j <= i; j++ {
			members = append(members, typ.LiteralString("k"+strconv.Itoa(j)))
		}
		if len(members) == 1 {
			return members[0]
		}
		return typ.NewUnion(members...)
	}, 8))

	// Widening numeric intervals fanning outward: [0,0], [-1,1], [-2,2], ... The
	// interval widening must release a moving bound to infinity.
	families = append(families, accGrowth{
		name: "widening_numeric_interval",
		stages: func() []AbstractValue {
			out := make([]AbstractValue, 0, 8)
			for i := int64(0); i < 8; i++ {
				out = append(out, numericRangeValue(-i, i))
			}
			return out
		}(),
	})

	// Deepening optional/presence: T, T?, (T?)?, ... Nilability is a presence-axis
	// join, so the chain must collapse onto the four-point presence lattice rather
	// than wrap an ever-deeper optional.
	families = append(families, staged("deepening_optional", func(i int) typ.Type {
		var t typ.Type = typ.String
		for j := 0; j < i; j++ {
			t = typ.NewOptional(t)
		}
		return t
	}, 8))

	// Growing tuple arity: (number,), (number, number), ...
	families = append(families, staged("growing_tuple_arity", func(i int) typ.Type {
		elems := make([]typ.Type, i+1)
		for j := range elems {
			elems[j] = typ.Number
		}
		return typ.NewTuple(elems...)
	}, 8))

	// Deepening array nesting: number[], number[][], ...
	families = append(families, staged("deepening_array_nesting", func(i int) typ.Type {
		var t typ.Type = typ.Number
		for j := 0; j <= i; j++ {
			t = typ.NewArray(t)
		}
		return t
	}, 8))

	// Distinct recursive families: each is a fresh family observation, so the
	// identity axis is forced from a concrete family to its upper bound (Top).
	families = append(families, accGrowth{
		name: "recursive_families",
		stages: []AbstractValue{
			FromType(muNext("Node")),
			FromType(muNextNamed("Named")),
			FromType(muMethodChain("Chain")),
			FromType(recursiveAliasNested("List", "Node")),
		},
	})

	return families
}

// TestDomain_ACCUnderWiden establishes, by an executable property over the
// widening operator rather than by inspection, that product.Domain satisfies
// ACC-under-Widen: folding any unbounded stream of growth observations through
// Widen reaches a fixed point in finitely many steps, and every accumulator over-
// approximates every observation it has absorbed. This is the termination
// guarantee the canonical fixed-point solver relies on at feedback-vertex cells.
//
// For each growth family the test drives the Cousot ascending chain the lattice
// contract documents (lattice.go Widen field):
//
//	s₀ = ⊥,  sᵢ₊₁ = Widen(sᵢ, Join(sᵢ, observationᵢ))
//
// cycling through the family's observations, and requires the chain to become
// stationary within the same bound the LawSuite ACC check uses. It then requires
// the stationary accumulator to cover every observation (the over-approximation
// soundness half). A widening that is not a true widening on any axis makes the
// chain never stabilize (caught here as a non-termination failure rather than a
// fixture hang); one that under-approximates fails the coverage check.
func TestDomain_ACCUnderWiden(t *testing.T) {
	withinTimeout(t, 120*time.Second, func() {
		const bound = 256
		for _, family := range accAdversarialGrowth() {
			if len(family.stages) == 0 {
				continue
			}
			cur := Bottom()
			stable := false
			var stableAt int
			for i := 0; i < bound; i++ {
				obs := family.stages[i%len(family.stages)]
				next := Widen(cur, Join(cur, obs))
				if Equal(next, cur) && i >= len(family.stages) {
					stable = true
					stableAt = i
					break
				}
				cur = next
			}
			if !stable {
				t.Errorf("[%s] Widen ascending chain did not stabilize within %d iterations (final=%s) — domain lacks a true widening on this axis",
					family.name, bound, cur.ProjectValue())
				continue
			}
			// The stationary accumulator must over-approximate every observation it
			// absorbed, on both the semantic coverage preorder and the join-induced
			// order. Covers is the soundness relation (γ(acc) ⊇ γ(obs)); LessOrEq is the
			// carrier order the solver iterates. A widening that under-approximates on
			// either fails here rather than silently losing a branch at runtime.
			for _, obs := range family.stages {
				if !Covers(cur, obs) {
					t.Errorf("[%s] stationary accumulator (stable at i=%d) does not Cover observation %s\n  accumulator=%s",
						family.name, stableAt, obs.ProjectValue(), cur.ProjectValue())
				}
				if !Domain.LessOrEq(obs, cur) {
					t.Errorf("[%s] stationary accumulator (stable at i=%d) does not over-approximate observation %s under the join order\n  accumulator=%s",
						family.name, stableAt, obs.ProjectValue(), cur.ProjectValue())
				}
			}
		}
	})
}

// TestDomain_ACCLawSuitePerAscendingChain reuses the lattice.LawSuite harness over
// the growth families whose stages form a comparable ascending sequence. For such
// a sample the join-semilattice laws hold, so Run exercises the full law set —
// crucially the harness's own checkWideningChainTerminates and
// checkWideningOverApproximates — against a genuine ascending chain on each of
// those infinite-height axes, with no parallel checker.
//
// Growth families whose successive observations are an antichain (deepening
// arrays, growing tuple arity, fresh recursive families, self-similar record
// towers) are exercised by TestDomain_ACCUnderWiden, which folds them through the
// documented Cousot chain; they are excluded here only because feeding an
// antichain to the join-associativity law tests an order-independence property of
// Join over incomparable elements, which is orthogonal to ACC.
func TestDomain_ACCLawSuitePerAscendingChain(t *testing.T) {
	ascending := map[string]bool{
		"growing_union_members":     true,
		"growing_literal_set":       true,
		"widening_numeric_interval": true,
		"deepening_optional":        true,
	}
	withinTimeout(t, 120*time.Second, func() {
		for _, family := range accAdversarialGrowth() {
			if !ascending[family.name] {
				continue
			}
			// Guard: the stages this suite covers must really be an ascending chain.
			for i := 0; i+1 < len(family.stages); i++ {
				if !Domain.LessOrEq(family.stages[i], family.stages[i+1]) {
					t.Fatalf("[%s] stage %d ⊑ stage %d expected to hold for the LawSuite ascending-chain run", family.name, i, i+1)
				}
			}
			sample := append([]AbstractValue{Bottom(), Top()}, family.stages...)
			suite := lattice.LawSuite[AbstractValue]{
				Name:   "product.AbstractValue/ACC/" + family.name,
				Domain: Domain,
				Sample: sample,
				Format: func(v AbstractValue) string { return v.ProjectValue().String() },
			}
			suite.Run(t)
		}
	})
}

// TestDomain_WidenOverApproximatesAcrossFamilies pins the soundness half of the
// widening operator over the full cross-product of every adversarial observation,
// including across distinct growth families: prev ⊑ Widen(prev,next),
// next ⊑ Widen(prev,next), and Join(a,b) ⊑ Widen(a,b). A widening that under-
// approximates would let the fixed-point solver converge below the collecting
// semantics. This isolates the over-approximation law from the join-associativity
// law (which the antichain families do not satisfy and which ACC does not need).
func TestDomain_WidenOverApproximatesAcrossFamilies(t *testing.T) {
	withinTimeout(t, 120*time.Second, func() {
		all := []AbstractValue{Bottom(), Top()}
		for _, family := range accAdversarialGrowth() {
			all = append(all, family.stages...)
		}
		for i, a := range all {
			for j, b := range all {
				w := Widen(a, b)
				if !Domain.LessOrEq(a, w) {
					t.Errorf("prev ⊑ Widen fails: sample[%d]=%s ⊑ Widen(.,sample[%d]=%s)=%s",
						i, a.ProjectValue(), j, b.ProjectValue(), w.ProjectValue())
				}
				if !Domain.LessOrEq(b, w) {
					t.Errorf("next ⊑ Widen fails: sample[%d]=%s ⊑ Widen(sample[%d]=%s,.)=%s",
						j, b.ProjectValue(), i, a.ProjectValue(), w.ProjectValue())
				}
				if !Domain.LessOrEq(Join(a, b), w) {
					t.Errorf("Join ⊑ Widen fails: Join(sample[%d],sample[%d])=%s ⊑ Widen=%s",
						i, j, Join(a, b).ProjectValue(), w.ProjectValue())
				}
			}
		}
	})
}
