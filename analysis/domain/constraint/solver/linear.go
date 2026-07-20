package solver

import (
	"math/big"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// linearBackend decides the supported affine fragment over exact rationals. It
// stores Le, SumLe, GeConst, and LeConst assertions as rows
//
//	sum(coeff[var] * var) <= bound
//
// and proves a Le or SumLe goal by checking whether the assertions plus the
// integer-strict negation of that goal are infeasible. Pure difference goals are
// attempted by the portfolio's graph backend first; this backend is the one
// exact general-linear path for scaled and bounded-sum residue.
type linearBackend struct {
	rows []exactLinearRow
}

// exactLinearRow is the arbitrary-precision form used by the simplex. The
// negated bound can lie one beyond int64, so conversion happens before strict
// goal normalization and no arithmetic overflow becomes semantic Unknown.
type exactLinearRow struct {
	coeffs map[pathdom.PathKey]*big.Int
	bound  *big.Int
}

// exactZero is an immutable shared coefficient used only as a read operand.
var exactZero big.Int

// NewLinearBackend creates a fresh exact linear-arithmetic Solver backend.
func NewLinearBackend() Solver {
	return &linearBackend{}
}

// AffineSatisfiable reports whether asserted has a model in the exact affine
// theory implemented by the linear backend. Unlike Entails, this is a direct
// consistency query: an infeasible assertion set returns false instead of
// proving every goal by explosion. The accepted vocabulary is deliberately
// the same closed affine subset consumed by linearBackend.Assert.
func AffineSatisfiable(asserted []numeric.NumericConstraint) bool {
	backend := &linearBackend{}
	for _, constraint := range asserted {
		switch constraint.(type) {
		case numeric.Le, numeric.SumLe, numeric.GeConst, numeric.LeConst:
		default:
			return false
		}
		backend.Assert(constraint)
	}
	return linearFeasible(backend.rows)
}

func (b *linearBackend) Assert(c numeric.NumericConstraint) {
	switch v := c.(type) {
	case numeric.Le:
		coeffs := make(map[pathdom.PathKey]*big.Int, 2)
		addInt64Coefficient(coeffs, v.X, 1)
		addInt64Coefficient(coeffs, v.Y, -1)
		b.addRow(coeffs, big.NewInt(v.C))
	case numeric.SumLe:
		b.addRow(sumCoeffs(v), big.NewInt(v.C))
	case numeric.GeConst:
		coeffs := make(map[pathdom.PathKey]*big.Int, 1)
		addInt64Coefficient(coeffs, v.X, -1)
		bound := new(big.Int).Neg(big.NewInt(v.C))
		b.addRow(coeffs, bound)
	case numeric.LeConst:
		coeffs := make(map[pathdom.PathKey]*big.Int, 1)
		addInt64Coefficient(coeffs, v.X, 1)
		b.addRow(coeffs, big.NewInt(v.C))
	}
}

// sumCoeffs builds coX*x + coY*y - z, merging coincident operands. An
// empty Y drops the second positive term.
func sumCoeffs(v numeric.SumLe) map[pathdom.PathKey]*big.Int {
	coeffs := make(map[pathdom.PathKey]*big.Int, 3)
	addInt64Coefficient(coeffs, v.X, v.CoX)
	if v.Y != "" {
		addInt64Coefficient(coeffs, v.Y, v.CoY)
	}
	addInt64Coefficient(coeffs, v.Z, -1)
	return coeffs
}

func addInt64Coefficient(coeffs map[pathdom.PathKey]*big.Int, variable pathdom.PathKey, coefficient int64) {
	if coefficient == 0 {
		return
	}
	addCoefficient(coeffs, variable, big.NewInt(coefficient))
}

func addCoefficient(coeffs map[pathdom.PathKey]*big.Int, variable pathdom.PathKey, coefficient *big.Int) {
	if current := coeffs[variable]; current != nil {
		current.Add(current, coefficient)
		if current.Sign() == 0 {
			delete(coeffs, variable)
		}
		return
	}
	coeffs[variable] = new(big.Int).Set(coefficient)
}

func (b *linearBackend) addRow(coeffs map[pathdom.PathKey]*big.Int, bound *big.Int) {
	b.rows = append(b.rows, exactLinearRow{coeffs: coeffs, bound: new(big.Int).Set(bound)})
}

func (b *linearBackend) Entails(goal numeric.NumericConstraint) decision.Result {
	coeffs, bound, ok := goalAffine(goal)
	if !ok {
		return decision.Unknown
	}
	rows := make([]exactLinearRow, 0, len(b.rows)+1)
	rows = append(rows, b.rows...)
	rows = append(rows, negatedStrictRow(coeffs, bound))
	if !linearFeasible(rows) {
		return decision.Valid
	}
	return decision.Unknown
}

// goalAffine reduces an entailment goal to sum(coeff[var]*var) <= bound.
func goalAffine(goal numeric.NumericConstraint) (map[pathdom.PathKey]*big.Int, *big.Int, bool) {
	switch g := goal.(type) {
	case numeric.Le:
		coeffs := make(map[pathdom.PathKey]*big.Int, 2)
		addInt64Coefficient(coeffs, g.X, 1)
		addInt64Coefficient(coeffs, g.Y, -1)
		return coeffs, big.NewInt(g.C), true
	case numeric.SumLe:
		return sumCoeffs(g), big.NewInt(g.C), true
	}
	return nil, nil, false
}

// negatedStrictRow encodes the integer negation of expr <= bound as
// -expr <= -(bound+1), entirely in arbitrary precision.
func negatedStrictRow(coeffs map[pathdom.PathKey]*big.Int, bound *big.Int) exactLinearRow {
	strictBound := new(big.Int).Set(bound)
	strictBound.Add(strictBound, big.NewInt(1))
	strictBound.Neg(strictBound)
	row := exactLinearRow{
		coeffs: make(map[pathdom.PathKey]*big.Int, len(coeffs)),
		bound:  strictBound,
	}
	for variable, coefficient := range coeffs {
		row.coeffs[variable] = new(big.Int).Neg(coefficient)
	}
	return row
}

// linearFeasible decides feasibility of exact affine rows. Variables are free;
// each x is represented as x+ - x- before entering the non-negative simplex.
func linearFeasible(rows []exactLinearRow) bool {
	variables := linearVariables(rows)
	sortExactRows(rows, variables)
	if len(variables) == 0 {
		for _, row := range rows {
			if row.bound.Sign() < 0 {
				return false
			}
		}
		return true
	}
	simplex := newRationalSimplex(rows, variables)
	return simplex.feasible()
}

func linearVariables(rows []exactLinearRow) []pathdom.PathKey {
	set := make(map[pathdom.PathKey]struct{})
	for _, row := range rows {
		for variable, coefficient := range row.coeffs {
			if coefficient.Sign() != 0 {
				set[variable] = struct{}{}
			}
		}
	}
	variables := make([]pathdom.PathKey, 0, len(set))
	for variable := range set {
		variables = append(variables, variable)
	}
	sort.Slice(variables, func(i, j int) bool { return variables[i] < variables[j] })
	return variables
}

func sortExactRows(rows []exactLinearRow, variables []pathdom.PathKey) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, variable := range variables {
			cmp := exactCoefficient(rows[i], variable).Cmp(exactCoefficient(rows[j], variable))
			if cmp != 0 {
				return cmp < 0
			}
		}
		return rows[i].bound.Cmp(rows[j].bound) < 0
	})
}

