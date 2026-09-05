import { Bell, Check, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Announcement, Language } from "@/types";
import { MarkdownContent } from "@/components/MarkdownContent";

export function AnnouncementReaderModal({ language, announcement, busy, onClose, onMarkRead }: { language: Language; announcement: Announcement | null; busy: boolean; onClose: () => void; onMarkRead: () => void }) {
  if (!announcement) return null;
  const zh = language === "zh";
  return <div className="fixed inset-0 z-[110] flex items-center justify-center bg-slate-950/65 p-4 backdrop-blur-sm"><Card role="dialog" aria-modal="true" aria-labelledby="announcement-reader-title" className="w-full max-w-2xl border-indigo-500/20 bg-white shadow-2xl dark:border-indigo-400/20 dark:bg-slate-900"><CardHeader className="flex flex-row items-start justify-between gap-4 border-b border-slate-200 dark:border-slate-800"><div><div className="mb-2 flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-300"><Bell className="h-4 w-4" />{zh ? "最新公告" : "Latest announcement"}</div><CardTitle id="announcement-reader-title" className="text-xl">{announcement.title}</CardTitle><CardDescription className="mt-2">{new Date(announcement.effective_at).toLocaleString(zh ? "zh-CN" : "en-US")}</CardDescription></div><Button type="button" variant="ghost" size="icon" onClick={onClose} aria-label={zh ? "关闭公告" : "Close announcement"}><X className="h-4 w-4" /></Button></CardHeader><CardContent><MarkdownContent content={announcement.content} className="max-h-[55vh] overflow-auto text-sm text-slate-700 dark:text-slate-300" /><div className="mt-5 flex justify-end border-t border-slate-200 pt-4 dark:border-slate-800"><Button type="button" onClick={onMarkRead} disabled={busy} className="gap-2"><Check className="h-4 w-4" />{zh ? "我已阅读" : "Mark as read"}</Button></div></CardContent></Card></div>;
}
