import { ensureTestPageEnabled } from "@/lib/testpage-gate";
import { TestWebDemo } from "../testweb/demo-shell";

export default function TestWebUserPage() {
  ensureTestPageEnabled();
  return <TestWebDemo role="user" />;
}
