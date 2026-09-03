import React, { useEffect, useState } from "react";
import { BadgeDollarSign, CheckCircle2, Copy, ExternalLink, QrCode, RefreshCw } from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { BillingAccount, Language, LoginMessage, PaymentOrder, PublicPaymentProvider, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn, formatDecimalWithoutTrailingZeros } from "@/lib/utils";

interface Props {
  language: Language;
  account: BillingAccount | null;
  providers: PublicPaymentProvider[];
  busy: boolean;
  message: LoginMessage;
  order: PaymentOrder | null;
  createOrder: (provider: PaymentOrder["provider"], amount: string, currency: string) => Promise<void>;
  refreshOrder: () => Promise<void>;
  capturePayPal: () => Promise<void>;
}

const providerLabels: Record<PaymentOrder["provider"], TranslationKey> = {
  wechat: "paymentWechat",
  alipay: "paymentAlipay",
  stripe: "paymentStripe",
  paypal: "paymentPayPal",
};

export function PaymentRechargePanel({ language, account, providers, busy, message, order, createOrder, refreshOrder, capturePayPal }: Props) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [provider, setProvider] = useState<PaymentOrder["provider"]>("wechat");
  const [amount, setAmount] = useState("10");
  const currency = account?.currency || "USD";

  useEffect(() => {
    const first = providers.find((item) => item.enabled)?.provider;
    if (first) setProvider(first);
  }, [providers]);

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await createOrder(provider, amount, currency);
  }

  const activeProviders = providers.filter((item) => item.enabled && (item.provider !== "wechat" && item.provider !== "alipay" || currency === "CNY"));
  const canCreate = activeProviders.length > 0 && Boolean(account) && !busy;
  const qrValue = order?.qr_code || "";

  return (
    <Card className="border-emerald-500/20 shadow-sm dark:bg-slate-900/60">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg"><BadgeDollarSign className="h-5 w-5 text-emerald-600" />{t("rechargeTitle")}</CardTitle>
        <CardDescription>{t("rechargeDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {message.text ? <div className={cn("rounded-xl border p-3 text-sm", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300")}>{message.text}</div> : null}
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-4"><div className="text-xs text-slate-500">{t("consoleBillingBalance")}</div><div className="mt-2 text-2xl font-bold text-emerald-700 dark:text-emerald-300">{account ? `${account.currency} ${formatMoney(account.balance)}` : "-"}</div></div>
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="text-xs text-slate-500">{t("rechargeProviders")}</div><div className="mt-2 text-2xl font-bold text-slate-900 dark:text-white">{activeProviders.length}</div></div>
          <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="text-xs text-slate-500">{t("rechargeCurrency")}</div><div className="mt-2 font-mono text-2xl font-bold text-slate-900 dark:text-white">{currency}</div></div>
        </div>
        {activeProviders.length === 0 ? <div className="rounded-xl border border-dashed border-slate-300 px-4 py-10 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">{t("rechargeUnavailable")}</div> : <form className="grid gap-4 rounded-xl border border-emerald-500/20 bg-emerald-50/40 p-4 dark:bg-emerald-500/5 sm:grid-cols-[minmax(0,1fr)_180px_auto] sm:items-end" onSubmit={(event) => void submit(event)}>
          <div className="space-y-2"><Label htmlFor="recharge-provider">{t("rechargeProvider")}</Label><select id="recharge-provider" value={provider} onChange={(event) => setProvider(event.target.value as PaymentOrder["provider"])} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm text-slate-900 outline-none dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100">{activeProviders.map((item) => <option key={item.provider} value={item.provider}>{t(providerLabels[item.provider])}</option>)}</select></div>
          <div className="space-y-2"><Label htmlFor="recharge-amount">{t("rechargeAmount")}</Label><Input id="recharge-amount" inputMode="decimal" min="0.01" value={amount} onChange={(event) => setAmount(event.target.value)} placeholder="10.00" required /></div>
          <Button type="submit" disabled={!canCreate} className="gap-2"><BadgeDollarSign className="h-4 w-4" />{busy ? t("rechargeCreating") : t("rechargeCreate")}</Button>
        </form>}
        {order ? <OrderStatus t={t} order={order} qrValue={qrValue} busy={busy} refreshOrder={refreshOrder} capturePayPal={capturePayPal} /> : null}
      </CardContent>
    </Card>
  );
}

