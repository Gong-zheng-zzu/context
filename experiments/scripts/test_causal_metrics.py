import unittest
from types import SimpleNamespace

from causal_metrics import evidence_gate, field_macro_f1, normalized_text_hash, percentile


class CausalMetricsTests(unittest.TestCase):
    def test_normalized_hash_ignores_unicode_width_and_whitespace(self):
        self.assertEqual(normalized_text_hash(" 李爷爷\t服药 "), normalized_text_hash("李爷爷 服药"))
        self.assertNotEqual(normalized_text_hash("李爷爷服药"), normalized_text_hash("王奶奶服药"))

    def test_percentile_uses_nearest_rank(self):
        self.assertEqual(percentile([3, 1, 2, 4], 0.5), 2.0)
        self.assertEqual(percentile([1, 2, 3, 4], 0.95), 4.0)
        self.assertIsNone(percentile([], 0.95))
        with self.assertRaises(ValueError):
            percentile([1], 1.1)

    def test_field_f1_penalizes_extra_relations(self):
        exact = SimpleNamespace(field_matches={"object": True, "mediator": True, "property": True, "result": True}, relation_count=1)
        noisy = SimpleNamespace(field_matches={"object": True, "mediator": False, "property": False, "result": False}, relation_count=2)
        macro, per_field = field_macro_f1([exact, noisy])
        self.assertAlmostEqual(per_field["object"], 0.8)
        self.assertLess(macro, 1.0)

    def test_evidence_gate_reports_each_threshold_without_mutating_metrics(self):
        metrics = {"strict_tuple_f1": 0.8, "field_macro_f1": 0.9, "latency_p95_ms": 2800}
        gate = evidence_gate(metrics)
        self.assertTrue(gate["passed"])
        self.assertEqual(metrics["strict_tuple_f1"], 0.8)
        failed = evidence_gate({**metrics, "latency_p95_ms": 3001})
        self.assertFalse(failed["passed"])
        self.assertFalse(failed["checks"]["p95_latency_ms"])


if __name__ == "__main__":
    unittest.main()
