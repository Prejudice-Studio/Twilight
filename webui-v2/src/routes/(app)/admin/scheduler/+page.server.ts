import { fail, redirect } from "@sveltejs/kit";
import type { Actions, RequestEvent } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";
import { apiJSON, apiJSONWithResponse } from "$lib/server/api";
import type { SchedulerJobItem, SchedulerJobRun, SchedulerRunDetail, SchedulerTriggerSpec } from "$lib/types";

const defaultPageSize = 20;
const validViews = new Set(["all", "running", "failed", "custom", "manual"]);

type SchedulerAction = { notice?: string; error?: string };

function text(form: FormData, name: string): string {
  return String(form.get(name) ?? "").trim();
}

function number(form: FormData, name: string, fallback: number): number {
  const value = Number(text(form, name));
  return Number.isFinite(value) ? Math.trunc(value) : fallback;
}

function checked(form: FormData, name: string, fallback = false): boolean {
  const values = form.getAll(name);
  return values.length ? values.some((value) => String(value) === "true") : fallback;
}

function csv(value: string): string[] {
  return value.split(/[\n,]+/).map((item) => item.trim()).filter(Boolean).slice(0, 100);
}

function runtimeParams(jobID: string, form: FormData, forManualRun: boolean): Record<string, unknown> {
  const params: Record<string, unknown> = {};
  switch (jobID) {
    case "cleanup_no_emby":
      params.days = Math.max(1, Math.min(3650, number(form, "days", 7)));
      params.preserve_tg_bound = checked(form, "preserve_tg_bound", true);
      params.ignore_enabled_flag = checked(form, "ignore_enabled_flag", forManualRun);
      break;
    case "cleanup_pending_emby_entitlements":
      params.ignore_enabled_flag = checked(form, "ignore_enabled_flag", forManualRun);
      if (forManualRun) params.enabled = params.ignore_enabled_flag;
      break;
    case "cleanup_audit_logs":
      params.enabled = checked(form, "ignore_enabled_flag", forManualRun) || checked(form, "enabled", true);
      params.retention_days = Math.max(0, Math.min(3650, number(form, "retention_days", 30)));
      params.max_entries = Math.max(0, Math.min(100000, number(form, "max_entries", 0)));
      params.preserve_admin = checked(form, "preserve_admin", true);
      break;
    case "cleanup_ticket_images":
      params.retention_days = Math.max(0, Math.min(3650, number(form, "retention_days", 30)));
      break;
    case "sync_emby_activity_logs":
      params.since_hours = Math.max(1, Math.min(720, number(form, "since_hours", 24)));
      break;
    case "cleanup_unlinked_emby":
      params.dry_run = checked(form, "dry_run", true);
      params.delete = checked(form, "allow_delete", false);
      break;
    case "cleanup_emby_devices":
      params.dry_run = checked(form, "dry_run", true);
      params.max_workers = Math.max(1, Math.min(10, number(form, "max_workers", 10)));
      params.skip_usernames = csv(text(form, "skip_usernames"));
      break;
    case "kick_unknown_group_members":
      params.dry_run = checked(form, "dry_run", true);
      params.max_per_run = Math.max(1, Math.min(500, number(form, "max_per_run", 200)));
      break;
    case "enforce_group_membership":
      params.auto_enable_rejoined = checked(form, "auto_enable_rejoined", false);
      break;
    case "emby_sync":
      params.max_users = Math.max(1, Math.min(50000, number(form, "max_users", 1000)));
      break;
  }
  return params;
}

function schedulePayload(form: FormData): { type: string; hour?: number; minute?: number; seconds?: number; runtime_params: Record<string, unknown> } {
  const type = text(form, "schedule_type");
  const runtimeParamsValue = runtimeParams(text(form, "job_id"), form, false);
  if (type === "manual") return { type: "manual", runtime_params: runtimeParamsValue };
  if (type === "cron_daily") {
    return {
      type,
      hour: Math.max(0, Math.min(23, number(form, "hour", 0))),
      minute: Math.max(0, Math.min(59, number(form, "minute", 0))),
      runtime_params: runtimeParamsValue
    };
  }
  const unit = text(form, "interval_unit") === "hours" ? 3600 : 60;
  const interval = Math.max(1, Math.min(168, number(form, "interval_value", 1)));
  return { type: "interval", seconds: Math.max(60, Math.min(604800, interval * unit)), runtime_params: runtimeParamsValue };
}

async function post<T>(event: RequestEvent, path: string, body: Record<string, unknown>) {
  return apiJSONWithResponse<T>(event, path, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(body)
  });
}

