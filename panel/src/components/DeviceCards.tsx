import React from 'react';
import { Text, View } from '@ray-js/ray';
import { CategoryIcon } from './CategoryIcon';
import { DpSchema, DpValue, PanelDevice } from '@/types';
import {
  formatTemperature,
  readBool,
  readNumber,
  readString,
} from '@/utils';
import styles from './DeviceCards.module.less';

interface CardProps {
  device: PanelDevice;
  state: Record<string, DpValue | undefined>;
  setDp: (code: string | undefined, value: DpValue) => void;
  schemaByCode: Record<string, DpSchema | undefined>;
}

const enumLabels: Record<string, string> = {
  auto: '自动',
  cool: '制冷',
  heat: '制热',
  fan: '送风',
  dry: '除湿',
  manual: '手动',
  low: '低',
  middle: '中',
  high: '高',
};

export function formatEnumLabel(value: string, capability?: string) {
  const label = enumLabels[value] || value;
  return capability === 'fan_speed' && value !== 'auto' ? `${label}风` : label;
}

function getCapabilitySchema(
  device: PanelDevice,
  capability: string,
  schemaByCode: CardProps['schemaByCode']
) {
  const code = device.dps[capability];
  return code ? schemaByCode[code] : undefined;
}

function getEnumOptions(
  device: PanelDevice,
  capability: string,
  schemaByCode: CardProps['schemaByCode']
) {
  const schema = getCapabilitySchema(device, capability, schemaByCode);
  return schema?.property.type === 'enum'
    ? Array.from(schema.property.range || [])
    : [];
}

function formatSchemaValue(value: number, schema?: DpSchema) {
  const scale = schema?.property.scale || 0;
  const unit = schema?.property.unit || '';
  const scaled = value / 10 ** scale;
  return `${scaled.toFixed(scale)}${unit}`;
}

function RoomTag({ device }: Pick<CardProps, 'device'>) {
  return (
    <View className={styles.roomTag}>
      <Text>{device.room || '全屋'}</Text>
    </View>
  );
}

function PowerButton({ enabled }: { enabled: boolean }) {
  return (
    <View className={`${styles.powerIcon} ${enabled ? styles.powerIconOn : ''}`}>
      <View className={styles.powerStem} />
      <View className={styles.powerRing} />
    </View>
  );
}

export function LightCard({ device, state, setDp }: CardProps) {
  const enabled = readBool(state, device, 'switch');
  const code = device.dps.switch;

  return (
    <View
      className={`${styles.smallCard} ${enabled ? styles.lightOn : ''}`}
      onClick={() => {
        void setDp(code, !enabled);
      }}
    >
      <View className={styles.cardTop}>
        <CategoryIcon category="light" active={enabled} />
        <PowerButton enabled={enabled} />
      </View>
      <View className={styles.cardBottom}>
        <Text className={styles.cardName}>{device.name}</Text>
        <View className={styles.deviceMeta}>
          <RoomTag device={device} />
          <Text className={styles.metaDivider}>|</Text>
          <Text className={styles.cardStatus}>{enabled ? '已开启' : '已关闭'}</Text>
        </View>
      </View>
    </View>
  );
}

export function SceneCard({ device, setDp }: CardProps) {
  return (
    <View
      className={`${styles.smallCard} ${styles.sceneCard}`}
      onClick={() => {
        void setDp(device.dps.trigger, true);
      }}
    >
      <View className={styles.cardTop}>
        <CategoryIcon category="scene" />
        <View className={styles.sceneRun}>
          <View className={styles.scenePlay} />
        </View>
      </View>
      <View className={styles.cardBottom}>
        <Text className={styles.cardName}>{device.name}</Text>
        <View className={styles.deviceMeta}>
          <RoomTag device={device} />
          <Text className={styles.metaDivider}>|</Text>
          <Text className={styles.cardStatus}>点击执行</Text>
        </View>
      </View>
    </View>
  );
}

