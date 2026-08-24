//go:build !searchprofile

package solver

import "backpack-brawl-solver/internal/model"

// The normal build reduces bound attribution to inlineable no-ops and keeps
// the production bound implementations as the only executable algorithms.
type boundOperationCounters struct{}

func newBoundOperationCounters(Config) *boundOperationCounters { return nil }
func (*boundOperationCounters) snapshot() *model.BoundAttributionOperationProfile {
	return nil
}
func (*boundOperationCounters) snapshotSearch(int64, int64) *model.BoundAttributionOperationProfile {
	return nil
}
func (*boundOperationCounters) snapshotRepair(int64, int64) *model.BoundAttributionOperationProfile {
	return nil
}
func (*boundOperationCounters) prioritySite(boundPriorityAttributionSite) *model.PriorityUpperBoundSiteProfile {
	return nil
}
func (*boundOperationCounters) outgoingSite(boundOutgoingAttributionSite) *model.OutgoingBoundSiteProfile {
	return nil
}
func (*boundOperationCounters) constellationFilterInvocation(int) {}
func (*boundOperationCounters) constellationFilterResult(bool)    {}
func recordPriorityUpperBoundResult(*model.PriorityUpperBoundSiteProfile, bool) {
}

func partialRepairV3PriorityUpperBoundProfiled(
	catalog model.Catalog,
	state partialRepairState,
	optionsByInstance map[string][]model.Placement,
	priorities []string,
	compatibility *priorityStarCompatibility,
	_ *model.PriorityUpperBoundSiteProfile,
) []int {
	return partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, compatibility)
}

func (ctx *outgoingBoundContext) shouldPruneProfiled(
	placements []model.Placement,
	results []model.Solution,
	topN int,
	_ *model.OutgoingBoundSiteProfile,
) bool {
	return ctx.shouldPrune(placements, results, topN)
}