func exactCoefficient(row exactLinearRow, variable pathdom.PathKey) *big.Int {
	if coefficient := row.coeffs[variable]; coefficient != nil {
		return coefficient
	}
	return &exactZero
}

// rationalSimplex is a deterministic two-phase tableau simplex. It uses exact
// rationals throughout and Bland's lowest-label entering/leaving rules, so
// degeneracy cannot cycle and neither row nor variable count needs a cap.
type rationalSimplex struct {
	m        int
	n        int
	basis    []int
	nonbasis []int
	tableau  [][]big.Rat
}

func newRationalSimplex(rows []exactLinearRow, variables []pathdom.PathKey) *rationalSimplex {
	m := len(rows)
	n := 2 * len(variables)
	tableau := make([][]big.Rat, m+2)
	for i := range tableau {
		tableau[i] = make([]big.Rat, n+2)
	}
	basis := make([]int, m)
	nonbasis := make([]int, n+1)
	for i, row := range rows {
		for variableIndex, variable := range variables {
			coefficient := exactCoefficient(row, variable)
			tableau[i][2*variableIndex].SetInt(coefficient)
			var negative big.Int
			negative.Neg(coefficient)
			tableau[i][2*variableIndex+1].SetInt(&negative)
		}
		basis[i] = n + i
		tableau[i][n].SetInt64(-1)
		tableau[i][n+1].SetInt(row.bound)
	}
	for j := 0; j < n; j++ {
		nonbasis[j] = j
	}
	nonbasis[n] = -1
	tableau[m+1][n].SetInt64(1)
	return &rationalSimplex{
		m:        m,
		n:        n,
		basis:    basis,
		nonbasis: nonbasis,
		tableau:  tableau,
	}
}

