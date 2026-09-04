import React from "react";
import { RefreshCw, Receipt } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { FinanceReport, Language, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";

interface AdminFinanceRechargeOrdersProps {
  language: Language;
  report: FinanceReport | null;
  busy: boolean;
}

export function AdminFinanceRechargeOrders({ language, report, busy }: AdminFinanceRechargeOrdersProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const orders = report?.recharge_orders || [];

  return (
    <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg text-slate-950 dark:text-white">
          <Receipt className="h-5 w-5 text-emerald-600" />
          {t("financeRechargeOrdersTitle")}
          {report ? <Badge variant="secondary" className="text-[10px]">{report.total_recharge_orders}</Badge> : null}
        </CardTitle>
        <CardDescription>{t("financeRechargeOrdersDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1120px] text-sm">
            <thead className="border-y border-slate-200 bg-slate-50/80 text-left text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-950/50 dark:text-slate-400">
              <tr>
                <th className="px-5 py-3 font-semibold">{t("financeCustomer")}</th>
                <th className="px-5 py-3 font-semibold">{t("financeOrderNumber")}</th>
                <th className="px-5 py-3 font-semibold">{t("financePaymentProvider")}</th>
                <th className="px-5 py-3 font-semibold">{t("financeOrderAmount")}</th>
                <th className="px-5 py-3 font-semibold">{t("financeCreditedAmount")}</th>
                <th className="px-5 py-3 font-semibold">{t("financeOrderStatus")}</th>
                <th className="px-5 py-3 font-semibold">{t("financeTransactionTime")}</th>
              </tr>
            </thead>
            <tbody>
              {busy && !report ? (
                <tr><td colSpan={7} className="py-14 text-center text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin text-emerald-600" />{t("financeLoading")}</td></tr>
              ) : orders.length === 0 ? (
                <tr><td colSpan={7} className="py-14 text-center text-slate-500 dark:text-slate-400">{t("financeRechargeOrdersEmpty")}</td></tr>
              ) : orders.map((order) => (
                <tr key={order.id} className="border-b border-slate-100 last:border-0 dark:border-slate-800/80">
                  <td className="px-5 py-4">
                    <div className="font-semibold text-slate-900 dark:text-white">{order.tenant_name}</div>
                    <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">{order.user_email || order.tenant_slug}</div>
                  </td>
                  <td className="px-5 py-4">
                    <div className="font-mono text-xs text-slate-800 dark:text-slate-200">{order.merchant_order_no}</div>
                    {order.provider_order_id ? <div className="mt-1 max-w-[190px] truncate font-mono text-[10px] text-slate-500" title={order.provider_order_id}>{order.provider_order_id}</div> : null}
                  </td>
                  <td className="px-5 py-4 text-xs font-medium text-slate-700 dark:text-slate-300">{paymentProviderLabel(order.provider, t)}</td>
                  <td className="whitespace-nowrap px-5 py-4 font-mono text-sm font-semibold text-slate-800 dark:text-slate-200">{order.currency} {formatMoney(order.amount)}</td>
                  <td className="whitespace-nowrap px-5 py-4 font-mono text-sm font-semibold text-emerald-700 dark:text-emerald-300">{order.currency} {formatMoney(order.credited_amount)}<div className="mt-1 text-[10px] font-normal text-slate-500">x{formatMoney(order.recharge_rate)}</div></td>
                  <td className="px-5 py-4"><Badge variant={paymentOrderStatusVariant(order.status)}>{paymentOrderStatusLabel(order.status, t)}</Badge>{order.failure_reason ? <div className="mt-1 max-w-[180px] truncate text-[10px] text-rose-600 dark:text-rose-300" title={order.failure_reason}>{order.failure_reason}</div> : null}</td>
                  <td className="whitespace-nowrap px-5 py-4 text-xs text-slate-500 dark:text-slate-400">{formatDate(order.created_at, language)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="border-t border-slate-200/80 px-5 py-3 text-xs text-slate-500 dark:border-slate-800/80 dark:text-slate-400">{report ? `${report.total_recharge_orders} ${t("financeRechargeOrdersUnit")}` : "-"}</div>
      </CardContent>
    </Card>
  );
}

function paymentProviderLabel(provider: string, t: (key: TranslationKey) => string) {
  switch (provider) {
    case "stripe": return t("financePaymentStripe");
    case "paypal": return t("financePaymentPayPal");
    case "wechat": return t("financePaymentWechat");
    case "alipay": return t("financePaymentAlipay");
    default: return provider || "-";
  }
}

function paymentOrderStatusLabel(status: string, t: (key: TranslationKey) => string) {
  switch (status) {
    case "paid": return t("financeOrderPaid");
    case "pending": return t("financeOrderPending");
    case "failed": return t("financeOrderFailed");
    case "cancelled": return t("financeOrderCancelled");
    case "expired": return t("financeOrderExpired");
    default: return status || "-";
  }
}

function paymentOrderStatusVariant(status: string): "success" | "warning" | "destructive" | "muted" {
  switch (status) {
    case "paid": return "success";
    case "pending": return "warning";
    case "failed": return "destructive";
    case "cancelled":
    case "expired": return "muted";
    default: return "muted";
  }
}

function formatMoney(value?: string) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "0";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits: 3 }).format(parsed);
}

function formatDate(value: string, language: Language) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(language === "zh" ? "zh-CN" : "en-US", { dateStyle: "short", timeStyle: "medium" }).format(date);
}
