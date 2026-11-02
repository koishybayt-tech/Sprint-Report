import json,re,sys
SN=json.load(open('sn.json'));M=json.load(open('../abs/tg_map.json'))
POS=json.load(open('../run4/up.json'))['POS'];TL=set(json.load(open('../run4/up.json'))['TL'])
def who(s):
    v=M.get(s) or {}
    return v.get('jira') or v.get('fio') or s
def cat(r):
    r=r.lower()
    if re.search(r'универ|учеб|учёб',r):return 'study'
    if re.search(r'врач|самочув|здоров|больн|операц',r):return 'health'
    if re.search(r'личн|семей',r):return 'personal'
    return ''
def pdate(s,dflt):
    m=re.search(r'(\d{1,2})\.(\d{1,2})',s or '')
    if m and 1<=int(m.group(2))<=12:return '2026-%02d-%02d'%(int(m.group(2)),int(m.group(1)))
    return dflt
ev=[];upd=[]
for line in open('raw.txt'):
    line=line.rstrip('\n')
    if not line:continue
    id,d,post,si,typ,rs,pl,otr,rsn,ag=line.split('~')
    date='2026-%s-%s'%(d[:2],d[2:]);post=post[:2]+':'+post[2:]
    s=SN[int(si)];n=who(s)
    st=cat(rsn) or ('study' if 'расписан' in otr.lower() else '')
    base=dict(id=int(id),date=date,post=post,n=n,otr=otr,rsn=rsn,ag=ag,c=st)
    if typ=='U':upd.append(dict(id=int(id),date=date,post=post,n=n));continue
    if typ in 'SV':
        ev.append({**base,'t':'sick' if typ=='S' else 'vac','m':0,'txt':pl});continue
    if not rs:
        m=re.match(r'^(\d{1,2}):(\d{2})$',pl.strip())
        if m:
            a=int(m.group(1))*60+int(m.group(2));ev.append({**base,'t':'early','m':max(0,1080-a),'a':a,'b':1080});continue
        ev.append({**base,'date':pdate(pl,date),'t':'day','m':480,'txt':pl});continue
    for r in rs.split(','):
        a,b=map(int,r.split('-'))
        if a<=545 and b>=1075:t='day';mm=480
        elif a<=545:t='late';mm=b-540
        elif b>=1075:t='early';mm=1080-a
        else:t='mid';mm=b-a
        ev.append({**base,'t':t,'m':mm,'a':a,'b':b})
# arrival check: upd after declared late end
for u in upd:
    h,mi=map(int,u['post'].split(':'));pm=h*60+mi
    for e in ev:
        if pm<=720 and e['n']==u['n'] and e['date']==u['date'] and e['t']=='late' and e['id']<u['id']:
            e['arr']=pm
offd={}
for e in ev:
    if e['t'] in ('sick','vac'):continue
    k=(e['n'],e['date']);o=offd.setdefault(k,{'m':0,'parts':[],'c':set(),'day':False})
    o['m']+=e['m'];o['day']|=e['t']=='day'
    if 'a' in e:o['parts'].append('%d:%02d–%d:%02d'%(e['a']//60,e['a']%60,e['b']//60,e['b']%60))
    if e['c']=='study':o['c'].add('учёба')
    elif e['rsn']:o['c'].add(e['rsn'])
off=[]
for (n,d),o in offd.items():
    if o['day'] or o['m']>=120:
        h=('весь день' if o['day'] else ', '.join(o['parts'])+' ('+(('%d ч'%(o['m']//60))+(' %d мин'%(o['m']%60) if o['m']%60 else ''))+')')
        off.append([n,'off',d,d,'Telegram','Отпросился: '+h+(' · '+', '.join(sorted(o['c'])) if o['c'] else '')])
json.dump(off,open('off.json','w'),ensure_ascii=False)
import datetime as dt
data=open(sys.argv[2] if len(sys.argv)>2 else '../'+json.load(open('../inc/state.json'))['data']).read()
def js(name):
    m=re.search(r'^const '+name+r'=(.*?);?$',data,re.M);return json.loads(m.group(1).rstrip(';'))
PJ={r[0]:r for r in js('PJ')};AX=js('AX')
try:
    POS.update(js('POS'))
except Exception:pass
jira={}
for r in AX:
    o=r[3]+r[4]+r[6]
    if r[1] in PJ:jira.setdefault(r[0],[]).append([r[1],PJ[r[1]][5],o,PJ[r[1]][2],r[2],r[3],r[4],r[6],0])
BLK={}
for x in js('X'):
    if x[2]=='Blocked':BLK[(x[3],x[0].split('-')[0])]=BLK.get((x[3],x[0].split('-')[0]),0)+1
for n,rows in jira.items():
    for r in rows:
        b=BLK.get((n,r[0]),0);r[8]=b;r[2]+=b
UL=js('U')
def otrd(e):
    t=e['otr'].lower();d0=dt.date.fromisoformat(e['date'])
    if 'завтра' in t:return str(d0+dt.timedelta(days=1))
    if 'суббот' in t:return str(d0+dt.timedelta(days=(5-d0.weekday())%7 or 7))
    m=re.search(r'(\d{1,2})\.(\d{1,2})',t)
    if m and 1<=int(m.group(2))<=12:
        d=dt.date(2026,int(m.group(2)),int(m.group(1)))
        if d>=d0:return str(d)
    return None
for e in ev:
    od=otrd(e)
    if od:e['od']=od
ppl={}
for e in ev:ppl.setdefault(e['n'],{'pos':POS.get(e['n'],''),'tl':e['n'] in TL,'jira':e['n'] in POS or e['n'] in jira or any(v.get('jira')==e['n'] for v in M.values()),'pr':jira.get(e['n'],[]),'login':UL.get(e['n'],'')})
absd=open('../abs/abs.js').read();ABS=json.loads(re.search(r'const ABS=(.*?);\n',absd).group(1))
ABS=[a for a in ABS if a[1] in ('vac','bs','sick')]
SF=json.load(open('staff.json'));coll=[n for n in SF['staff'] if n not in SF['lead']]
left=set(SF.get('left',[]))
for n in set([r[0] for r in AX if r[0]!='— не назначен'])|set(a[0] for a in ABS)|set(SF['staff']):
    if n in ppl or n in left:continue
    ppl[n]={'pos':POS.get(n,''),'tl':n in TL,'jira':n in POS or n in jira,'pr':jira.get(n,[]),'login':UL.get(n,'')}
ABS=[a for a in ABS if a[0] not in left]
out=dict(ev=ev,ppl=ppl,abs=ABS,absAt=re.search(r'const ABSAT=(.*?);',absd).group(1),coll=coll,lead=SF['lead'],cpos={n:POS.get(n,'') for n in coll},from_=min(e['date'] for e in ev),to=max(e['date'] for e in ev),stamp=sys.argv[1] if len(sys.argv)>1 else '')
open('att.js','w').write('const ATT='+json.dumps(out,ensure_ascii=False,separators=(',',':'))+';')
from collections import Counter
print(len(ev),Counter(e['t'] for e in ev),len(ppl),sum(1 for e in ev if 'arr' in e))
late=[e for e in ev if 'arr' in e and e['arr']>e['b']+10];print('late vs declared',len(late),[(e['n'],e['date'],e['b'],e['arr']) for e in late][:8])
