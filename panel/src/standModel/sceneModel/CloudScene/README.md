## 云端在线情景库

### 用法
#### 1. 使用 IDE 插件将情景数据注入到项目
- `src/standModel/sceneModel/CloudScene`

#### 2. 代码引用
```tsx
import { useDevice } from '@ray-js/panel-sdk';
import { jumpToCoolBar } from '@/standModel/sceneModel/CloudScene';

type Props = {};

export const Demo: React.FC<Props> = ({ style }) => {
  const { devInfo } = useDevice();

  const handleJumpToCoolBar = () => {
    const { devId, groupId } = devInfo || {};
    jumpToCoolBar(devId, groupId);
  };

  return (
    <View onClick={handleJumpToCoolBar}>
      跳转在线情景库
    </View>
  );
};
```
