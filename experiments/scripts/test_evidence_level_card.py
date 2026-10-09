from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "web" / "js" / "competition-audit.js"
CONSOLE = ROOT / "web" / "competition_console.html"


class EvidenceLevelCardTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.script = SCRIPT.read_text(encoding="utf-8")
        cls.console = CONSOLE.read_text(encoding="utf-8")

    def test_card_is_loaded_by_competition_console(self):
        self.assertIn('src="js/competition-audit.js"', self.console)
        self.assertIn("evidenceLevelCard", self.script)
        self.assertIn("PPT 能力状态与证据等级", self.script)

    def test_card_separates_main_chain_prototype_and_unavailable(self):
        self.assertIn("主链", self.script)
        self.assertIn("受控原型", self.script)
        self.assertIn("未接入", self.script)
        self.assertIn("不代表持久化图谱或模型遗忘", self.script)

    def test_lifecycle_status_is_server_owned(self):
        self.assertIn("/api/v1/lifecycle/status", self.script)
        self.assertIn("in_process_evidence_lifecycle", self.script)
        self.assertIn("response.clone().json()", self.script)
        self.assertNotIn("localStorage", self.script)
        self.assertNotIn("sessionStorage", self.script)


if __name__ == "__main__":
    unittest.main()
