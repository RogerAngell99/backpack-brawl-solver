import unittest

from analysis.tools.build_catalog import build_recipe, geometry_equivalent, merge_item, slug


class BuildCatalogTests(unittest.TestCase):
    def test_slug_is_stable(self):
        self.assertEqual(slug("Champion's Syrup"), "champion_s_syrup")

    def test_new_item_stars_are_unknown(self):
        item = merge_item(
            {
                "id": "New Item",
                "name": "New Item",
                "types": ["Weapon"],
                "shape": [[0, 0]],
                "star_positions": [[0, 1]],
            },
            {},
            None,
            [],
            [],
        )
        self.assertEqual(item["stars"][0]["rule_status"], "unknown")
        self.assertTrue(item["needs_review"])

    def test_runtime_star_condition_graph_is_preserved(self):
        graph = {"class": "Model.OtherItemIsOfType", "item_type": "Food"}
        item = merge_item(
            {
                "id": "fruit",
                "name": "Fruit",
                "types": ["Food"],
                "shape": [[0, 0]],
                "star_positions": [[0, 1]],
                "star_condition_graph": graph,
                "stats": [{"type": "CactusCount", "value": 1}],
            },
            {},
            None,
            [],
            [],
        )
        self.assertEqual(item["star_condition_graph"], graph)
        self.assertEqual(item["stat_types"], ["CactusCount"])

    def test_runtime_hero_scope_is_preserved(self):
        scope = {"available_to": ["Warrior"], "kind": "hero_specific", "status": "confirmed"}
        item = merge_item(
            {
                "id": "excalibur",
                "name": "Excalibur",
                "types": ["Weapon"],
                "shape": [[0, 0]],
                "star_positions": [],
                "hero_scope": scope,
            },
            {},
            None,
            [],
            [],
        )
        self.assertEqual(item["hero_scope"], scope)

    def test_existing_star_rules_are_preserved(self):
        existing = {
            "id": "known_item",
            "name": "Known Item",
            "types": ["Weapon"],
            "shape": [[0, 0]],
            "stars": [{"offset": [0, 1], "target_types": ["Armor"], "target_items": [], "effect_text": ""}],
        }
        conflicts = []
        item = merge_item(
            {"id": "known_item", "name": "Known Item", "types": ["Weapon"], "shape": [[0, 0]], "star_positions": [[0, 1]]},
            {},
            existing,
            conflicts,
            [],
        )
        self.assertEqual(item["stars"][0]["target_types"], ["Armor"])
        self.assertNotIn("rule_status", item["stars"][0])

    def test_runtime_types_shape_and_positions_replace_curated_values(self):
        conflicts = []
        overrides = []
        item = merge_item(
            {
                "id": "known_item",
                "name": "Known Item",
                "types": ["Armor"],
                "shape": [[0, 0], [1, 0]],
                "star_positions": [[-1, 0]],
            },
            {},
            {
                "id": "known_item",
                "name": "Known Item",
                "types": ["Weapon"],
                "shape": [[0, 0]],
                "stars": [{"offset": [0, 1], "target_types": ["Armor"], "target_items": [], "effect_text": ""}],
            },
            conflicts,
            overrides,
        )
        self.assertEqual(item["types"], ["Armor"])
        self.assertEqual(item["shape"], [[0, 0], [1, 0]])
        self.assertEqual(item["stars"][0]["offset"], [-1, 0])
        self.assertEqual(conflicts, [])
        self.assertEqual({entry["field"] for entry in overrides}, {"types", "shape", "star_positions"})

    def test_recipe_preserves_duplicate_ingredients(self):
        recipe, error = build_recipe(
            {"primary": "Iron Bar", "secondaries": ["Iron Bar"], "result": "Steel Bar"},
            {"iron_bar", "steel_bar"},
        )
        self.assertIsNone(error)
        self.assertEqual(recipe["ingredients"], ["iron_bar", "iron_bar"])

    def test_recipe_preserves_hero_scope(self):
        scope = {"available_to": ["Warrior"], "kind": "hero_specific", "status": "confirmed"}
        recipe, error = build_recipe(
            {"primary": "Iron Bar", "secondaries": [], "result": "Steel Bar", "hero_scope": scope},
            {"iron_bar", "steel_bar"},
        )
        self.assertIsNone(error)
        self.assertEqual(recipe["hero_scope"], scope)

    def test_geometry_conflict_ignores_rotation(self):
        self.assertTrue(
            geometry_equivalent(
                [[0, 0], [1, 0]],
                [[-1, 0], [0, -1], [0, 1], [2, 0]],
                [[0, 0], [0, 1]],
                [[1, 1], [0, 2], [-1, 1], [0, -1]],
            )
        )


if __name__ == "__main__":
    unittest.main()
