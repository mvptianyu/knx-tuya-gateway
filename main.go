// knx-tuya-gw —— 泰创 KNX(IP) <-> 涂鸦 双向同步中转服务（Go 版）
//
// 数据流：
//
//	A. KNX -> 涂鸦App：KNX 总线事件（按键/执行器反馈）-> 匹配 status_ga ->
//	   TuyaLink MQTT 网关属性上报 -> 涂鸦云/App
//	B. 涂鸦App -> KNX：App 点击 -> TuyaLink MQTT property/set ->
//	   本服务 -> 查映射 -> KNX 组写 -> 泰创 TCP01RM -> KNX 总线执行器
//	C. 启动：遍历全部 status_ga 发 GroupValueRead 轮询上电初始状态（限速防总线风暴）
//
// 网关地址自动探测：局域网 KNXnet/IP 组播发现泰创网关；也可在运行配置中固定 host。
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"knx-tuya-gw/internal/commission"
	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/discovery"
	"knx-tuya-gw/internal/gatewaydps"
	"knx-tuya-gw/internal/knxclient"
	"knx-tuya-gw/internal/knxdebug"
	"knx-tuya-gw/internal/logger"
	"knx-tuya-gw/internal/mapping"
	"knx-tuya-gw/internal/tuyamqtt"
)

