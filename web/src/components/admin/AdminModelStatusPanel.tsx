import { useMemo, useState } from "react";
import {
  Activity,
  Check,
  CheckCircle2,
  CircleAlert,
  CircleOff,
  Clock3,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  Save,
  Search,
  Server,
  Trash2,
  TriangleAlert,
  X,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";
import {
  GroupSummary,
  Language,
  LoginMessage,
  ModelMonitor,
  ModelMonitorFormState,
  ModelRouteStatus,
  ModelStatus,
  ModelStatusReport,
  TranslationKey,
} from "@/types";

interface AdminModelStatusPanelProps {
  language: Language;
  report: ModelStatusReport | null;
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean) => Promise<void>;
  groups: GroupSummary[];
  monitors: ModelMonitor[];
  monitorsBusy: boolean;
  monitorsMessage: LoginMessage;
  formOpen: boolean;
  form: ModelMonitorFormState;
  setForm: React.Dispatch<React.SetStateAction<ModelMonitorFormState>>;
  actionBusy: string;
  openCreate: () => void;
  openEdit: (monitor: ModelMonitor) => void;
  closeForm: () => void;
  save: (form: ModelMonitorFormState) => Promise<boolean>;
  remove: (monitor: ModelMonitor) => Promise<void>;
  probe: (monitor: ModelMonitor) => Promise<void>;
}

