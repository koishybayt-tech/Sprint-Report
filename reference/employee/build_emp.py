# Build employee profiles: python3 build_emp.py jira.json tg_hist.txt stamp > ../emp/emp.js
import json,re,sys,statistics as st,datetime as dt
from collections import defaultdict
J=json.load(open(sys.argv[1],encoding='utf-8'));I=J['I'] if isinstance(J,dict) else J
STAMP=sys.argv[3] if len(sys.argv)>3 else ''
ROOT='/home/claude/jira/'
up=json.load(open(ROOT+'run4/up.json'));POS=up['POS'];TL=set(up['TL'])
try:
    d=open(ROOT+json.load(open(ROOT+'inc/state.json'))['data']).read()
    m=re.search(r'^const POS=(.*?);?$',d,re.M);POS.update(json.loads(m.group(1).rstrip(';')))
    LOGIN=json.loads(re.search(r'^const U=(.*?);?$',d,re.M).group(1).rstrip(';'))
except Exception as e:LOGIN={}
SF=json.load(open(ROOT+'att/staff.json'));LEAD=set(SF['lead'])
MON=['2026-%02d'%i for i in range(1,13)]
def mon(s):return s[:7] if s and s.startswith('2026') else None
def ddays(a,b):return (dt.datetime.fromisoformat(b)-dt.datetime.fromisoformat(a)).total_seconds()/86400
TODO={'Сделать','To Do','Open','Backlog','Бэклог','Открыт'}
P=defaultdict(lambda:{'m':defaultdict(lambda:defaultdict(float)),'cyc':defaultdict(list),'tasks':[],'proj':defaultdict(int)})
def asg_at(row,t):
    ch=[c for c in row[18] if c[0]!='TRUNC' and c[1]=='a']
    prev=None
    for c in ch:
        if c[0]<=t:prev=c[3]
        else:
            return prev if prev is not None else c[2]
    return prev if prev is not None else row[5]
for r in I:
    key,summ,typ,stt,cat,asg,rep,cr,res,upd,due,ts,est,pri,sps,tester,epic,parent,ch=r
    pk=key.split('-')[0]
    chs=[c for c in ch if c[0]!='TRUNC' and c[1]=='s']
    bug=typ=='Ошибка'
    rw=[c for c in chs if c[3]=='Need Rework']
    start=next((c[0] for c in chs if c[2] in TODO and c[3] not in TODO),None) or next((c[0] for c in chs if c[3]=='В работе'),None)
    done=cat=='done' and res
    cyc=round(ddays(start,res),1) if done and start and res>start else None
    nsp=len(sps)
    if asg:
        p=P[asg];p['proj'][pk]+=1
        p['tasks'].append([key,summ[:90],typ,stt,cr[:10],(res or '')[:10],nsp,len(rw),cyc,round(ts/3600,1),pk])
        if done and mon(res):
            mm=p['m'][mon(res)];mm['done']+=1;mm['bug']+=bug;mm['h']+=ts/3600;mm['carry']+=nsp>1
            if cyc is not None:p['cyc'][mon(res)].append(cyc)
        if not done:
            p['open_'+('todo' if stt in TODO else 'wip')]=p.get('open_'+('todo' if stt in TODO else 'wip'),0)+1
        # sprint plan: closed sprints of 2026
        for s in sps:
            sid,sn,ss,se,sst=s
            if sst=='CLOSED' and se and se.startswith('2026'):
                mm=p['m'][se[:7]];mm['plan']+=1
                if done and res[:10]<=(dt.date.fromisoformat(se)+dt.timedelta(days=2)).isoformat() and res[:10]>=(ss or '2000'):mm['planDone']+=1
    for c in rw:
        who=asg_at(r,c[0])
        if who and mon(c[0]):P[who]['m'][mon(c[0])]['rw']+=1
    if rep and mon(cr):P[rep]['m'][mon(cr)]['rep']+=1
    if tester and done and mon(res):P[tester]['m'][mon(res)]['qa']+=1
# attendance
M=json.load(open(ROOT+'abs/tg_map.json'))
TR=str.maketrans('құүәіөңғһёҚҰҮӘІӨҢҒҺЁ','куyаиоңгхеКУYАИОНГХЕ')
def norm(x):return ' '.join(sorted(re.sub(r'[^a-zа-я ]','',x.lower().translate(str.maketrans('құүәіөңғһё','куyаиоңгхе')).replace('y','у').replace('ң','н')).split()))
JN={}
for nm in list(POS)+list(LOGIN)+SF['staff']:JN.setdefault(norm(nm),nm)
for v in M.values():
    if v.get('fio') and v.get('jira'):JN.setdefault(norm(v['fio']),v['jira'])
def lev(a,b):
    if abs(len(a)-len(b))>2:return 9
    p=list(range(len(b)+1))
    for i,x in enumerate(a,1):
        c=[i]
        for j,y in enumerate(b,1):c.append(min(p[j]+1,c[j-1]+1,p[j-1]+(x!=y)))
        p=c
    return p[-1]
for nm in list(P):JN.setdefault(norm(nm),nm)
JT=[(k.split(),v) for k,v in JN.items()]
def fio_match(f):
    t=norm(f).split()
    if not t:return None
    k=' '.join(t)
    if k in JN:return JN[k]
    best=None
    for toks,v in JT:
        if len(toks)<2:continue
        # each Jira token must match some message token (exact, prefix>=4, or edit distance<=1)
        ok=all(any(a==b or (len(a)>=4 and (b.startswith(a) or a.startswith(b))) or lev(a,b)<=1 for b in t) for a in toks[:2])
        if ok:best=v;break
    return best
