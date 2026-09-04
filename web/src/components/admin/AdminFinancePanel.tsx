import React from "react";
import { ArrowDownLeft, ArrowUpRight, CircleDollarSign, Receipt, RefreshCw, Search, WalletCards } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { FinanceReport, Language, LoginMessage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";
import { AdminFinanceRechargeOrders } from "@/components/admin/AdminFinanceRechargeOrders";

interface AdminFinancePanelProps {
  language: Language;
  report: FinanceReport | null;
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean, offset?: number) => Promise<void>;
  search: string;
  setSearch: (value: string) => void;
  currency: string;
  setCurrency: (value: string) => void;
  from: string;
  setFrom: (value: string) => void;
  to: string;
  setTo: (value: string) => void;
}

export function AdminFinancePanel({ language, report, busy, message, refresh, search, setSearch, currency, setCurrency, from, setFrom, to, setTo }: AdminFinancePanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const summary = report?.summaries || [];
  const accounts = report?.accounts || [];
  const transactions = report?.transactions || [];

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 rounded-2xl border border-emerald-500/20 bg-gradient-to-br from-emerald-500/10 via-cyan-500/5 to-white p-5 shadow-sm dark:from-emerald-500/10 dark:via-cyan-500/5 dark:to-slate-900/70 sm:flex-row sm:items-center sm:justify-between sm:p-6"><div><div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-emerald-700 dark:text-emerald-300"><CircleDollarSign className="h-4 w-4" />{t("financeEyebrow")}</div><h2 className="mt-2 text-2xl font-extrabold text-slate-950 dark:text-white">{t("financeTitle")}</h2><p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("financeDescription")}</p></div><Button type="button" variant="outline" onClick={() => void refresh(true, 0)} disabled={busy} className="gap-2 self-start sm:self-center"><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} />{t("financeRefresh")}</Button></div>
      {message.text ? <div className="rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{message.text}</div> : null}
      <div className="grid gap-2 rounded-xl border border-slate-200 bg-white/70 p-3 dark:border-slate-800 dark:bg-slate-900/60 sm:grid-cols-2 lg:grid-cols-5"><div className="relative sm:col-span-2"><Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("financeSearchPlaceholder")} className="h-9 w-full rounded-xl border border-slate-200 bg-white pl-9 pr-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" /></div><input value={currency} onChange={(event) => setCurrency(event.target.value)} placeholder="USD" className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs uppercase text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" /><input type="date" value={from} onChange={(event) => setFrom(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" /><input type="date" value={to} onChange={(event) => setTo(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" /></div>

      {summary.length === 0 ? <div className="grid gap-4 sm:grid-cols-3"><FinanceMetric label={t("financeCustomers")} value="0" icon={WalletCards} tone="indigo" /><FinanceMetric label={t("financeRemaining")} value="-" icon={CircleDollarSign} tone="emerald" /><FinanceMetric label={t("financeConsumed")} value="-" icon={ArrowDownLeft} tone="cyan" /></div> : <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">{summary.map((item) => <React.Fragment key={item.currency}><FinanceMetric label={`${t("financeRemaining")} · ${item.currency}`} value={`${item.currency} ${formatMoney(item.remaining_balance)}`} icon={WalletCards} tone="emerald" /><FinanceMetric label={`${t("financeConsumed")} · ${item.currency}`} value={`${item.currency} ${formatMoney(item.total_consumed)}`} icon={ArrowDownLeft} tone="cyan" /><FinanceMetric label={`${t("financeTopups")} · ${item.currency}`} value={`${item.currency} ${formatMoney(item.total_topups)}`} icon={ArrowUpLeftIcon} tone="indigo" /><FinanceMetric label={`${t("financeRequests")} · ${item.currency}`} value={formatInteger(item.request_count)} icon={Receipt} tone="amber" /></React.Fragment>)}</div>}

      <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white"><WalletCards className="h-5 w-5 text-emerald-600" />{t("financeAccountsTitle")}</CardTitle><CardDescription>{t("financeAccountsDescription")}</CardDescription></CardHeader><CardContent className="p-0"><div className="overflow-x-auto"><table className="w-full min-w-[900px] text-sm"><thead className="border-y border-slate-200 bg-slate-50/80 text-left text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-950/50 dark:text-slate-400"><tr><th className="px-5 py-3 font-semibold">{t("financeCustomer")}</th><th className="px-5 py-3 font-semibold">{t("financeCurrency")}</th><th className="px-5 py-3 font-semibold">{t("financeBalance")}</th><th className="px-5 py-3 font-semibold">{t("financeAccountConsumed")}</th><th className="px-5 py-3 font-semibold">{t("financeAccountTopups")}</th><th className="px-5 py-3 font-semibold">{t("financeRequests")}</th><th className="px-5 py-3 font-semibold">{t("financeLastUsage")}</th></tr></thead><tbody>{busy && !report ? <tr><td colSpan={7} className="py-14 text-center text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-emerald-600" />{t("financeLoading")}</td></tr> : accounts.length === 0 ? <tr><td colSpan={7} className="py-14 text-center text-slate-500 dark:text-slate-400">{t("financeAccountsEmpty")}</td></tr> : accounts.map((account) => <tr key={`${account.tenant_id}-${account.currency}`} className="border-b border-slate-100 last:border-0 dark:border-slate-800/80"><td className="px-5 py-4"><div className="font-semibold text-slate-900 dark:text-white">{account.tenant_name}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{account.tenant_slug} · {account.tenant_id}</div></td><td className="px-5 py-4 font-mono text-xs text-slate-600 dark:text-slate-300">{account.currency}</td><td className="px-5 py-4 font-mono text-sm font-bold text-emerald-700 dark:text-emerald-300">{account.currency} {formatMoney(account.balance)}</td><td className="px-5 py-4 font-mono text-xs text-slate-700 dark:text-slate-300">{formatMoney(account.total_consumed)}</td><td className="px-5 py-4 font-mono text-xs text-slate-700 dark:text-slate-300">{formatMoney(account.total_topups)}</td><td className="px-5 py-4 font-mono text-xs text-slate-700 dark:text-slate-300">{formatInteger(account.request_count)}</td><td className="whitespace-nowrap px-5 py-4 text-xs text-slate-500 dark:text-slate-400">{account.last_usage_at ? formatDate(account.last_usage_at, language) : "-"}</td></tr>)}</tbody></table></div><div className="border-t border-slate-200/80 px-5 py-3 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400">{report ? `${report.total_accounts} ${t("financeAccountsUnit")}` : "-"}</div></CardContent></Card>

      <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white"><Receipt className="h-5 w-5 text-indigo-600" />{t("financeTransactionsTitle")}</CardTitle><CardDescription>{t("financeTransactionsDescription")}</CardDescription></CardHeader><CardContent className="p-0"><div className="overflow-x-auto"><table className="w-full min-w-[960px] text-sm"><thead className="border-y border-slate-200 bg-slate-50/80 text-left text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-950/50 dark:text-slate-400"><tr><th className="px-5 py-3 font-semibold">{t("financeTransactionType")}</th><th className="px-5 py-3 font-semibold">{t("financeCustomer")}</th><th className="px-5 py-3 font-semibold">{t("financeTransactionModel")}</th><th className="px-5 py-3 font-semibold">{t("financeTransactionAmount")}</th><th className="px-5 py-3 font-semibold">{t("financeTransactionDescription")}</th><th className="px-5 py-3 font-semibold">{t("financeTransactionTime")}</th></tr></thead><tbody>{transactions.length === 0 ? <tr><td colSpan={6} className="py-14 text-center text-slate-500 dark:text-slate-400">{t("financeTransactionsEmpty")}</td></tr> : transactions.map((item) => <tr key={`${item.id}-${item.direction}`} className="border-b border-slate-100 last:border-0 dark:border-slate-800/80"><td className="px-5 py-4"><Badge variant={item.direction === "credit" ? "success" : "cyan"} className="gap-1.5">{item.direction === "credit" ? <ArrowUpLeftIcon className="h-3 w-3" /> : <ArrowDownLeft className="h-3 w-3" />}{item.transaction_type === "account_credit" ? t("financeTopup") : t("financeUsage")}</Badge></td><td className="px-5 py-4"><div className="font-semibold text-slate-900 dark:text-white">{item.tenant_name}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{item.tenant_id}</div></td><td className="px-5 py-4"><div className="font-medium text-slate-800 dark:text-slate-200">{item.model || "-"}</div><div className="mt-1 text-xs text-slate-500">{item.token_name || "-"}</div></td><td className={cn("whitespace-nowrap px-5 py-4 font-mono text-sm font-semibold", item.direction === "credit" ? "text-emerald-700 dark:text-emerald-300" : "text-cyan-700 dark:text-cyan-300")}>{item.direction === "credit" ? "+" : "-"}{item.currency} {formatMoney(item.amount)}</td><td className="max-w-[260px] truncate px-5 py-4 text-xs text-slate-600 dark:text-slate-300" title={item.description}>{item.description || "-"}</td><td className="whitespace-nowrap px-5 py-4 text-xs text-slate-500 dark:text-slate-400">{formatDate(item.created_at, language)}</td></tr>)}</tbody></table></div><div className="border-t border-slate-200/80 px-5 py-3 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400">{report ? `${report.total_transactions} ${t("financeTransactionsUnit")}` : "-"}</div></CardContent></Card>
      <AdminFinanceRechargeOrders language={language} report={report} busy={busy} />
    </div>
  );
}

function FinanceMetric({ label, value, icon: Icon, tone }: { label: string; value: string; icon: typeof CircleDollarSign; tone: "indigo" | "cyan" | "emerald" | "amber" }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardContent className="flex items-center justify-between p-4"><div><div className="text-xs font-medium text-slate-500 dark:text-slate-400">{label}</div><div className="mt-2 font-mono text-xl font-bold text-slate-950 dark:text-white">{value}</div></div><div className={cn("flex h-10 w-10 items-center justify-center rounded-xl", tone === "cyan" ? "bg-cyan-500/10 text-cyan-600 dark:text-cyan-300" : tone === "emerald" ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300" : tone === "amber" ? "bg-amber-500/10 text-amber-600 dark:text-amber-300" : "bg-indigo-500/10 text-indigo-600 dark:text-indigo-300")}><Icon className="h-5 w-5" /></div></CardContent></Card>;
}

function formatMoney(value?: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "0";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(parsed);
}

function formatInteger(value: number) {
  return new Intl.NumberFormat("en-US", { notation: value >= 10000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value || 0);
}

function formatDate(value: string, language: Language) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}

const ArrowUpLeftIcon = ArrowUpRight;
