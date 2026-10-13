import json,pathlib,urllib.request,uuid
ROOT=pathlib.Path(__file__).resolve().parents[2]
cfg=json.loads((ROOT/'client.json').read_text());token=(ROOT/cfg['token_file']).read_text().strip();seq=0
from urllib.parse import urlparse
endpoint=urlparse(cfg["endpoint"])
if endpoint.hostname not in {"127.0.0.1","localhost","::1"} or endpoint.port==7319:
 raise RuntimeError("Audit refuses default production port or a nonlocal endpoint")

def rpc(method,params,auth=token):
 global seq
 seq+=1
 req=urllib.request.Request(cfg['endpoint'],json.dumps({'jsonrpc':'2.0','id':seq,'method':method,'params':params}).encode(),{'Content-Type':'application/json','Accept':'application/json, text/event-stream','Authorization':'Bearer '+auth})
 with urllib.request.urlopen(req,timeout=20) as r: v=json.load(r)
 if 'error' in v:raise RuntimeError(v['error'])
 return v['result']

def tool(name,args,auth=token):
 r=rpc('tools/call',{'name':'memory_'+name,'arguments':args},auth)
 if r.get('isError'):raise RuntimeError(r['content'])
 return json.loads(r['content'][0]['text'])['data']

def op():return str(uuid.uuid4())
rpc('initialize',{'protocolVersion':'2025-11-25','capabilities':{},'clientInfo':{'name':'isolated-preproduction-audit','version':'1'}})
