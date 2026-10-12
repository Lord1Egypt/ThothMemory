#!/usr/bin/env python3
"""Deterministic local model fixture for REAL agent CLI interoperability tests.
Never used by ThothMemory. No inference, network service or paid key required.
"""
import json, time, uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

def data(message):
    content=message.get('content','')
    if isinstance(content,list):content=''.join(x.get('text','') for x in content if isinstance(x,dict))
    try:
        obj=json.loads(content)
        if 'data' in obj:return obj['data']
        if 'content' in obj:
            for block in obj['content']:
                if block.get('type')=='text':return json.loads(block['text'])['data']
    except (ValueError,KeyError,TypeError):pass
    return None
class Handler(BaseHTTPRequestHandler):
    def log_message(self,*args):pass
    def do_GET(self):
        b=json.dumps({'object':'list','data':[{'id':'fixture','object':'model','owned_by':'local-test'}]}).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers();self.wfile.write(b)
    def do_POST(self):
        req=json.loads(self.rfile.read(int(self.headers.get('Content-Length',0))))
        names=[t['function']['name'] for t in req.get('tools',[])]
        results=[data(m) for m in req.get('messages',[]) if m.get('role')=='tool'];results=[x for x in results if x is not None]
        suffix='';args={};stage=len(results)
        if stage==0:suffix='create_project';args={'name':'Actual OpenCode client fixture','repository':'https://github.com/thothmemory-example/opencode-client'}
        elif stage==1:suffix='create_task';args={'project_id':results[0]['id'],'operation_id':str(uuid.uuid4()),'title':'Deterministic client interoperability fixture — no engineering claim'}
        elif stage==2:suffix='get_task';args={'project_id':results[0]['id'],'task_id':results[1]['id']}
        name=next((n for n in names if n.endswith('memory_'+suffix)),None) if suffix else None
        call={'id':'call_'+uuid.uuid4().hex,'type':'function','function':{'name':name,'arguments':json.dumps(args)}} if name else None
        text='Local fixture finished: actual CLI memory write and retrieval completed.'
        chunks=[{'role':'assistant','content':''}]
        if call:chunks.append({'tool_calls':[{'index':0,**call}]})
        else:chunks.append({'content':text})
        self.send_response(200);self.send_header('Content-Type','text/event-stream' if req.get('stream') else 'application/json');self.end_headers()
        if req.get('stream'):
            for delta in chunks+[{}]:
                obj={'id':'fixture','object':'chat.completion.chunk','created':int(time.time()),'model':'fixture','choices':[{'index':0,'delta':delta,'finish_reason':('tool_calls' if call else 'stop') if not delta else None}]}
                self.wfile.write(('data: '+json.dumps(obj)+'\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n');self.wfile.flush()
        else:
            self.wfile.write(json.dumps({'id':'fixture','object':'chat.completion','created':int(time.time()),'model':'fixture','choices':[{'index':0,'message':{'role':'assistant','content':None if call else text,**({'tool_calls':[call]} if call else {})},'finish_reason':'tool_calls' if call else 'stop'}],'usage':{'prompt_tokens':1,'completion_tokens':1,'total_tokens':2}}).encode())
if __name__=='__main__':
    import sys
    ThreadingHTTPServer(('127.0.0.1',int(sys.argv[1]) if len(sys.argv)>1 else 18091),Handler).serve_forever()