CAN={}
def who(s):
    snd=None
    if s.startswith('FIO:'):
        f,_,snd=s[4:].partition('|')
        m=fio_match(f)
        if m:return m
        v=M.get(snd) or {}
        if v.get('jira'):return v['jira']
        if not re.search('[а-яА-ЯәіңғүұқөһӘІҢҒҮҰҚӨҺ]',f):return None
        k=' '.join(norm(f).split()[:2]);return CAN.setdefault(k,' '.join(w.capitalize() for w in f.split()[:2]))
    v=M.get(s)
    if not v:return None
    return v.get('jira') or v.get('fio') or s
SN=json.load(open(ROOT+'att/sn.json'))
rows=[]
if len(sys.argv)>2 and sys.argv[2]!='-':
    for l in open(sys.argv[2],encoding='utf-8'):
        l=l.rstrip('\n')
        f=l.split('~')
        if len(f)>=10:
            if len(f)>10 and f[10]:f[3]='FIO:'+f[10]+'|'+f[3]
            rows.append(f[:10])
seen={r[0] for r in rows}
for l in open(ROOT+'att/raw.txt'):
    l=l.rstrip('\n')
    if not l:continue
    f=l.split('~')
    if f[0] in seen:continue
    f[3]=SN[int(f[3])];rows.append(f)
A=defaultdict(lambda:defaultdict(lambda:defaultdict(float)));ALOG=defaultdict(list)
for id_,d,post,s,typ,rs,pl,otr,rsn,ag in rows:
    n=who(s)
    if not n:continue
    date='2026-%s-%s'%(d[:2],d[2:]);mo=date[:7]
    study=bool(re.search('универ|учеб|учёб',rsn,re.I)) or 'расписан' in otr.lower()
    if typ=='U':continue
    a=A[n][mo]
    if typ=='S':a['sick']+=1;ALOG[n].append([date,'Больничный',0,pl[:40]]);continue
    if typ=='V':a['vacmsg']+=1;continue
    if not rs:
        m=re.match(r'^(\d{1,2}):(\d{2})$',pl.strip())
        if m:
            x=int(m.group(1))*60+int(m.group(2));a['early']+=1;a['min']+=max(0,1080-x);ALOG[n].append([date,'Ушёл раньше',max(0,1080-x),rsn]);continue
        a['day']+=1;a['min']+=480;ALOG[n].append([date,'Весь день',480,(rsn or pl)[:40]]);continue
    for r_ in rs.split(','):
        x,y=map(int,r_.split('-'))
        if x<=545 and y>=1075:t,mm='day',480
        elif x<=545:t,mm='late',y-540
        elif y>=1075:t,mm='early',1080-x
        else:t,mm='mid',y-x
        if t=='late' and study:a['lateStudy']+=1
        a[t]+=1;a['min']+=mm
        ALOG[n].append([date,{'late':'Опоздание','early':'Ушёл раньше','mid':'Отлучка','day':'Весь день'}[t],mm,('%d:%02d–%d:%02d'%(x//60,x%60,y//60,y%60))+(' · '+rsn if rsn else '')])
# Bitrix leaves (current file)
names=sorted(set(P)|set(A))
out={'stamp':STAMP,'mon':[m for m in MON if m<=STAMP[:7] or not STAMP],'att_from':min((r[1] for r in rows),default=''),'ppl':{}}
for n in names:
    if not n or n=='— не назначен' or not re.search('[а-яА-ЯәіңғүұқөһӘІҢҒҮҰҚӨҺ]',n):continue
    p=P.get(n);ms={}
    for mo in out['mon']:
        j=p['m'].get(mo,{}) if p else {}
        a=A[n].get(mo,{})
        cy=p['cyc'].get(mo,[]) if p else []
        ms[mo]=[int(j.get('done',0)),int(j.get('bug',0)),round(j.get('h',0),1),int(j.get('carry',0)),int(j.get('plan',0)),int(j.get('planDone',0)),int(j.get('rw',0)),round(st.median(cy),1) if cy else None,int(j.get('rep',0)),int(j.get('qa',0)),
                int(a.get('late',0)),int(a.get('early',0)),int(a.get('mid',0)),int(a.get('day',0)),int(a.get('sick',0)),int(a.get('min',0)),int(a.get('lateStudy',0))]
    tasks=sorted(p['tasks'],key=lambda t:(t[5] or '9999',t[4]),reverse=True) if p else []
    import hashlib
    out['ppl'][n]={'xid':(LOGIN.get(n) if (LOGIN.get(n) or '').isascii() and LOGIN.get(n) else 'p'+hashlib.md5(n.encode()).hexdigest()[:10]).replace('.','_'),'pos':POS.get(n,''),'tl':n in TL,'lead':n in LEAD,'login':LOGIN.get(n,''),'proj':dict(sorted(p['proj'].items(),key=lambda x:-x[1])) if p else {},'open':[p.get('open_todo',0),p.get('open_wip',0)] if p else [0,0],'m':ms,'t':tasks,'alog':sorted(ALOG.get(n,[]),reverse=True)}
s=json.dumps(out,ensure_ascii=False,separators=(',',':'))
sys.stdout.write('const EMP='+s+';')
print(len(out['ppl']),len(s),file=sys.stderr)
