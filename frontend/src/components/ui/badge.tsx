import { cn } from '../../lib/utils'

interface BadgeProps {
    children: React.ReactNode
    variant?: 'default' | 'success' | 'warning' | 'error' | 'outline'
    className?: string
}

export function Badge({ children, variant = 'default', className }: BadgeProps) {
    return (
        <span className={cn(
            'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold',
            variant === 'default' && 'bg-slate-100 text-slate-700',
            variant === 'success' && 'bg-green-100 text-green-700',
            variant === 'warning' && 'bg-yellow-100 text-yellow-700',
            variant === 'error' && 'bg-red-100 text-red-700',
            variant === 'outline' && 'border border-slate-200 text-slate-600',
            className
        )}>
            {children}
        </span>
    )
}
