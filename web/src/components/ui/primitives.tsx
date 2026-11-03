import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import * as TabsPrimitive from '@radix-ui/react-tabs'
import * as CollapsiblePrimitive from '@radix-ui/react-collapsible'
import * as SliderPrimitive from '@radix-ui/react-slider'
import * as DropdownPrimitive from '@radix-ui/react-dropdown-menu'
import * as SeparatorPrimitive from '@radix-ui/react-separator'
import { X } from 'lucide-react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

/* -------------------------------------------------------------------------- */
/* Button                                                                      */
/* -------------------------------------------------------------------------- */

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-[background-color,border-color,color,box-shadow,transform,filter] duration-150 active:scale-[0.98] active:brightness-95 disabled:pointer-events-none disabled:opacity-45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50',
  {
    variants: {
      variant: {
        primary:
          'bg-gradient-to-b from-sky-400 to-sky-500 text-slate-950 shadow-[0_0_20px_-6px_rgba(56,189,248,0.65)] hover:from-sky-300 hover:to-sky-400',
        secondary: 'bg-white/[0.06] text-slate-100 border border-white/10 hover:bg-white/[0.1]',
        ghost: 'text-slate-300 hover:bg-white/[0.06] hover:text-slate-100',
        danger: 'bg-red-500/15 text-red-300 border border-red-500/30 hover:bg-red-500/25',
        outline: 'border border-white/15 bg-transparent text-slate-200 hover:bg-white/[0.06]',
      },
      size: {
        sm: 'h-8 px-3 text-xs',
        md: 'h-9 px-4',
        lg: 'h-12 px-6 text-base',
        icon: 'h-8 w-8',
      },
    },
    defaultVariants: { variant: 'secondary', size: 'md' },
  },
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
  ),
)
Button.displayName = 'Button'

/* -------------------------------------------------------------------------- */
/* Inputs                                                                      */
/* -------------------------------------------------------------------------- */

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => (
    <input
      ref={ref}
      className={cn(
        'h-9 w-full rounded-lg border border-white/10 bg-black/30 px-3 text-base text-slate-100 placeholder:text-slate-500 outline-none transition-colors focus:border-accent/60 focus:ring-2 focus:ring-accent/20 sm:text-sm',
        className,
      )}
      {...props}
    />
  ),
)
Input.displayName = 'Input'

export const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  ({ className, ...props }, ref) => (
    <textarea
      ref={ref}
      className={cn(
        'w-full rounded-lg border border-white/10 bg-black/30 px-3 py-2 text-base text-slate-100 placeholder:text-slate-500 outline-none transition-colors focus:border-accent/60 focus:ring-2 focus:ring-accent/20 sm:text-sm',
        className,
      )}
      {...props}
    />
  ),
)
Textarea.displayName = 'Textarea'

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn('text-xs font-medium uppercase tracking-wide text-slate-400', className)} {...props} />
}

export const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, children, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        'h-9 w-full appearance-none rounded-lg border border-white/10 bg-black/30 px-3 pr-8 text-base text-slate-100 outline-none transition-colors focus:border-accent/60 focus:ring-2 focus:ring-accent/20 sm:text-sm',
        className,
      )}
      {...props}
    >
      {children}
    </select>
  ),
)
Select.displayName = 'Select'

/* -------------------------------------------------------------------------- */
/* Surfaces                                                                    */
/* -------------------------------------------------------------------------- */

export function Card({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'rounded-2xl border border-white/[0.07] bg-surface/80 shadow-[0_1px_0_0_rgba(255,255,255,0.03)_inset] transition-colors duration-200 hover:border-white/[0.12]',
        className,
      )}
      {...props}
    />
  )
}

export function CardHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('flex items-start justify-between gap-4 px-5 pt-5', className)} {...props} />
}

export function CardTitle({ className, ...props }: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn('text-sm font-semibold tracking-tight text-slate-100', className)} {...props} />
}

export function CardContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('px-5 py-4', className)} {...props} />
}

export function Badge({
  className,
  tone = 'neutral',
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & { tone?: 'neutral' | 'accent' | 'success' | 'warn' | 'danger' | 'muted' }) {
  const tones: Record<string, string> = {
    neutral: 'border-white/10 bg-white/[0.05] text-slate-300',
    accent: 'border-sky-400/30 bg-sky-400/10 text-sky-300',
    success: 'border-emerald-400/30 bg-emerald-400/10 text-emerald-300',
    warn: 'border-amber-400/30 bg-amber-400/10 text-amber-300',
    danger: 'border-red-400/30 bg-red-400/10 text-red-300',
    muted: 'border-white/5 bg-white/[0.02] text-slate-500',
  }
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] font-medium',
        tones[tone],
        className,
      )}
      {...props}
    />
  )
}

export function Separator({ className, ...props }: React.ComponentPropsWithoutRef<typeof SeparatorPrimitive.Root>) {
  return <SeparatorPrimitive.Root className={cn('h-px w-full bg-white/[0.07]', className)} {...props} />
}

