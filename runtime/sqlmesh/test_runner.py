"""Shared native integrity checks; real DuckDB, no collector/SQLMesh plans."""
from collections import Counter
import importlib.util
from pathlib import Path
import unittest
import tempfile

import duckdb

spec = importlib.util.spec_from_file_location('bridge', Path(__file__).with_name('runner.py'))
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)


class BuildEvidence(unittest.TestCase):
    def setUp(self):
        self.request = dict(input_revision='revision', input_digest='digest', attempt_id='attempt', evaluation_nonce='nonce')
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 2, 1, 0, 1)

    def test_exact_fresh_build(self):
        self.assertEqual(bridge.check_build(self.request, 2, [self.proof], [(0, 0)]), self.proof)

    def test_stale_identity_fails_even_for_empty_membership(self):
        for index in range(4):
            with self.subTest(field=index):
                proof = list(self.proof[:4]) + [0, 0, 0, 0]
                proof[index] = 'stale'
                with self.assertRaises(bridge.RequiredCheck):
                    bridge.check_build(self.request, 0, [proof], [(0, 0)])

    def test_missing_duplicate_or_invalid_checks_fail(self):
        for checks in ([], [(0, 0), (0, 0)], [(None, 0)], [(-1, 0)], [(True, 0)], [(1, 0)], [(0,)], [(0, 0, 0)]):
            with self.subTest(checks=checks), self.assertRaises(bridge.RequiredCheck):
                bridge.check_build(self.request, 2, [self.proof], checks)

    def test_capability_is_separate_from_assertion(self):
        with self.assertRaises(bridge.Unavailable):
            bridge.check_build(self.request, 2, [self.proof], [(0, 1)])

    def test_incomplete_accounting_or_missing_evidence_fails(self):
        for proofs in ([], [self.proof, self.proof], [(*self.proof[:5], 0, 0, 1)]):
            with self.subTest(proofs=proofs), self.assertRaises(bridge.RequiredCheck):
                bridge.check_build(self.request, 2, proofs, [(0, 0)])


