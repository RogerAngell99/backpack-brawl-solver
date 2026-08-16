import unittest

from analysis.tools.build_visual_metadata import infer_base_rotation


class VisualMetadataTests(unittest.TestCase):
    def test_horizontal_shape_with_vertical_sprite_rotates_asset_back(self):
        self.assertEqual(infer_base_rotation([[0, 0], [0, 1]], (160, 320)), 90)

    def test_vertical_shape_with_vertical_sprite_keeps_asset_orientation(self):
        self.assertEqual(infer_base_rotation([[0, 0], [1, 0]], (160, 320)), 0)

    def test_square_shape_does_not_guess_orientation(self):
        self.assertEqual(infer_base_rotation([[0, 0], [0, 1], [1, 0]], (320, 320)), 0)


if __name__ == "__main__":
    unittest.main()
