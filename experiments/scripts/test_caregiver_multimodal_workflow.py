from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
HTML = ROOT / "web" / "caregiver_chat.html"
SCRIPT = ROOT / "web" / "js" / "caregiver-trusted-workflow.js"


class CaregiverMultimodalWorkflowTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.html = HTML.read_text(encoding="utf-8")
        cls.script = SCRIPT.read_text(encoding="utf-8")

    def test_caregiver_exposes_image_and_audio_privacy_actions(self):
        self.assertIn('id="trustedCareImage"', self.html)
        self.assertIn('id="trustedCareAudio"', self.html)
        self.assertIn('/api/v1/multimodal/image/deidentify', self.script)
        self.assertIn('/api/v1/multimodal/audio/privacy-analysis', self.script)

    def test_audio_contract_requires_memory_only_response(self):
        self.assertIn('payload.persisted !== false', self.script)
        self.assertIn('payload.raw_pcm_returned !== false', self.script)
        self.assertIn('payload.raw_pcm_persisted !== false', self.script)
        self.assertIn('syntheticPCMBase64', self.script)

    def test_javascript_syntax(self):
        node = shutil.which("node")
        if not node:
            self.skipTest("node is not installed")
        with tempfile.NamedTemporaryFile("w", suffix=".js", encoding="utf-8", delete=False) as handle:
            handle.write(self.script)
            path = handle.name
        try:
            result = subprocess.run([node, "--check", path], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        finally:
            Path(path).unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()
