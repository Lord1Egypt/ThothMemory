from pathlib import Path
exec(Path(__file__).with_name('rpc.py').read_text())
import time,statistics,sys
head='9a3f8c1275e4b60d21c9a813f640de7a25bc901e'
p=tool('create_project',{'name':'SIMULATED multilingual audit','repository':'https://github.com/thothmemory-example/audit-search'})['id']
fixtures=[('English','Running migrations safely prevents failed transactions.'),('Egyptian Arabic','المشكلة اتحلت لما غيرنا إعدادات الاتصال بقاعدة البيانات'),('Formal Arabic','فشل التحقق بسبب تعارض المراجعات وحماية البيانات'),('Mixed','إصلاح PostgreSQL transaction rollback مع اختبار الاتصال'),('Symbols','internal/store/operations.go HandleCheckpointHTTP ErrStaleRevision'),('Error','ERROR: duplicate key value violates unique constraint tasks_pkey SQLSTATE 23505'),('Unicode','café checkpoint'),('Similar','Repeated migration rollback observation')]
ids={}
for title,body in fixtures:
 r=tool('record_observation',{'project_id':p,'operation_id':'audit-search-'+title,'title':title,'body':body,'git_revision':head});ids[title]=r['id']
cases=[('English stem','migrate','English'),('Egyptian Arabic','اتحلت','Egyptian Arabic'),('Formal Arabic','تعارض','Formal Arabic'),('Mixed AND','PostgreSQL اختبار','Mixed'),('Technical partial','Checkpoint','Symbols'),('Body partial Arabic','إعداد','Egyptian Arabic'),('File path','internal/store/operations.go','Symbols'),('Error code','23505','Error'),('Commit prefix',head[:12],'Symbols'),('Unicode decomposed','cafe\u0301','Unicode')]
results=[]
for label,query,want in cases:
 times=[];found=[]
 for _ in range(20):
  start=time.perf_counter();found=tool('search',{'project_id':p,'query':query});times.append((time.perf_counter()-start)*1000)
 results.append({'case':label,'query':query,'expected_title':want,'retrieved_expected':any(x['id']==ids[want] for x in found),'matches':len(found),'median_ms':round(statistics.median(times),3),'p95_ms':round(sorted(times)[18],3)})
exact=tool('search',{'project_id':p,'exact_id':ids['Formal Arabic']})
print(json.dumps({'project':p,'cases':results,'exact_id_pass':len(exact)==1 and exact[0]['id']==ids['Formal Arabic']},ensure_ascii=False,indent=2))
