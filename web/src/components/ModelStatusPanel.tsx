import {
  Activity,
  CheckCircle2,
  CircleAlert,
  CircleOff,
  Clock3,
  Layers3,
  RefreshCw,
  Server,
  TriangleAlert,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { translations } from "@/locales/translations";
import { Language, LoginMessage, ModelRouteStatus, ModelStatusGroup, ModelStatusReport, TranslationKey } from "@/types";

interface ModelStatusPanelProps {
  language: Language;
  report: ModelStatusReport | null;
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean) => Promise<void>;
}

export function ModelStatusPanel({ language, report, busy, message, refresh }: ModelStatusPanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const groups = report?.groups || [];
  const models = groups.flatMap((group) => group.models);
  const normal = models.filter((model) => model.status === "normal").length;
  const attention = models.filter((model) => model.status !== "normal").length;

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-indigo-500/20 bg-gradient-to-br from-indigo-500/[0.08] via-cyan-500/[0.04] to-transparent p-5 dark:border-indigo-400/20 sm:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-400"><Activity className="h-4 w-4" />{t("consoleModelStatusEyebrow")}</div>
            <h2 className="mt-2 text-2xl font-extrabold text-slate-950 dark:text-white">{t("consoleModelStatusTitle")}</h2>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("consoleModelStatusDescription")}</p>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={() => void refresh(true)} disabled={busy} className="w-full gap-2 sm:w-auto"><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button>
        </div>
        <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <SummaryMetric icon={Layers3} label={t("consoleModelStatusGroups")} value={String(groups.length)} tone="indigo" />
          <SummaryMetric icon={Server} label={t("consoleModelStatusModels")} value={String(models.length)} tone="cyan" />
          <SummaryMetric icon={CheckCircle2} label={t("consoleModelStatusNormal")} value={String(normal)} tone="emerald" />
          <SummaryMetric icon={TriangleAlert} label={t("consoleModelStatusAttention")} value={String(attention)} tone="amber" />
        </div>
        <div className="mt-4 flex flex-col gap-2 text-xs text-slate-500 dark:text-slate-400 sm:flex-row sm:items-center sm:justify-between"><span className="flex items-center gap-1.5"><Clock3 className="h-3.5 w-3.5" />{t("consoleModelStatusUpdated")} {formatTime(report?.updated_at, language)}</span><span>{t("consoleModelStatusSource")}</span></div>
      </section>

      {message.text ? <div className={cn("rounded-xl border p-3 text-sm", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300")}>{message.text}</div> : null}

      {busy && !report ? <EmptyState icon={RefreshCw} label={t("consoleModelStatusLoading")} spinning /> : null}
      {!busy && groups.length === 0 ? <EmptyState icon={CircleOff} label={t("consoleModelStatusEmpty")} /> : null}
      {groups.map((group) => <GroupCard key={group.group_id} group={group} language={language} t={t} />)}
    </div>
  );
}

function SummaryMetric({ icon: Icon, label, value, tone }: { icon: typeof Activity; label: string; value: string; tone: "indigo" | "cyan" | "emerald" | "amber" }) {
  const style = tone === "cyan" ? "border-cyan-500/20 bg-cyan-500/[0.06] text-cyan-700 dark:text-cyan-300" : tone === "emerald" ? "border-emerald-500/20 bg-emerald-500/[0.06] text-emerald-700 dark:text-emerald-300" : tone === "amber" ? "border-amber-500/20 bg-amber-500/[0.06] text-amber-700 dark:text-amber-300" : "border-indigo-500/20 bg-indigo-500/[0.06] text-indigo-700 dark:text-indigo-300";
  return <div className={cn("flex items-center gap-3 rounded-xl border px-4 py-3", style)}><Icon className="h-4 w-4 shrink-0" /><div><div className="text-[11px] opacity-75">{label}</div><div className="mt-0.5 text-xl font-bold">{value}</div></div></div>;
}

function EmptyState({ icon: Icon, label, spinning = false }: { icon: typeof Activity; label: string; spinning?: boolean }) {
  return <div className="rounded-2xl border border-dashed border-slate-300 py-20 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400"><Icon className={cn("mx-auto mb-3 h-6 w-6", spinning ? "animate-spin text-indigo-600" : "")} />{label}</div>;
}

function GroupCard({ group, language, t }: { group: ModelStatusGroup; language: Language; t: (key: TranslationKey) => string }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
    <CardHeader className="flex flex-col gap-4 pb-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><CardTitle className="text-lg text-slate-950 dark:text-white">{group.group_name}</CardTitle><code className="rounded-md bg-slate-100 px-2 py-1 text-[10px] text-slate-500 dark:bg-slate-800 dark:text-slate-400">{group.group_code}</code></div><CardDescription className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs"><span>{t("consoleModelStatusMultiplier")} x{group.multiplier}</span><span>{t("consoleModelStatusRPM")} {group.rpm_limit || "-"}</span><span>{group.billing_type === "free" ? t("groupsBillingFree") : t("groupsBillingPrepaid")}</span></CardDescription></div>
      <StatusBadge status={group.status} t={t} />
    </CardHeader>
    <CardContent className="pt-0">
      {group.models.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">{t("consoleModelStatusNoModels")}</div> : <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800"><table className="w-full min-w-[760px] text-sm"><thead className="border-b border-slate-200 bg-slate-50 text-left text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-950/50 dark:text-slate-400"><tr><th className="px-4 py-3">{t("consoleModelStatusModel")}</th><th className="px-4 py-3">{t("consoleModelStatusState")}</th><th className="px-4 py-3">{t("consoleModelStatusRoutes")}</th><th className="px-4 py-3">{t("consoleModelStatusFailures")}</th><th className="px-4 py-3">{t("consoleModelStatusLastSuccess")}</th><th className="px-4 py-3">{t("consoleModelStatusLastFailure")}</th></tr></thead><tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">{group.models.map((model) => <tr key={`${model.provider}:${model.model}`}><td className="px-4 py-3"><div className="font-semibold text-slate-900 dark:text-white">{model.model}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{model.provider}</div></td><td className="px-4 py-3"><StatusBadge status={model.status} t={t} /></td><td className="px-4 py-3 font-mono text-xs text-slate-700 dark:text-slate-300">{model.available_routes} / {model.total_routes}</td><td className="px-4 py-3 font-mono text-xs text-slate-700 dark:text-slate-300">{model.consecutive_failures}</td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-500 dark:text-slate-400">{formatTime(model.last_success_at, language)}</td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-500 dark:text-slate-400">{formatTime(model.last_failure_at, language)}</td></tr>)}</tbody></table></div>}
    </CardContent>
  </Card>;
}

function StatusBadge({ status, t }: { status: ModelRouteStatus; t: (key: TranslationKey) => string }) {
  const values: Record<ModelRouteStatus, { label: TranslationKey; variant: "success" | "warning" | "destructive" | "muted"; icon: typeof CheckCircle2 }> = {
    normal: { label: "consoleModelStatusNormalLabel", variant: "success", icon: CheckCircle2 },
    pending: { label: "consoleModelStatusPendingLabel", variant: "warning", icon: Clock3 },
    degraded: { label: "consoleModelStatusDegradedLabel", variant: "warning", icon: CircleAlert },
    unavailable: { label: "consoleModelStatusUnavailableLabel", variant: "destructive", icon: CircleOff },
    disabled: { label: "consoleModelStatusDisabledLabel", variant: "muted", icon: CircleOff },
  };
  const item = values[status] || values.unavailable;
  const Icon = item.icon;
  return <Badge variant={item.variant}><Icon className="h-3.5 w-3.5" />{t(item.label)}</Badge>;
}

function formatTime(value: string | undefined, language: Language) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}
