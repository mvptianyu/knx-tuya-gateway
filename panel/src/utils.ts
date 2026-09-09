import { DpValue, PanelDevice } from './types';

export const readDp = (
  state: Record<string, DpValue | undefined>,
  device: PanelDevice,
  capability: string
) => state[device.dps[capability]];

export const readBool = (
  state: Record<string, DpValue | undefined>,
  device: PanelDevice,
  capability: string
) => Boolean(readDp(state, device, capability));

export const readNumber = (
  state: Record<string, DpValue | undefined>,
  device: PanelDevice,
  capability: string,
  fallback = 0
) => {
  const value = readDp(state, device, capability);
  return typeof value === 'number' ? value : fallback;
};

export const readString = (
  state: Record<string, DpValue | undefined>,
  device: PanelDevice,
  capability: string,
  fallback = ''
) => {
  const value = readDp(state, device, capability);
  return typeof value === 'string' ? value : fallback;
};

export const formatTemperature = (raw: number) => `${(raw / 10).toFixed(1)}°`;

export const isMockPreview = () => {
  const location = (globalThis as unknown as { location?: { search?: string } }).location;
  return Boolean(location?.search?.includes('mock=1'));
};
