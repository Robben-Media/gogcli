#!/usr/bin/env python3
"""Compare paired CLI/native workflow measurements; never invent missing metrics.

Input JSONL records have workflow, pair, route (cli/native), model, reasoning,
fixture_digest, concurrency, elapsed_ms, correct, and upstream_calls,
retries, prompt_tokens, output_tokens, tool_calls, peak_memory_bytes. Capture
startup/discovery/instruction loading in elapsed_ms for end-to-end comparisons.
This offline analyzer sends no network requests and performs no Google actions.
"""
import argparse
import hashlib
import json
import math
import statistics
from pathlib import Path


def percentile(values, fraction):
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * fraction) - 1)]


def compare(records, minimum_pairs):
    if minimum_pairs < 10:
        raise ValueError("screening requires at least ten pairs per workflow")
    fixture_path = Path(__file__).resolve().parent.parent / "internal/mcpcontract/testdata/workflows.json"
    fixture_bytes = fixture_path.read_bytes()
    expected_digest = hashlib.sha256(fixture_bytes).hexdigest()
    expected_workflows = {w["id"] for w in json.loads(fixture_bytes)["workflows"]}
    required_metrics = ("upstream_calls", "retries", "prompt_tokens", "output_tokens", "tool_calls", "peak_memory_bytes")
    controls = None
    pairs = {}
    for row in records:
        for key in ("workflow", "pair", "route", "model", "reasoning", "fixture_digest", "concurrency", "elapsed_ms", "correct"):
            if key not in row:
                raise ValueError(f"missing {key}")
        if row["route"] not in ("cli", "native"):
            raise ValueError("route must be cli or native")
        if not isinstance(row["correct"], bool):
            raise ValueError("correct must be boolean")
        if isinstance(row["elapsed_ms"], bool) or not isinstance(row["elapsed_ms"], (int, float)) or not math.isfinite(row["elapsed_ms"]) or row["elapsed_ms"] <= 0:
            raise ValueError("elapsed_ms must be finite and positive")
        if row["workflow"] not in expected_workflows:
            raise ValueError("unknown workflow")
        if row["fixture_digest"] != expected_digest:
            raise ValueError("fixture_digest must be the SHA256 of the canonical workflows.json")
        for field in ("model", "reasoning"):
            if not isinstance(row[field], str) or not row[field].strip():
                raise ValueError(f"{field} must be a nonempty string")
        if not isinstance(row["concurrency"], int) or isinstance(row["concurrency"], bool) or row["concurrency"] < 1:
            raise ValueError("concurrency must be a positive integer")
        control = (row["model"], row["reasoning"], row["fixture_digest"], row["concurrency"])
        if controls is not None and controls != control:
            raise ValueError("all runs must use the same model, reasoning, fixture digest, and concurrency")
        controls = control
        for metric in required_metrics:
            value = row.get(metric)
            if not isinstance(value, (int, float)) or isinstance(value, bool) or not math.isfinite(value) or value < 0:
                raise ValueError(f"{metric} must be a finite nonnegative measurement")
        key = (row["workflow"], row["pair"])
        routes = pairs.setdefault(key, {})
        if row["route"] in routes:
            raise ValueError(f"duplicate route in pair {key}")
        routes[row["route"]] = row
    workflows = {}
    for key, routes in pairs.items():
        if set(routes) != {"cli", "native"}:
            raise ValueError(f"unpaired run {key}")
        cli, native = routes["cli"], routes["native"]
        for field in ("model", "reasoning", "fixture_digest", "concurrency"):
            if cli[field] != native[field]:
                raise ValueError(f"mismatched {field} in pair {key}")
        workflows.setdefault(key[0], []).append((cli, native))
    if not workflows:
        raise ValueError("no paired measurements")
    results = []
    for name, runs in sorted(workflows.items()):
        cli_times = [x[0]["elapsed_ms"] for x in runs]
        native_times = [x[1]["elapsed_ms"] for x in runs]
        cli_success = sum(x[0]["correct"] for x in runs)
        native_success = sum(x[1]["correct"] for x in runs)
        median_change = statistics.median(native_times) / statistics.median(cli_times) - 1
        p95_change = percentile(native_times, .95) / percentile(cli_times, .95) - 1
        enough = len(runs) >= minimum_pairs
        row = {
            "workflow": name, "pairs": len(runs),
            "cli_p50_ms": statistics.median(cli_times), "native_p50_ms": statistics.median(native_times),
            "cli_p95_ms": percentile(cli_times, .95), "native_p95_ms": percentile(native_times, .95),
            "median_change_percent": round(100 * median_change, 3), "p95_change_percent": round(100 * p95_change, 3),
            "cli_correct": cli_success, "native_correct": native_success,
            "screening_pass": enough and native_success == len(runs) and native_success >= cli_success and median_change <= -.20 + 1e-9 and p95_change <= .10 + 1e-9,
            "p95_acceptance_sample_count_met": len(runs) >= 50,
        }
        for metric in ("upstream_calls", "retries", "prompt_tokens", "output_tokens", "tool_calls", "peak_memory_bytes"):
            if all(metric in item for pair in runs for item in pair):
                row[metric] = {route: statistics.median(pair[index][metric] for pair in runs) for index, route in enumerate(("cli", "native"))}
            else:
                row.setdefault("missing_metrics", []).append(metric)
        results.append(row)
    return {"workflows": results, "missing_workflows": sorted(expected_workflows - set(workflows)), "screening_pass": set(workflows) == expected_workflows and all(r["screening_pass"] for r in results), "release_acceptance_established": False,
            "limitation": "Screening only. Requires all five workflows, randomized paired trials, variance investigation, deterministic correctness, and actual host/live rollout gates."}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("measurements", type=Path)
    parser.add_argument("--minimum-pairs", type=int, default=10)
    args = parser.parse_args()
    if args.minimum_pairs < 10:
        parser.error("screening requires at least ten pairs per workflow")
    try:
        records = [json.loads(line) for line in args.measurements.read_text().splitlines() if line.strip()]
        result = compare(records, args.minimum_pairs)
    except (ValueError, OSError, TypeError) as exc:
        parser.error(str(exc))
    print(json.dumps(result, indent=2))
    return 0 if result["screening_pass"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