export function AdminModelStatusPanel({
  language,
  report,
  busy,
  message,
  refresh,
  groups,
  monitors,
  monitorsBusy,
  monitorsMessage,
  formOpen,
  form,
  setForm,
  actionBusy,
  openCreate,
  openEdit,
  closeForm,
  save,
  remove,
  probe,
}: AdminModelStatusPanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [groupFilter, setGroupFilter] = useState("all");
  const [providerFilter, setProviderFilter] = useState("all");
  const [statusFilter, setStatusFilter] = useState<"all" | ModelRouteStatus>("all");
  const [search, setSearch] = useState("");
  const groupsInReport = report?.groups ?? [];
  const providers = useMemo(
    () => Array.from(new Set(groupsInReport.flatMap((group) => group.models.map((model) => model.provider)))).sort(),
    [groupsInReport],
  );
  const filteredGroups = groupsInReport
    .filter((group) => groupFilter === "all" || group.group_id === groupFilter)
    .map((group) => ({
      ...group,
      models: group.models.filter((model) => {
        const haystack = `${model.model} ${model.provider}`.toLowerCase();
        return (
          (providerFilter === "all" || model.provider === providerFilter) &&
          (statusFilter === "all" || model.status === statusFilter) &&
          (!search.trim() || haystack.includes(search.trim().toLowerCase()))
        );
      }),
    }))
    .filter((group) => group.models.length > 0 || (!search.trim() && statusFilter === "all"));
  const models = groupsInReport.flatMap((group) => group.models);
  const normal = models.filter((model) => model.status === "normal").length;
  const attention = models.filter((model) => model.status === "degraded" || model.status === "pending").length;
  const unavailable = models.filter((model) => model.status === "unavailable" || model.status === "disabled").length;
  const routes = groupsInReport.reduce((sum, group) => sum + group.models.reduce((inner, model) => inner + model.total_routes, 0), 0);
  const availableRoutes = groupsInReport.reduce((sum, group) => sum + group.models.reduce((inner, model) => inner + model.available_routes, 0), 0);

  return (
    <div className="space-y-6">
      <section className="rounded-2xl border border-indigo-500/20 bg-gradient-to-br from-indigo-500/[0.08] via-cyan-500/[0.04] to-transparent p-5 dark:border-indigo-400/20 sm:p-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-600 dark:text-indigo-400">
              <Activity className="h-4 w-4" />
              {t("adminModelStatusEyebrow")}
            </div>
            <h2 className="mt-2 text-2xl font-extrabold text-slate-950 dark:text-white">{t("adminModelStatusTitle")}</h2>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("adminModelStatusDescription")}</p>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={() => void refresh(true)} disabled={busy} className="w-full gap-2 sm:w-auto">
            <RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} />
            {t("adminModelStatusRefresh")}
          </Button>
        </div>
        <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <SummaryMetric icon={Server} label={t("adminModelStatusModels")} value={String(models.length)} tone="indigo" />
          <SummaryMetric icon={CheckCircle2} label={t("adminModelStatusHealthy")} value={String(normal)} tone="emerald" />
          <SummaryMetric icon={TriangleAlert} label={t("adminModelStatusAttention")} value={String(attention)} tone="amber" />
          <SummaryMetric icon={CircleOff} label={t("adminModelStatusUnavailableCount")} value={String(unavailable)} tone="rose" />
          <SummaryMetric icon={Activity} label={t("adminModelStatusRoutes")} value={`${availableRoutes}/${routes}`} tone="cyan" />
        </div>
        <div className="mt-4 flex flex-col gap-2 text-xs text-slate-500 dark:text-slate-400 sm:flex-row sm:items-center sm:justify-between">
          <span className="flex items-center gap-1.5">
            <Clock3 className="h-3.5 w-3.5" />
            {t("adminModelStatusUpdated")} {formatTime(report?.updated_at, language)}
          </span>
          <span>{t("adminModelStatusSource")}</span>
        </div>
      </section>

      {monitorsMessage.text ? <Notice message={monitorsMessage} /> : null}
      {message.text ? <Notice message={message} /> : null}

      <Card className="glass-panel">
        <CardHeader className="space-y-4 pb-4">
          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
            <div>
              <CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white">
                <Activity className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />
                {t("adminModelMonitorConfigTitle")}
              </CardTitle>
              <CardDescription>{t("adminModelMonitorConfigDescription")}</CardDescription>
            </div>
            <Button type="button" size="sm" onClick={openCreate} disabled={Boolean(actionBusy)} className="gap-2">
              <Plus className="h-4 w-4" />
              {t("adminModelMonitorAdd")}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {monitorsBusy && monitors.length === 0 ? <EmptyState icon={RefreshCw} label={t("adminModelMonitorLoading")} spinning /> : null}
          {!monitorsBusy && monitors.length === 0 ? <EmptyState icon={Activity} label={t("adminModelMonitorEmpty")} /> : null}
          <div className="space-y-3">
            {monitors.map((monitor) => (
              <MonitorConfigRow
                key={monitor.id}
                monitor={monitor}
                language={language}
                t={t}
                busy={Boolean(actionBusy)}
                actionBusy={actionBusy}
                openEdit={openEdit}
                remove={remove}
                probe={probe}
              />
            ))}
          </div>
        </CardContent>
      </Card>

      <Card className="glass-panel">
        <CardHeader className="space-y-4 pb-4">
          <div>
            <CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white">
              <Server className="h-4 w-4 text-cyan-600 dark:text-cyan-400" />
              {t("adminModelStatusResultsTitle")}
            </CardTitle>
            <CardDescription>{t("adminModelStatusResultsDescription")}</CardDescription>
          </div>
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_180px_180px_180px]">
            <Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("adminModelStatusSearchPlaceholder")} aria-label={t("adminModelStatusSearchPlaceholder")} />
            <select value={groupFilter} onChange={(event) => setGroupFilter(event.target.value)} className={selectClass} aria-label={t("adminModelStatusGroupFilter")}>
              <option value="all">{t("adminModelStatusAllGroups")}</option>
              {groupsInReport.map((group) => <option key={group.group_id} value={group.group_id}>{group.group_name} ({group.group_code})</option>)}
            </select>
            <select value={providerFilter} onChange={(event) => setProviderFilter(event.target.value)} className={selectClass} aria-label={t("adminModelStatusProviderFilter")}>
              <option value="all">{t("adminModelStatusAllProviders")}</option>
              {providers.map((provider) => <option key={provider} value={provider}>{provider}</option>)}
            </select>
            <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value as "all" | ModelRouteStatus)} className={selectClass} aria-label={t("adminModelStatusStatusFilter")}>
              <option value="all">{t("adminModelStatusAllStatuses")}</option>
              <option value="normal">{t("consoleModelStatusNormalLabel")}</option>
              <option value="pending">{t("consoleModelStatusPendingLabel")}</option>
              <option value="degraded">{t("consoleModelStatusDegradedLabel")}</option>
              <option value="unavailable">{t("consoleModelStatusUnavailableLabel")}</option>
              <option value="disabled">{t("consoleModelStatusDisabledLabel")}</option>
            </select>
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

      {formOpen ? (
        <MonitorConfigModal
          groups={groups}
          form={form}
          setForm={setForm}
          busy={Boolean(actionBusy)}
          t={t}
          onClose={closeForm}
          onSubmit={() => void save(form)}
        />
      ) : null}
    </div>
  );
}

