import { useMemo, useState } from "react";
import { Activity, CheckCircle2, CircleAlert, CircleOff, Clock3, Filter, RefreshCw, Search, Server, TriangleAlert } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";
import { Language, LoginMessage, ModelRouteStatus, ModelStatus, ModelStatusReport, TranslationKey } from "@/types";

interface AdminModelStatusPanelProps {
  language: Language;
  report: ModelStatusReport | null;
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean) => Promise<void>;
}

export function AdminModelStatusPanel({ language, report, busy, message, refresh }: AdminModelStatusPanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [groupFilter, setGroupFilter] = useState("all");
  const [providerFilter, setProviderFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState<"all" | ModelRouteStatus>("all");
  const [search, setSearch] = useState("");
  const groups = report?.groups ?? [];
  const providers = useMemo(() => Array.from(new Set(groups.flatMap((group) => group.models.map((model) => model.provider)))).sort(), [groups]);
  const filteredGroups = groups
    .filter((group) => groupFilter === "all" || group.group_id === groupFilter)
    .map((group) => ({
      ...group,
      models: group.models.filter((model) => {
        const haystack = `${model.model} ${model.provider}`.toLowerCase();
        return (providerFilter === "all" || model.provider === providerFilter) &&
          (statusFilter === "all" || model.status === statusFilter) &&
          (!search.trim() || haystack.includes(search.trim().toLowerCase()));
      }),
    }))
    .filter((group) => group.models.length > 0 || (!search.trim() && statusFilter === "all"));
  const models = groups.flatMap((group) => group.models);
  const normal = models.filter((model) => model.status === "normal").length;
  const attention = models.filter((model) => model.status === "degraded" || model.status === "pending").length;
  const unavailable = models.filter((model) => model.status === "unavailable" || model.status === "disabled").length;
  const routes = groups.reduce((sum, group) => sum + group.models.reduce((inner, model) => inner + model.total_routes, 0), 0);
  const availableRoutes = groups.reduce((sum, group) => sum + group.models.reduce((inner, model) => inner + model.available_routes, 0), 0);

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-indigo-500/20 bg-gradient-to-br from-indigo-500/[0.08] via-cyan-500/[0.04] to-transparent p-5 dark:border-indigo-400/20 sm:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-400"><Activity className="h-4 w-4" />{t("adminModelStatusEyebrow")}</div>
            <h2 className="mt-2 text-2xl font-extrabold text-slate-950 dark:text-white">{t("adminModelStatusTitle")}</h2>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("adminModelStatusDescription")}</p>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={() => void refresh(true)} disabled={busy} className="w-full gap-2 sm:w-auto"><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} />{t("adminModelStatusRefresh")}</Button>
        </div>
        <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <SummaryMetric icon={Server} label={t("adminModelStatusModels")} value={String(models.length)} tone="indigo" />
          <SummaryMetric icon={CheckCircle2} label={t("adminModelStatusHealthy")} value={String(normal)} tone="emerald" />
          <SummaryMetric icon={TriangleAlert} label={t("adminModelStatusAttention")} value={String(attention)} tone="amber" />
          <SummaryMetric icon={CircleOff} label={t("adminModelStatusUnavailableCount")} value={String(unavailable)} tone="rose" />
          <SummaryMetric icon={Activity} label={t("adminModelStatusRoutes")} value={`${availableRoutes}/${routes}`} tone="cyan" />
        </div>
        <div className="mt-4 flex flex-col gap-2 text-xs text-slate-500 dark:text-slate-400 sm:flex-row sm:items-center sm:justify-between"><span className="flex items-center gap-1.5"><Clock3 className="h-3.5 w-3.5" />{t("adminModelStatusUpdated")} {formatTime(report?.updated_at, language)}</span><span>{t("adminModelStatusSource")}</span></div>
      </section>

      {message.text ? <div className={cn("rounded-xl border p-3 text-sm", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300")}>{message.text}</div> : null}

      <Card className="glass-panel">
        <CardHeader className="space-y-4 pb-4">
          <div><CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white"><Filter className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />{t("adminModelStatusFiltersTitle")}</CardTitle><CardDescription>{t("adminModelStatusFiltersDescription")}</CardDescription></div>
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px_180px_180px]">
            <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("adminModelStatusSearchPlaceholder")} aria-label={t("adminModelStatusSearchPlaceholder")} />
            <select value={groupFilter} onChange={(event) => setGroupFilter(event.target.value)} className="h-10 rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" aria-label={t("adminModelStatusGroupFilter")}><option value="all">{t("adminModelStatusAllGroups")}</option>{groups.map((group) => <option key={group.group_id} value={group.group_id}>{group.group_name} ({group.group_code})</option>)}</select>
            <select value={providerFilter} onChange={(event) => setProviderFilter(event.target.value)} className="h-10 rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" aria-label={t("adminModelStatusProviderFilter")}><option value="all">{t("adminModelStatusAllProviders")}</option>{providers.map((provider) => <option key={provider} value={provider}>{provider}</option>)}</select>
            <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as "all" | ModelRouteStatus)} className="h-10 rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" aria-label={t("adminModelStatusStatusFilter")}><option value="all">{t("adminModelStatusAllStatuses")}</option><option value="normal">{t("consoleModelStatusNormalLabel")}</option><option value="pending">{t("consoleModelStatusPendingLabel")}</option><option value="degraded">{t("consoleModelStatusDegradedLabel")}</option><option value="unavailable">{t("consoleModelStatusUnavailableLabel")}</option><option value="disabled">{t("consoleModelStatusDisabledLabel")}</option></select>
          </div>
        </CardHeader>
        <CardContent>
          {busy && !report ? <EmptyState icon={RefreshCw} label={t("adminModelStatusLoading")} spinning /> : null}
          {!busy && report && models.length === 0 ? <EmptyState icon={CircleOff} label={t("adminModelStatusEmpty")} /> : null}
          {report && models.length > 0 && filteredGroups.length === 0 ? <EmptyState icon={Search} label={t("adminModelStatusNoMatch")} /> : null}
          <div className="space-y-4">
            {filteredGroups.map((group) => <GroupStatusCard key={group.group_id} group={group} language={language} t={t} />)}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

