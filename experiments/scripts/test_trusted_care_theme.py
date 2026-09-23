#!/usr/bin/env python3
"""Static contracts for the shared trusted-care visual language."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[2]
THEME = ROOT / "web" / "css" / "trusted-care-theme.css"
PAGES = (
    "caregiver_chat.html",
    "doctor_chat.html",
    "family_chat.html",
    "elder_voice_chat.html",
    "role_selection.html",
    "competition_portal.html",
)


class TrustedCareThemeTests(unittest.TestCase):
    def test_all_public_role_and_competition_entries_load_the_shared_theme(self):
        for page_name in PAGES:
            with self.subTest(page=page_name):
                page = (ROOT / "web" / page_name).read_text(encoding="utf-8")
                self.assertIn('href="css/trusted-care-theme.css?v=', page)

    def test_theme_uses_a_single_operational_palette_without_broad_origin_rules(self):
        theme = THEME.read_text(encoding="utf-8")
        for token in ("--care-brand", "--care-panel", "--care-line", "--care-danger"):
            self.assertIn(token, theme)
        self.assertIn("prefers-reduced-motion", theme)
        self.assertNotIn("url(http", theme)
        self.assertNotIn("@import", theme)


if __name__ == "__main__":
    unittest.main()