class PublishedMembership(unittest.TestCase):
    def setUp(self):
        self.connection = duckdb.connect(config={'threads': 1})
        self.addCleanup(self.connection.close)
        self.connection.execute('CREATE SCHEMA demo')
        self.connection.execute("""CREATE TABLE demo.source_staging AS SELECT
          'observation' AS observation_id,'revision' AS parsed_revision,'file' AS parsed_file,
          0::BIGINT AS parsed_row,'{}' AS payload,'mapped' AS mapping_status,NULL::VARCHAR AS mapping_reject_reason""")
        self.connection.execute("""CREATE TABLE demo.output AS SELECT
          make_timestamp_ns(1792929600000000001) AS "@timestamp",
          {'version':'9.5.0'} AS ecs, {'kind':'event','action':'observed'} AS event,
          {'origins':[{'source_event_id':observation_id,'parsed_revision':parsed_revision,'file':parsed_file,'row':parsed_row}],
           'source_payload':payload,'input_revision':'revision','input_digest':'digest','attempt_id':'attempt','evaluation_nonce':'nonce'} AS batz
          FROM demo.source_staging""")
        self.request = dict(input_revision='revision', input_digest='digest', attempt_id='attempt', evaluation_nonce='nonce', contract={'outputs': {'output': 'probe.output'}})
        self.expected = Counter([('observation', 'revision', 'file', 0, '{}')])
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 1, 1, 0, 0)
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.request['reject_path'] = str(Path(self.scratch.name) / 'rejects.parquet')
        self.connection.execute("COPY (SELECT * FROM demo.source_staging WHERE false) TO ? (FORMAT PARQUET)", [self.request['reject_path']])

    def verify(self):
        bridge.verify_published(self.connection, 'demo', self.request, self.expected, self.proof)

    def test_real_native_nested_schema_and_precision(self):
        self.verify()
        self.assertEqual(self.connection.execute('SELECT typeof("@timestamp"),epoch_ns("@timestamp") FROM demo.output').fetchone(), ('TIMESTAMP_NS', 1792929600000000001))

    def test_reconciled_row_retains_every_exact_mapped_origin(self):
        self.connection.execute("INSERT INTO demo.source_staging SELECT 'repeat',parsed_revision,parsed_file,1,payload,mapping_status,mapping_reject_reason FROM demo.source_staging")
        self.expected.update([('repeat', 'revision', 'file', 1, '{}')])
        self.proof = (*self.proof[:4], 2, 2, 0, 0)
        self.connection.execute("UPDATE demo.output SET batz=struct_update(batz,origins := list_concat(batz.origins,[{'source_event_id':'repeat','parsed_revision':'revision','file':'file','row':1}]))")
        self.verify()
        self.connection.execute("UPDATE demo.output SET batz=struct_update(batz,origins := list_concat(batz.origins,[batz.origins[1]]))")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_equal_count_duplicate_staging_is_not_membership(self):
        self.connection.execute("UPDATE demo.source_staging SET observation_id='foreign'")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_duplicate_output_origin_is_not_valid_membership(self):
        self.connection.execute('INSERT INTO demo.output SELECT * FROM demo.output')
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_wrong_lineage_is_rejected(self):
        self.connection.execute("UPDATE demo.output SET batz=struct_update(batz,origins := [{'source_event_id':'observation','parsed_revision':'old','file':'file','row':0}])")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_stale_output_build_identity_is_rejected(self):
        self.connection.execute("UPDATE demo.output SET batz=struct_update(batz,evaluation_nonce := 'old')")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_incompatible_nested_canonical_type_cannot_publish(self):
        self.connection.execute("CREATE OR REPLACE TABLE demo.output AS SELECT * EXCLUDE(event),struct_insert(event,id := 42) AS event FROM demo.output")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_microsecond_output_cannot_claim_nanosecond_profile(self):
        self.connection.execute('ALTER TABLE demo.output ALTER COLUMN "@timestamp" TYPE TIMESTAMP')
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_unavailable_staging_cannot_be_hidden_in_coverage(self):
        self.connection.execute("INSERT INTO demo.source_staging VALUES ('blocked','revision','file',1,'{}','unavailable',NULL)")
        self.expected.update([('blocked', 'revision', 'file', 1, '{}')])
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 2, 1, 1, 0)
        with self.assertRaises(bridge.Unavailable):
            self.verify()

    def test_unknown_disposition_cannot_publish(self):
        self.connection.execute("UPDATE demo.source_staging SET mapping_status='unknown'")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_rejected_evidence_requires_a_reason(self):
        self.connection.execute("INSERT INTO demo.source_staging VALUES ('rejected','revision','file',1,'{}','rejected',NULL)")
        self.expected.update([('rejected', 'revision', 'file', 1, '{}')])
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 2, 1, 0, 1)
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_actual_durable_rejects_are_valid(self):
        self.connection.execute("INSERT INTO demo.source_staging VALUES ('rejected','revision','file',1,'{}','rejected','invalid-field')")
        self.expected.update([('rejected', 'revision', 'file', 1, '{}')])
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 2, 1, 0, 1)
        self.connection.execute("COPY (SELECT * FROM demo.source_staging WHERE mapping_status='rejected') TO ? (FORMAT PARQUET)", [self.request['reject_path']])
        self.verify()

    def test_missing_ecs_version_cannot_claim_the_profile(self):
        self.connection.execute("CREATE OR REPLACE TABLE demo.output AS SELECT * EXCLUDE(ecs),{'other':'9.5.0'} AS ecs FROM demo.output")
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_missing_exported_reject_cannot_publish(self):
        self.connection.execute("INSERT INTO demo.source_staging VALUES ('rejected','revision','file',1,'{}','rejected','invalid-field')")
        self.expected.update([('rejected', 'revision', 'file', 1, '{}')])
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 2, 1, 0, 1)
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_rejected_observation_cannot_be_reported_as_unsupported(self):
        self.connection.execute("INSERT INTO demo.source_staging VALUES ('rejected','revision','file',1,'{}','rejected','invalid-field')")
        self.expected.update([('rejected', 'revision', 'file', 1, '{}')])
        self.proof = ('revision', 'digest', 'attempt', 'nonce', 2, 1, 1, 0)
        with self.assertRaises(bridge.RequiredCheck):
            self.verify()

    def test_multiple_declared_datasets_can_share_origins(self):
        self.connection.execute('CREATE TABLE demo.another AS SELECT * FROM demo.output')
        self.request['contract']['outputs']['another'] = 'probe.another'
        self.verify()


if __name__ == '__main__':
    unittest.main(verbosity=2)
