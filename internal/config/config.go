// Package config loads and validates the runtime configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const TuyaModeGatewayDPs = "gateway_dps"

// Config 顶层配置
type Config struct {
	KNX           KNXConfig           `json:"knx"`
	TuyaMQTT      TuyaMQTTConfig      `json:"tuya_mqtt"`
	Poll          PollConfig          `json:"poll"`
	Commissioning CommissioningConfig `json:"commissioning"`
	RuntimeUpdate RuntimeUpdateConfig `json:"runtime_update"`
	MappingFile   string              `json:"mapping_file"`
	LogLevel      string              `json:"log_level"`
}

// KNXConfig KNX 连接与自动探测配置
type KNXConfig struct {
	// Gateway 固定网关地址；Host 为空时启用自动探测
	Gateway GatewayConfig `json:"gateway"`
	// Discovery 自动探测配置
	Discovery DiscoveryConfig `json:"discovery"`
}

// GatewayConfig 固定网关地址（手动指定时使用）
type GatewayConfig struct {
	Host string `json:"host"` // 如 "192.168.110.232"
	Port int    `json:"port"` // 默认 3671
}

// DiscoveryConfig KNXnet/IP 组播探测配置
type DiscoveryConfig struct {
	Enabled           bool   `json:"enabled"`
	MulticastGroup    string `json:"multicast_group"`    // 默认 224.0.23.12
	Port              int    `json:"port"`               // 默认 3671
	TimeoutSeconds    int    `json:"timeout_seconds"`    // 默认 5
	BroadcastFallback bool   `json:"broadcast_fallback"` // 组播无响应时尝试广播
}

// TuyaMQTTConfig configures direct TuyaLink MQTT access.
type TuyaMQTTConfig struct {
	Enabled           bool                `json:"enabled"`
	GatewayNodeID     string              `json:"gateway_node_id,omitempty"`
	ExpectedProductID string              `json:"expected_product_id,omitempty"`
	APIKey            string              `json:"api_key,omitempty"`
	Broker            string              `json:"broker"`
	CredentialsFile   string              `json:"credentials_file"`
	CAFile            string              `json:"ca_file,omitempty"`
	KeepAliveSeconds  int                 `json:"keep_alive_seconds"`
	ConnectTimeout    int                 `json:"connect_timeout_seconds"`
	GatewayDPPool     GatewayDPPoolConfig `json:"gateway_dp_pool,omitempty"`
}

// GatewayDPPoolConfig reserves a stable DP namespace on the gateway product.
type GatewayDPPoolConfig struct {
	Strict               bool           `json:"strict"`
	MaxFunctions         int            `json:"max_functions"`
	ModelPlanFile        string         `json:"model_plan_file"`
	PlatformTemplateFile string         `json:"platform_template_file"`
	PlatformXLSXFile     string         `json:"platform_xlsx_file"`
	Capacities           map[string]int `json:"capacities"`
}

// PollConfig 启动时对全部状态反馈组地址的轮询
type PollConfig struct {
	Enabled                 bool `json:"enabled"`
	IntervalMs              int  `json:"interval_ms"`                 // 默认 50，防 KNX 总线风暴
	AfterWriteDelayMs       int  `json:"after_write_delay_ms"`        // 默认 300，写入后回读状态地址
	ReportCommandOnWriteAck bool `json:"report_command_on_write_ack"` // 兼容无反馈设备；会把隧道写成功视为设备状态
}

// CommissioningConfig controls the local KNX capture and confirmation page.
type CommissioningConfig struct {
	Enabled    bool   `json:"enabled"`
	Listen     string `json:"listen"`
	ReviewFile string `json:"review_file"`
	MaxEvents  int    `json:"max_events"`
}

// RuntimeUpdateConfig controls local file watching and optional HTTPS delivery.
type RuntimeUpdateConfig struct {
	Enabled              bool   `json:"enabled"`
	WatchIntervalSeconds int    `json:"watch_interval_seconds"`
	RemoteURL            string `json:"remote_url,omitempty"`
	RemotePollSeconds    int    `json:"remote_poll_seconds"`
	MaxBytes             int64  `json:"max_bytes"`
}

