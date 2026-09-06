import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from causal_eval import CausalEvaluator


class CausalEvaluatorNormalizationTests(unittest.TestCase):
    def test_established_medical_abbreviation_matches_canonical_term(self):
        self.assertTrue(CausalEvaluator._label_matches("甲减", "甲状腺功能减退"))
        self.assertTrue(CausalEvaluator._label_matches("COPD", "慢性阻塞性肺疾病"))

    def test_unrelated_clinical_terms_do_not_match(self):
        self.assertFalse(CausalEvaluator._label_matches("低血压", "低血糖"))


if __name__ == "__main__":
    unittest.main()
