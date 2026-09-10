import React from 'react';
import {
  ScrollView,
  Text,
  View,
  device,
  home,
  openCreateTapToRunScene,
  openDevManualAndSmart,
  router,
  showToast,
} from '@ray-js/ray';
import { hooks, useActions, useProps } from '@ray-js/panel-sdk';
import { devices as sdmDevices } from '@/devices';
import { panelDevices } from '@/generated/manifest';
import { defaultSchema } from '@/generated/schema';
import {
  runtimeBundlePollSeconds,
  runtimeBundleURL,
  panelVersion,
} from '@/generated/runtime';
import { mockDevices, mockDpState } from '@/mock/devices';
import {
  AirConditionerCard,
  ClimateCard,
  formatEnumLabel,
  FreshAirCard,
  LightCard,
  SceneCard,
} from '@/components/DeviceCards';
import { CategoryIcon } from '@/components/CategoryIcon';
import { CategoryId, DpActions, DpSchema, DpValue, PanelDevice } from '@/types';
import {
  formatTemperature,
  isMockPreview,
  readBool,
  readNumber,
  readString,
} from '@/utils';
import styles from './index.module.less';

const categoryMeta: Array<{
  id: CategoryId;
  label: string;
}> = [
  { id: 'overview', label: '总览' },
  { id: 'scene', label: '场景' },
  { id: 'light', label: '灯光' },
  { id: 'air_conditioner', label: '空调' },
  { id: 'fresh_air', label: '新风' },
  { id: 'climate_sensor', label: '温控' },
];

const localDpByCode = defaultSchema.reduce<Record<string, (typeof defaultSchema)[number]>>(
  (result, item) => {
    result[item.code] = item;
    return result;
  },
  {}
);

const panelSchemaByCode = localDpByCode as unknown as Record<
  string,
  DpSchema | undefined
>;

function indexDpSchemas(source: unknown) {
  if (!Array.isArray(source)) return {};
  return source.reduce<Record<string, DpSchema | undefined>>((result, item) => {
    if (!item || typeof item !== 'object') return result;
    const candidate = item as Record<string, unknown>;
    if (typeof candidate.code !== 'string') return result;
    let property = candidate.property;
    if (typeof property === 'string') {
      try {
        property = JSON.parse(property);
      } catch (_error) {
        return result;
      }
    }
    if (!property || typeof property !== 'object') return result;
    result[candidate.code] = {
      ...(candidate as unknown as DpSchema),
      property: property as DpSchema['property'],
    };
    return result;
  }, {});
}

const localDpCodeById = defaultSchema.reduce<Record<string, string>>(
  (result, item) => {
    result[String(item.id)] = item.code;
    return result;
  },
  {}
);

type TuyaApiError = {
  errorMsg?: string;
  errorCode?: string | number;
  innerError?: {
    errorMsg?: string;
    errorCode?: string | number;
  };
};

type ThingModelInfo = {
  modelId: string;
  productId: string;
  productVersion: string;
  services: unknown[];
  extensions: Record<string, unknown>;
};

type SceneDiagnostics = {
  deviceId: string;
  productId: string;
  productVersion: string;
  cloudSchemaCount: number;
  isShare: boolean;
  homeId: string;
  homeName: string;
  homeAdmin: boolean;
  homeRole: number;
  error?: string;
};

type RuntimeMapping = {
  virtual_device_id?: string;
  name: string;
  category?: PanelDevice['category'];
  room?: string;
  slot?: number;
  capability?: string;
  tuya_dp_code: string;
  tuya_dp_type?: string;
  tuya_to_knx?: Record<string, unknown>;
  knx_to_tuya?: Record<string, unknown>;
  min?: number;
  max?: number;
  step?: number;
  scale?: number;
  unit?: string;
};

type RuntimeBundle = {
  schema_version: number;
  version: string;
  mappings: RuntimeMapping[];
};

type RuntimeResponse = {
  data?: unknown;
  statusCode?: number;
  header?: Record<string, unknown>;
};

const thingModelReady = new Map<string, Promise<ThingModelInfo>>();

function getErrorMessage(error: unknown) {
  if (error instanceof Error) return error.message;
  if (error && typeof error === 'object') {
    const details = error as Record<string, unknown> & TuyaApiError;
    const message =
      details.errorMsg || details.errMsg || details.message || details.errorMessage;
    const code = details.errorCode || details.errCode || details.code;
    const innerMessage = details.innerError?.errorMsg;
    const innerCode = details.innerError?.errorCode;
    if (message || innerMessage) {
      const outer = code ? `${String(code)} ${String(message || '')}`.trim() : message;
      const inner = innerCode
        ? `${String(innerCode)} ${String(innerMessage || '')}`.trim()
        : innerMessage;
      return [outer, inner].filter(Boolean).join(' / ');
    }
    if (message) return code ? `${String(code)} ${String(message)}` : String(message);
  }
  return String(error);
}

