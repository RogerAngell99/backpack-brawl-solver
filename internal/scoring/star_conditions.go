package scoring

import (
	"strings"
	"unicode"

	"backpack-brawl-solver/internal/model"
)

type ConditionState uint8

const (
	ConditionFalse ConditionState = iota
	ConditionTrue
	ConditionUnknown
)

type StarConditionContext struct {
	SourceID        string
	TargetID        string
	Source          *model.Item
	Target          *model.Item
	TargetBagEmpty  *bool
	TargetActivated *bool
}

func EvaluateStarCondition(condition *model.StarCondition, context StarConditionContext) ConditionState {
	if condition == nil {
		return ConditionUnknown
	}

	switch conditionClass(condition.Class) {
	case "CompoundStarCondition":
		return evaluateCompound(condition, context)
	case "OtherItemIsOfType":
		if context.Target == nil || condition.ItemType == "" {
			return ConditionUnknown
		}
		for _, itemType := range context.Target.Types {
			if itemType == condition.ItemType {
				return ConditionTrue
			}
		}
		return ConditionFalse
	case "OtherItemHasStatOfType":
		if context.Target == nil || condition.StatType == "" {
			return ConditionUnknown
		}
		wanted := canonicalStatType(condition.StatType)
		if wanted == "" {
			return ConditionUnknown
		}
		for _, statType := range context.Target.StatTypes {
			if canonicalStatType(statType) == wanted {
				return ConditionTrue
			}
		}
		return ConditionFalse
	case "OtherItemIsExactly":
		if context.Target == nil {
			return ConditionUnknown
		}
		expected := condition.DefinitionID
		if expected == "" && condition.Definition != nil {
			expected = condition.Definition.ID
			if expected == "" {
				expected = condition.Definition.Name
			}
		}
		expected = canonicalDefinition(expected)
		if expected == "" {
			return ConditionUnknown
		}
		targetDefinition := context.Target.ID
		if targetDefinition == "" {
			targetDefinition = context.Target.Name
		}
		if targetDefinition == "" {
			return ConditionUnknown
		}
		if canonicalDefinition(targetDefinition) == expected {
			return ConditionTrue
		}
		return ConditionFalse
	case "DefinitionIsDifferent":
		if context.SourceID == "" || context.TargetID == "" {
			return ConditionUnknown
		}
		if context.SourceID == context.TargetID {
			return ConditionFalse
		}
		return ConditionTrue
	case "DefinitionIsSame":
		if context.SourceID == "" || context.TargetID == "" {
			return ConditionUnknown
		}
		if context.SourceID == context.TargetID {
			return ConditionTrue
		}
		return ConditionFalse
	case "OtherIsEmptyBag":
		if context.TargetBagEmpty == nil {
			return ConditionUnknown
		}
		if *context.TargetBagEmpty {
			return ConditionTrue
		}
		return ConditionFalse
	case "OtherItemHasItemActivatedSignal":
		if context.Target == nil {
			return ConditionUnknown
		}
		return ConditionTrue
	default:
		return ConditionUnknown
	}
}

func conditionClass(className string) string {
	if index := strings.LastIndex(className, "."); index >= 0 {
		return className[index+1:]
	}
	return className
}

func canonicalStatType(value string) string {
	if comma := strings.Index(value, ","); comma >= 0 {
		value = value[:comma]
	}
	if dot := strings.LastIndex(value, "."); dot >= 0 {
		value = value[dot+1:]
	}
	return value
}

func canonicalDefinition(value string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func evaluateCompound(condition *model.StarCondition, context StarConditionContext) ConditionState {
	if len(condition.Conditions) == 0 {
		return ConditionUnknown
	}
	unknown := false
	for index := range condition.Conditions {
		state := EvaluateStarCondition(&condition.Conditions[index], context)
		if condition.Any && state == ConditionTrue {
			return ConditionTrue
		}
		if !condition.Any && state == ConditionFalse {
			return ConditionFalse
		}
		if state == ConditionUnknown {
			unknown = true
		}
	}
	if unknown {
		return ConditionUnknown
	}
	if condition.Any {
		return ConditionFalse
	}
	return ConditionTrue
}

func EvaluateCatalogStarCondition(catalog model.Catalog, sourceID string, targetID string, star *model.Star) ConditionState {
	source, sourceExists := catalog.Items[sourceID]
	target, targetExists := catalog.Items[targetID]
	if !sourceExists || !targetExists {
		return ConditionUnknown
	}
	if source.StarCondition != nil {
		return EvaluateStarCondition(source.StarCondition, StarConditionContext{
			SourceID: sourceID,
			TargetID: targetID,
			Source:   &source,
			Target:   &target,
		})
	}
	if star != nil && star.RuleStatus == "unknown" {
		return ConditionUnknown
	}
	if StarMatchesItem(sourceID, targetID, &target, star) {
		return ConditionTrue
	}
	return ConditionFalse
}

func StarMatchesCatalogItems(catalog model.Catalog, sourceID string, targetID string, star *model.Star) bool {
	return EvaluateCatalogStarCondition(catalog, sourceID, targetID, star) == ConditionTrue
}
