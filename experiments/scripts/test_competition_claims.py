"""Static guardrails for public competition pages.

These checks intentionally fail when a page starts presenting an unverified
prototype or historical benchmark as a current production capability.
"""

from pathlib import Path
import unittest


WEB = Path(__file__).resolve().parents[2] / "web"


class CompetitionClaimsTest(unittest.TestCase):
    def read(self, name: str) -> str:
        return (WEB / name).read_text(encoding="utf-8")

    def test_black_box_uses_auditable_language(self):
        page = self.read("黑匣子.html")
        for claim in ("司法级证据链", "不可篡改保证", "区块链存证技术", "导出区块链证书"):
            self.assertNotIn(claim, page)
        self.assertIn("Trace 审计证据", page)
        self.assertIn("导出审计 JSON", page)

    def test_tiered_page_does_not_publish_unverified_benchmarks(self):
        page = self.read("数据调度.html")
        for claim in ("69.5%", "8.2GB", "压缩率70%"):
            self.assertNotIn(claim, page)
        self.assertIn("主链指标待正式基准测试", page)

    def test_privacy_page_scopes_capabilities(self):
        page = self.read("隐私保护.html")
        self.assertIn("CASIA文本上下文判定", page)
        self.assertIn("音频/图像不使用该判定器", page)
        self.assertNotIn("声纹剥离</div>", page)
        self.assertNotIn("人脸模糊</div>", page)

    def test_memory_graph_is_marked_as_prototype(self):
        page = self.read("三维协同记忆图.html")
        self.assertIn("受控演示", page)


if __name__ == "__main__":
    unittest.main()
