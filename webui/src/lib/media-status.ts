export type MediaRequestStatusMode = "user" | "admin";

const ADMIN_STATUS_MAP: Record<string, string> = {
  pending: "pending",
  unhandled: "pending",
  accepted: "accepted",
  rejected: "rejected",
  completed: "completed",
  downloading: "downloading",
};

const USER_STATUS_MAP: Record<string, string> = {
  pending: "UNHANDLED",
  unhandled: "UNHANDLED",
  accepted: "ACCEPTED",
  rejected: "REJECTED",
  completed: "COMPLETED",
  downloading: "DOWNLOADING",
};

export function normalizeMediaRequestStatus(status?: string | null, mode: MediaRequestStatusMode = "user"): string {
  const raw = (status || "").trim().toLowerCase();
  if (mode === "admin") {
    return ADMIN_STATUS_MAP[raw] || "pending";
  }
  return USER_STATUS_MAP[raw] || (status || "UNHANDLED").toUpperCase();
}