func main() {
	cfgPath := flag.String("config", config.DefaultRuntimeConfigPath, "path to runtime config")
	noTuya := flag.Bool("no-tuya", false, "run without Tuya MQTT (log only, for KNX-side testing)")
	validateOnly := flag.Bool("validate", false, "validate config and mapping, then exit")
	discoverOnly := flag.Bool("discover", false, "discover KNXnet/IP gateways, then exit")
	tuyaCheck := flag.Bool("tuya-check", false, "connect Tuya MQTT, restore gateway subscription, then exit")
	tuyaProvision := flag.Bool("tuya-provision", false, "activate a Tuya virtual gateway using TUYA_API_KEY")
	tuyaModelPlan := flag.Bool(
		"tuya-model-plan",
		false,
		"validate gateway DP slots and write product model files, then exit",
	)
	flag.Parse()

	created, err := config.BootstrapRuntime(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime configuration requires attention: %v\n", err)
		os.Exit(2)
	}
	if len(created) > 0 {
		fmt.Fprintln(os.Stderr, "runtime configuration templates created:")
		for _, path := range created {
			fmt.Fprintf(os.Stderr, "  - %s\n", path)
		}
		fmt.Fprintln(os.Stderr, "edit the generated files, then start the service again")
		os.Exit(2)
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}
	logger.SetLevel(cfg.LogLevel)

	if *tuyaProvision {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		apiKey := cfg.TuyaMQTT.APIKey
		if override := os.Getenv("TUYA_API_KEY"); override != "" {
			apiKey = override
		}
		result, err := tuyamqtt.ProvisionGateway(ctx, cfg.TuyaMQTT.CredentialsFile, apiKey)
		if err != nil {
			logger.Errorf("Tuya virtual gateway provisioning failed: %v", err)
			os.Exit(1)
		}
		logger.Infof(
			"Tuya virtual gateway activated: region=%s broker=%s device=%s",
			result.Region,
			result.Broker,
			result.DeviceID,
		)
		if result.ShortURL != "" {
			fmt.Printf("Smart Life binding URL: %s\n", result.ShortURL)
		}
		return
	}

	if *noTuya {
		cfg.TuyaMQTT.Enabled = false
		logger.Warnf("--no-tuya: running without Tuya cloud (KNX-side standalone mode)")
	}

	// 1. 加载映射表
	mappingStore, err := mapping.NewStore(cfg.MappingFile, func(candidate *mapping.Mapping) error {
		return validateTuyaModeMapping(cfg, candidate)
	})
	if err != nil {
		logger.Errorf("load mapping failed: %v", err)
		os.Exit(1)
	}
	m := mappingStore.Current()
	logger.Infof("mapping loaded: %d devices", len(m.Items))
	if cfg.TuyaMQTT.Enabled {
		logger.Infof("Tuya expected product: %s", cfg.TuyaMQTT.ExpectedProductID)
	}
	for _, it := range m.Items {
		logger.Debugf("  %s/%s %s [%s] -> tuya %s.%s(%s)",
			it.GA, it.StatusGA, it.Name, it.DPT, it.TuyaDevID, it.TuyaDPCode, it.TuyaDPType)
	}
	if *tuyaModelPlan {
		if !cfg.TuyaMQTT.Enabled {
			logger.Errorf("--tuya-model-plan requires tuya_mqtt.enabled=true")
			os.Exit(1)
		}
		artifacts, err := gatewaydps.Build(cfg.TuyaMQTT.GatewayDPPool, m.Items)
		if err != nil {
			logger.Errorf("build Tuya gateway DP artifacts failed: %v", err)
			os.Exit(1)
		}
		if err := gatewaydps.WriteArtifacts(cfg.TuyaMQTT.GatewayDPPool, artifacts); err != nil {
			logger.Errorf("write Tuya gateway DP artifacts failed: %v", err)
			os.Exit(1)
		}
		logger.Infof(
			"Tuya gateway DP pool: allocated=%d reserved=%d model=%s platform_xlsx=%s",
			artifacts.ModelPlan.AllocatedFunctions,
			artifacts.ModelPlan.ReservedFunctions,
			cfg.TuyaMQTT.GatewayDPPool.ModelPlanFile,
			cfg.TuyaMQTT.GatewayDPPool.PlatformXLSXFile,
		)
		return
	}
	if *validateOnly {
		logger.Infof(
			"configuration valid: config=%s mapping=%s mappings=%d",
			*cfgPath,
			cfg.MappingFile,
			len(m.Items),
		)
		return
	}
	if !*tuyaProvision && !*discoverOnly && !*noTuya && cfg.TuyaMQTT.Enabled {
		if err := tuyamqtt.ValidateCredentials(cfg.TuyaMQTT.CredentialsFile); err != nil {
			logger.Errorf("Tuya runtime credentials require attention: %v", err)
			logger.Errorf(
				"run with -tuya-provision or edit %s, then start again",
				cfg.TuyaMQTT.CredentialsFile,
			)
			os.Exit(2)
		}
	}
	if *discoverOnly {
		if err := discoverGateways(cfg); err != nil {
			logger.Errorf("discovery failed: %v", err)
			os.Exit(1)
		}
		return
	}
	if *tuyaCheck {
		if !cfg.TuyaMQTT.Enabled {
			logger.Errorf("tuya_mqtt.enabled must be true for --tuya-check")
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		tuyaClient, err := tuyamqtt.New(cfg.TuyaMQTT)
		if err != nil {
			logger.Errorf("create Tuya MQTT client failed: %v", err)
			os.Exit(1)
		}
		if err := tuyaClient.StartOnce(ctx); err != nil {
			logger.Errorf("Tuya MQTT check failed: %v", err)
			os.Exit(1)
		}
		time.Sleep(5 * time.Second)
		connected := tuyaClient.IsConnected()
		disconnects := tuyaClient.DisconnectCount()
		tuyaClient.Close()
		if !connected || disconnects > 0 {
			logger.Errorf(
				"Tuya MQTT check unstable: connected=%t disconnects=%d; stop other instances using this Device ID",
				connected,
				disconnects,
			)
			os.Exit(1)
		}
		logger.Infof("Tuya MQTT check passed")
		return
	}
	if len(m.Items) == 0 {
		logger.Errorf("KNX mapping is empty: edit %s, then start again", cfg.MappingFile)
		os.Exit(2)
	}
	runtimeLock, err := acquireRuntimeLock(*cfgPath)
	if err != nil {
		logger.Errorf("start runtime failed: %v", err)
		os.Exit(1)
	}
	defer runtimeLock.Close()

	// 2. 确定 KNX 网关地址（固定 host 优先，否则自动探测）
	gwAddr := resolveGateway(cfg)
	if gwAddr == "" {
		logger.Errorf("no KNX gateway resolved (set knx.gateway.host or enable discovery)")
		os.Exit(1)
	}
	logger.Infof("KNX gateway: %s", gwAddr)

	// 3. 创建 KNX 客户端。连接放到 MQTT 启动后执行，避免 KNX 故障拖垮 App 在线状态。
	client := knxclient.New(gwAddr)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var commissioning *commission.Server
	if cfg.Commissioning.Enabled {
		commissioning = commission.New(
			cfg.Commissioning,
			cfg.MappingFile,
			cfg.TuyaMQTT.GatewayDPPool.Capacities,
		)
		commissioning.SetBus(client)
		go func() {
			if err := commissioning.Start(ctx); err != nil {
				logger.Errorf("KNX commissioning server failed: %v", err)
			}
		}()
	}

	// 4. TuyaLink MQTT 优先连接，使 KNX 暂时不可用时网关仍能在 App 中保持在线。
	var tuyaClient *tuyamqtt.Client
	var debugController *knxdebug.Controller
	if cfg.TuyaMQTT.Enabled {
		tuyaClient, err = tuyamqtt.New(cfg.TuyaMQTT)
		if err != nil {
			logger.Errorf("create Tuya MQTT client failed: %v", err)
			os.Exit(1)
		}
		debugController = knxdebug.New(client, tuyaClient, cfg.TuyaMQTT.GatewayNodeID)
		tuyaClient.OnCommand(func(nodeID, dpCode string, value interface{}) error {
			if debugController.Handles(dpCode) {
				return debugController.Handle(dpCode, value)
			}
			if err := handleTuyaCmd(
				client,
				mappingStore.Current(),
				nodeID,
				dpCode,
				value,
				cfg.Poll.AfterWriteDelayMs,
			); err != nil {
				return err
			}
			if cfg.Poll.ReportCommandOnWriteAck {
				if err := tuyaClient.Report(nodeID, dpCode, value); err != nil {
					logger.Warnf(
						"Tuya command fallback report failed: %s.%s=%v: %v",
						nodeID,
						dpCode,
						value,
						err,
					)
				} else {
					logger.Infof(
						"Tuya command fallback accepted/queued: %s.%s=%v (awaiting KNX status feedback)",
						nodeID,
						dpCode,
						value,
					)
				}
			}
			return nil
		})
		if err := tuyaClient.Start(ctx); err != nil {
			if errors.Is(err, tuyamqtt.ErrInitialConnectPending) {
				logger.Warnf(
					"Tuya MQTT initial connection timed out; service remains running and retries every 5s: %v",
					err,
				)
			} else {
				logger.Errorf("start Tuya MQTT failed: %v", err)
				os.Exit(1)
			}
		}
		defer tuyaClient.Close()
	} else {
		logger.Warnf("Tuya MQTT disabled: KNX events will only be logged")
	}

	// 5. KNX 总线事件 -> 涂鸦上报
	client.OnEvent(func(ev knxclient.Event) {
		if commissioning != nil {
			commissioning.Capture(ev)
		}
		if debugController != nil {
			debugController.Capture(ev)
		}
		if ev.Command != "write" && ev.Command != "response" {
			return // 忽略读请求
		}
		updates, err := statusUpdatesForEvent(mappingStore.Current(), ev.GA, ev.Data)
		if err != nil {
			logger.Warnf("cannot process KNX value for %s raw=%s: %v", ev.GA, ev.RawHex, err)
			return
		}
		if len(updates) == 0 {
			logger.Debugf("KNX event on unmapped GA %s, ignore", ev.GA)
			return
		}
		for _, update := range updates {
			item := update.Item
			logger.Infof(
				"KNX %s=%v -> tuya %s.%s",
				ev.GA, update.Value, item.TuyaDevID, item.TuyaDPCode,
			)
			if tuyaClient != nil {
				if err := tuyaClient.Report(item.TuyaDevID, item.TuyaDPCode, update.Value); err != nil {
					logger.Warnf("Tuya property report failed: %v", err)
				}
			}
		}
	})

	// 6. KNX 在后台连接并持续重试；首次连通后再轮询状态。
	knxReady := connectKNXInBackground(ctx, client)
	if cfg.Poll.Enabled {
		go func() {
			select {
			case <-knxReady:
				pollAllStatus(client, mappingStore.Current(), cfg.Poll.IntervalMs)
			case <-ctx.Done():
			}
		}()
	}
	if cfg.RuntimeUpdate.Enabled {
		startRuntimeUpdates(ctx, cfg, mappingStore, client)
	}

	// 7. 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Infof("shutting down...")
	cancel()
	client.Close()
}

func startRuntimeUpdates(
	ctx context.Context,
	cfg *config.Config,
	store *mapping.Store,
	client *knxclient.Client,
) {
	apply := func(source string, changed bool, current *mapping.Mapping, err error) {
		applyRuntimeMappingUpdate(cfg, client, source, changed, current, err)
	}

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		defer signal.Stop(hup)
		ticker := time.NewTicker(time.Duration(cfg.RuntimeUpdate.WatchIntervalSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				changed, current, err := store.ReloadFromDisk()
				apply("local-file", changed, current, err)
			case <-hup:
				changed, current, err := store.ReloadFromDisk()
				apply("sighup", changed, current, err)
			}
		}
	}()

	if cfg.RuntimeUpdate.RemoteURL != "" {
		go func() {
			fetch := func() {
				data, err := fetchRuntimeBundle(
					ctx,
					cfg.RuntimeUpdate.RemoteURL,
					cfg.RuntimeUpdate.MaxBytes,
				)
				if err != nil {
					apply("remote-https", false, store.Current(), err)
					return
				}
				changed, current, err := store.Install(data)
				apply("remote-https", changed, current, err)
			}
			fetch()
			ticker := time.NewTicker(time.Duration(cfg.RuntimeUpdate.RemotePollSeconds) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					fetch()
				}
			}
		}()
	}

}