function SummaryMetric({ icon: Icon, label, value, tone }: { icon: typeof Activity; label: string; value: string; tone: "indigo" | "cyan" | "emerald" | "amber" | "rose" }) {
  const style = tone === "cyan" ? "border-cyan-500/20 bg-cyan-500/[0.06] text-cyan-700 dark:text-cyan-300" : tone === "emerald" ? "border-emerald-500/20 bg-emerald-500/[0.06] text-emerald-700 dark:text-emerald-300" : tone === "amber" ? "border-amber-500/20 bg-amber-500/[0.06] text-amber-700 dark:text-amber-300" : tone === "rose" ? "border-rose-500/20 bg-rose-500/[0.06] text-rose-700 dark:text-rose-300" : "border-indigo-500/20 bg-indigo-500/[0.06] text-indigo-700 dark:text-indigo-300";
  return <div className={cn("flex items-center gap-3 rounded-xl border px-4 py-3", style)}><Icon className="h-4 w-4 shrink-0" /><div><div className="text-[11px] opacity-75">{label}</div><div className="mt-0.5 text-xl font-bold">{value}</div></div></div>;
}

function GroupStatusCard({ group, language, t }: { group: ModelStatusReport["groups"][number]; language: Language; t: (key: TranslationKey) => string }) {
  return <div className="overflow-hidden rounded-xl border border-slate-200 dark:border-slate-800">
    <div className="flex flex-col gap-3 border-b border-slate-200 bg-slate-50/80 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/50 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><span className="font-semibold text-slate-900 dark:text-white">{group.group_name}</span><code className="rounded bg-slate-200/70 px-1.5 py-0.5 font-mono text-[10px] text-slate-600 dark:bg-slate-800 dark:text-slate-400">{group.group_code}</code><StatusBadge status={group.status} t={t} /></div><div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-slate-500 dark:text-slate-400"><span>{t("adminModelStatusMultiplier")} x{group.multiplier}</span><span>{t("adminModelStatusGroupModels")} {group.models.length}</span><span>{group.rpm_limit ? `RPM ${group.rpm_limit}` : "RPM -"}</span></div></div>
      <div className="text-xs text-slate-500 dark:text-slate-400">{group.group_status === "active" ? t("adminModelStatusGroupActive") : t("adminModelStatusGroupDisabled")}</div>
    </div>
    <div className="overflow-x-auto"><table className="w-full min-w-[980px] text-sm"><thead className="border-b border-slate-200 text-left text-[11px] text-slate-500 dark:border-slate-800 dark:text-slate-400"><tr><th className="px-4 py-3">{t("adminModelStatusModel")}</th><th className="px-4 py-3">{t("adminModelStatusState")}</th><th className="px-4 py-3">{t("adminModelStatusRoutes")}</th><th className="px-4 py-3">{t("adminModelStatusLatency")}</th><th className="px-4 py-3">{t("adminModelStatusAvailability")}</th><th className="px-4 py-3">{t("adminModelStatusRecentRequests")}</th><th className="px-4 py-3">{t("adminModelStatusLastRequest")}</th></tr></thead><tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">{group.models.map((model) => <ModelStatusRow key={`${group.group_id}:${model.provider}:${model.model}`} model={model} language={language} t={t} />)}</tbody></table></div>
  </div>;
}

