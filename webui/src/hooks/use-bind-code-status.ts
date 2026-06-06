"use client";

import { useEffect, useRef } from "react";
import { api } from "@/lib/api";

export type BindCodeStatusData = {
  code?: string;
  status?: string;
  error_code?: string;
  message?: string;
  confirmed?: boolean;
  expires_in?: number;
  invalid?: boolean;
  terminal?: boolean;
  telegram_bound?: boolean;
  telegram_id?: number;
  telegram_username?: string;
};

export type BindCodeScene = "user" | "register";

export interface UseBindCodeStatusOptions {
  /** 绑定码；为 null / 空串时不订阅。 */
  code: string | null | undefined;
  /** user = 个人设置 / 换绑，register = 注册。决定走哪组状态端点。默认 user。 */
  scene?: BindCodeScene;
  /** 绑定码初始有效期(秒)，用于设定整体看护上限(deadline)。 */
  expiresIn?: number;
  /** false 时暂停订阅（如已绑定）。默认 true。 */
  enabled?: boolean;
  /** 轮询间隔(ms)。默认 2500。 */
  pollIntervalMs?: number;
  /** 进入"已绑定 / 已确认"终态。 */
  onBound: (data: BindCodeStatusData) => void;
  /** 进入"无效 / 过期 / 被占用"等失败终态。 */
  onTerminalError: (data: BindCodeStatusData) => void;
  /** 整体看护超时：绑定码 TTL 到期仍未达终态时触发。 */
  onTimeout?: () => void;
}

function isBoundStatus(data: BindCodeStatusData): boolean {
  return (
    Boolean(data.telegram_bound) ||
    data.status === "bound" ||
    data.status === "confirmed" ||
    Boolean(data.confirmed && !data.invalid)
  );
}

/**
 * useBindCodeStatus 统一绑定码状态轮询（个人设置绑定 / 换绑 / 注册共用）。
 * 解决三类问题：
 *   1. 调用方不再"生成绑定码后让用户刷新页面"——挂载即自动轮询到终态；
 *   2. **超时中断**：整体 deadline = 绑定码 TTL + 宽限，到点强制停轮询并回调
 *      onTimeout，避免服务端长时间无响应时无限轮询；
 *   3. **请求中断**：每次轮询用独立 AbortController，组件卸载 / code 变更 /
 *      达终态立即 abort 在途请求，不泄漏连接、不在卸载后 setState。
 * 回调用 ref 固定，避免每次渲染重订阅。
 */
export function useBindCodeStatus(options: UseBindCodeStatusOptions): void {
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const {
    code,
    scene = "user",
    expiresIn,
    enabled = true,
    pollIntervalMs = 2500,
  } = options;

  useEffect(() => {
    const trimmed = (code || "").trim();
    if (!trimmed || !enabled) return;

    let stopped = false;
    let pollTimer: ReturnType<typeof setTimeout> | null = null;
    let deadlineTimer: ReturnType<typeof setTimeout> | null = null;
    let controller: AbortController | null = null;

    const fetchStatus = (signal: AbortSignal) =>
      scene === "register"
        ? api.getRegisterBindCodeStatus(trimmed, signal)
        : api.getBindCodeStatus(trimmed, signal);

    const stop = () => {
      if (stopped) return;
      stopped = true;
      if (pollTimer) clearTimeout(pollTimer);
      if (deadlineTimer) clearTimeout(deadlineTimer);
      pollTimer = null;
      deadlineTimer = null;
      controller?.abort();
      controller = null;
    };

    const handle = (data: BindCodeStatusData) => {
      if (stopped) return;
      if (isBoundStatus(data)) {
        stop();
        optionsRef.current.onBound(data);
        return;
      }
      // pending 不是终态；其余 terminal（过期 / 无效 / 被占用 / 加群未通过）算失败终态。
      if (data.terminal && data.status !== "pending") {
        stop();
        optionsRef.current.onTerminalError(data);
      }
    };

    const poll = async () => {
      if (stopped) return;
      controller = new AbortController();
      try {
        const res = await fetchStatus(controller.signal);
        if (!stopped && res.success && res.data) {
          handle(res.data as BindCodeStatusData);
        }
      } catch {
        // 网络抖动 / 单次超时：忽略，保持轮询直到 deadline。
      }
      if (stopped) return;
      pollTimer = setTimeout(() => {
        void poll();
      }, pollIntervalMs);
    };

    // 整体看护上限：绑定码 TTL + 5s 宽限。expiresIn 缺省按后端默认 300s。
    const ttlSeconds = typeof expiresIn === "number" && expiresIn > 0 ? expiresIn : 300;
    deadlineTimer = setTimeout(() => {
      if (stopped) return;
      stop();
      optionsRef.current.onTimeout?.();
    }, ttlSeconds * 1000 + 5000);

    void poll();

    return stop;
  }, [code, scene, expiresIn, enabled, pollIntervalMs]);
}
