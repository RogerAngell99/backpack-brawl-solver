import unittest

from analysis.tools.normalize_runtime import hero_scope, runtime_coordinates, static_coordinates


class NormalizeRuntimeTests(unittest.TestCase):
    def test_runtime_coordinates_convert_xy_to_row_col(self):
        self.assertEqual(runtime_coordinates([[2, 1], [-1, 0]]), [[-1, 2], [0, -1]])

    def test_static_coordinates_are_already_row_col(self):
        self.assertEqual(static_coordinates([[2, 1], [-1, 0]]), [[2, 1], [-1, 0]])

    def test_present_empty_connected_heroes_means_shared(self):
        scope = hero_scope([], True, {"Warrior", "Mage"}, "connectedHeroes")
        self.assertEqual(scope["kind"], "shared")
        self.assertEqual(scope["available_to"], ["Mage", "Warrior"])

    def test_missing_connected_heroes_stays_unknown(self):
        scope = hero_scope([], False, {"Warrior"}, "connectedHeroes")
        self.assertEqual(scope["kind"], "unknown")
        self.assertEqual(scope["status"], "unknown")

    def test_single_connected_hero_is_specific(self):
        scope = hero_scope(["Warrior"], True, {"Warrior", "Mage"}, "connectedHeroes")
        self.assertEqual(scope["kind"], "hero_specific")
        self.assertEqual(scope["available_to"], ["Warrior"])


if __name__ == "__main__":
    unittest.main()
