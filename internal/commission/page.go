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
.status strong{color:var(--orange)}.analysis{padding:16px;border-radius:14px;background:#edf5ef;border:1px solid #c9dfd0}.analysis b{color:var(--green)}.analysis .warn{color:var(--orange)}
.probe{background:#f7f2e7}.probe-result{margin-top:12px;padding:12px;border-radius:12px;background:white;border:1px dashed var(--line)}.danger{background:#a9472c}.event-new{background:#fff3d8}
.table-wrap{overflow:auto}table{width:100%;border-collapse:collapse;white-space:nowrap}th,td{padding:9px;border-bottom:1px solid #e8e3d8;text-align:left}
th{font-size:12px;color:var(--muted)}code{font-size:12px}.candidates{max-width:380px;white-space:normal}.hint{color:var(--muted);font-size:13px}
details{margin-top:14px;border-top:1px solid var(--line);padding-top:12px}summary{cursor:pointer;color:var(--muted)}
@media(max-width:760px){.grid{grid-template-columns:1fr 1fr}.wide{grid-column:span 2}.hero{border-radius:16px}main{margin-top:12px}}
</style>
</head>
<body><main>
<section class="hero"><h1>KNX 现场识别台</h1><p>先命名，再操作实体设备。报文会被截获、解码并整理为可发布的审核记录。</p></section>
<section class="card probe"><h2>组地址读写探测</h2>
<p class="hint">用于验证一个候选地址是否可读、写后是否有回显或状态反馈。写操作会真实控制 KNX 设备，请先确认地址和值安全。</p>
<div class="grid">
<div><label>目标组地址</label><input id="probeGA" placeholder="例如：1/2/11"></div>
<div><label>写入 DPT</label><select id="probeDpt"><option>DPT-1.001</option><option>DPT-5.001</option><option>DPT-5.005</option><option>DPT-9.001</option><option>DPT-9.007</option><option>DPT-17.001</option><option>DPT-20.105</option></select></div>
<div class="wide"><label>写入值（JSON）</label><input id="probeValue" placeholder="开关填 true/false，数值填 1 或 23.5"></div>
</div>
<div class="actions"><button id="probeRead">发送读请求</button><button id="probeWrite" class="danger">发送写请求</button></div>
<div id="probeResult" class="probe-result">尚未发起探测。</div>
</section>
<section class="card">
<div class="grid">
<div><label>房间</label><input id="room" placeholder="例如：客厅"></div>
<div><label>设备 / 场景名称</label><input id="name" placeholder="例如：客厅主灯"></div>
<div><label>类别</label><select id="category"></select></div>
<div><label>能力</label><select id="capability"></select></div>
<div id="observedWrap" class="wide" style="display:none"><label>本次实际操作到的状态</label><select id="observed"></select><div class="hint">只需选择你刚才在实体面板上操作的中文状态，数值映射由系统生成。</div></div>
</div>
<p class="hint">空调、新风等设备请使用相同房间和名称，分别采集开关、模式、风速、温度等能力；系统会复用同一个虚拟设备槽位。</p>
<div class="actions"><button id="start">开始本次采集</button><button id="refresh" class="secondary">立即刷新</button></div>
<div id="status" class="status">尚未开始采集。</div>
</section>
<section class="card"><h2>自动分析结果</h2>
<div id="analysis" class="analysis">等待捕获报文，完成后系统会自动填写。你无需填写组地址、DPT 或 JSON。</div>
<details><summary>查看捕获报文（仅在自动判断错误时改选）</summary>
<p class="hint">系统会按报文时序自动识别控制和状态。一般不需要操作下表。</p>
<div class="table-wrap"><table><thead><tr><th>改选控制</th><th>改选状态</th><th>时间</th><th>命令</th><th>源地址</th><th>组地址</th><th>原始值</th><th>候选解释</th></tr></thead><tbody id="events"></tbody></table></div>
</details>
<details><summary>高级参数（一般不用修改）</summary>
<div class="grid">
<div><label>DPT</label><select id="dpt"><option>DPT-1.001</option><option>DPT-5.001</option><option>DPT-5.005</option><option>DPT-9.001</option><option>DPT-9.007</option><option>DPT-17.001</option><option>DPT-20.105</option></select></div>
<div><label>固定 KNX 写值（JSON，可空）</label><input id="writeValue" placeholder="场景可填 1"></div>
<div class="wide"><label>备注</label><input id="notes" placeholder="现场现象、按键动作等"></div>
<div class="wide"><label>Tuya → KNX 枚举映射（JSON，可空）</label><textarea id="toKnx" placeholder='{"cool":1,"heat":2}'></textarea></div>
<div class="wide"><label>KNX → Tuya 枚举映射（JSON，可空）</label><textarea id="toTuya" placeholder='{"1":"cool","2":"heat"}'></textarea></div>
</div>
</details>
<div class="actions"><button id="confirm">确认分析结果并记录</button><a class="download" href="/api/review.csv">下载已确认 CSV</a></div><div id="result" class="status"></div>
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
const observedOptions={
 'air_conditioner/mode':[['auto','自动'],['cool','制冷'],['heat','制热'],['fan','送风'],['dry','除湿']],
 'fresh_air/mode':[['auto','自动'],['manual','手动']],
 'air_conditioner/fan_speed':[['auto','自动'],['low','低速'],['middle','中速'],['high','高速']],
 'fresh_air/fan_speed':[['low','低速'],['middle','中速'],['high','高速']]
};
const $=id=>document.getElementById(id);let state={events:[]};
for(const [id,item] of Object.entries(model))$('category').add(new Option(item.label,id));
function key(){return $('category').value+'/'+$('capability').value}
function syncCaps(){const category=$('category').value;$('capability').innerHTML='';for(const [id,label] of Object.entries(model[category].caps))$('capability').add(new Option(label,id));syncDpt();syncObserved()}
function syncDpt(){$('dpt').value=suggested[key()]||'DPT-1.001'}
function syncObserved(){const options=observedOptions[key()]||[];$('observedWrap').style.display=options.length?'block':'none';$('observed').innerHTML='';for(const item of options)$('observed').add(new Option(item[1],item[0]))}
$('category').onchange=syncCaps;$('capability').onchange=()=>{syncDpt();syncObserved()};$('observed').onchange=updateAnalysis;$('dpt').onchange=updateAnalysis;syncCaps();
async function api(path,options){const response=await fetch(path,{headers:{'Content-Type':'application/json'},...options});const text=await response.text();if(!response.ok)throw new Error(text);return text?JSON.parse(text):{}}
function checked(name){return Number(document.querySelector('input[name="'+name+'"]:checked')?.value||0)}
function candidate(event,dpt){return event?.candidates.find(item=>item.dpt===dpt)}
function readonly(){return $('capability').value==='temp_current'||$('category').value==='climate_sensor'}
function autoSelect(events){const dpt=$('dpt').value;const matching=events.filter(event=>candidate(event,dpt));if(!matching.length)return [0,0];if(readonly()){const id=matching[matching.length-1].id;return [id,id]}let control=matching.find(event=>event.command==='write')||matching[0];let status=[...matching].reverse().find(event=>event.command==='response')||[...matching].reverse().find(event=>event.ga!==control.ga)||matching[matching.length-1];return [control.id,status.id]}
function eventByID(id){return state.events.find(event=>event.id===id)}
function render(data){const oldControl=checked('control'),oldStatus=checked('status');state=data;const session=data.session;$('status').innerHTML=session.active?'正在采集：<strong>'+session.room+' / '+session.name+'</strong>，请操作实体设备或场景按键。':'本项已记录，可以开始下一项。';
let selected=[oldControl,oldStatus];if(!eventByID(oldControl)||!eventByID(oldStatus))selected=autoSelect(data.events);
const baseline=data.probe?.baseline_event_id||Number.MAX_SAFE_INTEGER;
$('events').innerHTML=data.events.map(event=>'<tr class="'+(event.id>baseline?'event-new':'')+'"><td><input type="radio" name="control" value="'+event.id+'" '+(event.id===selected[0]?'checked':'')+'></td><td><input type="radio" name="status" value="'+event.id+'" '+(event.id===selected[1]?'checked':'')+'></td><td>'+event.time+'</td><td>'+commandLabel(event.command)+(event.origin==='probe'?'（主动发送）':'')+'</td><td>'+event.source+'</td><td><b>'+event.ga+'</b></td><td><code>'+(event.raw_hex||'无数据')+'</code></td><td class="candidates">'+(event.candidates.length?event.candidates.map(c=>'<code>'+c.dpt+'='+JSON.stringify(c.value)+'</code>').join(' · '):'<span class="hint">读请求不携带数值</span>')+'</td></tr>').join('');
document.querySelectorAll('input[name="control"],input[name="status"]').forEach(input=>input.onchange=updateAnalysis);renderProbe(data.probe);updateAnalysis()}
function commandLabel(command){return {read:'读请求',write:'写报文',response:'读响应'}[command]||command}
function renderProbe(probe){if(!probe){$('probeResult').textContent='尚未发起探测。';return}if(probe.error){$('probeResult').innerHTML='<strong>发送失败：</strong>'+probe.error;return}
const action=probe.action==='read'?'读请求':'写请求';let conclusion='';
if(probe.action==='read')conclusion=probe.has_response?'已收到该地址的读响应。':'暂未收到该地址的读响应；可能对象不可读、没有设备响应，或仍在等待。';
else conclusion=probe.has_write?'监听到该地址的写报文。':'写请求已发送，但尚未监听到同地址回显；本机隧道不回显时也可能正常，请结合设备动作和附近反馈报文判断。';
$('probeResult').innerHTML='<strong>'+action+'已发送：</strong><code>'+probe.ga+'</code>'+(probe.value?' = <code>'+probe.value+'</code>':'')+'<br>'+conclusion+'<br>同地址报文 '+probe.direct_matches+' 条，探测后附近报文 '+probe.nearby_events+' 条；新增报文已在明细中标黄。'}
async function runProbe(action){try{const ga=$('probeGA').value.trim();let value=null;if(action==='write'){const raw=$('probeValue').value.trim();if(!raw)throw new Error('写操作必须填写值');try{value=JSON.parse(raw)}catch(_){throw new Error('写入值格式不正确：开关填 true/false，数值直接填数字')} }await api('/api/probe',{method:'POST',body:JSON.stringify({action,ga,dpt:$('probeDpt').value,value})});await refresh()}catch(error){$('probeResult').textContent=error.message}}
function updateAnalysis(){const control=eventByID(checked('control')),status=eventByID(checked('status')),dpt=$('dpt').value,value=candidate(status,dpt)?.value;if(!control||!status||value===undefined){$('analysis').textContent='等待捕获到可识别报文，请操作实体设备。组地址、DPT 和映射值会由系统自动填写。';return}
if(key()==='scene/trigger')$('writeValue').value=JSON.stringify(candidate(control,dpt)?.value??value);
const enumOptions=observedOptions[key()];if(enumOptions){const semantic=$('observed').value,controlValue=candidate(control,dpt)?.value;$('toKnx').value=JSON.stringify({[semantic]:controlValue});$('toTuya').value=JSON.stringify({[String(value)]:semantic})}
const relation=control.ga===status.ga?'控制和状态共用同一组地址':'控制后由独立状态地址反馈';const semantic=enumOptions?'，对应“'+$('observed').selectedOptions[0].text+'”':'';
$('analysis').innerHTML='<b>已自动识别</b><br>控制地址：<code>'+control.ga+'</code>　状态地址：<code>'+status.ga+'</code><br>数据类型：<code>'+dpt+'</code>　当前解析值：<code>'+JSON.stringify(value)+'</code>'+semantic+'<br>'+relation+'。请核对设备实际动作和显示是否一致，然后直接确认。'}
async function refresh(){try{render(await api('/api/state'))}catch(error){$('status').textContent=error.message}}
$('probeRead').onclick=()=>runProbe('read');$('probeWrite').onclick=()=>runProbe('write');
$('refresh').onclick=refresh;$('start').onclick=async()=>{try{$('writeValue').value='';$('toKnx').value='';$('toTuya').value='';$('notes').value='';$('analysis').textContent='等待捕获到可识别报文，请操作实体设备。组地址、DPT 和映射值会由系统自动填写。';await api('/api/session',{method:'POST',body:JSON.stringify({room:$('room').value,name:$('name').value,category:$('category').value,capability:$('capability').value})});$('result').textContent='';syncDpt();await refresh()}catch(error){$('status').textContent=error.message}};
$('confirm').onclick=async()=>{try{const result=await api('/api/confirm',{method:'POST',body:JSON.stringify({control_event_id:checked('control'),status_event_id:checked('status'),dpt:$('dpt').value,observed_value:$('observedWrap').style.display==='none'?'':$('observed').value,knx_write_value:$('writeValue').value.trim(),tuya_to_knx_json:$('toKnx').value.trim(),knx_to_tuya_json:$('toTuya').value.trim(),notes:$('notes').value.trim()})});$('result').innerHTML='<strong>记录成功：</strong>'+result.record.tuya_dp_code+'。可以继续验证下一项。';await refresh()}catch(error){$('result').textContent=error.message}};
refresh();setInterval(refresh,1200);
</script></body></html>`
