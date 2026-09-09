// Package knxclient 封装 knx-go 隧道客户端（泰创 TCP01RM Tunneling）。
//
// 提供：
//   - 连接（knx-go 内部自带断线重连）
//   - 按 DPT 编码下发组写（涂鸦指令 -> KNX）
//   - 读请求（启动状态轮询）
//   - 总线事件订阅（KNX -> 涂鸦上报）
package knxclient

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vapourismo/knx-go/knx"
	"github.com/vapourismo/knx-go/knx/cemi"
	"github.com/vapourismo/knx-go/knx/dpt"

	"knx-tuya-gw/internal/logger"
)

// DPT 常量（映射表 dpt 字段取值）
const (
	DPT1_001  = "DPT-1.001"  // 开关
	DPT5_001  = "DPT-5.001"  // 0-100%
	DPT5_005  = "DPT-5.005"  // 0-255
	DPT9_001  = "DPT-9.001"  // 温度 °C
	DPT9_007  = "DPT-9.007"  // 湿度 %
	DPT17_001 = "DPT-17.001" // 场景号 0-63
	DPT20_105 = "DPT-20.105" // HVAC 控制模式
)

// Event 归一化后的 KNX 总线事件
type Event struct {
	Source  string // 源地址（物理地址）
	GA      string // 组地址
	Command string // write | response | read
	RawHex  string // 原始数据十六进制
	Data    []byte // APDU 数据，必须结合映射中的 DPT 解码
}

type DecodedCandidate struct {
	DPT   string      `json:"dpt"`
	Value interface{} `json:"value"`
}

// Client KNX 隧道客户端
type Client struct {
	addr string
	ctx  context.Context

	mu     sync.Mutex
	tunnel knx.GroupTunnel // 值类型，内部持有 *Tunnel

	eventCb func(Event)
}

// New 创建客户端（不连接，调用 Connect）。
func New(addr string) *Client {
	return &Client{addr: addr, ctx: context.Background()}
}

// Addr 返回目标网关地址
func (c *Client) Addr() string { return c.addr }

// Connect 建立 Tunneling 连接并启动事件监听。
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 已在连接中（tunnel 非零）则不重复连接
	if c.tunnel.Tunnel != nil {
		return nil
	}
	gt, err := knx.NewGroupTunnel(c.addr, knx.TunnelConfig{})
	if err != nil {
		return fmt.Errorf("knx tunnel %s: %w", c.addr, err)
	}
	c.tunnel = gt
	logger.Infof("KNX tunnel connected: %s", c.addr)
	go c.dispatch(gt)
	return nil
}

// Close 关闭连接。
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tunnel.Tunnel != nil {
		c.tunnel.Close()
		c.tunnel = knx.GroupTunnel{}
	}
}

// OnEvent 注册总线事件回调（单消费者）。
func (c *Client) OnEvent(cb func(Event)) { c.eventCb = cb }

// dispatch 从隧道入站通道读取事件并归一化回调。
func (c *Client) dispatch(gt knx.GroupTunnel) {
	in := gt.Inbound()
	for {
		select {
		case <-c.ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			evt := normalizeEvent(ev)
			if c.eventCb != nil {
				c.eventCb(evt)
			}
		}
	}
}

// WriteValue 按映射项 DPT 编码并下发组写。
func (c *Client) WriteValue(ga string, dptType string, value interface{}) error {
	c.mu.Lock()
	gt := c.tunnel
	c.mu.Unlock()
	if gt.Tunnel == nil {
		return fmt.Errorf("knx not connected")
	}
	g, err := cemi.NewGroupAddrString(ga)
	if err != nil {
		return fmt.Errorf("bad group address %q: %w", ga, err)
	}
	data, err := encodeDpt(dptType, value)
	if err != nil {
		return err
	}
	ev := knx.GroupEvent{
		Command:     knx.GroupWrite,
		Destination: g,
		Data:        data,
	}
	logger.Debugf("KNX write %s(%s) = %v [% x]", ga, dptType, value, data)
	return gt.Send(ev)
}

// ReadValue 发送组读请求（用于启动轮询）。
func (c *Client) ReadValue(ga string) error {
	c.mu.Lock()
	gt := c.tunnel
	c.mu.Unlock()
	if gt.Tunnel == nil {
		return fmt.Errorf("knx not connected")
	}
	g, err := cemi.NewGroupAddrString(ga)
	if err != nil {
		return fmt.Errorf("bad group address %q: %w", ga, err)
	}
	ev := knx.GroupEvent{Command: knx.GroupRead, Destination: g}
	logger.Debugf("KNX read %s", ga)
	return gt.Send(ev)
}

