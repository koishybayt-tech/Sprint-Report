// node build2.js <old dataN.js> <out dataM.js> <stamp YYYY-MM-DDTHH:MM> <dir with h.txt,new.txt,px.json[,up.json]>
const fs=require('fs');const [,,OLD,OUT,ST,DIR]=process.argv;const o={};new Function('o',fs.readFileSync(OLD,'utf8').replace(/^const /gm,'o.'))(o);
const h=s=>{let x=2166136261;for(const c of s){x^=c.codePointAt(0);x=Math.imul(x,16777619)>>>0}return x.toString(36)};const J=JSON.stringify;
const sum=Object.fromEntries(o.X.map(x=>[x[0],x[1]]));const M={};
const key=r=>{if(r[0]!='X')return h(r);const v=JSON.parse(r.slice(1));return h('X'+J([v[0],null,v[2],v[3],v[4],v[5]]))};
o.PJ.forEach(r=>M[h('P'+J(r))]='P'+J(r));o.AX.forEach(r=>M[h('A'+J(r))]='A'+J(r));o.QX.forEach(r=>M[h('Q'+J(r))]='Q'+J(r));o.X.forEach(x=>{const r='X'+J([x[0],null,x[2],x[3],x[4],x[5]]);M[h(r)]=r});
fs.readFileSync(DIR+'/new.txt','utf8').split('\n').filter(Boolean).forEach(r=>{M[key(r)]=r;if(r[0]=='X'){const v=JSON.parse(r.slice(1));if(v[1]!==null)sum[v[0]]=v[1]}});
const H=fs.readFileSync(DIR+'/h.txt','utf8').split(/\s+/).filter(Boolean);const miss=H.filter(x=>!M[x]);if(miss.length){console.log('MISSING',miss);process.exit(1)}
const P=[],A=[],Q=[],X=[];H.forEach(x=>{const r=M[x];const v=JSON.parse(r.slice(1));({P,A,Q,X})[r[0]].push(v)});
X.forEach(x=>{x[1]=sum[x[0]]??''});
let U=o.U,POS=o.POS,TL=o.TL;if(fs.existsSync(DIR+'/up.json')){const up=JSON.parse(fs.readFileSync(DIR+'/up.json','utf8'));U=up.U;POS=up.POS;TL=up.TL}
const PX=JSON.parse(fs.readFileSync(DIR+'/px.json','utf8'));
fs.writeFileSync(OUT,[`const SNAP=${J(ST.slice(0,10))};`,`const UPDATED=${J(ST)};`,`const PJ=${J(P)};`,`const AX=${J(A)};`,`const QX=${J(Q)};`,`const X=${J(X)};`,`const U=${J(U)};`,`const POS=${J(POS)};`,`const TL=${J(TL)};`,`const PX=${J(PX)};`].join('\n')+'\n');
const names=new Set();A.forEach(a=>names.add(a[0]));Q.forEach(q=>names.add(q[0]));
console.log(JSON.stringify({rows:H.length,proj:P.length,issues:P.reduce((s,p)=>s+p[6],0),ax:A.length,qx:Q.length,x:X.length,noSummary:X.filter(x=>!x[1]).map(x=>x[0]),missingU:[...names].filter(n=>!U[n]&&n[0]!='—')}));
