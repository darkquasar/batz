from pathlib import Path
import os,subprocess,json,hashlib
job=Path('/private/tmp/batz-s2-template-writer'); root=job/'baseline-checkout'
lead=Path('/Users/martinpopolito/GitHub/batz'); evidence=lead/'docs/contributing/evidence/s2/template-rejection'
record=json.loads((job/'prepared-baseline.json').read_text())
for name,row in record['files'].items():
 assert hashlib.sha256((root/name).read_bytes()).hexdigest()==row['sha256'],name
env=os.environ.copy(); env.update(GOTOOLCHAIN='local',GOMODCACHE=str(lead/'runtime/.local/gomod'),GOCACHE=str(lead/'runtime/.local/gocache'),GOPATH=str(lead/'runtime/.local/gopath'),GOPROXY='off',GOSUMDB='off',GOFLAGS='-mod=readonly')
cmd=[str(root/'runtime/.local/go/bin/go'),'test','./internal/app','./internal/run','-run','^(TestPreflightEmitsStructuredDiagnostic|TestPreviewBudgetIncludesWritableSiblingPaths)$','-p','1','-count=1','-json']
r=subprocess.run(cmd,cwd=root,env=env,capture_output=True,text=True)
(evidence/'baseline-extra.log').write_text(r.stdout+r.stderr)
failures=[]
for line in r.stdout.splitlines():
 item=json.loads(line)
 if item.get('Action')=='fail': failures.append({k:item[k] for k in ('Package','Test') if k in item})
result={'root':str(root),'base':'exact prepared source snapshot','command':cmd,'exit':r.returncode,'verified_original_files':len(record['files']),'failures':failures}
(evidence/'baseline-extra.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
raise SystemExit(r.returncode)
