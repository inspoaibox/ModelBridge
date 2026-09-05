import React, { useMemo, useState } from "react";
import { ArrowDownLeft, ArrowUpLeft, CircleDollarSign, ListTree, Receipt, RefreshCw, WalletCards } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { BillingAccount, CreditFormState, FinanceAccount, FinanceReport, Language, LoginMessage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";
import { AdminFinanceRechargeOrders } from "@/components/admin/AdminFinanceRechargeOrders";

interface AdminAccountFinancePanelProps {
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
  billingAccount: BillingAccount | null;
  billingMessage: LoginMessage;
  billingBusy: boolean;
  creditForm: CreditFormState;
  setCreditForm: React.Dispatch<React.SetStateAction<CreditFormState>>;
  creditBillingAccount: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  loadBillingAccount: () => Promise<void>;
  canReadBilling: boolean;
  canUpdateBilling: boolean;
}

type AccountAction = "recharge" | "adjust" | "reconcile";

export function AdminAccountFinancePanel({
  language,
  report,
  busy,
  message,
  refresh,
  search,
  setSearch,
  currency,
  setCurrency,
  from,
  setFrom,
  to,
  setTo,
  billingAccount,
  billingMessage,
  billingBusy,
  creditForm,
  setCreditForm,
  creditBillingAccount,
  loadBillingAccount,
  canReadBilling,
  canUpdateBilling,
}: AdminAccountFinancePanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [selectedTenantID, setSelectedTenantID] = useState("");
  const [selectedCurrency, setSelectedCurrency] = useState("");
  const [action, setAction] = useState<AccountAction | null>(null);
  const accounts = report?.accounts || [];
  const selectedAccount = useMemo(
    () => accounts.find((account) => account.tenant_id === selectedTenantID && (!selectedCurrency || account.currency === selectedCurrency)),
    [accounts, selectedCurrency, selectedTenantID]
  );
  const selectedOrders = report?.recharge_orders.filter((order) => !selectedTenantID || order.tenant_id === selectedTenantID) || [];
  const selectedTransactions = report?.transactions.filter((item) => (!selectedTenantID || item.tenant_id === selectedTenantID) && (!selectedCurrency || item.currency === selectedCurrency)) || [];

  function selectAccount(account: FinanceAccount, nextAction: AccountAction) {
    setSelectedTenantID(account.tenant_id);
    setSelectedCurrency(account.currency);
    setAction(nextAction);
    setCreditForm((current) => ({
      ...current,
      tenant_id: account.tenant_id,
      currency: account.currency,
      direction: nextAction === "recharge" ? "credit" : current.direction || "credit",
    }));
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 rounded-2xl border border-indigo-500/20 bg-gradient-to-br from-indigo-500/10 via-cyan-500/5 to-white p-5 shadow-sm dark:from-indigo-500/10 dark:via-cyan-500/5 dark:to-slate-900/70 sm:flex-row sm:items-center sm:justify-between sm:p-6">
        <div>
          <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.16em] text-indigo-700 dark:text-indigo-300"><WalletCards className="h-4 w-4" />{t("accountFinanceEyebrow")}</div>
          <h2 className="mt-2 text-2xl font-extrabold text-slate-950 dark:text-white">{t("accountFinanceTitle")}</h2>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-slate-600 dark:text-slate-400">{t("accountFinanceDescription")}</p>
        </div>
        <Button type="button" variant="outline" onClick={() => void refresh(true, 0)} disabled={busy} className="gap-2 self-start sm:self-center"><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} />{t("financeRefresh")}</Button>
      </div>

      {message.text ? <div className="rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-sm text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{message.text}</div> : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <AccountMetric label={t("financeCustomers")} value={String(report?.total_accounts || 0)} icon={WalletCards} tone="indigo" />
        <AccountMetric label={t("financeRemaining")} value={formatTotals(report?.summaries || [], "remaining_balance")} icon={CircleDollarSign} tone="emerald" />
        <AccountMetric label={t("financeTopups")} value={formatTotals(report?.summaries || [], "total_topups")} icon={ArrowUpLeft} tone="cyan" />
        <AccountMetric label={t("financeTransactionsTitle")} value={String(report?.total_transactions || 0)} icon={Receipt} tone="amber" />
      </div>

      <div className="grid gap-2 rounded-xl border border-slate-200 bg-white/70 p-3 dark:border-slate-800 dark:bg-slate-900/60 sm:grid-cols-2 lg:grid-cols-5">
        <div className="relative sm:col-span-2"><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("financeSearchPlaceholder")} className="h-9 w-full rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" /></div>
        <input value={currency} onChange={(event) => setCurrency(event.target.value)} placeholder="USD" className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs uppercase text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" />
        <input type="date" value={from} onChange={(event) => setFrom(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" />
        <input type="date" value={to} onChange={(event) => setTo(event.target.value)} className="h-9 rounded-xl border border-slate-200 bg-white px-3 text-xs text-slate-800 outline-none focus:border-indigo-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200" />
      </div>

      <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
        <CardHeader><CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white"><WalletCards className="h-5 w-5 text-indigo-600" />{t("accountFinanceAccountsTitle")}</CardTitle><CardDescription>{t("accountFinanceAccountsDescription")}</CardDescription></CardHeader>
        <CardContent className="p-0"><div className="overflow-x-auto"><Table className="min-w-[1120px]"><TableHeader><TableRow><TableHead>{t("financeCustomer")}</TableHead><TableHead>{t("financeCurrency")}</TableHead><TableHead>{t("financeBalance")}</TableHead><TableHead>{t("financeAccountConsumed")}</TableHead><TableHead>{t("financeAccountTopups")}</TableHead><TableHead>{t("financeRequests")}</TableHead><TableHead className="text-right">{t("accountFinanceActions")}</TableHead></TableRow></TableHeader><TableBody>
          {busy && !report ? <TableRow><TableCell colSpan={7} className="py-14 text-center text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-indigo-600" />{t("financeLoading")}</TableCell></TableRow> : accounts.length === 0 ? <TableRow><TableCell colSpan={7} className="py-14 text-center text-slate-500 dark:text-slate-400">{t("financeAccountsEmpty")}</TableCell></TableRow> : accounts.map((account) => {
            const active = selectedTenantID === account.tenant_id && selectedCurrency === account.currency;
            return <TableRow key={`${account.tenant_id}-${account.currency}`} className={cn("border-b border-slate-100 last:border-0 dark:border-slate-800/80", active && "bg-indigo-50/70 dark:bg-indigo-500/10")}>
              <TableCell><div className="font-semibold text-slate-900 dark:text-white">{account.tenant_name}</div><div className="mt-1 font-mono text-[10px] text-slate-500 dark:text-slate-400">{account.tenant_slug} · {account.tenant_id}</div></TableCell>
              <TableCell className="font-mono text-xs text-slate-600 dark:text-slate-300">{account.currency}</TableCell>
              <TableCell className="font-mono text-sm font-bold text-emerald-700 dark:text-emerald-300">{account.currency} {formatMoney(account.balance)}</TableCell>
              <TableCell className="font-mono text-xs text-slate-700 dark:text-slate-300">{formatMoney(account.total_consumed)}</TableCell>
              <TableCell className="font-mono text-xs text-slate-700 dark:text-slate-300">{formatMoney(account.total_topups)}</TableCell>
              <TableCell className="font-mono text-xs text-slate-700 dark:text-slate-300">{formatInteger(account.request_count)}</TableCell>
              <TableCell className="text-right"><div className="inline-flex flex-wrap justify-end gap-1"><Button type="button" variant={active && action === "recharge" ? "secondary" : "ghost"} size="sm" className="h-8 gap-1 px-2 text-xs" onClick={() => selectAccount(account, "recharge")}><Receipt className="h-3.5 w-3.5" />{t("accountFinanceRechargeRecords")}</Button><Button type="button" variant={active && action === "adjust" ? "secondary" : "ghost"} size="sm" className="h-8 gap-1 px-2 text-xs" onClick={() => selectAccount(account, "adjust")} disabled={!canUpdateBilling}><CircleDollarSign className="h-3.5 w-3.5" />{t("accountFinanceAdjustBalance")}</Button><Button type="button" variant={active && action === "reconcile" ? "secondary" : "ghost"} size="sm" className="h-8 gap-1 px-2 text-xs" onClick={() => selectAccount(account, "reconcile")}><ListTree className="h-3.5 w-3.5" />{t("accountFinanceReconciliation")}</Button></div></TableCell>
            </TableRow>;
          })}
        </TableBody></Table></div><div className="border-t border-slate-200/80 px-5 py-3 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400">{report ? `${report.total_accounts} ${t("financeAccountsUnit")}` : "-"}</div></CardContent>
      </Card>

      {selectedTenantID && action === "adjust" ? <Card className="border-amber-500/30 shadow-sm dark:border-amber-500/30 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white"><CircleDollarSign className="h-5 w-5 text-amber-600" />{t("accountFinanceAdjustTitle")}</CardTitle><CardDescription>{t("accountFinanceSelectedAccount")}: {selectedAccount?.tenant_name || selectedTenantID} · {selectedCurrency}</CardDescription></CardHeader><CardContent><form className="grid gap-4 lg:grid-cols-[1.1fr_0.7fr_0.7fr_1.4fr_auto] lg:items-end" onSubmit={creditBillingAccount}><div className="space-y-1.5"><Label htmlFor="account-finance-tenant" className="text-xs font-semibold">{t("billingTenantID")}</Label><Input id="account-finance-tenant" value={creditForm.tenant_id} onChange={(event) => setCreditForm((current) => ({ ...current, tenant_id: event.target.value }))} className="font-mono text-xs" required /></div><div className="space-y-1.5"><Label htmlFor="account-finance-direction" className="text-xs font-semibold">{t("accountFinanceDirection")}</Label><select id="account-finance-direction" value={creditForm.direction || "credit"} onChange={(event) => setCreditForm((current) => ({ ...current, direction: event.target.value as "credit" | "debit" }))} className="h-9 w-full rounded-xl border border-slate-200 bg-white px-3 text-xs dark:border-slate-700 dark:bg-slate-900"><option value="credit">{t("accountFinanceIncrease")}</option><option value="debit">{t("accountFinanceDecrease")}</option></select></div><div className="space-y-1.5"><Label htmlFor="account-finance-amount" className="text-xs font-semibold">{t("billingAmount")}</Label><Input id="account-finance-amount" inputMode="decimal" value={creditForm.amount} onChange={(event) => setCreditForm((current) => ({ ...current, amount: event.target.value }))} placeholder="0.00" required /></div><div className="space-y-1.5"><Label htmlFor="account-finance-reason" className="text-xs font-semibold">{t("billingReason")}</Label><Input id="account-finance-reason" value={creditForm.reason} onChange={(event) => setCreditForm((current) => ({ ...current, reason: event.target.value }))} placeholder={t("accountFinanceReasonPlaceholder")} /></div><div className="flex gap-2"><Button type="button" variant="outline" size="sm" onClick={() => void loadBillingAccount()} disabled={!canReadBilling || billingBusy} className="h-9 gap-1.5 text-xs"><RefreshCw className={cn("h-3.5 w-3.5", billingBusy && "animate-spin")} />{t("billingAccountLoad")}</Button><Button type="submit" size="sm" disabled={!canUpdateBilling || billingBusy} className="h-9 gap-1.5 text-xs"><CircleDollarSign className="h-3.5 w-3.5" />{billingBusy ? t("billingCrediting") : t("accountFinanceSubmitAdjustment")}</Button></div></form>{billingAccount && billingAccount.tenant_id === selectedTenantID && billingAccount.currency === selectedCurrency ? <div className="mt-4 rounded-xl border border-emerald-500/30 bg-emerald-50/80 p-4 dark:bg-emerald-950/20"><div className="flex items-center justify-between text-xs"><span className="font-semibold">{t("billingAccountBalance")}</span><Badge variant="success">{billingAccount.status}</Badge></div><div className="mt-1 font-mono text-2xl font-extrabold text-emerald-700 dark:text-emerald-400">{billingAccount.currency} {formatMoney(billingAccount.balance)}</div></div> : null}{billingMessage.text ? <div className={cn("mt-3 rounded-xl border p-3 text-xs", billingMessage.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300")}>{billingMessage.text}</div> : null}</CardContent></Card> : null}

      {selectedTenantID && action === "reconcile" ? <AccountTransactions language={language} transactions={selectedTransactions} title={t("accountFinanceDetailsTitle")} description={t("accountFinanceReconciliation")} /> : null}
      {selectedTenantID && action === "recharge" ? <AdminFinanceRechargeOrders language={language} report={report ? { ...report, recharge_orders: selectedOrders, total_recharge_orders: selectedOrders.length } : report} busy={busy} tenantID={selectedTenantID} currency={selectedCurrency} /> : null}
    </div>
  );
}

function AccountTransactions({ language, transactions, title, description }: { language: Language; transactions: FinanceReport["transactions"]; title: string; description: string }) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader><CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white"><ListTree className="h-5 w-5 text-indigo-600" />{title}</CardTitle><CardDescription>{description}</CardDescription></CardHeader><CardContent className="p-0"><div className="overflow-x-auto"><Table className="min-w-[900px]"><TableHeader><TableRow><TableHead>{t("financeTransactionType")}</TableHead><TableHead>{t("financeTransactionAmount")}</TableHead><TableHead>{t("financeTransactionDescription")}</TableHead><TableHead>{t("financeTransactionTime")}</TableHead></TableRow></TableHeader><TableBody>{transactions.length === 0 ? <TableRow><TableCell colSpan={4} className="py-14 text-center text-slate-500 dark:text-slate-400">{t("financeTransactionsEmpty")}</TableCell></TableRow> : transactions.map((item) => <TableRow key={`${item.id}-${item.direction}`}><TableCell><Badge variant={item.direction === "credit" ? "success" : "cyan"} className="gap-1.5">{item.direction === "credit" ? <ArrowUpLeft className="h-3 w-3" /> : <ArrowDownLeft className="h-3 w-3" />}{item.transaction_type === "account_credit" ? t("financeTopup") : t("financeUsage")}</Badge><div className="mt-1 text-xs text-slate-500">{item.model || item.reference_id || "-"}</div></TableCell><TableCell className={cn("font-mono text-sm font-semibold", item.direction === "credit" ? "text-emerald-700 dark:text-emerald-300" : "text-cyan-700 dark:text-cyan-300")}>{item.direction === "credit" ? "+" : "-"}{item.currency} {formatMoney(item.amount)}</TableCell><TableCell className="max-w-[360px] truncate text-xs text-slate-600 dark:text-slate-300" title={item.description}>{item.description || "-"}</TableCell><TableCell className="whitespace-nowrap text-xs text-slate-500 dark:text-slate-400">{formatDate(item.created_at, language)}</TableCell></TableRow>)}</TableBody></Table></div><div className="border-t border-slate-200/80 px-5 py-3 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400">{transactions.length} {t("financeTransactionsUnit")}</div></CardContent></Card>;
}

function AccountMetric({ label, value, icon: Icon, tone }: { label: string; value: string; icon: typeof CircleDollarSign; tone: "indigo" | "cyan" | "emerald" | "amber" }) {
  return <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardContent className="flex items-center justify-between p-4"><div><div className="text-xs font-medium text-slate-500 dark:text-slate-400">{label}</div><div className="mt-2 font-mono text-xl font-bold text-slate-950 dark:text-white">{value}</div></div><div className={cn("flex h-10 w-10 items-center justify-center rounded-xl", tone === "cyan" ? "bg-cyan-500/10 text-cyan-600 dark:text-cyan-300" : tone === "emerald" ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300" : tone === "amber" ? "bg-amber-500/10 text-amber-600 dark:text-amber-300" : "bg-indigo-500/10 text-indigo-600 dark:text-indigo-300")}><Icon className="h-5 w-5" /></div></CardContent></Card>;
}

function formatTotals(summaries: FinanceReport["summaries"], field: "remaining_balance" | "total_topups") {
  if (summaries.length === 0) return "-";
  return summaries.map((item) => `${item.currency} ${formatMoney(item[field])}`).join(" · ");
}

function formatMoney(value?: string) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(parsed) : "0";
}

function formatInteger(value: number) {
  return new Intl.NumberFormat("en-US", { notation: value >= 10000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value || 0);
}

function formatDate(value: string, language: Language) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "-" : new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}
