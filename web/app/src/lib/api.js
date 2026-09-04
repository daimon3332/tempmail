import axios from 'axios'

const secure = window.sessionStorage
function validToken(value) {
  if (!value) return ''
  try {
    const part = value.split('.')[1]
    if (!part) return ''
    const payload = JSON.parse(atob(part.replace(/-/g, '+').replace(/_/g, '/')))
    if (payload.exp && Number(payload.exp) <= Math.floor(Date.now() / 1000)) return ''
    return value
  } catch {
    return ''
  }
}
const store = {
  address: localStorage.getItem('tm_address') || '',
  addressJwt: validToken(secure.getItem('tm_address_jwt')),
  userJwt: validToken(secure.getItem('tm_user_jwt')),
  userAccessToken: validToken(secure.getItem('tm_user_access')),
  isAdmin: secure.getItem('tm_is_admin') === '1',
  adminAuth: secure.getItem('tm_admin_auth') || '',
}

if (!store.addressJwt) localStorage.removeItem('tm_address')
if (!store.userJwt) {
  store.isAdmin = false
  store.userAccessToken = ''
  store.adminAuth = ''
} else if (!store.userAccessToken && !store.adminAuth) {
  store.isAdmin = false
}

function persist() {
  localStorage.setItem('tm_address', store.address)
  secure.setItem('tm_address_jwt', store.addressJwt)
  secure.setItem('tm_user_jwt', store.userJwt)
  secure.setItem('tm_user_access', store.userAccessToken)
  secure.setItem('tm_is_admin', store.isAdmin ? '1' : '0')
  secure.setItem('tm_admin_auth', store.adminAuth)
}

export const state = store

export function setAddress(a, jwt) { store.address = a; store.addressJwt = jwt || ''; persist() }
export function setUser(jwt, isAdmin = false) { store.userJwt = jwt || ''; store.isAdmin = !!isAdmin; persist() }
export function setAdmin(auth) { store.adminAuth = auth || ''; store.isAdmin = !!auth; persist() }
export function setAccessToken(t) { store.userAccessToken = t || ''; persist() }
export function clearAddress() { setAddress('', ''); }
export function clearUser() { setUser(''); setAccessToken(''); }
export function clearAdmin() { setAdmin(''); }

export async function download(path, filename) {
  const headers = { 'x-lang': localStorage.getItem('tm_locale') || 'zh' }
  if (store.adminAuth) headers['x-admin-auth'] = store.adminAuth
  if (store.addressJwt) headers.Authorization = 'Bearer ' + store.addressJwt
  const res = await fetch(path, { headers })
  if (!res.ok) throw new Error(await res.text())
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a'); a.href = url; a.download = filename; a.click()
  URL.revokeObjectURL(url)
}

const client = axios.create({ timeout: 30000, validateStatus: () => true })

client.interceptors.request.use(cfg => {
  const h = cfg.headers
  h['x-lang'] = localStorage.getItem('tm_locale') || 'zh'
  if (store.addressJwt) h['Authorization'] = 'Bearer ' + store.addressJwt
  if (store.userJwt) h['x-user-token'] = store.userJwt
  if (store.userAccessToken) h['x-user-access-token'] = store.userAccessToken
  if (store.adminAuth) h['x-admin-auth'] = store.adminAuth
  return cfg
})

client.interceptors.response.use(res => {
  // pull a fresh access token from user settings responses
  if (typeof res.data === 'object' && res.data && res.data.access_token) {
    setAccessToken(res.data.access_token)
  }
  return res
})

export async function api(path, method = 'GET', data, extra) {
  const res = await client.request({ url: path, method, data, ...extra })
  return res
}

export default api