export function Progress({ value, className, tone = 'accent' }: { value: number; className?: string; tone?: 'accent' | 'success' | 'warn' }) {
  const tones = { accent: 'bg-sky-400', success: 'bg-emerald-400', warn: 'bg-amber-400' }
  return (
    <div className={cn('h-1.5 w-full overflow-hidden rounded-full bg-white/[0.07]', className)}>
      <div
        className={cn(
          'h-full origin-left rounded-full transition-transform duration-300 ease-[cubic-bezier(0.16,1,0.3,1)]',
          tones[tone],
        )}
        style={{ transform: `scaleX(${Math.max(0, Math.min(100, value)) / 100})` }}
      />
    </div>
  )
}

export function Skeleton({ className }: { className?: string }) {
  return <div className={cn('animate-pulse rounded-md bg-white/[0.06]', className)} />
}

export function Switch({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label?: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      onClick={() => onChange(!checked)}
      className={cn(
        'relative h-5 w-9 shrink-0 rounded-full border transition-colors',
        checked ? 'border-sky-400/50 bg-sky-400/80' : 'border-white/10 bg-white/[0.08]',
      )}
    >
      <span
        className={cn(
          'absolute top-0.5 h-3.5 w-3.5 rounded-full bg-white transition-transform',
          checked ? 'translate-x-[18px]' : 'translate-x-0.5',
        )}
      />
    </button>
  )
}

/* -------------------------------------------------------------------------- */
/* Slider / Tabs / Tooltip / Collapsible                                       */
/* -------------------------------------------------------------------------- */

export function Slider({
  value,
  min,
  max,
  step = 1,
  onValueChange,
}: {
  value: number
  min: number
  max: number
  step?: number
  onValueChange: (n: number) => void
}) {
  return (
    <SliderPrimitive.Root
      className="relative flex h-5 w-full touch-none select-none items-center"
      value={[value]}
      min={min}
      max={max}
      step={step}
      onValueChange={(v) => onValueChange(v[0])}
    >
      <SliderPrimitive.Track className="relative h-1 w-full grow overflow-hidden rounded-full bg-white/10">
        <SliderPrimitive.Range className="absolute h-full bg-sky-400/80" />
      </SliderPrimitive.Track>
      <SliderPrimitive.Thumb className="block h-3.5 w-3.5 rounded-full border border-sky-300/60 bg-sky-400 shadow-[0_0_10px_rgba(56,189,248,0.5)] focus:outline-none" />
    </SliderPrimitive.Root>
  )
}

export const Tabs = TabsPrimitive.Root

export function TabsList({ className, ...props }: React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>) {
  return (
    <TabsPrimitive.List
      className={cn('inline-flex items-center gap-1 rounded-lg border border-white/10 bg-black/30 p-1', className)}
      {...props}
    />
  )
}

export function TabsTrigger({ className, ...props }: React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      className={cn(
        'rounded-md px-3 py-1.5 text-xs font-medium text-slate-400 transition-colors data-[state=active]:bg-white/[0.08] data-[state=active]:text-slate-100',
        className,
      )}
      {...props}
    />
  )
}

export function TabsContent({ className, ...props }: React.ComponentPropsWithoutRef<typeof TabsPrimitive.Content>) {
  return <TabsPrimitive.Content className={cn('mt-4 focus:outline-none', className)} {...props} />
}

export const TooltipProvider = TooltipPrimitive.Provider

export function Tooltip({ children, content }: { children: React.ReactNode; content: React.ReactNode }) {
  return (
    <TooltipPrimitive.Root delayDuration={150}>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          sideOffset={8}
          style={{ transformOrigin: 'var(--radix-tooltip-content-transform-origin)' }}
          className="sd-pop z-50 max-w-[min(24rem,calc(100vw-2rem))] rounded-xl border border-white/10 bg-elevated/95 px-3 py-2 text-xs text-slate-300 shadow-2xl backdrop-blur-md"
        >
          {content}
          <TooltipPrimitive.Arrow className="fill-elevated" />
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  )
}

export const Collapsible = CollapsiblePrimitive.Root
export const CollapsibleTrigger = CollapsiblePrimitive.Trigger
export const CollapsibleContent = CollapsiblePrimitive.Content

/* -------------------------------------------------------------------------- */
/* Dialog / Sheet                                                              */
/* -------------------------------------------------------------------------- */

export const Dialog = DialogPrimitive.Root
export const DialogTrigger = DialogPrimitive.Trigger
export const DialogClose = DialogPrimitive.Close

