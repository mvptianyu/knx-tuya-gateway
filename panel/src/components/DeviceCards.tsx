import React from 'react';
import { Text, View } from '@ray-js/ray';
import { CategoryIcon } from './CategoryIcon';
import { DpValue, PanelDevice } from '@/types';
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
}

const modeLabels: Record<string, string> = {
  auto: '自动',
  cool: '制冷',
  heat: '制热',
  fan: '送风',
  dry: '除湿',
  manual: '手动',
};

const fanLabels: Record<string, string> = {
  auto: '自动风',
  low: '低风',
  middle: '中风',
  high: '高风',
};

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
      onClick={() => setDp(code, !enabled)}
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
      onClick={() => setDp(device.dps.trigger, true)}
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

export function AirConditionerCard({ device, state, setDp }: CardProps) {
  const enabled = readBool(state, device, 'switch');
  const mode = readString(state, device, 'mode', 'auto');
  const fan = readString(state, device, 'fan_speed', 'auto');
  const target = readNumber(state, device, 'temp_set', 240);
  const current = readNumber(state, device, 'temp_current', target);
  const modes = ['auto', 'cool', 'heat', 'fan', 'dry'];
  const fans = ['auto', 'low', 'middle', 'high'];

  return (
    <View className={`${styles.wideCard} ${enabled ? styles.acOn : ''}`}>
      <View className={styles.wideHeader}>
        <View className={styles.identity}>
          <CategoryIcon category="air_conditioner" active={enabled} />
          <View>
            <Text className={styles.cardName}>{device.name}</Text>
            <Text className={styles.cardStatus}>
              {enabled ? `${modeLabels[mode]} · ${fanLabels[fan]}` : '已关闭'}
            </Text>
            <RoomTag device={device} />
          </View>
        </View>
        <View
          className={styles.powerControl}
          onClick={() => setDp(device.dps.switch, !enabled)}
        >
          <PowerButton enabled={enabled} />
        </View>
      </View>
      <View className={styles.temperatureRow}>
        <View>
          <Text className={styles.currentTemp}>{formatTemperature(current)}</Text>
          <Text className={styles.metricLabel}>当前室温</Text>
        </View>
        <View className={styles.targetControl}>
          <View onClick={() => setDp(device.dps.temp_set, Math.max(160, target - 5))}>
            <Text className={styles.stepButton}>−</Text>
          </View>
          <View className={styles.targetValue}>
            <Text>{formatTemperature(target)}</Text>
            <Text className={styles.targetLabel}>设定</Text>
          </View>
          <View onClick={() => setDp(device.dps.temp_set, Math.min(300, target + 5))}>
            <Text className={styles.stepButton}>+</Text>
          </View>
        </View>
      </View>
      <View className={`${styles.controlGroup} ${styles.modeGroup}`}>
        <Text className={styles.controlLabel}>运行模式</Text>
        <View className={styles.optionRow}>
          {modes.map(item => (
            <View
              key={item}
              className={`${styles.option} ${item === mode ? styles.optionActive : ''}`}
              onClick={() => setDp(device.dps.mode, item)}
            >
              <Text>{modeLabels[item]}</Text>
            </View>
          ))}
        </View>
      </View>
      <View className={`${styles.controlGroup} ${styles.fanGroup}`}>
        <Text className={styles.controlLabel}>风速档位</Text>
        <View className={styles.optionRow}>
          {fans.map(item => (
            <View
              key={item}
              className={`${styles.option} ${item === fan ? styles.optionActive : ''}`}
              onClick={() => setDp(device.dps.fan_speed, item)}
            >
              <Text>{fanLabels[item]}</Text>
            </View>
          ))}
        </View>
      </View>
    </View>
  );
}

export function FreshAirCard({ device, state, setDp }: CardProps) {
  const enabled = readBool(state, device, 'switch');
  const mode = readString(state, device, 'mode', 'auto');
  const fan = readString(state, device, 'fan_speed', 'middle');

  return (
    <View className={`${styles.wideCard} ${enabled ? styles.freshOn : ''}`}>
      <View className={styles.wideHeader}>
        <View className={styles.identity}>
          <CategoryIcon category="fresh_air" active={enabled} />
          <View>
            <Text className={styles.cardName}>{device.name}</Text>
            <Text className={styles.cardStatus}>
              {enabled ? `${modeLabels[mode]} · ${fanLabels[fan]}` : '已关闭'}
            </Text>
            <RoomTag device={device} />
          </View>
        </View>
        <View
          className={styles.powerControl}
          onClick={() => setDp(device.dps.switch, !enabled)}
        >
          <PowerButton enabled={enabled} />
        </View>
      </View>
      <View className={styles.airflow}>
        <View className={styles.airLine} />
        <View className={styles.airLineShort} />
        <Text>新鲜空气循环中</Text>
      </View>
      <View className={`${styles.controlGroup} ${styles.modeGroup}`}>
        <Text className={styles.controlLabel}>运行模式</Text>
        <View className={styles.optionRow}>
          {['auto', 'manual'].map(item => (
            <View
              key={item}
              className={`${styles.option} ${item === mode ? styles.optionActive : ''}`}
              onClick={() => setDp(device.dps.mode, item)}
            >
              <Text>{modeLabels[item]}</Text>
            </View>
          ))}
        </View>
      </View>
      <View className={`${styles.controlGroup} ${styles.fanGroup}`}>
        <Text className={styles.controlLabel}>风速档位</Text>
        <View className={styles.optionRow}>
          {['low', 'middle', 'high'].map(item => (
            <View
              key={item}
              className={`${styles.option} ${item === fan ? styles.optionActive : ''}`}
              onClick={() => setDp(device.dps.fan_speed, item)}
            >
              <Text>{fanLabels[item]}</Text>
            </View>
          ))}
        </View>
      </View>
    </View>
  );
}
