// В вкладке Telegram (после __g2()): записи посещаемости с id > LAST в window.__AT2 (куски ≤880), список отправителей в window.__SN
(LAST=>{const T=window.__TG2,S=window.__SM||{};const sn=window.__SNL||(window.__SNL=[]);
const D={'Today':new Date(),'Yesterday':new Date(Date.now()-864e5)};const W=['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday'];const MO=['January','February','March','April','May','June','July','August','September','October','November','December'];
const dd=l=>{let d=D[l];if(!d){const wi=W.indexOf(l);if(wi>=0){d=new Date();while(d.getDay()!=wi)d=new Date(d-864e5)}else{const m=l.match(/^(\w+) (\d+)/);d=new Date(2026,MO.indexOf(m[1]),+m[2])}}return String(d.getMonth()+1).padStart(2,'0')+String(d.getDate()).padStart(2,'0')};
const tm=s=>{s=s.replace('.',':');const m=s.match(/^(\d{1,2})(?::(\d{2}))?$/);return m?(+m[1])*60+(+(m[2]||0)):null};
const rows=[];Object.keys(T).map(Number).filter(k=>k>LAST).sort((a,b)=>a-b).forEach(id=>{const o=T[id];
 let snd=(S[id]||[''])[0];if(!snd){const el=document.querySelector(`.Message[data-message-id="${id}"]`);let p=el;while(p&&!snd){const t=p.querySelector&&p.querySelector('.sender-title,.message-title-name');if(t)snd=t.textContent;p=p.previousElementSibling}}
 const lines=o.t.replace(/\n(edited )?\d{1,2}:\d{2}\s*$/,'').split('\n').map(x=>x.trim()).filter(Boolean);const tx=lines.join(' / ');const post=(o.time.match(/\d{1,2}:\d{2}/)||[''])[0].padStart(5,'0').replace(':','');
 let typ='';if(/^upd\.?$/i.test(lines[lines.length-1]||''))typ='U';else if(/больнич/i.test(tx))typ='S';else if(/отпуск/i.test(tx))typ='V';else if(/пропуск/i.test(tx))typ='P';else return;
 const pl=(lines.find(l=>/пропуск/i.test(l))||'');const rs=[];const re=/(\d{1,2}(?:[:.]\d{2})?)\s*[-–—]\s*(\d{1,2}[:.]\d{2})/g;let m;if(typ=='P')while((m=re.exec(pl))){const a=tm(m[1]),b=tm(m[2]);if(a!=null&&b!=null&&a>=360&&b<=1440&&b>a)rs.push(a+'-'+b)}
 const otr=(lines.find(l=>/отработ|отрабат/i.test(l))||'').replace(/^отработ\S*:?\s*/i,'').slice(0,30);
 const rsn=(lines.find(l=>/причин/i.test(l))||(/универ/i.test(tx)?'универ':'')).replace(/^причин\S*:?\s*/i,'').slice(0,30);
 const ag=(tx.match(/@[\w.]+/g)||[]).join(',').replace(/@/g,'');
 let si=sn.indexOf(snd);if(si<0){sn.push(snd);si=sn.length-1}
 rows.push([id,dd(o.d),post,'S'+si,typ,rs.join(','),rs.length?'':pl.replace(/^пропуск:?\s*/i,'').slice(0,40),otr,rsn,ag].join('~'))});
window.__AT2=[];let cur='';rows.forEach(r=>{if((cur+r).length>880){window.__AT2.push(cur);cur=''}cur+=r+'\n'});if(cur)window.__AT2.push(cur);return{n:rows.length,pieces:window.__AT2.length,sn:sn}})
