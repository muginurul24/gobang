import { cva, type VariantProps } from 'class-variance-authority';

export const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 rounded-full text-sm font-semibold transition duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300 focus-visible:ring-offset-2 focus-visible:ring-offset-canvas-50 disabled:pointer-events-none disabled:opacity-60 disabled:shadow-none',
  {
    variants: {
      variant: {
        default:
          'bg-linear-to-r from-canvas-900 to-canvas-800 text-ink-50 shadow-[0_14px_32px_rgba(7,16,12,0.28)] hover:-translate-y-0.5 hover:from-canvas-800 hover:to-canvas-900',
        outline: 'button-surface-outline hover:-translate-y-0.5',
        ghost: 'button-surface-ghost hover:-translate-y-0.5',
        brand:
          'bg-linear-to-r from-brand-500 via-brand-500 to-accent-500 text-canvas-900 shadow-[0_16px_34px_rgba(34,201,119,0.24)] hover:-translate-y-0.5 hover:from-brand-300 hover:to-accent-300',
      },
      size: {
        default: 'h-11 px-5',
        sm: 'h-9 px-4 text-xs',
        lg: 'h-12 px-6 text-base',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  },
);

export type ButtonVariant = VariantProps<typeof buttonVariants>['variant'];
export type ButtonSize = VariantProps<typeof buttonVariants>['size'];
