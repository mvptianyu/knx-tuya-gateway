import { SmartDeviceModel } from '@ray-js/panel-sdk';
import { defaultSchema } from '@/generated/schema';

export const devices = {
  gateway: new SmartDeviceModel<typeof defaultSchema>(),
};
