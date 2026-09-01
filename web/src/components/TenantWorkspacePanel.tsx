import React, { useEffect, useState } from "react";
import { FolderKanban, Pencil, Plus, RefreshCw, ShieldCheck, Trash2, UserPlus, Users, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  Language,
  LoginMessage,
  ProjectFormState,
  ProjectMember,
  ProjectSummary,
  TenantMember,
  TranslationKey,
} from "@/types";
import { translations } from "@/locales/translations";
import { cn } from "@/lib/utils";

interface TenantWorkspacePanelProps {
  language: Language;
  canManageTenant: boolean;
  canManageProjects: boolean;
  projects: ProjectSummary[];
  projectsBusy: boolean;
  projectsMessage: LoginMessage;
  refreshProjects: (showPending?: boolean) => Promise<void>;
  saveProject: (form: ProjectFormState) => Promise<boolean>;
  deleteProject: (project: ProjectSummary) => Promise<void>;
  projectActionBusy: string;
  projectDeleteConfirm: string;
  members: TenantMember[];
  membersBusy: boolean;
  membersMessage: LoginMessage;
  refreshMembers: (showPending?: boolean) => Promise<void>;
  addMember: (email: string, role: TenantMember["role"]) => Promise<void>;
  updateMember: (member: TenantMember, role: TenantMember["role"], status: TenantMember["status"]) => Promise<void>;
  removeMember: (member: TenantMember) => Promise<void>;
  memberActionBusy: string;
  projectMembers: ProjectMember[];
  projectMembersBusy: boolean;
  projectMembersMessage: LoginMessage;
  selectedProjectID: string;
  selectProject: (projectID: string) => void;
  refreshProjectMembers: (projectID: string, showPending?: boolean) => Promise<void>;
  addProjectMember: (email: string, role: ProjectMember["role"]) => Promise<void>;
  updateProjectMember: (member: ProjectMember, role: ProjectMember["role"]) => Promise<void>;
  removeProjectMember: (member: ProjectMember) => Promise<void>;
  projectMemberActionBusy: string;
}