const selectClass = "h-10 rounded-md border border-slate-200 bg-white px-3 text-sm text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200";

function SummaryMetric({ icon: Icon, label, value, tone }: { icon: typeof Activity; label: string; value: string; tone: "indigo" | "cyan" | "emerald" | "amber" | "rose" }) {
  const style = tone === "cyan"
    ? "border-cyan-500/20 bg-cyan-500/[0.06] text-cyan-700 dark:text-cyan-300"
    : tone === "emerald"
    ? "border-emerald-500/20 bg-emerald-500/[0.06] text-emerald-700 dark:text-emerald-300"
    : tone === "amber"
    ? "border-amber-500/20 bg-amber-500/[0.06] text-amber-700 dark:text-amber-300"
    : tone === "rose"
    ? "border-rose-500/20 bg-rose-500/[0.06] text-rose-700 dark:text-rose-300"
    : "border-indigo-500/20 bg-indigo-500/[0.06] text-indigo-700 dark:text-indigo-300";
  return <div className={cn("flex items-center gap-3 rounded-xl border px-4 py-3", style)}><Icon className="h-4 w-4 shrink-0" /><div><div className="text-[11px] opacity-75">{label}</div><div className="mt-0.5 text-xl font-bold">{value}</div></div></div>;
}

function Notice({ message }: { message: LoginMessage }) {
  return <div className={cn("rounded-xl border p-3 text-sm", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "success" ? "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300" : "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300")}>{message.text}</div>;
}

