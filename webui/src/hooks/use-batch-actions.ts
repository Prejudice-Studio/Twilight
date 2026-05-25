"use client";

import { useCallback, useMemo, useState } from "react";

/**
 * useBatchActions 收敛 admin 列表页"批量选择 → 批量操作"这一组重复模板。
 *
 * 在 admin/users 之前，每个有批量功能的页面都要重复以下五段：
 *   const [selected, setSelected] = useState<Set<ID>>(new Set());
 *   const selectedItems = useMemo(() => items.filter(i => selected.has(getId(i))), [items, selected]);
 *   const allPageSelected = items.length > 0 && items.every(i => selected.has(getId(i)));
 *   const toggle = (id) => setSelected(prev => toggleSetMember(prev, id));
 *   const toggleAll = () => setSelected(prev => allPageSelected ? new Set([...prev]存外没选的) : new Set([...prev, ...本页全选]));
 *
 * 这个 hook 把上面五段统一成一份实现：调用方只给 items 和 getId，剩下都自动算。
 *
 *
 *
 * 设计选择：
 *   - 用 Set 而非 Array：toggle/has 都是 O(1)，列表 1k+ 行性能不会塌。
 *   - 跨页保留：toggleAll 只操作"当前 items"，被翻页过滤掉的 ID 不会被移除，
 *     避免分页切换时丢失已选项。需要一次性收回时调 retainVisible(items)。
 *   - getId 必传：列表元素的 id 字段名因业务而异（uid / id / code / ...），
 *     不假设。返回 number | string，因为 admin 既有 UID 也有字符串 code。
 */
export interface BatchActions<T, ID extends number | string> {
  /** 当前选中的 ID 集合（不可变 Set，外部按需 Array.from） */
  selectedIds: Set<ID>;
  /** 选中数量（等价 selectedIds.size） */
  selectedCount: number;
  /** 选中的元素列表（filter 自 items） */
  selectedItems: T[];
  /** 选中 IDs 的 Array 形式（提交批量 API 时常用） */
  selectedIdArray: ID[];
  /** 当前 items 是否全部已选（用于 header checkbox 半选/全选） */
  allPageSelected: boolean;
  /** 当前 items 是否部分已选（partial select 状态） */
  somePageSelected: boolean;
  /** 切换单个 ID 选中态 */
  toggle: (id: ID) => void;
  /** 切换"当前页全选"：已全选 → 反选当前页；否则把当前页 ID 全部加入选中 */
  toggleAllOnPage: () => void;
  /** 仅保留"当前 items 中存在的"已选 ID（翻页/搜索后清理已不可见的旧选项） */
  retainVisible: () => void;
  /** 直接设置选中集合（高级用法，例如外部反向同步） */
  setSelectedIds: (next: Set<ID>) => void;
  /** 清空所有选中 */
  clear: () => void;
  /** 当前 ID 是否已选（封装 selectedIds.has，避免外部反复 .has） */
  isSelected: (id: ID) => boolean;
}

export function useBatchActions<T, ID extends number | string = number>(
  items: T[],
  getId: (item: T) => ID,
): BatchActions<T, ID> {
  const [selectedIds, setSelectedIdsState] = useState<Set<ID>>(() => new Set());

  const selectedItems = useMemo(
    () => items.filter((item) => selectedIds.has(getId(item))),
    [items, selectedIds, getId],
  );
  const selectedIdArray = useMemo(() => Array.from(selectedIds), [selectedIds]);

  const allPageSelected = useMemo(
    () => items.length > 0 && items.every((item) => selectedIds.has(getId(item))),
    [items, selectedIds, getId],
  );
  const somePageSelected = useMemo(
    () => !allPageSelected && items.some((item) => selectedIds.has(getId(item))),
    [items, selectedIds, getId, allPageSelected],
  );

  const toggle = useCallback((id: ID) => {
    setSelectedIdsState((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const toggleAllOnPage = useCallback(() => {
    setSelectedIdsState((prev) => {
      const next = new Set(prev);
      const pageIds = items.map(getId);
      const allOn = items.length > 0 && pageIds.every((id) => next.has(id));
      if (allOn) {
        for (const id of pageIds) next.delete(id);
      } else {
        for (const id of pageIds) next.add(id);
      }
      return next;
    });
  }, [items, getId]);

  const retainVisible = useCallback(() => {
    setSelectedIdsState((prev) => {
      const visible = new Set(items.map(getId));
      const filtered = new Set(Array.from(prev).filter((id) => visible.has(id)));
      // 复用同一个 Set 引用避免不必要 re-render
      return filtered.size === prev.size ? prev : filtered;
    });
  }, [items, getId]);

  const clear = useCallback(() => {
    setSelectedIdsState(new Set());
  }, []);

  const isSelected = useCallback((id: ID) => selectedIds.has(id), [selectedIds]);

  const setSelectedIds = useCallback((next: Set<ID>) => {
    setSelectedIdsState(next);
  }, []);

  return {
    selectedIds,
    selectedCount: selectedIds.size,
    selectedItems,
    selectedIdArray,
    allPageSelected,
    somePageSelected,
    toggle,
    toggleAllOnPage,
    retainVisible,
    setSelectedIds,
    clear,
    isSelected,
  };
}
