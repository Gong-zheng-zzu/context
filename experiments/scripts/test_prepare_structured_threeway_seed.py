import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

import prepare_structured_threeway_seed as seed


class StructuredThreeWaySeedTests(unittest.TestCase):
    def test_manifest_declares_deterministic_no_llm_ingestion(self):
        manifest, requests_to_submit = seed.build_requests(
            seed.DEFAULT_CORPUS, seed.ALLOWED_SESSION_ID
        )

        self.assertEqual(
            manifest["three_way_contract"]["ingestion_mode"],
            "deterministic_preannotated_no_llm",
        )
        self.assertTrue(requests_to_submit)
        self.assertTrue(
            all(
                item["request"]["metadata"]["seed_schema"]
                == "structured-threeway-v1"
                for item in requests_to_submit
            )
        )

    def test_seed_requests_preserve_all_three_lane_provenance(self):
        _, requests_to_submit = seed.build_requests(
            seed.DEFAULT_CORPUS, seed.ALLOWED_SESSION_ID
        )

        for item in requests_to_submit:
            self.assertEqual(
                item["lanes"],
                {
                    "vector": "unified_create_context",
                    "timeline": "unified_create_context",
                    "graph": "unified_create_context",
                },
            )


if __name__ == "__main__":
    unittest.main()
