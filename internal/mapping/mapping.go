// Package mapping 加载并查询 KNX 组地址 <-> 涂鸦网关 DP 映射表。
//
// 映射表由 ETS 工程（.knxproj）解析生成，字段说明：
//
//	ga          KNX 控制组地址（涂鸦指令下发到该地址）
//	status_ga   KNX 状态反馈组地址（ETS 中执行器的 Status 对象，上报状态只用它）
//	name        设备名（仅展示）
//	dpt         数据类型，支持开关、百分比、字节、温湿度、场景号和 HVAC 模式
//	tuya_dev_id 网关节点名，默认 gateway
//	tuya_dp_code 涂鸦功能点编码（与平台功能定义一致）
//	tuya_dp_type bool | percent | int（用于值规范化）
//	category/slot/capability 描述 gateway_dps 模式的固定 DP 槽位
//	room        面板展示和筛选使用的房间区域，缺省为“全屋”
package mapping

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const BundleSchemaVersion = 1

// Item 一条映射记录
type Item struct {
	GA              string                 `json:"ga"`
	StatusGA        string                 `json:"status_ga"`
	Name            string                 `json:"name"`
	DPT             string                 `json:"dpt"`
	TuyaDevID       string                 `json:"tuya_dev_id"`
	TuyaDPCode      string                 `json:"tuya_dp_code"`
	TuyaDPType      string                 `json:"tuya_dp_type"`
	VirtualDeviceID string                 `json:"virtual_device_id,omitempty"`
	Category        string                 `json:"category,omitempty"`
	Room            string                 `json:"room,omitempty"`
	Slot            int                    `json:"slot,omitempty"`
	Capability      string                 `json:"capability,omitempty"`
	TuyaScale       int                    `json:"tuya_scale,omitempty"`
	KNXWriteValue   interface{}            `json:"knx_write_value,omitempty"`
	TuyaToKNX       map[string]interface{} `json:"tuya_to_knx,omitempty"`
	KNXToTuya       map[string]interface{} `json:"knx_to_tuya,omitempty"`
}

// Mapping 映射表
type Mapping struct {
	Version    string
	Items      []Item
	byTuya     map[string]*Item // key: devID + "\x00" + dpCode
	byStatusGA map[string]*Item // key: status_ga
}

// Bundle is the versioned runtime source shared by the gateway and panel.
type Bundle struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	Mappings      []Item `json:"mappings"`
}

// Load 从 JSON 文件加载映射表并建索引。
func Load(path string) (*Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping %s: %w", path, err)
	}
	m, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("parse mapping %s: %w", path, err)
	}
	return m, nil
}

// Decode accepts both the legacy array and the unified runtime bundle.
func Decode(data []byte) (*Mapping, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty mapping")
	}
	if trimmed[0] == '[' {
		var items []Item
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		return New(items)
	}

	var bundle Bundle
	if err := json.Unmarshal(trimmed, &bundle); err != nil {
		return nil, err
	}
	if bundle.SchemaVersion != BundleSchemaVersion {
		return nil, fmt.Errorf(
			"unsupported runtime bundle schema_version %d, want %d",
			bundle.SchemaVersion,
			BundleSchemaVersion,
		)
	}
	if strings.TrimSpace(bundle.Version) == "" {
		return nil, fmt.Errorf("runtime bundle version is required")
	}
	m, err := New(bundle.Mappings)
	if err != nil {
		return nil, err
	}
	m.Version = bundle.Version
	return m, nil
}