function MonitorConfigRow({
  monitor,
  language,
  t,
  busy,
  actionBusy,
  openEdit,
  remove,
  probe,
}: {
  monitor: ModelMonitor;
  language: Language;
  t: (key: TranslationKey) => string;
  busy: boolean;
  actionBusy: string;
  openEdit: (monitor: ModelMonitor) => void;
  remove: (monitor: ModelMonitor) => Promise<void>;
  probe: (monitor: ModelMonitor) => Promise<void>;
}) {
  const probeBusy = actionBusy === `probe:${monitor.id}`;
  return (
    <div className="rounded-xl border border-slate-200 bg-white/70 p-4 dark:border-slate-800 dark:bg-slate-950/30">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate font-semibold text-slate-900 dark:text-white">{monitor.name}</span>
            <Badge variant={monitor.enabled ? "success" : "muted"}>{monitor.enabled ? t("adminModelMonitorEnabled") : t("adminModelMonitorDisabled")}</Badge>
            <Badge variant={monitor.mode === "active" ? "cyan" : "secondary"}>{monitor.mode === "active" ? t("adminModelMonitorActive") : t("adminModelMonitorPassive")}</Badge>
          </div>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500 dark:text-slate-400">
            <span>{t("adminModelMonitorGroup")}: <strong className="text-slate-700 dark:text-slate-300">{monitor.group_name}</strong> <code className="font-mono">({monitor.group_code})</code></span>
            <span>{t("adminModelMonitorModels")}: <strong className="text-slate-700 dark:text-slate-300">{monitor.selection_mode === "all" ? t("adminModelMonitorAllModels") : `${monitor.model_names.length} ${t("adminModelMonitorSelectedUnit")}`}</strong></span>
            <span>{t("adminModelMonitorPrimaryModel")}: <strong className="font-mono text-slate-700 dark:text-slate-300">{monitor.primary_model || "-"}</strong></span>
            <span>{t("adminModelMonitorRecentRequests")}: <strong className="text-slate-700 dark:text-slate-300">{monitor.recent_request_limit || 60}</strong></span>
            {monitor.mode === "active" ? <span>{t("adminModelMonitorInterval")}: <strong className="text-slate-700 dark:text-slate-300">{Math.max(1, Math.round(monitor.probe_interval_seconds / 60))} {t("adminModelMonitorMinutes")}</strong></span> : null}
          </div>
           <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-400 dark:text-slate-500">
             <span>{t("adminModelMonitorLastRun")}: {isProbeRunning(monitor) ? t("adminModelMonitorRunning") : formatTime(monitor.last_probe_finished_at, language)}</span>
            {monitor.mode === "active" ? <span>{t("adminModelMonitorNextRun")}: {isProbeRunning(monitor) ? t("adminModelMonitorNextRunPending") : formatTime(monitor.next_probe_at, language)}</span> : null}
             {monitor.last_probe_status || isProbeRunning(monitor) ? <span className={isProbeRunning(monitor) ? "text-cyan-500" : monitor.last_probe_status === "failed" ? "text-rose-500" : monitor.last_probe_status === "success" ? "text-emerald-500" : "text-amber-500"}>{isProbeRunning(monitor) ? t("adminModelMonitorRunning") : probeStatusLabel(monitor.last_probe_status, t)}</span> : null}
            {monitor.last_probe_error ? <span className="max-w-full truncate text-rose-500" title={monitor.last_probe_error}>{monitor.last_probe_error}</span> : null}
          </div>
        </div>
        <div className="flex flex-wrap gap-2 xl:shrink-0">
          {monitor.mode === "active" ? (
            <Button type="button" variant="outline" size="sm" onClick={() => void probe(monitor)} disabled={busy} className="gap-1.5">
              <Play className={cn("h-3.5 w-3.5", probeBusy ? "animate-pulse" : "")} />
              {probeBusy ? t("adminModelMonitorProbing") : t("adminModelMonitorProbeNow")}
            </Button>
          ) : null}
          <Button type="button" variant="ghost" size="sm" onClick={() => openEdit(monitor)} disabled={busy} className="gap-1.5">
            <Pencil className="h-3.5 w-3.5" />
            {t("adminModelMonitorEdit")}
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={() => void remove(monitor)} disabled={busy} className="gap-1.5 text-rose-600 hover:bg-rose-50 hover:text-rose-700 dark:text-rose-400 dark:hover:bg-rose-500/10">
            <Trash2 className="h-3.5 w-3.5" />
            {t("adminModelMonitorDelete")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function MonitorConfigModal({
  groups,
  form,
  setForm,
  busy,
  t,
  onClose,
  onSubmit,
}: {
  groups: GroupSummary[];
  form: ModelMonitorFormState;
  setForm: React.Dispatch<React.SetStateAction<ModelMonitorFormState>>;
  busy: boolean;
  t: (key: TranslationKey) => string;
  onClose: () => void;
  onSubmit: () => void;
}) {
  const selectedGroup = groups.find((group) => group.id === form.group_id);
  const groupModels = selectedGroup?.models ?? [];
  const availableModels = Array.from(new Set(form.selection_mode === "all" ? groupModels : form.model_names)).sort();

  function setGroup(groupID: string) {
    const group = groups.find((item) => item.id === groupID);
    const nextGroupModels = group?.models ?? [];
    setForm((current) => {
      const nextModels = current.selection_mode === "all"
        ? []
        : nextGroupModels.filter((model) => current.model_names.includes(model));
      const scopeModels = current.selection_mode === "all" ? nextGroupModels : nextModels;
      return {
        ...current,
        group_id: groupID,
        model_names: nextModels,
        primary_model: scopeModels.includes(current.primary_model) ? current.primary_model : scopeModels[0] || "",
        name: current.name || (group ? `${group.name} ${t("adminModelMonitorDefaultName")}` : ""),
      };
    });
  }

  function setSelectionMode(mode: "all" | "selected") {
    setForm((current) => {
      const nextModels = mode === "all"
        ? []
        : current.selection_mode === "selected" && current.model_names.length > 0
          ? current.model_names.filter((model) => groupModels.includes(model))
          : [...groupModels];
      const scopeModels = mode === "all" ? groupModels : nextModels;
      return {
        ...current,
        selection_mode: mode,
        model_names: nextModels,
        primary_model: scopeModels.includes(current.primary_model) ? current.primary_model : scopeModels[0] || "",
      };
    });
  }

  function toggleModel(model: string) {
    setForm((current) => {
      const modelNames = current.model_names.includes(model)
        ? current.model_names.filter((item) => item !== model)
        : [...current.model_names, model];
      return {
        ...current,
        model_names: modelNames,
        primary_model: modelNames.includes(current.primary_model) ? current.primary_model : modelNames[0] || "",
      };
    });
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/60 p-3 backdrop-blur-sm sm:p-6">
      <button type="button" aria-label={t("adminModelMonitorClose")} className="absolute inset-0 h-full w-full cursor-default" onClick={onClose} disabled={busy} />
      <div role="dialog" aria-modal="true" aria-labelledby="model-monitor-dialog-title" className="relative z-10 flex max-h-[92vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white text-slate-900 shadow-2xl dark:border-slate-700/80 dark:bg-slate-900 dark:text-slate-100">
        <div className="flex items-center justify-between gap-4 border-b border-slate-200 bg-slate-50/80 px-5 py-4 dark:border-slate-800/80 dark:bg-slate-950/60 sm:px-6">
          <div>
            <h2 id="model-monitor-dialog-title" className="text-lg font-bold text-slate-900 dark:text-white">{form.id ? t("adminModelMonitorEditTitle") : t("adminModelMonitorAddTitle")}</h2>
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorModalDescription")}</p>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={onClose} disabled={busy} title={t("adminModelMonitorClose")}><X className="h-4 w-4" /></Button>
        </div>
        <div className="min-h-0 overflow-y-auto px-5 py-5 sm:px-6">
          <form id="model-monitor-form" className="space-y-5" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="model-monitor-name">{t("adminModelMonitorName")}</Label>
                <Input id="model-monitor-name" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} disabled={busy} placeholder={t("adminModelMonitorNamePlaceholder")} />
              </div>
              <div className="space-y-2 sm:col-span-2">
                <Label htmlFor="model-monitor-group">{t("adminModelMonitorGroup")}</Label>
                <select id="model-monitor-group" value={form.group_id} onChange={(event) => setGroup(event.target.value)} disabled={busy} className={cn(selectClass, "w-full")}>
                  <option value="">{t("adminModelMonitorSelectGroup")}</option>
                  {groups.map((group) => <option key={group.id} value={group.id}>{group.name} ({group.code})</option>)}
                </select>
                <p className="text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorGroupHint")}</p>
              </div>
            </div>

            <section className="space-y-3 rounded-xl border border-slate-200 p-4 dark:border-slate-800">
              <div>
                <h3 className="text-sm font-semibold text-slate-900 dark:text-white">{t("adminModelMonitorModelScope")}</h3>
                <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorModelScopeHint")}</p>
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                <ScopeButton active={form.selection_mode === "all"} title={t("adminModelMonitorAllModels")} description={t("adminModelMonitorAllModelsHint")} onClick={() => setSelectionMode("all")} disabled={busy} />
                <ScopeButton active={form.selection_mode === "selected"} title={t("adminModelMonitorSelectedModels")} description={t("adminModelMonitorSelectedModelsHint")} onClick={() => setSelectionMode("selected")} disabled={busy} />
              </div>
              {form.selection_mode === "selected" ? (
                <div className="grid gap-2 sm:grid-cols-2">
                  {availableModels.length === 0 ? <p className="text-xs text-amber-600 dark:text-amber-300">{t("adminModelMonitorNoModels")}</p> : availableModels.map((model) => {
                    const selected = form.model_names.includes(model);
                    return <label key={model} className={cn("flex cursor-pointer items-center gap-2 rounded-lg border px-3 py-2 text-sm", selected ? "border-indigo-500/50 bg-indigo-50 text-indigo-700 dark:border-indigo-400/50 dark:bg-indigo-500/10 dark:text-indigo-200" : "border-slate-200 text-slate-700 dark:border-slate-800 dark:text-slate-300")}>
                      <input type="checkbox" checked={selected} onChange={() => toggleModel(model)} disabled={busy} className="h-4 w-4 accent-indigo-600" />
                      <span className="truncate font-mono text-xs">{model}</span>
                      {selected ? <Check className="ml-auto h-4 w-4 shrink-0" /> : null}
                    </label>;
                  })}
                </div>
              ) : null}
              <div className="space-y-2">
                <Label htmlFor="model-monitor-primary-model">{t("adminModelMonitorPrimaryModel")}</Label>
                <select
                  id="model-monitor-primary-model"
                  value={form.primary_model}
                  onChange={(event) => setForm((current) => ({ ...current, primary_model: event.target.value }))}
                  disabled={busy || availableModels.length === 0}
                  className={cn(selectClass, "w-full")}
                >
                  <option value="">{t("adminModelMonitorPrimaryModelAuto")}</option>
                  {availableModels.map((model) => <option key={model} value={model}>{model}</option>)}
                </select>
                <p className="text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorPrimaryModelHint")}</p>
              </div>
            </section>

            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="model-monitor-mode">{t("adminModelMonitorMode")}</Label>
                <select id="model-monitor-mode" value={form.mode} onChange={(event) => setForm((current) => ({ ...current, mode: event.target.value === "active" ? "active" : "passive" }))} disabled={busy} className={cn(selectClass, "w-full")}>
                  <option value="passive">{t("adminModelMonitorPassive")}</option>
                  <option value="active">{t("adminModelMonitorActive")}</option>
                </select>
              </div>
              {form.mode === "active" ? (
                <div className="space-y-2">
                  <Label htmlFor="model-monitor-interval">{t("adminModelMonitorInterval")}</Label>
                  <div className="flex items-center gap-2">
                    <Input id="model-monitor-interval" type="number" min={1} max={1440} value={Math.max(1, Math.round(form.probe_interval_seconds / 60))} onChange={(event) => setForm((current) => ({ ...current, probe_interval_seconds: Math.max(60, (Number(event.target.value) || 1) * 60) }))} disabled={busy} />
                    <span className="shrink-0 text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorMinutes")}</span>
                  </div>
                  <p className="text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorActiveHint")}</p>
                </div>
              ) : null}
              <div className="space-y-2">
                <Label htmlFor="model-monitor-recent-request-limit">{t("adminModelMonitorRecentRequests")}</Label>
                <select id="model-monitor-recent-request-limit" value={String(form.recent_request_limit || 60)} onChange={(event) => setForm((current) => ({ ...current, recent_request_limit: Number(event.target.value) || 60 }))} disabled={busy} className={cn(selectClass, "w-full")}>
                  <option value="30">30</option>
                  <option value="60">60</option>
                  <option value="120">120</option>
                </select>
                <p className="text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorRecentRequestsHint")}</p>
              </div>
            </div>
            <label className="flex items-center gap-3 rounded-xl border border-slate-200 px-4 py-3 text-sm dark:border-slate-800">
              <input type="checkbox" checked={form.enabled} onChange={(event) => setForm((current) => ({ ...current, enabled: event.target.checked }))} disabled={busy} className="h-4 w-4 accent-indigo-600" />
              <span><span className="block font-semibold text-slate-900 dark:text-white">{t("adminModelMonitorEnabled")}</span><span className="mt-1 block text-xs text-slate-500 dark:text-slate-400">{t("adminModelMonitorEnabledHint")}</span></span>
            </label>
          </form>
        </div>
        <div className="flex flex-col-reverse gap-2 border-t border-slate-200 px-5 py-4 dark:border-slate-800/80 sm:flex-row sm:justify-end sm:px-6">
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t("adminModelMonitorCancel")}</Button>
          <Button type="submit" form="model-monitor-form" disabled={busy || !form.group_id} className="gap-2"><Save className="h-4 w-4" />{busy ? t("adminModelMonitorSaving") : t("adminModelMonitorSave")}</Button>
        </div>
      </div>
    </div>
  );
}