function unwrapDpValue(value: unknown): DpValue | undefined {
  const raw =
    value && typeof value === 'object' && 'value' in value
      ? (value as { value: unknown }).value
      : value;
  return typeof raw === 'boolean' ||
    typeof raw === 'number' ||
    typeof raw === 'string'
    ? raw
    : undefined;
}

function checkThingModelSupport(devId: string) {
  return new Promise<boolean>((resolve, reject) => {
    device.deviceIsSupportThingModel({
      devId,
      success: result => resolve(result.isSupport),
      fail: reject,
    });
  });
}

function getThingModelInfo(devId: string) {
  return new Promise<ThingModelInfo>((resolve, reject) => {
    device.getDeviceThingModelInfo({
      devId,
      success: result => resolve(result as ThingModelInfo),
      fail: reject,
    });
  });
}

function ensureThingModelReady(devId: string) {
  const existing = thingModelReady.get(devId);
  if (existing) return existing;

  const ready = checkThingModelSupport(devId)
    .then(isSupport => {
      if (!isSupport) {
        throw new Error('当前网关设备实例未开启 TuyaLink 物模型控制能力');
      }
      return getThingModelInfo(devId);
    })
    .then(info => {
      console.info('TuyaLink thing model ready', {
        devId,
        modelId: info.modelId,
        productId: info.productId,
        productVersion: info.productVersion,
        serviceCount: info.services?.length || 0,
      });
      return info;
    })
    .catch(error => {
      thingModelReady.delete(devId);
      throw error;
    });

  thingModelReady.set(devId, ready);
  return ready;
}

function publishThingModelProperty(
  devId: string,
  code: string,
  value: DpValue
) {
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

function decodeUtf8(bytes: Uint8Array) {
  let result = '';
  for (let index = 0; index < bytes.length; ) {
    const first = bytes[index++];
    if (first < 0x80) {
      result += String.fromCharCode(first);
      continue;
    }

    const second = bytes[index++] & 0x3f;
    if (first < 0xe0) {
      result += String.fromCharCode(((first & 0x1f) << 6) | second);
      continue;
    }

    const third = bytes[index++] & 0x3f;
    if (first < 0xf0) {
      result += String.fromCharCode(
        ((first & 0x0f) << 12) | (second << 6) | third
      );
      continue;
    }

    const fourth = bytes[index++] & 0x3f;
    const codePoint =
      ((first & 0x07) << 18) | (second << 12) | (third << 6) | fourth;
    const offset = codePoint - 0x10000;
    result += String.fromCharCode(
      0xd800 + (offset >> 10),
      0xdc00 + (offset & 0x3ff)
    );
  }
  return result;
}

function decodeRuntimeBinary(payload: unknown): string | undefined {
  if (typeof ArrayBuffer === 'undefined') return undefined;
  if (payload instanceof ArrayBuffer) {
    return decodeUtf8(new Uint8Array(payload));
  }
  if (ArrayBuffer.isView(payload)) {
    return decodeUtf8(
      new Uint8Array(payload.buffer, payload.byteOffset, payload.byteLength)
    );
  }
  if (payload && typeof payload === 'object') {
    const entries = Object.entries(payload as Record<string, unknown>);
    if (
      entries.length > 0 &&
      entries.every(
        ([key, value]) => /^\d+$/.test(key) && typeof value === 'number'
      )
    ) {
      const bytes = entries
        .sort(([left], [right]) => Number(left) - Number(right))
        .map(([, value]) => value as number);
      return decodeUtf8(new Uint8Array(bytes));
    }
  }
  return undefined;
}

function decodeBase64Utf8(value: string) {
  const alphabet =
    'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
  const normalized = value.replace(/\s+/g, '').replace(/=+$/, '');
  const bytes: number[] = [];
  let buffer = 0;
  let bits = 0;

  for (const character of normalized) {
    const index = alphabet.indexOf(character);
    if (index < 0) {
      throw new Error('Gitee 配置内容不是有效的 Base64');
    }
    buffer = (buffer << 6) | index;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((buffer >> bits) & 0xff);
    }
  }
  return decodeUtf8(new Uint8Array(bytes));
}