function ModelStatusRow({ model, language, t }: { model: ModelStatus; language: Language; t: (key: TranslationKey) => string }) {
  return <tr className="align-top"><td className="px-4 py-3"><div className="font-semibold text-slate-900 dark:text-white">{model.model}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{model.provider}</div></td><td className="px-4 py-3"><StatusBadge status={model.status} t={t} /><div className="mt-1 text-[10px] text-slate-500 dark:text-slate-400">{model.consecutive_failures} {t("adminModelStatusConsecutiveFailures")}</div></td><td className="px-4 py-3 font-mono text-xs text-slate-700 dark:text-slate-300"><div>{model.available_routes} / {model.total_routes}</div><div className="mt-2 h-1.5 w-24 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800"><div className={cn("h-full rounded-full", model.available_routes === 0 ? "bg-rose-500" : model.available_routes < model.total_routes ? "bg-amber-500" : "bg-emerald-500")} style={{ width: `${model.total_routes > 0 ? Math.min(100, model.available_routes / model.total_routes * 100) : 0}%` }} /></div></td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-700 dark:text-slate-300">{model.last_latency_ms > 0 ? `${model.last_latency_ms} ms` : "-"}</td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-700 dark:text-slate-300">{model.request_count_7d > 0 ? `${model.availability_7d.toFixed(2)}%` : t("adminModelStatusNoData")}</td><td className="px-4 py-3"><RequestStrip statuses={model.recent_statuses || []} t={t} /></td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-500 dark:text-slate-400"><div>{statusLabel(model.last_request_status, t)}</div><div className="mt-1">{formatTime(model.last_request_at, language)}</div>{model.last_failure_reason ? <div className="mt-1 max-w-[220px] truncate text-rose-600 dark:text-rose-300" title={model.last_failure_reason}>{model.last_failure_reason}</div> : null}</td></tr>;
}

function RequestStrip({ statuses, t }: { statuses: string[]; t: (key: TranslationKey) => string }) {
  if (statuses.length === 0) return <span className="text-xs text-slate-400">{t("adminModelStatusNoData")}</span>;
  return <div className="flex min-w-[220px] items-center gap-0.5" title={t("adminModelStatusRecentRequestsHint")}>{statuses.map((status, index) => <span key={`${status}:${index}`} className={cn("h-5 w-1.5 rounded-sm", status === "settled" ? "bg-emerald-500" : status === "started" || status === "pending" || status === "settlement_pending" ? "bg-amber-400" : "bg-rose-500")} />)}</div>;
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

function EmptyState({ icon: Icon, label, spinning = false }: { icon: typeof Activity; label: string; spinning?: boolean }) {
  return <div className="rounded-xl border border-dashed border-slate-300 py-16 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400"><Icon className={cn("mx-auto mb-3 h-6 w-6", spinning ? "animate-spin text-indigo-600" : "")} />{label}</div>;
}

function statusLabel(status: string | undefined, t: (key: TranslationKey) => string) {
  if (status === "settled") return t("adminModelStatusRequestSuccess");
  if (status === "failed") return t("adminModelStatusRequestFailed");
  if (status === "settlement_pending") return t("adminModelStatusRequestPending");
  if (status) return status;
  return t("adminModelStatusNoData");
}

function formatTime(value: string | undefined, language: Language) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}
