package solver

import "backpack-brawl-solver/internal/model"

// searchDiscoveredRank4Placements is a lower-bound witness found by the V4
// rank-4 dedicated exact search. It is test-only and never enters solver search.
var searchDiscoveredRank4Placements = []model.Placement{
	{InstanceID: "banana#0", ItemID: "banana", Rotation: 270, Origin: model.Coord{Row: 2, Col: 3}},
	{InstanceID: "cactrio#1", ItemID: "cactrio", Rotation: 0, Origin: model.Coord{Row: 0, Col: 7}},
	{InstanceID: "champion_s_ripper#2", ItemID: "champion_s_ripper", Rotation: 90, Origin: model.Coord{Row: 0, Col: 2}},
	{InstanceID: "cleansing_crown#3", ItemID: "cleansing_crown", Rotation: 0, Origin: model.Coord{Row: 5, Col: 5}},
	{InstanceID: "death_essence#4", ItemID: "death_essence", Rotation: 0, Origin: model.Coord{Row: 4, Col: 7}},
	{InstanceID: "discordant_harp#5", ItemID: "discordant_harp", Rotation: 270, Origin: model.Coord{Row: 4, Col: 0}},
	{InstanceID: "donut#6", ItemID: "donut", Rotation: 0, Origin: model.Coord{Row: 3, Col: 5}},
	{InstanceID: "ginseng_root#7", ItemID: "ginseng_root", Rotation: 0, Origin: model.Coord{Row: 2, Col: 5}},
	{InstanceID: "ginseng_root#8", ItemID: "ginseng_root", Rotation: 0, Origin: model.Coord{Row: 4, Col: 4}},
	{InstanceID: "green_snapper#9", ItemID: "green_snapper", Rotation: 0, Origin: model.Coord{Row: 5, Col: 7}},
	{InstanceID: "hooded_cowl#10", ItemID: "hooded_cowl", Rotation: 90, Origin: model.Coord{Row: 2, Col: 0}},
	{InstanceID: "longing_begonia#11", ItemID: "longing_begonia", Rotation: 0, Origin: model.Coord{Row: 3, Col: 8}},
	{InstanceID: "pitahaya#12", ItemID: "pitahaya", Rotation: 0, Origin: model.Coord{Row: 3, Col: 6}},
	{InstanceID: "pitahaya#13", ItemID: "pitahaya", Rotation: 180, Origin: model.Coord{Row: 2, Col: 2}},
	{InstanceID: "spice#14", ItemID: "spice", Rotation: 0, Origin: model.Coord{Row: 4, Col: 3}},
	{InstanceID: "spice#15", ItemID: "spice", Rotation: 0, Origin: model.Coord{Row: 5, Col: 4}},
	{InstanceID: "spice#16", ItemID: "spice", Rotation: 180, Origin: model.Coord{Row: 1, Col: 4}},
	{InstanceID: "spicy_sausage#17", ItemID: "spicy_sausage", Rotation: 0, Origin: model.Coord{Row: 1, Col: 2}},
	{InstanceID: "spiked_sickle#18", ItemID: "spiked_sickle", Rotation: 180, Origin: model.Coord{Row: 0, Col: 5}},
	{InstanceID: "spirit_biscuit#19", ItemID: "spirit_biscuit", Rotation: 90, Origin: model.Coord{Row: 3, Col: 3}},
	{InstanceID: "steadfast_boots#20", ItemID: "steadfast_boots", Rotation: 180, Origin: model.Coord{Row: 1, Col: 7}},
	{InstanceID: "tender_sausage#21", ItemID: "tender_sausage", Rotation: 90, Origin: model.Coord{Row: 2, Col: 7}},
	{InstanceID: "thornwall#22", ItemID: "thornwall", Rotation: 0, Origin: model.Coord{Row: 0, Col: 0}},
	{InstanceID: "twinmaw#23", ItemID: "twinmaw", Rotation: 90, Origin: model.Coord{Row: 5, Col: 0}},
}

func searchDiscoveredRank4FixturePlacements() []model.Placement {
	return append([]model.Placement(nil), searchDiscoveredRank4Placements...)
}
