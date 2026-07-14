package kasten

import "sort"

// computeFilterFacets returns the distinct policy names, statuses and actions
// present in the collected jobs, plus the min/max start timestamp. These mirror
// the filter controls in the HTML report so external consumers know the
// available filter values and the time span the report covers.
//
// StartTime values are RFC3339 UTC strings (…Z), so lexical comparison is a
// valid chronological comparison.
func computeFilterFacets(jobs []Job) FilterFacets {
	polSet := map[string]bool{}
	statSet := map[string]bool{}
	actSet := map[string]bool{}
	var dmin, dmax string
	for _, j := range jobs {
		if j.PolicyName != "" {
			polSet[j.PolicyName] = true
		}
		if j.Status != "" {
			statSet[j.Status] = true
		}
		if j.Action != "" {
			actSet[j.Action] = true
		}
		if j.StartTime != "" {
			if dmin == "" || j.StartTime < dmin {
				dmin = j.StartTime
			}
			if dmax == "" || j.StartTime > dmax {
				dmax = j.StartTime
			}
		}
	}
	return FilterFacets{
		Policies: sortedSetKeys(polSet),
		Statuses: sortedSetKeys(statSet),
		Actions:  sortedSetKeys(actSet),
		DateMin:  dmin,
		DateMax:  dmax,
	}
}

// computeFailuresByPolicy aggregates failed/errored job runs per policy,
// ordered by failure count (desc) then policy name. This is the structured
// source for a "which policies failed" report or a scheduled digest.
func computeFailuresByPolicy(jobs []Job) []PolicyFailureSummary {
	type agg struct {
		count       int
		lastFailure string
		lastError   string
		errs        map[string]bool
	}
	m := map[string]*agg{}
	for _, j := range jobs {
		if j.Status != "Failed" && j.Status != "Error" {
			continue
		}
		key := j.PolicyName
		if key == "" {
			key = "(no policy)"
		}
		a := m[key]
		if a == nil {
			a = &agg{errs: map[string]bool{}}
			m[key] = a
		}
		a.count++
		if j.StartTime > a.lastFailure {
			a.lastFailure = j.StartTime
			a.lastError = j.Error
		}
		if j.Error != "" {
			a.errs[j.Error] = true
		}
	}
	out := make([]PolicyFailureSummary, 0, len(m))
	for k, a := range m {
		out = append(out, PolicyFailureSummary{
			PolicyName:  k,
			FailedCount: a.count,
			LastFailure: a.lastFailure,
			LastError:   a.lastError,
			Errors:      sortedSetKeys(a.errs),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FailedCount != out[j].FailedCount {
			return out[i].FailedCount > out[j].FailedCount
		}
		return out[i].PolicyName < out[j].PolicyName
	})
	return out
}

// computeSuccessByAction computes the per-action success rate from the collected
// jobs — the direct source for KPIs such as "%snapshot success" and "%export
// success". Complete/Success count as success, Failed/Error as failure; Skipped,
// Running and Cancelled are excluded from the denominator, matching the semantics
// of Compliance.SuccessRate7d. Results are sorted by action name. Actions with no
// actionable outcome (only Skipped/Running) report SuccessRate == -1.
func computeSuccessByAction(jobs []Job) []ActionSuccess {
	type agg struct{ completed, failed, skipped int }
	m := map[string]*agg{}
	for _, j := range jobs {
		if j.Action == "" {
			continue
		}
		a := m[j.Action]
		if a == nil {
			a = &agg{}
			m[j.Action] = a
		}
		switch j.Status {
		case "Complete", "Success":
			a.completed++
		case "Failed", "Error":
			a.failed++
		case "Skipped":
			a.skipped++
		}
	}
	out := make([]ActionSuccess, 0, len(m))
	for action, a := range m {
		total := a.completed + a.failed
		rate := -1.0
		if total > 0 {
			rate = float64(a.completed) / float64(total) * 100
		}
		out = append(out, ActionSuccess{
			Action:      action,
			Completed:   a.completed,
			Failed:      a.failed,
			Skipped:     a.skipped,
			Total:       total,
			SuccessRate: rate,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}

// sortedSetKeys returns the keys of a string set in ascending order.
func sortedSetKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
