/// <reference path="./types/index.d.ts" />

interface UserInfo {
  id: string
  email: string
  name: string
  avatar_url: string
}

interface IAppOption {
  globalData: {
    userInfo?: UserInfo
  }
  userInfoReadyCallback?: WechatMiniprogram.GetUserInfoSuccessCallback
}