export function ClimateCard({ device, state }: CardProps) {
  const temperature = readNumber(state, device, 'temperature');
  const humidity = readNumber(state, device, 'humidity');

  return (
    <View className={`${styles.wideCard} ${styles.climateCard}`}>
      <View className={styles.wideHeader}>
        <View className={styles.identity}>
          <CategoryIcon category="climate_sensor" />
          <View>
            <Text className={styles.cardName}>{device.name}</Text>
            <Text className={styles.cardStatus}>实时环境</Text>
            <RoomTag device={device} />
          </View>
        </View>
        <Text className={styles.liveMark}>LIVE</Text>
      </View>
      <View className={styles.metricRow}>
        <View className={styles.metric}>
          <Text className={styles.metricValue}>{formatTemperature(temperature)}</Text>
          <Text className={styles.metricLabel}>室内温度</Text>
        </View>
        <View className={styles.divider} />
        <View className={styles.metric}>
          <Text className={styles.metricValue}>{(humidity / 10).toFixed(0)}%</Text>
          <Text className={styles.metricLabel}>相对湿度</Text>
        </View>
      </View>
    </View>
  );
}

export function AirConditionerCard({
  device,
  state,
  setDp,
  schemaByCode,
}: CardProps) {
  const enabled = readBool(state, device, 'switch');
  const modes = getEnumOptions(device, 'mode', schemaByCode);
  const fans = getEnumOptions(device, 'fan_speed', schemaByCode);
  const mode = readString(state, device, 'mode', modes[0] || '');
  const fan = readString(state, device, 'fan_speed', fans[0] || '');
  const targetSchema = getCapabilitySchema(device, 'temp_set', schemaByCode);
  const targetMinimum = targetSchema?.property.min ?? 160;
  const targetMaximum = targetSchema?.property.max ?? 300;
  const targetStep = targetSchema?.property.step ?? 1;
  const target = readNumber(state, device, 'temp_set', targetMinimum);
  const current = readNumber(state, device, 'temp_current', target);
  const currentSchema = getCapabilitySchema(device, 'temp_current', schemaByCode);
  const statusDetails = [
    mode ? formatEnumLabel(mode, 'mode') : '',
    fan ? formatEnumLabel(fan, 'fan_speed') : '',
  ].filter(Boolean);

  return (
    <View className={`${styles.wideCard} ${enabled ? styles.acOn : ''}`}>
      <View className={styles.wideHeader}>
        <View className={styles.identity}>
          <CategoryIcon category="air_conditioner" active={enabled} />
          <View>
            <Text className={styles.cardName}>{device.name}</Text>
            <Text className={styles.cardStatus}>
              {enabled ? statusDetails.join(' · ') || '运行中' : '已关闭'}
            </Text>
            <RoomTag device={device} />
          </View>
        </View>
        <View
          className={styles.powerControl}
          onClick={() => {
            void setDp(device.dps.switch, !enabled);
          }}
        >
          <PowerButton enabled={enabled} />
        </View>
      </View>
      <View className={styles.temperatureRow}>
        <View>
          <Text className={styles.currentTemp}>
            {formatSchemaValue(current, currentSchema || targetSchema)}
          </Text>
          <Text className={styles.metricLabel}>当前室温</Text>
        </View>
        {device.dps.temp_set && (
          <View className={styles.targetControl}>
            <View
              onClick={() =>
                void setDp(
                  device.dps.temp_set,
                  Math.max(targetMinimum, target - targetStep)
                )
              }
            >
              <Text className={styles.stepButton}>−</Text>
            </View>
            <View className={styles.targetValue}>
              <Text>{formatSchemaValue(target, targetSchema)}</Text>
              <Text className={styles.targetLabel}>设定</Text>
            </View>
            <View
              onClick={() =>
                void setDp(
                  device.dps.temp_set,
                  Math.min(targetMaximum, target + targetStep)
                )
              }
            >
              <Text className={styles.stepButton}>+</Text>
            </View>
          </View>
        )}
      </View>
      {modes.length > 0 && (
        <View className={`${styles.controlGroup} ${styles.modeGroup}`}>
          <Text className={styles.controlLabel}>运行模式</Text>
          <View className={styles.optionRow}>
            {modes.map(item => (
              <View
                key={item}
                className={`${styles.option} ${item === mode ? styles.optionActive : ''}`}
                onClick={() => {
                  void setDp(device.dps.mode, item);
                }}
              >
                <Text>{formatEnumLabel(item, 'mode')}</Text>
              </View>
            ))}
          </View>
        </View>
      )}
      {fans.length > 0 && (
        <View className={`${styles.controlGroup} ${styles.fanGroup}`}>
          <Text className={styles.controlLabel}>风速档位</Text>
          <View className={styles.optionRow}>
            {fans.map(item => (
              <View
                key={item}
                className={`${styles.option} ${item === fan ? styles.optionActive : ''}`}
                onClick={() => {
                  void setDp(device.dps.fan_speed, item);
                }}
              >
                <Text>{formatEnumLabel(item, 'fan_speed')}</Text>
              </View>
            ))}
          </View>
        </View>
      )}
    </View>
  );
}