export function TenantWorkspacePanel({
  language,
  canManageTenant,
  canManageProjects,
  projects,
  projectsBusy,
  projectsMessage,
  refreshProjects,
  saveProject,
  deleteProject,
  projectActionBusy,
  projectDeleteConfirm,
  members,
  membersBusy,
  membersMessage,
  refreshMembers,
  addMember,
  updateMember,
  removeMember,
  memberActionBusy,
  projectMembers,
  projectMembersBusy,
  projectMembersMessage,
  selectedProjectID,
  selectProject,
  refreshProjectMembers,
  addProjectMember,
  updateProjectMember,
  removeProjectMember,
  projectMemberActionBusy,
}: TenantWorkspacePanelProps) {
  const t = (key: TranslationKey) => translations[language][key] ?? translations.en[key] ?? key;
  const [projectDialog, setProjectDialog] = useState<ProjectFormState | null>(null);
  const [memberEmail, setMemberEmail] = useState("");
  const [memberRole, setMemberRole] = useState<TenantMember["role"]>("developer");
  const [projectMemberEmail, setProjectMemberEmail] = useState("");
  const [projectMemberRole, setProjectMemberRole] = useState<ProjectMember["role"]>("developer");

  useEffect(() => {
    if (selectedProjectID && projects.some((project) => project.id === selectedProjectID)) return;
    if (projects[0]) selectProject(projects[0].id);
  }, [projects, selectedProjectID]);

  useEffect(() => {
    if (canManageProjects && selectedProjectID) void refreshProjectMembers(selectedProjectID);
  }, [canManageProjects, selectedProjectID]);

  useEffect(() => {
    if (canManageTenant) void refreshMembers();
  }, [canManageTenant]);

  const selectedProject = projects.find((project) => project.id === selectedProjectID) || null;
  const roleLabel = (role: string) => ({
    tenant_owner: t("consoleRoleOwner"),
    tenant_admin: t("consoleRoleAdmin"),
    project_admin: t("consoleRoleProjectAdmin"),
    developer: t("consoleRoleDeveloper"),
    viewer: t("consoleRoleViewer"),
  }[role] || role);

  async function submitProject(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!projectDialog) return;
    if (await saveProject(projectDialog)) setProjectDialog(null);
  }

  async function submitMember(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!memberEmail.trim()) return;
    await addMember(memberEmail.trim(), memberRole);
    setMemberEmail("");
  }

  async function submitProjectMember(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!projectMemberEmail.trim() || !selectedProject) return;
    await addProjectMember(projectMemberEmail.trim(), projectMemberRole);
    setProjectMemberEmail("");
  }

  return (
    <div className="space-y-6">
      <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
        <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <CardTitle className="flex items-center gap-2 text-lg"><FolderKanban className="h-5 w-5 text-indigo-600" />{t("consoleProjectsTitle")}</CardTitle>
            <CardDescription>{t("consoleProjectsDescription")}</CardDescription>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => void refreshProjects(true)} disabled={projectsBusy} className="gap-2"><RefreshCw className={cn("h-4 w-4", projectsBusy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button>
            {canManageTenant ? <Button size="sm" onClick={() => setProjectDialog({ id: "", name: "", slug: "", status: "active" })} className="gap-2"><Plus className="h-4 w-4" />{t("consoleProjectCreate")}</Button> : null}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <Notice message={projectsMessage} />
          <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800">
            <Table>
            <TableHeader><TableRow><TableHead>{t("consoleProjectName")}</TableHead><TableHead>{t("consoleProjectSlug")}</TableHead><TableHead>{t("consoleProjectStatus")}</TableHead><TableHead>{t("consoleProjectMembers")}</TableHead><TableHead className="text-right">{canManageProjects ? t("tokensActions") : t("consoleProjectAccess")}</TableHead></TableRow></TableHeader>
              <TableBody>
                {projectsBusy && projects.length === 0 ? <TableRow><TableCell colSpan={5} className="py-12 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-5 w-5 animate-spin" />{t("consoleUsageLoading")}</TableCell></TableRow> : projects.length === 0 ? <TableRow><TableCell colSpan={5} className="py-12 text-center text-sm text-slate-500">{t("consoleNoProjects")}</TableCell></TableRow> : projects.map((project) => <TableRow key={project.id} className={selectedProjectID === project.id ? "bg-indigo-500/[0.04]" : ""}>
                  <TableCell><button type="button" className="text-left font-semibold text-slate-900 hover:text-indigo-600 dark:text-white dark:hover:text-indigo-300" onClick={() => selectProject(project.id)}>{project.name}</button><div className="mt-1 font-mono text-[10px] text-slate-500">{project.id}</div></TableCell>
                  <TableCell className="font-mono text-xs text-slate-600 dark:text-slate-400">{project.slug}</TableCell>
                  <TableCell><Badge variant={project.status === "active" ? "success" : "muted"}>{project.status === "active" ? t("consoleStatusActive") : t("consoleStatusSuspended")}</Badge></TableCell>
                  <TableCell className="text-sm text-slate-600 dark:text-slate-300">{project.member_count}</TableCell>
                  <TableCell className="text-right"><div className="flex justify-end gap-1"><Button variant="ghost" size="icon" onClick={() => selectProject(project.id)} title={t("consoleProjectMembers")} aria-label={t("consoleProjectMembers")}><Users className="h-4 w-4" /></Button>{canManageTenant ? <><Button variant="ghost" size="icon" onClick={() => setProjectDialog({ id: project.id, name: project.name, slug: project.slug, status: project.status })} title={t("consoleProjectEdit")} aria-label={t("consoleProjectEdit")}><Pencil className="h-4 w-4" /></Button><Button variant={projectDeleteConfirm === project.id ? "destructive" : "ghost"} size="icon" onClick={() => void deleteProject(project)} disabled={Boolean(projectActionBusy)} title={t("consoleProjectDelete")} aria-label={t("consoleProjectDelete")}><Trash2 className="h-4 w-4" /></Button></> : null}</div></TableCell>
                </TableRow>)}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>

      {canManageTenant ? <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60">
        <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle className="flex items-center gap-2 text-lg"><Users className="h-5 w-5 text-cyan-600" />{t("consoleMembersTitle")}</CardTitle><CardDescription>{t("consoleMembersDescription")}</CardDescription></div><Button variant="outline" size="sm" onClick={() => void refreshMembers(true)} disabled={membersBusy} className="gap-2"><RefreshCw className={cn("h-4 w-4", membersBusy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button></CardHeader>
        <CardContent className="space-y-4"><Notice message={membersMessage} /><form onSubmit={(event) => void submitMember(event)} className="grid gap-3 rounded-xl border border-cyan-500/20 bg-cyan-500/[0.04] p-4 sm:grid-cols-[minmax(0,1fr)_180px_auto]"><div className="space-y-2"><Label htmlFor="tenant-member-email">{t("consoleMemberEmail")}</Label><Input id="tenant-member-email" type="email" value={memberEmail} onChange={(event) => setMemberEmail(event.target.value)} placeholder={t("consoleMemberEmailPlaceholder")} disabled={Boolean(memberActionBusy)} required /></div><div className="space-y-2"><Label htmlFor="tenant-member-role">{t("consoleMemberRole")}</Label><select id="tenant-member-role" value={memberRole} onChange={(event) => setMemberRole(event.target.value as TenantMember["role"])} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm dark:border-slate-700 dark:bg-slate-950" disabled={Boolean(memberActionBusy)}><option value="tenant_admin">{t("consoleRoleAdmin")}</option><option value="developer">{t("consoleRoleDeveloper")}</option><option value="viewer">{t("consoleRoleViewer")}</option></select></div><Button type="submit" className="self-end gap-2" disabled={Boolean(memberActionBusy)}><UserPlus className="h-4 w-4" />{t("consoleMemberAdd")}</Button></form>
          <div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800"><Table><TableHeader><TableRow><TableHead>{t("consoleMemberEmail")}</TableHead><TableHead>{t("consoleMemberRole")}</TableHead><TableHead>{t("consoleMemberStatus")}</TableHead><TableHead>{t("consoleMemberProjects")}</TableHead><TableHead className="text-right">{t("tokensActions")}</TableHead></TableRow></TableHeader><TableBody>{membersBusy && members.length === 0 ? <TableRow><TableCell colSpan={5} className="py-10 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-4 w-4 animate-spin" />{t("consoleUsageLoading")}</TableCell></TableRow> : members.length === 0 ? <TableRow><TableCell colSpan={5} className="py-10 text-center text-sm text-slate-500">{t("consoleNoProjects")}</TableCell></TableRow> : members.map((member) => <MemberRow key={member.user_id} member={member} t={t} busy={memberActionBusy === member.user_id} roleLabel={roleLabel} onSave={updateMember} onRemove={removeMember} />)}</TableBody></Table></div>
        </CardContent>
      </Card> : null}

      {canManageProjects && selectedProject && selectedProject.status === "active" ? <Card className="border-slate-200/80 shadow-sm dark:border-slate-800 dark:bg-slate-900/60"><CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between"><div><CardTitle className="flex items-center gap-2 text-lg"><ShieldCheck className="h-5 w-5 text-emerald-600" />{t("consoleProjectMembersTitle")} <span className="text-slate-400">/</span> {selectedProject.name}</CardTitle><CardDescription>{t("consoleProjectMembersDescription")}</CardDescription></div><Button variant="outline" size="sm" onClick={() => void refreshProjectMembers(selectedProject.id, true)} disabled={projectMembersBusy} className="gap-2"><RefreshCw className={cn("h-4 w-4", projectMembersBusy ? "animate-spin" : "")} />{t("consoleRefresh")}</Button></CardHeader><CardContent className="space-y-4"><Notice message={projectMembersMessage} /><form onSubmit={(event) => void submitProjectMember(event)} className="grid gap-3 rounded-xl border border-emerald-500/20 bg-emerald-500/[0.04] p-4 sm:grid-cols-[minmax(0,1fr)_180px_auto]"><div className="space-y-2"><Label htmlFor="project-member-email">{t("consoleProjectMemberEmail")}</Label><Input id="project-member-email" type="email" value={projectMemberEmail} onChange={(event) => setProjectMemberEmail(event.target.value)} placeholder={t("consoleMemberEmailPlaceholder")} disabled={Boolean(projectMemberActionBusy)} required /></div><div className="space-y-2"><Label htmlFor="project-member-role">{t("consoleProjectMemberRole")}</Label><select id="project-member-role" value={projectMemberRole} onChange={(event) => setProjectMemberRole(event.target.value as ProjectMember["role"])} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm dark:border-slate-700 dark:bg-slate-950" disabled={Boolean(projectMemberActionBusy)}><option value="project_admin">{t("consoleRoleProjectAdmin")}</option><option value="developer">{t("consoleRoleDeveloper")}</option><option value="viewer">{t("consoleRoleViewer")}</option></select></div><Button type="submit" className="self-end gap-2" disabled={Boolean(projectMemberActionBusy)}><UserPlus className="h-4 w-4" />{t("consoleProjectMemberAdd")}</Button></form><div className="overflow-x-auto rounded-xl border border-slate-200 dark:border-slate-800"><Table><TableHeader><TableRow><TableHead>{t("consoleProjectMemberEmail")}</TableHead><TableHead>{t("consoleProjectMemberRole")}</TableHead><TableHead className="text-right">{t("tokensActions")}</TableHead></TableRow></TableHeader><TableBody>{projectMembersBusy && projectMembers.length === 0 ? <TableRow><TableCell colSpan={3} className="py-10 text-center text-sm text-slate-500"><RefreshCw className="mx-auto mb-2 h-4 w-4 animate-spin" />{t("consoleUsageLoading")}</TableCell></TableRow> : projectMembers.length === 0 ? <TableRow><TableCell colSpan={3} className="py-10 text-center text-sm text-slate-500">{t("consoleProjectMemberEmpty")}</TableCell></TableRow> : projectMembers.map((member) => <ProjectMemberRow key={member.user_id} member={member} t={t} busy={projectMemberActionBusy === member.user_id} roleLabel={roleLabel} onSave={updateProjectMember} onRemove={removeProjectMember} />)}</TableBody></Table></div></CardContent></Card> : null}

      {projectDialog ? <div className="fixed inset-0 z-[80] flex items-center justify-center bg-slate-950/55 p-4 backdrop-blur-sm"><Card className="w-full max-w-lg border-slate-200 bg-white shadow-2xl dark:border-slate-700 dark:bg-slate-900"><CardHeader className="flex flex-row items-start justify-between gap-4"><div><CardTitle>{projectDialog.id ? t("consoleProjectEdit") : t("consoleProjectCreate")}</CardTitle><CardDescription>{t("consoleProjectsDescription")}</CardDescription></div><Button variant="ghost" size="icon" onClick={() => setProjectDialog(null)} disabled={Boolean(projectActionBusy)} aria-label={t("close")}><X className="h-4 w-4" /></Button></CardHeader><CardContent><form onSubmit={(event) => void submitProject(event)} className="space-y-4"><div className="space-y-2"><Label htmlFor="project-name">{t("consoleProjectName")}</Label><Input id="project-name" value={projectDialog.name} onChange={(event) => setProjectDialog({ ...projectDialog, name: event.target.value })} maxLength={128} required /></div><div className="space-y-2"><Label htmlFor="project-slug">{t("consoleProjectSlug")}</Label><Input id="project-slug" value={projectDialog.slug} onChange={(event) => setProjectDialog({ ...projectDialog, slug: event.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, "-") })} maxLength={64} pattern="[a-z][a-z0-9_-]*" required /></div>{projectDialog.id ? <div className="space-y-2"><Label htmlFor="project-status">{t("consoleProjectStatus")}</Label><select id="project-status" value={projectDialog.status} onChange={(event) => setProjectDialog({ ...projectDialog, status: event.target.value as ProjectFormState["status"] })} className="flex h-10 w-full rounded-xl border border-slate-200 bg-white px-3 text-sm dark:border-slate-700 dark:bg-slate-950"><option value="active">{t("consoleStatusActive")}</option><option value="disabled">{t("consoleStatusSuspended")}</option></select></div> : null}<div className="flex justify-end gap-2 pt-2"><Button type="button" variant="outline" onClick={() => setProjectDialog(null)} disabled={Boolean(projectActionBusy)}>{t("cancel")}</Button><Button type="submit" disabled={Boolean(projectActionBusy)}>{t("consoleProjectSave")}</Button></div></form></CardContent></Card></div> : null}
    </div>
  );
}

function MemberRow({ member, t, busy, roleLabel, onSave, onRemove }: { member: TenantMember; t: (key: TranslationKey) => string; busy: boolean; roleLabel: (role: string) => string; onSave: (member: TenantMember, role: TenantMember["role"], status: TenantMember["status"]) => Promise<void>; onRemove: (member: TenantMember) => Promise<void> }) {
  const [role, setRole] = useState(member.role);
  const [status, setStatus] = useState(member.status);
  return <TableRow><TableCell><div className="font-semibold text-slate-900 dark:text-white">{member.display_name || member.email}</div><div className="text-xs text-slate-500">{member.email}</div></TableCell><TableCell><select value={role} onChange={(event) => setRole(event.target.value as TenantMember["role"])} disabled={busy || member.role === "tenant_owner"} className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs dark:border-slate-700 dark:bg-slate-950"><option value="tenant_owner">{roleLabel("tenant_owner")}</option><option value="tenant_admin">{roleLabel("tenant_admin")}</option><option value="developer">{roleLabel("developer")}</option><option value="viewer">{roleLabel("viewer")}</option></select></TableCell><TableCell><select value={status} onChange={(event) => setStatus(event.target.value as TenantMember["status"])} disabled={busy || member.role === "tenant_owner"} className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs dark:border-slate-700 dark:bg-slate-950"><option value="active">{t("consoleStatusActive")}</option><option value="suspended">{t("consoleStatusSuspended")}</option></select></TableCell><TableCell>{member.project_count}</TableCell><TableCell className="text-right"><div className="flex justify-end gap-1"><Button variant="ghost" size="sm" onClick={() => void onSave(member, role, status)} disabled={busy || (role === member.role && status === member.status)}>{t("consoleMemberUpdateSuccess")}</Button><Button variant="ghost" size="icon" onClick={() => void onRemove(member)} disabled={busy || member.role === "tenant_owner"} title={t("consoleMemberRemove")} aria-label={t("consoleMemberRemove")}><Trash2 className="h-4 w-4" /></Button></div></TableCell></TableRow>;
}

function ProjectMemberRow({ member, t, busy, roleLabel, onSave, onRemove }: { member: ProjectMember; t: (key: TranslationKey) => string; busy: boolean; roleLabel: (role: string) => string; onSave: (member: ProjectMember, role: ProjectMember["role"]) => Promise<void>; onRemove: (member: ProjectMember) => Promise<void> }) {
  const [role, setRole] = useState(member.role);
  return <TableRow><TableCell><div className="font-semibold text-slate-900 dark:text-white">{member.display_name || member.email}</div><div className="text-xs text-slate-500">{member.email}</div></TableCell><TableCell><select value={role} onChange={(event) => setRole(event.target.value as ProjectMember["role"])} disabled={busy} className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs dark:border-slate-700 dark:bg-slate-950"><option value="project_admin">{roleLabel("project_admin")}</option><option value="developer">{roleLabel("developer")}</option><option value="viewer">{roleLabel("viewer")}</option></select></TableCell><TableCell className="text-right"><div className="flex justify-end gap-1"><Button variant="ghost" size="sm" onClick={() => void onSave(member, role)} disabled={busy || role === member.role}>{t("consoleProjectMemberUpdate")}</Button><Button variant="ghost" size="icon" onClick={() => void onRemove(member)} disabled={busy} title={t("consoleProjectMemberRemove")} aria-label={t("consoleProjectMemberRemove")}><Trash2 className="h-4 w-4" /></Button></div></TableCell></TableRow>;
}

function Notice({ message }: { message: LoginMessage }) {
  if (!message.text) return null;
  return <div className={cn("rounded-xl border p-3 text-xs", message.kind === "error" ? "border-rose-500/30 bg-rose-50 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300" : "border-indigo-500/30 bg-indigo-50 text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300")}>{message.text}</div>;
}
