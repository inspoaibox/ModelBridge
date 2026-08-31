import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-semibold tracking-wide transition-colors focus:outline-none focus:ring-2 focus:ring-ring focus:ring-offset-2",
  {
    variants: {
      variant: {
        default: "border-indigo-500/30 bg-indigo-50 dark:bg-indigo-500/15 text-indigo-700 dark:text-indigo-300 shadow-sm shadow-indigo-500/10",
        secondary: "border-slate-200 dark:border-slate-700 bg-slate-100 dark:bg-slate-800/80 text-slate-700 dark:text-slate-300",
        outline: "border-slate-200 dark:border-slate-700/80 text-slate-700 dark:text-slate-300 bg-white dark:bg-slate-900/40",
        success: "border-emerald-500/30 bg-emerald-50 dark:bg-emerald-500/15 text-emerald-700 dark:text-emerald-400 shadow-sm shadow-emerald-500/10",
        warning: "border-amber-500/30 bg-amber-50 dark:bg-amber-500/15 text-amber-700 dark:text-amber-400 shadow-sm shadow-amber-500/10",
        destructive: "border-rose-500/30 bg-rose-50 dark:bg-rose-500/15 text-rose-700 dark:text-rose-400 shadow-sm shadow-rose-500/10",
        cyan: "border-cyan-500/30 bg-cyan-50 dark:bg-cyan-500/15 text-cyan-700 dark:text-cyan-300 shadow-sm shadow-cyan-500/10",
        purple: "border-purple-500/30 bg-purple-50 dark:bg-purple-500/15 text-purple-700 dark:text-purple-300 shadow-sm shadow-purple-500/10",
        muted: "border-slate-200 dark:border-slate-800 bg-slate-100 dark:bg-slate-800/60 text-slate-500 dark:text-slate-400",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  }
);

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