export function FreshAirCard({
  device,
  state,
  setDp,
  schemaByCode,
}: CardProps) {
  const enabled = readBool(state, device, 'switch');
  const modes = getEnumOptions(device, 'mode', schemaByCode);
  const fans = getEnumOptions(device, 'fan_speed', schemaByCode);
  const mode = readString(state, device, 'mode', modes[0] || '');
  const fan = readString(state, device, 'fan_speed', fans[0] || '');
  const statusDetails = [
    mode ? formatEnumLabel(mode, 'mode') : '',
    fan ? formatEnumLabel(fan, 'fan_speed') : '',
  ].filter(Boolean);

  return (
    <View className={`${styles.wideCard} ${enabled ? styles.freshOn : ''}`}>
      <View className={styles.wideHeader}>
        <View className={styles.identity}>
          <CategoryIcon category="fresh_air" active={enabled} />
          <View>
            <Text className={styles.cardName}>{device.name}</Text>
            <Text className={styles.cardStatus}>
              {enabled ? statusDetails.join(' · ') || '运行中' : '已关闭'}
            </Text>
            <RoomTag device={device} />
          </View>
        </View>
        <View
          className={styles.powerControl}
          onClick={() => {
            void setDp(device.dps.switch, !enabled);
          }}
        >
          <PowerButton enabled={enabled} />
        </View>
      </View>
      <View className={styles.airflow}>
        <View className={styles.airLine} />
        <View className={styles.airLineShort} />
        <Text>新鲜空气循环中</Text>
      </View>
      {modes.length > 0 && (
        <View className={`${styles.controlGroup} ${styles.modeGroup}`}>
          <Text className={styles.controlLabel}>运行模式</Text>
          <View className={styles.optionRow}>
            {modes.map(item => (
              <View
                key={item}
                className={`${styles.option} ${item === mode ? styles.optionActive : ''}`}
                onClick={() => {
                  void setDp(device.dps.mode, item);
                }}
              >
                <Text>{formatEnumLabel(item, 'mode')}</Text>
              </View>
            ))}
          </View>
        </View>
      )}
      {fans.length > 0 && (
        <View className={`${styles.controlGroup} ${styles.fanGroup}`}>
          <Text className={styles.controlLabel}>风速档位</Text>
          <View className={styles.optionRow}>
            {fans.map(item => (
              <View
                key={item}
                className={`${styles.option} ${item === fan ? styles.optionActive : ''}`}
                onClick={() => {
                  void setDp(device.dps.fan_speed, item);
                }}
              >
                <Text>{formatEnumLabel(item, 'fan_speed')}</Text>
              </View>
            ))}
          </View>
        </View>
      )}
    </View>
  );
}
