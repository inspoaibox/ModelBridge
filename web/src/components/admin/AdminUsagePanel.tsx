import React from "react";
import { Activity, ChevronLeft, ChevronRight, Clock3, Database, RefreshCw, Search } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Language, LoginMessage, UsageReport, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface AdminUsagePanelProps {
  language: Language;
  report: UsageReport | null;
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean, offset?: number) => Promise<void>;
  search: string;
  setSearch: (value: string) => void;
  status: string;
  setStatus: (value: string) => void;
  tenant: string;
  setTenant: (value: string) => void;
  model: string;
  setModel: (value: string) => void;
  group: string;
  setGroup: (value: string) => void;
  from: string;
  setFrom: (value: string) => void;
  to: string;
  setTo: (value: string) => void;
  offset: number;
}

export function AdminUsagePanel({ language, report, busy, message, refresh, search, setSearch, status, setStatus, tenant, setTenant, model, setModel, group, setGroup, from, setFrom, to, setTo, offset }: AdminUsagePanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const summary = report?.summary;
  const hasNext = Boolean(report && report.records.length >= report.limit && offset + report.records.length < report.summary.total_records);

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label={t("usageRecordsTotalRequests")} value={formatInteger(summary?.total_records || 0)} icon={Activity} tone="indigo" />
        <StatCard label={t("usageRecordsTotalTokens")} value={formatInteger(summary?.total_tokens || 0)} icon={Database} tone="cyan" />
        <StatCard label={t("usageRecordsTotalCost")} value={`${currencyLabel(summary?.total_cost, report?.records[0]?.currency)} ${formatDecimal(summary?.total_cost)}`} icon={Clock3} tone="emerald" />
        <StatCard label={t("usageRecordsPageRange")} value={report ? `${report.records.length} / ${report.summary.total_records}` : "-"} icon={Activity} tone="amber" />
      </div>

      <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
        <CardHeader className="flex flex-col gap-4 border-b border-slate-200/80 pb-4 dark:border-slate-800/80 lg:flex-row lg:items-end lg:justify-between">
          <div><CardTitle className="flex items-center gap-2 text-xl text-slate-950 dark:text-white"><Activity className="h-5 w-5 text-indigo-600" />{t("usageRecordsTitle")}</CardTitle><CardDescription>{t("usageRecordsDescription")}</CardDescription></div>
          <div className="grid w-full gap-2 sm:grid-cols-2 lg:grid-cols-4 xl:grid-cols-8">
            <div className="relative sm:col-span-2"><Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("usageRecordsSearchPlaceholder")} className="h-9 pl-9" /></div>
            <Input value={tenant} onChange={(event) => setTenant(event.target.value)} placeholder={t("usageRecordsTenantPlaceholder")} className="h-9 font-mono text-xs" />
            <Input value={model} onChange={(event) => setModel(event.target.value)} placeholder={t("usageRecordsModelPlaceholder")} className="h-9 text-xs" />
            <Input value={group} onChange={(event) => setGroup(event.target.value)} placeholder={t("usageRecordsGroupPlaceholder")} className="h-9 font-mono text-xs" />
            <Input type="date" value={from} onChange={(event) => setFrom(event.target.value)} className="h-9 text-xs" />
            <Input type="date" value={to} onChange={(event) => setTo(event.target.value)} className="h-9 text-xs" />
            <select value={status} onChange={(event) => setStatus(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"><option value="">{t("usageRecordsAllStatus")}</option><option value="settled">{t("usageRecordsStatusSettled")}</option><option value="failed">{t("usageRecordsStatusFailed")}</option><option value="started">{t("usageRecordsStatusStarted")}</option><option value="settlement_pending">{t("usageRecordsStatusPending")}</option></select>
            <Button type="button" variant="outline" size="icon" onClick={() => void refresh(true, 0)} disabled={busy} title={t("usageRecordsRefresh")} aria-label={t("usageRecordsRefresh")}><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} /></Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4 p-0">
          {message.text ? <div className="mx-5 mt-5 rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{message.text}</div> : null}
          <div className="overflow-x-auto">
            <Table className="min-w-[1320px]">
              <TableHeader><TableRow><TableHead>{t("usageRecordsAPIKey")}</TableHead><TableHead>{t("usageRecordsModel")}</TableHead><TableHead>{t("usageRecordsReasoning")}</TableHead><TableHead>{t("usageRecordsEndpoint")}</TableHead><TableHead>{t("usageRecordsIP")}</TableHead><TableHead>{t("usageRecordsGroup")}</TableHead><TableHead>{t("usageRecordsType")}</TableHead><TableHead>{t("usageRecordsBillingMode")}</TableHead><TableHead>{t("usageRecordsTokens")}</TableHead><TableHead>{t("usageRecordsCost")}</TableHead><TableHead>{t("usageRecordsLatency")}</TableHead><TableHead>{t("usageRecordsTime")}</TableHead></TableRow></TableHeader>
              <TableBody>
                {busy && !report ? <TableRow><TableCell colSpan={12} className="py-16 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-indigo-600" />{t("usageRecordsLoading")}</TableCell></TableRow> : !report || report.records.length === 0 ? <TableRow><TableCell colSpan={12} className="py-16 text-center text-sm text-slate-500 dark:text-slate-400">{t("usageRecordsEmpty")}</TableCell></TableRow> : report.records.map((record) => <TableRow key={record.id}>
                  <TableCell><div className="max-w-[180px] truncate font-semibold text-slate-900 dark:text-white">{record.token_name || t("usageRecordsUnnamedToken")}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{record.token_prefix || "-"}...</div></TableCell>
                  <TableCell><div className="font-semibold text-slate-900 dark:text-white">{record.model}</div><div className="mt-1 text-[10px] uppercase text-slate-500 dark:text-slate-400">{record.provider}</div></TableCell>
                  <TableCell className="text-xs text-slate-600 dark:text-slate-300">{record.reasoning_effort || "-"}</TableCell>
                  <TableCell className="max-w-[170px] truncate font-mono text-xs text-slate-600 dark:text-slate-300" title={record.endpoint}>{record.endpoint || "-"}</TableCell>
                  <TableCell className="font-mono text-xs text-slate-600 dark:text-slate-300">{record.client_ip || "-"}</TableCell>
                  <TableCell><Badge variant="outline">{record.group_name || record.group_code || t("usageRecordsDefaultGroup")}</Badge></TableCell>
                  <TableCell><Badge variant="secondary">{record.request_type === "sync" ? t("usageRecordsSync") : record.request_type || "-"}</Badge></TableCell>
                  <TableCell><Badge variant={record.billing_type === "free" ? "success" : "cyan"}>{record.billing_type === "free" ? t("usageRecordsFree") : t("usageRecordsPrepaid")}</Badge></TableCell>
                  <TableCell><TokenBreakdown input={record.input_tokens} output={record.output_tokens} cached={record.cached_input_tokens} reasoning={record.reasoning_tokens} /><MeterBreakdown metrics={record.usage_metrics} /></TableCell>
                  <TableCell className="whitespace-nowrap font-mono text-xs font-semibold text-emerald-700 dark:text-emerald-300">{record.currency} {formatDecimal(record.cost)}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-slate-600 dark:text-slate-300">{record.latency_ms > 0 ? `${(record.latency_ms / 1000).toFixed(2)}s` : "-"}</TableCell>
                  <TableCell className="whitespace-nowrap text-xs text-slate-500 dark:text-slate-400">{formatDate(record.created_at, language)}</TableCell>
                </TableRow>)}
              </TableBody>
            </Table>
          </div>
          <div className="flex flex-col gap-3 border-t border-slate-200/80 px-5 py-4 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400 sm:flex-row sm:items-center sm:justify-between"><span>{report ? `${offset + 1}-${offset + report.records.length} / ${report.summary.total_records}` : "-"}</span><div className="flex gap-2"><Button type="button" variant="outline" size="sm" onClick={() => void refresh(true, Math.max(0, offset - 50))} disabled={busy || offset === 0} className="gap-1"><ChevronLeft className="h-3.5 w-3.5" />{t("usageRecordsPrevious")}</Button><Button type="button" variant="outline" size="sm" onClick={() => void refresh(true, offset + 50)} disabled={busy || !hasNext} className="gap-1">{t("usageRecordsNext")}<ChevronRight className="h-3.5 w-3.5" /></Button></div></div>
        </CardContent>
      </Card>
    </div>
  );
}