func (s *rationalSimplex) feasible() bool {
	leaving := 0
	for i := 1; i < s.m; i++ {
		cmp := s.tableau[i][s.n+1].Cmp(&s.tableau[leaving][s.n+1])
		if cmp < 0 || (cmp == 0 && s.basis[i] < s.basis[leaving]) {
			leaving = i
		}
	}
	if s.m > 0 && s.tableau[leaving][s.n+1].Sign() < 0 {
		s.pivot(leaving, s.n)
		if !s.runSimplex(1) || s.tableau[s.m+1][s.n+1].Sign() < 0 {
			return false
		}
		if s.tableau[s.m+1][s.n+1].Sign() != 0 {
			return false
		}
		for i := 0; i < s.m; i++ {
			if s.basis[i] != -1 {
				continue
			}
			entering := -1
			for j := 0; j <= s.n; j++ {
				if s.nonbasis[j] == -1 || s.tableau[i][j].Sign() == 0 {
					continue
				}
				if entering < 0 || s.nonbasis[j] < s.nonbasis[entering] {
					entering = j
				}
			}
			if entering >= 0 {
				s.pivot(i, entering)
			}
		}
	}
	// Phase two has the zero objective because callers ask feasibility. Running
	// the same deterministic kernel keeps the tableau contract complete while
	// normally returning immediately.
	return s.runSimplex(2)
}

func (s *rationalSimplex) runSimplex(phase int) bool {
	objective := s.m
	if phase == 1 {
		objective = s.m + 1
	}
	for {
		entering := -1
		for j := 0; j <= s.n; j++ {
			if phase == 2 && s.nonbasis[j] == -1 {
				continue
			}
			if s.tableau[objective][j].Sign() >= 0 {
				continue
			}
			if entering < 0 || s.nonbasis[j] < s.nonbasis[entering] {
				entering = j
			}
		}
		if entering < 0 {
			return true
		}

		leaving := -1
		for i := 0; i < s.m; i++ {
			if s.tableau[i][entering].Sign() <= 0 {
				continue
			}
			if leaving < 0 {
				leaving = i
				continue
			}
			var left, right big.Rat
			left.Quo(&s.tableau[i][s.n+1], &s.tableau[i][entering])
			right.Quo(&s.tableau[leaving][s.n+1], &s.tableau[leaving][entering])
			cmp := left.Cmp(&right)
			if cmp < 0 || (cmp == 0 && s.basis[i] < s.basis[leaving]) {
				leaving = i
			}
		}
		if leaving < 0 {
			return false
		}
		s.pivot(leaving, entering)
	}
}

func (s *rationalSimplex) pivot(row, column int) {
	var inverse big.Rat
	inverse.Inv(&s.tableau[row][column])
	for i := 0; i < s.m+2; i++ {
		if i == row {
			continue
		}
		for j := 0; j < s.n+2; j++ {
			if j == column {
				continue
			}
			var term big.Rat
			term.Mul(&s.tableau[row][j], &s.tableau[i][column])
			term.Mul(&term, &inverse)
			s.tableau[i][j].Sub(&s.tableau[i][j], &term)
		}
	}
	for j := 0; j < s.n+2; j++ {
		if j != column {
			s.tableau[row][j].Mul(&s.tableau[row][j], &inverse)
		}
	}
	for i := 0; i < s.m+2; i++ {
		if i == row {
			continue
		}
		var scaled big.Rat
		scaled.Mul(&s.tableau[i][column], &inverse)
		s.tableau[i][column].Neg(&scaled)
	}
	s.tableau[row][column].Set(&inverse)
	s.basis[row], s.nonbasis[column] = s.nonbasis[column], s.basis[row]
}
