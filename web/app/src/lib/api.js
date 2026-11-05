import axios from 'axios'

// Login tokens survive browser restarts and are renewed by refreshSession();
// the raw admin password (admin-password login only) stays per-tab.
const persistent = window.localStorage
const session = window.sessionStorage
const persistentKeys = ['tm_address_jwt', 'tm_user_jwt', 'tm_user_access', 'tm_is_admin']
for (const k of persistentKeys) {
  const legacy = session.getItem(k)
  if (legacy && !persistent.getItem(k)) persistent.setItem(k, legacy)
  session.removeItem(k)
}

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
  address: persistent.getItem('tm_address') || '',
  addressJwt: validToken(persistent.getItem('tm_address_jwt')),
  userJwt: validToken(persistent.getItem('tm_user_jwt')),
  userAccessToken: validToken(persistent.getItem('tm_user_access')),
  isAdmin: persistent.getItem('tm_is_admin') === '1',
  adminAuth: session.getItem('tm_admin_auth') || '',
}

if (!store.addressJwt) store.address = ''
if (!store.userJwt) {
  store.userAccessToken = ''
  store.isAdmin = !!store.adminAuth
}

function persist() {
  persistent.setItem('tm_address', store.address)
  persistent.setItem('tm_address_jwt', store.addressJwt)
  persistent.setItem('tm_user_jwt', store.userJwt)
  persistent.setItem('tm_user_access', store.userAccessToken)
  persistent.setItem('tm_is_admin', store.isAdmin ? '1' : '0')
  session.setItem('tm_admin_auth', store.adminAuth)
}
persist()

export const state = store

export function setAddress(a, jwt) { store.address = a; store.addressJwt = jwt || ''; persist() }
export function setUser(jwt, isAdmin = false) { store.userJwt = jwt || ''; store.isAdmin = !!isAdmin; persist() }
export function setAdmin(auth) { store.adminAuth = auth || ''; store.isAdmin = !!auth; persist() }
export function setAccessToken(t) { store.userAccessToken = t || ''; persist() }
export function clearAddress() { setAddress('', ''); }
export function clearUser() { setUser(''); setAccessToken(''); }
export function clearAdmin() { setAdmin(''); }

function authHeaders() {
  const h = { 'x-lang': persistent.getItem('tm_locale') || 'zh' }
  if (store.addressJwt) h.Authorization = 'Bearer ' + store.addressJwt
  if (store.userJwt) h['x-user-token'] = store.userJwt
  if (store.userAccessToken) h['x-user-access-token'] = store.userAccessToken
  if (store.adminAuth) h['x-admin-auth'] = store.adminAuth
  return h
}

export async function download(path, filename) {
  const res = await fetch(path, { headers: authHeaders() })
  if (!res.ok) throw new Error(await res.text())
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a'); a.href = url; a.download = filename; a.click()
  URL.revokeObjectURL(url)
}

const client = axios.create({ timeout: 30000, validateStatus: () => true })

client.interceptors.request.use(cfg => {
  Object.assign(cfg.headers, authHeaders())
  return cfg
})

client.interceptors.response.use(res => {
  // user settings responses carry a fresh access token and, near expiry, a renewed user token
  if (typeof res.data === 'object' && res.data) {
    if (res.data.access_token) setAccessToken(res.data.access_token)
    if (res.data.new_user_token) setUser(res.data.new_user_token, store.isAdmin)
  }
  return res
})

export async function api(path, method = 'GET', data, extra) {
  const res = await client.request({ url: path, method, data, ...extra })
  return res
}

let lastRefresh = 0

// refreshSession renews the access token (1h) and, within 7 days of expiry,
// the user token (30d), so an account used at least monthly stays signed in.
export async function refreshSession() {
  if (!store.userJwt) return
  lastRefresh = Date.now()
  try {
    const res = await api('/user_api/settings')
    if (res.status === 401) { clearUser(); return }
    if (res.status === 200 && res.data) {
      store.isAdmin = !!res.data.is_admin || !!store.adminAuth
      persist()
    }
  } catch {
    // network failure: keep the current session and retry on the next tick
  }
}

export function keepSessionAlive() {
  const stale = () => Date.now() - lastRefresh > 50 * 60 * 1000
  setInterval(() => { if (stale()) refreshSession() }, 60 * 1000)
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'visible' && stale()) refreshSession() })
}

export default api