function ScopeButton({ active, title, description, onClick, disabled }: { active: boolean; title: string; description: string; onClick: () => void; disabled: boolean }) {
  return <button type="button" onClick={onClick} disabled={disabled} className={cn("rounded-xl border p-3 text-left transition-colors", active ? "border-indigo-500/60 bg-indigo-50 dark:border-indigo-400/50 dark:bg-indigo-500/10" : "border-slate-200 hover:border-indigo-300 dark:border-slate-800 dark:hover:border-indigo-500/50")}>
    <span className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-white">{active ? <CheckCircle2 className="h-4 w-4 text-indigo-600 dark:text-indigo-300" /> : <span className="h-4 w-4 rounded-full border border-slate-300 dark:border-slate-700" />}{title}</span>
    <span className="mt-1 block text-xs text-slate-500 dark:text-slate-400">{description}</span>
  </button>;
}

function GroupStatusCard({ group, language, t }: { group: ModelStatusReport["groups"][number]; language: Language; t: (key: TranslationKey) => string }) {
  return <div className="overflow-hidden rounded-xl border border-slate-200 dark:border-slate-800">
    <div className="flex flex-col gap-3 border-b border-slate-200 bg-slate-50/80 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/50 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2"><span className="font-semibold text-slate-900 dark:text-white">{group.group_name}</span><code className="rounded bg-slate-200/70 px-1.5 py-0.5 font-mono text-[10px] text-slate-600 dark:bg-slate-800 dark:text-slate-400">{group.group_code}</code><StatusBadge status={group.status} t={t} /></div>
        <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-slate-500 dark:text-slate-400">
          <span>{t("adminModelStatusMonitor")}: {group.monitor_name || "-"}</span>
          <span>{group.monitor_mode === "active" ? t("adminModelMonitorActive") : t("adminModelMonitorPassive")}</span>
          <span>{group.selection_mode === "all" ? t("adminModelMonitorAllModels") : t("adminModelMonitorSelectedModels")}</span>
          <span>{t("adminModelStatusMultiplier")} x{group.multiplier}</span>
          <span>{t("adminModelStatusGroupModels")} {group.models.length}</span>
          <span>{t("adminModelMonitorRecentRequests")} {group.recent_request_limit || 60}</span>
          <span>{group.rpm_limit ? `RPM ${group.rpm_limit}` : "RPM -"}</span>
        </div>
        {group.monitor_mode === "active" ? <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-400 dark:text-slate-500"><span>{t("adminModelMonitorLastRun")}: {isProbeRunning(group) ? t("adminModelMonitorRunning") : formatTime(group.last_probe_finished_at, language)}</span><span>{isProbeRunning(group) ? t("adminModelMonitorRunning") : probeStatusLabel(group.last_probe_status, t)}</span><span>{t("adminModelStatusProbeNote")}</span></div> : null}
      </div>
      <div className="text-xs text-slate-500 dark:text-slate-400">{group.group_status === "active" ? t("adminModelStatusGroupActive") : t("adminModelStatusGroupDisabled")}</div>
    </div>
    <div className="overflow-x-auto"><table className="w-full min-w-[980px] text-sm"><thead className="border-b border-slate-200 text-left text-[11px] text-slate-500 dark:border-slate-800 dark:text-slate-400"><tr><th className="px-4 py-3">{t("adminModelStatusModel")}</th><th className="px-4 py-3">{t("adminModelStatusState")}</th><th className="px-4 py-3">{t("adminModelStatusRoutes")}</th><th className="px-4 py-3">{t("adminModelStatusLatency")}</th><th className="px-4 py-3">{t("adminModelStatusAvailability")}</th><th className="px-4 py-3">{t("adminModelStatusRecentRequests")}</th><th className="px-4 py-3">{t("adminModelStatusLastRequest")}</th></tr></thead><tbody className="divide-y divide-slate-100 dark:divide-slate-800/80">{group.models.map((model) => <ModelStatusRow key={`${group.group_id}:${model.provider}:${model.model}`} model={model} language={language} t={t} />)}</tbody></table></div>
  </div>;
}

function ModelStatusRow({ model, language, t }: { model: ModelStatus; language: Language; t: (key: TranslationKey) => string }) {
  const observationAt = latestHealthObservationAt(model);
  return <tr className="align-top"><td className="px-4 py-3"><div className="font-semibold text-slate-900 dark:text-white">{model.model}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{model.provider}</div></td><td className="px-4 py-3"><StatusBadge status={model.status} t={t} /><div className="mt-1 text-[10px] text-slate-500 dark:text-slate-400">{model.consecutive_failures} {t("adminModelStatusConsecutiveFailures")}</div>{observationAt ? <div className="mt-1 whitespace-nowrap text-[10px] text-slate-400 dark:text-slate-500">{t("adminModelStatusLastObservation")}: {formatTime(observationAt, language)}</div> : null}</td><td className="px-4 py-3 font-mono text-xs text-slate-700 dark:text-slate-300"><div>{model.available_routes} / {model.total_routes}</div><div className="mt-2 h-1.5 w-24 overflow-hidden rounded-full bg-slate-200 dark:bg-slate-800"><div className={cn("h-full rounded-full", model.available_routes === 0 ? "bg-rose-500" : model.available_routes < model.total_routes ? "bg-amber-500" : "bg-emerald-500")} style={{ width: `${model.total_routes > 0 ? Math.min(100, model.available_routes / model.total_routes * 100) : 0}%` }} /></div></td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-700 dark:text-slate-300">{model.last_latency_ms > 0 ? `${model.last_latency_ms} ms` : "-"}</td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-700 dark:text-slate-300">{model.request_count_7d > 0 ? `${model.availability_7d.toFixed(2)}%` : t("adminModelStatusNoData")}</td><td className="px-4 py-3"><RequestStrip statuses={model.recent_statuses || []} t={t} /></td><td className="whitespace-nowrap px-4 py-3 text-xs text-slate-500 dark:text-slate-400"><div>{statusLabel(model.last_request_status, t)}</div><div className="mt-1">{formatTime(model.last_request_at, language)}</div>{model.last_failure_reason ? <div className="mt-1 max-w-[220px] truncate text-rose-600 dark:text-rose-300" title={model.last_failure_reason}>{model.last_failure_reason}</div> : null}</td></tr>;
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

function latestHealthObservationAt(model: ModelStatus) {
  const successAt = model.last_success_at ? new Date(model.last_success_at) : null;
  const failureAt = model.last_failure_at ? new Date(model.last_failure_at) : null;
  const successTime = successAt && Number.isFinite(successAt.getTime()) ? successAt.getTime() : 0;
  const failureTime = failureAt && Number.isFinite(failureAt.getTime()) ? failureAt.getTime() : 0;
  if (!successTime && !failureTime) return undefined;
  return new Date(Math.max(successTime, failureTime)).toISOString();
}

function probeStatusLabel(status: string | undefined, t: (key: TranslationKey) => string) {
  if (status === "success") return t("adminModelMonitorProbeSuccess");
  if (status === "failed") return t("adminModelMonitorProbeFailed");
  if (status === "skipped") return t("adminModelMonitorProbeSkipped");
  return t("adminModelMonitorNotRun");
}

function isProbeRunning(value: { last_probe_started_at?: string; last_probe_finished_at?: string }) {
  if (!value.last_probe_started_at) return false;
  if (!value.last_probe_finished_at) return true;
  const started = new Date(value.last_probe_started_at).getTime();
  const finished = new Date(value.last_probe_finished_at).getTime();
  return Number.isFinite(started) && Number.isFinite(finished) && started > finished;
}

function formatTime(value: string | undefined, language: Language) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}
