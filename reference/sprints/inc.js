// Incremental Jira extractor. Inject into the jira.adata.kz tab, then call:
//   await window.__INC({mode:'inc'|'full', ck:'<checksum of old hash list>', oldHs:'<space separated, only if asked>', oldXs:'<space separated X keys>', upck:'<checksum of U+POS json>'})
// Returns {need:'oldHs'} if its remembered hash list does not match ck.
window.__INC=async function(opt){
const h=s=>{let x=2166136261;for(const c of s){x^=c.codePointAt(0);x=Math.imul(x,16777619)>>>0}return x.toString(36)};
const J=v=>JSON.stringify(v);
const get=async u=>{const r=await fetch(u);if(!r.ok)throw new Error(u.slice(0,80)+' '+r.status);return r.json()};
const search=async(jql,fields)=>{let all=[],s=0,t=0;do{const r=await get(`/rest/api/2/search?jql=${encodeURIComponent(jql)}&startAt=${s}&maxResults=1000&fields=${fields}`);t=r.total;all=all.concat(r.issues);s+=r.issues.length;if(!r.issues.length)break}while(s<t);return all};
const FIELDS='summary,status,assignee,issuetype,project,priority,created,updated,customfield_10104,customfield_11204,customfield_10902,customfield_11101,timespent';
const BASEQ='cf[10104] in openSprints()';
// --- IndexedDB ---
const db=await new Promise((res,rej)=>{const q=indexedDB.open('adataCache',1);q.onupgradeneeded=()=>{const d=q.result;d.createObjectStore('issues',{keyPath:'key'});d.createObjectStore('meta')};q.onsuccess=()=>res(q.result);q.onerror=()=>rej(q.error)});
const tx=(st,mode,fn)=>new Promise((res,rej)=>{const t=db.transaction(st,mode);const r=fn(t);t.oncomplete=()=>res(r&&r.result!==undefined?r.result:r);t.onerror=()=>rej(t.error)});
const allIssues=()=>new Promise((res,rej)=>{const r=db.transaction('issues').objectStore('issues').getAll();r.onsuccess=()=>res(r.result);r.onerror=()=>rej(r.error)});
const getMeta=k=>new Promise(res=>{const r=db.transaction('meta').objectStore('meta').get(k);r.onsuccess=()=>res(r.result);r.onerror=()=>res(undefined)});
const setMeta=(k,v)=>tx('meta','readwrite',t=>t.objectStore('meta').put(v,k));
const slim=i=>{const f=i.fields;return {key:i.key,pk:f.project.key,pn:f.project.name,sid:f.status.id,sn:f.status.name,an:f.assignee?f.assignee.displayName:null,al:f.assignee?f.assignee.name:null,it:f.issuetype.name,pr:f.priority?f.priority.name:'',cr:f.created,up:f.updated,sp:f.customfield_10104||[],pl:f.customfield_11204?f.customfield_11204.value:null,qn:f.customfield_10902?f.customfield_10902.displayName:null,ql:f.customfield_10902?f.customfield_10902.name:null,pos:f.customfield_11101||null,ts:f.timespent||0,su:(f.summary||'').slice(0,90)}};
const now=new Date();const pad=v=>String(v).padStart(2,'0');
const jqlTime=d=>`${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
let mode=opt.mode||'inc';const last=await getMeta('lastRun');if(!last)mode='full';
const stat={mode};
if(mode=='full'){
  const all=await search(BASEQ+' ORDER BY created ASC',FIELDS);
  await tx('issues','readwrite',t=>{const s=t.objectStore('issues');s.clear();all.forEach(i=>s.put(slim(i)))});
  const projs=await get('/rest/api/2/project?expand=lead');await setMeta('leads',Object.fromEntries(projs.map(p=>[p.key,p.lead?p.lead.displayName:'—'])));
  const tl=await search('assignee in membersOf("adata-team-leads") AND updated >= -180d','assignee');
  await setMeta('TL',[...new Set(tl.map(i=>i.fields.assignee&&i.fields.assignee.displayName.trim()).filter(Boolean))].sort());
  stat.fetched=all.length;
}else{
  const since=new Date(new Date(last).getTime()-5*60e3);
  const keys=await search(BASEQ,'key');const KS=new Set(keys.map(i=>i.key));
  const ch=await search(`${BASEQ} AND updated >= "${jqlTime(since)}"`,FIELDS);
  const cached=await allIssues();const CK=new Set(cached.map(c=>c.key));
  const gone=cached.filter(c=>!KS.has(c.key)).map(c=>c.key);
  const chK=new Set(ch.map(i=>i.key));const miss=[...KS].filter(k=>!CK.has(k)&&!chK.has(k));
  let extra=[];for(let i=0;i<miss.length;i+=100)extra=extra.concat(await search(`key in (${miss.slice(i,i+100).join(',')})`,FIELDS));
  await tx('issues','readwrite',t=>{const s=t.objectStore('issues');gone.forEach(k=>s.delete(k));ch.concat(extra).forEach(i=>s.put(slim(i)))});
  Object.assign(stat,{since:jqlTime(since),changed:ch.length,removed:gone.length,added:extra.length,inSprints:KS.size});
}
await setMeta('lastRun',now.toISOString());
// --- compute (same rules as the full extractor) ---
const LEADS=await getMeta('leads')||{};const TL=await getMeta('TL')||[];
const iss=(await allIssues()).sort((a,b)=>a.cr<b.cr?-1:a.cr>b.cr?1:(a.key<b.key?-1:1));
const dd=d=>Math.floor((now-new Date(d))/864e5);
const ps=x=>{if(typeof x!='string')return x;const o={};(x.match(/\[(.*)\]$/)||[,''])[1].split(',').forEach(p=>{const i=p.indexOf('=');if(i>0)o[p.slice(0,i)]=p.slice(i+1)});return o};
const G={"10100":"bl","10011":"wk","10008":"cr","10000":"rw","10001":"dn"};
const PJ={},AX={},QX={},POSc={},U={},X=[],PXo={};
iss.forEach(f=>{const k=f.pk,g=G[f.sid]||'ts',up=dd(f.up),cr=dd(f.cr);
 const sp=f.sp.map(ps),a=sp.filter(x=>x.state=='ACTIVE').pop()||{};
 const p=PJ[k]=PJ[k]||{k,n:f.pn,lead:LEADS[k]||'—',sp:a.name||'',st:(a.startDate||'').slice(0,10),en:(a.endDate||'').slice(0,10),t:0,bl:0,wk:0,cr:0,rw:0,ts:0,dn:0,bug:0,carry:0,add:0,unpl:0,blk:0,stale:0,h:0,ppl:new Set(),rdy:0,tst:0};
 const px=PXo[k]=PXo[k]||[null,-1,0];if(g!='dn'){if(cr>px[1]){px[0]=f.key;px[1]=cr}if(sp.length>1)px[2]++}
 const bug=f.it=='Ошибка';p.t++;p[g]++;if(bug)p.bug++;if(sp.length>1)p.carry++;if(a.startDate&&new Date(f.cr)>new Date(a.startDate))p.add++;
 if(f.pl=='Внеплановая')p.unpl++;if(f.sn=='Blocked')p.blk++;if(g!='dn'&&up>14)p.stale++;p.h+=f.ts/3600;
 if(/Ready for deploy|In Staging/.test(f.sn))p.rdy++;if(f.sn=='Test')p.tst++;
 const n=f.an||'— не назначен';if(f.an){p.ppl.add(n);U[n]=f.al}
 if(f.pos){const c=POSc[n]=POSc[n]||{};c[f.pos]=(c[f.pos]||0)+1}
 const x=AX[n+'|'+k]=AX[n+'|'+k]||[n,k,0,0,0,0,0,0,0,0,0,-1,-1,0,-1];const ix={bl:3,wk:4,cr:5,rw:6,ts:7,dn:8};
 x[2]++;x[ix[g]]++;if(bug)x[9]++;
 if(g=='bl'){if(up>7)x[10]++;x[13]=Math.max(x[13],cr);x[14]=x[14]<0?up:Math.min(x[14],up)}
 if(g=='wk'||g=='cr'||g=='rw')x[11]=x[11]<0?up:Math.min(x[11],up);
 if(g!='dn')x[12]=x[12]<0?up:Math.min(x[12],up);
 if(f.qn){U[f.qn]=U[f.qn]||f.ql;const q=QX[f.qn+'|'+k]=QX[f.qn+'|'+k]||[f.qn,k,0,0,0];q[2]++;if(f.sn=='Test')q[3]++;if(g=='dn')q[4]++}
 if(g!='dn'&&(f.sn=='Blocked'||f.sn=='Need Rework'||f.sn=='Требует уточнения'||up>30))X.push([f.key,f.su,f.sn,n,up,f.pr]);
});
const r1=v=>Math.round(v*10)/10;
const POS=Object.fromEntries(Object.entries(POSc).map(([n,o])=>[n,Object.entries(o).sort((a,b)=>b[1]-a[1])[0][0]]));
const rows=[];
Object.values(PJ).forEach(p=>rows.push('P'+J([p.k,p.n,p.lead,p.sp,p.st,p.en,p.t,p.bl,p.wk,p.cr,p.rw,p.ts,p.dn,p.bug,p.carry,p.add,p.unpl,p.blk,p.stale,r1(p.h),p.ppl.size,p.rdy,p.tst])));
Object.values(AX).sort((a,b)=>a[0].localeCompare(b[0])||a[1].localeCompare(b[1])).forEach(r=>rows.push('A'+J(r)));
Object.values(QX).sort((a,b)=>a[0].localeCompare(b[0])||a[1].localeCompare(b[1])).forEach(r=>rows.push('Q'+J(r)));
const oldX=new Set((opt.oldXs||'').split(/\s+/).filter(Boolean));
const XR=X.sort((a,b)=>b[4]-a[4]);
const H=[],NEW=[];
let oldSet;
const lastH=window.__LASTH||[];
if(opt.oldHs)oldSet=new Set(opt.oldHs.split(/\s+/).filter(Boolean));
else if(lastH.length&&h(lastH.join(' '))===opt.ck)oldSet=new Set(lastH);
else return {need:'oldHs',have:lastH.length?h(lastH.join(' ')):null,stat};
rows.forEach(r=>{const x=h(r);H.push(x);if(!oldSet.has(x))NEW.push(r)});
XR.forEach(x=>{const nul='X'+J([x[0],null,x[2],x[3],x[4],x[5]]);const k=h(nul);H.push(k);if(!oldSet.has(k))NEW.push(oldX.has(x[0])?nul:'X'+J(x))});
window.__LASTH=H;
const UP=J({U,POS,TL});const upck=h(UP);
const pieces=[];let cur='';NEW.forEach(r=>{if(cur&&cur.length+r.length+1>900){pieces.push(cur);cur=''}cur+=(cur?'\n':'')+r});if(cur)pieces.push(cur);
window.__PP=pieces;
const out={stat,issues:iss.length,rows:H.length,newRows:NEW.length,pieces:pieces.length,stamp:`${now.getFullYear()}-${pad(now.getMonth()+1)}-${pad(now.getDate())}T${pad(now.getHours())}:${pad(now.getMinutes())}`,hck:h(H.join(' ')),
 PX:J(Object.fromEntries(Object.entries(PXo).map(([k,v])=>[k,[v[0],Math.max(0,v[1]),v[2]]])))};
for(let i=0;i<H.length;i+=100)out['h'+String(i/100).padStart(2,'0')]=H.slice(i,i+100).join(' ');
pieces.forEach((p,i)=>{if(i<6)out['n'+String(i).padStart(2,'0')]=p});
if(upck!==opt.upck){const s=UP;const up=[];for(let i=0;i<s.length;i+=850)up.push(s.slice(i,i+850));window.__UP=up;out.upChanged=up.length}
out.upck=upck;
return out;
};'ok'
