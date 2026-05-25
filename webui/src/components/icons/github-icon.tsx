import { forwardRef, type SVGProps } from "react";

import { cn } from "@/lib/utils";

// lucide-react 在 v1.0 起把 GitHub 等品牌图标移出主包（lucide/lucide#1675），
// 升级时不能再 `import { Github } from "lucide-react"`。
// 这里内联一个与 lucide 视觉风格一致的 24x24 / stroke=currentColor 图标，
// 调用点直接用 `<GithubIcon className="h-4 w-4" />` 替代原 `<Github />`。
//
// SVG 路径来自 simple-icons (CC0)：https://simpleicons.org/icons/github
// 不再依赖 lucide 的 stroke 风格——品牌图标本身就是 fill 实心字形，
// 跟 lucide 的描边风格其实并不一致，强行还原 stroke 反而失真。
export type GithubIconProps = SVGProps<SVGSVGElement>;

export const GithubIcon = forwardRef<SVGSVGElement, GithubIconProps>(function GithubIcon(
  { className, ...props },
  ref,
) {
  return (
    <svg
      ref={ref}
      viewBox="0 0 24 24"
      fill="currentColor"
      aria-hidden={props["aria-label"] ? undefined : true}
      role={props["aria-label"] ? "img" : undefined}
      className={cn("inline-block", className)}
      {...props}
    >
      <path d="M12 .5C5.65.5.5 5.65.5 12c0 5.08 3.29 9.39 7.86 10.91.58.1.79-.25.79-.56 0-.28-.01-1.02-.02-2-3.2.69-3.87-1.54-3.87-1.54-.52-1.32-1.27-1.67-1.27-1.67-1.04-.71.08-.7.08-.7 1.15.08 1.76 1.18 1.76 1.18 1.02 1.75 2.68 1.25 3.34.95.1-.74.4-1.25.72-1.54-2.55-.29-5.24-1.28-5.24-5.7 0-1.26.45-2.29 1.18-3.1-.12-.29-.51-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11.04 11.04 0 0 1 5.79 0c2.21-1.49 3.18-1.18 3.18-1.18.62 1.59.23 2.76.11 3.05.74.81 1.18 1.84 1.18 3.1 0 4.43-2.69 5.41-5.26 5.69.41.36.78 1.06.78 2.13 0 1.54-.01 2.78-.01 3.16 0 .31.21.67.8.56C20.21 21.39 23.5 17.08 23.5 12 23.5 5.65 18.35.5 12 .5z" />
    </svg>
  );
});

GithubIcon.displayName = "GithubIcon";

export default GithubIcon;
