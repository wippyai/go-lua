package solver

import (
	"maps"
	"math/bits"

	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// linearBackend is a partial linear-arithmetic Solver over the rational numbers.
// It collects the affine constraints it understands (Le, SumLe, GeConst, LeConst)
// as normalized rows sum(coeff[var]*var) <= bound, then decides a single Le goal
// by Fourier-Motzkin elimination: it negates the goal as an integer-strict row
// and reports decision.Valid only when the combined system derives a numeric
// contradiction. It is a sound but incomplete procedure and never returns
// decision.Invalid: when in doubt, including any int64 overflow or a size cap,
// it returns decision.Unknown.
type linearBackend struct {
	rows []linearRow
}

// linearRow is sum(coeffs[v]*v) <= bound, with int64 coefficients keyed by
// variable. A row with no variables and a negative bound is a contradiction.
type linearRow struct {
	coeffs map[pathdom.PathKey]int64
	bound  int64
}

const (
	maxLinearRows = 1024
	maxLinearVars = 32
)

// NewLinearBackend creates a fresh linear-arithmetic Solver backend.
func NewLinearBackend() Solver {
	return &linearBackend{}
}

func (b *linearBackend) Assert(c numeric.NumericConstraint) {
	switch v := c.(type) {
	case numeric.Le:
		b.addRow(map[pathdom.PathKey]int64{v.X: 1, v.Y: -1}, v.C)
	case numeric.SumLe:
		b.addRow(sumCoeffs(v.X, v.Y, v.Z), v.C)
	case numeric.GeConst:
		b.addRow(map[pathdom.PathKey]int64{v.X: -1}, -v.C)
	case numeric.LeConst:
		b.addRow(map[pathdom.PathKey]int64{v.X: 1}, v.C)
	}
}

// sumCoeffs builds the coefficient map for x + y - z, merging coefficients when
// operands coincide.
func sumCoeffs(x, y, z pathdom.PathKey) map[pathdom.PathKey]int64 {
	coeffs := make(map[pathdom.PathKey]int64, 3)
	coeffs[x] += 1
	coeffs[y] += 1
	coeffs[z] += -1
	return coeffs
}

func (b *linearBackend) addRow(coeffs map[pathdom.PathKey]int64, bound int64) {
	for v, c := range coeffs {
		if c == 0 {
			delete(coeffs, v)
		}
	}
	b.rows = append(b.rows, linearRow{coeffs: coeffs, bound: bound})
}

func (b *linearBackend) Entails(goal numeric.NumericConstraint) decision.Result {
	le, ok := goal.(numeric.Le)
	if !ok {
		return decision.Unknown
	}
	rows := make([]linearRow, 0, len(b.rows)+1)
	for _, r := range b.rows {
		rows = append(rows, r.clone())
	}
	// Negate the goal gX - gY <= gC as the integer-strict row gY - gX <= -(gC+1),
	// i.e. {gX:-1, gY:+1} <= -(gC+1). A contradiction with the asserted rows
	// proves the goal.
	negBound, ok := negStrictBound(le.C)
	if !ok {
		return decision.Unknown
	}
	rows = append(rows, linearRow{
		coeffs: map[pathdom.PathKey]int64{le.X: -1, le.Y: 1},
		bound:  negBound,
	})
	for v, c := range rows[len(rows)-1].coeffs {
		if c == 0 {
			delete(rows[len(rows)-1].coeffs, v)
		}
	}
	if refutable(rows) {
		return decision.Valid
	}
	return decision.Unknown
}

// negStrictBound returns -(c+1) with overflow check.
func negStrictBound(c int64) (int64, bool) {
	cPlus, ok := addInt64(c, 1)
	if !ok {
		return 0, false
	}
	return negInt64(cPlus)
}

// refutable runs Fourier-Motzkin elimination over rows and reports whether a
// numeric contradiction 0 <= negative is derivable. Any overflow or size-cap
// breach abandons the search and reports false, keeping the procedure sound.
func refutable(rows []linearRow) bool {
	for {
		if contradiction(rows) {
			return true
		}
		v, ok := pickVar(rows)
		if !ok {
			return false
		}
		next, ok := eliminate(rows, v)
		if !ok {
			return false
		}
		rows = next
		if len(rows) > maxLinearRows {
			return false
		}
	}
}

// pickVar returns a variable still present in some row, and false when none
// remain (the system is variable-free). It also enforces the distinct-variable
// cap.
func pickVar(rows []linearRow) (pathdom.PathKey, bool) {
	seen := make(map[pathdom.PathKey]struct{}, maxLinearVars+1)
	var pick pathdom.PathKey
	have := false
	for _, r := range rows {
		for v := range r.coeffs {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
			}
			if !have {
				pick = v
				have = true
			}
		}
	}
	if len(seen) > maxLinearVars {
		return "", false
	}
	return pick, have
}

