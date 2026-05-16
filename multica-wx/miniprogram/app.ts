import { wxSilentLogin, isLoggedIn } from './utils/auth'

App<IAppOption>({
  globalData: {
    userInfo: undefined,
  },
  onLaunch() {
    if (isLoggedIn()) {
      return
    }
    wxSilentLogin()
      .then((result) => {
        if (result) {
          this.globalData.userInfo = result.user
          console.log('silent login success')
        }
      })
      .catch((err) => {
        console.warn('silent login failed:', err)
      })
  },
})
