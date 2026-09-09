#!/usr/bin/env bash
# 树莓派部署脚本（root 执行）：
#   把项目目录拷贝到 /opt/knx-tuya-gw 并注册 systemd 服务
set -euo pipefail

APP_DIR="${RPI_APP_DIR:-/opt/knx-tuya-gw}"
BIN_NAME=knx-tuya-gw

echo "==> 1/4 安装程序与 config 运行配置"
install -d -m 0755 "$APP_DIR"
install -d -m 0755 "$APP_DIR/state"
# 本脚本应位于项目 deploy/ 下，项目根为上级目录
SRC_DIR="$(cd "$(dirname "$0")/.." && pwd)"
# 二进制：优先用预编译 arm64 版本
if [ -f "$SRC_DIR/bin/$BIN_NAME-arm64" ]; then
  install -m 0755 "$SRC_DIR/bin/$BIN_NAME-arm64" "$APP_DIR/$BIN_NAME.new"
  mv "$APP_DIR/$BIN_NAME.new" "$APP_DIR/$BIN_NAME"
else
  echo "!! 未找到 bin/$BIN_NAME-arm64，请先执行 make rpi ACTION=build"
  exit 1
fi
install -d -m 0700 "$APP_DIR/config"
if [ -f "$SRC_DIR/config/config.json" ]; then
  install -m 0600 "$SRC_DIR/config/config.json" "$APP_DIR/config/config.json"
fi
if [ -f "$SRC_DIR/config/knx-mapping.json" ]; then
  install -m 0644 "$SRC_DIR/config/knx-mapping.json" "$APP_DIR/config/knx-mapping.json"
fi

has_tuya_credentials() {
  local path="$1"
  [ -f "$path" ] &&
    grep -Eq '^TUYA_PRODUCT_ID=.+$' "$path" &&
    grep -Eq '^TUYA_DEVICE_ID=.+$' "$path" &&
    grep -Eq '^TUYA_DEVICE_SECRET=.+$' "$path"
}

if has_tuya_credentials "$APP_DIR/config/tuya-mqtt.env"; then
  echo "    保留现有 Tuya 网关凭证: $APP_DIR/config/tuya-mqtt.env"
elif has_tuya_credentials "$APP_DIR/data/tuya-mqtt.env"; then
  install -m 0600 "$APP_DIR/data/tuya-mqtt.env" "$APP_DIR/config/tuya-mqtt.env"
  echo "    已迁移旧 Tuya 网关凭证到: $APP_DIR/config/tuya-mqtt.env"
elif [ -f "$SRC_DIR/config/tuya-mqtt.env" ]; then
  install -m 0600 "$SRC_DIR/config/tuya-mqtt.env" "$APP_DIR/config/tuya-mqtt.env"
elif [ -f "$APP_DIR/config/tuya-mqtt.env" ]; then
  echo "    保留待填写的 Tuya 网关凭证模板: $APP_DIR/config/tuya-mqtt.env"
else
  echo "    未提供 Tuya 网关凭证；服务首次启动会创建可编辑模板"
fi
# The deployment package contains only the CA file selected by config.json.
find "$APP_DIR/config" -maxdepth 1 \( -name '*.cer' -o -name '*.pem' \) -type f -delete
find "$SRC_DIR/config" -maxdepth 1 \( -name '*.cer' -o -name '*.pem' \) -type f \
  -exec install -m 0644 {} "$APP_DIR/config/" \;
# Remove the retired data directory after migrating its credentials.
rm -rf "$APP_DIR/data"

echo "==> 2/4 校验架构"
file "$APP_DIR/$BIN_NAME" | grep -q aarch64 || { echo "!! 二进制不是 aarch64"; exit 1; }

echo "==> 3/4 注册 systemd 服务"
SERVICE_TMP="$(mktemp)"
sed "s|/opt/knx-tuya-gw|$APP_DIR|g" "$SRC_DIR/deploy/knx-tuya-bridge.service" > "$SERVICE_TMP"
install -m 0644 "$SERVICE_TMP" /etc/systemd/system/knx-tuya-bridge.service
rm -f "$SERVICE_TMP"
systemctl daemon-reload
systemctl enable knx-tuya-bridge.service

echo "==> 4/4 完成"
echo "启动:   systemctl start knx-tuya-bridge"
echo "日志:   journalctl -u knx-tuya-bridge -f"
echo "采集:   http://树莓派IP:8090（仅在 commissioning.enabled=true 时开放）"
echo "缺少运行配置时，服务会在 $APP_DIR/config 生成模板并停止，修改后再启动。"
echo
echo "Tuya MQTT 自检:"
echo "      $APP_DIR/$BIN_NAME -config $APP_DIR/config/config.json -tuya-check"
echo "仅验证 KNX 侧:"
echo "      $APP_DIR/$BIN_NAME -config $APP_DIR/config/config.json -no-tuya"
