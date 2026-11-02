// node build_abs.js <dataN.js> <bitrixStamp> > abs.js
const fs=require('fs'),D=__dirname;const o={};new Function('o',fs.readFileSync(process.argv[2],'utf8').replace(/^const /gm,'o.'))(o);
const L2N=Object.fromEntries(Object.entries(o.U).map(([n,l])=>[l,n]));
const ID2L=Object.fromEntries(fs.readFileSync(D+'/emp.txt','utf8').trim().split(/\s+/).map(x=>x.split(':')));
const yr=o.SNAP.slice(0,4);const out=[];
fs.readFileSync(D+'/vac.txt','utf8').trim().split(/\s+/).filter(Boolean).forEach(r=>{const [id,t,f,e,st]=r.split(',');const l=ID2L[id];if(!l){console.error('no login for',id);return}
  out.push([L2N[l]||l,t=='B'?'bs':'vac',yr+'-'+f,yr+'-'+e,'Bitrix',t=='B'?'Отпуск без содержания':'Трудовой отпуск'])});
fs.readFileSync(D+'/sick.txt','utf8').split('\n').filter(s=>s&&s[0]!='#').forEach(s=>{const [l,f,e,note]=s.split('|');
  if(out.some(a=>a[0]==(L2N[l]||l)&&a[1]!='sick'&&a[2]<=(e||f)&&a[3]>=f))return;
  out.push([L2N[l]||l,'sick',f,e||null,'Telegram',note])});
try{JSON.parse(fs.readFileSync(D+'/../att/off.json','utf8')).forEach(r=>{if(!out.some(a=>a[0]==r[0]&&a[1]!='off'&&a[2]<=r[2]&&(a[3]||'9')>=r[2]))out.push(r)})}catch(e){}
const tg=JSON.parse(fs.readFileSync(D+'/tg_state.json','utf8'));
console.log(`const ABS=${JSON.stringify(out)};\nconst ABSAT=${JSON.stringify({bx:process.argv[3],tg:tg.lastMsgAt,tgChecked:tg.checkedAt})};`);
