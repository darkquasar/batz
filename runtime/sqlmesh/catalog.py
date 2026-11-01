"""Native SQLMesh metadata, declarative admission and private project snapshots.

Model identities and dependency edges come exclusively from SQLMesh. Source paths
supply ownership; a schema name is never a pipeline identity.
"""
import json
from pathlib import Path
import shutil
from ruamel.yaml import YAML

NATIVE_DIRS = {'models', 'audits', 'macros', 'seeds'}
UNSUPPORTED = {'config.py', 'config.yml', 'external_models.yaml', 'external_models.yml',
               'metrics', 'signals', 'materializations', 'linter', 'external_models', 'schema.yaml', 'hooks'}
TEMPLATES = ('{{', '{%', '{#', 'JINJA_QUERY_BEGIN', 'JINJA_STATEMENT_BEGIN', 'JINJA_END')


def admitted_files(root, tests=False):
    root = Path(root)
    files = []
    for entry in root.iterdir():
        if entry.name in UNSUPPORTED:
            raise ValueError('unsupported-content: ' + entry.name)
        if entry.name != 'config.yaml' and entry.name not in NATIVE_DIRS | ({'tests'} if tests else set()):
            continue
        pending = [entry]
        while pending:
            path = pending.pop()
            if path.is_symlink():
                raise ValueError('symlink in native content: ' + str(path))
            if path.is_dir():
                pending.extend(path.iterdir())
                continue
            if path.suffix not in {'.sql', '.yaml', '.yml', '.csv'}:
                if tests and path.relative_to(root).parts[0] == 'tests' and path.suffix == '.md':
                    continue
                raise ValueError('unsupported-content: ' + str(path))
            text = path.read_text() if path.suffix != '.csv' else ''
            upper = text.upper()
            if any(m in upper for m in TEMPLATES):
                raise ValueError('unsupported Jinja templating in native content: ' + str(path))
            if path.suffix == '.sql' and ('SEED' in upper or 'PYTHON' in upper):
                # Native parsing is admission only, never a second dependency graph.
                from sqlmesh.core import dialect as d
                for expression in d.parse(text, default_dialect='duckdb'):
                    if isinstance(expression, d.PythonCode):
                        raise ValueError('unsupported-content: embedded Python')
                    for kind in expression.find_all(d.ModelKind):
                        if kind.name != 'SEED':
                            continue
                        for prop in kind.expressions:
                            if prop.name == 'path':
                                seed = (path.parent / prop.args['value'].name).resolve()
                                if not seed.is_relative_to((root/'seeds').resolve()) or not seed.is_file():
                                    raise ValueError('seed path must stay in admitted seeds: ' + str(path))
            files.append(path.relative_to(root).as_posix())
    if 'config.yaml' not in files:
        raise ValueError('missing native config.yaml')
    config = YAML(typ='safe').load((root/'config.yaml').read_text())
    allowed = {'gateways', 'default_gateway', 'model_defaults', 'disable_anonymized_analytics'}
    if not isinstance(config, dict) or set(config) - allowed:
        raise ValueError('unsupported native configuration (hooks/loaders/globals)')
    if set(config.get('gateways', {})) != {'local'} or config.get('default_gateway') != 'local':
        raise ValueError('requires the qualified local gateway')
    gateway = config['gateways']['local']
    if set(gateway) - {'connection', 'state_connection', 'test_connection'}:
        raise ValueError('unsupported gateway configuration')
    for connection in gateway.values():
        allowed_connection = {'type', 'database', 'concurrent_tasks', 'extensions', 'connector_config'}
        if set(connection) - allowed_connection or connection.get('type') != 'duckdb':
            raise ValueError('unsupported native connection')
        settings = connection.get('connector_config', {})
        if (connection.get('extensions') or settings.get('autoinstall_known_extensions') is not False
                or settings.get('autoload_known_extensions') is not False
                or settings.get('threads') != 1 or connection.get('concurrent_tasks', 1) != 1
                or set(settings) - {'threads', 'autoinstall_known_extensions', 'autoload_known_extensions'}):
            raise ValueError('requires bounded DuckDB with extension loading disabled')
    if set(config.get('model_defaults', {})) - {'dialect', 'start'}:
        raise ValueError('unsupported native model defaults')
    return sorted(files)


def snapshot(source, destination, tests=False):
    files = admitted_files(source, tests=tests)
    destination = Path(destination)
    for relative in files:
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(Path(source) / relative, target)
    admitted_files(destination, tests=tests)
    return destination


def configure_private(project, state):
    """Only the private copy is rewritten. Never follow contributor DB locations."""
    project, state = Path(project), Path(state)
    state.mkdir(parents=True, exist_ok=True)
    config = YAML(typ='safe').load((project/'config.yaml').read_text())
    gateway = config['gateways']['local']
    for key, name in [('connection', 'working.duckdb'), ('state_connection', 'sqlmesh-state.duckdb')]:
        gateway[key]['database'] = str(state/name)
    if 'test_connection' in gateway:
        gateway['test_connection']['database'] = ':memory:'
    (project/'config.yaml').write_text(json.dumps(config, indent=2)+'\n')


def model_sources(context):
    root = Path(context.path).resolve()
    result = {}
    for name, model in context.models.items():
        if model.kind.is_external:
            continue
        if model._path is None:
            raise ValueError('unresolved model provenance: ' + name)
        path = Path(model._path).resolve().relative_to(root)
        if len(path.parts) < 3 or path.parts[0] != 'models':
            raise ValueError('unresolved model ownership: ' + str(path))
        result[name] = path.as_posix()
    return result


def roots(context, contract):
    names = [contract[k] for k in ('staging', 'evidence', 'checks')]
    names += list(contract['outputs'].values())
    return sorted({context.get_model(name, raise_if_missing=True).fqn for name in names})


def execution_scope(context, contract, pipeline_id):
    sources = model_sources(context)
    selected = set(roots(context, contract))
    for name in tuple(selected):
        selected.update(context.dag.upstream(name))
    from sqlmesh.core.dialect import normalize_model_name
    allowed_inputs = {normalize_model_name('batz_input.'+name, default_catalog=context.default_catalog, dialect='duckdb')
                      for name in ('parsed','build_context')}
    for name in selected:
        if name not in sources:
            if name not in allowed_inputs:
                raise ValueError('unsupported-input-scope: unresolved input ' + name)
            continue
        owner = Path(sources[name]).parts[1]
        if owner not in (pipeline_id, 'shared'):
            raise ValueError('unsupported-input-scope: foreign source owner ' + owner + ' for ' + name)
    return sorted(selected & sources.keys())


def scope_record(context, contract, files, selected):
    sources = model_sources(context)
    globals_ = sorted(p for p in files if p == 'config.yaml' or Path(p).parts[0] in {'audits','macros','seeds'})
    return {'roots': roots(context, contract), 'models': selected,
            'sources': {name: sources[name] for name in selected}, 'globals': globals_}


def impacted_models(base_catalog, head_catalog, changed_paths):
    impacted = set()
    for context in (base_catalog, head_catalog):
        for name, path in model_sources(context).items():
            if path in changed_paths:
                impacted.add(name)
                impacted.update(context.dag.downstream(name))
    return impacted


def load_context(role, **kwargs):
    """Report discovery separately from workloads; state belongs to the caller."""
    import sys
    import time
    from sqlmesh import Context
    started = time.monotonic()
    context = Context(**kwargs)
    print(json.dumps({'native_context': role, 'seconds': round(time.monotonic()-started, 6)}), file=sys.stderr, flush=True)
    return context
