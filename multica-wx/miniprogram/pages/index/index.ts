import { isLoggedIn, logout } from '../../utils/auth'

const app = getApp<IAppOption>()
const defaultAvatarUrl = 'https://mmbiz.qpic.cn/mmbiz/icTdbqWNOwNRna42FI242Lcia07jQodd2FJGIYQfG0LAJGFxM4FbnQP6yfMxBgJ0F3YRqJCJ1aPAK2dQagdusBZg/0'

Component({
  data: {
    userInfo: {} as UserInfo,
    defaultAvatarUrl,
  },

  lifetimes: {
    attached() {
      this.checkAuth()
    },
  },

  pageLifetimes: {
    show() {
      this.checkAuth()
    },
  },

  methods: {
    checkAuth() {
      if (!isLoggedIn()) {
        wx.redirectTo({ url: '/pages/login/login' })
        return
      }
      const user = app.globalData.userInfo
      this.setData({ userInfo: user || {} })
    },

    onLogout() {
      wx.showModal({
        title: '提示',
        content: '确定要退出登录吗？',
        success: (res) => {
          if (res.confirm) {
            logout()
            app.globalData.userInfo = undefined
            wx.redirectTo({ url: '/pages/login/login' })
          }
        },
      })
    },
  },
})
