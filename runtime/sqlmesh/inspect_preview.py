"""Inspect native scratch outputs; never imports project code or fetches assets."""
import json
from pathlib import Path
import sys


def expectation_failed(message):
    print(message, file=sys.stderr)
    raise SystemExit(4)


def reviewed_expectations(path):
    try:
        expected = json.loads(Path(path).read_text())
    except json.JSONDecodeError:
        expectation_failed('invalid reviewed expectation document')
    if (not isinstance(expected, dict) or
            expected.get('apiVersion') != 'batz/v0alpha1' or
            expected.get('kind') != 'SampleExpectations' or
            expected.get('dataset') != 'api_activity'):
        expectation_failed('unsupported expectation contract')
    if (type(expected.get('rows')) is not int or expected['rows'] < 0 or
            not isinstance(expected.get('values'), dict)):
        expectation_failed('invalid reviewed expectation fields')
    return expected


def safe(value):
    if isinstance(value, int) and not isinstance(value, bool) and abs(value) > 2**53-1:
        return str(value)
    if isinstance(value, dict):
        return {k: safe(v) for k, v in value.items()}
    if isinstance(value, list):
        return [safe(v) for v in value]
    return value


def field(name, native):
    children = []
    if native.id in ('struct', 'list', 'array'):
        children = [field(n, t) for n, t in native.children if n != 'size']
    return {'name': name, 'native_type': str(native), 'nullable': None,
            'encoding': 'json-text' if name in ('payload', 'source_payload') else
                        'iso-string' if native.id.startswith('timestamp') else
                        'decimal-string-if-unsafe' if native.id in ('bigint', 'ubigint', 'hugeint', 'uhugeint') else 'native',
            'fields': children}


volatile = {'observation_id', 'run_id', 'batch_id', 'attempt_id', 'input_id',
            'input_locator', 'acquired_at', 'parsed_revision', 'parsed_file',
            'input_revision', 'input_digest', 'evaluation_nonce'}
def separate(value, path, runtime):
    if isinstance(value, dict):
        result = {}
        for k, v in value.items():
            key = path + '.' + k
            if k in volatile or k == 'origins':
                runtime[key] = v
                result[k] = {'runtime_ref': key}
            else:
                result[k] = separate(v, key, runtime)
        return result
    if isinstance(value, list):
        return [separate(v, path + '.' + str(i), runtime) for i, v in enumerate(value)]
    return value


def inferred(name, value):
    children = []
    if isinstance(value, dict):
        children = [inferred(k, v) for k, v in value.items()]
    elif isinstance(value, list) and value:
        children = [inferred('item', value[0])]
    return {'name': name, 'native_type': 'JSON ' + type(value).__name__,
            'nullable': None, 'encoding': 'json', 'fields': children}

def check_reviewed_values(expected, records, count):
    """Validate sampled values; contract/semantic failures keep exit code 4."""
    if count != expected['rows']:
        expectation_failed('reviewed row expectation failed')
    for record in records:
        for key, want in expected['values'].items():
            actual = record
            for component in key.split('.'):
                if not isinstance(actual, dict) or component not in actual:
                    expectation_failed('reviewed value expectation failed: ' + key)
                actual = actual[component]
            if actual != want:
                expectation_failed('reviewed value expectation failed: ' + key)


def main(request_path):
    import duckdb

    request = json.loads(Path(request_path).read_text())
    connection = duckdb.connect(request['catalog'], read_only=True, config={
        'autoinstall_known_extensions': False, 'autoload_known_extensions': False,
        'threads': 1,
    })
    runtime = {}
    samples, structures = [], []
    source = json.loads(Path(request['source']).read_text())
    samples.append({'stage': 'source', 'basis': 'sample-executed', 'records': [safe(source)], 'rows': 1,
                    'reason': 'Exact admitted local source document'})
    structures.append({'stage': 'source', 'basis': 'inferred',
                       'fields': [inferred(k, v) for k, v in source.items()]})
    for dataset in ['parsed', 'source_staging', 'api_activity']:
        query = 'SELECT * FROM "preview"."' + dataset + '"'
        cursor = connection.execute(query + ' LIMIT 0')
        fields = [field(d[0], d[1]) for d in cursor.description]
        # DuckDB's JSON serializer preserves TIMESTAMP_NS precision, unlike Python datetime.
        records = [json.loads(r[0]) for r in connection.execute('SELECT to_json(t) FROM (' + query + ') t LIMIT 100').fetchall()]
        count = connection.execute('SELECT count(*) FROM (' + query + ')').fetchone()[0]
        for record in records:
            for column in fields:
                if column['native_type'].startswith('TIMESTAMP') and record[column['name']] is not None:
                    record[column['name']] = record[column['name']].replace(' ', 'T', 1)
        if dataset == 'parsed':
            decoded = [safe(json.loads(r['payload'])) for r in records]
            samples.append({'stage': 'decoded', 'basis': 'sample-executed', 'records': decoded,
                            'rows': count, 'reason': 'Payloads emitted by actual Bento decoding'})
            structures.append({'stage': 'decoded', 'basis': 'inferred',
                               'fields': [inferred(k, v) for k, v in (decoded[0].items() if decoded else [])]})
        if dataset == 'api_activity' and request.get('expectations'):
            expected = reviewed_expectations(request['expectations'])
            check_reviewed_values(expected, records, count)
        samples.append({'stage': dataset, 'basis': 'sample-executed',
                        'records': [separate(safe(r), dataset + '.' + str(i), runtime) for i, r in enumerate(records)],
                        'rows': count, 'reason': 'Actual native output; at most 100 displayed records.'})
        structures.append({'stage': dataset, 'basis': 'sample-executed', 'fields': fields})

    counts = request.get('counts', [])
    observed = {
        'fractional-time': connection.execute(
            'SELECT count(*) FROM "preview"."source_staging" WHERE fraction_digits > 0').fetchone()[0] > 0,
        'rejects': any(boundary['rejected'] > 0 for boundary in counts),
        'empty-input': not source['Records'],
        'unsupported-events': any(boundary['unsupported'] > 0 for boundary in counts),
    }
    observed_names = [name for name, present in observed.items() if present]
    unobserved = [name for name, present in observed.items() if not present]
    coverage_reason = ('Observed preview branches: ' + (', '.join(observed_names) or 'none') +
                       '; unobserved preview branches: ' + (', '.join(unobserved) or 'none') +
                       '. This example does not establish universal source-variant coverage.')
    for sample in samples:
        sample['reason'] += ' ' + coverage_reason
    Path(request['result']).write_text(json.dumps({'samples': samples, 'structures': structures,
                                                 'runtime': runtime, 'unobserved': unobserved}))
    connection.close()


if __name__ == '__main__':
    main(sys.argv[1])
