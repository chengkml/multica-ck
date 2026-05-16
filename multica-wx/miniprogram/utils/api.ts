const BASE_URL = 'https://www.quizck.cn/multica'

type Method = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH'

interface RequestOptions {
  url: string
  method?: Method
  data?: Record<string, unknown>
  header?: Record<string, string>
}

interface ApiResponse<T = unknown> {
  data: T
  statusCode: number
}

function getToken(): string {
  return wx.getStorageSync('auth_token') || ''
}

function setToken(token: string): void {
  wx.setStorageSync('auth_token', token)
}

function removeToken(): void {
  wx.removeStorageSync('auth_token')
}

function request<T = unknown>(options: RequestOptions): Promise<ApiResponse<T>> {
  const token = getToken()
  const header: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Client-Platform': 'miniprogram',
    ...options.header,
  }
  if (token) {
    header['Authorization'] = `Bearer ${token}`
  }

  return new Promise((resolve, reject) => {
    wx.request({
      url: `${BASE_URL}${options.url}`,
      method: options.method || 'GET',
      data: options.data,
      header,
      success: (res) => {
        if (res.statusCode === 401) {
          removeToken()
          reject({ statusCode: 401, message: 'unauthorized' })
          return
        }
        if (res.statusCode >= 400) {
          const data = res.data as Record<string, string>
          const errMsg = (data && data.error) || `request failed with status ${res.statusCode}`
          reject({ statusCode: res.statusCode, message: errMsg })
          return
        }
        resolve({ data: res.data as T, statusCode: res.statusCode })
      },
      fail: (err) => {
        reject({ statusCode: 0, message: err.errMsg || 'network error' })
      },
    })
  })
}

export { request, getToken, setToken, removeToken, BASE_URL }
