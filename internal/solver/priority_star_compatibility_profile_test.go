//go:build searchprofile

package solver

import (
	"math/rand"
	"reflect"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestPriorityStarCompatibilityPreservesLogicalProfileExactly(t *testing.T) {
	catalog, instances, optionsByInstance, states, priorities := priorityCompatibilityBoundFixture(t)
	compatibility := newPriorityStarCompatibility(catalog, instances, []string{"source"})
	if compatibility == nil {
		t.Fatal("expected compatibility")
	}
	random := rand.New(rand.NewSource(0x52314943))
	for index := 0; index < 200; index++ {
		states = append(states, randomPriorityCompatibilityState(random, instances, optionsByInstance))
	}
	for stateIndex, state := range states {
		var legacyProfile model.PriorityUpperBoundSiteProfile
		legacyUpper := partialRepairV3PriorityUpperBoundProfiled(catalog, state, optionsByInstance, priorities, nil, &legacyProfile)
		recordPriorityUpperBoundResult(&legacyProfile, true)
		var cachedProfile model.PriorityUpperBoundSiteProfile
		cachedUpper := partialRepairV3PriorityUpperBoundProfiled(catalog, state, optionsByInstance, priorities, compatibility, &cachedProfile)
		recordPriorityUpperBoundResult(&cachedProfile, true)
		if !reflect.DeepEqual(cachedUpper, legacyUpper) {
			t.Fatalf("state %d: cached upper=%v legacy=%v", stateIndex, cachedUpper, legacyUpper)
		}
		if !reflect.DeepEqual(cachedProfile, legacyProfile) {
			t.Fatalf("state %d: cached profile=%+v legacy=%+v", stateIndex, cachedProfile, legacyProfile)
		}
		assertPriorityUpperBoundSiteProfileIdentities(t, cachedProfile)
	}
}
