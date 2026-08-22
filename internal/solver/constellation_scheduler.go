package solver

import (
	"sort"

	"backpack-brawl-solver/internal/model"
)

type constellationRootPackingFamilySession struct {
	root          constellationSkeleton
	familyID      string
	session       *constellationRootPackingSession
	result        constellationRootPackingResult
	nodesReserved int64
	rounds        []model.ConstellationRootPackingAllocationRound
}

type constellationRootPackingSchedule struct {
	policy        model.ConstellationRootPackingSchedulerPolicy
	families      []constellationRootPackingFamilySession
	nodesConsumed int64
}

// resolveConstellationRootPackingSchedulerPolicy resolves the M1 policy only
// after construction has established both the packing budget and family count.
// The first pass gives each family half of an equal share; each later pass uses
// that same quantum. Any tail capacity is assigned in stable family-ID order.
func resolveConstellationRootPackingSchedulerPolicy(policy ResolvedSearchPolicy, stageID string, packingBudget int64, familyCount int) model.ConstellationRootPackingSchedulerPolicy {
	resolved := model.ConstellationRootPackingSchedulerPolicy{
		StageID:                constellationSchedulerStageID(stageID),
		Name:                   policy.ConstellationRootPackingScheduler,
		AvailablePackingBudget: packingBudget,
		FamilyCount:            familyCount,
	}
	if packingBudget <= 0 || familyCount <= 0 {
		return resolved
	}
	initialDivisor := policy.ConstellationRootPackingInitialQuantumDivisor
	if initialDivisor <= 0 {
		initialDivisor = 2
	}
	roundDivisor := policy.ConstellationRootPackingRoundQuantumDivisor
	if roundDivisor <= 0 {
		roundDivisor = 2
	}
	equalShare := packingBudget / int64(familyCount)
	resolved.InitialQuantum = equalShare / initialDivisor
	if resolved.InitialQuantum <= 0 {
		resolved.InitialQuantum = 1
	}
	resolved.RoundQuantum = equalShare / roundDivisor
	if resolved.RoundQuantum <= 0 {
		resolved.RoundQuantum = 1
	}
	return resolved
}

func constellationSchedulerStageID(stageID string) string {
	if stageID == "" {
		return "single"
	}
	return stageID
}

func constellationRootPackingFamilyID(stageID string, root constellationSkeleton) string {
	rootID := root.rootID
	if rootID == "" {
		rootID = root.id
	}
	if rootID == "" {
		rootID = root.exactKey
	}
	return constellationSchedulerStageID(stageID) + "/" + rootID
}

// constellationProgressiveRootPacking runs only selected constellation roots.
// Each root is a one-member family with its own resumable MRV session; V5
// parent/frontier family searches intentionally never enter this scheduler.
func constellationProgressiveRootPacking(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	roots []constellationSkeleton,
	config Config,
	gridMask uint64,
	packingBudget int64,
	reportNode func(bool) bool,
	stopped func() bool,
) constellationRootPackingSchedule {
	policy := policyForConfig(config)
	schedule := constellationRootPackingSchedule{
		policy:   resolveConstellationRootPackingSchedulerPolicy(policy, config.stageID, packingBudget, len(roots)),
		families: make([]constellationRootPackingFamilySession, 0, len(roots)),
	}
	for _, root := range roots {
		family := constellationRootPackingFamilySession{
			root:     root,
			familyID: constellationRootPackingFamilyID(config.stageID, root),
		}
		family.session = newConstellationRootPackingSession(catalog, instances, optionsByInstance, root, config, gridMask, reportNode)
		family.result = family.session.Run(0)
		schedule.families = append(schedule.families, family)
	}
	sort.Slice(schedule.families, func(i, j int) bool {
		if schedule.families[i].familyID != schedule.families[j].familyID {
			return schedule.families[i].familyID < schedule.families[j].familyID
		}
		return schedule.families[i].root.exactKey < schedule.families[j].root.exactKey
	})

	nodesLeft := packingBudget
	for round := 1; nodesLeft > 0 && (stopped == nil || !stopped()); round++ {
		living := make([]int, 0, len(schedule.families))
		for index := range schedule.families {
			family := &schedule.families[index]
			if !constellationRootPackingFamilyLiving(family) {
				continue
			}
			living = append(living, index)
		}
		if len(living) == 0 {
			break
		}
		quantum := schedule.policy.RoundQuantum
		if round == 1 {
			quantum = schedule.policy.InitialQuantum
		}
		if quantum <= 0 {
			break
		}

		consumedThisRound := int64(0)
		for _, index := range living {
			if nodesLeft <= 0 || (stopped != nil && stopped()) {
				break
			}
			reserved := quantum
			if reserved > nodesLeft {
				reserved = nodesLeft
			}
			family := &schedule.families[index]
			before := family.result.nodes
			family.result = family.session.Run(reserved)
			consumed := family.result.nodes - before
			if consumed < 0 {
				consumed = 0
			}
			returned := reserved - consumed
			family.nodesReserved += reserved
			family.rounds = append(family.rounds, model.ConstellationRootPackingAllocationRound{
				Round:    round,
				Reserved: reserved,
				Consumed: consumed,
				Returned: returned,
			})
			schedule.nodesConsumed += consumed
			consumedThisRound += consumed
			nodesLeft -= consumed
		}
		if consumedThisRound == 0 {
			break
		}
	}
	return schedule
}

func constellationRootPackingFamilyLiving(family *constellationRootPackingFamilySession) bool {
	if family.session.Done() {
		return false
	}
	switch family.result.terminationReason {
	case "completed", "no_states", "hard_dead":
		return false
	default:
		return true
	}
}
