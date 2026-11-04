"""Batz-owned runtime bridge. Projects remain declarative YAML and native SQL.

This program never imports a project as Python. The parent copies admitted native
content into an attempt directory and verifies runtime identity before execution.
"""
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import sys

import duckdb

sys.path.insert(0, str(Path(__file__).resolve().parent))
from catalog import load_context, execution_scope, scope_record, admitted_files, configure_private


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


def validate_contract(context, contract):
    if contract.get("apiVersion") != "batz-execution/v0alpha1":
        raise RequiredCheck("unsupported execution contract")
    names = [contract[k] for k in ("staging", "evidence", "checks")]
    names += list(contract["outputs"].values())
    if any(name in ("parsed", "source_staging") for name in contract["outputs"]):
        raise RequiredCheck("reserved output dataset name")
    if not contract["outputs"] or len(set(names)) != len(names):
        raise RequiredCheck("missing or duplicate required model references")
    for name in names:
        context.get_model(name, raise_if_missing=True)
    return execution_scope(context, contract, contract["pipeline"])


# Versioned minimum common profile. Optional canonical fields, when supplied,
# retain these types; pipeline checks own their source-specific values.
_REQUIRED_TYPES = {
    "@timestamp": "timestamp_ns", "ecs": "struct", "ecs.version": "varchar",
    "event": "struct", "event.kind": "varchar", "event.action": "varchar",
    "batz": "struct", "batz.source_payload": "varchar",
    "batz.input_revision": "varchar", "batz.input_digest": "varchar",
    "batz.attempt_id": "varchar", "batz.evaluation_nonce": "varchar",
    "batz.origins": "list", "batz.origins[]": "struct",
    "batz.origins[].source_event_id": "varchar", "batz.origins[].parsed_revision": "varchar",
    "batz.origins[].file": "varchar", "batz.origins[].row": "bigint",
}
_OPTIONAL_TYPES = {
    "batz.source_event_id": "varchar", "batz.logical_event_id": "varchar",
    "batz.norm_event_variant_id": "varchar", "batz.conflict": "boolean",
    "event.id": "varchar", "event.outcome": "varchar", "event.provider": "varchar",
    "event.category": "list", "event.category[]": "varchar",
    "event.type": "list", "event.type[]": "varchar",
    "user": "struct", "user.id": "varchar", "source": "struct", "source.ip": "varchar",
    "cloud": "struct", "cloud.provider": "varchar", "cloud.region": "varchar",
    "cloud.account": "struct", "cloud.account.id": "varchar",
    "batz.profile": "varchar", "batz.actor_account_id": "varchar", "batz.time": "struct",
    "batz.time.original": "varchar", "batz.time.offset": "varchar",
    "batz.time.fraction_digits": "integer", "batz.time.uncertainty": "varchar",
}


def check_profile(reader, table):
    fields = {}

    def visit(name, native):
        fields[name] = native.id
        if native.id == "struct":
            for child, kind in native.children:
                visit(name + "." + child, kind)
        elif native.id == "list":
            visit(name + "[]", native.children[0][1])

    for column in reader.execute("SELECT * FROM " + table + " LIMIT 0").description:
        visit(column[0], column[1])
    if any(fields.get(name) != kind for name, kind in _REQUIRED_TYPES.items()):
        raise RequiredCheck("required nested native profile schema failed")
    if any(name in fields and fields[name] != kind for name, kind in _OPTIONAL_TYPES.items()):
        raise RequiredCheck("incompatible nested canonical type")


