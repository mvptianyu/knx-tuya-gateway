import React from 'react';
import { ScrollView, Text, View, device, request, showToast } from '@ray-js/ray';
import { hooks, useActions, useProps } from '@ray-js/panel-sdk';
import { devices as sdmDevices } from '@/devices';
import { panelDevices } from '@/generated/manifest';
import { defaultSchema } from '@/generated/schema';
import { runtimeBundleURL } from '@/generated/runtime';
import { mockDevices, mockDpState } from '@/mock/devices';
import {
  AirConditionerCard,
  ClimateCard,
  FreshAirCard,
  LightCard,
  SceneCard,
} from '@/components/DeviceCards';
import { CategoryIcon } from '@/components/CategoryIcon';
import { CategoryId, DpActions, DpValue, PanelDevice } from '@/types';
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

type RuntimeBundle = {
  schema_version: number;
  version: string;
  mappings: Array<{
    virtual_device_id?: string;
    name: string;
    category?: PanelDevice['category'];
    room?: string;
    slot?: number;
    capability?: string;
    tuya_dp_code: string;
  }>;
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

function fetchRuntimeDevices(url: string) {
  return new Promise<{ version: string; devices: PanelDevice[] }>((resolve, reject) => {
    const separator = url.includes('?') ? '&' : '?';
    request({
      url: `${url}${separator}_ts=${Date.now()}`,
      method: 'GET',
      timeout: 10000,
      responseType: 'text',
      success: response => {
        try {
          const payload = (response as { data?: unknown }).data;
          const bundle =
            typeof payload === 'string'
              ? (JSON.parse(payload) as RuntimeBundle)
              : (payload as RuntimeBundle);
          resolve({ version: bundle.version, devices: buildRuntimeDevices(bundle) });
        } catch (error) {
          reject(error);
        }
      },
      fail: reject,
    });
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

  const devices = mock ? mockDevices : runtimeDevices || panelDevices;
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
  const homeScene = devices.find(
    device => device.category === 'scene' && device.name.includes('回家')
  );
  const awayScene = devices.find(
    device =>
      device.category === 'scene' &&
      (device.name.includes('离家') || device.name.includes('外出'))
  );
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
      schema?: unknown[];
      dps?: Record<string, unknown>;
    };
    applyDpIds(devInfo?.dps);
    console.info('Panel device diagnostics', {
      deviceId: devInfo?.devId,
      productId: devInfo?.productId,
      productVersion: (devInfo as { productVer?: string })?.productVer,
      cloudSchemaCount: devInfo?.schema?.length || 0,
      localDpCount: defaultSchema.length,
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
          console.info('Panel runtime bundle loaded', {
            version: result.version,
            deviceCount: result.devices.length,
          });
        })
        .catch(error => {
          console.warn('Panel runtime bundle failed; using last valid config', error);
        });
    };
    load();
    const timer = setInterval(load, 60000);
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

  const renderDevice = (device: PanelDevice) => {
    const props = {
      key: device.id,
      device,
      state,
      setDp,
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

  const renderSceneShortcut = (
    label: string,
    scene: PanelDevice | undefined,
    tone: 'home' | 'away'
  ) => (
    <View
      className={`${styles.sceneShortcut} ${styles[tone]} ${
        scene ? '' : styles.sceneShortcutDisabled
      }`}
      onClick={() => {
        if (!scene) {
          showToast({ title: `请先配置${label}场景映射` });
          return;
        }
        setDp(scene.dps.trigger, true);
      }}
    >
      <View className={styles.sceneShortcutIcon}>
        <Text>{tone === 'home' ? '归' : '行'}</Text>
      </View>
      <View className={styles.sceneShortcutText}>
        <Text className={styles.sceneShortcutName}>{label}</Text>
        <Text className={styles.sceneShortcutHint}>
          {scene ? '轻触立即执行' : '待配置场景 DP'}
        </Text>
      </View>
      <Text className={styles.sceneShortcutArrow}>›</Text>
    </View>
  );

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
                    ? `${freshAirMode === 'manual' ? '手动' : '自动'} · ${
                        freshAirFan === 'low'
                          ? '低风'
                          : freshAirFan === 'high'
                            ? '高风'
                            : '中风'
                      }`
                    : '新风状态'}
                </Text>
              </View>
            </View>
            <View className={styles.sceneShortcuts}>
              {renderSceneShortcut('回家模式', homeScene, 'home')}
              {renderSceneShortcut('离家模式', awayScene, 'away')}
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
