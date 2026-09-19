"""Offline regression checks for the paired workflow acceptance analyzer."""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest

SCRIPT = Path(__file__).with_name("compare-mcp-benchmarks.py")
SPEC = importlib.util.spec_from_file_location("compare_mcp_benchmarks", SCRIPT)
ANALYZER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ANALYZER)


class ComparisonTests(unittest.TestCase):
    def setUp(self):
        fixture = SCRIPT.parent.parent / "internal/mcpcontract/testdata/workflows.json"
        raw = fixture.read_bytes()
        self.rows = []
        for workflow in json.loads(raw)["workflows"]:
            for pair in range(10):
                for route, elapsed in (("cli", 100), ("native", 80)):
                    self.rows.append({
                        "workflow": workflow["id"], "pair": pair, "route": route,
                        "model": "fixture-model", "reasoning": "fixed",
                        "fixture_digest": hashlib.sha256(raw).hexdigest(),
                        "concurrency": 1, "elapsed_ms": elapsed, "correct": True,
                        "upstream_calls": 2, "retries": 0, "prompt_tokens": 10,
                        "output_tokens": 10, "tool_calls": 2, "peak_memory_bytes": 100,
                    })

    def test_complete_screening_never_claims_release_acceptance(self):
        result = ANALYZER.compare(self.rows, 10)
        self.assertTrue(result["screening_pass"])
        self.assertFalse(result["release_acceptance_established"])
        self.assertTrue(all(not w["p95_acceptance_sample_count_met"] for w in result["workflows"]))

    def test_incomplete_workflows_and_wrong_answers_fail(self):
        self.assertFalse(ANALYZER.compare(self.rows[:20], 10)["screening_pass"])
        self.rows[-1]["correct"] = False
        self.assertFalse(ANALYZER.compare(self.rows, 10)["screening_pass"])

    def test_unpaired_and_heterogeneous_runs_are_rejected(self):
        with self.assertRaises(ValueError):
            ANALYZER.compare(self.rows[:-1], 10)
        for field, value in (("model", "other"), ("reasoning", "other"), ("concurrency", 2), ("fixture_digest", "other")):
            with self.subTest(field=field):
                rows = copy.deepcopy(self.rows)
                rows[-1][field] = value
                with self.assertRaises(ValueError):
                    ANALYZER.compare(rows, 10)

    def test_malformed_controls_and_missing_cost_metrics_are_rejected(self):
        for field, value in (("model", ""), ("reasoning", ""), ("concurrency", 0), ("concurrency", True), ("elapsed_ms", True), ("elapsed_ms", float("nan")), ("upstream_calls", -1)):
            with self.subTest(field=field, value=value):
                rows = copy.deepcopy(self.rows)
                rows[0][field] = value
                with self.assertRaises(ValueError):
                    ANALYZER.compare(rows, 10)
        del self.rows[0]["prompt_tokens"]
        with self.assertRaises(ValueError):
            ANALYZER.compare(self.rows, 10)

    def test_minimum_pair_count_cannot_be_weakened(self):
        with self.assertRaises(ValueError):
            ANALYZER.compare(self.rows, 1)


if __name__ == "__main__":
    unittest.main()
