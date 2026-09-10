
declare namespace ty.home {
  
  export function getLocalDeviceConfigWithDevId(params: {
    
    devId: string
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function updateHomeInfoData(params: {
    
    homeName: string
    
    homeId: string
    
    longitude: string
    
    latitude: string
    
    address: string
    
    admin: boolean
    
    mode: number
    
    role: number
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getCurrentHomeInfo(params?: {
    success?: (params: {
      
      homeName: string
      
      homeId: string
      
      longitude: string
      
      latitude: string
      
      address: string
      
      admin: boolean
      
      mode: number
      
      role: number
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getDeviceRoomInfo(params: {
    
    deviceId: string
    success?: (params: {
      
      roomId: number
      
      name: string
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getGroupRoomInfo(params: {
    
    groupId: string
    success?: (params: {
      
      roomId: number
      
      name: string
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function switchHome(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function switchHomeWithHomeId(params: {
    
    homeId: string
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function createHome(params: {
    
    homeName: string
    
    mode: number
    
    longitude?: string
    
    latitude?: string
    
    address?: string
    
    rooms?: string[]
    success?: (params: {
      
      homeId: string
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function switchHomeDialog(params: {
    
    hiddenHouseManager: boolean
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getHomeListInfo(params?: {
    success?: (params: {
      
      homeList: HomeInfoData[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function registerHomeChangeListener(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function unregisterHomeChangeListener(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function refreshCurrentHome(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function inviteMember(params: {
    
    homeId: number
    
    role?: number
    
    customRoleId?: number
    success?: (params: {
      
      invitationMsgContent: string
      
      invitationCode: string
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function paymentControlEntry(params: {
    
    entryID: string
    success?: (params: {
      
      entryID: string
      
      isDisplayed: boolean
      
      entryExtensionInfo?: any
      
      entryName: string
      
      isIAPForced: boolean
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function registerHomeMemberManagerListener(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function unregisterHomeMemberManagerListener(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function canDisplayDIYHomeCard(params: {
    
    card: DIYHomeCard
    
    gid: number
    success?: (params: {
      
      result: boolean
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function didDisplayDIYHomeCard(params: {
    
    card: DIYHomeCard
    
    gid: number
    success?: (params: {
      
      result: boolean
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function addDIYHomeCard(params: {
    
    card: DIYHomeCardWithStyle
    
    gid: number
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function removeDIYHomeCard(params: {
    
    card: DIYHomeCard
    
    gid: number
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function mediaPlayerControl(params: {
    
    op: string
    
    data: string
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function registerStateChangeListener(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function unregisterStateChangeListener(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function setupFloatWindow(params: {
    
    visible: boolean
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getAIAssistantSwitch(params?: {
    success?: (params: {
      
      open: boolean
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getDeviceIdList(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      devIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getRoomList(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      roomDatas: RoomData[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getGroupIdList(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      groupIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getShareDeviceIdList(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      devIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getShareGroupIdList(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      groupIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function registerHomeListListener(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function unregisterHomeListListener(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getDeviceIdsInRoom(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      devIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getGroupIdsInRoom(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      groupIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function getDeviceIdListWithDevId(params: {
    
    ownerId: number
    
    roomId: number
    
    devId: string
    success?: (params: {
      
      devIds: string[]
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function switchDeviceRoom(params: {
    
    deviceId: string
    
    roomId: number
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function onTicketSuccess(params?: {
    
    map?: any
    success?: (params: {
      
      map?: any
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function logout(params?: {
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function openRecommendSceneDetail(params: {
    
    source: string
    
    sceneModel: any
    success?: (params: {
      
      status?: boolean
      
      type: number
      
      data?: any
    }) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function openDeviceExecutionAndAnutomation(params: {
    
    deviceId: string
    
    title?: string
    success?: (params: null) => void
    fail?: (params: {
      errorMsg: string
      errorCode: string | number
      innerError: {
        errorCode: string | number
        errorMsg: string
      }
    }) => void
    complete?: () => void
  }): void

  
  export function onHomeChangeComplete(
    listener: (params: HomeInfoData) => void
  ): void

  
  export function offHomeChangeComplete(
    listener: (params: HomeInfoData) => void
  ): void

  
  export function onRemoveMemberHandler(
    listener: (params: MemberHandlerResponse) => void
  ): void

  
  export function offRemoveMemberHandler(
    listener: (params: MemberHandlerResponse) => void
  ): void

  
  export function onResetMemberNameHandler(
    listener: (params: MemberHandlerResponse) => void
  ): void

  
  export function offResetMemberNameHandler(
    listener: (params: MemberHandlerResponse) => void
  ): void

  
  export function onResetMemberRoleHandler(
    listener: (params: MemberHandlerResponse) => void
  ): void

  
  export function offResetMemberRoleHandler(
    listener: (params: MemberHandlerResponse) => void
  ): void

  
  export function onPlayerStateChangeHander(
    listener: (params: ThingPlayerStateHandlerResponse) => void
  ): void

  
  export function offPlayerStateChangeHander(
    listener: (params: ThingPlayerStateHandlerResponse) => void
  ): void

  
  export function onHomeDeviceRemoved(
    listener: (params: OnDeviceRemovedBody) => void
  ): void

  
  export function offHomeDeviceRemoved(
    listener: (params: OnDeviceRemovedBody) => void
  ): void

  
  export function onHomeDeviceAdd(
    listener: (params: OnDeviceAddBody) => void
  ): void

  
  export function offHomeDeviceAdd(
    listener: (params: OnDeviceAddBody) => void
  ): void

  
  export function onHomeGroupRemoved(
    listener: (params: OnGroupRemovedBody) => void
  ): void

  
  export function offHomeGroupRemoved(
    listener: (params: OnGroupRemovedBody) => void
  ): void

  
  export function onHomeGroupAdd(
    listener: (params: OnGroupAddBody) => void
  ): void

  
  export function offHomeGroupAdd(
    listener: (params: OnGroupAddBody) => void
  ): void

  
  export function onShareListChange(
    listener: (params: OnShareListChangeBody) => void
  ): void

  
  export function offShareListChange(
    listener: (params: OnShareListChangeBody) => void
  ): void

  
  export function onHomeChanged(
    listener: (params: {
      
      homeName: string
      
      homeId: string
      
      longitude: string
      
      latitude: string
      
      address: string
      
      admin: boolean
      
      mode: number
      
      role: number
    }) => void
  ): void

  
  export function offHomeChanged(
    listener: (params: {
      
      homeName: string
      
      homeId: string
      
      longitude: string
      
      latitude: string
      
      address: string
      
      admin: boolean
      
      mode: number
      
      role: number
    }) => void
  ): void

  export type HomeInfoData = {
    
    homeName: string
    
    homeId: string
    
    longitude: string
    
    latitude: string
    
    address: string
    
    admin: boolean
    
    mode: number
    
    role: number
  }

  export type DIYHomeCard = {
    
    type: number
    
    contentId?: string
  }

  export type DIYHomeCardWithStyle = {
    
    type: number
    
    style?: number
    
    contentId?: string
  }

  export type RoomData = {
    
    roomId: number
    
    name: string
    
    deviceIds: string[]
  }

  export type MemberHandlerResponse = {
    
    memberId: string
    
    success: boolean
  }

  export type ThingPlayerStateHandlerResponse = {
    
    status: number
    
    data: string
  }

  export type OnDeviceRemovedBody = {
    
    deviceId: string
  }

  export type OnDeviceAddBody = {
    
    deviceId: string
  }

  export type OnGroupRemovedBody = {
    
    groupId: string
  }

  export type OnGroupAddBody = {
    
    groupId: string
  }

  export type OnShareListChangeBody = {
    
    change: boolean
  }

  export type GetDeviceRoomInfoParams = {
    
    deviceId: string
  }

  export type GetDeviceRoomInfoResponse = {
    
    roomId: number
    
    name: string
  }

  export type GroupRoomInfoParams = {
    
    groupId: string
  }

  export type GroupRoomInfoResponse = {
    
    roomId: number
    
    name: string
  }

  export type HomeIdBean = {
    
    homeId: string
  }

  export type CreateHomeParams = {
    
    homeName: string
    
    mode: number
    
    longitude?: string
    
    latitude?: string
    
    address?: string
    
    rooms?: string[]
  }

  export type SwitchHomeData = {
    
    hiddenHouseManager: boolean
  }

  export type HomeListData = {
    
    homeList: HomeInfoData[]
  }

  export type InviteParams = {
    
    homeId: number
    
    role?: number
    
    customRoleId?: number
  }

  export type InvitationMessageBean = {
    
    invitationMsgContent: string
    
    invitationCode: string
  }

  export type ThingPaymentControlEntryRequest = {
    
    entryID: string
  }

  export type ThingPaymentEntryData = {
    
    entryID: string
    
    isDisplayed: boolean
    
    entryExtensionInfo?: any
    
    entryName: string
    
    isIAPForced: boolean
  }

  export type CanDisplayDIYHomeCardParams = {
    
    card: DIYHomeCard
    
    gid: number
  }

  export type CanDisplayDIYHomeCardResult = {
    
    result: boolean
  }

  export type DidDisplayDIYHomeCardParams = {
    
    card: DIYHomeCard
    
    gid: number
  }

  export type DidDisplayDIYHomeCardResult = {
    
    result: boolean
  }

  export type AddDIYHomeCardParams = {
    
    card: DIYHomeCardWithStyle
    
    gid: number
  }

  export type RemoveDIYHomeCardParams = {
    
    card: DIYHomeCard
    
    gid: number
  }

  export type ThingMediaControlParams = {
    
    op: string
    
    data: string
  }

  export type ThingSetupFloatWindowParams = {
    
    visible: boolean
  }

  export type AIAssistantSwitchResponse = {
    
    open: boolean
  }

  export type OwnerId = {
    
    ownerId: number
    
    roomId: number
    
    devId: string
  }

  export type DeviceIdList = {
    
    devIds: string[]
  }

  export type RoomList = {
    
    roomDatas: RoomData[]
  }

  export type GroupIdList = {
    
    groupIds: string[]
  }

  export type SwitchDeviceBean = {
    
    deviceId: string
    
    roomId: number
  }

  export type TicketModel = {
    
    map?: any
  }

  export type RecommendSceneParams = {
    
    source: string
    
    sceneModel: any
  }

  export type RecommendSceneCallBack = {
    
    status?: boolean
    
    type: number
    
    data?: any
  }

  export type OpenDeviceExecutionAndAnutomationParams = {
    
    deviceId: string
    
    title?: string
  }
}