export function DialogContent({
  className,
  children,
  title,
  description,
  wide,
}: {
  className?: string
  children: React.ReactNode
  title: string
  description?: string
  wide?: boolean
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="sd-overlay fixed inset-0 z-40 bg-black/70 backdrop-blur-sm" />
      <DialogPrimitive.Content
        className={cn(
          'sd-dialog fixed left-1/2 top-1/2 z-50 max-h-[88dvh] w-[calc(100vw-2rem)] -translate-x-1/2 -translate-y-1/2 overflow-hidden rounded-2xl border border-white/10 bg-[#0d0f16]/95 shadow-2xl backdrop-blur-xl focus:outline-none max-sm:bottom-0 max-sm:left-0 max-sm:top-auto max-sm:max-h-[92dvh] max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-b-none',
          wide ? 'max-w-4xl' : 'max-w-lg',
          className,
        )}
      >
        <div className="flex items-start justify-between gap-4 border-b border-white/[0.07] px-5 py-4">
          <div>
            <DialogPrimitive.Title className="text-sm font-semibold text-slate-100">{title}</DialogPrimitive.Title>
            {description && (
              <DialogPrimitive.Description className="mt-1 text-xs text-slate-500">{description}</DialogPrimitive.Description>
            )}
          </div>
          <DialogPrimitive.Close className="rounded-md p-1 text-slate-500 transition-colors hover:bg-white/[0.06] hover:text-slate-200">
            <X className="h-4 w-4" />
          </DialogPrimitive.Close>
        </div>
        <div className="scroll-contain max-h-[calc(88dvh-70px)] overflow-y-auto px-5 pb-[max(1rem,env(safe-area-inset-bottom,0px))] pt-4 max-sm:max-h-[calc(92dvh-70px)]">
          {children}
        </div>
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}

export const Sheet = DialogPrimitive.Root
export const SheetTrigger = DialogPrimitive.Trigger

export function SheetContent({
  children,
  title,
  description,
}: {
  children: React.ReactNode
  title: string
  description?: string
}) {
  return (
    <DialogPrimitive.Portal>
      <DialogPrimitive.Overlay className="sd-overlay fixed inset-0 z-40 bg-black/60 backdrop-blur-sm" />
      <DialogPrimitive.Content className="sd-sheet scroll-contain fixed right-0 top-0 z-50 h-[100dvh] w-[min(440px,100vw)] overflow-y-auto border-l border-white/10 bg-[#0d0f16]/95 p-5 pb-[max(1.25rem,env(safe-area-inset-bottom,0px))] pt-[max(1.25rem,env(safe-area-inset-top,0px))] shadow-2xl backdrop-blur-xl focus:outline-none">
        <div className="mb-4 flex items-start justify-between gap-4">
          <div>
            <DialogPrimitive.Title className="text-sm font-semibold text-slate-100">{title}</DialogPrimitive.Title>
            {description && <DialogPrimitive.Description className="mt-1 text-xs text-slate-500">{description}</DialogPrimitive.Description>}
          </div>
          <DialogPrimitive.Close className="rounded-md p-1 text-slate-500 hover:bg-white/[0.06] hover:text-slate-200">
            <X className="h-4 w-4" />
          </DialogPrimitive.Close>
        </div>
        {children}
      </DialogPrimitive.Content>
    </DialogPrimitive.Portal>
  )
}

/* -------------------------------------------------------------------------- */
/* Dropdown menu                                                               */
/* -------------------------------------------------------------------------- */

export const DropdownMenu = DropdownPrimitive.Root
export const DropdownMenuTrigger = DropdownPrimitive.Trigger

export function DropdownMenuContent({ children }: { children: React.ReactNode }) {
  return (
    <DropdownPrimitive.Portal>
      <DropdownPrimitive.Content
        sideOffset={6}
        style={{ transformOrigin: 'var(--radix-dropdown-menu-content-transform-origin)' }}
        className="sd-pop scroll-contain z-50 max-h-[60dvh] min-w-[180px] overflow-y-auto rounded-xl border border-white/10 bg-elevated/95 p-1 text-xs text-slate-300 shadow-2xl backdrop-blur-md"
      >
        {children}
      </DropdownPrimitive.Content>
    </DropdownPrimitive.Portal>
  )
}

export function DropdownMenuItem({
  children,
  onSelect,
  disabled,
  danger,
}: {
  children: React.ReactNode
  onSelect?: () => void
  disabled?: boolean
  danger?: boolean
}) {
  return (
    <DropdownPrimitive.Item
      disabled={disabled}
      onSelect={onSelect}
      className={cn(
        'cursor-pointer select-none rounded-lg px-2.5 py-1.5 outline-none transition-colors data-[highlighted]:bg-white/[0.07]',
        danger ? 'text-red-300' : 'text-slate-300',
        disabled && 'cursor-not-allowed opacity-40',
      )}
    >
      {children}
    </DropdownPrimitive.Item>
  )
}

/* -------------------------------------------------------------------------- */
/* Table                                                                       */
/* -------------------------------------------------------------------------- */

export function Table({ className, ...props }: React.TableHTMLAttributes<HTMLTableElement>) {
  return (
    <div className="scroll-contain w-full overflow-x-auto">
      <table className={cn('w-full caption-bottom text-left text-sm', className)} {...props} />
    </div>
  )
}

export function THead({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <thead className={cn('[&_th]:h-9 [&_th]:px-3 [&_th]:text-[11px] [&_th]:font-medium [&_th]:uppercase [&_th]:tracking-wide [&_th]:text-slate-500', className)} {...props} />
}

export function TBody({ className, ...props }: React.HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody className={cn('[&_td]:px-3 [&_td]:py-2.5 [&_tr]:border-t [&_tr]:border-white/[0.05]', className)} {...props} />
}
