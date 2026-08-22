package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

type Scenario struct {
	Name                  string                  `json:"name"`
	Grid                  []string                `json:"grid"`
	Items                 map[string]int          `json:"items"`
	Top                   *int                    `json:"top"`
	Workers               *int                    `json:"workers"`
	MaxNodes              *int64                  `json:"max_nodes"`
	NoSkips               *bool                   `json:"no_skips"`
	StopOnCoverageCeiling *bool                   `json:"stop_on_coverage_ceiling"`
	StopOnPriorityCeiling *bool                   `json:"stop_on_priority_ceiling"`
	RepairSearch          *bool                   `json:"repair_search"`
	PrioritySemantics     model.PrioritySemantics `json:"priority_semantics"`
	Priorities            []string                `json:"priorities"`
	CoverageGroups        []CoverageGroup         `json:"coverage_groups"`
	HeroFilter            model.HeroFilter        `json:"hero_filter"`
}

type CoverageGroup struct {
	Name    string   `json:"name"`
	Sources []string `json:"sources"`
	Targets []string `json:"targets"`
}

func Load(path string) (Scenario, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}

	var loaded Scenario
	if err := json.Unmarshal(content, &loaded); err != nil {
		return Scenario{}, err
	}
	return loaded, loaded.Validate()
}

func (s Scenario) Validate() error {
	if len(s.Items) == 0 {
		return fmt.Errorf("scenario items cannot be empty")
	}
	itemCount := 0
	for itemID, count := range s.Items {
		if strings.TrimSpace(itemID) == "" {
			return fmt.Errorf("scenario item id cannot be empty")
		}
		if count <= 0 {
			return fmt.Errorf("scenario item %q count must be positive", itemID)
		}
		if count > geometry.GridCells-itemCount {
			return fmt.Errorf("scenario has more than %d items, the maximum for the %dx%d grid", geometry.GridCells, geometry.GridRows, geometry.GridCols)
		}
		itemCount += count
	}
	for idx, priority := range s.Priorities {
		if strings.TrimSpace(priority) == "" {
			return fmt.Errorf("scenario priorities[%d] cannot be empty", idx)
		}
	}
	if s.PrioritySemantics != "" &&
		s.PrioritySemantics != model.PrioritySemanticsLegacyIncomingV1 &&
		s.PrioritySemantics != model.PrioritySemanticsOutgoingV2 &&
		s.PrioritySemantics != model.PrioritySemanticsOutgoingPerInstanceV3 {
		return fmt.Errorf("unsupported priority_semantics %q", s.PrioritySemantics)
	}
	for idx, priority := range s.Priorities {
		kind, value, ok := strings.Cut(strings.TrimSpace(priority), ":")
		if !ok || strings.TrimSpace(kind) != "coverage_group" {
			continue
		}
		groupIndex, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || groupIndex < 0 || groupIndex >= len(s.CoverageGroups) {
			return fmt.Errorf("priorities[%d] references invalid coverage group %q", idx, value)
		}
	}
	for groupIndex, group := range s.CoverageGroups {
		if strings.TrimSpace(group.Name) == "" {
			return fmt.Errorf("scenario coverage_groups[%d].name cannot be empty", groupIndex)
		}
		if len(group.Sources) == 0 {
			return fmt.Errorf("scenario coverage_groups[%d].sources cannot be empty", groupIndex)
		}
		seen := map[string]bool{}
		for sourceIndex, source := range group.Sources {
			source = strings.TrimSpace(source)
			if source == "" {
				return fmt.Errorf("scenario coverage_groups[%d].sources[%d] cannot be empty", groupIndex, sourceIndex)
			}
			if seen[source] {
				return fmt.Errorf("scenario coverage_groups[%d] contains duplicate source %q", groupIndex, source)
			}
			seen[source] = true
		}
		seenTargets := map[string]bool{}
		for targetIndex, target := range group.Targets {
			target = strings.TrimSpace(target)
			if target == "" {
				return fmt.Errorf("scenario coverage_groups[%d].targets[%d] cannot be empty", groupIndex, targetIndex)
			}
			if seenTargets[target] {
				return fmt.Errorf("scenario coverage_groups[%d] contains duplicate target %q", groupIndex, target)
			}
			seenTargets[target] = true
		}
	}
	return nil
}

func (s Scenario) ItemIDs() []string {
	itemIDs := make([]string, 0, len(s.Items))
	for itemID := range s.Items {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)

	var expanded []string
	for _, itemID := range itemIDs {
		for idx := 0; idx < s.Items[itemID]; idx++ {
			expanded = append(expanded, itemID)
		}
	}
	return expanded
}

func (s Scenario) GridText() string {
	return strings.Join(s.Grid, "\n")
}

func (s Scenario) ModelCoverageGroups() []model.CoverageGroup {
	groups := make([]model.CoverageGroup, 0, len(s.CoverageGroups))
	for _, group := range s.CoverageGroups {
		sources := make([]string, 0, len(group.Sources))
		for _, source := range group.Sources {
			source = strings.TrimSpace(source)
			if source != "" {
				sources = append(sources, source)
			}
		}
		if len(sources) == 0 {
			continue
		}
		targets := make([]string, 0, len(group.Targets))
		for _, target := range group.Targets {
			target = strings.TrimSpace(target)
			if target != "" {
				targets = append(targets, target)
			}
		}
		groups = append(groups, model.CoverageGroup{
			Name:    strings.TrimSpace(group.Name),
			Sources: sources,
			Targets: targets,
		})
	}
	return groups
}
