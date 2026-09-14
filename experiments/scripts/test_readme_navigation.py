from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]


class ReadmeNavigationTest(unittest.TestCase):
    def test_readme_is_the_repository_entrypoint(self):
        readme = (ROOT / "README.md").read_text(encoding="utf-8")
        for required in (
            "docker compose up -d --build",
            "competition_console.html?mode=fixture",
            "experiments/scripts/smoke_test.py",
            "不得生成伪造结果",
            "effective.env",
        ):
            self.assertIn(required, readme)

    def test_role_selection_exposes_only_verified_competition_entries(self):
        page = (ROOT / "web" / "role_selection.html").read_text(encoding="utf-8")
        self.assertIn("competition_portal.html?mode=fixture", page)
        self.assertIn("competition_console.html?mode=fixture", page)


if __name__ == "__main__":
    unittest.main()