// Default 返回带默认值的配置
func Default() *Config {
	return &Config{
		KNX: KNXConfig{
			Gateway: GatewayConfig{Host: "", Port: 3671},
			Discovery: DiscoveryConfig{
				Enabled:           true,
				MulticastGroup:    "224.0.23.12",
				Port:              3671,
				TimeoutSeconds:    5,
				BroadcastFallback: true,
			},
		},
		TuyaMQTT: TuyaMQTTConfig{
			Enabled:          true,
			GatewayNodeID:    "gateway",
			Broker:           "tls://m1.tuyacn.com:8883",
			CredentialsFile:  "tuya-mqtt.env",
			KeepAliveSeconds: 60,
			ConnectTimeout:   15,
			GatewayDPPool: GatewayDPPoolConfig{
				Strict:               true,
				MaxFunctions:         80,
				ModelPlanFile:        "../data/tuya-gateway-dp-plan.json",
				PlatformTemplateFile: "../data/tuya-register-dp-template.xlsx",
				PlatformXLSXFile:     "../data/tuya-gateway-dp-platform.xlsx",
				Capacities: map[string]int{
					"light":           24,
					"air_conditioner": 4,
					"scene":           23,
					"fresh_air":       1,
					"climate_sensor":  3,
				},
			},
		},
		Poll: PollConfig{
			Enabled:                 true,
			IntervalMs:              50,
			AfterWriteDelayMs:       300,
			ReportCommandOnWriteAck: false,
		},
		Commissioning: CommissioningConfig{
			Enabled:    false,
			Listen:     "0.0.0.0:8090",
			ReviewFile: "../state/knx-commission-review.csv",
			MaxEvents:  100,
		},
		RuntimeUpdate: RuntimeUpdateConfig{
			Enabled:              true,
			WatchIntervalSeconds: 3,
			RemotePollSeconds:    300,
			MaxBytes:             2 * 1024 * 1024,
		},
		MappingFile: "knx-mapping.json",
		LogLevel:    "info",
	}
}

// Load 从文件加载配置。缺失文件由 BootstrapRuntime 创建后再加载。
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	resolvePaths(path, cfg)
	return cfg, nil
}

func resolvePaths(configPath string, cfg *Config) {
	cfg.MappingFile = resolveRelativePath(configPath, cfg.MappingFile)
	cfg.TuyaMQTT.CredentialsFile = resolveRelativePath(configPath, cfg.TuyaMQTT.CredentialsFile)
	cfg.TuyaMQTT.GatewayDPPool.ModelPlanFile = resolveRelativePath(
		configPath,
		cfg.TuyaMQTT.GatewayDPPool.ModelPlanFile,
	)
	cfg.TuyaMQTT.GatewayDPPool.PlatformTemplateFile = resolveRelativePath(
		configPath,
		cfg.TuyaMQTT.GatewayDPPool.PlatformTemplateFile,
	)
	cfg.TuyaMQTT.GatewayDPPool.PlatformXLSXFile = resolveRelativePath(
		configPath,
		cfg.TuyaMQTT.GatewayDPPool.PlatformXLSXFile,
	)
	cfg.Commissioning.ReviewFile = resolveRelativePath(configPath, cfg.Commissioning.ReviewFile)
	if cfg.TuyaMQTT.CAFile != "" {
		cfg.TuyaMQTT.CAFile = resolveRelativePath(configPath, cfg.TuyaMQTT.CAFile)
	}
}

func resolveRelativePath(configPath, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(filepath.Dir(configPath), value)
}

