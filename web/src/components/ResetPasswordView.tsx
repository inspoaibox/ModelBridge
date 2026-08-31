import { FormEvent, useState } from "react";
import { ArrowLeft, KeyRound, Mail, Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language } from "@/types";

interface ResetPasswordViewProps {
  language: Language;
  token?: string;
  routeTo: (target: string) => void;
}

export function ResetPasswordView({ language, token, routeTo }: ResetPasswordViewProps) {
  const zh = language === "zh";
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  const requestReset = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/console/v1/auth/password-reset/request", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ email }),
      });
      if (!response.ok) throw new Error();
      setMessage(zh ? "如该邮箱已登记，重置链接已发送。请检查收件箱。" : "If the email is registered, a reset link has been sent.");
    } catch {
      setMessage(zh ? "暂时无法提交重置请求，请联系平台管理员。" : "The reset request is unavailable. Contact your platform administrator.");
    } finally {
      setBusy(false);
    }
  };

  const confirmReset = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (password.length < 12 || password !== confirmPassword) {
      setMessage(zh ? "请确认两次密码一致，且密码至少 12 个字符。" : "Passwords must match and contain at least 12 characters.");
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/console/v1/auth/password-reset/confirm", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ token, new_password: password }),
      });
      if (!response.ok) throw new Error();
      setMessage(zh ? "密码已重置，请使用新密码登录。" : "Password reset. Sign in with your new password.");
    } catch {
      setMessage(zh ? "重置链接无效或已过期。" : "The reset link is invalid or expired.");
    } finally {
      setBusy(false);
    }
  };

  const resetMode = Boolean(token);
  return (
    <div className="mx-auto flex min-h-[58vh] max-w-md items-center py-10">
      <Card className="w-full">
        <CardHeader>
          <CardTitle>{resetMode ? (zh ? "设置新密码" : "Set a new password") : (zh ? "重置密码" : "Reset your password")}</CardTitle>
          <CardDescription>{resetMode ? (zh ? "此操作将注销该账号的其他登录会话。" : "This action signs the account out of other sessions.") : (zh ? "输入邮箱后，我们会发送一次性重置链接。" : "Enter your email to receive a one-time reset link.")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={resetMode ? confirmReset : requestReset}>
            {resetMode ? (
              <>
                <div className="space-y-2"><Label htmlFor="reset-password">{zh ? "新密码" : "New password"}</Label><Input id="reset-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={12} required disabled={busy} /></div>
                <div className="space-y-2"><Label htmlFor="reset-password-confirm">{zh ? "确认新密码" : "Confirm password"}</Label><Input id="reset-password-confirm" type="password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} minLength={12} required disabled={busy} /></div>
              </>
            ) : (
              <div className="space-y-2"><Label htmlFor="reset-email">{zh ? "邮箱" : "Email"}</Label><div className="relative"><Mail className="absolute left-3 top-2.5 h-4 w-4 text-slate-400" /><Input id="reset-email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} className="pl-9" required disabled={busy} /></div></div>
            )}
            {message ? <p className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-xs leading-5 text-slate-700 dark:border-slate-800 dark:bg-slate-950/40 dark:text-slate-300">{message}</p> : null}
            <Button type="submit" className="w-full gap-2" disabled={busy}>{resetMode ? <KeyRound className="h-4 w-4" /> : <Send className="h-4 w-4" />}{busy ? (zh ? "正在提交..." : "Submitting...") : resetMode ? (zh ? "更新密码" : "Update password") : (zh ? "发送重置链接" : "Send reset link")}</Button>
            <Button type="button" variant="ghost" className="w-full gap-2" onClick={() => routeTo("#login")}><ArrowLeft className="h-4 w-4" />{zh ? "返回登录" : "Back to sign in"}</Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
