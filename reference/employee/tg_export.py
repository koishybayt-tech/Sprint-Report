# Parse Telegram Desktop export (result.json) of Adata Group into raw rows (same format as att/raw.txt but sender as name)
# usage: python3 tg_export.py result.json > tg_hist.txt   ; row: id~MMDD~HHMM~<sender>~typ~ranges~pl~otr~rsn~ag
import json,re,sys
def tm(s):
    s=s.replace('.',':');m=re.match(r'^(\d{1,2})(?::(\d{2}))?$',s)
    return int(m.group(1))*60+int(m.group(2) or 0) if m else None
def text(t):
    if isinstance(t,str):return t
    return ''.join(x if isinstance(x,str) else x.get('text','') for x in t)
d=json.load(open(sys.argv[1],encoding='utf-8'))
msgs=d.get('messages',d)
out=[]
for m in msgs:
    if m.get('type')!='message':continue
    dt=m['date'];
    if not dt.startswith('2026'):continue
    snd=m.get('from') or ''
    tx0=text(m.get('text',''))
    lines=[x.strip() for x in tx0.split('\n') if x.strip()]
    if not lines:continue
    tx=' / '.join(lines)
    if re.match(r'^upd\.?$',lines[-1],re.I):typ='U'
    elif re.search('больнич',tx,re.I):typ='S'
    elif re.search('отпуск',tx,re.I):typ='V'
    elif re.search('пропуск',tx,re.I):typ='P'
    else:continue
    pl=next((l for l in lines if re.search('пропуск',l,re.I)),'')
    rs=[]
    if typ=='P':
        for a,b in re.findall(r'(\d{1,2}(?:[:.]\d{2})?)\s*[-–—]\s*(\d{1,2}[:.]\d{2})',pl):
            a,b=tm(a),tm(b)
            if a is not None and b is not None and a>=360 and b<=1440 and b>a:rs.append('%d-%d'%(a,b))
    otr=re.sub(r'^отработ\S*:?\s*','',next((l for l in lines if re.search('отработ|отрабат',l,re.I)),''),flags=re.I)[:30]
    rsn=next((l for l in lines if re.search('причин',l,re.I)),'универ' if re.search('универ',tx,re.I) else '')
    rsn=re.sub(r'^причин\S*:?\s*','',rsn,flags=re.I)[:30]
    ag=','.join(x[1:] for x in re.findall(r'@[\w.]+',tx))
    pl2='' if rs else re.sub(r'^пропуск:?\s*','',pl,flags=re.I)[:40]
    fio=lines[0] if re.match(r'^[A-Za-zА-Яа-яЁёӘәҒғҚқҢңӨөҰұҮүҺһІі\- ]{3,40}$',lines[0]) and not re.search('пропуск|отпуск|больнич|отработ|причин|upd',lines[0],re.I) and len(lines[0].split())in(2,3) else ''
    out.append('~'.join([str(m['id']),dt[5:7]+dt[8:10],dt[11:13]+dt[14:16],snd.replace('~',' '),typ,','.join(rs),pl2.replace('~',' '),otr.replace('~',' '),rsn.replace('~',' '),ag,fio.strip()]))
print('\n'.join(out))
print(len(out),file=sys.stderr)
