# 树莓派定制网关

这是网关 PID `ue267xspp2htjgal` 的 Ray 自定义面板。它运行在 Smart Life 中，不部署到
树莓派。树莓派只运行根目录的 Go 网关服务。

## 界面能力

- 总览、场景、灯光、空调、新风、温控分类 Tab。
- 每类设备采用独立卡片、图标、状态和交互。
- 总览按设备类别分段，灯光和场景使用移动端双栏布局。
- 面板读取映射中的 `room` 字段展示区域标签，并支持按房间筛选。
- 场景页可进入 Smart Life 原生“一键执行”创建器和当前网关的联动管理页。
- 使用 `useProps` 实时接收网关 DP，使用 `useActions` 下发控制。
- 正式构建以内置清单兜底，并从统一 HTTPS bundle 动态刷新 KNX 逻辑设备。
- 浏览器地址添加 `?mock=1` 可查看全类别样例，并在本地模拟交互，不会下发真实 DP。

## 配置来源

不要直接修改 `src/generated/`。执行：

```bash
make gateway ACTION=model
```

同步脚本读取：

```text
config/knx-mapping.json
data/tuya-gateway-dp-plan.json
```

逻辑设备 manifest 由映射中的 `virtual_device_id/category/room/slot/capability` 动态生成，
不再需要单独维护 `tuya-panel-manifest.json`。

并生成：

```text
panel/src/generated/schema.ts
panel/src/generated/manifest.ts
panel/src/generated/runtime.ts
```

`runtime.ts` 中的地址和轮询周期来自 `config/config.json` 的
`runtime_update.remote_url/remote_poll_seconds`。面板启动立即拉取，之后按配置周期刷新；
请求失败时保留最后有效配置或使用内置清单。

设备所属 Tab 由清单中的 `category` 决定，目前支持：

```text
light
air_conditioner
climate_sensor
scene
fresh_air
```

设备房间由同一逻辑设备各条映射中的 `room` 决定，例如：

```json
{
  "name": "公卫灯",
  "virtual_device_id": "public_bathroom_light",
  "category": "light",
  "room": "公卫",
  "slot": 1
}
```

`room` 可以使用“客厅”“主卧”“公卫”“厨房”“全屋”等现场名称。字段不填时面板按
“全屋”处理；同一 `virtual_device_id/category/slot` 的所有能力必须配置相同房间。

这里的房间用于网关自定义面板内的标签和筛选。Smart Life 原生家庭房间以及 App 的自动化
设备选择器由涂鸦平台管理，不能由一个网关 DP 动态创建；为了在原生自动化列表中也容易区分，
建议平台 DP 名称和映射 `name` 同时包含房间，例如“公卫灯开关”。

## 本地预览

```bash
make panel ACTION=install
make panel ACTION=dev
```

浏览器打开：

```text
http://localhost:8899/?mock=1
```

正式映射预览去掉 `?mock=1`。Web 预览用于检查布局和交互，不代表真实设备通信；真实 DP
订阅和下发必须在涂鸦 MiniApp IDE 或 Smart Life 真机中验证。

## 构建与发布

```bash
make panel ACTION=check
make panel ACTION=build
```

正式构建要求 `config/config.json` 中已有 HTTPS `remote_url`；空地址会直接失败，避免误发布成只使用静态清单的版本。
涂鸦平台请求合法域名需包含 `gitee.com`。不要使用 `raw.giteeusercontent.com` 或
`gitee.com/.../raw/...`，它们会拒绝 Smart Life 自动携带的 Referer。当前使用 Gitee
Contents API，面板会自动解码其 Base64 文件内容。

Tuya 小程序产物在 `panel/dist/tuya/`。使用涂鸦 MiniApp IDE 创建面板项目后：

1. 导入 `panel/`，项目类型选择设备面板/Ray。
2. 将 IDE 分配的项目 ID 写入 `project.tuya.json` 的 `projectId`。
3. 在 IDE 的基础库与 Kit 配置中不要选择 `latest`，保持最低基础库为 `2.27.0`，
   `BaseKit` 为 `3.2.6`。
4. 在 IDE 中绑定测试设备并确认 DP Schema 与平台 PID 一致。
5. 上传体验版，在涂鸦平台把面板关联到 PID `ue267xspp2htjgal`。
6. 先用体验设备测试，再提交审核、发布面板和产品。

首页顶部显示来自 `panel/package.json` 的 Panel 版本、远程 bundle 的配置版本，并提供
“总线调试”入口。该页面使用固定 DP `177-180` 经 TuyaLink MQTT 访问树莓派 KNX 总线，
不直连局域网 HTTP，也不会写 commissioning CSV。平台必须先发布：

| DP ID | code | 类型/权限 |
| --- | --- | --- |
| 177 | `knx_debug_request` | string/rw |
| 178 | `knx_debug_trigger` | bool/rw |
| 179 | `knx_debug_status` | enum/ro，`idle,running,success,error` |
| 180 | `knx_debug_result` | string/ro |

