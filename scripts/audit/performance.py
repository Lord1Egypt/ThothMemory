from pathlib import Path
exec(Path(__file__).with_name('rpc.py').read_text())
import subprocess,time,statistics,concurrent.futures,sys,os
project_name=os.environ.get('COMPOSE_PROJECT_NAME','')
if not project_name.startswith('thoth-audit-'):raise RuntimeError('A unique thoth-audit-* Compose project is required')
compose=['docker','compose','-p',project_name]
head='a'*40
mode=sys.argv[1]
resolved=json.loads(subprocess.check_output(compose+['config','--format','json'],text=True))
active_db=resolved['services']['app']['environment']['THOTH_DB_NAME']
state=ROOT/'.validation/audit-performance-state.json'

def measure(fn,n=60):
 values=[]
 for i in range(n):
  st=time.perf_counter();fn(i);values.append((time.perf_counter()-st)*1000)
 return {'n':n,'median_ms':round(statistics.median(values),3),'p95_ms':round(sorted(values)[int(n*.95)-1],3),'max_ms':round(max(values),3)}

if mode=='seed':
 projects={}
 for label,size in [('empty',0),('small',100),('moderate',2500)]:
  pid=tool('create_project',{'name':'SIMULATED controlled performance '+label,'repository':'https://github.com/thothmemory-example/audit-performance-'+label})['id'];projects[label]=pid
  for i in range(size):
   tool('record_observation',{'project_id':pid,'operation_id':'audit-perf-seed-'+str(i),'title':'Migration investigation '+str(i),'body':'PostgreSQL durable checkpoint evidence. المشكلة اتحلت مع اختبار الاتصال. HandleCheckpointHTTP internal/store/operations.go rollback failure SQLSTATE 23505. '+('bounded representative engineering context '*8),'git_revision':head})
  print(label,size,flush=True)
 state.write_text(json.dumps(projects));sys.exit()
projects=json.loads(state.read_text());result={'label':mode,'dataset_records':{'empty':0,'small':100,'moderate':2500},'workloads':{}}
for label,pid in projects.items():
 result['workloads'][label]={
 'context':measure(lambda _:tool('get_project_context',{'project_id':pid,'git_revision':head})),
 'search':measure(lambda _:tool('search',{'project_id':pid,'query':'PostgreSQL checkpoint','limit':20})),
 'write':measure(lambda i:tool('append_event',{'project_id':pid,'operation_id':mode+'-write-'+str(i),'body':'Controlled concurrent checkpoint','git_revision':head}))}
start=time.perf_counter()
with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
 list(pool.map(lambda i:tool('append_event',{'project_id':projects['moderate'],'operation_id':mode+'-concurrent-'+str(i),'body':'Eight independent HTTP writer requests'}),range(120)))
result['concurrent_120_events_ms']=round((time.perf_counter()-start)*1000,3)
ids=[subprocess.check_output(compose+['ps','-q',x],text=True).strip() for x in ['app','db']]
resource=[]
for cid in ids:
 top=subprocess.check_output(['docker','top',cid,'-eo','pid,rss,nlwp,comm'],text=True)
 resource.append({'role':'app' if cid==ids[0] else 'db','top':top})

# Read proc through a disposable helper in each container's PID namespace. No Docker socket mount.
for cid,r in zip(ids,resource):
 out=subprocess.check_output(['docker','run','--rm','--pid=container:'+cid,'--user','0','--cap-add','SYS_PTRACE','--entrypoint','sh','postgres:18-alpine','-c',"cat /proc/1/status; echo FD_COUNT; ls /proc/1/fd | wc -l"],text=True)
 r['proc1']=out
samples=[]
for i in range(12):
 samples.append([json.loads(x) for x in subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}']+ids,text=True).splitlines()]);time.sleep(1)
result['idle_samples']=samples;result['processes']=resource
sql="SELECT json_build_object('database_bytes',pg_database_size(current_database()),'connections',(SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()),'events',(SELECT count(*) FROM events),'records',(SELECT count(*) FROM records))"
result['database']=json.loads(subprocess.check_output(compose+['exec','-T','db','psql','-U','postgres','-d',active_db,'-Atc',sql],text=True))
start=time.perf_counter();subprocess.run(compose+['restart','app'],check=True,stdout=subprocess.DEVNULL)
while True:
 try:
  with urllib.request.urlopen(cfg['endpoint'].replace('/mcp','/healthz'),timeout=1) as r:
   if r.status==200:break
 except Exception:
  if time.perf_counter()-start>60:raise
  time.sleep(.1)
result['restart_ready_ms']=round((time.perf_counter()-start)*1000,3)
print(json.dumps(result,ensure_ascii=False,indent=2))