def verify_published(reader, binding, request, expected_observations, proof):
    contract = request["contract"]
    staging = binding + ".source_staging"
    observed = Counter(reader.execute(
        "SELECT observation_id,parsed_revision,parsed_file,parsed_row,payload FROM " + staging).fetchall())
    if observed != expected_observations:
        raise RequiredCheck("required exact staging membership assertion failed")
    dispositions = dict(reader.execute("SELECT mapping_status,count(*) FROM " + staging + " GROUP BY mapping_status").fetchall())
    if dispositions.get("unavailable", 0):
        raise Unavailable("timestamp-representation-unavailable: staged representation is unavailable")
    if set(dispositions) - {"mapped", "unsupported", "rejected"}:
        raise RequiredCheck("unknown staging disposition")
    if tuple(dispositions.get(status, 0) for status in ("mapped", "unsupported", "rejected")) != tuple(proof[5:8]):
        raise RequiredCheck("actual staging dispositions differ from build counts")
    if reader.execute("SELECT count(*) FROM " + staging + " WHERE mapping_status='rejected' AND coalesce(mapping_reject_reason,'')=''").fetchone()[0]:
        raise RequiredCheck("mapping rejection is missing a durable reason")
    reject_columns = "observation_id,parsed_revision,parsed_file,parsed_row,payload,mapping_reject_reason"
    expected_rejects = Counter(reader.execute("SELECT " + reject_columns + " FROM " + staging + " WHERE mapping_status='rejected'").fetchall())
    actual_rejects = Counter(reader.execute("SELECT " + reject_columns + " FROM read_parquet(?)", [request["reject_path"]]).fetchall())
    if actual_rejects != expected_rejects:
        raise RequiredCheck("exported reject membership differs from staging")
    expected_mapped = Counter(reader.execute(
        "SELECT observation_id,parsed_revision,parsed_file,parsed_row,payload FROM " + staging +
        " WHERE mapping_status='mapped'").fetchall())
    actual_origins = Counter()
    for name in contract["outputs"]:
        table = binding + "." + ident(name)
        # Common nested profile and build identity; vendor values belong to SQL.
        check_profile(reader, table)
        invalid = reader.execute("SELECT count(*) FROM " + table + " WHERE "
            "batz.input_revision IS DISTINCT FROM ? OR batz.input_digest IS DISTINCT FROM ? OR "
            "batz.attempt_id IS DISTINCT FROM ? OR batz.evaluation_nonce IS DISTINCT FROM ? OR "
            "coalesce(len(batz.origins),0)<1",
            [request[k] for k in ("input_revision", "input_digest", "attempt_id", "evaluation_nonce")]).fetchone()[0]
        if invalid:
            raise RequiredCheck("required published identity/origin assertion failed")
        output_origins = Counter(reader.execute(
            "SELECT origin.source_event_id,origin.parsed_revision,"
            "origin.file,origin.row,batz.source_payload FROM " + table + ",unnest(batz.origins) AS u(origin)").fetchall())
        if output_origins - expected_mapped:
            raise RequiredCheck("required exact mapped membership/origin assertion failed")
        actual_origins |= output_origins
    if actual_origins != expected_mapped or sum(actual_origins.values()) != proof[5]:
        raise RequiredCheck("required exact mapped membership/origin assertion failed")


def reconciliation_counts(reader, binding):
    staging = binding + ".source_staging"
    columns = {c[0]: c[1] for c in reader.execute("SELECT * FROM " + staging + " LIMIT 0").description}
    native = columns.get("batz")
    fields = dict(native.children) if native is not None and native.id == "struct" else {}
    logical = "batz.logical_event_id" if str(fields.get("logical_event_id", "")).lower() == "varchar" else "NULL::VARCHAR"
    variant = "batz.norm_event_variant_id" if str(fields.get("norm_event_variant_id", "")).lower() == "varchar" else "NULL::VARCHAR"
    rows = reader.execute("SELECT observation_id," + logical + "," + variant + " FROM " + staging + " WHERE mapping_status='mapped'").fetchall()
    groups = {}
    variants = set()
    unidentified = 0
    for source_id, event_id, variant_id in rows:
        variant_key = (event_id, variant_id) if event_id and variant_id else (None, source_id)
        variants.add(variant_key)
        if event_id:
            groups.setdefault(event_id, set()).add(variant_key)
        else:
            unidentified += 1
    return dict(observations=len(rows), variants=len(variants), logical_events=len(groups),
                conflicting_events=sum(len(v)>1 for v in groups.values()), unidentified_observations=unidentified)


def check_build(request, expected_rows, proof_rows, check_rows):
    if len(check_rows) != 1 or len(check_rows[0]) != 2 or any(type(v) is not int or v < 0 for v in check_rows[0]):
        raise RequiredCheck("missing or invalid required classified checks")
    assertions, unavailable = check_rows[0]
    if unavailable:
        raise Unavailable("timestamp-representation-unavailable: native pipeline representation requires owner decision")
    if assertions:
        raise RequiredCheck("required pipeline semantic assertion failed")
    if len(proof_rows) != 1 or len(proof_rows[0]) != 8:
        raise RequiredCheck("missing or duplicate native evaluation evidence")
    proof = proof_rows[0]
    expected = tuple(request[k] for k in ("input_revision", "input_digest", "attempt_id", "evaluation_nonce"))
    if tuple(proof[:4]) != expected or proof[4] != expected_rows:
        raise RequiredCheck("stale or mismatched native evaluation evidence")
    if any(type(v) is not int or v < 0 for v in proof[4:]) or sum(proof[5:]) != expected_rows:
        raise RequiredCheck("native observation accounting failed")
    return proof