function StatCard({ label, value, icon: Icon, tone }: { label: string; value: string; icon: typeof Activity; tone: "indigo" | "cyan" | "emerald" | "amber" }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardContent className="flex items-center justify-between p-4"><div><div className="text-xs font-medium text-slate-500 dark:text-slate-400">{label}</div><div className="mt-2 font-mono text-xl font-bold text-slate-950 dark:text-white">{value}</div></div><div className={cn("flex h-10 w-10 items-center justify-center rounded-xl", tone === "cyan" ? "bg-cyan-500/10 text-cyan-600 dark:text-cyan-300" : tone === "emerald" ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300" : tone === "amber" ? "bg-amber-500/10 text-amber-600 dark:text-amber-300" : "bg-indigo-500/10 text-indigo-600 dark:text-indigo-300")}><Icon className="h-5 w-5" /></div></CardContent></Card>;
}

function TokenBreakdown({ input, output, cached, reasoning }: { input: number; output: number; cached: number; reasoning: number }) {
  return <div className="space-y-1 whitespace-nowrap font-mono text-[11px]"><div><span className="text-cyan-600">↓</span> {formatInteger(input)} <span className="ml-2 text-indigo-600">↑</span> {formatInteger(output)}</div>{cached > 0 || reasoning > 0 ? <div className="text-[10px] text-slate-400">{cached > 0 ? `cache ${formatInteger(cached)}` : ""}{reasoning > 0 ? ` · reason ${formatInteger(reasoning)}` : ""}</div> : null}</div>;
}

function MeterBreakdown({ metrics }: { metrics?: Record<string, string> }) {
  const entries = Object.entries(metrics || {}).filter(([key, value]) => !["input_tokens", "output_tokens", "cached_input_tokens", "reasoning_tokens"].includes(key) && value !== "0");
  if (entries.length === 0) return null;
  return <div className="mt-1 max-w-[180px] truncate text-[10px] text-slate-400" title={entries.map(([key, value]) => `${key}: ${value}`).join(" · ")}>{entries.map(([key, value]) => `${key.replace(/_/g, " ")}: ${value}`).join(" · ")}</div>;
}

function formatInteger(value: number) {
  return new Intl.NumberFormat("en-US", { notation: value >= 10000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value || 0);
}

function formatDecimal(value?: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "0.000000";
  return new Intl.NumberFormat("en-US", { minimumFractionDigits: 6, maximumFractionDigits: 6 }).format(parsed);
}

function currencyLabel(value?: string, fallback?: string) {
  return fallback || (Number(value) === 0 ? "USD" : "USD");
}

function formatDate(value: string, language: Language) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}
