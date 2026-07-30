package readmodel

import (
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// callPoints returns call-site points in the solved graph's deterministic RPO
// order.
func (r Reader) callPoints() []cfg.Point {
	if r.result == nil || r.result.Graph() == nil {
		return nil
	}
	return append([]cfg.Point(nil), r.result.Graph().RPO()...)
}

// ForEachCall visits assembled solved call records in deterministic RPO order.
func (r Reader) ForEachCall(visit func(CallSite) bool) bool {
	if visit == nil {
		return false
	}
	visited := false
	for _, point := range r.callPoints() {
		if !r.result.PointReachable(point) {
			continue
		}
		site, ok := r.result.CallSiteView(point)
		if !ok {
			continue
		}
		var args []CallArgument
		r.forEachCallArgument(point, func(arg CallArgument) bool {
			args = append(args, arg)
			return true
		})
		contract, hasContract := r.callContractAt(point)
		paramObligations := r.callParamObligationsAt(point)
		callee := r.callCalleeReport(point, site)
		callee = r.withCalleeNilability(point, callee)
		call := CallSite{
			Point:      point,
			CallSpan:   sourceSpanFromFactflow(site.CallSpan()),
			CalleeSpan: sourceSpanFromFactflow(site.CalleeSpan()),
			Arguments:  args,
			SendSafety: r.sendSafetyReports(point, args),
			Reports:    r.callArgumentReports(point, contract, hasContract, args, paramObligations),
			Arity:      r.callArityReport(site, contract, hasContract),
			Callee:     callee,
		}
		visited = true
		if !visit(call) {
			return true
		}
	}
	return visited
}

func (r Reader) CallCalleeReportAt(point cfg.Point) (CallCalleeReport, bool) {
	if r.result == nil {
		return CallCalleeReport{}, false
	}
	site, ok := r.result.CallSiteView(point)
	if !ok {
		return CallCalleeReport{}, false
	}
	report := r.withCalleeNilability(point, r.callCalleeReport(point, site))
	return report, report.Kind != readapi.CallCalleeReportNone
}

func (r Reader) withCalleeNilability(point cfg.Point, report CallCalleeReport) CallCalleeReport {
	if report.Kind != readapi.CallCalleeReportMayBeNil {
		return report
	}
	report.Nilability = nilabilityProvenanceForCallee(r, point, product.Value{})
	return report
}
