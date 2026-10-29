from pathlib import Path
import json,copy,re,hashlib,importlib.metadata as m
from jsonschema import Draft202012Validator
from referencing import Registry,Resource
root=Path(__file__).resolve().parents[2]
schemas=[json.loads(p.read_text()) for p in (root/'schemas/v0alpha1').glob('*.json')]
registry=Registry().with_resources((s['$id'],Resource.from_contents(s)) for s in schemas)
for s in schemas:Draft202012Validator.check_schema(s)
checks=0
for p in sorted((root/'docs/contracts/examples').glob('*.json')):
 schema=json.loads((root/'schemas/v0alpha1'/(p.stem+'.schema.json')).read_text())
 value=json.loads(p.read_text());validator=Draft202012Validator(schema,registry=registry)
 validator.validate(value);checks+=1
 for key,val in [('apiVersion','batz/v9'),('unexpected',True)]:
  bad=copy.deepcopy(value);bad[key]=val;assert list(validator.iter_errors(bad));checks+=1
 print('PASS',p.stem,'positive / unsupported version / unknown field')
 if p.stem=='pipeline-binding':
  bad=copy.deepcopy(value);bad['input']['list']=['a','b'];validator.validate(bad);checks+=1
 if p.stem in ('collection-receipt','published-inventory'):
  for path in ['../escape.parquet','/absolute.parquet','a/../../escape.parquet','C:/escape.parquet','a\\escape.parquet']:
   bad=copy.deepcopy(value)
   f=bad['parsed_files'][0] if p.stem=='collection-receipt' else bad['datasets'][0]['files'][0]
   f['path']=path;assert list(validator.iter_errors(bad));checks+=1
  bad=copy.deepcopy(value)
  f=bad['parsed_files'][0] if p.stem=='collection-receipt' else bad['datasets'][0]['files'][0]
  f['rows']=-1;assert list(validator.iter_errors(bad));checks+=1
 if p.stem=='collection-receipt':
  for k,val in [('boundary','normalize'),('unsupported',1),('excluded',1)]:
   bad=copy.deepcopy(value);bad['counts'][k]=val;assert list(validator.iter_errors(bad));checks+=1
for p in [root/'docs/contracts/first-tracer-contract.md']:
 text=p.read_text()
 for block in re.findall(r'```text\n(.*?)```',text,re.S):
  assert '\t' not in block and max(map(len,block.splitlines()))<=100
 for _,target in re.findall(r'\[([^\]]+)\]\(([^)]+)\)',text):
  if not target.startswith('https://'):assert (p.parent/target).exists(),target
lock=json.loads((root/'runtime/dependencies.lock.json').read_text())
for name,digest in lock['lock_files'].items():assert hashlib.sha256((root/name).read_bytes()).hexdigest()==digest,name
print('PASS',checks,'document schema cases; local links; diagram width; exact lock-file hashes')
print('Runtime package metadata:',{n:m.version(n) for n in ['sqlmesh','duckdb','jsonschema']})
print('Scope: document/type checks only. No native pipeline behavior proven.')
