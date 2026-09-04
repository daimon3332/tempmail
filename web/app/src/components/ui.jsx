import { cn } from '../lib/utils'
import { X } from 'lucide-react'

export function Button({ className, variant = 'default', size = 'md', ...props }) {
  const base = 'inline-flex items-center justify-center gap-2 rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 active:translate-y-px'
  const variants = {
    default: 'bg-primary text-primary-foreground hover:bg-primary/90',
    secondary: 'bg-secondary text-secondary-foreground hover:bg-secondary/80',
    outline: 'border border-border bg-background hover:bg-accent',
    ghost: 'hover:bg-accent',
    destructive: 'bg-destructive text-destructive-foreground hover:bg-destructive/90',
  }
  const sizes = { sm: 'h-8 px-3 text-xs', md: 'h-9 px-4 text-sm', lg: 'h-10 px-5 text-sm' }
  return <button className={cn(base, variants[variant], sizes[size], className)} {...props} />
}

export function Input({ className, ...props }) {
  return <input className={cn('h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', className)} {...props} />
}

export function Textarea({ className, ...props }) {
  return <textarea className={cn('w-full rounded-lg border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', className)} {...props} />
}

export function Select({ className, ...props }) {
  return <select className={cn('h-9 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', className)} {...props} />
}

export function Card({ className, ...props }) {
  return <div className={cn('rounded-md border border-border bg-card text-card-foreground', className)} {...props} />
}

export function Badge({ className, variant = 'default', ...props }) {
  const v = {
    default: 'bg-primary/10 text-primary',
    muted: 'bg-muted text-muted-foreground',
    green: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
    gray: 'bg-muted text-muted-foreground',
  }
  return <span className={cn('inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium', v[variant], className)} {...props} />
}

export function Spinner({ className }) {
  return <div className={cn('h-5 w-5 animate-spin rounded-full border-2 border-muted border-t-primary', className)} />
}

export function Modal({ title, onClose, t, children, resizable = false }) {
  const canResize = resizable || String(title || '').includes('邮件')
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div className={`w-full ${canResize ? 'max-w-[92vw] resize' : 'max-w-md'} rounded-md border border-border bg-card p-5 shadow-xl`} style={canResize ? { minWidth: 'min(32rem, 92vw)', minHeight: 'min(24rem, 88vh)', maxHeight: '88vh', overflow: 'auto' } : undefined} onClick={(e) => e.stopPropagation()}>
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-lg font-semibold">{title}</h3>
          <button onClick={onClose}><X className="h-5 w-5" /></button>
        </div>
        {children}
      </div>
    </div>
  )
}