function redirectAfterAction(event: RequestEvent, notice: string): never {
  throw redirect(303, `/admin/scheduler?notice=${encodeURIComponent(notice)}`);
}

export const load: PageServerLoad = async (event) => {
  const notice = event.url.searchParams.get("notice") || "";
  const requestedView = event.url.searchParams.get("view") || "all";
  const view = validViews.has(requestedView) ? requestedView : "all";
  const response = await apiJSON<{ jobs: SchedulerJobItem[] }>(event, "/api/v2/admin/scheduler/jobs", { cache: "no-store" });
  const allJobs = response?.success ? response.data?.jobs || [] : [];
  const jobs = allJobs.filter((job) => {
    if (view === "running") return job.is_running;
    if (view === "failed") return job.last_run?.status === "failed";
    if (view === "custom") return job.is_custom;
    if (view === "manual") return Boolean(job.manual_only || job.trigger_spec?.type === "manual");
    return true;
  });

  const logsID = event.url.searchParams.get("logs") || "";
  const selectedJob = allJobs.find((job) => job.id === logsID) || null;
  let logs: SchedulerRunDetail | null = null;
  if (selectedJob) {
    const results = await Promise.allSettled([
      apiJSON<{ job_id: string; last_run: SchedulerJobRun | null }>(event, `/api/v2/admin/scheduler/jobs/${encodeURIComponent(logsID)}/last-run`, { cache: "no-store" }),
      apiJSON<{ job_id: string; history: SchedulerJobRun[] }>(event, `/api/v2/admin/scheduler/jobs/${encodeURIComponent(logsID)}/history?limit=20`, { cache: "no-store" })
    ]);
    const last = results[0].status === "fulfilled" ? results[0].value : null;
    const history = results[1].status === "fulfilled" ? results[1].value : null;
    if (last?.success || history?.success) {
      logs = {
        job_id: logsID,
        last_run: last?.success ? last.data?.last_run || null : null,
        history: history?.success ? history.data?.history || [] : []
      };
    }
  }

  return {
    jobs,
    selected_job_id: logsID,
    selected_job: selectedJob,
    logs,
    view,
    notice,
    refreshed_at: Math.floor(Date.now() / 1000),
    load_error: response?.success ? null : "调度器信息暂时无法读取，请重新刷新。"
  };
};

export const actions: Actions = {
  run: async (event) => {
    const form = await event.request.formData();
    const jobID = text(form, "job_id");
    if (!jobID) return fail(400, { error: "任务编号无效" } satisfies SchedulerAction);
    const result = await post<{ job_id: string }>(event, `/api/v2/admin/scheduler/jobs/${encodeURIComponent(jobID)}/run`, {
      runtime_params: runtimeParams(jobID, form, true)
    });
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 400, { error: "任务启动失败，请稍后重试" } satisfies SchedulerAction);
    redirectAfterAction(event, "run");
  },

  terminate: async (event) => {
    const form = await event.request.formData();
    const jobID = text(form, "job_id");
    if (!jobID) return fail(400, { error: "任务编号无效" } satisfies SchedulerAction);
    const result = await post<{ job_id: string }>(event, `/api/v2/admin/scheduler/jobs/${encodeURIComponent(jobID)}/terminate`, {});
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 400, { error: "终止任务失败，请稍后重试" } satisfies SchedulerAction);
    redirectAfterAction(event, "terminate");
  },

  schedule: async (event) => {
    const form = await event.request.formData();
    const jobID = text(form, "job_id");
    if (!jobID) return fail(400, { error: "任务编号无效" } satisfies SchedulerAction);
    const result = await apiJSONWithResponse<{ job_id: string; trigger_spec: SchedulerTriggerSpec }>(event, `/api/v2/admin/scheduler/jobs/${encodeURIComponent(jobID)}/schedule`, {
      method: "PUT",
      headers: { "content-type": "application/json" },
      body: JSON.stringify(schedulePayload(form))
    });
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 400, { error: "任务计划保存失败，请稍后重试" } satisfies SchedulerAction);
    redirectAfterAction(event, "schedule");
  },

  resetSchedule: async (event) => {
    const form = await event.request.formData();
    const jobID = text(form, "job_id");
    if (!jobID) return fail(400, { error: "任务编号无效" } satisfies SchedulerAction);
    const result = await apiJSONWithResponse(event, `/api/v2/admin/scheduler/jobs/${encodeURIComponent(jobID)}/schedule`, { method: "DELETE" });
    if (!result?.response.ok || !result.envelope?.success) return fail(result?.response.status || 400, { error: "任务计划恢复失败，请稍后重试" } satisfies SchedulerAction);
    redirectAfterAction(event, "reset");
  }
};