function formatMoney(value?: string) {
  return formatDecimalWithoutTrailingZeros(value);
}

function OrderStatus({ t, order, qrValue, busy, refreshOrder, capturePayPal }: { t: (key: TranslationKey) => string; order: PaymentOrder; qrValue: string; busy: boolean; refreshOrder: () => Promise<void>; capturePayPal: () => Promise<void> }) {
  const statusLabel = order.status === "paid" ? t("rechargeStatusPaid") : order.status === "failed" ? t("rechargeStatusFailed") : order.status === "expired" ? t("rechargeStatusExpired") : t("rechargeStatusPending");
  const canOpen = Boolean(order.checkout_url);
  return <div className="rounded-xl border border-slate-200 p-4 dark:border-slate-800"><div className="flex flex-wrap items-center justify-between gap-3"><div><div className="text-sm font-semibold text-slate-900 dark:text-white">{t("rechargeOrderTitle")}</div><div className="mt-1 font-mono text-[11px] text-slate-500">{order.merchant_order_no}</div></div><Badge variant={order.status === "paid" ? "success" : order.status === "failed" || order.status === "expired" ? "destructive" : "warning"}>{statusLabel}</Badge></div><div className="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_220px]">{order.status === "pending" && qrValue ? <div className="flex flex-col items-center justify-center gap-3 rounded-xl bg-white p-4"><QrCode className="h-5 w-5 text-emerald-600" /><QRCodeSVG value={qrValue} size={170} includeMargin /><p className="text-center text-xs text-slate-500">{t("rechargeScanHint")}</p></div> : null}<div className="space-y-3"><div className="grid grid-cols-2 gap-3 text-sm"><Info label={t("rechargeAmount")} value={`${order.currency} ${formatMoney(order.amount)}`} /><Info label={t("rechargeProvider")} value={t(providerLabels[order.provider])} /></div>{order.failure_reason ? <div className="rounded-lg bg-rose-50 p-3 text-xs text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{order.failure_reason}</div> : null}<div className="flex flex-wrap gap-2">{canOpen ? <Button type="button" variant="outline" size="sm" className="gap-2" onClick={() => window.open(order.checkout_url, "_blank", "noopener,noreferrer")}><ExternalLink className="h-4 w-4" />{t("rechargeOpenPayment")}</Button> : null}{order.provider === "paypal" && order.status === "pending" ? <Button type="button" size="sm" onClick={() => void capturePayPal()} disabled={busy} className="gap-2"><CheckCircle2 className="h-4 w-4" />{t("rechargeConfirmPayPal")}</Button> : null}{order.status === "pending" ? <Button type="button" variant="ghost" size="icon" onClick={() => void refreshOrder()} disabled={busy} title={t("rechargeRefresh")} aria-label={t("rechargeRefresh")}><RefreshCw className={cn("h-4 w-4", busy ? "animate-spin" : "")} /></Button> : null}</div>{order.checkout_url ? <div className="flex items-center gap-2"><code className="min-w-0 flex-1 break-all text-[11px] text-slate-500">{order.checkout_url}</code><Button type="button" variant="outline" size="icon" onClick={() => void navigator.clipboard?.writeText(order.checkout_url || "")} title={t("rechargeCopyLink")} aria-label={t("rechargeCopyLink")}><Copy className="h-4 w-4" /></Button></div> : null}</div></div></div>;
}

function Info({ label, value }: { label: string; value: string }) { return <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-slate-800 dark:bg-slate-950/40"><div className="text-[11px] text-slate-500">{label}</div><div className="mt-1 text-sm font-semibold text-slate-900 dark:text-white">{value}</div></div>; }
