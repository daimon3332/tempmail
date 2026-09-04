import axios from 'axios'

const store = {
  address: localStorage.getItem('tm_address') || '',
  addressJwt: localStorage.getItem('tm_address_jwt') || '',
  userJwt: localStorage.getItem('tm_user_jwt') || '',
  userAccessToken: localStorage.getItem('tm_user_access') || '',
  adminAuth: localStorage.getItem('tm_admin_auth') || '',
}

function persist() {
  localStorage.setItem('tm_address', store.address)
  localStorage.setItem('tm_address_jwt', store.addressJwt)
  localStorage.setItem('tm_user_jwt', store.userJwt)
  localStorage.setItem('tm_user_access', store.userAccessToken)
  localStorage.setItem('tm_admin_auth', store.adminAuth)
}

export const state = store

export function setAddress(a, jwt) { store.address = a; store.addressJwt = jwt || ''; persist() }
export function setUser(jwt) { store.userJwt = jwt || ''; persist() }
export function setAdmin(auth) { store.adminAuth = auth || ''; persist() }
export function setAccessToken(t) { store.userAccessToken = t || ''; persist() }
export function clearAddress() { setAddress('', ''); }
export function clearUser() { setUser(''); setAccessToken(''); }
export function clearAdmin() { setAdmin(''); }

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
