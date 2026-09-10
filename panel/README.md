# 树莓派定制网关

这是网关 PID `ue267xspp2htjgal` 的 Ray 自定义面板。它运行在 Smart Life 中，不部署到
树莓派。树莓派只运行根目录的 Go 网关服务。

## 界面能力

- 总览、场景、灯光、空调、新风、温控分类 Tab。
- 每类设备采用独立卡片、图标、状态和交互。
- 总览按设备类别分段，灯光和场景使用移动端双栏布局。
- 面板读取映射中的 `room` 字段展示区域标签，并支持按房间筛选。
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

`runtime.ts` 中的地址来自 `config/config.json` 的 `runtime_update.remote_url`。面板启动立即
拉取，之后每 60 秒刷新；请求失败时保留最后有效配置或使用内置清单。

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
涂鸦平台请求合法域名需包含 `raw.giteeusercontent.com`。

Tuya 小程序产物在 `panel/dist/tuya/`。使用涂鸦 MiniApp IDE 创建面板项目后：

1. 导入 `panel/`，项目类型选择设备面板/Ray。
2. 将 IDE 分配的项目 ID 写入 `project.tuya.json` 的 `projectId`。
3. 在 IDE 的基础库与 Kit 配置中不要选择 `latest`，保持最低基础库为 `2.27.0`，
   `BaseKit` 为 `3.2.6`。
4. 在 IDE 中绑定测试设备并确认 DP Schema 与平台 PID 一致。
5. 上传体验版，在涂鸦平台把面板关联到 PID `ue267xspp2htjgal`。
6. 先用体验设备测试，再提交审核、发布面板和产品。

平台上必须预先存在面板引用的 DP code。面板只能展示、订阅和控制已有 DP，不能在运行时
创建产品功能点。

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
