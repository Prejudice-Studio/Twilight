"use client";

import type { ReactNode } from "react";

/**
 * AdminTable renders one dataset twice: a table on md+ viewports and a card
 * list below that. Admin pages used to hardcode a table with a fixed
 * `min-w-[980px]` / `min-w-[1160px]`, which forced horizontal dragging on every
 * phone. Cards keep the same columns but stack them, so narrow screens read
 * without scrolling sideways.
 */
export interface AdminTableColumn<T> {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  /** Field label inside the card view; falls back to `header`. */
  cardLabel?: ReactNode;
  /** Secondary fields can be dropped from the card to keep it readable. */
  hideOnCard?: boolean;
  /** Mark the leading column so it can act as the card heading. */
  primary?: boolean;
  className?: string;
  headerClassName?: string;
}

interface AdminTableProps<T> {
  columns: AdminTableColumn<T>[];
  rows: T[];
  rowKey: (row: T) => string | number;
  empty?: ReactNode;
  onRowClick?: (row: T) => void;
  /** Right-click opens the row action menu on desktop; cards tap through to `onRowClick`. */
  onRowContextMenu?: (row: T, event: React.MouseEvent) => void;
  rowClassName?: (row: T) => string;
  /** Extra classes for the scroll container, e.g. a max height. */
  containerClassName?: string;
}

export function AdminTable<T>({
  columns,
  rows,
  rowKey,
  empty,
  onRowClick,
  onRowContextMenu,
  rowClassName,
  containerClassName,
}: AdminTableProps<T>) {
  if (rows.length === 0) {
    return <>{empty}</>;
  }

  const ordered = [...columns].sort((a, b) => Number(b.primary ?? false) - Number(a.primary ?? false));
  // Only advertise a click affordance when a click handler exists; a row that
  // merely supports right-click must not look clickable.
  const rowState = `${onRowClick ? "cursor-pointer " : ""}${onRowContextMenu ? "hover:bg-muted/40" : ""}`;

  return (
    <>
      <div className={`custom-scrollbar hidden overflow-auto overscroll-contain md:block ${containerClassName ?? ""}`}>
        <table className="w-full border-collapse text-sm">
          <thead>
            <tr className="border-b border-border/60 text-left text-xs uppercase tracking-wide text-muted-foreground">
              {ordered.map((column) => (
                <th key={column.key} className={`px-3 py-2 font-medium ${column.headerClassName ?? ""}`}>
                  {column.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr
                key={rowKey(row)}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
                onContextMenu={onRowContextMenu ? (event) => onRowContextMenu(row, event) : undefined}
                className={`border-b border-border/40 last:border-0 ${rowState} ${rowClassName ? rowClassName(row) : ""}`}
              >
                {ordered.map((column) => (
                  <td key={column.key} className={`px-3 py-2 align-middle ${column.className ?? ""}`}>
                    {column.cell(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <ul className="space-y-2 md:hidden">
        {rows.map((row) => (
          <li
            key={rowKey(row)}
            onClick={onRowClick ? () => onRowClick(row) : undefined}
            className={`rounded-lg border border-border/60 bg-card/60 p-3 text-sm ${rowClassName ? rowClassName(row) : ""}`}
          >
            {ordered
              .filter((column) => !column.hideOnCard)
              .map((column, index) =>
                index === 0 ? (
                  <div key={column.key} className="font-medium">
                    {column.cell(row)}
                  </div>
                ) : (
                  <div key={column.key} className="mt-1.5 flex items-start justify-between gap-3 text-muted-foreground">
                    <span className="shrink-0 text-xs">{column.cardLabel ?? column.header}</span>
                    <span className="min-w-0 break-words text-right">{column.cell(row)}</span>
                  </div>
                ),
              )}
          </li>
        ))}
      </ul>
    </>
  );
}
