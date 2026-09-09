# KNX-Tuya Gateway

运行在树莓派上的泰创 KNXnet/IP 与 TuyaLink MQTT 双向网关。Smart Life 只注册一个虚拟
网关，灯光、空调、场景、新风和温湿度作为网关 DP，由 Ray 自定义面板组织成逻辑设备。

## 工程结构

```text
main.go, internal/             Go 网关服务
cmd/knx-mapping-tool/          KNX 清单导入、审核和映射校验工具
panel/                         Smart Life Ray 面板，独立安装和构建
vendor/                        Go 依赖本地副本，常规构建不联网下载
panel/node_modules/            面板依赖本地副本，依赖未变化时无需重装
config/                        本地与树莓派运行配置
data/                          本地 Excel、审核表和平台导入产物，不部署
tools/                         树莓派运维、Gitee 配置发布工具
deploy/                        systemd 服务与安装脚本
docs/                          接入手册和架构说明
```

`config/` 是运行边界，树莓派只接收其中必要的 `config.json`、`knx-mapping.json`、
`tuya-mqtt.env` 和可选 CA 文件。`data/` 是本地工作区，绝不部署到树莓派。

## 常用命令

```bash
make help
make check
make run
make gateway ACTION=discover|model|provision|tuya-check|validate
make rpi ACTION=build|deploy|config|status|logs|restart|info|review
make panel ACTION=install|dev|check|build
make mapping ACTION=import|finalize|validate
make publish
```

`vendor/` 和 `panel/node_modules/` 保留在工程目录。只有修改 Go 模块版本时才执行
`go mod vendor`；只有修改 `panel/package-lock.json` 时才执行 `make panel ACTION=install`。

TuyaLink 不能在运行时创建产品 DP。先执行 `make gateway ACTION=model`，再将生成的
`data/tuya-gateway-dp-platform.xlsx` 导入并发布到涂鸦平台。

从零接入、部署、验证、映射审核和热更新见 [docs/getting-started.md](docs/getting-started.md)；
现场逐项识别 KNX 组地址见 [docs/commissioning.md](docs/commissioning.md)；
实现边界见 [docs/architecture.md](docs/architecture.md)；面板开发见
[panel/README.md](panel/README.md)。
