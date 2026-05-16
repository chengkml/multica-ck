import { request, setToken, removeToken, getToken } from '../utils/api'

interface WxLoginResponse {
  token: string
  user: {
    id: string
    email: string
    name: string
    avatar_url: string
  }
  bound: boolean
}

interface SendCodeResponse {
  message: string
}

interface MiniprogramTokenItem {
  id: string
  appid: string
  expires_at: string | null
  last_used_at: string | null
  created_at: string
}

function wxLogin(): Promise<string> {
  return new Promise((resolve, reject) => {
    wx.login({
      success: (res) => {
        if (res.code) {
          resolve(res.code)
        } else {
          reject(new Error(res.errMsg || 'wx.login failed'))
        }
      },
      fail: (err) => {
        reject(new Error(err.errMsg || 'wx.login failed'))
      },
    })
  })
}

async function sendCode(email: string): Promise<string> {
  const res = await request<SendCodeResponse>({
    url: '/auth/send-code',
    method: 'POST',
    data: { email },
  })
  return res.data.message
}

async function wxBind(email: string, code: string, wxCode: string): Promise<WxLoginResponse> {
  const res = await request<WxLoginResponse>({
    url: '/auth/wx-bind',
    method: 'POST',
    data: { email, code, wx_code: wxCode },
  })
  setToken(res.data.token)
  return res.data
}

async function wxSilentLogin(): Promise<WxLoginResponse | null> {
  const code = await wxLogin()
  try {
    const res = await request<WxLoginResponse>({
      url: '/auth/wx-login',
      method: 'POST',
      data: { code },
    })
    setToken(res.data.token)
    return res.data
  } catch (err: unknown) {
    const e = err as { statusCode?: number; message?: string }
    if (e.statusCode === 401) {
      return null
    }
    throw err
  }
}

async function listMiniprogramTokens(): Promise<MiniprogramTokenItem[]> {
  const res = await request<MiniprogramTokenItem[]>({
    url: '/api/miniprogram-tokens',
    method: 'GET',
  })
  return res.data
}

async function revokeMiniprogramToken(id: string): Promise<void> {
  await request({
    url: `/api/miniprogram-tokens/${id}`,
    method: 'DELETE',
  })
}

function logout(): void {
  removeToken()
}

function isLoggedIn(): boolean {
  return !!getToken()
}

export {
  wxLogin,
  sendCode,
  wxBind,
  wxSilentLogin,
  listMiniprogramTokens,
  revokeMiniprogramToken,
  logout,
  isLoggedIn,
}

export type { WxLoginResponse, MiniprogramTokenItem }
