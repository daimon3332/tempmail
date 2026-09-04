import { createContext, useContext, useEffect, useState } from 'react'
import { translate } from '../lib/i18n'

const Ctx = createContext(null)

export function AppProvider({ children }) {
  const [locale, setLocale] = useState(localStorage.getItem('tm_locale') || 'zh')
  const [dark, setDark] = useState(localStorage.getItem('tm_dark') === '1')
  useEffect(() => {
    localStorage.setItem('tm_locale', locale)
    document.documentElement.lang = locale
  }, [locale])
  useEffect(() => {
    localStorage.setItem('tm_dark', dark ? '1' : '0')
    document.documentElement.classList.toggle('dark', dark)
  }, [dark])
  const t = (key) => translate(locale, key)
  const value = { locale, setLocale, dark, setDark, t }
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useApp() {
  return useContext(Ctx)
}
