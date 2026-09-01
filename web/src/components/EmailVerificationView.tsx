import { useState, type FormEvent } from "react";
import { ArrowLeft, CheckCircle2, MailCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Language } from "@/types";

interface EmailVerificationViewProps {
  language: Language;
  token?: string;
  routeTo: (target: string) => void;
}

export function EmailVerificationView({ language, token: initialToken, routeTo }: EmailVerificationViewProps) {
  const zh = language === "zh";
  const [token, setToken] = useState(initialToken || "");
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [success, setSuccess] = useState(false);

  async function verify(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token.trim()) {
      setMessage(zh ? "请输入邮箱验证令牌。" : "Enter the email verification token.");
      return;
    }
    setBusy(true);
    setMessage("");
    setSuccess(false);
    try {
      const response = await fetch("/console/v1/auth/email/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ token: token.trim() }),
      });
      if (!response.ok) throw new Error();
      setSuccess(true);
      setMessage(zh ? "邮箱验证成功，现在可以登录控制台。" : "Email verified. You can now sign in to the console.");
    } catch {
      setMessage(zh ? "验证令牌无效或已过期，请重新发送验证邮件。" : "The verification token is invalid or expired. Request a new email.");
    } finally {
      setBusy(false);
    }
  }

  async function resend() {
    if (!email.trim()) {
      setMessage(zh ? "请输入注册邮箱。" : "Enter the registered email.");
      setSuccess(false);
      return;
    }
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch("/console/v1/auth/email/resend", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ email: email.trim() }),
      });
      if (!response.ok) throw new Error();
      setMessage(zh ? "如果账号处于待验证状态，新的验证邮件已发送。" : "If the account needs verification, a new email has been sent.");
    } catch {
      setMessage(zh ? "暂时无法发送验证邮件，请稍后重试。" : "The verification email could not be sent. Try again later.");
    } finally {
      setBusy(false);
    }
  }

  return <div className="mx-auto flex min-h-[58vh] max-w-md items-center py-10"><Card className="w-full"><CardHeader><div className="mb-2 flex h-12 w-12 items-center justify-center rounded-xl bg-cyan-500/10 text-cyan-600 dark:bg-cyan-500/15 dark:text-cyan-300"><MailCheck className="h-6 w-6" /></div><CardTitle>{zh ? "验证邮箱" : "Verify email"}</CardTitle><CardDescription>{zh ? "验证完成后，账号才可以登录租户控制台。" : "Verify your address before signing in to the tenant console."}</CardDescription></CardHeader><CardContent><form className="space-y-4" onSubmit={(event) => void verify(event)}><div className="space-y-2"><Label htmlFor="email-verification-token">{zh ? "验证令牌" : "Verification token"}</Label><Input id="email-verification-token" value={token} onChange={(event) => setToken(event.target.value)} autoComplete="one-time-code" disabled={busy || success} required /></div><div className="space-y-2"><Label htmlFor="email-verification-resend">{zh ? "注册邮箱" : "Registered email"}</Label><Input id="email-verification-resend" type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="email" disabled={busy || success} /></div>{message ? <p className={success ? "flex items-start gap-2 rounded-lg border border-emerald-500/30 bg-emerald-50 p-3 text-xs leading-5 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300" : "rounded-lg border border-rose-500/30 bg-rose-50 p-3 text-xs leading-5 text-rose-700 dark:bg-rose-500/10 dark:text-rose-300"}>{success ? <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" /> : null}{message}</p> : null}<Button type="submit" className="w-full" disabled={busy || success}>{busy ? (zh ? "正在验证..." : "Verifying...") : (zh ? "验证邮箱" : "Verify email")}</Button><Button type="button" variant="outline" className="w-full" onClick={() => void resend()} disabled={busy || success}>{zh ? "重新发送验证邮件" : "Resend verification email"}</Button><Button type="button" variant="ghost" className="w-full gap-2" onClick={() => routeTo("#login")}><ArrowLeft className="h-4 w-4" />{zh ? "返回登录" : "Back to sign in"}</Button></form></CardContent></Card></div>;
}
