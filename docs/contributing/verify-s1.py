"""Fresh bounded S1 verification. Run after explicitly rebuilding/receipting tools."""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import tempfile
import time

root = Path(__file__).resolve().parents[2]
evidence = root / "docs/contributing/evidence/s1"
env = dict(os.environ, GOTOOLCHAIN="local", GOMODCACHE=str(root / "runtime/.local/gomod"),
           GOCACHE=str(root / "runtime/.local/gocache"), GOPATH=str(root / "runtime/.local/gopath"),
           GOPROXY="off", GOSUMDB="off")
go = str(root / "runtime/.local/go/bin/go")
python = str(root / "runtime/.local/venv/bin/python")
results = []

def digest(path):
    with path.open("rb") as f:
        h = hashlib.sha256()
        for block in iter(lambda: f.read(1024 * 1024), b""):
            h.update(block)
        return h.hexdigest()


def check(name, command, timeout=300):
    started = time.time()
    path = evidence / ("corrected-" + name + ".log")
    with path.open("w") as log:
        result = subprocess.run(command, cwd=root, env=env, stdout=log, stderr=subprocess.STDOUT, timeout=timeout)
    results.append({"name": name, "command": command, "exit": result.returncode,
                    "seconds": time.time() - started, "log": path.name})
    (evidence / "corrected-check-results.json").write_text(json.dumps({
        "snapshot_sha256": digest(evidence / "corrected-source.json"),
        "environment": {k: env[k] for k in ("GOTOOLCHAIN", "GOMODCACHE", "GOCACHE", "GOPATH", "GOPROXY", "GOSUMDB")},
        "checks": results}, indent=2) + "\n")
    print(name, result.returncode, flush=True)
    if result.returncode:
        raise SystemExit(result.returncode)


source = {}
for directory in ("cmd", "collector", "internal", "normalization", "pipelines", "fixtures", "schemas", "runtime"):
    for path in (root / directory).rglob("*"):
        if path.is_file() and ".local" not in path.relative_to(root).parts:
            source[path.relative_to(root).as_posix()] = digest(path)
for name in ("go.mod", "go.sum", "docs/contributing/verify-s1.py", "docs/contributing/check-contracts.py"):
    source[name] = digest(root / name)
(evidence / "corrected-source.json").write_text(json.dumps({
    "root": str(root), "base": None, "head": None, "branch": "unborn main", "files": source}, indent=2) + "\n")

check("build-cli", [go, "build", "-o", "runtime/.local/batz", "./cmd/batz"])
check("aggregate-tests", [go, "test", "./...", "-count=1", "-v"])
check("supervisor-race", [go, "test", "-race", "./internal/run", "-count=1", "-v"])
check("vet", [go, "vet", "./..."])
check("module-integrity", [go, "mod", "verify"])
check("contract-checks", [python, "-I", "docs/contributing/check-contracts.py"])
workspace = tempfile.mkdtemp(prefix="s1-native-", dir=root / "runtime/.local")
command = [str(root / "runtime/.local/batz"), "run", "pipelines/aws-cloudtrail.yaml", "--project", str(root),
           "--workspace", workspace, "--route", "local_raw", "--binding-id", "demo",
           "--input", "path=repo://fixtures/cloudtrail/api-read.json"]
check("cli-run", command, 60)
report = json.loads((evidence / "corrected-cli-run.log").read_text())
(evidence / "corrected-cli-run.json").write_text(json.dumps(report, indent=2) + "\n")
query = """SELECT event.action,"user".id,epoch_ns("@timestamp"),source.ip,
json_extract_string(batz.source_payload,'$.unmapped_example'),batz.time.fraction_digits,
typeof(event.category),typeof(event.type) FROM demo.api_activity"""
script = """import duckdb,json,sys
c=duckdb.connect(sys.argv[1],read_only=True)
actual=c.execute(sys.argv[2]).fetchall()
expected=[('object-read','arn:aws:iam::111122223333:user/example-responder',1792929600000000000,'192.0.2.10','kept',0,'VARCHAR[]','VARCHAR[]')]
print(json.dumps(actual))
assert actual==expected,(actual,expected)
"""
check("cli-query", [python, "-I", "-c", script, report["catalog"], query], 20)
check("cli-explain", [str(root / "runtime/.local/batz"), "explain", "pipelines/aws-cloudtrail.yaml",
                      "--project", str(root), "--format", "json"], 20)
for name, old_digest in source.items():
    if digest(root / name) != old_digest:
        raise SystemExit("source drift after checks: " + name)
print("source snapshot remained identical", flush=True)
