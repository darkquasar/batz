"""Batz-owned runtime bridge. Projects remain declarative YAML and native SQL.

This program never imports a project as Python. The parent copies admitted native
content into an attempt directory and verifies runtime identity before execution.
"""
from collections import Counter
import calendar
from datetime import datetime
import re
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

import duckdb
from sqlmesh import Context


class RequiredCheck(ValueError):
    pass


class Unavailable(ValueError):
    pass


def quote(value):
    return "'" + str(value).replace("'", "''") + "'"


def ident(value):
    return '"' + str(value).replace('"', '""') + '"'


def file_ref(path, root, rows):
    with path.open("rb") as f:
        digest = hashlib.file_digest(f, "sha256").hexdigest()
    return {"path": path.relative_to(root).as_posix(), "sha256": digest,
            "bytes": path.stat().st_size, "rows": rows}


def sync(path):
    with path.open("rb") as f:
        os.fsync(f.fileno())


def source_epoch_ns(value):
    if value is None:
        return None
    match = re.fullmatch(r"([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(?:[.]([0-9]{1,9}))?(Z|[+-][0-9]{2}:[0-9]{2})", value)
    if match is None:
        raise RequiredCheck("invalid mapped source time")
    seconds = calendar.timegm(datetime.strptime(match[1], "%Y-%m-%dT%H:%M:%S").timetuple())
    offset = match[3]
    if offset != "Z":
        minutes = int(offset[1:3]) * 60 + int(offset[4:6])
        seconds -= (1 if offset[0] == "+" else -1) * minutes * 60
    return seconds * 1_000_000_000 + int((match[2] or "").ljust(9, "0"))



def check_timestamp_capability(connection, payload):
    event = json.loads(payload)
    identity = event.get("userIdentity")
    if not isinstance(identity, dict) or (event.get("eventSource"), event.get("eventName"), event.get("eventType"), identity.get("type")) != ("s3.amazonaws.com", "GetObject", "AwsApiCall", "IAMUser"):
        return
    value = event.get("eventTime")
    if not isinstance(value, str):
        return  # Invalid types remain record-level mapping rejects.
    match = re.fullmatch(r"([0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})(?:[.]([0-9]+))?(Z|[+-][0-9]{2}:[0-9]{2})", value)
    if match is None:
        return
    try:
        datetime.strptime(match[1], "%Y-%m-%dT%H:%M:%S")
    except ValueError:
        return  # Malformed calendar values are not representational limitations.
    offset = match[3]
    if offset != "Z" and (int(offset[1:3]) > 23 or int(offset[4:6]) > 59):
        return
    if len(match[2] or "") > 9:
        raise Unavailable("timestamp-representation-unavailable: supplied precision exceeds TIMESTAMP_NS; owner tradeoff required")
    naive = match[1] + ("." + match[2] if match[2] else "")
    native = connection.execute("SELECT try_cast(? AS TIMESTAMP_NS)", [naive]).fetchone()[0]
    instant = source_epoch_ns(value)
    if native is None or not (-(2**63) < instant < 2**63 - 1):
        raise Unavailable("timestamp-representation-unavailable: supplied instant exceeds native TIMESTAMP_NS range; owner tradeoff required")


