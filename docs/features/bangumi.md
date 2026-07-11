# Bangumi 同步

Bangumi 功能包含播放同步、收藏管理、本地封面缓存和仪表盘最近动态。

## 功能开关

- `BangumiEnabled`：控制自动同步、Webhook、历史记录等能力。
- `BangumiManageEnabled`：控制收藏管理和用户管理模式字段。

两个开关都必须在后端 handler 中检查。

## 缓存模型

- `BangumiSubjectCache`：全局作品缓存，按 Bangumi subject ID 去重。
- `BangumiCollectionCache`：用户维度的收藏缓存，只保存用户态字段。
- 读取收藏时由全局作品缓存回填 subject。
- `UpsertBangumiCollectionCache` 负责拆分完整 API 数据，不要在 handler 中重复实现拆分逻辑。

## 封面缓存

封面下载到 `uploads/bangumi/{BGMID}.{ext}`。BGMID 必须是正整数，下载时必须校验 host、scheme、大小、MIME 和写入路径。

## “看过”状态

收藏类型 `2` 表示看过时，后端应自动标记全系列已看并忽略前端传入的 `ep_status`。部分进度应使用“在看”类型。

## 调度

`refresh_bangumi_collections` 调度任务用于定期刷新已开启管理且配置 Token 的用户收藏缓存。
