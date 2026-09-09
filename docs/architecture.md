# 架构

## 代码边界

```text
Go gateway      main.go + internal/
Mapping CLI     cmd/knx-mapping-tool/
Commission UI   internal/commission/
Smart Life UI   panel/
Runtime input   config/
Local workflow  data/
Pi state        /opt/knx-tuya-gw/state/
Operations      tools/ + deploy/
```

网关和面板独立构建，只共享远程映射的数据契约。`data/` 中的平台文件和原始清单不属于运行
时依赖。

## 设备模型

```text
Smart Life
  `-- 树莓派虚拟网关（唯一设备入口）
        `-- 网关自身 DP
              |-- 灯光槽位
              |-- 空调槽位
              |-- 场景槽位
              |-- 新风槽位
              `-- 温湿度槽位
```

面板中的“设备卡片”是 UI 层逻辑分组，不是涂鸦子设备。所有状态、控制和自动化能力最终
都落在网关 PID 预先发布的 DP 上。

## 数据流

KNX 到涂鸦：

```text
KNX write/response
  -> status_ga 查映射
  -> DPT 解码和数值转换
  -> tylink/{gatewayDeviceId}/thing/property/report
  -> Smart Life 面板和自动化
```

涂鸦到 KNX：

```text
tylink/{gatewayDeviceId}/thing/property/set
  -> gateway + dp_code 查映射
  -> 枚举/倍率/固定写值转换
  -> KNX GroupValueWrite 到 ga
  -> property/set_response
```

## 配置职责

- `config/config.json`：KNX 地址、MQTT 连接、DP 容量和本地生成文件位置。
- `config/tuya-mqtt.env`：激活后得到的 PID、Device ID、Device Secret 和 MQTT broker。
- `config/knx-mapping.json`：启动保底和远程更新落盘文件。
- 稳定 HTTPS `knx-mapping.json`：网关与面板共同使用的生产配置真源。
- `data/knx-mapping-for-upgrade.json`：审核工具生成的本地待发布文件。
- `data/tuya-gateway-dp-plan.json`：根据容量生成的完整网关产品功能清单。
- `state/knx-commission-review.csv`：树莓派现场识别台写出的已确认记录。

面板逻辑设备清单由 `panel/scripts/sync-manifest.mjs` 直接从
`config/knx-mapping.json` 构建，不再维护独立的 `tuya-panel-manifest.json`。

## 安全与持久化

`config/config.json` 和 `config/tuya-mqtt.env` 含敏感信息，部署权限为 `0600`。
`data/` 完全属于本地开发和平台建模，不进入树莓派运行目录。树莓派运行服务读取
`config/`，现场识别结果写入 `state/`，不会在设备上生成 DP 平台资料。MQTT 使用 TLS
1.2，凭证按 TuyaLink HMAC-SHA256 规则动态生成。

TuyaLink MQTT 不承担映射配置下发。HTTPS 映射通过完整校验后原子覆盖本地映射；请求
失败或内容非法时继续使用上一有效版本。
