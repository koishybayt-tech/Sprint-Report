// JS-порт snap/sprint.py (часть 1 analyze.py + sprint.py) — выполняется во вкладке Jira над window.__JX
// window.__SNAPIN = {PJ,STAFF,EP,ABSD,ABSN}; результат -> window.__SNAPOUT (текст sprint.js)
window.__SNAPRUN=function(TODAY,HIST_END){
const IN=window.__SNAPIN,POS=__UPN.POS,TL=new Set(__UPN.TL);
const PJ=IN.PJ,STAFF=new Set(IN.STAFF),EP=IN.EP,ABSN=IN.ABSN;
const LEADP={};PJ.forEach(p=>LEADP[p[0]]=p[2]);const LEADS=new Set(Object.values(LEADP));
const WAITS=new Set(['Test','Тестирование','Ready for deploy','In Staging','In Production','Готово','Done','Закрыт','Closed','Resolved']);
const TODO=new Set(['Сделать','To Do','Open','Backlog','Бэклог','Открыт','Selected For Development','Требует уточнения']);
const HOL=new Set(['2026-07-06','2026-08-31','2026-10-26']);
const iso=d=>d.toISOString().slice(0,10);const addD=(s,n)=>{const d=new Date(s+'T00:00:00Z');d.setUTCDate(d.getUTCDate()+n);return iso(d)};
const days=(a,b)=>Math.round((new Date(b+'T00:00:00Z')-new Date(a+'T00:00:00Z'))/864e5);
const WD=[];for(let x='2026-07-01';x<=HIST_END;x=addD(x,1)){const wd=new Date(x+'T00:00:00Z').getUTCDay();if(wd>0&&wd<6&&!HOL.has(x))WD.push(x)}
const wdays=(a,b)=>WD.filter(d=>a<=d&&d<=b);
const X=__JX.map(r=>{const A=r.ch.filter(c=>c[1]=='a'),S=r.ch.filter(c=>c[1]=='s');
 const sp=r.sps.filter(s=>s[2]&&s[3]&&s[2][0]=='2'&&s[3][0]=='2').map(s=>[s[2],s[3],s[1],s[4]]);
 const fut=r.sps.some(s=>s[4]=='FUTURE'&&/\d/.test(s[1]||''));
 return {fut,key:r.key,pk:r.key.split('-')[0],typ:r.typ,stt:r.stt,sid:r.sid,cat:r.cat,asg:r.asg,rep:r.rep,cr:r.cr,res:r.cat=='done'?r.res:'',sp,nsp:r.sps.length,A,S,ch:r.ch,raw:r}});
const atF=(L,t,cur)=>{let prev;for(const c of L){if(c[0]<=t)prev=c[3];else return prev!==undefined?prev:c[2]}return prev!==undefined?prev:cur};
const asg_at=(r,t)=>atF(r.A,t,r.asg),st_at=(r,t)=>atF(r.S,t,r.stt);
const open_at=(r,t)=>r.cr<=t&&!(r.res&&r.res<=t);
const in_sprint=(r,d)=>r.sp.some(([a,b,_,s])=>a<=d&&d<=((s=='ACTIVE'&&b<HIST_END)?HIST_END:b));
const planned=(r,d)=>{const lim=addD(d,14);if(r.sp.some(([a])=>d<a&&a<=lim))return true;return d>='2026-10-02'&&r.fut};
const SET=()=>({});const add=(M,k,d)=>{(M[k]=M[k]||new Set()).add(d)};const has=(M,k,d)=>!!(M[k]&&M[k].has(d));
const has_any={},activity={};
for(const r of X){if(r.typ=='Epic')continue;
 for(const c of r.ch)add(activity,c[4],c[0].slice(0,10));if(r.rep)add(activity,r.rep,r.cr.slice(0,10));
 const lo=r.cr.slice(0,10)>WD[0]?r.cr.slice(0,10):WD[0];const hi=((r.res||'9999').slice(0,10)<HIST_END)?r.res.slice(0,10):HIST_END;
 for(const d of WD){if(d<lo)continue;if(d>hi)break;const t=d+'T18:00';if(!open_at(r,t))continue;const w=asg_at(r,t);if(!w)continue;add(has_any,w,d)}}
const absd={};for(const n in IN.ABSD)absd[n]=new Set(IN.ABSD[n]);
// CUR
const CUR={};PJ.forEach(p=>CUR[p[0]]={k:p[0],n:p[1],lead:p[2],sp:p[3],st:p[4],en:p[5],t:p[6],todo:p[7],wk:p[8],cr:p[9],rw:p[10],ts:p[11],dn:p[12],bug:p[13],carry:p[14],add:p[15],unpl:p[16],blk:p[17],stale:p[18],h:p[19],ppl:p[20],rdy:p[21],tst:p[22]});
const SPI={};for(const r of __JX){const k=r.key.split('-')[0];if(CUR[k]&&r.sps.some(s=>s[1]==CUR[k].sp&&s[4]=='ACTIVE')&&r.typ!='Epic')(SPI[k]=SPI[k]||[]).push(r.key)}
const XK={};X.forEach(r=>XK[r.key]=r);
const DONE_ST=new Set(['Готово','Done','In Production','Закрыт','Closed','Resolved']);
for(const k in CUR){const c=CUR[k];
 c.ended=c.en<TODAY;c.overdue=c.en<TODAY?days(c.en,TODAY):0;
 const ln=Math.max(1,days(c.st,c.en)),el=days(c.st,TODAY);
 c.time=Math.max(0,Math.min(100,Math.round(100*el/ln)));c.ready=c.t?Math.round(100*c.dn/c.t):0;
 const keys=SPI[k]||[];c.nI=keys.length;
 c.rwEv=keys.filter(key=>XK[key].S.some(ch=>ch[3]=='Need Rework'&&ch[0].slice(0,10)>=c.st)).length;
 let g=0;keys.forEach(key=>XK[key].A.forEach(ch=>{if(ch[0].slice(0,10)>=c.st&&ch[4]==c.lead&&ch[3]&&ch[3]!=c.lead)g++}));c.gave=g;
 const ser=[];const hi=c.en<HIST_END?c.en:HIST_END;
 for(const d of wdays(c.st,hi)){const t=d+'T18:00';const row=[d,0,0,0,0,0];
  for(const key of keys){const r=XK[key];if(r.cr>t)continue;if(r.res&&r.res<=t){row[5]++;continue}
   const s=st_at(r,t);if(DONE_ST.has(s)){row[5]++;continue}
   if(TODO.has(s)){const w=asg_at(r,t);if(w==c.lead)row[1]++;else if(!w)row[2]++;else row[3]++}else row[4]++}
  ser.push(row)}
 c.ser=ser}
// AX (открытые спринты) -> PP
const G={"10100":"todo","10011":"wk","10008":"cr","10000":"rw","10001":"dn"};
const PP={},UNAS={};
for(const r of X){if(!r.raw.sps.some(s=>s[4]=='ACTIVE'))continue;const n=r.asg||'— не назначен';const g=G[r.sid]||'ts';
 const o=(PP[n]=PP[n]||{})[r.pk]=PP[n][r.pk]||{t:0,todo:0,wk:0,cr:0,rw:0,ts:0,dn:0};o.t++;o[g]++}
const pn=PP['— не назначен']||{};for(const k in pn)UNAS[k]={t:pn[k].t,todo:pn[k].todo};delete PP['— не назначен'];
const posOf=n=>POS[n]||(EP[n]&&EP[n][0])||'';
const isPM=n=>LEADS.has(n)||n=='Рысмандай Диас'||/manager|менеджер/i.test(posOf(n));
const isQA=(n,main)=>/qa|тест|test/i.test(posOf(n))||(!posOf(n)&&main=='AQA');
const SA={};
for(const r of X){if(r.typ=='Epic'||!r.sp.length)continue;const lo=r.cr.slice(0,10)>'2026-09-14'?r.cr.slice(0,10):'2026-09-14';const hi=((r.res||'9999').slice(0,10)<HIST_END)?r.res.slice(0,10):HIST_END;
 for(const d of WD){if(d<lo)continue;if(d>hi)break;const t=d+'T18:00';if(!open_at(r,t)||!in_sprint(r,d))continue;if(WAITS.has(st_at(r,t)))continue;const w=asg_at(r,t);if(w)add(SA,w,d)}}
const PEOPLE=[];
const names=[...new Set([...Object.keys(PP),...IN.STAFF.filter(z=>EP[z])])].sort();
for(const n of names){if(!STAFF.has(n))continue;const pr=PP[n]||{};
 const pks=Object.keys(pr);const main=pks.length?pks.reduce((a,b)=>pr[b].t>pr[a].t?b:a):((EP[n]&&EP[n][1])||'');
 if(isPM(n)||isQA(n,main))continue;const c=CUR[main];
 const lo=(c&&c.st>='2026-09-14')?c.st:'2026-09-28',hi=HIST_END;
 const dd=wdays(lo,hi).filter(d=>!has(absd,n,d));if(!dd.length)continue;
 const idle=dd.filter(d=>!has(SA,n,d)),none=idle.filter(d=>!has(has_any,n,d)),quiet=idle.filter(d=>!has(activity,n,d));
 let cur=0;for(const d of dd)cur=idle.includes(d)?cur+1:0;
 const tot={t:0,todo:0,wk:0,cr:0,rw:0,ts:0,dn:0};for(const v of Object.values(pr))for(const kk in v)tot[kk]+=v[kk];
 const left=tot.todo+tot.wk+tot.rw;
 const absnow=ABSN.some(a=>a[0]==n&&a[1]<=TODAY&&TODAY<=(a[2]||a[1]));
 PEOPLE.push([n,posOf(n),main,c?c.sp:'',lo,hi,dd.length,idle.length,none.length,quiet.length,cur,tot.t,tot.dn,tot.todo,tot.wk,tot.rw,tot.ts+tot.cr,left,absnow,pks.sort()])}
// бэклог вне спринта
const byp={};for(const r of X)if(r.typ!='Epic'&&r.typ!='Подзадача')(byp[r.pk]=byp[r.pk]||[]).push(r);
const blcount=(k,d)=>{const t=d+'T18:00',L=CUR[k].lead;let tot=0,lead=0,un=0;for(const r of byp[k]||[]){if(!open_at(r,t)||in_sprint(r,d)||planned(r,d))continue;const w=asg_at(r,t);tot++;if(w==L)lead++;if(!w)un++}return [tot,lead,un]};
for(const k in CUR){const c=CUR[k];let d0=c.st>WD[0]?c.st:WD[0];d0=WD.find(d=>d>=d0)||HIST_END;c.blStart=blcount(k,d0);c.blNow=blcount(k,HIST_END)}
window.__SNAPOUT='const CUR='+JSON.stringify(CUR)+';\nconst PP='+JSON.stringify(PEOPLE)+';\nconst AXP='+JSON.stringify(PP)+';\nconst UNAS='+JSON.stringify(UNAS)+';\n';
return [PEOPLE.length,window.__SNAPOUT.length]};
'ok'
