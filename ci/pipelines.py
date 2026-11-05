"""Manifest discovery and path ownership; no SQL graph or pipeline registry."""
from pathlib import Path, PurePosixPath
import json
import posixpath
import re
import subprocess

from ruamel.yaml import YAML

ROOT = Path(__file__).resolve().parents[1]


def git(*args, cwd=ROOT):
    return subprocess.check_output(['git', *args], cwd=cwd)


def documents(paths, read, versions=('batz/v0alpha2',)):
    result = {}
    for name in sorted(paths):
        if not name.endswith(('.yaml', '.yml')):
            continue
        text = read(name)
        try:
            value = YAML(typ='safe').load(text)
        except Exception:
            if 'SourcePipeline' in text:
                raise ValueError('unreadable pipeline manifest: ' + name)
            continue
        if not isinstance(value, dict) or value.get('kind') != 'SourcePipeline':
            continue
        if value.get('apiVersion') not in versions:
            raise ValueError('unsupported SourcePipeline version: ' + name)
        identity = value.get('name', '')
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,127}', identity) or identity in result:
            raise ValueError('invalid or duplicate pipeline identity: ' + name)
        result[identity] = {'path': name, 'document': value}
    return result


def working_catalog():
    paths = set(git('ls-files', '-c', '-o', '--exclude-standard', '-z').decode().split('\0'))
    paths = [p for p in paths if p and (ROOT / p).is_file()]
    return documents(paths, lambda p: (ROOT / p).read_text())


def reference(manifest, value):
    if value.startswith('repo://'):
        path = value.removeprefix('repo://')
    elif '://' not in value and not value.startswith('/'):
        path = str(PurePosixPath(manifest).parent / value)
    else:
        raise ValueError('unresolvable repository reference: ' + value)
    path = posixpath.normpath(path)
    if path == '..' or path.startswith('../'):
        raise ValueError('reference escapes repository: ' + value)
    return path


def validate_manifest(value, manifest='pipeline.yaml'):
    """Use the same schema/vocabulary and supported version as loadPipeline."""
    from jsonschema import Draft7Validator, ValidationError
    schema = json.loads((ROOT/'schemas/v0alpha1/source-pipeline.schema.json').read_text())
    try:
        Draft7Validator(schema).validate(value)
    except ValidationError as error:
        raise ValueError('invalid pipeline manifest: '+error.message) from error
    if value['version'] != 1:
        raise ValueError('unsupported pipeline contract version')
    if reference(manifest, value['normalization']['project']) != '.':
        raise ValueError('unsupported native project layout')
    return value
