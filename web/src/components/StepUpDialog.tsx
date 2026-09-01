import React from "react";
import { KeyRound, ShieldCheck, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface StepUpDialogProps {
  language: Language;
  open: boolean;
  code: string;
  error: string;
  busy: boolean;
  setCode: (value: string) => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => void;
  onCancel: () => void;
}

export function StepUpDialog({ language, open, code, error, busy, setCode, onSubmit, onCancel }: StepUpDialogProps) {
  if (!open) return null;
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-slate-950/60 p-4 backdrop-blur-sm">
      <Card className="w-full max-w-md border-indigo-500/20 bg-white shadow-2xl dark:border-indigo-400/20 dark:bg-slate-900">
        <CardHeader className="flex flex-row items-start justify-between gap-4 border-b border-slate-200/80 dark:border-slate-800">
          <div>
            <CardTitle className="flex items-center gap-2 text-lg text-slate-900 dark:text-white"><ShieldCheck className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />{t("stepUpTitle")}</CardTitle>
            <CardDescription className="mt-1">{t("stepUpDescription")}</CardDescription>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={onCancel} disabled={busy} aria-label={t("close")}><X className="h-4 w-4" /></Button>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={onSubmit}>
            <div className="space-y-2"><Label htmlFor="admin-step-up-code">{t("stepUpCode")}</Label><Input id="admin-step-up-code" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))} placeholder={t("stepUpPlaceholder")} disabled={busy} autoFocus required /></div>
            {error ? <p className="rounded-lg border border-rose-500/30 bg-rose-50 p-3 text-xs text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{error}</p> : null}
            <div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={onCancel} disabled={busy}>{t("stepUpCancel")}</Button><Button type="submit" disabled={busy || code.length !== 6} className="gap-2"><KeyRound className="h-4 w-4" />{t("stepUpConfirm")}</Button></div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