func applyRuntimeMappingUpdate(
	cfg *config.Config,
	client *knxclient.Client,
	source string,
	changed bool,
	current *mapping.Mapping,
	err error,
) {
	if err != nil {
		logger.Warnf("runtime mapping update rejected from %s; keeping previous version: %v", source, err)
		return
	}
	if !changed {
		logger.Debugf(
			"runtime mapping already current: source=%s version=%s",
			source,
			mappingVersion(current),
		)
		return
	}
	logger.Infof(
		"runtime mapping hot-reloaded: source=%s version=%s mappings=%d",
		source,
		mappingVersion(current),
		len(current.Items),
	)
	if cfg.Poll.Enabled {
		go pollAllStatus(client, current, cfg.Poll.IntervalMs)
	}
}

func fetchRuntimeBundle(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	return fetchRuntimeBundleWithClient(
		ctx,
		&http.Client{Timeout: 20 * time.Second},
		url,
		maxBytes,
	)
}

func fetchRuntimeBundleWithClient(
	ctx context.Context,
	client *http.Client,
	url string,
	maxBytes int64,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Cache-Control", "no-cache")
	request.Header.Set("Pragma", "no-cache")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("runtime bundle exceeds max_bytes=%d", maxBytes)
	}
	return unwrapRuntimeBundle(data, maxBytes)
}

