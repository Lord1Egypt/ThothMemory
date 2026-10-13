import os,subprocess,json,re,pathlib,sys,hashlib,time
root=pathlib.Path(__file__).resolve().parents[2];os.chdir(root)
assert os.environ.get('COMPOSE_PROJECT_NAME','').startswith('thoth-audit-'), 'Unique audit Compose project required'
assert not os.environ.get('THOTH_DATABASE'), 'Unset THOTH_DATABASE override to test .env activation'
sys.path.insert(0,str(root/'scripts/audit'));import rpc
run_id=__import__('uuid').uuid4().hex

cli=os.environ.get('THOTH_AUDIT_CLI','./bin/thothmemory')
def run(a,**kw):return subprocess.check_output(a,text=True,**kw)
def sql(db,q):return run(['docker','compose','exec','-T','db','psql','-U','postgres','-d',db,'-Atc',q]).strip()
old=json.loads(run(['docker','compose','config','--format','json']))['services']['app']['environment']['THOTH_DB_NAME']
tables=['projects','principals','agents','sessions','tasks','task_dependencies','evidence','attestations','events','records','operations','schema_migrations']
def snapshot(db):
 return {t:sql(db,"SELECT count(*)||':'||md5(coalesce(string_agg(v::text, E'\\n' ORDER BY v::text),'')) FROM (SELECT to_jsonb(t)-'search_vector'-'search_text'-'search_simple' AS v FROM "+t+" t) s") for t in tables}
(root/'.validation').mkdir(exist_ok=True)
p=root/('.validation/recovery-'+run_id+'.dump')
run([cli,'backup',str(p)])
original=snapshot(old)
out=run([cli,'restore',str(p),'--confirm']);new=re.search(r'Restored into (\w+)',out)[1]
assert snapshot(new)==original
before_dbs=sql('postgres','SELECT count(*) FROM pg_database')
bad=p.with_name('corrupt-'+run_id+'.dump');bad.write_bytes(p.read_bytes()+b'corruption');bad.with_suffix(bad.suffix+'.sha256').write_text(p.with_suffix(p.suffix+'.sha256').read_text())
fail=subprocess.run([cli,'restore',str(bad),'--confirm'],capture_output=True,text=True)
assert fail.returncode!=0 and 'integrity check failed' in fail.stderr
assert sql('postgres','SELECT count(*) FROM pg_database')==before_dbs
(root/'.env').write_text(re.sub(r'^THOTH_DATABASE=.+$', 'THOTH_DATABASE='+new,(root/'.env').read_text(),flags=re.M))
run(['docker','compose','up','-d','--wait','--wait-timeout','120'])
projects=rpc.tool('list_projects',{})
# Preserve all records from original and establish authenticated reads after activation.
assert projects
sentinel='SIMULATED recovery activation sentinel '+run_id
pid=projects[0]['id'] if isinstance(projects,list) else projects['projects'][0]['id']
rpc.tool('append_event',{'project_id':pid,'operation_id':rpc.op(),'kind':'checkpoint','body':sentinel})
assert sentinel in sql(new,"SELECT body FROM events WHERE body LIKE '%"+sentinel+"'")
assert sentinel not in sql(old,"SELECT body FROM events WHERE body LIKE '%"+sentinel+"'")
newarchive=p.with_name('activated-'+run_id+'.dump');run([cli,'backup',str(newarchive)])
out2=run([cli,'restore',str(newarchive),'--confirm']);new2=re.search(r'Restored into (\w+)',out2)[1]
assert sentinel in sql(new2,"SELECT body FROM events WHERE body LIKE '%"+sentinel+"'")
assert snapshot(old)==original
result={'source_database_retained':True,'all_table_counts_and_content_hashes_equal':True,'tables_compared':tables,'corrupt_archive_rejected_before_create':True,'activated_mcp_authenticated_read':True,'activated_mcp_write':True,'second_backup_contains_activated_sentinel':True,'original_unchanged':True,'source_counts':{k:int(v.split(':')[0]) for k,v in original.items()}}
(root/'.validation/recovery-results.json').write_text(json.dumps(result,indent=2));print(json.dumps(result,indent=2))
