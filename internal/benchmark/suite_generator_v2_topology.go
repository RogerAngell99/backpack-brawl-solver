package benchmark

import (
	"fmt"
	"math/rand"
)

type v2TopologyTemplate struct {
	lines []string
}

var searchSuiteV2TopologyTemplates = map[string][]v2TopologyTemplate{
	GridTopologyFull: {
		{lines: []string{"111111111", "111111111", "111111111", "111111111", "111111111", "111111111"}},
	},
	GridTopologyBottleneck: {
		{lines: []string{"111101111", "111101111", "111111111", "111101111", "111101111", "111101111"}},
		{lines: []string{"111101111", "111101111", "111101111", "111111111", "111101111", "111101111"}},
	},
	GridTopologyHoles: {
		{lines: []string{"111111111", "110111011", "111111111", "111011101", "111111111", "111111111"}},
		{lines: []string{"111111111", "111011111", "110111011", "111111111", "111110111", "111111111"}},
		{lines: []string{"111111111", "111111011", "111011111", "110111111", "111111111", "111111111"}},
	},
	GridTopologyTwoLobes: {
		{lines: []string{"111101111", "111101111", "111101111", "111101111", "111101111", "111101111"}},
	},
	GridTopologyNarrowCorridors: {
		// A broad 7x6 chamber is joined to a one-cell-wide vertical branch by
		// one bridge. The branch supplies several articulation/corridor cells
		// while leaving enough usable geometry for dense inventories.
		{lines: []string{"101111111", "101111111", "111111111", "101111111", "101111111", "101111111"}},
	},
}

func chooseTopologyGridV2(topology string, random *rand.Rand) ([]string, error) {
	templates, ok := searchSuiteV2TopologyTemplates[topology]
	if !ok || len(templates) == 0 {
		return nil, fmt.Errorf("no v2 topology template for %q", topology)
	}
	base := templates[random.Intn(len(templates))].lines
	variants := uniqueTopologyTransformsV2(base)
	return append([]string(nil), variants[random.Intn(len(variants))]...), nil
}

func uniqueTopologyTransformsV2(lines []string) [][]string {
	variants := make([][]string, 0, 4)
	seen := map[string]struct{}{}
	for _, transformed := range [][]string{
		append([]string(nil), lines...),
		mirrorTopologyHorizontallyV2(lines),
		mirrorTopologyVerticallyV2(lines),
		mirrorTopologyVerticallyV2(mirrorTopologyHorizontallyV2(lines)),
	} {
		key := ""
		for _, line := range transformed {
			key += line + "/"
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		variants = append(variants, transformed)
	}
	return variants
}

func mirrorTopologyHorizontallyV2(lines []string) []string {
	result := make([]string, len(lines))
	for row, line := range lines {
		bytes := []byte(line)
		for left, right := 0, len(bytes)-1; left < right; left, right = left+1, right-1 {
			bytes[left], bytes[right] = bytes[right], bytes[left]
		}
		result[row] = string(bytes)
	}
	return result
}

func mirrorTopologyVerticallyV2(lines []string) []string {
	result := make([]string, len(lines))
	for row := range lines {
		result[row] = lines[len(lines)-1-row]
	}
	return result
}
