import { GlobalConfig } from '@ray-js/types';

export const tuya = {
  window: {
    backgroundColor: '#f3f0e9',
    navigationBarTitleText: '树莓派定制网关',
    navigationBarBackgroundColor: '#f3f0e9',
    navigationBarTextStyle: 'black',
  },
  functionalPages: {
    rayPlayCoolFunctional: {
      // tyg0szxsm3vog8nf6n 为 ‘酷玩吧’ 功能页id
      appid: 'tyg0szxsm3vog8nf6n',
    },
    settings: {
      appid: 'tycryc71qaug8at6yt',
      entryCode: 'entrye0n05idydmmfv',
    },
  },
};

const globalConfig: GlobalConfig = {
  basename: '',
};

export default globalConfig;
