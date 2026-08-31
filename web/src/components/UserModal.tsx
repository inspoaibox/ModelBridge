import React from "react";
import { KeyRound, Save, ShieldCheck, UserPlus, UserRound, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TenantSummary, UserAdminFormState, UserTenantRole, Language, LoginMessage, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface UserModalProps {
  open: boolean;
  mode: "create" | "edit";
  form: UserAdminFormState;
  setForm: React.Dispatch<React.SetStateAction<UserAdminFormState>>;
  tenants: TenantSummary[];
  language: Language;
  busy: boolean;
  message: LoginMessage;
  onClose: () => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function UserModal({ open, mode, form, setForm, tenants, language, busy, message, onClose, onSubmit }: UserModalProps) {
  if (!open) return null;

  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const editing = mode === "edit";

  return (
    <div className="fixed inset-0 z-[75] flex items-center justify-center overflow-y-auto bg-slate-950/60 p-3 backdrop-blur-sm sm:p-6">
      <button type="button" aria-label={t("usersModalCancel")} className="absolute inset-0 h-full w-full cursor-default" onClick={onClose} disabled={busy} />
      <div role="dialog" aria-modal="true" aria-labelledby="user-dialog-title" className="relative z-10 flex max-h-[92vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white text-slate-900 shadow-2xl dark:border-slate-700/80 dark:bg-slate-900 dark:text-slate-100">
        <div className="flex items-center justify-between gap-4 border-b border-slate-200 bg-slate-50/80 px-5 py-4 dark:border-slate-800/80 dark:bg-slate-950/60 sm:px-6">
          <div className="flex items-center gap-3">
            <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-cyan-500 text-white shadow-md shadow-indigo-500/20">
              {editing ? <UserRound className="h-5 w-5" /> : <UserPlus className="h-5 w-5" />}
            </div>
            <div>
              <h2 id="user-dialog-title" className="text-lg font-bold tracking-tight text-slate-900 dark:text-white">{editing ? t("usersEditTitle") : t("usersCreateTitle")}</h2>
              <p className="text-xs text-slate-500 dark:text-slate-400">{editing ? t("usersEditHint") : t("usersCreateHint")}</p>
            </div>
          </div>
          <Button type="button" variant="ghost" size="icon" onClick={onClose} disabled={busy} title={t("usersModalCancel")}><X className="h-4 w-4" /></Button>
        </div>

        <div className="min-h-0 overflow-y-auto px-5 py-5 sm:px-6">
          <form id="user-form" className="space-y-6" onSubmit={onSubmit}>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="user-email">{t("usersEmail")}</Label>
                <Input id="user-email" type="email" autoComplete="off" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} placeholder="name@company.com" disabled={busy} required />
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="user-display-name">{t("usersDisplayName")}</Label>
                <Input id="user-display-name" value={form.display_name} onChange={(event) => setForm((current) => ({ ...current, display_name: event.target.value }))} placeholder={t("usersDisplayNamePlaceholder")} disabled={busy} required />
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="user-password">{t("usersPassword")}</Label>
                <div className="relative">
                  <KeyRound className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
                  <Input id="user-password" type="password" autoComplete={editing ? "new-password" : "new-password"} value={form.password} onChange={(event) => setForm((current) => ({ ...current, password: event.target.value }))} placeholder={editing ? t("usersPasswordKeep") : t("usersPasswordPlaceholder")} disabled={busy} required={!editing} className="pl-9" />
                </div>
                <p className="text-xs text-slate-500 dark:text-slate-400">{t("usersPasswordHint")}</p>
              </div>
            </div>

            {editing ? (
              <div className="flex items-start gap-3 rounded-xl border border-indigo-500/20 bg-indigo-50/70 p-4 text-xs text-indigo-800 dark:border-indigo-400/20 dark:bg-indigo-500/10 dark:text-indigo-200">
                <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{t("usersEditTenantHint")}</span>
              </div>
            ) : (
              <section className="space-y-4 rounded-xl border border-slate-200 bg-slate-50/70 p-4 dark:border-slate-800 dark:bg-slate-950/40">
                <div>
                  <h3 className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-white"><ShieldCheck className="h-4 w-4 text-indigo-600 dark:text-indigo-400" />{t("usersAccessTitle")}</h3>
                  <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("usersAccessHint")}</p>
                </div>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="user-tenant">{t("usersTenant")}</Label>
                    <select id="user-tenant" value={form.tenant_id} onChange={(event) => setForm((current) => ({ ...current, tenant_id: event.target.value }))} disabled={busy || tenants.length === 0} required className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus-visible:border-indigo-500 focus-visible:ring-2 focus-visible:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100">
                      <option value="">{tenants.length === 0 ? t("usersNoTenants") : t("usersSelectTenant")}</option>
                      {tenants.map((tenant) => <option key={tenant.id} value={tenant.id}>{tenant.name} ({tenant.slug})</option>)}
                    </select>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="user-tenant-role">{t("usersTenantRole")}</Label>
                    <select id="user-tenant-role" value={form.tenant_role} onChange={(event) => setForm((current) => ({ ...current, tenant_role: event.target.value as UserTenantRole }))} disabled={busy} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus-visible:border-indigo-500 focus-visible:ring-2 focus-visible:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100">
                      <option value="tenant_admin">{t("usersRoleTenantAdmin")}</option>
                      <option value="developer">{t("usersRoleDeveloper")}</option>
                      <option value="viewer">{t("usersRoleViewer")}</option>
                    </select>
                  </div>
                </div>
              </section>
            )}

            {message.text ? <p className={cn("text-sm", message.kind === "error" ? "text-rose-600 dark:text-rose-400" : message.kind === "success" ? "text-emerald-600 dark:text-emerald-400" : "text-amber-600 dark:text-amber-400")}>{message.text}</p> : null}
          </form>
        </div>

        <div className="flex flex-col-reverse gap-2 border-t border-slate-200 px-5 py-4 dark:border-slate-800/80 sm:flex-row sm:justify-end sm:px-6">
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t("usersModalCancel")}</Button>
          <Button type="submit" form="user-form" disabled={busy || (!editing && tenants.length === 0)}><Save className="h-4 w-4" />{busy ? t("usersSavePending") : t("usersSave")}</Button>
        </div>
      </div>
    </div>
  );
}