Smart Life 的原生设备属性/设置菜单由宿主 App 和产品平台控制，面板代码不能任意向该原生页
插入菜单；项目内的自定义功能入口应放在 Ray 自定义面板页面中。

平台上必须预先存在面板引用的 DP code。面板只能展示、订阅和控制已有 DP，不能在运行时
创建产品功能点。

## Smart Life 场景联动

当前产品采用“单网关、多 DP”模式。Smart Life 自动化中显示的是“树莓派网关”及其
`light_01_switch`、`scene_01_trigger` 等功能点；面板中的逻辑卡片不是独立子设备。
若必须显示成独立灯、空调设备，则需要改用真实子设备注册和子设备 PID。

先在涂鸦开发者平台为 PID 的功能点配置场景联动能力：

- 灯、空调、新风等可控 DP：按需要同时允许“作为条件”和“作为任务”。
- 温湿度等只读 DP：只允许“作为条件”。
- `scene_XX_trigger`：至少允许“作为任务”，用于由 Smart Life 调用 KNX 场景。

导入 `tuya-gateway-dp-platform.xlsx` 只会创建功能点，不会自动打开场景条件或任务权限。
`tuya-gateway-dp-plan.json` 中的 `scene_condition/scene_action` 是平台配置清单，不是运行时
自动生效的开关。请在产品的场景联动设置中逐项启用并再次发布产品；页面入口和 MQTT
程序都无法代替该平台操作。

当前已映射并建议先开放的功能点：

- 条件和任务：`light_01_switch` 至 `light_04_switch`。
- 条件和任务：`ac_01_switch`、`ac_01_mode`、`ac_01_fan_speed`、`ac_01_temp_set`。
- 条件和任务：`fresh_air_01_switch`、`fresh_air_01_fan_speed`。
- 仅任务：`scene_01_trigger`、`scene_02_trigger`。

真机进入面板“场景”Tab：

1. 点击“新建组合场景”，进入 Smart Life 原生一键执行编辑器。
2. 添加任务“树莓派网关 → 场景XX触发 → true”，再添加其他 Smart Life 设备动作。
3. 保存后，该场景由涂鸦云统一调度；树莓派仅负责收到场景 DP 后转发 KNX。
4. 保存至少一条引用网关 DP 的规则后，“联动管理”才会显示内容；首次进入为空是正常的。

如果需要“现场 KNX 按键触发后，再联动其他 Smart Life 设备”，不要直接复用同一个
`scene_XX_trigger` 形成回环。应预留单独的只读事件 DP，或在网关增加来源去重和脉冲复位
逻辑后，再把该事件 DP 配置为自动化条件。

如果在 Smart Life 新建场景时完全看不到树莓派网关，打开面板的“场景”页查看“场景资格
诊断”：

- 显示“共享设备”：改用首次绑定该网关的原始账号，在设备所属家庭中创建场景。
- 显示“家庭成员”：改用家庭管理员账号验证。
- 云端 DP 少于本地 DP：产品模型尚未刷新到设备实例，先结束 Smart Life 进程重新打开；
  仍未刷新时，删除设备后重新绑定作为最终验证。
- 显示“家庭管理员”，且云端/本地均为 80 个 DP，但场景候选列表仍没有网关：保存诊断
  中的 Device ID、PID、产品版本和截图，向涂鸦提交工单检查该产品实例的场景联动索引。

单网关 DP 模式只会在场景候选列表显示一个“树莓派网关”，不会把面板中的灯、空调和
场景卡片显示为独立设备。选择网关后，才应继续选择已开放的 DP。要让每个 KNX 设备作为
独立候选设备出现，必须切换到真实 TuyaLink 子设备模式并为各品类配置子设备 PID。

后续在预建 DP 槽位内增加设备、修改名称、房间、设备映射或枚举选项时，只需发布新的
`knx/knx-mapping.json`，无需重新上传面板。枚举控件按映射中 `tuya_to_knx` 的 key
顺序展示，但这些 key 必须已包含在涂鸦平台该 DP 的枚举允许值中，否则平台可能拒绝下发。

布尔和数值 DP 默认继续使用涂鸦平台/面板内置 Schema。映射以后若增加 `min`、`max`、
`step`、`scale`、`unit` 数值 UI 元数据，面板会用远程值覆盖显示范围；平台允许范围仍以
产品 Schema 为准。新增控件类型、增加平台未注册的 DP/枚举值或修改远程 URL 时，仍需先
修改涂鸦平台配置，必要时重新构建和发布面板。

如果真机扫码提示“基础库不支持”，先升级 Smart Life；然后在 IDE 中重新选择
`2.27.0`、`BaseKit 3.2.6` 并重新编译生成二维码。旧二维码仍携带旧编译版本，修改配置后
不能继续使用。