func unwrapRuntimeBundle(data []byte, maxBytes int64) ([]byte, error) {
	var wrapper struct {
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil ||
		wrapper.Encoding != "base64" ||
		wrapper.Content == "" {
		return data, nil
	}

	content, err := base64.StdEncoding.DecodeString(wrapper.Content)
	if err != nil {
		return nil, fmt.Errorf("decode remote runtime bundle base64: %w", err)
	}
	if int64(len(content)) > maxBytes {
		return nil, fmt.Errorf("decoded runtime bundle exceeds max_bytes=%d", maxBytes)
	}
	return content, nil
}

func mappingVersion(m *mapping.Mapping) string {
	if m == nil || m.Version == "" {
		return "legacy"
	}
	return m.Version
}

func acquireRuntimeLock(configPath string) (*os.File, error) {
	lockPath := filepath.Join(filepath.Dir(configPath), ".knx-tuya-gw.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock %s: %w", lockPath, err)
	}
	if err := unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf(
			"another knx-tuya-gw instance is already using %s",
			configPath,
		)
	}
	return lockFile, nil
}

func connectKNXInBackground(ctx context.Context, client *knxclient.Client) <-chan struct{} {
	ready := make(chan struct{})
	go func() {
		for {
			if err := client.WaitUntilConnected(30 * time.Second); err == nil {
				close(ready)
				return
			} else {
				logger.Warnf(
					"KNX unavailable; Tuya MQTT remains online, retrying in 5s: %v",
					err,
				)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	return ready
}

func validateTuyaModeMapping(cfg *config.Config, m *mapping.Mapping) error {
	if !cfg.TuyaMQTT.Enabled {
		return nil
	}
	for _, item := range m.Items {
		if item.TuyaDevID != cfg.TuyaMQTT.GatewayNodeID {
			return fmt.Errorf(
				"gateway_dps mode requires tuya_dev_id %q, mapping %q uses %q",
				cfg.TuyaMQTT.GatewayNodeID,
				item.Name,
				item.TuyaDevID,
			)
		}
	}
	_, err := gatewaydps.Build(cfg.TuyaMQTT.GatewayDPPool, m.Items)
	if err != nil {
		return err
	}
	return nil
}

// resolveGateway 解析 KNX 网关地址：固定 host -> 自动探测。
func resolveGateway(cfg *config.Config) string {
	if cfg.KNX.Gateway.Host != "" {
		return fmt.Sprintf("%s:%d", cfg.KNX.Gateway.Host, cfg.KNX.Gateway.Port)
	}
	if !cfg.KNX.Discovery.Enabled {
		return ""
	}
	logger.Infof("auto-discovering KNX gateways on %s:%d (timeout %ds)...",
		cfg.KNX.Discovery.MulticastGroup, cfg.KNX.Discovery.Port, cfg.KNX.Discovery.TimeoutSeconds)
	opts := discovery.Options{
		MulticastGroup:    cfg.KNX.Discovery.MulticastGroup,
		Port:              cfg.KNX.Discovery.Port,
		Timeout:           time.Duration(cfg.KNX.Discovery.TimeoutSeconds) * time.Second,
		BroadcastFallback: cfg.KNX.Discovery.BroadcastFallback,
	}
	gws, err := discovery.Discover(opts)
	if err != nil {
		logger.Errorf("discovery error: %v", err)
		return ""
	}
	if len(gws) == 0 {
		logger.Errorf("no KNX gateway found; check network or set knx.gateway.host")
		return ""
	}
	for i, g := range gws {
		logger.Infof("found gateway #%d: %s (name=%q)", i+1, g.Addr, g.Name)
	}
	return gws[0].Addr
}

func discoverGateways(cfg *config.Config) error {
	opts := discovery.Options{
		MulticastGroup:    cfg.KNX.Discovery.MulticastGroup,
		Port:              cfg.KNX.Discovery.Port,
		Timeout:           time.Duration(cfg.KNX.Discovery.TimeoutSeconds) * time.Second,
		BroadcastFallback: cfg.KNX.Discovery.BroadcastFallback,
	}
	logger.Infof("discovering KNX gateways on %s:%d (timeout %ds)...",
		opts.MulticastGroup, opts.Port, cfg.KNX.Discovery.TimeoutSeconds)
	gateways, err := discovery.Discover(opts)
	if err != nil {
		return err
	}
	if len(gateways) == 0 {
		return fmt.Errorf("no KNX gateway found")
	}
	for i, gateway := range gateways {
		logger.Infof("gateway #%d: %s name=%q", i+1, gateway.Addr, gateway.Name)
	}
	return nil
}

// handleTuyaCmd 处理涂鸦App下发的控制指令，转换为 KNX 组写。
func handleTuyaCmd(
	client *knxclient.Client,
	m *mapping.Mapping,
	nodeID,
	dpCode string,
	value interface{},
	afterWriteDelayMs int,
) error {
	item := m.ByTuya(nodeID, dpCode)
	if item == nil {
		return fmt.Errorf("no KNX mapping for %s.%s", nodeID, dpCode)
	}
	knxValue := denormalizeKNXValue(item, value)
	logger.Infof("tuya %s.%s=%v -> KNX %s(%s)=%v", nodeID, dpCode, value, item.GA, item.DPT, knxValue)
	if err := client.WriteValue(item.GA, item.DPT, knxValue); err != nil {
		return fmt.Errorf("KNX write %s: %w", item.GA, err)
	}
	logger.Infof("KNX write acknowledged by tunnel: ga=%s status_ga=%s", item.GA, item.StatusGA)
	if afterWriteDelayMs > 0 && item.StatusGA != "" {
		statusGA := item.StatusGA
		delay := time.Duration(afterWriteDelayMs) * time.Millisecond
		go func() {
			time.Sleep(delay)
			logger.Infof("reading KNX status after write: ga=%s", statusGA)
			if err := client.ReadValue(statusGA); err != nil {
				logger.Warnf("post-write status read %s failed: %v", statusGA, err)
			}
		}()
	}
	return nil
}

// pollAllStatus 启动轮询全部状态反馈组地址，限速防总线风暴。
func pollAllStatus(client *knxclient.Client, m *mapping.Mapping, intervalMs int) {
	gas := m.AllStatusGAs()
	if len(gas) == 0 {
		return
	}
	logger.Infof("polling %d status GAs...", len(gas))
	for i, ga := range gas {
		if err := client.ReadValue(ga); err != nil {
			logger.Warnf("poll read %s failed: %v", ga, err)
		}
		if i < len(gas)-1 {
			time.Sleep(time.Duration(intervalMs) * time.Millisecond)
		}
	}
	logger.Infof("status polling done")
}

type statusUpdate struct {
	Item  *mapping.Item
	Value interface{}
}

func statusUpdatesForEvent(m *mapping.Mapping, ga string, data []byte) ([]statusUpdate, error) {
	items := m.ByStatusGA(ga)
	updates := make([]statusUpdate, 0, len(items))
	for _, item := range items {
		decoded, err := knxclient.DecodeValue(item.DPT, data)
		if err != nil {
			return nil, fmt.Errorf("%s(%s): %w", ga, item.DPT, err)
		}

		if strings.EqualFold(strings.TrimSpace(item.Category), "scene") {
			if !sameScalarValue(decoded, item.KNXWriteValue) {
				continue
			}
			// Scene DPs are triggers: scene number 0 still means the matched DP was activated.
			updates = append(updates, statusUpdate{Item: item, Value: true})
			continue
		}

		value := normalizeTuyaValue(decoded, item)
		if value == nil {
			return nil, fmt.Errorf(
				"cannot normalize %s as %s, decoded=%v",
				item.Name, item.TuyaDPType, decoded,
			)
		}
		updates = append(updates, statusUpdate{Item: item, Value: value})
	}
	return updates, nil
}

func sameScalarValue(left, right interface{}) bool {
	leftNumber, leftOK := numericValue(left)
	rightNumber, rightOK := numericValue(right)
	if leftOK || rightOK {
		return leftOK && rightOK && leftNumber == rightNumber
	}
	return valueMapKey(left) == valueMapKey(right)
}

// normalizeTuyaValue 按映射项的涂鸦 DP 类型规范化已解码的 KNX 值。
func normalizeTuyaValue(decoded interface{}, item *mapping.Item) interface{} {
	if mapped, ok := item.KNXToTuya[valueMapKey(decoded)]; ok {
		return mapped
	}
	scaled := scaleNumber(decoded, item.TuyaScale)
	switch item.TuyaDPType {
	case "bool":
		switch value := scaled.(type) {
		case bool:
			return value
		case int:
			return value != 0
		case float64:
			return value != 0
		}
	case "percent", "int", "value":
		switch value := scaled.(type) {
		case int:
			return value
		case float64:
			return int(math.Round(value))
		case bool:
			if value {
				return 1
			}
			return 0
		}
	case "enum", "string":
		if value, ok := scaled.(string); ok {
			return value
		}
	default:
		return scaled
	}
	return nil
}

func denormalizeKNXValue(item *mapping.Item, value interface{}) interface{} {
	if item.KNXWriteValue != nil {
		return item.KNXWriteValue
	}
	if mapped, ok := item.TuyaToKNX[valueMapKey(value)]; ok {
		return mapped
	}
	return scaleNumber(value, -item.TuyaScale)
}

func scaleNumber(value interface{}, decimalPlaces int) interface{} {
	if decimalPlaces == 0 {
		return value
	}
	number, ok := numericValue(value)
	if !ok {
		return value
	}
	factor := math.Pow10(decimalPlaces)
	return number * factor
}

func numericValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func valueMapKey(value interface{}) string {
	if number, ok := numericValue(value); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprint(value)
}
