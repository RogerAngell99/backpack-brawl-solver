//go:build searchprofile

package solver

import (
	"reflect"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestOutgoingPlacementIndexProfileDifferentialCorpus(t *testing.T) {
	for state := 0; state < 1000; state++ {
		ctx, placements := generatedOutgoingIndexState(outgoingIndexGeneratedSeed+2, state)
		index, ok := ctx.buildOutgoingPlacementIndex(placements)
		if !ok {
			t.Fatalf("state %d refused valid generated domain", state)
		}
		var legacyProfile model.OutgoingBoundSiteProfile
		legacy := ctx.upperPriorityCountsLegacyProfiled(placements, &legacyProfile)
		var indexedProfile model.OutgoingBoundSiteProfile
		indexed := ctx.upperPriorityCountsIndexedProfiled(placements, index, &indexedProfile)
		if !reflect.DeepEqual(indexed, legacy) {
			t.Fatalf("state %d indexed upper=%v, legacy=%v", state, indexed, legacy)
		}
		if !reflect.DeepEqual(indexedProfile, legacyProfile) {
			t.Fatalf("state %d indexed profile=%+v, legacy=%+v", state, indexedProfile, legacyProfile)
		}
	}
}

func TestOutgoingPlacementIndexProfiledFallbackPreservesLegacyCounters(t *testing.T) {
	ctx, placements := generatedOutgoingIndexState(outgoingIndexGeneratedSeed+3, 0)
	placements = append(placements, placements[0])
	if _, ok := ctx.buildOutgoingPlacementIndex(placements); ok {
		t.Fatal("duplicate placement unexpectedly accepted")
	}
	var wantProfile model.OutgoingBoundSiteProfile
	want := ctx.upperPriorityCountsLegacyProfiled(placements, &wantProfile)
	var gotProfile model.OutgoingBoundSiteProfile
	got := ctx.upperPriorityCountsProfiled(placements, &gotProfile)
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotProfile, wantProfile) {
		t.Fatalf("fallback got upper/profile %v/%+v, want %v/%+v", got, gotProfile, want, wantProfile)
	}
}
