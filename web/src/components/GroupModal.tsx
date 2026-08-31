import React from "react";
import { Check, Layers, Save, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ChannelSummary, GroupFormState, Language, LoginMessage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface GroupModalProps {
  open: boolean;
  form: GroupFormState;
  setForm: React.Dispatch<React.SetStateAction<GroupFormState>>;
  channels: ChannelSummary[];
  language: Language;
  busy: boolean;
  message: LoginMessage;
  onClose: () => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function GroupModal({ open, form, setForm, channels, language, busy, message, onClose, onSubmit }: GroupModalProps) {
  if (!open) {
    return null;
  }

  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;

  function toggleChannel(channelID: string) {
    setForm((current) => ({
      ...current,
      channel_ids: current.channel_ids.includes(channelID)
        ? current.channel_ids.filter((id) => id !== channelID)
        : [...current.channel_ids, channelID],
    }));
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/60 p-3 backdrop-blur-sm sm:p-6">
      <button type="button" aria-label={t("channelsCancel")} className="absolute inset-0 h-full w-full cursor-default" onClick={onClose} disabled={busy} />
      <div role="dialog" aria-modal="true" aria-labelledby="group-dialog-title" className="relative z-10 flex max-h-[92vh] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white text-slate-900 shadow-2xl dark:border-slate-700/80 dark:bg-slate-900 dark:text-slate-100">
        <div className="flex items-center justify-between gap-4 border-b border-slate-200 bg-slate-50/80 px-5 py-4 dark:border-slate-800/80 dark:bg-slate-950/60 sm:px-6">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-600 text-white shadow-md shadow-indigo-600/25">
              <Layers className="h-5 w-5" />
            </div>
            <div>
              <h2 id="group-dialog-title" className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">
                {form.id ? t("groupsEditTitle") : t("groupsCreateTitle")}
              </h2>
              <p className="text-xs text-slate-500 dark:text-slate-400">{t("groupsModalHint")}</p>
            </div>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={onClose} disabled={busy} title={t("channelsCancel")}>
            <X className="h-4 w-4" />
          </Button>
        </div>

        <div className="min-h-0 overflow-y-auto px-5 py-5 sm:px-6">
          <form id="group-form" className="space-y-6" onSubmit={onSubmit}>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="group-code">{t("groupsCode")}</Label>
                <Input id="group-code" value={form.code} onChange={(event) => setForm((current) => ({ ...current, code: event.target.value }))} placeholder="standard" disabled={Boolean(form.id) || busy} />
                <p className="text-xs text-slate-500 dark:text-slate-400">a-z, 0-9, - and _</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="group-name">{t("groupsName")}</Label>
                <Input id="group-name" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder={t("groupsName")} disabled={busy} />
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="group-description">{t("groupsDescriptionField")}</Label>
                <Input id="group-description" value={form.description} onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))} placeholder={t("groupsDescriptionField")} disabled={busy} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="group-multiplier">{t("groupsMultiplier")}</Label>
                <Input id="group-multiplier" inputMode="decimal" value={form.multiplier} onChange={(event) => setForm((current) => ({ ...current, multiplier: event.target.value }))} placeholder="1.000000" disabled={busy} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="group-rpm">{t("groupsRPM")}</Label>
                <Input id="group-rpm" type="number" min={0} max={10000000} value={form.rpm_limit} onChange={(event) => setForm((current) => ({ ...current, rpm_limit: Number(event.target.value) }))} disabled={busy} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="group-priority">{t("groupsPriority")}</Label>
                <Input id="group-priority" type="number" min={0} max={10000} value={form.priority} onChange={(event) => setForm((current) => ({ ...current, priority: Number(event.target.value) }))} disabled={busy} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="group-status">{t("groupsStatus")}</Label>
                <select id="group-status" value={form.status} onChange={(event) => setForm((current) => ({ ...current, status: event.target.value === "disabled" ? "disabled" : "active" }))} disabled={busy} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 ring-offset-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500/30 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:ring-offset-slate-900">
                  <option value="active">{t("groupsStatusActive")}</option>
                  <option value="disabled">{t("groupsStatusDisabled")}</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="group-billing-type">{t("groupsBillingType")}</Label>
                <select id="group-billing-type" value={form.billing_type} onChange={(event) => setForm((current) => ({ ...current, billing_type: event.target.value === "free" ? "free" : "prepaid" }))} disabled={busy} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 ring-offset-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500/30 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:ring-offset-slate-900">
                  <option value="prepaid">{t("groupsBillingPrepaid")}</option>
                  <option value="free">{t("groupsBillingFree")}</option>
                </select>
              </div>
            </div>

            <section className="space-y-3" aria-labelledby="group-channels-title">
              <div>
                <h3 id="group-channels-title" className="text-sm font-bold text-slate-900 dark:text-white">{t("groupsChannelSelector")}</h3>
                <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("groupsChannelSelectorHint")}</p>
              </div>
              {channels.length === 0 ? (
                <div className="rounded-xl border border-dashed border-slate-300 bg-slate-50 px-4 py-6 text-center text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-400">{t("groupsNoChannels")}</div>
              ) : (
                <div className="grid gap-2 md:grid-cols-2">
                  {channels.map((channel) => {
                    const selected = form.channel_ids.includes(channel.id);
                    return (
                      <label key={channel.id} className={cn("flex cursor-pointer items-center gap-3 rounded-xl border p-3 transition-colors", selected ? "border-indigo-500/50 bg-indigo-50 dark:border-indigo-400/50 dark:bg-indigo-500/10" : "border-slate-200 bg-white hover:border-indigo-300 dark:border-slate-800 dark:bg-slate-950/30 dark:hover:border-indigo-500/50")}>
                        <input type="checkbox" checked={selected} onChange={() => toggleChannel(channel.id)} disabled={busy} className="h-4 w-4 accent-indigo-600" />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-semibold text-slate-900 dark:text-white">{channel.name}</span>
                          <span className="mt-0.5 block truncate text-xs text-slate-500 dark:text-slate-400">{channel.provider.toUpperCase()} · {channel.base_url}</span>
                        </span>
                        {selected ? <Check className="h-4 w-4 shrink-0 text-indigo-600 dark:text-indigo-400" /> : null}
                      </label>
                    );
                  })}
                </div>
              )}
            </section>

            {message.text ? <p className={cn("text-sm", message.kind === "error" ? "text-rose-600 dark:text-rose-400" : message.kind === "success" ? "text-emerald-600 dark:text-emerald-400" : "text-amber-600 dark:text-amber-400")}>{message.text}</p> : null}
          </form>
        </div>

        <div className="flex flex-col-reverse gap-2 border-t border-slate-200 px-5 py-4 dark:border-slate-800/80 sm:flex-row sm:justify-end sm:px-6">
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t("channelsCancel")}</Button>
          <Button type="submit" form="group-form" disabled={busy}>
            <Save className="h-4 w-4" />
            {busy ? t("groupsSavePending") : t("groupsSave")}
          </Button>
        </div>
      </div>
    </div>
  );
}
