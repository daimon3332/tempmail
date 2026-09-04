import { clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'
export function cn(...inputs) { return twMerge(clsx(inputs)) }
export function fmtTime(s) {
  if (!s) return ''
  const d = new Date(s.endsWith('Z') ? s : s + 'Z')
  const now = new Date()
  const diff = (now - d) / 1000
  if (diff < 60) return 'now'
  if (diff < 3600) return Math.floor(diff/60) + 'm'
  if (diff < 86400) return Math.floor(diff/3600) + 'h'
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}
export async function sha256(s) {
  const d = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(s))
  return [...new Uint8Array(d)].map(b => b.toString(16).padStart(2, '0')).join('')
}
