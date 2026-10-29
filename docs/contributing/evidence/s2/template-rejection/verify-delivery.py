from pathlib import Path
import subprocess, os, sys, json, hashlib
job=Path('/private/tmp/batz-s2-template-writer')
root=Path('/Users/martinpopolito/GitHub/batz')
evidence=root/'docs/contributing/evidence/s2/template-rejection'
evidence.mkdir(exist_ok=True)
env=os.environ.copy()
env.update(GOTOOLCHAIN='local',GOMODCACHE=str(root/'runtime/.local/gomod'),GOCACHE=str(root/'runtime/.local/gocache'),GOPATH=str(root/'runtime/.local/gopath'),GOPROXY='off',GOSUMDB='off',GOFLAGS='-mod=readonly')
mode=sys.argv[1]
if mode=='staticcheck':
 env['PATH']=str(root/'runtime/.local/go/bin')+os.pathsep+env.get('PATH','')
go=str(root/'runtime/.local/go/bin/go')
commands={
 'baseline':[go,'test','./internal/app','-run','^(TestInspectionOperationalFailureIsNotIntegrityFailure|TestExplainUsesContextSelectedNormalizationProject)$','-count=1','-json'],
 'aggregate':[go,'test','./...','-p','1','-count=1','-json'],
 'vet':[go,'vet','./...'],
 'build':[go,'build','-o',str(root/'runtime/.local/batz'),'./cmd/batz'],
 'staticcheck':['/Users/martinpopolito/go/bin/staticcheck','./internal/app'],
}
cmd=commands[mode]
paths=[str(p.relative_to(root)) for p in root.rglob('*.go') if '/runtime/.local/' not in str(p) and '/.git/' not in str(p)]
paths+=['go.mod','go.sum','runtime/dependencies.lock.json','runtime/sqlmesh/runner.py','runtime/sqlmesh/inspect_preview.py','normalization/config.yaml','docs/usage/explain.md']
paths+=['normalization/models/'+p.name for p in (root/'normalization/models').glob('*.sql')]
hashes={p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in sorted(set(paths))}
result=subprocess.run(cmd,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
output=result.stdout
(evidence/(mode+'.log')).write_text(output)
if any(hashlib.sha256((root/p).read_bytes()).hexdigest()!=want for p,want in hashes.items()):
 raise SystemExit('Source changed during check')
summary={'root':str(root),'base':None,'head':None,'command':cmd,'exit':result.returncode,'hashes':hashes,'environment':{k:env[k] for k in ('GOTOOLCHAIN','GOMODCACHE','GOCACHE','GOPATH','GOPROXY','GOSUMDB','GOFLAGS')},'events':{},'failures':[],'skipped':[]}
if '-json' in cmd:
 for line in output.splitlines():
  try: item=json.loads(line)
  except ValueError: continue
  action=item.get('Action'); summary['events'][action]=summary['events'].get(action,0)+1
  if action=='fail': summary['failures'].append({k:item[k] for k in ('Package','Test','Elapsed') if k in item})
  if action=='skip': summary['skipped'].append({k:item[k] for k in ('Package','Test') if k in item})
else:
 print(output)
(evidence/(mode+'.json')).write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps({k:summary[k] for k in ('command','exit','events','failures','skipped')},indent=2))
raise SystemExit(result.returncode)
