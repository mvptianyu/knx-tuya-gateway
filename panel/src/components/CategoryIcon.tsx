import React from 'react';
import { View } from '@ray-js/ray';
import { CategoryId } from '@/types';
import styles from './CategoryIcon.module.less';

interface Props {
  category: Exclude<CategoryId, 'overview'>;
  active?: boolean;
  compact?: boolean;
}

function DeviceGlyph({ category }: Pick<Props, 'category'>) {
  switch (category) {
    case 'light':
      return (
        <View className={styles.lightGlyph}>
          <View className={styles.bulb} />
          <View className={styles.bulbBase} />
        </View>
      );
    case 'air_conditioner':
      return (
        <View className={styles.snowflake}>
          <View className={styles.snowLineOne} />
          <View className={styles.snowLineTwo} />
          <View className={styles.snowLineThree} />
        </View>
      );
    case 'scene':
      return (
        <View className={styles.homeGlyph}>
          <View className={styles.homeRoof} />
          <View className={styles.homeBody}>
            <View className={styles.homeDoor} />
          </View>
        </View>
      );
    case 'fresh_air':
      return (
        <View className={styles.windGlyph}>
          <View className={styles.windLineLong} />
          <View className={styles.windLineShort} />
          <View className={styles.windLineMedium} />
        </View>
      );
    case 'climate_sensor':
      return (
        <View className={styles.thermometer}>
          <View className={styles.thermometerStem} />
          <View className={styles.thermometerBulb} />
        </View>
      );
    default:
      return null;
  }
}

export function CategoryIcon({ category, active = false, compact = false }: Props) {
  return (
    <View
      className={`${styles.icon} ${styles[category]} ${active ? styles.active : ''} ${
        compact ? styles.compact : ''
      }`}
    >
      <DeviceGlyph category={category} />
    </View>
  );
}