function parseRuntimeBundle(payload: unknown, depth = 0): RuntimeBundle {
  if (depth > 4) {
    throw new Error('运行时配置包装层级过深');
  }

  const binaryText = decodeRuntimeBinary(payload);
  if (binaryText !== undefined) {
    return parseRuntimeBundle(binaryText, depth + 1);
  }

  if (typeof payload === 'string') {
    const text = payload.replace(/^\uFEFF/, '').trim();
    if (!text) throw new Error('运行时配置响应为空');
    return parseRuntimeBundle(JSON.parse(text), depth + 1);
  }

  if (!payload || typeof payload !== 'object') {
    throw new Error(`运行时配置响应类型不支持：${typeof payload}`);
  }

  const candidate = payload as Record<string, unknown>;
  if (
    candidate.encoding === 'base64' &&
    typeof candidate.content === 'string'
  ) {
    return parseRuntimeBundle(
      decodeBase64Utf8(candidate.content),
      depth + 1
    );
  }
  if ('schema_version' in candidate || 'mappings' in candidate) {
    const schemaVersion = Number(candidate.schema_version);
    const version =
      typeof candidate.version === 'string' ? candidate.version.trim() : '';
    if (
      schemaVersion !== 1 ||
      !version ||
      !Array.isArray(candidate.mappings)
    ) {
      throw new Error(
        `运行时配置格式不受支持：schema_version=${String(
          candidate.schema_version
        )}, version=${String(candidate.version)}, mappings=${Array.isArray(
          candidate.mappings
        )}`
      );
    }
    return {
      ...(candidate as RuntimeBundle),
      schema_version: schemaVersion,
      version,
    };
  }

  const wrapperKeys = ['data', 'body', 'payload', 'result'];
  for (const key of wrapperKeys) {
    if (key in candidate && candidate[key] !== payload) {
      try {
        return parseRuntimeBundle(candidate[key], depth + 1);
      } catch (_error) {
        // Some Tuya request implementations add unrelated wrapper fields.
      }
    }
  }
  throw new Error(
    `运行时配置对象缺少 schema_version/mappings，字段=${Object.keys(candidate)
      .slice(0, 12)
      .join(',')}`
  );
}

function describeRuntimeResponse(response: RuntimeResponse) {
  const payload = response.data;
  const binaryText = decodeRuntimeBinary(payload);
  let preview = '';
  if (typeof payload === 'string') {
    preview = payload.slice(0, 240);
  } else if (binaryText !== undefined) {
    preview = binaryText.slice(0, 240);
  } else if (payload && typeof payload === 'object') {
    try {
      preview = JSON.stringify(payload).slice(0, 240);
    } catch (_error) {
      preview = '[unserializable object]';
    }
  } else {
    preview = String(payload);
  }
  return {
    statusCode: response.statusCode,
    payloadType:
      binaryText !== undefined
        ? 'binary'
        : Array.isArray(payload)
          ? 'array'
          : typeof payload,
    headers: response.header,
    preview,
  };
}

function buildRuntimeDevices(bundle: RuntimeBundle): PanelDevice[] {
  if (bundle.schema_version !== 1 || !bundle.version || !Array.isArray(bundle.mappings)) {
    throw new Error('运行时配置格式不受支持');
  }
  const devices = new Map<string, PanelDevice>();
  bundle.mappings.forEach(item => {
    if (
      !item.virtual_device_id ||
      !item.category ||
      !item.slot ||
      !item.capability ||
      !localDpByCode[item.tuya_dp_code]
    ) {
      return;
    }
    const key = `${item.category}:${item.slot}`;
    const room = item.room || '全屋';
    const existing = devices.get(key);
    if (existing) {
      if (
        existing.id !== item.virtual_device_id ||
        existing.name !== item.name ||
        existing.room !== room
      ) {
        throw new Error(`设备槽位配置冲突：${key}`);
      }
      existing.dps[item.capability] = item.tuya_dp_code;
      return;
    }
    devices.set(key, {
      id: item.virtual_device_id,
      name: item.name,
      category: item.category,
      room,
      slot: item.slot,
      dps: { [item.capability]: item.tuya_dp_code },
    });
  });
  return Array.from(devices.values()).sort(
    (left, right) =>
      left.category.localeCompare(right.category) || left.slot - right.slot
  );
}

function buildRuntimeSchemas(
  bundle: RuntimeBundle
): Record<string, DpSchema | undefined> {
  const schemas: Record<string, DpSchema | undefined> = {};
  bundle.mappings.forEach(item => {
    const base = panelSchemaByCode[item.tuya_dp_code];
    if (!base) return;

    const property: DpSchema['property'] = { ...base.property };
    let overridden = false;
    if (
      item.tuya_dp_type === 'enum' &&
      item.tuya_to_knx &&
      Object.keys(item.tuya_to_knx).length > 0
    ) {
      property.type = 'enum';
      property.range = Object.keys(item.tuya_to_knx);
      overridden = true;
    }

    if (typeof item.min === 'number') {
      property.min = item.min;
      overridden = true;
    }
    if (typeof item.max === 'number') {
      property.max = item.max;
      overridden = true;
    }
    if (typeof item.step === 'number') {
      property.step = item.step;
      overridden = true;
    }
    if (typeof item.scale === 'number') {
      property.scale = item.scale;
      overridden = true;
    }
    if (typeof item.unit === 'string') {
      property.unit = item.unit;
      overridden = true;
    }

    if (overridden) {
      schemas[item.tuya_dp_code] = {
        ...base,
        property,
      };
    }
  });
  return schemas;
}

