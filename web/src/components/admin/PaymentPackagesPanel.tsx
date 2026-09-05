import React, { useEffect, useState } from "react";
import { CalendarClock, CreditCard, Plus, Save, Trash2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language, LoginMessage, PaymentRechargePackage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface Props {
  language: Language;
  packages: PaymentRechargePackage[];
  busy: boolean;
  message: LoginMessage;
  save: (packages: PaymentRechargePackage[]) => Promise<void>;
  canUpdate: boolean;
}

const emptyPackage = (): PaymentRechargePackage => ({
  id: crypto.randomUUID(),
  name: "",
  description: "",
  kind: "recharge",
  currency: "USD",
  amount: "",
  credited_amount: "",
  bonus_amount: "0",
  validity_days: 0,
  starts_at: "",
  ends_at: "",
  subscription_plan_code: "",
  enabled: true,
});

export function PaymentPackagesPanel({ language, packages, busy, message, save, canUpdate }: Props) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [draft, setDraft] = useState<PaymentRechargePackage[]>(packages);

  useEffect(() => setDraft(packages), [packages]);

  function update(id: string, patch: Partial<PaymentRechargePackage>) {
    setDraft((current) => current.map((item) => item.id === id ? { ...item, ...patch } : item));
  }

  function addPackage() {
    setDraft((current) => [...current, emptyPackage()]);
  }

  function removePackage(id: string) {
    setDraft((current) => current.filter((item) => item.id !== id));
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-4 rounded-2xl border border-emerald-500/20 bg-emerald-500/5 p-5 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-emerald-700 dark:text-emerald-300"><CreditCard className="h-4 w-4" />{t("paymentRechargePackagesEyebrow")}</div>
          <h2 className="mt-2 text-2xl font-extrabold text-slate-950 dark:text-white">{t("paymentRechargePackagesTitle")}</h2>
          <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("paymentRechargePackagesDescription")}</p>
        </div>
        <Button type="button" variant="outline" onClick={addPackage} disabled={busy || !canUpdate} className="gap-2"><Plus className="h-4 w-4" />{t("paymentRechargePackageAdd")}</Button>
      </div>
      {message.text ? <div className={`rounded-xl border p-3 text-sm ${message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300"}`}>{message.text}</div> : null}
      <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
        <CardHeader><CardTitle>{t("paymentRechargePackageListTitle")}</CardTitle><CardDescription>{t("paymentRechargePackageListDescription")}</CardDescription></CardHeader>
        <CardContent className="space-y-4">
          {draft.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 py-14 text-center text-sm text-slate-500 dark:border-slate-700">{t("paymentRechargePackageEmpty")}</div> : draft.map((item, index) => (
            <div key={item.id} className="rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-950/40">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3"><div className="flex items-center gap-2"><Badge variant={item.kind === "subscription" ? "cyan" : "success"}>{item.kind === "subscription" ? t("paymentRechargePackageSubscription") : t("paymentRechargePackageRecharge")}</Badge><span className="text-sm font-semibold text-slate-900 dark:text-white">{item.name || `${t("paymentRechargePackageUntitled")} ${index + 1}`}</span></div><Button type="button" variant="ghost" size="icon" onClick={() => removePackage(item.id)} disabled={busy || !canUpdate} title={t("paymentRechargePackageRemove")} aria-label={t("paymentRechargePackageRemove")} className="text-rose-600"><Trash2 className="h-4 w-4" /></Button></div>
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <Field label={t("paymentRechargePackageName")}><Input value={item.name} onChange={(event) => update(item.id, { name: event.target.value })} disabled={busy || !canUpdate} placeholder={t("paymentRechargePackageNamePlaceholder")} /></Field>
                <Field label={t("paymentRechargePackageKind")}><select value={item.kind} onChange={(event) => update(item.id, { kind: event.target.value })} disabled={busy || !canUpdate} className="h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm dark:border-slate-700 dark:bg-slate-900"><option value="recharge">{t("paymentRechargePackageRecharge")}</option><option value="subscription">{t("paymentRechargePackageSubscription")}</option></select></Field>
                <Field label={t("paymentRechargePackageCurrency")}><Input value={item.currency} onChange={(event) => update(item.id, { currency: event.target.value.toUpperCase() })} disabled={busy || !canUpdate} maxLength={3} /></Field>
                <Field label={t("paymentRechargePackageAmount")}><Input value={item.amount} onChange={(event) => update(item.id, { amount: event.target.value })} disabled={busy || !canUpdate} inputMode="decimal" placeholder="100.00" /></Field>
                <Field label={t("paymentRechargePackageCredited")}><Input value={item.credited_amount} onChange={(event) => update(item.id, { credited_amount: event.target.value })} disabled={busy || !canUpdate} inputMode="decimal" placeholder="100.00" /></Field>
                <Field label={t("paymentRechargePackageBonus")}><Input value={item.bonus_amount} onChange={(event) => update(item.id, { bonus_amount: event.target.value })} disabled={busy || !canUpdate} inputMode="decimal" placeholder="0.00" /></Field>
                <Field label={t("paymentRechargePackageValidity")}><Input value={String(item.validity_days)} onChange={(event) => update(item.id, { validity_days: Math.max(0, Number(event.target.value) || 0) })} disabled={busy || !canUpdate} type="number" min={0} max={36500} placeholder="0" /></Field>
                <Field label={t("paymentRechargePackagePlanCode")}><Input value={item.subscription_plan_code || ""} onChange={(event) => update(item.id, { subscription_plan_code: event.target.value })} disabled={busy || !canUpdate} placeholder={t("paymentRechargePackagePlanCodePlaceholder")} /></Field>
                <Field label={t("paymentRechargePackageStartsAt")}><Input type="datetime-local" value={toLocalDateTime(item.starts_at)} onChange={(event) => update(item.id, { starts_at: fromLocalDateTime(event.target.value) })} disabled={busy || !canUpdate} /></Field>
                <Field label={t("paymentRechargePackageEndsAt")}><Input type="datetime-local" value={toLocalDateTime(item.ends_at)} onChange={(event) => update(item.id, { ends_at: fromLocalDateTime(event.target.value) })} disabled={busy || !canUpdate} /></Field>
                <Field label={t("paymentRechargePackageDescriptionField")} className="md:col-span-2 xl:col-span-2"><Input value={item.description || ""} onChange={(event) => update(item.id, { description: event.target.value })} disabled={busy || !canUpdate} placeholder={t("paymentRechargePackageDescriptionPlaceholder")} /></Field>
              </div>
              <label className="mt-4 inline-flex items-center gap-2 text-xs font-medium text-slate-600 dark:text-slate-300"><input type="checkbox" checked={item.enabled} onChange={(event) => update(item.id, { enabled: event.target.checked })} disabled={busy || !canUpdate} />{t("paymentRechargePackageEnabled")}</label>
            </div>
          ))}
          <div className="flex justify-end border-t border-slate-200 pt-4 dark:border-slate-800"><Button type="button" onClick={() => void save(draft)} disabled={busy || !canUpdate} className="gap-2"><Save className="h-4 w-4" />{busy ? t("paymentRechargePackagesSaving") : t("paymentRechargePackagesSave")}</Button></div>
        </CardContent>
      </Card>
      <div className="flex items-start gap-2 rounded-xl border border-indigo-500/20 bg-indigo-50/60 p-4 text-xs leading-5 text-indigo-900 dark:bg-indigo-500/10 dark:text-indigo-100"><CalendarClock className="mt-0.5 h-4 w-4 shrink-0" />{t("paymentRechargePackageValidityHint")}</div>
    </div>
  );
}

function Field({ label, children, className = "" }: { label: string; children: React.ReactNode; className?: string }) {
  return <div className={`space-y-2 ${className}`}><Label>{label}</Label>{children}</div>;
}

function toLocalDateTime(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function fromLocalDateTime(value: string) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}
