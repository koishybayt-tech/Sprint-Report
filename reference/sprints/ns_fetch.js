// В вкладке Jira: собирает следующий спринт по каждому проекту → window.__NS (строки k~id~name~start~end~total~statuses~assignees)
// CUR = имена текущих спринтов (из PJ[3] свежего dataN.js) — подставить перед запуском.
(async CUR=>{const g=async u=>{const r=await fetch(u,{headers:{Accept:'application/json'}});if(!r.ok)throw new Error(u+' '+r.status);return r.json()};
const FS={};await Promise.all(Object.keys(CUR).map(async k=>{try{const b=await g('/rest/agile/1.0/board?projectKeyOrId='+k);const sp=[];for(const bd of b.values.filter(v=>v.type=='scrum')){if(bd.id==140)continue;const s=await g(`/rest/agile/1.0/board/${bd.id}/sprint?state=future&maxResults=50`);s.values.forEach(x=>sp.push([bd.id,x.id,'f',x.name,(x.startDate||'').slice(0,10),(x.endDate||'').slice(0,10)]))}FS[k]=sp}catch(e){}}));
const tpl=s=>s.replace(/[\d.]+/g,'#').replace(/[,\s]+/g,' ').trim();const num=s=>+((s.match(/\d+/g)||[]).find(x=>!/^20\d\d$/.test(x))||0);
const out=[];for(const [k,v] of Object.entries(FS)){const cur=CUR[k];const seen={};v.forEach(x=>seen[x[1]]=x);const fut=Object.values(seen);
 let c=fut.filter(x=>tpl(x[3])==tpl(cur));
 if(!c.length)c=fut.filter(x=>tpl(x[3]).startsWith(tpl(cur).split('#')[0].trim())&&/\d/.test(x[3])&&!/backlog|бэклог/i.test(x[3]));
 if(!c.length)c=fut.filter(x=>x[4]&&!/backlog|бэклог/i.test(x[3]));
 c.sort((a,b)=>num(a[3])-num(b[3]));const x=c[0];if(!x)continue;
 let st=0,tot=0;const by={},ss={};while(true){const s=await g('/rest/api/2/search?jql='+encodeURIComponent('cf[10104] = '+x[1])+'&startAt='+st+'&maxResults=100&fields=assignee,status');tot=s.total;s.issues.forEach(i=>{const a=i.fields.assignee?i.fields.assignee.displayName:'— не назначен';by[a]=(by[a]||0)+1;const sn=i.fields.status.name;ss[sn]=(ss[sn]||0)+1});st+=100;if(st>=s.total)break}
 out.push([k,x[1],x[3],x[4],x[5],tot,Object.entries(ss).map(e=>e.join(':')).join(','),Object.entries(by).map(e=>e.join(':')).join(',')])}
window.__NS=out;return out.length})