function fetchRuntimeDevices(url: string) {
  return new Promise<{
    version: string;
    devices: PanelDevice[];
    schemas: Record<string, DpSchema | undefined>;
  }>((resolve, reject) => {
    const separator = url.includes('?') ? '&' : '?';
    const requestURL = `${url}${separator}_ts=${Date.now()}`;
    const rejectRequest = (error: unknown) => {
      console.warn('Panel runtime request failed', {
        url: requestURL,
        error: getErrorMessage(error),
      });
      reject(error);
    };

    console.info('Panel runtime request started', {
      url: requestURL,
      method: 'GET',
    });

    try {
      const task = ty.request({
        url: requestURL,
        data: '',
        header: {
          Accept: 'application/json, text/plain, */*',
        },
        method: 'GET',
        dataType: 'text',
        responseType: 'text',
        timeout: 10000,
        success: response => {
          try {
            const runtimeResponse = response as RuntimeResponse;
            let bundle: RuntimeBundle;
            try {
              bundle = parseRuntimeBundle(runtimeResponse.data);
            } catch (_dataError) {
              bundle = parseRuntimeBundle(runtimeResponse);
            }
            resolve({
              version: bundle.version,
              devices: buildRuntimeDevices(bundle),
              schemas: buildRuntimeSchemas(bundle),
            });
          } catch (error) {
            console.warn(
              'Panel runtime response parse failed',
              describeRuntimeResponse(response as RuntimeResponse),
              error
            );
            reject(error);
          }
        },
        fail: rejectRequest,
      });

      // Some Smart Life/base-library combinations expose callback APIs as thenables.
      // Observe their rejection as well so Ark errors never escape as unhandled promises.
      const thenableTask = task as unknown as {
        catch?: (handler: (error: unknown) => void) => unknown;
      };
      if (typeof thenableTask?.catch === 'function') {
        thenableTask.catch(rejectRequest);
      }
    } catch (error) {
      rejectRequest(error);
    }
  });
}

function LiveStatus() {
  const online = hooks.useDeviceOnline();
  return (
    <View className={`${styles.status} ${online ? styles.online : ''}`}>
      <View className={styles.statusDot} />
      <Text>{online ? '在线' : '离线'}</Text>
    </View>
  );
}

function DeviceStatus({ mock }: { mock: boolean }) {
  if (!mock) return <LiveStatus />;
  return (
    <View className={`${styles.status} ${styles.online}`}>
      <View className={styles.statusDot} />
      <Text>预览</Text>
    </View>
  );
}

function VerticalScroller({
  mock,
  children,
}: {
  mock: boolean;
  children: React.ReactNode;
}) {
  if (mock) {
    return <View className={`${styles.scroll} ${styles.mockScroll}`}>{children}</View>;
  }
  return (
    <ScrollView scrollY className={styles.scroll}>
      {children}
    </ScrollView>
  );
}

function CategoryScroller({
  mock,
  children,
}: {
  mock: boolean;
  children: React.ReactNode;
}) {
  if (mock) {
    return <View className={`${styles.tabs} ${styles.mockTabs}`}>{children}</View>;
  }
  return (
    <ScrollView scrollX className={styles.tabs}>
      {children}
    </ScrollView>
  );
}

function RoomScroller({
  mock,
  children,
}: {
  mock: boolean;
  children: React.ReactNode;
}) {
  if (mock) {
    return <View className={`${styles.roomScroller} ${styles.mockTabs}`}>{children}</View>;
  }
  return (
    <ScrollView scrollX className={styles.roomScroller}>
      {children}
    </ScrollView>
  );
}

