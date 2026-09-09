# KNX 现场识别与确认

## 能做什么

树莓派网关内置一个只在局域网使用的现场识别台。它复用正在运行的 KNX 隧道，不会再建立
第二条连接，因此可与正常 Tuya 同步同时运行。

页面会记录每条报文的时间、物理源地址、组地址、`read/write/response`、原始值以及所有
可解码 DPT 候选。用户确认后，工具自动分配或复用 DP 槽位，并输出标准审核 CSV。

KNX 电报不包含 ETS 对象名称和权威 DPT，所以工具只能做候选推断，不能凭一条报文保证
DPT 正确。尤其 `DPT-5`、`DPT-17`、`DPT-20` 可能使用相同字节宽度，必须通过多次操作和
现场状态核对后确认。

## 启用

在 `config/config.json` 中设置：

```json
{
  "commissioning": {
    "enabled": true,
    "listen": "0.0.0.0:8090",
    "review_file": "../state/knx-commission-review.csv",
    "max_events": 100
  }
}
```

首次上线或服务代码有变化时：

```bash
make rpi ACTION=deploy
```

只改了配置时：

```bash
make rpi ACTION=config
```

浏览器打开：

```text
http://树莓派IP:8090
```

## 单项识别

1. 填写房间和设备/场景名称，选择类别及能力。
2. 点击“开始本次采集”，保持其他人暂时不要操作 KNX 设备。
3. 对灯光开关一次、再关一次；对枚举或数值设备至少操作两个不同值。
4. 在报文列表中根据时间和物理源地址识别控制报文。
5. 若执行器另发反馈或读响应，选择它作为状态报文；纯传感器可让控制和状态选择同一行。
6. 选择符合现场值的 DPT。模式、风速等枚举填写双向 JSON 映射。
7. 场景若需要固定场景号，填写 `knx_write_value`；留空时工具尝试采用所选报文值。
8. 点击确认。相同类别、槽位和能力再次确认会覆盖旧记录，不会产生重复行。

建议判断方式：

| 现场对象 | 常见候选 | 验证方法 |
| --- | --- | --- |
| 开关 | `DPT-1.001` | 开和关分别出现 `true/false` |
| 百分比 | `DPT-5.001` | 调到多个位置，数值随 0-100 变化 |
| 风速原始值 | `DPT-5.005` | 不同档位得到稳定且不同的 0-255 值 |
| 温度 | `DPT-9.001` | 与现场温度计或面板显示接近 |
| 湿度 | `DPT-9.007` | 与现场湿度显示接近 |
| 场景号 | `DPT-17.001` | 不同场景得到稳定的 0-63 编号 |
| HVAC 模式 | `DPT-20.105` | 制冷、制热、送风等分别得到固定枚举 |

控制 GA 与状态 GA 可以相同，也可以不同。若 App 写控制 GA 后设备动作，但状态没有变化，
应继续查找执行器反馈 GA，或确认 ETS 状态对象是否已绑定并允许读取。

## 生成升级配置

网页可直接点击“下载已确认 CSV”，也可以从本地工程拉取：

```bash
make rpi ACTION=review
```

文件落到 `data/knx-commission-review.csv`。生成并校验 bundle：

```bash
make mapping ACTION=finalize VERSION=2026.09.09-1
make mapping ACTION=validate
```

结果为 `data/knx-mapping-for-upgrade.json`。发布到统一 GitHub 配置仓库：

```bash
make publish
```

网关和 Smart Life 面板会从同一个 HTTPS 地址读取新映射。只要仍在平台预建 DP 容量和已有
控件类型内，新增名称、房间、组地址或设备槽位不需要重新编译网关，也不需要重新发布面板。

## 收尾

识别完成后把 `commissioning.enabled` 改回 `false`：

```bash
make rpi ACTION=config
```

该页面没有登录认证，只应在可信局域网临时开放。CSV 保存在树莓派 `state/`，重新部署不会
删除；本地 `data/` 和树莓派 `state/` 都不包含在运行配置同步中。