// eliminate removes variable v by pairing each row with a positive v-coefficient
// against each row with a negative v-coefficient (Fourier-Motzkin). Rows without
// v pass through unchanged. It returns false on any int64 overflow.
func eliminate(rows []linearRow, v pathdom.PathKey) ([]linearRow, bool) {
	var pos, neg, rest []linearRow
	for _, r := range rows {
		switch {
		case r.coeffs[v] > 0:
			pos = append(pos, r)
		case r.coeffs[v] < 0:
			neg = append(neg, r)
		default:
			rest = append(rest, r)
		}
	}
	out := rest
	for _, p := range pos {
		for _, n := range neg {
			combined, ok := combine(p, n, v)
			if !ok {
				return nil, false
			}
			out = append(out, combined)
			if len(out) > maxLinearRows {
				return nil, false
			}
		}
	}
	return out, true
}

// combine eliminates v from a positive row p (coeff a>0) and a negative row n
// (coeff -d, d>0) using non-negative multipliers: d*p + a*n cancels v. The
// result keeps int64 coefficients; it returns false on overflow.
func combine(p, n linearRow, v pathdom.PathKey) (linearRow, bool) {
	a := p.coeffs[v]  // > 0
	d := -n.coeffs[v] // > 0
	coeffs := make(map[pathdom.PathKey]int64, len(p.coeffs)+len(n.coeffs))
	for w, c := range p.coeffs {
		if w == v {
			continue
		}
		scaled, ok := mulInt64(c, d)
		if !ok {
			return linearRow{}, false
		}
		coeffs[w] = scaled
	}
	for w, c := range n.coeffs {
		if w == v {
			continue
		}
		scaled, ok := mulInt64(c, a)
		if !ok {
			return linearRow{}, false
		}
		sum, ok := addInt64(coeffs[w], scaled)
		if !ok {
			return linearRow{}, false
		}
		if sum == 0 {
			delete(coeffs, w)
		} else {
			coeffs[w] = sum
		}
	}
	pBound, ok := mulInt64(p.bound, d)
	if !ok {
		return linearRow{}, false
	}
	nBound, ok := mulInt64(n.bound, a)
	if !ok {
		return linearRow{}, false
	}
	bound, ok := addInt64(pBound, nBound)
	if !ok {
		return linearRow{}, false
	}
	return linearRow{coeffs: coeffs, bound: bound}, true
}

// contradiction reports whether any row has no variables and a negative bound,
// i.e. 0 <= negative.
func contradiction(rows []linearRow) bool {
	for _, r := range rows {
		if len(r.coeffs) == 0 && r.bound < 0 {
			return true
		}
	}
	return false
}

func (r linearRow) clone() linearRow {
	return linearRow{coeffs: maps.Clone(r.coeffs), bound: r.bound}
}

func addInt64(a, b int64) (int64, bool) {
	sum := a + b
	if (a > 0 && b > 0 && sum < 0) || (a < 0 && b < 0 && sum >= 0) {
		return 0, false
	}
	return sum, true
}

func negInt64(a int64) (int64, bool) {
	if a == minInt64 {
		return 0, false
	}
	return -a, true
}

const minInt64 = -1 << 63

func mulInt64(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	hi, lo := bits.Mul64(magnitude(a), magnitude(b))
	if hi != 0 {
		return 0, false
	}
	negative := (a < 0) != (b < 0)
	if negative {
		if lo > 1<<63 {
			return 0, false
		}
		return -int64(lo), true
	}
	if lo > (1<<63)-1 {
		return 0, false
	}
	return int64(lo), true
}

// magnitude returns the absolute value of a as a uint64, correct for minInt64.
func magnitude(a int64) uint64 {
	if a < 0 {
		return uint64(-(a + 1)) + 1
	}
	return uint64(a)
}
