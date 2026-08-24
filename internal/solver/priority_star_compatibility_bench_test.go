package solver

import (
	"testing"

	"backpack-brawl-solver/internal/scoring"
)

var priorityCompatibilityBenchmarkMatch bool
var priorityCompatibilityBenchmarkUpper []int
var priorityCompatibilityBenchmarkRelation *priorityStarCompatibility

func BenchmarkPriorityStarCompatibilityDirect(b *testing.B) {
	catalog, instances, _, _, _ := priorityCompatibilityBoundFixture(b)
	source := catalog.Items["source"]
	target := instances[2]
	b.ReportAllocs()
	b.ResetTimer()
	matched := false
	for index := 0; index < b.N; index++ {
		matched = matched != scoring.StarMatchesCatalogItems(catalog, source.ID, target.ItemID, &source.Stars[index%len(source.Stars)])
	}
	priorityCompatibilityBenchmarkMatch = matched
}

func BenchmarkPriorityStarCompatibilityCached(b *testing.B) {
	catalog, instances, _, _, _ := priorityCompatibilityBoundFixture(b)
	compatibility := newPriorityStarCompatibility(catalog, instances, []string{"source"})
	b.ReportAllocs()
	b.ResetTimer()
	matched := false
	for index := 0; index < b.N; index++ {
		match, cached := compatibility.match(instances[0].OriginalIndex, index%len(catalog.Items["source"].Stars), instances[2].OriginalIndex)
		matched = matched != (match && cached)
	}
	priorityCompatibilityBenchmarkMatch = matched
}

func BenchmarkPartialRepairPriorityUpperLegacy(b *testing.B) {
	catalog, _, optionsByInstance, states, priorities := priorityCompatibilityBoundFixture(b)
	state := states[4]
	b.ReportAllocs()
	b.ResetTimer()
	var upper []int
	for index := 0; index < b.N; index++ {
		upper = partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, nil)
	}
	priorityCompatibilityBenchmarkUpper = upper
}

func BenchmarkPartialRepairPriorityUpperCached(b *testing.B) {
	catalog, instances, optionsByInstance, states, priorities := priorityCompatibilityBoundFixture(b)
	compatibility := newPriorityStarCompatibility(catalog, instances, []string{"source"})
	state := states[4]
	b.ReportAllocs()
	b.ResetTimer()
	var upper []int
	for index := 0; index < b.N; index++ {
		upper = partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, compatibility)
	}
	priorityCompatibilityBenchmarkUpper = upper
}

func BenchmarkBuildPriorityStarCompatibility(b *testing.B) {
	catalog, instances, _, _, _ := priorityCompatibilityBoundFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	var compatibility *priorityStarCompatibility
	for index := 0; index < b.N; index++ {
		compatibility = newPriorityStarCompatibility(catalog, instances, []string{"source"})
	}
	priorityCompatibilityBenchmarkRelation = compatibility
}
