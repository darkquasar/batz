"""Bounded native limit probes; each child touches at most 128 MiB then exits."""
import json
import platform
import subprocess
import sys

results = []
for name in ("RLIMIT_AS", "RLIMIT_DATA", "RLIMIT_RSS"):
    program = '''import json,resource,sys
name=sys.argv[1]
limit=64*1024*1024
try:
 resource.setrlimit(getattr(resource,name),(limit,limit))
except (ValueError,OSError) as exc:
 print(json.dumps({"limit":name,"limit_bytes":limit,"set_result":"rejected","error":str(exc)}))
else:
 try:
  data=bytearray(128*1024*1024)
  for i in range(0,len(data),4096):data[i]=1
  print(json.dumps({"limit":name,"limit_bytes":limit,"set_result":"accepted","touched_bytes":len(data),"allocation":"succeeded","maxrss":resource.getrusage(resource.RUSAGE_SELF).ru_maxrss}))
 except MemoryError:
  print(json.dumps({"limit":name,"limit_bytes":limit,"set_result":"accepted","allocation":"refused"}))
'''
    child = subprocess.run([sys.executable, "-I", "-c", program, name], capture_output=True, text=True, timeout=10)
    results.append({"limit": name, "exit": child.returncode, "stdout": child.stdout, "stderr": child.stderr})
print(json.dumps({"platform": platform.platform(), "python": sys.version, "results": results}, indent=2))
