import {
  Activity,
  CheckCircle2,
  CircleAlert,
  CircleOff,
  Clock3,
  Gauge,
  HeartPulse,
  Layers3,
  RefreshCw,
  Server,
  TriangleAlert,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { translations } from "@/locales/translations";
import { Language, LoginMessage, ModelRouteStatus, ModelStatus, ModelStatusGroup, ModelStatusReport, TranslationKey } from "@/types";

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
      {groups.map((group) => <GroupStatusSection key={group.group_id} group={group} language={language} t={t} />)}
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

function GroupStatusSection({ group, language, t }: { group: ModelStatusGroup; language: Language; t: (key: TranslationKey) => string }) {
  const recentRequestLimit = group.recent_request_limit || 60;
  return (
    <section className="border-t border-slate-200/80 pt-6 first:border-t-0 first:pt-0 dark:border-slate-800">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-lg font-bold text-slate-950 dark:text-white">{group.group_name}</h3>
            <code className="rounded-md bg-slate-100 px-2 py-1 font-mono text-[10px] text-slate-500 dark:bg-slate-800 dark:text-slate-400">{group.group_code}</code>
            <StatusBadge status={group.status} t={t} />
          </div>
          <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-slate-500 dark:text-slate-400">
            <span>{t("consoleModelStatusMultiplier")} x{group.multiplier}</span>
            <span>{t("consoleModelStatusRPM")} {group.rpm_limit || "-"}</span>
            <span>{group.billing_type === "free" ? t("groupsBillingFree") : t("groupsBillingPrepaid")}</span>
            <span>{t("consoleModelStatusRecentRequests")} {recentRequestLimit}</span>
          </div>
        </div>
        <span className="text-xs text-slate-500 dark:text-slate-400">{t("consoleModelStatusModels")} {group.models.length}</span>
      </div>
      {group.models.length === 0 ? (
        <div className="rounded-xl border border-dashed border-slate-300 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">{t("consoleModelStatusNoModels")}</div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
          {group.models.map((model) => (
            <ModelHealthCard key={`${group.group_id}:${model.provider}:${model.model}`} model={model} recentRequestLimit={recentRequestLimit} language={language} t={t} />
          ))}
        </div>
      )}
    </section>
  );
}

function ModelHealthCard({
  model,
  recentRequestLimit,
  language,
  t,
}: {
  model: ModelStatus;
  recentRequestLimit: number;
  language: Language;
  t: (key: TranslationKey) => string;
}) {
  const availability = model.request_count_7d > 0 ? model.availability_7d : null;
  const healthTone = availability === null ? "text-slate-500 dark:text-slate-400" : availability >= 99 ? "text-emerald-600 dark:text-emerald-400" : availability >= 95 ? "text-amber-600 dark:text-amber-400" : "text-rose-600 dark:text-rose-400";
  const provider = providerLabel(model.provider);
  return (
    <Card className="overflow-hidden border-slate-200/90 bg-white shadow-sm transition-shadow hover:shadow-md dark:border-slate-800 dark:bg-slate-900/70">
      <CardContent className="p-0">
        <div className="flex items-start justify-between gap-3 border-b border-slate-100 px-4 py-4 dark:border-slate-800/80">
          <div className="flex min-w-0 items-start gap-3">
            <div className={cn("flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-xs font-bold uppercase", provider.tone)}>{provider.initials}</div>
            <div className="min-w-0">
              <div className="truncate font-semibold text-slate-950 dark:text-white" title={model.model}>{model.model}</div>
              <div className="mt-1 truncate font-mono text-[10px] text-slate-500 dark:text-slate-400">{model.provider}</div>
            </div>
          </div>
          <StatusBadge status={model.status} t={t} />
        </div>

        <div className="grid grid-cols-2 gap-px bg-slate-100 dark:bg-slate-800">
          <MetricCell icon={Gauge} label={t("consoleModelStatusLatency")} value={model.last_latency_ms > 0 ? formatLatency(model.last_latency_ms) : "-"} />
          <MetricCell icon={Server} label={t("consoleModelStatusRoutes")} value={`${model.available_routes} / ${model.total_routes}`} />
        </div>

        <div className="px-4 pt-4">
          <div className="flex items-end justify-between gap-3">
            <div className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400"><HeartPulse className="h-3.5 w-3.5" />{t("consoleModelStatusHealth")}<span className="hidden sm:inline">{t("consoleModelStatusLast7Days")}</span></div>
            <div className={cn("text-2xl font-extrabold tabular-nums", healthTone)}>{availability === null ? "-" : `${formatPercent(availability)}%`}</div>
          </div>
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
            <div className={cn("h-full rounded-full", availability === null ? "w-0" : availability >= 99 ? "bg-emerald-500" : availability >= 95 ? "bg-amber-500" : "bg-rose-500")} style={{ width: `${availability ?? 0}%` }} />
          </div>
        </div>

        <div className="px-4 pb-4 pt-5">
          <div className="flex items-center justify-between gap-3 text-xs text-slate-500 dark:text-slate-400">
            <span>{t("consoleModelStatusRecentRequests")} {recentRequestLimit}</span>
            <span>{model.recent_statuses?.length ? `${model.recent_statuses.length} ${t("consoleModelStatusRecorded")}` : t("consoleModelStatusNoData")}</span>
          </div>
          <RequestHistory statuses={model.recent_statuses || []} t={t} />
          <div className="mt-2 flex items-center justify-between text-[10px] text-slate-400 dark:text-slate-500"><span>{t("consoleModelStatusEarlier")}</span><span>{model.last_request_at ? formatTime(model.last_request_at, language) : t("consoleModelStatusNow")}</span></div>
          {model.last_failure_reason ? <p className="mt-3 truncate text-xs text-rose-600 dark:text-rose-300" title={model.last_failure_reason}>{model.last_failure_reason}</p> : null}
        </div>
      </CardContent>
    </Card>
  );
}

function MetricCell({ icon: Icon, label, value }: { icon: typeof Gauge; label: string; value: string }) {
  return (
    <div className="min-w-0 bg-white px-4 py-3 dark:bg-slate-900/70">
      <div className="flex items-center gap-1.5 text-[11px] text-slate-500 dark:text-slate-400"><Icon className="h-3.5 w-3.5" />{label}</div>
      <div className="mt-1 truncate font-mono text-sm font-semibold tabular-nums text-slate-900 dark:text-white">{value}</div>
    </div>
  );
}

function RequestHistory({ statuses, t }: { statuses: string[]; t: (key: TranslationKey) => string }) {
  if (statuses.length === 0) return <div className="mt-3 flex h-10 items-center justify-center rounded-md border border-dashed border-slate-200 text-xs text-slate-400 dark:border-slate-800 dark:text-slate-500">{t("consoleModelStatusNoData")}</div>;
  return <div className="mt-3 grid h-10 grid-flow-col auto-cols-fr items-end gap-0.5" title={t("consoleModelStatusRecentRequestsHint")}>{statuses.map((status, index) => <span key={`${status}:${index}`} className={cn("min-w-0 rounded-sm", requestStatusTone(status), requestStatusHeight(status, index))} title={status} />)}</div>;
}

function requestStatusTone(status: string) {
  if (status === "settled") return "bg-emerald-500";
  if (status === "started" || status === "pending" || status === "settlement_pending") return "bg-amber-400";
  return "bg-rose-500";
}

function requestStatusHeight(status: string, index: number) {
  if (status === "failed") return index % 3 === 0 ? "h-4" : "h-3";
  if (status === "started" || status === "pending" || status === "settlement_pending") return "h-5";
  return index % 5 === 0 ? "h-8" : index % 3 === 0 ? "h-7" : "h-6";
}

function providerLabel(provider: string) {
  const normalized = provider.toLowerCase().trim();
  if (normalized === "openai") return { initials: "O", tone: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" };
  if (normalized === "anthropic") return { initials: "A", tone: "bg-orange-500/10 text-orange-700 dark:text-orange-300" };
  if (normalized === "gemini" || normalized === "google") return { initials: "G", tone: "bg-blue-500/10 text-blue-700 dark:text-blue-300" };
  if (normalized === "grok" || normalized === "xai") return { initials: "X", tone: "bg-slate-500/10 text-slate-700 dark:text-slate-300" };
  return { initials: normalized.slice(0, 1) || "AI", tone: "bg-indigo-500/10 text-indigo-700 dark:text-indigo-300" };
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

function formatLatency(value: number) {
  if (value >= 1_000) return `${(value / 1_000).toFixed(value % 1_000 === 0 ? 0 : 2)} s`;
  return `${value} ms`;
}

function formatPercent(value: number) {
  return value.toFixed(value >= 99.95 ? 2 : value >= 99 ? 1 : 2).replace(/\.?0+$/, "");
}
