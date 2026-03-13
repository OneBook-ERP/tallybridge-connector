import { cn } from '../../lib/utils'

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
    variant?: 'default' | 'outline' | 'ghost' | 'destructive'
    size?: 'sm' | 'md' | 'lg'
}

export function Button({ variant = 'default', size = 'md', className, ...props }: ButtonProps) {
    return (
        <button
            className={cn(
                'inline-flex items-center justify-center gap-2 font-medium rounded-lg transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 disabled:opacity-50 disabled:cursor-not-allowed',
                variant === 'default' && 'bg-slate-900 text-white hover:bg-slate-700',
                variant === 'outline' && 'border border-slate-200 bg-white text-slate-700 hover:bg-slate-50',
                variant === 'ghost' && 'text-slate-600 hover:bg-slate-100 hover:text-slate-900',
                variant === 'destructive' && 'bg-red-600 text-white hover:bg-red-500',
                size === 'sm' && 'text-xs px-3 py-1.5',
                size === 'md' && 'text-sm px-4 py-2',
                size === 'lg' && 'text-base px-6 py-3',
                className
            )}
            {...props}
        />
    )
}
