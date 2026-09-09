import React from 'react';
import { initPanelEnvironment } from '@ray-js/ray';
import { SdmProvider } from '@ray-js/panel-sdk';
import { devices } from '@/devices';
import { isMockPreview } from '@/utils';
import './app.less';

const mockPreview = isMockPreview();

if (!mockPreview) {
  initPanelEnvironment({ useDefaultOffline: true });
}

interface Props {
  children: React.ReactNode;
}

class App extends React.Component<Props> {
  onLaunch() {
    if (!mockPreview) {
      devices.gateway.init();
    }
  }

  render() {
    return (
      <SdmProvider
        value={devices.gateway}
        isNeedMainDeviceInitialized={!mockPreview}
      >
        {this.props.children}
      </SdmProvider>
    );
  }
}

// Ray's web history does not always emit its initial route in standalone preview.
// Keep the official router for Tuya, and mount the same page directly only for mock mode.
if (mockPreview && typeof document !== 'undefined') {
  setTimeout(async () => {
    const root = document.getElementById('root');
    if (!root) return;

    const [{ default: ReactDOM }, { default: Home }] = await Promise.all([
      import('react-dom'),
      import('@/pages/home'),
    ]);
    ReactDOM.render(
      <SdmProvider value={devices.gateway} isNeedMainDeviceInitialized={false}>
        <Home />
      </SdmProvider>,
      root
    );
  }, 0);
}

export default App;
