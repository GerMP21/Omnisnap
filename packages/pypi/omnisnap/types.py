"""TypedDicts for summary.json, generated from schema/*.schema.json by
packages/types/generate.py; a drift check in CI regenerates and diffs.
Do not edit by hand."""
from __future__ import annotations

from typing import Any, TypedDict


class Cause(TypedDict):
    id: str
    tier: str
    title: str
    kinds: list[str]
    consequences: list[str]
    selector: str
    boxes: dict[str, Any]
    delta: dict[str, float]
    kb: str | None
    evidence: dict[str, Any]
    score: float | None
    count: int
    instances: list[str]


class Result(TypedDict, total=False):
    host: dict[str, str]
    page: dict[str, str]
    baseline: dict[str, str]
    engine: dict[str, str]
    status: str
    warnings: list[str]
    stats: dict[str, float]
    shots: dict[str, Any]
    findings: str
    causes: list[Cause]
    intent: list[dict[str, Any]]
    blocked: dict[str, list[str]]
    dynamic: dict[str, list[str]]
    audit: dict[str, Any]


class Summary(TypedDict, total=False):
    schema: str
    tool: dict[str, str]
    generated: str
    job: dict[str, Any]
    hosts: list[dict[str, Any]]
    skipped: list[dict[str, str]]
    pages: list[dict[str, Any]]
    results: list[Result]