def main(request_path):
    request = json.loads(Path(request_path).read_text())
    artifact_root = Path(request["artifact_root"])
    project = Path(request["project"])
    state = Path(request["model_state"])
    output = Path(request["output"])
    admitted_files(project)
    state.mkdir(parents=True, exist_ok=False)
    configure_private(project, state)
    output.mkdir(parents=True, exist_ok=False)
    working = state / "working.duckdb"
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

    context = load_context("runtime", paths=[str(project)])
    selected = validate_contract(context, request["contract"])
    if request.get("scope", {}).get("models") and selected != request["scope"]["models"]:
        context.close()
        raise RequiredCheck("execution scope differs from admitted discovery")
    # Fresh attempt state has no intervals or prior environments to reuse. A
    # single selected plan evaluates every FULL dependency against exact current
    # membership; nonce and membership checks below independently prove freshness.
    commands = [{"skip_tests": True, "no_prompts": True, "auto_apply": True,
                 "execution_time": "2026-01-03", "select_models": selected,
                 "backfill_models": selected}]
    datasets = []
    try:
        for command in commands:
            context.plan(**command)
        adapter = context.engine_adapter
        proof_name = context.table_name(request["contract"]["evidence"])
        proof_rows = adapter.fetchall("SELECT input_revision,input_digest,attempt_id,evaluation_nonce,"
                                 "observations,mapped,unsupported,rejected FROM " + proof_name)
        check_rows = adapter.fetchall("SELECT assertion_failures,capability_failures FROM " + context.table_name(request["contract"]["checks"]))
        proof = check_build(request, expected_rows, proof_rows, check_rows)
        for logical, model in [("source_staging", request["contract"]["staging"]), *sorted(request["contract"]["outputs"].items())]:
            table = context.table_name(model)
            path = output / (logical + ".parquet")
            count = adapter.fetchone("SELECT count(*) FROM " + table)[0]
            adapter.execute("COPY (SELECT * FROM " + table + ") TO " + quote(path) + " (FORMAT PARQUET)")
            sync(path)
            datasets.append({"name": logical, "model_id": model,
                             "schema": "batz-source-staging/v0alpha1" if logical == "source_staging" else "batz-investigation/v0alpha1",
                             "files": [file_ref(path, artifact_root, count)], "rows": count})
        reject_path = output / "mapping-rejects.parquet"
        staging = context.table_name(request["contract"]["staging"])
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
    request["reject_path"] = str(reject_path)
    verify_published(reader, binding, request, expected_observations, proof)
    reject_rows = reader.execute("SELECT count(*) FROM read_parquet(?)", [str(reject_path)]).fetchone()[0]
    reconciliation = reconciliation_counts(reader, binding)
    reader.close()
    sync(catalog)
    result = {"reconciliation": reconciliation, "input_revision": request["input_revision"], "input_digest": request["input_digest"],
              "attempt_id": request["attempt_id"], "evaluation_nonce": request["evaluation_nonce"],
              "datasets": datasets, "catalog": file_ref(catalog, artifact_root, 0),
              "rejects": [file_ref(reject_path, artifact_root, reject_rows)],
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
        request = json.loads(Path(sys.argv[1]).read_text())
        if request.get("validate_only"):
            files = admitted_files(Path(request["project"]))
            # Use an in-memory override so the hashed admitted config stays exact.
            from sqlmesh.core.config import Config
            from ruamel.yaml import YAML
            config = Config(**YAML(typ="safe").load((Path(request["project"])/"config.yaml").read_text()))
            gateway = config.gateways["local"]
            gateway.connection.database = str(Path(request["project"]).parent / "working.duckdb")
            gateway.state_connection.database = str(Path(request["project"]).parent / "state.duckdb")
            context = load_context("discovery", paths=[request["project"]], config=config, load_state=False)
            try:
                selected = validate_contract(context, request["contract"])
                record = scope_record(context, request["contract"], files, selected)
                Path(request["result"]).write_text(json.dumps(record, sort_keys=True))
            except ValueError as error:
                if str(error).startswith("unsupported-input-scope:"):
                    Path(request["result"]).write_text(json.dumps({"error": "unsupported-input-scope"}))
                raise
            finally:
                context.close()
        else:
            main(sys.argv[1])
    except duckdb.BinderException as error:
        print("required common result schema failed: " + str(error), file=sys.stderr)
        sys.exit(4)
    except RequiredCheck as error:
        print(str(error), file=sys.stderr)
        sys.exit(4)
    except Unavailable as error:
        print(str(error), file=sys.stderr)
        sys.exit(3)
