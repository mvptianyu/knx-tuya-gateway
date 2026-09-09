package tuyamqtt

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"knx-tuya-gw/internal/config"
	"knx-tuya-gw/internal/logger"
)

// ErrInitialConnectPending means Paho is still retrying the initial connection
// in the background. Runtime services may continue; explicit health checks should fail.
var ErrInitialConnectPending = errors.New("Tuya MQTT initial connection still pending")

type Credentials struct {
	ProductID    string
	DeviceID     string
	DeviceSecret string
	Broker       string
}

type CommandHandler func(nodeID, dpCode string, value interface{}) error

type Client struct {
	cfg   config.TuyaMQTTConfig
	creds Credentials
	mqtt  mqtt.Client
	onCmd CommandHandler

	disconnects atomic.Uint64
	pendingMu   sync.Mutex
	pendingSeq  uint64
	pending     map[string]pendingProperty
}

type pendingProperty struct {
	sequence uint64
	value    interface{}
}

type envelope struct {
	MsgID   string      `json:"msgId"`
	Time    int64       `json:"time"`
	Version string      `json:"version,omitempty"`
	Code    int         `json:"code,omitempty"`
	Message string      `json:"msg,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Sys     interface{} `json:"sys,omitempty"`
}

func New(cfg config.TuyaMQTTConfig) (*Client, error) {
	creds, err := loadCredentials(cfg.CredentialsFile)
	if err != nil {
		return nil, err
	}
	if cfg.ExpectedProductID != "" && creds.ProductID != cfg.ExpectedProductID {
		return nil, fmt.Errorf(
			"Tuya product mismatch: expected PID %s, credentials belong to PID %s; activate credentials for the expected product or correct expected_product_id",
			cfg.ExpectedProductID,
			creds.ProductID,
		)
	}
	if cfg.GatewayNodeID == "" {
		cfg.GatewayNodeID = "gateway"
	}
	if creds.Broker != "" {
		cfg.Broker = creds.Broker
	}
	return &Client{
		cfg:     cfg,
		creds:   creds,
		pending: make(map[string]pendingProperty),
	}, nil
}

func (c *Client) OnCommand(handler CommandHandler) {
	c.onCmd = handler
}

func (c *Client) Start(ctx context.Context) error {
	return c.start(ctx, true)
}

// StartOnce performs one connection attempt so diagnostics can expose the
// broker's TLS or CONNACK error instead of hiding it behind automatic retries.
func (c *Client) StartOnce(ctx context.Context) error {
	return c.start(ctx, false)
}

func (c *Client) start(ctx context.Context, retry bool) error {
	tlsConfig, err := c.tlsConfig()
	if err != nil {
		return err
	}
	opts := mqtt.NewClientOptions().
		AddBroker(c.cfg.Broker).
		SetClientID("tuyalink_" + c.creds.DeviceID).
		SetCredentialsProvider(func() (string, string) {
			_, username, password := MQTTCredentials(c.creds.DeviceID, c.creds.DeviceSecret, time.Now())
			return username, password
		}).
		SetTLSConfig(tlsConfig).
		SetKeepAlive(time.Duration(c.cfg.KeepAliveSeconds) * time.Second).
		SetConnectTimeout(time.Duration(c.cfg.ConnectTimeout) * time.Second).
		SetAutoReconnect(true).
		SetConnectRetry(retry).
		SetConnectRetryInterval(5 * time.Second).
		SetCleanSession(false).
		SetOrderMatters(false)
	opts.SetOnConnectHandler(func(_ mqtt.Client) {
		logger.Infof(
			"Tuya MQTT connected: broker=%s product=%s device=%s",
			c.cfg.Broker,
			c.creds.ProductID,
			c.creds.DeviceID,
		)
		go c.restoreSession()
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.disconnects.Add(1)
		logger.Warnf("Tuya MQTT disconnected: %v", err)
	})
	c.mqtt = mqtt.NewClient(opts)

	token := c.mqtt.Connect()
	if !token.WaitTimeout(time.Duration(c.cfg.ConnectTimeout+5) * time.Second) {
		if !retry {
			return fmt.Errorf("Tuya MQTT one-shot connect timeout: %s", c.cfg.Broker)
		}
		return fmt.Errorf("%w: %s", ErrInitialConnectPending, c.cfg.Broker)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("Tuya MQTT connect %s: %w", c.cfg.Broker, err)
	}

	go func() {
		<-ctx.Done()
		c.Close()
	}()
	return nil
}

func (c *Client) Close() {
	if c.mqtt != nil && c.mqtt.IsConnectionOpen() {
		c.mqtt.Disconnect(500)
	}
}

func (c *Client) IsConnected() bool {
	return c.connected()
}

func (c *Client) DisconnectCount() uint64 {
	return c.disconnects.Load()
}

func (c *Client) Report(nodeID, dpCode string, value interface{}) error {
	if nodeID != c.cfg.GatewayNodeID {
		return fmt.Errorf(
			"Tuya gateway DP reporting requires node_id %q, got %q",
			c.cfg.GatewayNodeID,
			nodeID,
		)
	}
	if !c.connected() {
		c.queueProperty(dpCode, value)
		return nil
	}
	if err := c.publishProperty(dpCode, value); err != nil {
		c.queueProperty(dpCode, value)
		logger.Warnf("Tuya property %s publish failed and latest value was queued: %v", dpCode, err)
	}
	return nil
}

func MQTTCredentials(deviceID, deviceSecret string, now time.Time) (clientID, username, password string) {
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	clientID = "tuyalink_" + deviceID
	username = deviceID + "|signMethod=hmacSha256,timestamp=" + timestamp + ",secureMode=1,accessType=1"
	content := "deviceId=" + deviceID + ",timestamp=" + timestamp + ",secureMode=1,accessType=1"
	mac := hmac.New(sha256.New, []byte(deviceSecret))
	_, _ = mac.Write([]byte(content))
	password = hex.EncodeToString(mac.Sum(nil))
	return
}

func (c *Client) restoreSession() {
	base := fmt.Sprintf("tylink/%s/thing", c.creds.DeviceID)
	topics := map[string]byte{
		base + "/property/set":             1,
		base + "/property/report_response": 1,
		base + "/model/get_response":       1,
	}
	if err := waitToken(c.mqtt.SubscribeMultiple(topics, c.handleMessage), 10*time.Second); err != nil {
		logger.Errorf("Tuya MQTT subscriptions failed: %v", err)
		return
	}
	logger.Infof("Tuya gateway DP mode ready: node=%s", c.cfg.GatewayNodeID)
	logger.Infof("Tuya MQTT subscriptions restored")
	if err := c.requestThingModel(); err != nil {
		logger.Warnf("Tuya cloud model request failed: %v", err)
	}
	c.flushPendingProperties()
}

func (c *Client) queueProperty(dpCode string, value interface{}) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.pendingSeq++
	c.pending[dpCode] = pendingProperty{sequence: c.pendingSeq, value: value}
}

func (c *Client) flushPendingProperties() {
	c.pendingMu.Lock()
	snapshot := make(map[string]pendingProperty, len(c.pending))
	for dpCode, property := range c.pending {
		snapshot[dpCode] = property
	}
	c.pendingMu.Unlock()

	flushed := 0
	for dpCode, property := range snapshot {
		if err := c.publishProperty(dpCode, property.value); err != nil {
			logger.Warnf(
				"Tuya pending property flush paused: flushed=%d remaining=%d error=%v",
				flushed,
				c.pendingPropertyCount(),
				err,
			)
			return
		}
		c.pendingMu.Lock()
		if current, ok := c.pending[dpCode]; ok && current.sequence == property.sequence {
			delete(c.pending, dpCode)
			flushed++
		}
		c.pendingMu.Unlock()
	}
	if flushed > 0 {
		logger.Infof(
			"Tuya pending property reports flushed: count=%d remaining=%d",
			flushed,
			c.pendingPropertyCount(),
		)
	}
}

func (c *Client) pendingPropertyCount() int {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	return len(c.pending)
}

func (c *Client) handleMessage(_ mqtt.Client, message mqtt.Message) {
	topic := message.Topic()
	var msg envelope
	if err := json.Unmarshal(message.Payload(), &msg); err != nil {
		logger.Warnf("Tuya MQTT invalid JSON on %s: %v", topic, err)
		return
	}
	switch {
	case strings.HasSuffix(topic, "/thing/property/set"):
		c.handlePropertySet(msg)
	case strings.HasSuffix(topic, "/thing/property/report_response"):
		if msg.Code != 0 {
			logger.Errorf("Tuya property report rejected: code=%d msg=%s", msg.Code, msg.Message)
		} else {
			logger.Debugf("Tuya property report acknowledged: msg_id=%s", msg.MsgID)
		}
	case strings.HasSuffix(topic, "/thing/model/get_response"):
		if msg.Code != 0 {
			logger.Errorf("Tuya cloud model rejected: code=%d msg=%s", msg.Code, msg.Message)
		} else {
			logger.Infof(
				"Tuya cloud model acknowledged: product=%s device=%s",
				c.creds.ProductID,
				c.creds.DeviceID,
			)
		}
	default:
		logger.Debugf("Tuya MQTT message ignored: topic=%s", topic)
	}
}

func (c *Client) requestThingModel() error {
	topic := fmt.Sprintf("tylink/%s/thing/model/get", c.creds.DeviceID)
	return c.publish(topic, envelope{
		MsgID:   newMessageID(),
		Time:    nowMillis(),
		Version: "1.0",
		Data:    map[string]interface{}{},
	})
}

func (c *Client) handlePropertySet(msg envelope) {
	properties, ok := msg.Data.(map[string]interface{})
	if !ok {
		logger.Warnf("Tuya gateway command has invalid property payload")
		c.respondPropertySet(msg, fmt.Errorf("invalid property payload"))
		return
	}
	var commandErr error
	for code, value := range properties {
		if c.onCmd == nil {
			continue
		}
		if err := c.onCmd(c.cfg.GatewayNodeID, code, value); err != nil {
			logger.Errorf("Tuya command %s.%s failed: %v", c.cfg.GatewayNodeID, code, err)
			commandErr = err
			break
		}
	}
	c.respondPropertySet(msg, commandErr)
}

func (c *Client) respondPropertySet(request envelope, commandErr error) {
	if !ackRequested(request.Sys) {
		return
	}
	response := envelope{MsgID: request.MsgID, Time: nowMillis(), Code: 0}
	if commandErr != nil {
		response.Code = 1
		response.Message = commandErr.Error()
	}
	topic := fmt.Sprintf("tylink/%s/thing/property/set_response", c.creds.DeviceID)
	if err := c.publish(topic, response); err != nil {
		logger.Warnf("Tuya property set response failed: error=%v", err)
	}
}

func ackRequested(sys interface{}) bool {
	values, ok := sys.(map[string]interface{})
	if !ok {
		return false
	}
	switch value := values["ack"].(type) {
	case float64:
		return value == 1
	case int:
		return value == 1
	case json.Number:
		return value.String() == "1"
	default:
		return false
	}
}

func (c *Client) publishProperty(dpCode string, value interface{}) error {
	now := nowMillis()
	topic := fmt.Sprintf("tylink/%s/thing/property/report", c.creds.DeviceID)
	return c.publish(topic, envelope{
		MsgID: newMessageID(),
		Time:  now,
		Data: map[string]interface{}{
			dpCode: map[string]interface{}{"value": value, "time": now},
		},
		Sys: map[string]interface{}{"ack": 1},
	})
}

func (c *Client) publish(topic string, payload envelope) error {
	if !c.connected() {
		return fmt.Errorf("Tuya MQTT is not connected")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return waitToken(c.mqtt.Publish(topic, 1, false, raw), 10*time.Second)
}

func (c *Client) connected() bool {
	return c.mqtt != nil && c.mqtt.IsConnectionOpen()
}

func (c *Client) tlsConfig() (*tls.Config, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if c.cfg.CAFile != "" {
		pem, err := os.ReadFile(c.cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read Tuya MQTT CA file: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("Tuya MQTT CA file contains no certificate")
		}
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
		RootCAs:    pool,
	}, nil
}

func loadCredentials(path string) (Credentials, error) {
	file, err := os.Open(path)
	if err != nil {
		return Credentials{}, fmt.Errorf("open Tuya MQTT credentials %s: %w", path, err)
	}
	defer file.Close()
	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	if err := scanner.Err(); err != nil {
		return Credentials{}, err
	}
	creds := Credentials{
		ProductID:    values["TUYA_PRODUCT_ID"],
		DeviceID:     values["TUYA_DEVICE_ID"],
		DeviceSecret: values["TUYA_DEVICE_SECRET"],
		Broker:       values["TUYA_MQTT_BROKER"],
	}
	if creds.ProductID == "" || creds.DeviceID == "" || creds.DeviceSecret == "" ||
		strings.HasPrefix(creds.ProductID, "replace_") ||
		strings.HasPrefix(creds.DeviceID, "replace_") ||
		strings.HasPrefix(creds.DeviceSecret, "replace_") {
		return Credentials{}, fmt.Errorf("fill TUYA_PRODUCT_ID, TUYA_DEVICE_ID and TUYA_DEVICE_SECRET in %s", path)
	}
	return creds, nil
}

// ValidateCredentials checks that runtime MQTT credentials are present and usable.
func ValidateCredentials(path string) error {
	_, err := loadCredentials(path)
	return err
}

func waitToken(token mqtt.Token, timeout time.Duration) error {
	if !token.WaitTimeout(timeout) {
		return fmt.Errorf("MQTT operation timed out")
	}
	return token.Error()
}

func newMessageID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(id[:])
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}