// New 由条目构建映射表索引。
func New(items []Item) (*Mapping, error) {
	m := &Mapping{
		Items:      items,
		byTuya:     make(map[string]*Item, len(items)),
		byStatusGA: make(map[string]*Item, len(items)),
	}
	for i := range items {
		it := &items[i]
		if it.GA == "" || it.StatusGA == "" || it.DPT == "" || it.TuyaDevID == "" || it.TuyaDPCode == "" {
			return nil, fmt.Errorf("mapping item %d missing required field: %+v", i, it)
		}
		if !supportedDPT(it.DPT) {
			return nil, fmt.Errorf("mapping item %d has unsupported dpt %q", i, it.DPT)
		}
		if !supportedDPType(it.TuyaDPType) {
			return nil, fmt.Errorf("mapping item %d has unsupported tuya_dp_type %q", i, it.TuyaDPType)
		}
		tuyaKey := it.TuyaDevID + "\x00" + it.TuyaDPCode
		if previous := m.byTuya[tuyaKey]; previous != nil {
			return nil, fmt.Errorf("duplicate tuya mapping %s.%s (%q and %q)",
				it.TuyaDevID, it.TuyaDPCode, previous.Name, it.Name)
		}
		if previous := m.byStatusGA[it.StatusGA]; previous != nil {
			return nil, fmt.Errorf("duplicate status_ga %s (%q and %q)", it.StatusGA, previous.Name, it.Name)
		}
		m.byStatusGA[it.StatusGA] = it
		m.byTuya[tuyaKey] = it
	}
	return m, nil
}

func supportedDPT(value string) bool {
	switch strings.ToUpper(value) {
	case "DPT-1.001", "DPT-1", "1.001",
		"DPT-5.001", "DPT-5", "5.001", "DPT-5.005", "5.005",
		"DPT-9.001", "9.001", "DPT-9.007", "9.007",
		"DPT-17.001", "17.001", "DPT-20.105", "20.105":
		return true
	default:
		return false
	}
}

func supportedDPType(value string) bool {
	switch strings.ToLower(value) {
	case "bool", "percent", "int", "value", "enum", "string":
		return true
	default:
		return false
	}
}

// ByTuya 按网关节点名和 DP 码查找（涂鸦指令 -> KNX 控制地址）。
func (m *Mapping) ByTuya(devID, dpCode string) *Item {
	return m.byTuya[devID+"\x00"+dpCode]
}

// ByStatusGA 按 KNX 状态反馈组地址查找（KNX 事件 -> 涂鸦上报）。
func (m *Mapping) ByStatusGA(ga string) *Item {
	return m.byStatusGA[ga]
}

// AllStatusGAs 返回全部状态反馈组地址（用于启动轮询）。
func (m *Mapping) AllStatusGAs() []string {
	out := make([]string, 0, len(m.Items))
	seen := make(map[string]struct{}, len(m.Items))
	for _, it := range m.Items {
		if _, ok := seen[it.StatusGA]; ok {
			continue
		}
		seen[it.StatusGA] = struct{}{}
		out = append(out, it.StatusGA)
	}
	return out
}

// Store exposes only fully validated mappings and keeps the last valid version on errors.
type Store struct {
	path      string
	validate  func(*Mapping) error
	current   atomic.Pointer[Mapping]
	signature atomic.Value
	mu        sync.Mutex
}

func NewStore(path string, validate func(*Mapping) error) (*Store, error) {
	store := &Store{path: path, validate: validate}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping %s: %w", path, err)
	}
	candidate, err := store.prepare(data)
	if err != nil {
		return nil, fmt.Errorf("load mapping %s: %w", path, err)
	}
	store.current.Store(candidate)
	store.signature.Store(sha256.Sum256(data))
	return store, nil
}

func (s *Store) Current() *Mapping {
	return s.current.Load()
}

func (s *Store) ReloadFromDisk() (bool, *Mapping, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return false, s.Current(), err
	}
	return s.install(data, false)
}

// Install validates and atomically persists a remotely delivered bundle.
func (s *Store) Install(data []byte) (bool, *Mapping, error) {
	return s.install(data, true)
}

func (s *Store) install(data []byte, persist bool) (bool, *Mapping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	signature := sha256.Sum256(data)
	if current, ok := s.signature.Load().([32]byte); ok && current == signature {
		return false, s.Current(), nil
	}
	candidate, err := s.prepare(data)
	if err != nil {
		return false, s.Current(), err
	}
	if persist {
		if err := writeAtomic(s.path, data); err != nil {
			return false, s.Current(), err
		}
	}
	s.current.Store(candidate)
	s.signature.Store(signature)
	return true, candidate, nil
}

func (s *Store) prepare(data []byte) (*Mapping, error) {
	candidate, err := Decode(data)
	if err != nil {
		return nil, err
	}
	if s.validate != nil {
		if err := s.validate(candidate); err != nil {
			return nil, err
		}
	}
	return candidate, nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".runtime-bundle-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
