"use client";

import { useRef, useState } from "react";
import { ImagePlus, Loader2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogOverlay } from "@/components/ui/dialog";
import { useToast } from "@/hooks/use-toast";
import { api, type TicketAttachment } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { friendlyError } from "@/lib/validators";

const ALLOWED_TYPES = ["image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp"];

interface TicketImagesProps {
  ticketId: number;
  attachments: TicketAttachment[];
  /** 是否允许上传（默认 true）。工单关闭后传 false 仅展示。 */
  editable?: boolean;
  /** 是否允许删除图片。不传时跟随 editable；工单关闭后管理员仍可删除，故管理端单独传 true。 */
  canDelete?: boolean;
  /** 单张最大字节数（来自系统信息 limits.ticket_image_max_size）。 */
  maxSize: number;
  /** 最大图片数量（来自系统信息 limits.ticket_image_max_count）。 */
  maxCount: number;
  /** 上传/删除成功后回调，用于刷新最新附件列表。 */
  onChange?: (attachments: TicketAttachment[]) => void;
}

export function TicketImages({
  ticketId,
  attachments,
  editable = true,
  canDelete,
  maxSize,
  maxCount,
  onChange,
}: TicketImagesProps) {
  const { t } = useI18n();
  const { toast } = useToast();
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [previewSrc, setPreviewSrc] = useState<string | null>(null);

  const sizeMB = Math.round((maxSize / (1024 * 1024)) * 10) / 10;
  const list = Array.isArray(attachments) ? attachments : [];
  const canAdd = editable && list.length < maxCount;
  const allowDelete = canDelete ?? editable;

  const handlePick = () => inputRef.current?.click();

  const handleFile = async (file: File | undefined) => {
    if (!file) return;
    if (list.length >= maxCount) {
      toast({ title: t("tickets.imageTooMany", { count: maxCount }), variant: "destructive" });
      return;
    }
    if (!ALLOWED_TYPES.includes(file.type)) {
      toast({ title: t("tickets.imageInvalidType"), variant: "destructive" });
      return;
    }
    if (file.size > maxSize) {
      toast({ title: t("tickets.imageTooLarge", { size: sizeMB }), variant: "destructive" });
      return;
    }
    setUploading(true);
    try {
      const res = await api.uploadTicketImage(ticketId, file);
      if (res.success && res.data) {
        toast({ title: t("tickets.imageUploaded") });
        onChange?.(res.data.attachments);
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  };

  const handleDelete = async (filename: string) => {
    setDeleting(filename);
    try {
      const res = await api.deleteTicketImage(ticketId, filename);
      if (res.success && res.data) {
        toast({ title: t("tickets.imageDeleted") });
        onChange?.(res.data.attachments);
      } else {
        toast({ title: friendlyError(res.error_code, res.message), variant: "destructive" });
      }
    } catch (err: any) {
      toast({ title: friendlyError(err?.errorCode, err?.message), variant: "destructive" });
    } finally {
      setDeleting(null);
    }
  };

  if (!editable && !allowDelete && list.length === 0) return null;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <span className="text-xs font-medium text-muted-foreground">{t("tickets.images")}</span>
        {editable && (
          <span className="text-[10px] text-muted-foreground">
            {t("tickets.imagesHint", { count: maxCount, size: sizeMB })}
          </span>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        {list.map((att) => {
          const src = api.ticketImageSrc(att.url);
          if (!src) return null;
          return (
            <div key={att.filename} className="relative group h-20 w-20 rounded-lg overflow-hidden border border-border/60 bg-muted/30 cursor-pointer" onClick={() => setPreviewSrc(src)}>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={src} alt={att.filename} className="h-full w-full object-cover" loading="lazy" />
              {allowDelete && (
                <button
                  type="button"
                  onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    void handleDelete(att.filename);
                  }}
                  disabled={deleting === att.filename}
                  title={t("tickets.deleteImage")}
                  className="absolute top-0.5 right-0.5 flex h-5 w-5 items-center justify-center rounded-full bg-black/60 text-white opacity-0 group-hover:opacity-100 transition-opacity hover:bg-destructive disabled:opacity-100"
                >
                  {deleting === att.filename ? <Loader2 className="h-3 w-3 animate-spin" /> : <X className="h-3 w-3" />}
                </button>
              )}
            </div>
          );
        })}
        {canAdd && (
          <button
            type="button"
            onClick={handlePick}
            disabled={uploading}
            className="flex h-20 w-20 flex-col items-center justify-center gap-1 rounded-lg border border-dashed border-border text-muted-foreground hover:border-primary hover:text-primary transition-colors disabled:opacity-60"
          >
            {uploading ? (
              <>
                <Loader2 className="h-5 w-5 animate-spin" />
                <span className="text-[10px]">{t("tickets.uploading")}</span>
              </>
            ) : (
              <>
                <ImagePlus className="h-5 w-5" />
                <span className="text-[10px]">{t("tickets.addImage")}</span>
              </>
            )}
          </button>
        )}
      </div>
      <input
        ref={inputRef}
        type="file"
        accept={ALLOWED_TYPES.join(",")}
        className="hidden"
        onChange={(e) => void handleFile(e.target.files?.[0])}
      />

      {/* 大图预览 */}
      <Dialog open={!!previewSrc} onOpenChange={(open) => { if (!open) setPreviewSrc(null); }}>
        <DialogOverlay className="bg-black/70" />
        <DialogContent className="max-w-[90vw] max-h-[90vh] p-0 bg-transparent border-0 shadow-none">
          <button
            type="button"
            onClick={() => setPreviewSrc(null)}
            className="absolute -top-10 right-0 z-50 flex h-8 w-8 items-center justify-center rounded-full bg-black/50 text-white hover:bg-black/70 transition-colors"
          >
            <X className="h-5 w-5" />
          </button>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          {previewSrc && <img src={previewSrc} alt="" className="max-h-[85vh] w-auto mx-auto rounded-lg object-contain" />}
        </DialogContent>
      </Dialog>
    </div>
  );
}
