const $ = (id) => document.getElementById(id);
async function json(url, opts){ const r = await fetch(url, opts); const j = await r.json(); if(!r.ok) throw new Error(j.error?.message || r.statusText); return j; }
async function refresh(){
  const h = await json('/v1/health'); $('health').textContent = h.status; $('health').className = 'pill ok';
  const s = await json('/v1/sandboxes'); const list = s.data || [];
  $('sandbox-count').textContent = list.length; $('ready-count').textContent = list.filter(x=>x.state==='ready').length;
  $('sandboxes').innerHTML = list.map(x=>`<div class="item"><div class="row"><strong>${x.id}</strong><span class="pill ${x.state==='ready'?'ok':'warn'}">${x.state}</span><span class="muted">${x.environment}/${x.template}</span><button onclick="selectSandbox('${x.id}')">Select</button></div><div class="muted">lease expires ${new Date(x.lease.expires_at).toLocaleString()}</div></div>`).join('') || '<p class="muted">No sandboxes yet.</p>';
}
window.selectSandbox = (id) => { $('sandbox-id').value = id; };
$('refresh').onclick = refresh;
$('create').onclick = async () => { const r = await json('/v1/sandboxes',{method:'POST',headers:{'content-type':'application/json'},body:'{}'}); $('sandbox-id').value = r.data.id; await refresh(); alert('Client session issued. Start a client with the returned session credentials from the API response.'); };
$('run').onclick = async () => {
  const sid = $('sandbox-id').value.trim(); const cmd = $('command').value; $('output').textContent = 'starting...';
  const r = await json(`/v1/sandboxes/${sid}/commands`,{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({command:cmd})});
  let after = 0; let done = false; let text = '';
  for(let i=0;i<60 && !done;i++){
    await new Promise(r=>setTimeout(r,500));
    const ev = await json(`/v1/sandboxes/${sid}/commands/${r.data.id}/events?after=${after}`); after = ev.next_cursor || after;
    for(const e of ev.data){ if(e.type==='command.stdout'||e.type==='command.stderr') text += e.data.chunk; if(['command.exit','command.failed','command.killed'].includes(e.type)){ text += `\n[${e.type}]`; done=true; } }
    $('output').textContent = text || 'waiting for output...';
  }
};
refresh().catch(e=>{$('health').textContent='error'; $('output').textContent=e.message});
