import React, { useState } from "react";
import { Check, Copy, KeyRound, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language, IssuedTokenResponse, LoginMessage, TokenCreateFormState, TokenGroupOption, TranslationKey } from "@/types";
import { translations } from "@/locales/translations";
import { formatDecimalWithoutTrailingZeros } from "@/lib/utils";

interface TokenCreateModalProps {
  open: boolean;
  mode: "admin" | "console";
  language: Language;
  form: TokenCreateFormState;
  setForm: React.Dispatch<React.SetStateAction<TokenCreateFormState>>;
  projectIDs: string[];
  groups: TokenGroupOption[];
  groupsBusy: boolean;
  busy: boolean;
  message: LoginMessage;
  issuedToken: IssuedTokenResponse | null;
  editing?: boolean;
  onClose: () => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
}

export function TokenCreateModal({
  open,
  mode,
  language,
  form,
  setForm,
  projectIDs,
  groups,
  groupsBusy,
  busy,
  message,
  issuedToken,
  editing = false,
  onClose,
  onSubmit,
}: TokenCreateModalProps) {
  const [copied, setCopied] = useState(false);
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const selectedGroup = groups.find((group) => group.id === form.group_id);

  if (!open) return null;

  async function copyToken() {
    if (!issuedToken?.token) return;
    await navigator.clipboard?.writeText(issuedToken.token);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  }

  return (
    <div className="fixed inset-0 z-[70] flex items-center justify-center bg-slate-950/55 p-4 backdrop-blur-sm">
      <Card className="w-full max-w-xl border-slate-200/80 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900">
        <CardHeader className="flex flex-row items-start justify-between gap-4 border-b border-slate-200/80 pb-4 dark:border-slate-800">
          <div>
            <CardTitle className="flex items-center gap-2 text-xl text-slate-900 dark:text-white">
              <KeyRound className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />
              {editing ? t("tokensEditTitle") : t("tokensCreateTitle")}
            </CardTitle>
            <CardDescription className="mt-1">
              {editing ? t("tokensEditHint") : mode === "admin" ? t("tokensCreateAdminHint") : t("tokensCreateConsoleHint")}
            </CardDescription>
          </div>
          <Button variant="ghost" size="icon" onClick={onClose} disabled={busy} aria-label={t("close")}>
            <X className="h-4 w-4" />
          </Button>
        </CardHeader>

        <CardContent className="space-y-5 pt-5">
          {issuedToken && !editing ? (
            <div className="space-y-4">
              <div className="rounded-xl border border-emerald-500/30 bg-emerald-50 p-4 text-sm text-emerald-800 dark:bg-emerald-500/10 dark:text-emerald-200">
                <div className="font-semibold">{t("tokensCreateSuccess")}</div>
                <div className="mt-1 text-xs opacity-80">{t("tokensSecretOnce")}</div>
              </div>
              <div className="space-y-2">
                <Label>{t("tokensSecretLabel")}</Label>
                <div className="flex gap-2">
                  <Input readOnly value={issuedToken.token} className="font-mono text-xs" />
                  <Button type="button" variant="outline" size="icon" onClick={() => void copyToken()} title={t("tokensCopySecret")}>
                    {copied ? <Check className="h-4 w-4 text-emerald-600" /> : <Copy className="h-4 w-4" />}
                  </Button>
                </div>
              </div>
              <Button type="button" className="w-full" onClick={onClose}>{t("done")}</Button>
            </div>
          ) : (
            <form className="space-y-4" onSubmit={(event) => void onSubmit(event)}>
              {mode === "admin" ? (
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="token-tenant-id">{t("tokensTenantID")}</Label>
                    <Input id="token-tenant-id" value={form.tenant_id} onChange={(event) => setForm((current) => ({ ...current, tenant_id: event.target.value }))} placeholder="tenant UUID" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="token-project-id">{t("tokensProjectID")}</Label>
                    <Input id="token-project-id" value={form.project_id} onChange={(event) => setForm((current) => ({ ...current, project_id: event.target.value }))} placeholder="project UUID" />
                  </div>
                </div>
              ) : (
                <div className="space-y-2">
                  <Label htmlFor="token-project-select">{t("tokensProjectID")}</Label>
                  {projectIDs.length > 0 ? (
                    <select id="token-project-select" value={form.project_id} onChange={(event) => setForm((current) => ({ ...current, project_id: event.target.value }))} className="flex h-10 w-full rounded-md border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 outline-none ring-offset-white focus-visible:ring-2 focus-visible:ring-indigo-500/30 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100 dark:ring-offset-slate-950">
                      <option value="">{t("tokensProjectPlaceholder")}</option>
                      {projectIDs.map((projectID) => <option key={projectID} value={projectID}>{projectID}</option>)}
                    </select>
                  ) : (
                    <Input id="token-project-select" value="" readOnly placeholder={t("tokensNoProjects")} />
                  )}
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="token-group-select">{t("tokensGroup")}</Label>
                <select id="token-group-select" value={form.group_id} onChange={(event) => setForm((current) => ({ ...current, group_id: event.target.value }))} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100" required disabled={groupsBusy}>
                  <option value="">{groupsBusy ? t("tokensGroupsLoading") : groups.length > 0 ? t("tokensGroupPlaceholder") : t("tokensNoGroups")}</option>
                  {groups.map((group) => <option key={group.id} value={group.id}>{group.name} · x{formatDecimalWithoutTrailingZeros(group.multiplier)}{group.billing_type === "free" ? ` · ${t("groupsBillingFree")}` : ""}</option>)}
                </select>
                <p className="text-[11px] text-slate-500 dark:text-slate-400">{t("tokensGroupBindingHint")}</p>
              </div>

              <section className="space-y-3 rounded-xl border border-indigo-500/20 bg-indigo-50/60 p-4 dark:border-indigo-400/20 dark:bg-indigo-500/10" aria-labelledby="token-group-models-title">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <h3 id="token-group-models-title" className="text-sm font-semibold text-slate-900 dark:text-white">{t("tokensAvailableModels")}</h3>
                    <p className="mt-1 text-[11px] text-slate-500 dark:text-slate-400">{selectedGroup ? selectedGroup.name : t("tokensSelectGroupFirst")}</p>
                  </div>
                  {selectedGroup ? <span className="shrink-0 rounded-md border border-indigo-500/20 bg-white/70 px-2 py-1 font-mono text-[10px] text-indigo-700 dark:border-indigo-400/20 dark:bg-slate-900/50 dark:text-indigo-300">{selectedGroup.models.length} {t("tokensModelsUnit")}</span> : null}
                </div>
                {selectedGroup && selectedGroup.models.length > 0 ? (
                  <div className="flex max-h-28 flex-wrap gap-1.5 overflow-y-auto pr-1">
                    {selectedGroup.models.map((model) => <span key={model} className="max-w-full break-all rounded-md border border-indigo-500/20 bg-white/80 px-2 py-1 font-mono text-[11px] leading-4 text-slate-700 dark:border-indigo-400/20 dark:bg-slate-950/50 dark:text-slate-200">{model}</span>)}
                  </div>
                ) : (
                  <p className="text-xs text-slate-500 dark:text-slate-400">{selectedGroup ? t("tokensGroupNoModels") : t("tokensSelectGroupFirst")}</p>
                )}
              </section>

              <div className="space-y-3 rounded-xl border border-slate-200/80 bg-slate-50/70 p-4 dark:border-slate-800 dark:bg-slate-950/40">
                <div>
                  <div className="text-sm font-semibold text-slate-800 dark:text-slate-200">{t("tokensNetworkAllowlistTitle")}</div>
                  <p className="mt-1 text-[11px] leading-5 text-slate-500 dark:text-slate-400">{t("tokensNetworkAllowlistHint")}</p>
                </div>
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="token-allowed-ips">{t("tokensAllowedIPs")}</Label>
                    <textarea id="token-allowed-ips" rows={3} value={form.allowed_ips} onChange={(event) => setForm((current) => ({ ...current, allowed_ips: event.target.value }))} placeholder={t("tokensAllowedIPsPlaceholder")} className="flex min-h-[84px] w-full resize-y rounded-xl border border-slate-200 bg-white px-3.5 py-2.5 text-xs text-slate-900 placeholder:text-slate-400 outline-none transition-colors focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:placeholder:text-slate-500" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="token-allowed-domains">{t("tokensAllowedDomains")}</Label>
                    <textarea id="token-allowed-domains" rows={3} value={form.allowed_domains} onChange={(event) => setForm((current) => ({ ...current, allowed_domains: event.target.value }))} placeholder={t("tokensAllowedDomainsPlaceholder")} className="flex min-h-[84px] w-full resize-y rounded-xl border border-slate-200 bg-white px-3.5 py-2.5 text-xs text-slate-900 placeholder:text-slate-400 outline-none transition-colors focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:placeholder:text-slate-500" />
                  </div>
                </div>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="token-name">{t("tokensName")}</Label>
                  <Input id="token-name" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} placeholder={t("tokensNamePlaceholder")} maxLength={100} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="token-expires-at">{t("tokensExpiresAt")}</Label>
                  <Input id="token-expires-at" type="datetime-local" value={form.expires_at} onChange={(event) => setForm((current) => ({ ...current, expires_at: event.target.value }))} />
                </div>
              </div>

              {message.text ? <div className="rounded-lg border border-rose-500/30 bg-rose-50 p-3 text-xs text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{message.text}</div> : null}
              <div className="flex justify-end gap-2 pt-2">
                <Button type="button" variant="outline" onClick={onClose} disabled={busy}>{t("cancel")}</Button>
                <Button type="submit" disabled={busy || groupsBusy || groups.length === 0 || (mode === "console" && projectIDs.length === 0)} className="gap-2">
                  {busy ? <span className="h-4 w-4 rounded-full border-2 border-white/30 border-t-white animate-spin" /> : <KeyRound className="h-4 w-4" />}
                  {editing ? t("tokensSaveAction") : t("tokensCreateAction")}
                </Button>
              </div>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
