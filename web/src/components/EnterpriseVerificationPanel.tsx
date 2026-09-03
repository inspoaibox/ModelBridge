import React from "react";
import { Building2, CheckCircle2, Clock3, FileUp, RefreshCw, XCircle } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { EnterpriseVerification, Language, LoginMessage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface Props {
  language: Language;
  item: EnterpriseVerification | null;
  busy: boolean;
  message: LoginMessage;
  refresh: () => Promise<void>;
  submit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function EnterpriseVerificationPanel({ language, item, busy, message, refresh, submit }: Props) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const status = item?.status || "not_submitted";
  const canSubmit = status === "not_submitted" || status === "rejected";
  const statusIcon = status === "approved" ? <CheckCircle2 className="h-5 w-5 text-emerald-600" /> : status === "rejected" ? <XCircle className="h-5 w-5 text-rose-600" /> : status === "pending" ? <Clock3 className="h-5 w-5 text-amber-600" /> : <Building2 className="h-5 w-5 text-indigo-600" />;
  const statusLabel = status === "approved" ? t("enterpriseStatusApproved") : status === "rejected" ? t("enterpriseStatusRejected") : status === "pending" ? t("enterpriseStatusPending") : t("enterpriseStatusNotSubmitted");

  return <div className="space-y-6">
    <Card className="border-indigo-500/20 shadow-sm dark:bg-slate-900/60">
      <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div><CardTitle className="flex items-center gap-2 text-lg"><Building2 className="h-5 w-5 text-indigo-600" />{t("enterpriseTitle")}</CardTitle><CardDescription>{t("enterpriseDescription")}</CardDescription></div>
        <Button type="button" variant="outline" size="sm" onClick={() => void refresh()} disabled={busy} className="gap-2"><RefreshCw className={busy ? "h-4 w-4 animate-spin" : "h-4 w-4"} />{t("consoleRefresh")}</Button>
      </CardHeader>
      <CardContent className="space-y-5">
        {message.text ? <div className={message.kind === "error" ? "rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "rounded-xl border border-indigo-500/20 bg-indigo-50 p-3 text-sm text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-200"}>{message.text}</div> : null}
        <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/40"><span>{statusIcon}</span><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{statusLabel}</div><div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{status === "rejected" ? item?.rejection_reason || t("enterpriseRejectedNoReason") : t("enterpriseStatusHint")}</div></div>{item?.bank_account_masked ? <Badge className="ml-auto" variant="muted">{item.bank_account_masked}</Badge> : null}</div>
        {item && status !== "not_submitted" && status !== "rejected" ? <div className="grid gap-4 sm:grid-cols-2"><Info label={t("enterpriseName")} value={item.enterprise_name} /><Info label={t("enterpriseCreditCode")} value={item.unified_credit_code} /><Info label={t("enterpriseBankAccountName")} value={item.bank_account_name} /><Info label={t("enterpriseBankName")} value={item.bank_name} /></div> : null}
        {canSubmit ? <form className="space-y-5 rounded-xl border border-indigo-500/20 bg-indigo-50/40 p-4 dark:bg-indigo-500/5" onSubmit={(event) => void submit(event)}>
          <div className="grid gap-4 sm:grid-cols-2"><Field id="enterprise-name" label={t("enterpriseName")} placeholder={t("enterpriseNamePlaceholder")} name="enterprise_name" required /><Field id="enterprise-credit-code" label={t("enterpriseCreditCode")} placeholder={t("enterpriseCreditCodePlaceholder")} name="unified_credit_code" required maxLength={18} /></div>
          <div className="space-y-2"><Label htmlFor="enterprise-license">{t("enterpriseLicense")}</Label><Input id="enterprise-license" name="license" type="file" accept=".jpg,.jpeg,.png,.pdf,image/jpeg,image/png,application/pdf" required /><p className="text-xs text-slate-500 dark:text-slate-400">{t("enterpriseLicenseHint")}</p></div>
          <div className="grid gap-4 sm:grid-cols-2"><Field id="enterprise-account-name" label={t("enterpriseBankAccountName")} placeholder={t("enterpriseBankAccountNamePlaceholder")} name="bank_account_name" required /><Field id="enterprise-bank-name" label={t("enterpriseBankName")} placeholder={t("enterpriseBankNamePlaceholder")} name="bank_name" required /></div>
          <Field id="enterprise-bank-account" label={t("enterpriseBankAccount")} placeholder={t("enterpriseBankAccountPlaceholder")} name="bank_account" inputMode="numeric" required />
          <div className="flex justify-end"><Button type="submit" disabled={busy} className="gap-2"><FileUp className="h-4 w-4" />{busy ? t("enterpriseSubmitting") : status === "rejected" ? t("enterpriseResubmit") : t("enterpriseSubmit")}</Button></div>
        </form> : null}
      </CardContent>
    </Card>
  </div>;
}

function Field({ id, label, placeholder, name, required = false, maxLength, inputMode }: { id: string; label: string; placeholder: string; name: string; required?: boolean; maxLength?: number; inputMode?: React.HTMLAttributes<HTMLInputElement>["inputMode"] }) { return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><Input id={id} name={name} placeholder={placeholder} required={required} maxLength={maxLength} inputMode={inputMode} /></div>; }
function Info({ label, value }: { label: string; value: string }) { return <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800"><div className="text-xs text-slate-500 dark:text-slate-400">{label}</div><div className="mt-1 text-sm font-semibold text-slate-900 dark:text-white">{value || "-"}</div></div>; }