// Validate 校验并填充默认值
func (c *Config) Validate() error {
	if c.KNX.Discovery.Port == 0 {
		c.KNX.Discovery.Port = 3671
	}
	if c.KNX.Gateway.Port == 0 {
		c.KNX.Gateway.Port = 3671
	}
	if c.KNX.Discovery.MulticastGroup == "" {
		c.KNX.Discovery.MulticastGroup = "224.0.23.12"
	}
	if c.KNX.Discovery.TimeoutSeconds <= 0 {
		c.KNX.Discovery.TimeoutSeconds = 5
	}
	if c.TuyaMQTT.Broker == "" {
		c.TuyaMQTT.Broker = "tls://m1.tuyacn.com:8883"
	}
	if c.TuyaMQTT.GatewayNodeID == "" {
		c.TuyaMQTT.GatewayNodeID = "gateway"
	}
	if c.TuyaMQTT.CredentialsFile == "" {
		c.TuyaMQTT.CredentialsFile = "tuya-mqtt.env"
	}
	if c.TuyaMQTT.KeepAliveSeconds <= 0 {
		c.TuyaMQTT.KeepAliveSeconds = 60
	}
	if c.TuyaMQTT.ConnectTimeout <= 0 {
		c.TuyaMQTT.ConnectTimeout = 15
	}
	if c.TuyaMQTT.GatewayDPPool.MaxFunctions <= 0 {
		c.TuyaMQTT.GatewayDPPool.MaxFunctions = 80
	}
	if c.TuyaMQTT.GatewayDPPool.ModelPlanFile == "" {
		c.TuyaMQTT.GatewayDPPool.ModelPlanFile = "../data/tuya-gateway-dp-plan.json"
	}
	if c.TuyaMQTT.GatewayDPPool.PlatformTemplateFile == "" {
		c.TuyaMQTT.GatewayDPPool.PlatformTemplateFile = "../data/tuya-register-dp-template.xlsx"
	}
	if c.TuyaMQTT.GatewayDPPool.PlatformXLSXFile == "" {
		c.TuyaMQTT.GatewayDPPool.PlatformXLSXFile = "../data/tuya-gateway-dp-platform.xlsx"
	}
	if c.TuyaMQTT.GatewayDPPool.Capacities == nil {
		c.TuyaMQTT.GatewayDPPool.Capacities = map[string]int{
			"light":           24,
			"air_conditioner": 4,
			"scene":           23,
			"fresh_air":       1,
			"climate_sensor":  3,
		}
	}
	for category, capacity := range c.TuyaMQTT.GatewayDPPool.Capacities {
		if capacity < 0 {
			return fmt.Errorf("tuya_mqtt.gateway_dp_pool.capacities.%s must not be negative", category)
		}
	}
	if c.Poll.IntervalMs <= 0 {
		c.Poll.IntervalMs = 50
	}
	if c.Poll.AfterWriteDelayMs < 0 {
		return fmt.Errorf("poll.after_write_delay_ms must be >= 0")
	}
	if c.Commissioning.Listen == "" {
		c.Commissioning.Listen = "0.0.0.0:8090"
	}
	if c.Commissioning.ReviewFile == "" {
		c.Commissioning.ReviewFile = "../state/knx-commission-review.csv"
	}
	if c.Commissioning.MaxEvents <= 0 {
		c.Commissioning.MaxEvents = 100
	}
	if c.RuntimeUpdate.WatchIntervalSeconds <= 0 {
		c.RuntimeUpdate.WatchIntervalSeconds = 3
	}
	if c.RuntimeUpdate.RemotePollSeconds <= 0 {
		c.RuntimeUpdate.RemotePollSeconds = 300
	}
	if c.RuntimeUpdate.MaxBytes <= 0 {
		c.RuntimeUpdate.MaxBytes = 2 * 1024 * 1024
	}
	if c.MappingFile == "" {
		c.MappingFile = "knx-mapping.json"
	}
	if c.KNX.Gateway.Host == "" && !c.KNX.Discovery.Enabled {
		return fmt.Errorf("neither knx.gateway.host nor knx.discovery.enabled is set")
	}
	return nil
}
