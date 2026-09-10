import React from 'react';
import {
  Button,
  Input,
  ScrollView,
  Text,
  View,
  device,
  showToast,
} from '@ray-js/ray';
import { useProps } from '@ray-js/panel-sdk';
import { devices } from '@/devices';
import { defaultSchema } from '@/generated/schema';
import { DpValue } from '@/types';
import { isMockPreview } from '@/utils';
import styles from './index.module.less';

const codes = {
  request: 'knx_debug_request',
  trigger: 'knx_debug_trigger',
  status: 'knx_debug_status',
  result: 'knx_debug_result',
} as const;

const dpCodeById = defaultSchema.reduce<Record<string, string>>((result, item) => {
  result[String(item.id)] = item.code;
  return result;
}, {});

function unwrap(value: unknown): DpValue | undefined {
  const raw =
    value && typeof value === 'object' && 'value' in value
      ? (value as { value: unknown }).value
      : value;
  if (typeof raw === 'boolean' || typeof raw === 'number' || typeof raw === 'string') {
    return raw;
  }
  return undefined;
}

function publish(devId: string, code: string, value: DpValue) {
  return new Promise<void>((resolve, reject) => {
    device.publishThingModelMessage({
      devId,
      type: 0,
      payload: { [code]: value },
      success: resolve,
      fail: reject,
    });
  });
}

function errorMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  if (error && typeof error === 'object') {
    const detail = error as Record<string, unknown>;
    return String(detail.errorMsg || detail.errMsg || detail.message || error);
  }
  return String(error);
}

function parseWriteValue(value: string): boolean | number | string {
  const trimmed = value.trim();
  if (!trimmed) throw new Error('写入操作必须填写值');
  try {
    const parsed = JSON.parse(trimmed);
    if (
      typeof parsed === 'boolean' ||
      typeof parsed === 'number' ||
      typeof parsed === 'string'
    ) {
      return parsed;
    }
  } catch (_error) {
    return trimmed;
  }
  throw new Error('值只支持布尔、数值或字符串');
}

export default function KNXDebug() {
  const props = useProps<typeof defaultSchema>(current => current) as Record<
    string,
    DpValue | undefined
  >;
  const mock = isMockPreview();
  const [ga, setGA] = React.useState('1/2/11');
  const [dpt, setDPT] = React.useState('DPT-1.001');
  const [value, setValue] = React.useState('true');
  const [state, setState] = React.useState<Record<string, DpValue>>({});
  const [sending, setSending] = React.useState(false);
  const triggerValue = React.useRef(false);

  React.useEffect(() => {
    if (mock) return undefined;
    const apply = (dps: Record<string, unknown> | undefined) => {
      if (!dps) return;
      const updates: Record<string, DpValue> = {};
      Object.keys(dps).forEach(id => {
        const code = dpCodeById[id];
        const next = unwrap(dps[id]);
        if (code && next !== undefined) updates[code] = next;
      });
      if (Object.keys(updates).length) {
        setState(previous => ({ ...previous, ...updates }));
      }
    };
    apply(devices.gateway.getDevInfo()?.dps as Record<string, unknown> | undefined);
    const listener = devices.gateway.onDpDataChange(event => {
      apply(event.dps as unknown as Record<string, unknown>);
    });
    return () => devices.gateway.offDpDataChange(listener);
  }, [mock]);

  const live = { ...props, ...state };
  const status = String(live[codes.status] || 'idle');
  const resultText = String(live[codes.result] || '尚未执行');

  const execute = async (action: 'read' | 'write') => {
    const targetGA = ga.trim();
    if (!targetGA) {
      showToast({ title: '请填写 KNX 组地址' });
      return;
    }
    try {
      setSending(true);
      const request: Record<string, unknown> = {
        id: String(Date.now()),
        action,
        ga: targetGA,
      };
      if (dpt.trim()) request.dpt = dpt.trim();
      if (action === 'write') request.value = parseWriteValue(value);

      if (mock) {
        setState({
          [codes.status]: 'success',
          [codes.result]: JSON.stringify({
            id: request.id,
            ok: true,
            action,
            ga: targetGA,
            command: action === 'read' ? 'response' : 'write',
            dpt: request.dpt,
            value: action === 'read' ? true : request.value,
          }),
        });
        return;
      }

      const devId = devices.gateway.getDevInfo()?.devId;
      if (!devId) throw new Error('未获取到网关 Device ID');
      await publish(devId, codes.request, JSON.stringify(request));
      triggerValue.current = !triggerValue.current;
      await publish(devId, codes.trigger, triggerValue.current);
      showToast({ title: action === 'read' ? '读取请求已发送' : '写入请求已发送' });
    } catch (error) {
      console.warn('KNX debug request failed', error);
      showToast({ title: `操作失败：${errorMessage(error)}` });
    } finally {
      setSending(false);
    }
  };

  let formattedResult = resultText;
  try {
    formattedResult = JSON.stringify(JSON.parse(resultText), null, 2);
  } catch (_error) {
    // Non-JSON gateway errors remain readable as plain text.
  }

  return (
    <View className={styles.page}>
      <ScrollView scrollY className={styles.scroll}>
        <View className={styles.content}>
          <View className={styles.notice}>
            <Text className={styles.noticeTitle}>通过云端安全中转</Text>
            <Text className={styles.noticeText}>
              请求由 Tuya DP 发到树莓派，再由树莓派访问局域网 KNX 总线。
            </Text>
          </View>

          <View className={styles.card}>
            <Text className={styles.cardTitle}>探测参数</Text>
            <Text className={styles.label}>组地址</Text>
            <Input
              className={styles.input}
              value={ga}
              placeholder="例如 1/2/11"
              onInput={event => setGA(event.detail.value)}
            />
            <Text className={styles.label}>DPT</Text>
            <Input
              className={styles.input}
              value={dpt}
              placeholder="例如 DPT-1.001；纯读取可留空"
              onInput={event => setDPT(event.detail.value)}
            />
            <Text className={styles.label}>写入值</Text>
            <Input
              className={styles.input}
              value={value}
              placeholder='true、23.5、"auto" 或 auto'
              onInput={event => setValue(event.detail.value)}
            />
            <View className={styles.actions}>
              <Button
                className={`${styles.button} ${styles.readButton}`}
                disabled={sending || status === 'running'}
                onClick={() => void execute('read')}
              >
                读取组地址
              </Button>
              <Button
                className={`${styles.button} ${styles.writeButton}`}
                disabled={sending || status === 'running'}
                onClick={() => void execute('write')}
              >
                写入组地址
              </Button>
            </View>
          </View>

          <View className={styles.card}>
            <View className={styles.resultHeading}>
              <Text className={styles.cardTitle}>总线结果</Text>
              <Text className={`${styles.badge} ${styles[status] || ''}`}>
                {status}
              </Text>
            </View>
            <Text className={styles.result}>{formattedResult}</Text>
            <Text className={styles.tip}>
              KNX 报文不携带权威 DPT。读取无响应通常表示地址不可读、状态地址不同或执行器未配置反馈。
            </Text>
          </View>
        </View>
      </ScrollView>
    </View>
  );
}
