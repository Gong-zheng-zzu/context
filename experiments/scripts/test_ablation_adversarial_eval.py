import unittest

from ablation_adversarial_eval import COMPETITION_ABLATION_PROFILES
from security_eval import SecurityAblationEvaluator


class CompetitionAblationProfileTests(unittest.TestCase):
    def test_runner_requests_only_deterministic_server_profiles(self):
        evaluator = SecurityAblationEvaluator(profiles=COMPETITION_ABLATION_PROFILES)

        self.assertEqual(
            evaluator.profiles,
            ["regex_only", "asdf", "asdf_casia", "asdf_casia_pccm"],
        )
        self.assertNotIn("asdf_casia_pccm_llm", evaluator.profiles)


if __name__ == "__main__":
    unittest.main()
