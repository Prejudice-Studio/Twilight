import { ensureTestPageEnabled } from "@/lib/testpage-gate";
import { TestWebDemo } from "../testweb/demo-shell";

export default function TestWebAdminPage() {
  ensureTestPageEnabled();
  return <TestWebDemo role="admin" />;
}
