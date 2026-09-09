package commission

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>KNX 现场识别</title>
<style>
:root{--ink:#15211c;--muted:#6b756f;--paper:#f4f1e8;--card:#fffdf7;--green:#176b4d;--line:#d8d3c5;--orange:#d66b2c}
*{box-sizing:border-box}body{margin:0;color:var(--ink);background:radial-gradient(circle at 10% 0,#d9eadc,transparent 34%),var(--paper);font:15px/1.5 ui-rounded,"PingFang SC",sans-serif}
main{width:min(1100px,calc(100% - 24px));margin:24px auto 60px}.hero{padding:22px 24px;color:white;border-radius:22px;background:linear-gradient(120deg,#123d31,#24775a);box-shadow:0 18px 45px #174a3830}
h1{margin:0 0 5px;font-size:28px}.hero p{margin:0;color:#dcece5}.card{margin-top:16px;padding:18px;background:var(--card);border:1px solid var(--line);border-radius:18px}
.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px}.wide{grid-column:span 2}label{display:block;font-size:12px;color:var(--muted);margin-bottom:5px}
input,select,textarea,button,.download{width:100%;border:1px solid var(--line);border-radius:10px;padding:10px 11px;background:white;color:var(--ink);font:inherit}
textarea{min-height:72px;resize:vertical}button{cursor:pointer;border:0;background:var(--green);color:white;font-weight:650}.secondary{background:#e9e4d8;color:var(--ink)}
.actions{display:flex;gap:10px;margin-top:14px}.actions button,.actions .download{width:auto;padding-inline:20px}.download{text-decoration:none;background:#e9e4d8}.status{margin-top:10px;color:var(--muted)}
.status strong{color:var(--orange)}.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse;white-space:nowrap}th,td{padding:9px;border-bottom:1px solid #e8e3d8;text-align:left}
th{font-size:12px;color:var(--muted)}code{font-size:12px}.candidates{max-width:380px;white-space:normal}.hint{color:var(--muted);font-size:13px}
@media(max-width:760px){.grid{grid-template-columns:1fr 1fr}.wide{grid-column:span 2}.hero{border-radius:16px}main{margin-top:12px}}
</style>
</head>
<body><main>
<section class="hero"><h1>KNX 现场识别台</h1><p>先命名，再操作实体设备。报文会被截获、解码并整理为可发布的审核记录。</p></section>
<section class="card">
<div class="grid">
<div><label>房间</label><input id="room" placeholder="例如：客厅"></div>
<div><label>设备 / 场景名称</label><input id="name" placeholder="例如：客厅主灯"></div>
<div><label>类别</label><select id="category"></select></div>
<div><label>能力</label><select id="capability"></select></div>
</div>
<div class="actions"><button id="start">开始本次采集</button><button id="refresh" class="secondary">立即刷新</button></div>
<div id="status" class="status">尚未开始采集。</div>
</section>
<section class="card"><h2>捕获报文</h2><p class="hint">选择控制 GA 和状态 GA。两者相同时可选择同一行；只读传感器也选择同一行。</p>
<div class="table-wrap"><table><thead><tr><th>控制</th><th>状态</th><th>时间</th><th>命令</th><th>源地址</th><th>组地址</th><th>原始值</th><th>候选解释</th></tr></thead><tbody id="events"></tbody></table></div>
</section>
<section class="card"><h2>确认记录</h2>
<div class="grid">
<div><label>DPT</label><select id="dpt"><option>DPT-1.001</option><option>DPT-5.001</option><option>DPT-5.005</option><option>DPT-9.001</option><option>DPT-9.007</option><option>DPT-17.001</option><option>DPT-20.105</option></select></div>
<div><label>固定 KNX 写值（JSON，可空）</label><input id="writeValue" placeholder="场景可填 1"></div>
<div class="wide"><label>备注</label><input id="notes" placeholder="现场现象、按键动作等"></div>
<div class="wide"><label>Tuya → KNX 枚举映射（JSON，可空）</label><textarea id="toKnx" placeholder='{"cool":1,"heat":2}'></textarea></div>
<div class="wide"><label>KNX → Tuya 枚举映射（JSON，可空）</label><textarea id="toTuya" placeholder='{"1":"cool","2":"heat"}'></textarea></div>
</div>
<div class="actions"><button id="confirm">确认并写入 CSV</button><a class="download" href="/api/review.csv">下载已确认 CSV</a></div><div id="result" class="status"></div>
</section>
</main>
<script>
const model={
 light:{label:'灯光',caps:{switch:'开关'}},
 air_conditioner:{label:'空调',caps:{switch:'开关',mode:'模式',fan_speed:'风速',temp_set:'设定温度',temp_current:'当前温度'}},
 scene:{label:'场景',caps:{trigger:'触发'}},
 fresh_air:{label:'新风',caps:{switch:'开关',mode:'模式',fan_speed:'风速'}},
 climate_sensor:{label:'温湿度',caps:{temperature:'温度',humidity:'湿度'}}
};
const suggested={'light/switch':'DPT-1.001','air_conditioner/switch':'DPT-1.001','fresh_air/switch':'DPT-1.001','scene/trigger':'DPT-17.001','air_conditioner/mode':'DPT-20.105','fresh_air/mode':'DPT-20.105','air_conditioner/fan_speed':'DPT-5.005','fresh_air/fan_speed':'DPT-5.005','air_conditioner/temp_set':'DPT-9.001','air_conditioner/temp_current':'DPT-9.001','climate_sensor/temperature':'DPT-9.001','climate_sensor/humidity':'DPT-9.007'};
const $=id=>document.getElementById(id);let state={events:[]};
for(const [id,item] of Object.entries(model))$('category').add(new Option(item.label,id));
function syncCaps(){const category=$('category').value;$('capability').innerHTML='';for(const [id,label] of Object.entries(model[category].caps))$('capability').add(new Option(label,id));syncDpt()}
function syncDpt(){$('dpt').value=suggested[$('category').value+'/'+$('capability').value]||'DPT-1.001'}
$('category').onchange=syncCaps;$('capability').onchange=syncDpt;syncCaps();
async function api(path,options){const response=await fetch(path,{headers:{'Content-Type':'application/json'},...options});const text=await response.text();if(!response.ok)throw new Error(text);return text?JSON.parse(text):{}}
function checked(name){return Number(document.querySelector('input[name="'+name+'"]:checked')?.value||0)}
function render(data){state=data;const session=data.session;$('status').innerHTML=session.active?'正在采集：<strong>'+session.room+' / '+session.name+'</strong>，请操作实体设备或场景按键。':'采集已停止，开始下一项前请重新填写并启动。';
$('events').innerHTML=data.events.map((event,index)=>'<tr><td><input type="radio" name="control" value="'+event.id+'" '+(index===0?'checked':'')+'></td><td><input type="radio" name="status" value="'+event.id+'" '+(index===data.events.length-1?'checked':'')+'></td><td>'+event.time+'</td><td>'+event.command+'</td><td>'+event.source+'</td><td><b>'+event.ga+'</b></td><td><code>'+event.raw_hex+'</code></td><td class="candidates">'+event.candidates.map(c=>'<code>'+c.dpt+'='+JSON.stringify(c.value)+'</code>').join(' · ')+'</td></tr>').join('')}
async function refresh(){try{render(await api('/api/state'))}catch(error){$('status').textContent=error.message}}
$('refresh').onclick=refresh;$('start').onclick=async()=>{try{await api('/api/session',{method:'POST',body:JSON.stringify({room:$('room').value,name:$('name').value,category:$('category').value,capability:$('capability').value})});$('result').textContent='';syncDpt();await refresh()}catch(error){$('status').textContent=error.message}};
$('confirm').onclick=async()=>{try{const result=await api('/api/confirm',{method:'POST',body:JSON.stringify({control_event_id:checked('control'),status_event_id:checked('status'),dpt:$('dpt').value,knx_write_value:$('writeValue').value.trim(),tuya_to_knx_json:$('toKnx').value.trim(),knx_to_tuya_json:$('toTuya').value.trim(),notes:$('notes').value.trim()})});$('result').innerHTML='<strong>已写入：</strong>'+result.record.tuya_dp_code+' → '+result.review_file;await refresh()}catch(error){$('result').textContent=error.message}};
refresh();setInterval(refresh,1200);
</script></body></html>`
