import React, { useMemo, useState } from "react";
import { KeyRound, Pencil, Plus, RefreshCw, ShieldCheck, ShieldOff, UserRound, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Language,
  LoginMessage,
  PlatformPermission,
  PlatformRole,
  PlatformRoleFormState,
  TranslationKey,
  UserSummary,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface AdminRoleManagementPanelProps {
  language: Language;
  currentUserID?: string;
  users: UserSummary[];
  roles: PlatformRole[];
  permissions: PlatformPermission[];
  busy: boolean;
  message: LoginMessage;
  refresh: (showPending?: boolean) => Promise<void>;
  saveRole: (form: PlatformRoleFormState) => Promise<boolean>;
  disableRole: (role: PlatformRole) => Promise<void>;
  loadUserRoles: (userID: string) => Promise<PlatformRole[]>;
  saveUserRoles: (user: UserSummary, roleIDs: string[]) => Promise<boolean>;
  canManage: boolean;
}

export function AdminRoleManagementPanel({
  language,
  currentUserID,
  users,
  roles,
  permissions,
  busy,
  message,
  refresh,
  saveRole,
  disableRole,
  loadUserRoles,
  saveUserRoles,
  canManage,
}: AdminRoleManagementPanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [roleForm, setRoleFormState] = useState<PlatformRoleFormState | null>(null);
  const setRoleForm = (next: PlatformRoleFormState | null) => {
    if (!canManage && next !== null) return;
    setRoleFormState(next);
  };
  const [roleUser, setRoleUser] = useState<UserSummary | null>(null);
  const [roleUserSelection, setRoleUserSelection] = useState<string[]>([]);
  const [roleUserBusy, setRoleUserBusy] = useState(false);
  const [roleUserError, setRoleUserError] = useState("");
  const [disableConfirm, setDisableConfirm] = useState("");

  const permissionGroups = useMemo(() => {
    const grouped = new Map<string, PlatformPermission[]>();
    for (const permission of permissions) {
      const items = grouped.get(permission.resource) || [];
      items.push(permission);
      grouped.set(permission.resource, items);
    }
    return Array.from(grouped.entries()).sort(([left], [right]) => left.localeCompare(right));
  }, [permissions]);
  const bindableUsers = users.filter((user) => user.status === "active");

  async function openUserRoles(user: UserSummary) {
    if (!canManage || user.id === currentUserID) return;
    setRoleUserBusy(true);
    setRoleUserError("");
    try {
      const assigned = await loadUserRoles(user.id);
      setRoleUser(user);
      setRoleUserSelection(assigned.map((role) => role.id));
    } catch {
      setRoleUserError(t("rolesBindingFailed"));
    } finally {
      setRoleUserBusy(false);
    }
  }

  async function submitRole(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage || !roleForm) return;
    if (await saveRole(roleForm)) setRoleForm(null);
  }

  async function submitUserRoles(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManage || !roleUser) return;
    setRoleUserBusy(true);
    setRoleUserError("");
    try {
      if (await saveUserRoles(roleUser, roleUserSelection)) setRoleUser(null);
    } finally {
      setRoleUserBusy(false);
    }
  }

  return (
    <div className="mt-6 space-y-5 border-t border-slate-200/80 pt-6 dark:border-slate-800/80">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="flex items-center gap-2 text-lg font-bold text-slate-900 dark:text-white">
            <ShieldCheck className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />
            {t("rolesTitle")}
          </h3>
          <p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">{t("rolesDescription")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => void refresh(true)} disabled={busy} className="gap-2">
            <RefreshCw className={cn("h-3.5 w-3.5", busy ? "animate-spin" : "")} />
            {t("rolesRefresh")}
          </Button>
          {canManage ? <Button type="button" size="sm" onClick={() => setRoleForm({ id: "", code: "", name: "", status: "active", permissions: [] })} disabled={busy} className="gap-2">
            <Plus className="h-3.5 w-3.5" />
            {t("rolesNew")}
          </Button> : null}
        </div>
      </div>

      {message.text ? <div className={cn("rounded-xl border p-3 text-xs", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : message.kind === "pending" ? "border-amber-500/30 bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300" : "border-emerald-500/30 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300")}>{message.text}</div> : null}
      {roleUserError ? <div className="rounded-xl border border-rose-500/30 bg-rose-50 p-3 text-xs text-rose-700 dark:bg-rose-500/10 dark:text-rose-300">{roleUserError}</div> : null}

      <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
        <table className="w-full min-w-[760px] text-sm">
          <thead className="border-b border-slate-200 bg-slate-50/80 text-left text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-950/50 dark:text-slate-400">
            <tr><th className="px-4 py-3 font-semibold">{t("rolesName")}</th><th className="px-4 py-3 font-semibold">{t("rolesPermissions")}</th><th className="px-4 py-3 font-semibold">{t("rolesMembers")}</th><th className="px-4 py-3 font-semibold">{t("rolesStatus")}</th><th className="px-4 py-3 text-right font-semibold">{t("rolesActions")}</th></tr>
          </thead>
          <tbody>
            {busy && roles.length === 0 ? <tr><td colSpan={5} className="py-10 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin" />{t("rolesLoading")}</td></tr> : roles.length === 0 ? <tr><td colSpan={5} className="py-10 text-center text-sm text-slate-500">{t("rolesEmpty")}</td></tr> : roles.map((role) => {
              const protectedRole = role.code === "platform_owner";
              return <tr key={role.id} className="border-b border-slate-100 last:border-0 dark:border-slate-800/80">
                <td className="px-4 py-4"><div className="font-semibold text-slate-900 dark:text-white">{role.name}</div><div className="mt-1 font-mono text-[11px] text-indigo-600 dark:text-indigo-300">{role.code}</div></td>
                <td className="max-w-[360px] px-4 py-4"><div className="flex flex-wrap gap-1.5">{role.permissions.length === 0 ? <span className="text-xs text-slate-400">{t("rolesNoPermissions")}</span> : role.permissions.map((permission) => <span key={permission} className="rounded-md bg-slate-100 px-2 py-1 font-mono text-[10px] text-slate-700 dark:bg-slate-800 dark:text-slate-300">{permission}</span>)}</div></td>
                <td className="px-4 py-4 text-slate-700 dark:text-slate-300">{role.member_count}</td>
                <td className="px-4 py-4"><Badge variant={role.status === "active" ? "success" : "muted"}>{role.status === "active" ? t("rolesActive") : t("rolesDisabled")}</Badge></td>
                <td className="whitespace-nowrap px-4 py-4 text-right"><div className="inline-flex items-center gap-1"><Button type="button" variant="ghost" size="icon" onClick={() => setRoleForm({ id: role.id, code: role.code, name: role.name, status: role.status, permissions: role.permissions })} disabled={busy || protectedRole} title={protectedRole ? t("rolesProtected") : t("rolesEdit")} aria-label={protectedRole ? t("rolesProtected") : t("rolesEdit")}><Pencil className="h-4 w-4" /></Button><Button type="button" variant={disableConfirm === role.id ? "destructive" : "ghost"} size="icon" onClick={() => { if (disableConfirm !== role.id) { setDisableConfirm(role.id); return; } setDisableConfirm(""); void disableRole(role); }} disabled={busy || protectedRole || role.status !== "active"} title={protectedRole ? t("rolesProtected") : disableConfirm === role.id ? t("rolesDisableConfirm") : t("rolesDisable")} aria-label={t("rolesDisable")}><ShieldOff className="h-4 w-4" /></Button></div></td>
              </tr>;
            })}
          </tbody>
        </table>
      </div>

      <section className="space-y-3 rounded-xl border border-cyan-500/20 bg-cyan-500/[0.04] p-4">
        <div><h4 className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-white"><UserRound className="h-4 w-4 text-cyan-600" />{t("rolesBindingsTitle")}</h4><p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("rolesBindingsDescription")}</p></div>
        <div className="overflow-x-auto rounded-lg border border-slate-200 dark:border-slate-800">
          <table className="w-full min-w-[640px] text-sm"><thead className="border-b border-slate-200 bg-white/70 text-left text-xs text-slate-500 dark:border-slate-800 dark:bg-slate-950/30 dark:text-slate-400"><tr><th className="px-3 py-2 font-semibold">{t("usersColName")}</th><th className="px-3 py-2 font-semibold">{t("rolesAssigned")}</th><th className="px-3 py-2 text-right font-semibold">{t("rolesActions")}</th></tr></thead><tbody>{bindableUsers.length === 0 ? <tr><td colSpan={3} className="py-8 text-center text-xs text-slate-500">{t("rolesNoUsers")}</td></tr> : bindableUsers.map((user) => <tr key={user.id} className="border-b border-slate-100 last:border-0 dark:border-slate-800/80"><td className="px-3 py-3"><div className="font-medium text-slate-900 dark:text-white">{user.display_name || user.email}</div><div className="text-xs text-slate-500">{user.email}</div></td><td className="px-3 py-3"><div className="flex flex-wrap gap-1">{user.platform_roles.length === 0 ? <Badge variant="muted">{t("rolesNoPlatformRole")}</Badge> : user.platform_roles.map((role) => <Badge key={role} variant="purple">{role}</Badge>)}</div></td><td className="px-3 py-3 text-right"><Button type="button" variant="outline" size="sm" onClick={() => void openUserRoles(user)} disabled={user.id === currentUserID || busy || roleUserBusy} className="gap-1.5 text-xs"><KeyRound className="h-3.5 w-3.5" />{user.id === currentUserID ? t("rolesCurrentAdmin") : t("rolesManageBinding")}</Button></td></tr>)}</tbody></table>
        </div>
      </section>

      {roleForm ? <div className="fixed inset-0 z-[90] flex items-center justify-center overflow-y-auto bg-slate-950/60 p-4 backdrop-blur-sm"><Card className="w-full max-w-2xl border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900"><CardHeader className="flex flex-row items-start justify-between gap-4 border-b border-slate-200 dark:border-slate-800"><div><CardTitle>{roleForm.id ? t("rolesEditTitle") : t("rolesCreateTitle")}</CardTitle><CardDescription>{t("rolesFormDescription")}</CardDescription></div><Button type="button" variant="ghost" size="icon" onClick={() => setRoleForm(null)} disabled={busy} aria-label={t("close")}><X className="h-4 w-4" /></Button></CardHeader><CardContent><form className="space-y-5" onSubmit={(event) => void submitRole(event)}><div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="platform-role-code">{t("rolesCode")}</Label><Input id="platform-role-code" value={roleForm.code} onChange={(event) => setRoleForm({ ...roleForm, code: event.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, "") })} placeholder="operations_admin" disabled={busy || roleForm.code === "platform_owner"} required /></div><div className="space-y-2"><Label htmlFor="platform-role-name">{t("rolesName")}</Label><Input id="platform-role-name" value={roleForm.name} onChange={(event) => setRoleForm({ ...roleForm, name: event.target.value })} disabled={busy || roleForm.code === "platform_owner"} required maxLength={100} /></div></div><div className="space-y-2"><Label htmlFor="platform-role-status">{t("rolesStatus")}</Label><select id="platform-role-status" value={roleForm.status} onChange={(event) => setRoleForm({ ...roleForm, status: event.target.value as PlatformRoleFormState["status"] })} disabled={busy || roleForm.code === "platform_owner"} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100"><option value="active">{t("rolesActive")}</option><option value="disabled">{t("rolesDisabled")}</option></select></div><div className="space-y-3"><div><Label>{t("rolesPermissionSelector")}</Label><p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t("rolesPermissionSelectorHint")}</p></div>{permissionGroups.length === 0 ? <div className="rounded-lg border border-dashed border-slate-300 p-4 text-xs text-slate-500 dark:border-slate-700">{t("rolesNoPermissions")}</div> : <div className="grid gap-3 sm:grid-cols-2">{permissionGroups.map(([resource, items]) => <div key={resource} className="rounded-lg border border-slate-200 p-3 dark:border-slate-800"><div className="mb-2 text-xs font-bold uppercase tracking-wide text-slate-500 dark:text-slate-400">{resource}</div><div className="space-y-2">{items.map((permission) => <label key={permission.id} className="flex items-start gap-2 text-xs text-slate-700 dark:text-slate-300"><input type="checkbox" checked={roleForm.permissions.includes(permission.name)} onChange={(event) => setRoleForm({ ...roleForm, permissions: event.target.checked ? [...roleForm.permissions, permission.name] : roleForm.permissions.filter((item) => item !== permission.name) })} disabled={busy || roleForm.code === "platform_owner"} /><span><span className="font-mono">{permission.name}</span><span className="ml-1 text-slate-400">{permission.resource}:{permission.action}</span></span></label>)}</div></div>)}</div>}</div><div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={() => setRoleForm(null)} disabled={busy}>{t("cancel")}</Button><Button type="submit" disabled={busy}>{busy ? t("rolesSaving") : t("rolesSave")}</Button></div></form></CardContent></Card></div> : null}

      {roleUser ? <div className="fixed inset-0 z-[90] flex items-center justify-center overflow-y-auto bg-slate-950/60 p-4 backdrop-blur-sm"><Card className="w-full max-w-lg border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900"><CardHeader className="flex flex-row items-start justify-between gap-4 border-b border-slate-200 dark:border-slate-800"><div><CardTitle>{t("rolesBindingTitle")}</CardTitle><CardDescription>{roleUser.display_name || roleUser.email} · {roleUser.email}</CardDescription></div><Button type="button" variant="ghost" size="icon" onClick={() => setRoleUser(null)} disabled={roleUserBusy} aria-label={t("close")}><X className="h-4 w-4" /></Button></CardHeader><CardContent><form className="space-y-5" onSubmit={(event) => void submitUserRoles(event)}><div className="space-y-2"><Label>{t("rolesBindingSelector")}</Label><p className="text-xs leading-5 text-slate-500 dark:text-slate-400">{t("rolesBindingSelectorHint")}</p><div className="space-y-2 rounded-xl border border-slate-200 p-3 dark:border-slate-800">{roles.filter((role) => role.status === "active").map((role) => <label key={role.id} className="flex items-start gap-3 rounded-lg px-2 py-2 text-sm hover:bg-slate-50 dark:hover:bg-slate-950/50"><input type="checkbox" checked={roleUserSelection.includes(role.id)} onChange={(event) => setRoleUserSelection(event.target.checked ? [...roleUserSelection, role.id] : roleUserSelection.filter((item) => item !== role.id))} disabled={roleUserBusy} /><span><span className="font-semibold text-slate-800 dark:text-slate-200">{role.name}</span><span className="ml-2 font-mono text-xs text-slate-500">{role.code}</span></span></label>)}{roles.filter((role) => role.status === "active").length === 0 ? <div className="text-xs text-slate-500">{t("rolesEmpty")}</div> : null}</div></div><div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={() => setRoleUser(null)} disabled={roleUserBusy}>{t("cancel")}</Button><Button type="submit" disabled={roleUserBusy}>{roleUserBusy ? t("rolesSaving") : t("rolesSaveBinding")}</Button></div></form></CardContent></Card></div> : null}
    </div>
  );
}
