import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 active:scale-[0.98]",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90 shadow-md hover:shadow-lg",
        destructive: "bg-destructive text-destructive-foreground hover:bg-destructive/90 shadow-md",
        outline: "border border-input bg-background hover:bg-accent hover:text-accent-foreground",
        secondary: "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost: "hover:bg-accent hover:text-accent-foreground",
        link: "text-primary underline-offset-4 hover:underline",
        gradient: "bg-gradient-to-r from-twilight-500 to-sunset-500 text-white hover:from-twilight-600 hover:to-sunset-600 shadow-lg hover:shadow-xl",
        glow: "bg-primary text-primary-foreground shadow-lg shadow-primary/30 hover:shadow-primary/50 hover:bg-primary/90",
      },
      size: {
        default: "h-11 px-4 py-2 sm:h-10",
        sm: "h-10 rounded-md px-3 sm:h-9",
        lg: "h-11 rounded-md px-8",
        xl: "h-12 rounded-lg px-10 text-base",
        icon: "h-11 w-11 sm:h-10 sm:w-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

/**
 * iconOnlyA11yWarned 用模块级 Set 去重，让同一调用栈只在 dev 期打一次告警，
 * 避免大列表（regcodes / scheduler 几十行）刷屏。
 */
const iconOnlyA11yWarned = new Set<string>();

/**
 * warnIfIconOnlyMissingLabel 在 dev 期检测 size="icon" 但既无 aria-label
 * 也无 aria-labelledby 的 Button——纯图标按钮屏幕阅读器只能朗读"按钮"，
 * 违反 WCAG 4.1.2。
 *
 * 类型层面新增 `IconButton` 组件强制 aria-label；存量 50+ 调用点逐步迁移
 * 时由本运行时告警兜底。生产构建静默，不影响最终用户。
 */
function warnIfIconOnlyMissingLabel(props: ButtonProps): void {
  if (process.env.NODE_ENV === "production") return;
  if (props.size !== "icon") return;
  const ariaLabel = (props as Record<string, unknown>)["aria-label"];
  const ariaLabelledBy = (props as Record<string, unknown>)["aria-labelledby"];
  const hasLabel = typeof ariaLabel === "string" && ariaLabel.trim().length > 0;
  const hasLabelledBy = typeof ariaLabelledBy === "string" && ariaLabelledBy.trim().length > 0;
  if (hasLabel || hasLabelledBy) return;
  // 用 className + variant 当指纹去重，命中同一组件就只警告一次。
  const key = `${props.variant ?? "default"}|${props.className ?? ""}`;
  if (iconOnlyA11yWarned.has(key)) return;
  iconOnlyA11yWarned.add(key);
  // eslint-disable-next-line no-console
  console.warn(
    "[a11y] <Button size=\"icon\"> 缺少 aria-label / aria-labelledby，屏幕阅读器只能朗读\"按钮\"。" +
      "请补 aria-label 或改用 <IconButton aria-label=\"...\">。",
  );
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    warnIfIconOnlyMissingLabel({ ...props, variant, size, className });
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

/**
 * IconButton 是 Button 的类型级 a11y 封装：固定 size="icon"，并在 props
 * 类型上强制要求 `aria-label` 必传（非空字符串）。新增图标按钮一律走这里，
 * 编译期就拦掉裸 `<Button size="icon">` 的 a11y 缺失。
 *
 * 设计取舍：没在 ButtonProps 顶层做条件类型要求 aria-label，因为这会让
 * 50+ 现存调用点立刻 type-error；通过 IconButton + 运行时 dev warn
 * 兜底，存量按域逐步迁移即可，新人写新代码用 IconButton 一次成型。
 */
export type IconButtonProps = Omit<ButtonProps, "size" | "aria-label"> & {
  "aria-label": string;
};

const IconButton = React.forwardRef<HTMLButtonElement, IconButtonProps>(
  (props, ref) => <Button ref={ref} size="icon" {...props} />,
);
IconButton.displayName = "IconButton";

export { Button, IconButton, buttonVariants };

