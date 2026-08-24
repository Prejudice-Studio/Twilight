import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/**
 * inputVariants 把表单原子的视觉态收拢到 CVA 中。
 *
 * 旧实现是单字符串 className + 业务页用 `border-destructive` 散点拼接，
 * 视觉令牌散落、a11y 与设计无法联动；改成 variant 后：
 *   - `variant="default"` 走 ring/border 默认
 *   - `variant="error"` 自动联动 `border-destructive` + 错误聚焦环
 *   - `size` 统一 sm/default/lg 三档高度
 *   - `invalid` 便捷 prop 等价于 variant="error" 且自动设 aria-invalid，
 *     避免业务侧只设 className 忘了无障碍属性。
 */
const inputVariants = cva(
  "flex w-full rounded-md border bg-background px-3 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 transition-all",
  {
    variants: {
      variant: {
        default: "border-input focus-visible:ring-ring",
        error: "border-destructive focus-visible:ring-destructive",
        success: "border-success focus-visible:ring-success",
      },
      inputSize: {
        sm: "h-9",
        default: "h-12 sm:h-10",
        lg: "h-12",
      },
    },
    defaultVariants: {
      variant: "default",
      inputSize: "default",
    },
  },
);

export interface InputProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "size">,
    VariantProps<typeof inputVariants> {
  /**
   * invalid=true 等价于 variant="error" 且自动 aria-invalid="true"。
   * 业务页拿到表单校验失败状态后传 invalid 即可，不必再手拼 className。
   */
  invalid?: boolean;
}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, variant, inputSize, invalid, ...props }, ref) => {
    const effectiveVariant = invalid ? "error" : variant;
    return (
      <input
        type={type}
        aria-invalid={invalid ? true : props["aria-invalid"]}
        className={cn(inputVariants({ variant: effectiveVariant, inputSize, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Input.displayName = "Input";

export { Input, inputVariants };
