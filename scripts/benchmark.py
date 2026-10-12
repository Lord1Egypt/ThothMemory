#!/usr/bin/env python3
"""Measure local Streamable HTTP calls. Requires an existing, healthy deployment."""
import json, pathlib, platform, statistics, subprocess, time, urllib.request, uuid
ROOT = pathlib.Path(__file__).resolve().parents[1]
cfg = json.loads((ROOT / 'client.json').read_text())
token = (ROOT / cfg['token_file']).read_text().strip()
url = cfg['endpoint']; counter = 0

def rpc(method, params):
    global counter
    counter += 1
    body = json.dumps({'jsonrpc':'2.0','id':counter,'method':method,'params':params}).encode()
    req = urllib.request.Request(url, body, {'Content-Type':'application/json','Accept':'application/json, text/event-stream','Authorization':'Bearer '+token})
    with urllib.request.urlopen(req, timeout=20) as r:
        data = json.load(r)
    if 'error' in data: raise RuntimeError(data['error'])
    return data['result']

def tool(name, args):
    r = rpc('tools/call', {'name':'memory_'+name,'arguments':args})
    if r.get('isError'): raise RuntimeError(r['content'])
    return json.loads(r['content'][0]['text'])['data']

def measure(fn, count=100):
    values=[]
    for i in range(count):
        start=time.perf_counter(); fn(i); values.append((time.perf_counter()-start)*1000)
    values.sort()
    return {'requests':count,'median_ms':round(statistics.median(values),3),'p95_ms':round(values[int(len(values)*.95)-1],3),'max_ms':round(max(values),3)}

head='a'*40
init=lambda _:rpc('initialize', {'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'benchmark','version':'1'}})
result={'environment':{'os':platform.system(),'architecture':platform.machine(),'docker':subprocess.check_output(['docker','version','--format','{{.Server.Version}}'],text=True).strip(),'cpu':next((x.split(':',1)[1].strip() for x in pathlib.Path('/proc/cpuinfo').read_text().splitlines() if x.startswith('model name')),'unknown')}}
result['mcp_initialize']=measure(init,20)
pid=tool('create_project', {'name':'Benchmark fixture','repository':'https://github.com/thothmemory-example/benchmark'})['id']
for i in range(200):
    tool('record_observation', {'project_id':pid,'operation_id':str(uuid.uuid4()),'title':'Checkpoint '+str(i),'body':'PostgreSQL durable engineering context with evidence provenance.','git_revision':head})
result['dataset']={'project_id':pid,'records_added':200,'fixture_only':True}
result['context']=measure(lambda _:tool('get_project_context',{'project_id':pid,'git_revision':head}))
result['event_write']=measure(lambda i:tool('append_event',{'project_id':pid,'operation_id':str(uuid.uuid4()),'body':'Measured checkpoint '+str(i)}))
result['search']=measure(lambda _:tool('search',{'project_id':pid,'query':'PostgreSQL provenance','git_revision':head,'limit':20}))
from concurrent.futures import ThreadPoolExecutor
start=time.perf_counter()
with ThreadPoolExecutor(max_workers=8) as workers:
    list(workers.map(lambda i:tool('append_event',{'project_id':pid,'operation_id':str(uuid.uuid4()),'body':'Concurrent checkpoint '+str(i)}),range(100)))
result['concurrent_event_writes']={'writers':8,'writes':100,'elapsed_ms':round((time.perf_counter()-start)*1000,3)}
time.sleep(10)  # Allow checkpoint/write work to settle before sampling idle resources.
result['idle_docker_stats']=json.loads('['+','.join(subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}','thothmemory-app-1','thothmemory-db-1'],text=True).splitlines())+']')
start=time.perf_counter();subprocess.run(['docker','compose','restart','app'],cwd=ROOT,check=True,stdout=subprocess.DEVNULL)
while True:
    try:
        with urllib.request.urlopen(url.replace('/mcp','/healthz'),timeout=1) as r:
            if r.status==200: break
    except Exception:
        if time.perf_counter()-start>60: raise
        time.sleep(.1)
result['app_restart_ready_ms']=round((time.perf_counter()-start)*1000,3)
print(json.dumps(result,indent=2))
