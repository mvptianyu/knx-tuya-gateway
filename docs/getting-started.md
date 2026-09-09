# 从零接入、部署与上线

## 1. 准备环境

本地安装 Go、Node.js、Git、SSH、rsync。推荐使用 SSH 密钥，也支持直接从 `.env.rpi`
读取密码。树莓派应运行 64 位 Raspberry Pi OS，并与泰创 KNXnet/IP 网关网络互通。

运行配置只放在 `config/`：

```text
config/config.json              KNX、Tuya、远程更新参数，含 API Key
config/knx-mapping.json         本地保底映射
config/tuya-mqtt.env            TuyaLink 设备凭证
config/*.cer 或 *.pem           可选 CA
```

首次配置：

```bash
cp config/config.example.json config/config.json
cp config/tuya-mqtt.env.example config/tuya-mqtt.env
cp .env.rpi.example .env.rpi
```

`config/config.json`、`config/tuya-mqtt.env` 和 `.env.rpi` 均已忽略，不要提交。统一远程映射
地址为：

```text
https://raw.githubusercontent.com/mvptianyu/config-gateway/refs/heads/main/knx/knx-mapping.json
```

## 2. 配置树莓派登录

推荐密钥方式：

```bash
RPI_HOST=192.168.110.10
RPI_USER=pi
RPI_SSH_KEY=/Users/你的用户/.ssh/id_ed25519
RPI_PORT=22
RPI_APP_DIR=/opt/knx-tuya-gw
```

也可在 `.env.rpi` 使用密码；特殊字符密码请用单引号：

```bash
RPI_HOST=192.168.110.10
RPI_USER=pi
RPI_PASSWORD='SSH登录密码'
RPI_SUDO_PASSWORD='sudo密码'
```

`RPI_SUDO_PASSWORD` 留空时自动复用 `RPI_PASSWORD`。配置密码后，`make rpi ...` 会通过
临时 `SSH_ASKPASS` 和 `sudo -S` 自动输入，无需安装 `sshpass`，也不再交互询问。

## 3. KNX 扫描与运行配置

本机不在 KNX 局域网时不要本地扫描。先部署到树莓派：

```bash
make rpi ACTION=deploy
make rpi ACTION=info
```

然后在树莓派执行发现：

```bash
ssh pi@树莓派IP
sudo /opt/knx-tuya-gw/knx-tuya-gw \
  -config /opt/knx-tuya-gw/config/config.json -discover
```

发现成功后，生产环境建议将地址固定到 `config.json` 的 `knx.gateway.host`。组播发现失败时
检查 VLAN、组播过滤、防火墙和 UDP 3671。

## 4. 涂鸦产品与网关激活

1. 在 TuyaLink 网关产品 `ue267xspp2htjgal` 中准备产品功能。
2. 执行 `make gateway ACTION=model`。
3. 将 `data/tuya-gateway-dp-platform.xlsx` 导入产品，核对 DP code、类型、读写权限后发布。
4. 对需要参与自动化的 DP 开启场景条件或任务能力。
5. 执行 `make gateway ACTION=provision` 激活网关，并在 Smart Life 完成绑定。
6. 执行 `make gateway ACTION=tuya-check` 检查 MQTT。

同一 Device ID 不要同时运行两个 MQTT 客户端，否则连接会互相顶掉。TuyaLink MQTT 只能
收发已发布 DP，不能动态创建或修改产品功能。

## 5. 测试和部署

```bash
make check
make rpi ACTION=build
make rpi ACTION=deploy
make rpi ACTION=status
make rpi ACTION=logs
```

仅修改运行配置时，无需重新编译：

```bash
make rpi ACTION=config
```

正常日志应包含 `Tuya MQTT connected`、`KNX tunnel connected`、
`Tuya gateway DP mode ready` 和 `Tuya MQTT subscriptions restored`。

公卫灯链路为：实体状态 `1/2/11 -> light_01_switch`；Smart Life 控制
`light_01_switch -> 1/2/12`；执行器再由 `1/2/11` 反馈最终状态。若灯动作但 UI 不变，
应检查状态对象是否可读及执行器是否发送反馈。

## 6. 面板开发与发布

```bash
make panel ACTION=install
make panel ACTION=check
make panel ACTION=dev
make panel ACTION=build
```

