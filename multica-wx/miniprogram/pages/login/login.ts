import { sendCode, wxBind, wxLogin as getWxCode } from '../../utils/auth'
import { MULTICA_LOGO_BASE64 } from '../../utils/constants'

const app = getApp<IAppOption>()

function isValidEmail(email: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)
}

Component({
  data: {
    email: '',
    code: '',
    loading: false,
    sendingCode: false,
    countdown: 0,
    canSendCode: false,
    canSubmit: false,
    sendCodeText: '发送验证码',
    logoSrc: MULTICA_LOGO_BASE64,
  },

  methods: {
    onEmailInput(e: WechatMiniprogram.Input) {
      const email = e.detail.value
      this.setData({
        email,
        canSendCode: isValidEmail(email),
        canSubmit: isValidEmail(email) && this.data.code.length >= 4,
      })
    },

    onCodeInput(e: WechatMiniprogram.Input) {
      const code = e.detail.value
      this.setData({
        code,
        canSubmit: isValidEmail(this.data.email) && code.length >= 4,
      })
    },

    async onSendCode() {
      const { email } = this.data
      if (!email) return

      this.setData({ sendingCode: true })
      try {
        await sendCode(email)
        wx.showToast({ title: '验证码已发送', icon: 'success' })
        this.startCountdown()
      } catch (err: unknown) {
        const e = err as { message?: string }
        wx.showToast({ title: e.message || '发送失败', icon: 'none' })
      } finally {
        this.setData({ sendingCode: false })
      }
    },

    startCountdown() {
      this.setData({ countdown: 60, sendCodeText: '60s' })
      const timer = setInterval(() => {
        const next = this.data.countdown - 1
        if (next <= 0) {
          clearInterval(timer)
          this.setData({ countdown: 0, sendCodeText: '发送验证码' })
        } else {
          this.setData({ countdown: next, sendCodeText: `${next}s` })
        }
      }, 1000)
    },

    async onSubmit() {
      const { email, code } = this.data
      if (!email || !code) return

      this.setData({ loading: true })
      try {
        const wxCode = await getWxCode()
        const result = await wxBind(email, code, wxCode)
        app.globalData.userInfo = result.user
        wx.showToast({ title: '登录成功', icon: 'success' })
        setTimeout(() => {
          wx.reLaunch({ url: '/pages/index/index' })
        }, 1000)
      } catch (err: unknown) {
        const e = err as { message?: string }
        wx.showToast({ title: e.message || '登录失败', icon: 'none' })
      } finally {
        this.setData({ loading: false })
      }
    },
  },
})
