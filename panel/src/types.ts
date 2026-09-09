export type CategoryId =
  | 'overview'
  | 'light'
  | 'air_conditioner'
  | 'scene'
  | 'fresh_air'
  | 'climate_sensor';

export type DpValue = boolean | number | string;

export interface PanelDevice {
  id: string;
  name: string;
  category: Exclude<CategoryId, 'overview'>;
  room?: string;
  slot: number;
  dps: Record<string, string>;
}

export interface DpAction {
  set: (value: DpValue) => Promise<boolean>;
  toggle?: () => Promise<boolean>;
}

export type DpActions = Record<string, DpAction | undefined>;
