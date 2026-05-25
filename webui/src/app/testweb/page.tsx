import { ensureTestPageEnabled } from "@/lib/testpage-gate";
import { TestWebLanding } from "./test-web-landing";

// 生产环境默认屏蔽 demo 入口；通过 NEXT_PUBLIC_ENABLE_TESTPAGES=1 显式开启。
export default function TestWebPage() {
  ensureTestPageEnabled();
  return <TestWebLanding />;
}