export function Home() {
  const liveState = useProps<typeof defaultSchema>(props => props) as Record<
    string,
    DpValue | undefined
  >;
  const liveActions = useActions<typeof defaultSchema>() as unknown as DpActions;
  const mock = isMockPreview();
  const [activeCategory, setActiveCategory] = React.useState<CategoryId>('overview');
  const [activeRoom, setActiveRoom] = React.useState('all');
  const [previewState, setPreviewState] = React.useState<Record<string, DpValue>>(
    mockDpState
  );
  const [tuyaLinkState, setTuyaLinkState] = React.useState<Record<string, DpValue>>(
    {}
  );
  const [runtimeDevices, setRuntimeDevices] = React.useState<PanelDevice[] | null>(
    null
  );
  const [cloudSchemaByCode, setCloudSchemaByCode] = React.useState<
    Record<string, DpSchema | undefined>
  >({});
  const [runtimeSchemaByCode, setRuntimeSchemaByCode] = React.useState<
    Record<string, DpSchema | undefined>
  >({});
  const [runtimeVersion, setRuntimeVersion] = React.useState('内置配置');
  const [sceneDiagnostics, setSceneDiagnostics] =
    React.useState<SceneDiagnostics | null>(null);

  const devices = mock ? mockDevices : runtimeDevices || panelDevices;
  const schemaByCode = mock
    ? panelSchemaByCode
    : {
        ...panelSchemaByCode,
        ...cloudSchemaByCode,
        ...runtimeSchemaByCode,
      };
  // TuyaLink updates must win over the traditional schema snapshot, which can be stale.
  const state = mock ? previewState : { ...liveState, ...tuyaLinkState };
  const visibleCategories = categoryMeta.filter(
    item =>
      item.id === 'overview' ||
      devices.some(device => device.category === item.id)
  );
  const categoryDevices =
    activeCategory === 'overview'
      ? devices
      : devices.filter(device => device.category === activeCategory);
  const roomOptions = Array.from(
    new Set(categoryDevices.map(device => device.room || '全屋'))
  );
  const visibleDevices =
    activeRoom === 'all'
      ? categoryDevices
      : categoryDevices.filter(device => (device.room || '全屋') === activeRoom);
  const climate = devices.find(device => device.category === 'climate_sensor');
  const freshAir = devices.find(device => device.category === 'fresh_air');
  const temperature = climate
    ? readNumber(state, climate, 'temperature', 0)
    : undefined;
  const humidity = climate
    ? readNumber(state, climate, 'humidity', 0)
    : undefined;
  const freshAirEnabled = freshAir
    ? readBool(state, freshAir, 'switch')
    : undefined;
  const freshAirMode = freshAir
    ? readString(state, freshAir, 'mode', 'auto')
    : undefined;
  const freshAirFan = freshAir
    ? readString(state, freshAir, 'fan_speed', 'middle')
    : undefined;

  React.useEffect(() => {
    if (mock) return undefined;

    const applyDpIds = (dps: Record<string, unknown> | undefined) => {
      if (!dps) return;
      const updates: Record<string, DpValue> = {};
      Object.keys(dps).forEach(dpId => {
        const code = localDpCodeById[String(dpId)];
        const value = unwrapDpValue(dps[dpId]);
        if (code && value !== undefined) updates[code] = value;
      });
      if (Object.keys(updates).length > 0) {
        setTuyaLinkState(previous => ({ ...previous, ...updates }));
      }
    };

    const devInfo = sdmDevices.gateway.getDevInfo() as unknown as {
      devId?: string;
      productId?: string;
      productVer?: string;
      isShare?: boolean;
      schema?: unknown[];
      dps?: Record<string, unknown>;
    };
    applyDpIds(devInfo?.dps);
    setCloudSchemaByCode(indexDpSchemas(devInfo?.schema));
    console.info('Panel device diagnostics', {
      deviceId: devInfo?.devId,
      productId: devInfo?.productId,
      productVersion: (devInfo as { productVer?: string })?.productVer,
      cloudSchemaCount: devInfo?.schema?.length || 0,
      localDpCount: defaultSchema.length,
      isShare: devInfo?.isShare,
    });
    home.getCurrentHomeInfo({
      success: home => {
        const diagnostics: SceneDiagnostics = {
          deviceId: devInfo?.devId || '',
          productId: devInfo?.productId || '',
          productVersion: devInfo?.productVer || '',
          cloudSchemaCount: devInfo?.schema?.length || 0,
          isShare: Boolean(devInfo?.isShare),
          homeId: home.homeId,
          homeName: home.homeName,
          homeAdmin: home.admin,
          homeRole: home.role,
        };
        setSceneDiagnostics(diagnostics);
        console.info('Panel scene eligibility diagnostics', diagnostics);
      },
      fail: error => {
        const diagnostics: SceneDiagnostics = {
          deviceId: devInfo?.devId || '',
          productId: devInfo?.productId || '',
          productVersion: devInfo?.productVer || '',
          cloudSchemaCount: devInfo?.schema?.length || 0,
          isShare: Boolean(devInfo?.isShare),
          homeId: '',
          homeName: '',
          homeAdmin: false,
          homeRole: -1,
          error: getErrorMessage(error),
        };
        setSceneDiagnostics(diagnostics);
        console.warn('Panel home diagnostics failed', error);
      },
    });
    if (devInfo?.devId) {
      ensureThingModelReady(devInfo.devId).catch(error => {
        console.warn('TuyaLink thing model initialization failed', error);
      });
    }

    const listenerId = sdmDevices.gateway.onDpDataChange(data => {
      applyDpIds(data.dps as unknown as Record<string, unknown>);
      console.info('Panel DP update', data.dps);
    });
    return () => sdmDevices.gateway.offDpDataChange(listenerId);
  }, [mock]);

  React.useEffect(() => {
    if (mock || !runtimeBundleURL) return;
    let active = true;
    const load = () => {
      fetchRuntimeDevices(runtimeBundleURL)
        .then(result => {
          if (!active) return;
          setRuntimeDevices(result.devices);
          setRuntimeSchemaByCode(result.schemas);
          setRuntimeVersion(result.version);
          console.info('Panel runtime bundle loaded', {
            version: result.version,
            deviceCount: result.devices.length,
            runtimeSchemaCount: Object.keys(result.schemas).length,
          });
        })
        .catch(error => {
          console.warn('Panel runtime bundle failed; using last valid config', error);
        });
    };
    load();
    const timer = setInterval(load, runtimeBundlePollSeconds * 1000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [mock]);

  const setDp = async (code: string | undefined, value: DpValue) => {
    if (!code) return;
    if (mock) {
      setPreviewState(previous => ({ ...previous, [code]: value }));
      return;
    }
    const action = liveActions[code];
    try {
      let accepted: unknown;
      let channel = 'tuyalink-thing-model';
      if (!localDpByCode[code]) {
        showToast({ title: `未找到功能点 ${code}` });
        return;
      }
      const devInfo = sdmDevices.gateway.getDevInfo();
      if (!devInfo?.devId) {
        showToast({ title: '未获取到网关 Device ID' });
        return;
      }

      try {
        const model = await ensureThingModelReady(devInfo.devId);
        const serializedModel = JSON.stringify(model.services || []);
        if (!serializedModel.includes(`"${code}"`)) {
          console.warn('TuyaLink DP code not found in cached thing model', {
            code,
            modelId: model.modelId,
            productId: model.productId,
          });
        }
        await publishThingModelProperty(devInfo.devId, code, value);
        accepted = true;
      } catch (thingModelError) {
        const unsupported =
          getErrorMessage(thingModelError).includes('未开启 TuyaLink 物模型控制能力');
        if (!unsupported || !action || typeof action.set !== 'function') {
          throw thingModelError;
        }
        channel = 'schema-action';
        accepted = await action.set(value);
      }
      if (accepted === false) {
        showToast({ title: '设备未接受控制指令' });
        return;
      }
      setTuyaLinkState(previous => ({ ...previous, [code]: value }));
      console.info('DP command sent', { code, value, channel });
      showToast({ title: '指令已发送' });
    } catch (error) {
      console.warn('DP command failed', code, value, error);
      showToast({ title: `下发失败：${getErrorMessage(error)}` });
    }
  };

  const openSceneCreator = async () => {
    if (mock) {
      showToast({ title: '真机中将打开 Smart Life 场景编辑器' });
      return;
    }
    try {
      await openCreateTapToRunScene();
    } catch (error) {
      console.warn('Open Smart Life scene creator failed', error);
      showToast({ title: `打开场景创建失败：${getErrorMessage(error)}` });
    }
  };

  const openSceneManager = async () => {
    if (mock) {
      showToast({ title: '真机中将打开网关联动管理' });
      return;
    }
    const devId = sdmDevices.gateway.getDevInfo()?.devId;
    if (!devId) {
      showToast({ title: '未获取到网关 Device ID' });
      return;
    }
    try {
      await openDevManualAndSmart({ devId });
    } catch (error) {
      console.warn('Open Smart Life scene manager failed', error);
      showToast({ title: `打开联动管理失败：${getErrorMessage(error)}` });
    }
  };

  const renderDevice = (device: PanelDevice) => {
    const props = {
      key: device.id,
      device,
      state,
      setDp,
      schemaByCode,
    };
    switch (device.category) {
      case 'light':
        return <LightCard {...props} />;
      case 'air_conditioner':
        return <AirConditionerCard {...props} />;
      case 'scene':
        return <SceneCard {...props} />;
      case 'fresh_air':
        return <FreshAirCard {...props} />;
      case 'climate_sensor':
        return <ClimateCard {...props} />;
      default:
        return null;
    }
  };

  const renderDeviceSection = (
    category: Exclude<CategoryId, 'overview'>,
    sectionDevices: PanelDevice[]
  ) => {
    const meta = categoryMeta.find(item => item.id === category);
    return (
      <View key={category} className={styles.categorySection}>
        <View className={styles.categoryHeading}>
          <View className={styles.categoryIdentity}>
            <CategoryIcon category={category} compact />
            <View>
              <Text className={styles.categoryTitle}>{meta?.label}</Text>
              <Text className={styles.categorySubtitle}>
                {category === 'scene' ? '场景与自动化入口' : '按房间快速找到设备'}
              </Text>
            </View>
          </View>
          <View className={styles.categoryRule} />
          <Text className={styles.categoryCount}>{sectionDevices.length}</Text>
        </View>
        <View className={styles.deviceGrid}>{sectionDevices.map(renderDevice)}</View>
      </View>
    );
  };

  return (
    <View className={styles.page}>
      <VerticalScroller mock={mock}>
        <View className={styles.content}>
          <View className={styles.hero}>
            <View className={styles.heroTop}>
              <View>
                <Text className={styles.title}>树莓派定制网关</Text>
                <Text className={styles.eyebrow}>全屋设备与场景</Text>
              </View>
              <DeviceStatus mock={mock} />
            </View>
            <View className={styles.summaryRow}>
              <View className={styles.environmentItem}>
                <Text className={styles.environmentValue}>
                  {temperature === undefined ? '--' : formatTemperature(temperature)}
                </Text>
                <Text className={styles.summaryLabel}>室内温度</Text>
              </View>
              <View className={styles.environmentItem}>
                <Text className={styles.environmentValue}>
                  {humidity === undefined ? '--' : `${(humidity / 10).toFixed(0)}%`}
                </Text>
                <Text className={styles.summaryLabel}>室内湿度</Text>
              </View>
              <View className={styles.environmentItem}>
                <Text
                  className={`${styles.environmentValue} ${styles.freshAirValue} ${
                    freshAirEnabled ? styles.freshAirOn : ''
                  }`}
                >
                  {freshAirEnabled === undefined
                    ? '未接入'
                    : freshAirEnabled
                      ? '运行中'
                      : '已关闭'}
                </Text>
                <Text className={styles.summaryLabel}>
                  {freshAirEnabled
                    ? [freshAirMode, freshAirFan]
                        .filter(Boolean)
                        .map((value, index) =>
                          formatEnumLabel(
                            value as string,
                            index === 0 ? 'mode' : 'fan_speed'
                          )
                        )
                        .join(' · ') || '运行中'
                    : '新风状态'}
                </Text>
              </View>
            </View>
            <View className={styles.panelMeta}>
              <View className={styles.versionBlock}>
                <Text className={styles.versionLabel}>Panel v{panelVersion}</Text>
                <Text className={styles.versionHint}>配置 {runtimeVersion}</Text>
              </View>
              <View className={styles.debugEntry} onClick={() => router.push('/debug')}>
                <View className={styles.debugEntryIcon}>
                  <Text>KNX</Text>
                </View>
                <View className={styles.debugEntryText}>
                  <Text className={styles.debugEntryTitle}>总线调试</Text>
                  <Text className={styles.debugEntryHint}>读写组地址</Text>
                </View>
                <Text className={styles.debugEntryArrow}>›</Text>
              </View>
            </View>
            <View className={styles.heroGlow} />
            <View className={styles.heroOrbit} />
          </View>

          <CategoryScroller mock={mock}>
            <View className={styles.tabRow}>
              {visibleCategories.map(category => {
                const active = category.id === activeCategory;
                return (
                  <View
                    key={category.id}
                    className={`${styles.tab} ${active ? styles.tabActive : ''}`}
                    onClick={() => {
                      setActiveCategory(category.id);
                      setActiveRoom('all');
                    }}
                  >
                    <Text>{category.label}</Text>
                  </View>
                );
              })}
            </View>
          </CategoryScroller>

        {categoryDevices.length > 0 && (
          <RoomScroller mock={mock}>
            <View className={styles.roomRow}>
              <View
                className={`${styles.roomFilter} ${
                  activeRoom === 'all' ? styles.roomFilterActive : ''
                }`}
                onClick={() => setActiveRoom('all')}
              >
                <Text>全部区域</Text>
              </View>
              {roomOptions.map(room => (
                <View
                  key={room}
                  className={`${styles.roomFilter} ${
                    activeRoom === room ? styles.roomFilterActive : ''
                  }`}
                  onClick={() => setActiveRoom(room)}
                >
                  <View className={styles.roomFilterDot} />
                  <Text>{room}</Text>
                </View>
              ))}
            </View>
          </RoomScroller>
        )}

        {activeCategory === 'scene' && (
          <>
            <View className={styles.sceneTools}>
              <View
                className={`${styles.sceneTool} ${styles.sceneToolPrimary}`}
                onClick={() => {
                  void openSceneCreator();
                }}
              >
                <View className={styles.sceneToolIcon}>
                  <Text>+</Text>
                </View>
                <View className={styles.sceneToolText}>
                  <Text className={styles.sceneToolTitle}>新建组合场景</Text>
                  <Text className={styles.sceneToolHint}>从树莓派网关选择已开放的 DP</Text>
                </View>
              </View>
              <View
                className={styles.sceneTool}
                onClick={() => {
                  void openSceneManager();
                }}
              >
                <View className={`${styles.sceneToolIcon} ${styles.sceneToolIconManage}`}>
                  <Text>联</Text>
                </View>
                <View className={styles.sceneToolText}>
                  <Text className={styles.sceneToolTitle}>联动管理</Text>
                  <Text className={styles.sceneToolHint}>仅显示已经引用网关的规则</Text>
                </View>
              </View>
            </View>
            {!mock && sceneDiagnostics && (
              <View className={styles.sceneDiagnostics}>
                <View className={styles.sceneDiagnosticsHeader}>
                  <Text className={styles.sceneDiagnosticsTitle}>场景资格诊断</Text>
                  <Text
                    className={`${styles.sceneDiagnosticsBadge} ${
                      sceneDiagnostics.isShare || !sceneDiagnostics.homeAdmin
                        ? styles.sceneDiagnosticsWarn
                        : styles.sceneDiagnosticsOk
                    }`}
                  >
                    {sceneDiagnostics.isShare
                      ? '共享设备'
                      : sceneDiagnostics.homeAdmin
                        ? '家庭管理员'
                        : '家庭成员'}
                  </Text>
                </View>
                <Text className={styles.sceneDiagnosticsLine}>
                  PID {sceneDiagnostics.productId || '未知'} · 产品版本{' '}
                  {sceneDiagnostics.productVersion || '未知'}
                </Text>
                <Text className={styles.sceneDiagnosticsLine}>
                  云端 DP {sceneDiagnostics.cloudSchemaCount} / 本地 DP {defaultSchema.length} · 家庭{' '}
                  {sceneDiagnostics.homeName || sceneDiagnostics.homeId || '读取失败'}
                </Text>
                <Text className={styles.sceneDiagnosticsHint}>
                  {sceneDiagnostics.isShare
                    ? '当前是共享设备，请用设备原始绑定账号在所属家庭中创建自动化。'
                    : !sceneDiagnostics.homeAdmin
                      ? '当前账号不是家庭管理员，请切换管理员账号验证自动化候选设备。'
                      : sceneDiagnostics.cloudSchemaCount < defaultSchema.length
                        ? '设备实例仍是旧 DP 模型，请刷新产品版本；必要时删除后重新绑定网关。'
                        : '账号归属和 DP 模型正常；若添加任务时仍看不到网关，请让涂鸦核查该产品实例的场景资格、品类限制和场景索引。'}
                </Text>
                {sceneDiagnostics.error && (
                  <Text className={styles.sceneDiagnosticsError}>
                    家庭信息读取失败：{sceneDiagnostics.error}
                  </Text>
                )}
              </View>
            )}
          </>
        )}

        {visibleDevices.length > 0 ? (
          activeCategory === 'overview' ? (
            <View className={styles.overviewSections}>
              {categoryMeta
                .filter(
                  (
                    item
                  ): item is {
                    id: Exclude<CategoryId, 'overview'>;
                    label: string;
                  } => item.id !== 'overview'
                )
                .map(item => {
                  const sectionDevices = visibleDevices.filter(
                    device => device.category === item.id
                  );
                  return sectionDevices.length > 0
                    ? renderDeviceSection(item.id, sectionDevices)
                    : null;
                })}
            </View>
          ) : (
            <>
              <View className={styles.sectionHeading}>
                <Text className={styles.sectionTitle}>
                  {categoryMeta.find(item => item.id === activeCategory)?.label}
                </Text>
                <Text className={styles.sectionHint}>
                  {activeCategory === 'scene'
                    ? `${visibleDevices.length} 个场景`
                    : `${visibleDevices.length} 个设备`}
                </Text>
              </View>
              <View className={styles.deviceGrid}>{visibleDevices.map(renderDevice)}</View>
            </>
          )
        ) : (
          <View className={styles.empty}>
            <Text className={styles.emptyTitle}>这一栏还很安静</Text>
            <Text className={styles.emptyText}>
              在运行时配置中增加映射后即可显示。
            </Text>
          </View>
        )}

        {mock && (
          <View className={styles.previewNote}>
            <Text>当前是 UI 样例数据，不会向真实网关下发 DP。</Text>
          </View>
        )}
        <View className={styles.bottomSpace} />
        </View>
      </VerticalScroller>
    </View>
  );
}

export default Home;
