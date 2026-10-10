from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "web" / "js" / "ppt-runtime-status.js"
PAGES = [
    ROOT / "web" / "黑匣子.html",
    ROOT / "web" / "数据调度.html",
    ROOT / "web" / "隐私保护.html",
    ROOT / "web" / "三维协同记忆图.html",
]


class PptRuntimeStatusContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.script = SCRIPT.read_text(encoding="utf-8")
        cls.pages = [page.read_text(encoding="utf-8") for page in PAGES]

    def test_all_ppt_pages_load_shared_status_bridge(self):
        for page in self.pages:
            self.assertIn('src="js/ppt-runtime-status.js"', page)
            self.assertIn('src="js/config.js"', page)

    def test_bridge_has_explicit_fixture_boundary_and_live_error(self):
        self.assertIn("offline_fixture", self.script)
        self.assertIn("不会联网、调用模型、写入存储或执行删除", self.script)
        self.assertIn("无法获取实时证据", self.script)
        self.assertIn("不回退到写死的成功数据", self.script)
        self.assertNotIn("localStorage", self.script)
        self.assertNotIn("sessionStorage", self.script)

    def test_bridge_reads_server_owned_status(self):
        self.assertIn("/api/v1/lifecycle/status", self.script)
        self.assertIn("/api/v1/evidence/communities", self.script)
        self.assertIn("/api/v1/multimodal/image/deidentify", self.script)
        self.assertIn("/api/v1/multimodal/audio/privacy-analysis", self.script)
        self.assertIn("new FormData()", self.script)
        self.assertIn("X-Trace-ID", self.script)
        self.assertIn("implementation", self.script)

    def test_script_escapes_server_values(self):
        self.assertRegex(self.script, re.compile(r"replace\(/\[&<>"))

    def test_pages_declare_explicit_runtime_kind(self):
        expected = {
            "黑匣子.html": "audit",
            "数据调度.html": "lifecycle",
            "隐私保护.html": "multimodal",
            "三维协同记忆图.html": "memory_graph",
        }
        for name, kind in expected.items():
            page = (ROOT / "web" / name).read_text(encoding="utf-8")
            self.assertIn(f'data-ppt-page="{kind}"', page)


if __name__ == "__main__":
    unittest.main()