在 Tuya MiniApp IDE 导入 `panel/`，选择设备面板/Ray，关联网关 PID 和测试设备。完成真机
调试后上传体验版、提交审核并发布，再把面板关联到网关产品。平台需要将
`raw.githubusercontent.com` 加入请求合法域名。更多细节见 `panel/README.md`。

## 7. KNX 清单审核

推荐先使用树莓派内置现场识别台逐项确认。将 `config/config.json` 中
`commissioning.enabled` 设为 `true` 后部署：

```bash
make rpi ACTION=deploy
```

在同一可信局域网浏览器打开 `http://树莓派IP:8090`，填写房间、名称、类别和能力，再操作
物理按键。选择控制 GA、状态 GA 和 DPT 后确认，结果写到树莓派
`/opt/knx-tuya-gw/state/knx-commission-review.csv`。完成后下载并生成升级配置：

```bash
make rpi ACTION=review
make mapping ACTION=finalize VERSION=2026.09.09-2
make mapping ACTION=validate
```

KNX 报文本身不携带 ETS 名称和权威 DPT，页面展示的是可解码候选；名称、控制/状态职责、
DPT 和枚举含义必须结合现场现象确认。识别结束后应关闭 `commissioning.enabled` 并执行
`make rpi ACTION=config`，避免在局域网长期开放无认证页面。完整步骤见
`docs/commissioning.md`。

已有原始清单也可先批量导入：

把原始 Excel 放到 `data/` 后导入：

```bash
make mapping ACTION=import KNX_SOURCE="data/你的清单.xlsx"
```

在 `data/knx-mapping-review.csv` 中逐条现场确认：

- `passed`：物理状态、App 控制和最终反馈均正确，可发布。
- `pending`：尚未完成现场确认。
- `needs_manual`：多 GA、私有编码、复用或容量问题，需要人工处理。
- `skip`：不接入。
- `failed`：现场验证失败。

生成并校验升级映射：

```bash
make mapping ACTION=finalize VERSION=2026.09.09-2
make mapping ACTION=validate
```

输出为 `data/knx-mapping-for-upgrade.json`，只有 `passed` 行会进入结果。重复 DP、重复状态
GA、DPT 错误和槽位超限都会被拒绝。

## 8. GitHub 热更新

发布前确保本机 Git 已能访问 `mvptianyu/config-gateway`：

```bash
make publish
```

工具会临时克隆配置仓库，只把升级文件发布为 `knx/knx-mapping.json`，不会上传源码仓库
历史、密钥或 Excel。需要覆盖默认值时可直接传环境变量：

```bash
GITHUB_REMOTE=git@github.com:mvptianyu/config-gateway.git \
GITHUB_BRANCH=main \
GITHUB_BUNDLE_PATH=knx/knx-mapping.json \
make publish
```

网关启动时立即拉取，之后按 `remote_poll_seconds` 轮询；面板启动时拉取并定时刷新。预建
DP 槽位内增加设备、改名称或房间无需重新编译网关或发布面板。新增控件类型、改变 DP
Schema、超过槽位容量或修改远程 URL 时仍需更新平台或重新发布面板。

回滚时在配置仓库恢复上一版 `knx/knx-mapping.json` 并推送。

## 9. 常见问题

| 现象 | 处理 |
| --- | --- |
| 部署仍询问 SSH 密码 | 检查 `.env.rpi` 的路径和 `RPI_PASSWORD` 引号；或改用 SSH Key |
| sudo 仍询问密码 | 设置 `RPI_SUDO_PASSWORD`；留空只会复用 `RPI_PASSWORD` |
| KNX 扫描失败 | 在同网段树莓派扫描，检查组播和 UDP 3671，生产可固定 IP |
| Smart Life 网关离线 | 核对 MQTT、Device ID、broker 区域，并排除重复客户端 |
| 设备不存在功能点 | PID 未发布对应 DP，或 DP code/类型不一致 |
| 面板下发 `20028` | 核对体验设备授权、TuyaLink 物模型、PID 和面板版本 |
| 树莓派无下发日志 | 面板到涂鸦云链路或 Device ID 不一致 |
| HTTPS 更新不生效 | 检查 HTTP 200、JSON 版本、域名白名单、登录跳转和缓存 |
| 更新被拒绝 | 查看 `runtime mapping update rejected`，再执行本地映射校验 |