def main(request_path):
    request = json.loads(Path(request_path).read_text())
    artifact_root = Path(request["artifact_root"])
    project = Path(request["project"])
    state = Path(request["model_state"])
    output = Path(request["output"])
    state.mkdir(parents=True, exist_ok=True)
    output.mkdir(parents=True, exist_ok=False)
    working = state / "working.duckdb"
    config = json.loads((project / "config.yaml").read_text())
    gateway = config["gateways"]["local"]
    gateway["connection"]["database"] = str(working)
    gateway["state_connection"]["database"] = str(state / "sqlmesh-state.duckdb")
    (project / "config.yaml").write_text(json.dumps(config, indent=2) + "\n")
    settings = {"threads": 1,
                "autoinstall_known_extensions": "false",
                "autoload_known_extensions": "false"}
    connection = duckdb.connect(str(working), config=settings)
    connection.execute("CREATE SCHEMA IF NOT EXISTS batz_input")
    queries = []
    for member in request["members"]:
        path = artifact_root / member["file"]["path"]
        queries.append("SELECT *, " + quote(member["revision"]) +
                       " AS parsed_revision, " + quote(member["file"]["path"]) +
                       " AS parsed_file, file_row_number::BIGINT AS parsed_row "
                       "FROM read_parquet(" + quote(path) + ", file_row_number=true)")
    if not queries:
        raise RequiredCheck("missing explicit parsed inventory; empty datasets still have a typed closed file")
    connection.execute("CREATE OR REPLACE VIEW batz_input.parsed AS " + " UNION ALL ".join(queries))
    connection.execute("CREATE OR REPLACE TABLE batz_input.build_context AS SELECT " +
                       ",".join(quote(request[k]) + " AS " + ident(k) for k in
                                ("input_revision", "input_digest", "attempt_id", "evaluation_nonce")))
    expected_rows = sum(m["file"]["rows"] for m in request["members"])
    actual_rows = connection.execute("SELECT count(*) FROM batz_input.parsed").fetchone()[0]
    if actual_rows != expected_rows:
        raise RequiredCheck("committed input membership row mismatch")
    for (payload,) in connection.execute("SELECT payload FROM batz_input.parsed").fetchall():
        check_timestamp_capability(connection, payload)
    # Native external model declarations allow SQLMesh to expand SELECT * exactly.
    external = []
    for table in ("parsed", "build_context"):
        columns = connection.execute("DESCRIBE batz_input." + table).fetchall()
        external.append({"name": "batz_input." + table,
                         "columns": {row[0]: row[1] for row in columns}})
    (project / "external_models.yaml").write_text(json.dumps(external, indent=2) + "\n")
    expected_observations = Counter(connection.execute(
        "SELECT observation_id,parsed_revision,parsed_file,parsed_row,payload FROM batz_input.parsed").fetchall())
    connection.close()

    cli = [sys.executable, "-I", "-c", "from sqlmesh.cli.main import cli; cli()",
           "-p", str(project), "--log-file-dir", str(output / "logs")]
    common = ["plan", "--no-prompts", "--auto-apply",
              "--execution-time", "2026-01-03"]
    inherited = ()
    try:
        os.fstat(3)
        inherited = (3,)  # The workspace owner lease survives the coordinator.
    except OSError:
        pass
    commands = [cli + common, cli + common + ["--restate-model", "*",
                 "--start", "2026-01-01", "--end", "2026-01-02"]]
    for command in commands:
        subprocess.run(command, check=True, cwd=project, pass_fds=inherited)
    context = Context(paths=[str(project)])
    datasets = []
    try:
        adapter = context.engine_adapter
        proof_name = context.table_name("cloudtrail.build_evidence")
        proof = adapter.fetchone("SELECT input_revision,input_digest,attempt_id,evaluation_nonce,"
                                 "observations,mapped,unsupported,rejected FROM " + proof_name)
        expected = tuple(request[k] for k in
                         ("input_revision", "input_digest", "attempt_id", "evaluation_nonce"))
        if tuple(proof[:4]) != expected or proof[4] != expected_rows:
            raise RequiredCheck("stale or mismatched native evaluation evidence")
        for logical, model in [("source_staging", "cloudtrail.source_staging"),
                               ("api_activity", "cloudtrail.api_activity")]:
            table = context.table_name(model)
            path = output / (logical + ".parquet")
            count = adapter.fetchone("SELECT count(*) FROM " + table)[0]
            adapter.execute("COPY (SELECT * FROM " + table + ") TO " + quote(path) + " (FORMAT PARQUET)")
            sync(path)
            datasets.append({"name": logical, "model_id": model,
                             "schema": "batz-investigation/v0alpha1" if logical == "api_activity" else "batz-source-staging/v0alpha1",
                             "files": [file_ref(path, artifact_root, count)], "rows": count})
        reject_path = output / "mapping-rejects.parquet"
        staging = context.table_name("cloudtrail.source_staging")
        adapter.execute("COPY (SELECT * FROM " + staging +
                        " WHERE mapping_status='rejected') TO " + quote(reject_path) + " (FORMAT PARQUET)")
        sync(reject_path)
        proof_path = output / "evaluation.json"
        proof_path.write_text(json.dumps({"input_revision": proof[0], "input_digest": proof[1],
                                         "attempt_id": proof[2], "evaluation_nonce": proof[3],
                                         "observations": proof[4], "mapped": proof[5],
                                         "unsupported": proof[6], "rejected": proof[7],
                                         "commands": commands}, indent=2) + "\n")
        sync(proof_path)
    finally:
        context.close()

    # Publishable readers get a new closed file; no analyst-held catalog is modified.
    catalog = output / "reader.duckdb"
    reader = duckdb.connect(str(catalog), config=settings)
    binding = ident(request["binding_id"])
    reader.execute("CREATE SCHEMA " + binding)
    reader.execute("CREATE SCHEMA batz_inventory")
    for dataset in datasets:
        paths = [str(artifact_root / f["path"]) for f in dataset["files"]]
        reader.execute("CREATE VIEW " + binding + "." + ident(dataset["name"]) +
                       " AS SELECT * FROM read_parquet([" + ",".join(map(quote, paths)) + "])")
    paths = [str(artifact_root / m["file"]["path"]) for m in request["members"]]
    reader.execute("CREATE VIEW " + binding + ".parsed AS SELECT * FROM read_parquet([" +
                   ",".join(map(quote, paths)) + "])")
    reader.execute("CREATE TABLE batz_inventory.membership "
                   "(binding_id VARCHAR,dataset VARCHAR,revision VARCHAR,input_digest VARCHAR,path VARCHAR,sha256 VARCHAR,rows BIGINT)")
    parsed_dataset = {"name": "parsed", "model_id": None, "schema": "batz-parsed/v0alpha1",
                      "files": [m["file"] for m in request["members"]], "rows": expected_rows}
    datasets.append(parsed_dataset)
    for dataset in datasets:
        for member in dataset["files"]:
            reader.execute("INSERT INTO batz_inventory.membership VALUES (?,?,?,?,?,?,?)",
                           [request["binding_id"], dataset["name"], request["revision"],
                            request["input_digest"], member["path"], member["sha256"], member["rows"]])
    # Staging must preserve the exact committed observation multiset and lineage.
    staging_observations = Counter(reader.execute(
        "SELECT observation_id,parsed_revision,parsed_file,parsed_row,payload FROM " + binding + ".source_staging").fetchall())
    if staging_observations != expected_observations:
        raise RequiredCheck("required exact staging membership assertion failed")
    expected_mapped = Counter(reader.execute(
        "SELECT observation_id,parsed_revision,parsed_file,parsed_row,payload FROM " + binding +
        ".source_staging WHERE mapping_status='mapped'").fetchall())
    actual_mapped_origins = Counter(reader.execute(
        "SELECT batz.origins[1].observation_id,batz.origins[1].parsed_revision,"
        "batz.origins[1].file,batz.origins[1].row,batz.source_payload FROM " + binding +
        ".api_activity WHERE len(batz.origins)=1").fetchall())
    if actual_mapped_origins != expected_mapped:
        raise RequiredCheck("required exact mapped membership/origin assertion failed")
    # Independently check published values against retained source evidence.
    invalid = reader.execute("SELECT count(*) FROM " + binding + ".api_activity WHERE "
                             "event.action IS DISTINCT FROM 'object-read' OR event.kind IS DISTINCT FROM 'event' OR "
                             "ecs.version IS DISTINCT FROM '9.5.0' OR user.id IS DISTINCT FROM "
                             "json_extract_string(batz.source_payload,'$.userIdentity.arn') OR "
                             "batz.input_digest IS DISTINCT FROM ? OR batz.evaluation_nonce IS DISTINCT FROM ?",
                             [request["input_digest"], request["evaluation_nonce"]]).fetchone()[0]
    if invalid:
        raise RequiredCheck("required published value/identity assertion failed")
    actual_mapped = reader.execute("SELECT count(*) FROM " + binding + ".api_activity").fetchone()[0]
    if actual_mapped != proof[5]:
        raise RequiredCheck("native mapped count mismatch")
    origins = reader.execute('SELECT epoch_ns(a."@timestamp"),a.batz.source_payload,p.payload,'
                             'a.batz.time.original,a.batz.time.fraction_digits '
                             'FROM ' + binding + '.api_activity a LEFT JOIN ' + binding +
                             '.parsed p ON a.batz.origins[1].observation_id=p.observation_id '
                             'WHERE len(a.batz.origins)=1').fetchall()
    if len(origins) != actual_mapped:
        raise RequiredCheck("required origin cardinality assertion failed")
    for instant, retained, source, original, digits in origins:
        if source is None or retained != source:
            raise RequiredCheck("required source lineage assertion failed")
        event_time = json.loads(source).get("eventTime")
        expected_digits = len(event_time.split(".", 1)[1].split("Z")[0].split("+")[0].split("-")[0]) if event_time and "." in event_time else (0 if event_time else None)
        if instant != source_epoch_ns(event_time) or original != event_time or digits != expected_digits:
            raise RequiredCheck("required timestamp value/precision assertion failed")
    reader.close()
    sync(catalog)
    result = {"input_revision": request["input_revision"], "input_digest": request["input_digest"],
              "attempt_id": request["attempt_id"], "evaluation_nonce": request["evaluation_nonce"],
              "datasets": datasets, "catalog": file_ref(catalog, artifact_root, 0),
              "rejects": [file_ref(reject_path, artifact_root, proof[7])],
              "counts": {"boundary": "normalize", "accepted": proof[5], "rejected": proof[7],
                         "unsupported": proof[6], "excluded": 0},
              "checks_passed": True}
    result_path = output / "built.json"
    result_path.write_text(json.dumps(result, indent=2) + "\n")
    sync(result_path)
    fd = os.open(output, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


if __name__ == "__main__":
    try:
        main(sys.argv[1])
    except RequiredCheck as error:
        print(str(error), file=sys.stderr)
        sys.exit(4)
    except Unavailable as error:
        print(str(error), file=sys.stderr)
        sys.exit(3)
