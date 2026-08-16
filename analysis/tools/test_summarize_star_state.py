import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from analysis.tools import summarize_star_state


class SummarizeStarStateTests(unittest.TestCase):
    def test_direct_condition_coverage_excludes_regular_hook_calls(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            runtime = root / "runtime.json"
            metadata = root / "metadata.json"
            report = root / "report.json"
            runtime.write_text(
                json.dumps(
                    {
                        "star_state": {
                            "observations": [],
                            "visual_snapshots": [],
                            "inventory_snapshots": [],
                            "condition_observations": [
                                {
                                    "source": "runtime_call",
                                    "condition": "Model.DefinitionIsSame",
                                    "result": True,
                                    "parameters": [{"id": "hooded_cowl"}, {"id": "hooded_cowl"}],
                                },
                                {
                                    "source": "direct_probe",
                                    "condition": "Model.DefinitionIsSame",
                                    "result": False,
                                    "parameters": [{"id": "hooded_cowl"}, {"id": "cactus"}],
                                },
                            ],
                        }
                    }
                ),
                encoding="utf-8",
            )
            metadata.write_text(json.dumps({"items": []}), encoding="utf-8")

            with patch.object(
                sys,
                "argv",
                [
                    "summarize_star_state.py",
                    "--runtime",
                    str(runtime),
                    "--metadata",
                    str(metadata),
                    "--output",
                    str(report),
                ],
            ):
                summarize_star_state.main()

            result = json.loads(report.read_text(encoding="utf-8"))
            self.assertEqual(result["condition_observation_count"], 2)
            self.assertEqual(result["direct_condition_observation_count"], 1)
            coverage = result["direct_condition_coverage"]["Model.DefinitionIsSame"]
            self.assertEqual(coverage["false"], 1)
            self.assertEqual(coverage["contexts"][0]["target"], "cactus")


if __name__ == "__main__":
    unittest.main()