// encodeDpt 按 DPT 类型编码值为 cEMI 数据字节。
func encodeDpt(dptType string, value interface{}) ([]byte, error) {
	switch dptType {
	case DPT1_001, "DPT-1", "1.001":
		v, ok := toBool(value)
		if !ok {
			return nil, fmt.Errorf("DPT-1.001 expects bool, got %T(%v)", value, value)
		}
		return dpt.DPT_1001(v).Pack(), nil
	case DPT5_001, "DPT-5", "5.001":
		n, ok := toFloat(value)
		if !ok {
			return nil, fmt.Errorf("%s expects number, got %T(%v)", dptType, value, value)
		}
		if n < 0 || n > 100 {
			return nil, fmt.Errorf("%s value %.2f out of range [0,100]", dptType, n)
		}
		return dpt.DPT_5001(float32(n)).Pack(), nil
	case DPT5_005, "5.005":
		n, ok := toInt(value)
		if !ok {
			return nil, fmt.Errorf("%s expects number, got %T(%v)", dptType, value, value)
		}
		if n < 0 || n > 255 {
			return nil, fmt.Errorf("%s value %d out of range [0,255]", dptType, n)
		}
		return dpt.DPT_5005(uint8(n)).Pack(), nil
	case DPT9_001, "9.001":
		n, ok := toFloat(value)
		if !ok {
			return nil, fmt.Errorf("%s expects number, got %T(%v)", dptType, value, value)
		}
		return dpt.DPT_9001(float32(n)).Pack(), nil
	case DPT9_007, "9.007":
		n, ok := toFloat(value)
		if !ok {
			return nil, fmt.Errorf("%s expects number, got %T(%v)", dptType, value, value)
		}
		if n < 0 || n > 100 {
			return nil, fmt.Errorf("%s value %.2f out of range [0,100]", dptType, n)
		}
		return dpt.DPT_9007(float32(n)).Pack(), nil
	case DPT17_001, "17.001":
		n, ok := toInt(value)
		if !ok {
			return nil, fmt.Errorf("%s expects integer, got %T(%v)", dptType, value, value)
		}
		if n < 0 || n > 63 {
			return nil, fmt.Errorf("%s value %d out of range [0,63]", dptType, n)
		}
		return dpt.DPT_17001(uint8(n)).Pack(), nil
	case DPT20_105, "20.105":
		n, ok := toInt(value)
		if !ok {
			return nil, fmt.Errorf("%s expects integer, got %T(%v)", dptType, value, value)
		}
		if n < 0 || n > 255 {
			return nil, fmt.Errorf("%s value %d out of range [0,255]", dptType, n)
		}
		return dpt.DPT_20105(uint8(n)).Pack(), nil
	default:
		return nil, fmt.Errorf("unsupported DPT %q", dptType)
	}
}

// normalizeEvent 把 knx-go 事件归一化为统一 Event。
func normalizeEvent(ev knx.GroupEvent) Event {
	return Event{
		Source:  ev.Source.String(),
		GA:      ev.Destination.String(),
		Command: commandName(ev.Command),
		RawHex:  fmt.Sprintf("% x", ev.Data),
		Data:    append([]byte(nil), ev.Data...),
	}
}

// DecodeValue 按映射中的 DPT 解码 KNX APDU 数据。
func DecodeValue(dptType string, data []byte) (interface{}, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%s has empty payload", dptType)
	}
	switch dptType {
	case DPT1_001, "DPT-1", "1.001":
		var value dpt.DPT_1001
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return bool(value), nil
	case DPT5_001, "DPT-5", "5.001":
		var value dpt.DPT_5001
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return int(float32(value) + 0.5), nil
	case DPT5_005, "5.005":
		var value dpt.DPT_5005
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return int(value), nil
	case DPT9_001, "9.001":
		var value dpt.DPT_9001
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return float64(value), nil
	case DPT9_007, "9.007":
		var value dpt.DPT_9007
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return float64(value), nil
	case DPT17_001, "17.001":
		var value dpt.DPT_17001
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return int(value), nil
	case DPT20_105, "20.105":
		var value dpt.DPT_20105
		if err := value.Unpack(data); err != nil {
			return nil, fmt.Errorf("decode %s: %w", dptType, err)
		}
		return int(value), nil
	default:
		return nil, fmt.Errorf("unsupported DPT %q", dptType)
	}
}

// DecodeCandidates exposes plausible interpretations; KNX telegrams do not carry DPT metadata.
func DecodeCandidates(data []byte) []DecodedCandidate {
	types := []string{
		DPT1_001, DPT5_001, DPT5_005, DPT9_001, DPT9_007, DPT17_001, DPT20_105,
	}
	candidates := make([]DecodedCandidate, 0, len(types))
	for _, dptType := range types {
		value, err := DecodeValue(dptType, data)
		if err == nil {
			candidates = append(candidates, DecodedCandidate{DPT: dptType, Value: value})
		}
	}
	return candidates
}

func commandName(c knx.GroupCommand) string {
	switch c {
	case knx.GroupWrite:
		return "write"
	case knx.GroupResponse:
		return "response"
	case knx.GroupRead:
		return "read"
	default:
		return "unknown"
	}
}

func toBool(v interface{}) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case int:
		return t != 0, true
	case uint8:
		return t != 0, true
	case string:
		switch strings.ToLower(t) {
		case "true", "1", "on":
			return true, true
		case "false", "0", "off":
			return false, true
		}
	}
	return false, false
}

func toInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int32:
		return int(t), true
	case int64:
		return int(t), true
	case uint8:
		return int(t), true
	case uint16:
		return int(t), true
	case uint32:
		return int(t), true
	case float64:
		return int(t), true
	case float32:
		return int(t), true
	case string:
		var n int
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case int:
		return float64(t), true
	case int32:
		return float64(t), true
	case int64:
		return float64(t), true
	case uint8:
		return float64(t), true
	case uint16:
		return float64(t), true
	case uint32:
		return float64(t), true
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case string:
		var n float64
		if _, err := fmt.Sscanf(t, "%f", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

// WaitUntilConnected 阻塞直至连接成功（带超时重试），用于启动阶段。
func (c *Client) WaitUntilConnected(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		err := c.Connect()
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("connect timeout: %w", lastErr)
}
