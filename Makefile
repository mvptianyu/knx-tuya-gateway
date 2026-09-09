SHELL := /bin/bash
export GOCACHE ?= /tmp/knx-tuya-go-cache
export GOFLAGS ?= -mod=vendor

ACTION ?=
KNX_SOURCE ?= data/KNX设备清单_v10 - 副本.xlsx
KNX_REVIEW ?= $(if $(wildcard data/knx-commission-review.csv),data/knx-commission-review.csv,data/knx-mapping-review.csv)
UPGRADE_BUNDLE ?= data/knx-mapping-for-upgrade.json
VERSION ?= $(shell date +%Y.%m.%d-%H%M%S)

.PHONY: help check run gateway rpi panel mapping publish

help:
	@printf '%s\n' \
	  'make check                         全量测试和配置检查' \
	  'make run                           本地运行网关服务' \
	  'make gateway ACTION=discover       扫描 KNX 网关' \
	  'make gateway ACTION=model          生成涂鸦 DP 平台文件' \
	  'make gateway ACTION=provision      激活 TuyaLink 网关' \
	  'make gateway ACTION=tuya-check     检查 Tuya MQTT' \
	  'make rpi ACTION=deploy             编译并部署树莓派' \
	  'make rpi ACTION=config             仅同步运行配置' \
	  'make rpi ACTION=review             下载现场确认 CSV' \
	  'make rpi ACTION=status|logs|restart|info 线上运维' \
	  'make panel ACTION=dev|check|build  小程序开发、检查、构建' \
	  'make mapping ACTION=import         导入 KNX Excel' \
	  'make mapping ACTION=finalize       生成已审核升级配置' \
	  'make mapping ACTION=validate       校验升级配置' \
	  'make publish                       发布配置到 GitHub'

check:
	go test ./...
	go run . -config config/config.json -validate
	cd panel && npm run typecheck

run:
	go run . -config config/config.json

gateway:
	@case "$(ACTION)" in \
	  discover) go run . -config config/config.json -discover ;; \
	  model) go run . -config config/config.json -tuya-model-plan ;; \
	  provision) go run . -config config/config.json -tuya-provision ;; \
	  tuya-check) go run . -config config/config.json -tuya-check ;; \
	  validate|"") go run . -config config/config.json -validate ;; \
	  *) echo "ACTION must be discover, model, provision, tuya-check, or validate" >&2; exit 2 ;; \
	esac

rpi:
	./tools/rpi.sh "$(or $(ACTION),deploy)"

panel:
	@case "$(ACTION)" in \
	  install) cd panel && npm install ;; \
	  dev) cd panel && npm run start:web ;; \
	  build) cd panel && npm run build ;; \
	  check|"") cd panel && npm run typecheck ;; \
	  *) echo "ACTION must be install, dev, check, or build" >&2; exit 2 ;; \
	esac

mapping:
	@case "$(ACTION)" in \
	  import) go run ./cmd/knx-mapping-tool import \
	    -input "$(KNX_SOURCE)" -review "$(KNX_REVIEW)" \
	    -base config/knx-mapping.json -config config/config.json ;; \
	  finalize) go run ./cmd/knx-mapping-tool finalize \
	    -review "$(KNX_REVIEW)" -base config/knx-mapping.json \
	    -output "$(UPGRADE_BUNDLE)" -version "$(VERSION)" \
	    -config config/config.json ;; \
	  validate|"") go run ./cmd/knx-mapping-tool validate \
	    -input "$(UPGRADE_BUNDLE)" -config config/config.json ;; \
	  *) echo "ACTION must be import, finalize, or validate" >&2; exit 2 ;; \
	esac

publish:
	./tools/publish-config.sh
