package solver

import (
	"math/bits"

	"backpack-brawl-solver/internal/model"
)

// starPotentialContext is a limited-search ordering heuristic. It records the
// distinct instance partners each placement option can validly star or be starred
// by. It is intentionally not used for pruning because option pairs can conflict.
type starPotentialContext struct {
	placementPotential         map[string]int
	instancePotential          map[string]int
	priorityPlacementPotential map[string]int
	priorityInstancePotential  map[string]int
	outgoingTargets            map[string]uint64
	instanceOutgoingTargets    map[string]uint64
	preferPriorityLinks        bool
}

func newStarPotentialContext(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	priorities []string,
	semantics model.PrioritySemantics,
) *starPotentialContext {
	if len(instances) == 0 || len(instances) > 64 {
		return nil
	}
	outgoing := map[string]uint64{}
	incoming := map[string]uint64{}
	priorityPlacementPotential := map[string]int{}
	prioritySources := map[string]struct{}{}
	if semantics == model.PrioritySemanticsOutgoingPerInstanceV3 {
		for _, priority := range priorities {
			kind, sourceItemID, ok := parsePriorityForSolver(priority)
			if ok && kind == "star_source" {
				prioritySources[sourceItemID] = struct{}{}
			}
		}
	}
	for _, sourceInstance := range instances {
		sourceOptions := optionsByInstance[sourceInstance.InstanceID]
		if len(sourceOptions) == 0 {
			continue
		}
		for _, targetInstance := range instances {
			if sourceInstance.InstanceID == targetInstance.InstanceID {
				continue
			}
			targetBit := uint64(1) << uint(targetInstance.OriginalIndex)
			for _, sourceOption := range sourceOptions {
				for _, targetOption := range optionsByInstance[targetInstance.InstanceID] {
					if sourceOption.Mask&targetOption.Mask != 0 || !sourceHitsTargetWithCatalog(catalog, sourceOption, targetOption) {
						continue
					}
					outgoing[coveragePlacementKey(sourceOption)] |= targetBit
					incoming[coveragePlacementKey(targetOption)] |= uint64(1) << uint(sourceInstance.OriginalIndex)
					if _, prioritized := prioritySources[sourceInstance.ItemID]; prioritized {
						priorityPlacementPotential[coveragePlacementKey(sourceOption)]++
						priorityPlacementPotential[coveragePlacementKey(targetOption)]++
					}
				}
			}
		}
	}

	context := &starPotentialContext{
		placementPotential:         make(map[string]int, len(outgoing)+len(incoming)),
		instancePotential:          make(map[string]int, len(instances)),
		priorityPlacementPotential: priorityPlacementPotential,
		priorityInstancePotential:  make(map[string]int, len(instances)),
		outgoingTargets:            outgoing,
		instanceOutgoingTargets:    make(map[string]uint64, len(instances)),
		preferPriorityLinks:        semantics == model.PrioritySemanticsOutgoingPerInstanceV3 && len(prioritySources) > 0,
	}
	for _, instance := range instances {
		best := 0
		for _, option := range optionsByInstance[instance.InstanceID] {
			key := coveragePlacementKey(option)
			context.instanceOutgoingTargets[instance.InstanceID] |= outgoing[key]
			potential := bits.OnesCount64(outgoing[key]) + bits.OnesCount64(incoming[key])
			context.placementPotential[key] = potential
			if potential > best {
				best = potential
			}
			if priority := priorityPlacementPotential[key]; priority > context.priorityInstancePotential[instance.InstanceID] {
				context.priorityInstancePotential[instance.InstanceID] = priority
			}
		}
		context.instancePotential[instance.InstanceID] = best
	}
	return context
}

func (ctx *starPotentialContext) priorityForPlacement(placement model.Placement) int {
	if ctx == nil {
		return 0
	}
	if ctx.preferPriorityLinks {
		return ctx.priorityPlacementPotential[coveragePlacementKey(placement)]*1024 + ctx.placementPotential[coveragePlacementKey(placement)]
	}
	return ctx.placementPotential[coveragePlacementKey(placement)]
}

func (ctx *starPotentialContext) priorityForInstance(instance model.InventoryInstance) int {
	if ctx == nil {
		return 0
	}
	if ctx.preferPriorityLinks {
		return ctx.priorityInstancePotential[instance.InstanceID]*1024 + ctx.instancePotential[instance.InstanceID]
	}
	return ctx.instancePotential[instance.InstanceID]
}
