"""Verify the S2 local fixture, configuration, reports and native isolation."""
from copy import deepcopy
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

from jsonschema import Draft202012Validator
from referencing import Registry, Resource


root = Path(__file__).resolve().parents[2]
evidence = root / "docs/contributing/evidence/s2/completion"
evidence.mkdir(parents=True, exist_ok=True)
go = root / "runtime/.local/go/bin/go"
python = root / "runtime/.local/venv/bin/python"
cli = root / "runtime/.local/batz"
checker = os.environ.get("BATZ_STATICCHECK") or shutil.which("staticcheck")
if not checker:
    raise SystemExit("Set BATZ_STATICCHECK to a checker compatible with the pinned Go compiler")
env = dict(os.environ, GOTOOLCHAIN="local", GOMODCACHE=str(root / "runtime/.local/gomod"),
           GOCACHE=str(root / "runtime/.local/gocache"), GOPATH=str(root / "runtime/.local/gopath"),
           GOPROXY="off", GOSUMDB="off", GOFLAGS="-mod=readonly",
           PATH=str(go.parent) + os.pathsep + os.environ.get("PATH", ""),
           STATICCHECK_CACHE=str(root / "runtime/.local/staticcheck-cache"))


def digest(path):
    with path.open("rb") as source:
        result = hashlib.sha256()
        for block in iter(lambda: source.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


paths = []
for directory in ("cmd", "collector", "internal", "normalization", "pipelines", "fixtures", "schemas", "runtime"):
    for base, directories, names in os.walk(root / directory):
        directories[:] = [name for name in directories if name not in (".local", "__pycache__")]
        paths.extend(Path(base) / name for name in names)
paths.extend(root / name for name in (
    "go.mod", "go.sum", "README.md", "docs/usage/selection.md", "docs/usage/explain.md",
    "docs/contracts/s2-contract.md", "docs/contracts/s2-explain-proposal.md",
    "docs/contributing/check-contracts.py", "docs/contributing/verify-s2.py"))
source = {path.relative_to(root).as_posix(): digest(path) for path in sorted(set(paths))}
snapshot = {"root": str(root), "base": None, "head": None, "files": source,
            "go_version": subprocess.check_output([str(go), "version"], text=True).strip(),
            "staticcheck": {"path": checker, "sha256": digest(Path(checker)),
                            "version": subprocess.check_output([checker, "-version"], text=True).strip()}}
(evidence / "final-source.json").write_text(json.dumps(snapshot, indent=2) + "\n")
results = []


def save_results():
    (evidence / "final-check-results.json").write_text(json.dumps({
        "snapshot_sha256": digest(evidence / "final-source.json"),
        "environment": {key: env[key] for key in (
            "GOTOOLCHAIN", "GOMODCACHE", "GOCACHE", "GOPATH", "GOPROXY", "GOSUMDB", "GOFLAGS",
            "PATH", "STATICCHECK_CACHE")},
        "checks": results}, indent=2) + "\n")


def check(name, command, timeout=600):
    started = time.monotonic()
    path = evidence / ("final-" + name + ".log")
    with path.open("w") as log:
        result = subprocess.run([str(part) for part in command], cwd=root, env=env,
                                stdout=log, stderr=subprocess.STDOUT, timeout=timeout)
    results.append({"name": name, "command": [str(part) for part in command],
                    "exit": result.returncode, "seconds": time.monotonic() - started,
                    "log": path.name})
    save_results()
    print(name, result.returncode, flush=True)
    if result.returncode:
        raise SystemExit(result.returncode)
    return path


check("build-cli", [go, "build", "-o", cli, "./cmd/batz"])
check("aggregate-tests", [go, "test", "./...", "-p", "1", "-count=1", "-json"])
check("supervisor-race", [go, "test", "-race", "./internal/run", "-count=1", "-json"])
check("vet", [go, "vet", "./..."])
check("staticcheck", [checker, "./..."])
check("module-integrity", [go, "mod", "verify"])
check("contract-checks", [python, "-I", "docs/contributing/check-contracts.py"])

schemas = [json.loads(path.read_text()) for path in (root / "schemas/v0alpha1").glob("*.json")]
registry = Registry().with_resources((schema["$id"], Resource.from_contents(schema)) for schema in schemas)
report_schema = json.loads((root / "schemas/v0alpha1/explain-report.schema.json").read_text())
validator = Draft202012Validator(report_schema, registry=registry)
reports = []
for level in (1, 2, 3):
    path = check("cli-level" + str(level), [cli, "explain", "pipelines/aws-cloudtrail.yaml",
                 "--project", root, "--level", str(level), "--format", "json", "--strict"], 430)
    report = json.loads(path.read_text())
    validator.validate(report)
    for key, value in (("apiVersion", "batz/v9"), ("unexpected", True)):
        invalid = deepcopy(report)
        invalid[key] = value
        assert list(validator.iter_errors(invalid)), (level, key)
    reports.append(report)

assert all(report["apiVersion"] == "batz/v0alpha1" for report in reports)
assert reports[0]["stages"] == [] and reports[0]["runtime"] == {}
assert len(reports[1]["stages"]) == 5 and reports[1]["runtime"] == {}
assert len(reports[2]["samples"]) == len(reports[2]["structures"]) == 5
assert all(sample["basis"] == "sample-executed" and sample["rows"] == 1
           for sample in reports[2]["samples"])
assert reports[2]["runtime"]["run"]["limits"]["scratch_bytes"] == 1 << 30
assert reports[2]["runtime"]["run"]["limits"]["collector_memory_bytes"] is None
assert reports[2]["runtime"]["run"]["limits"]["model_memory_bytes"] is None

for name, wanted in source.items():
    if digest(root / name) != wanted:
        raise SystemExit("source drift during S2 verification: " + name)
results.append({"name": "live-report-contracts-and-source-stability", "exit": 0,
                "scope": "Three actual CLI levels, closed report/version checks, native boundaries, RAM amendment"})
save_results()
print("S2 composed checks passed; source snapshot remained identical", flush=True)
