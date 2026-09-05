import { Fragment, type ReactNode } from "react";

export function MarkdownContent({ content, className = "" }: { content: string; className?: string }) {
  const lines = content.replace(/\r\n?/g, "\n").split("\n");
  const nodes: ReactNode[] = [];
  let inCode = false;
  let codeLines: string[] = [];
  let listItems: string[] = [];

  const flushList = () => {
    if (listItems.length === 0) return;
    nodes.push(<ul key={`list-${nodes.length}`} className="my-3 list-disc space-y-1 pl-5">{listItems.map((item, index) => <li key={`${item}-${index}`}>{inlineMarkdown(item)}</li>)}</ul>);
    listItems = [];
  };

  lines.forEach((line, index) => {
    if (line.trim().startsWith("```")) {
      flushList();
      if (inCode) {
        nodes.push(<pre key={`code-${index}`} className="my-3 overflow-x-auto rounded-lg bg-slate-950 p-3 font-mono text-xs leading-5 text-slate-100"><code>{codeLines.join("\n")}</code></pre>);
        codeLines = [];
      }
      inCode = !inCode;
      return;
    }
    if (inCode) {
      codeLines.push(line);
      return;
    }
    const trimmed = line.trim();
    if (!trimmed) {
      flushList();
      return;
    }
    const heading = /^(#{1,3})\s+(.+)$/.exec(trimmed);
    if (heading) {
      flushList();
      const level = heading[1].length;
      const Tag = level === 1 ? "h2" : level === 2 ? "h3" : "h4";
      nodes.push(<Tag key={`heading-${index}`} className={level === 1 ? "mt-5 text-xl font-bold" : level === 2 ? "mt-4 text-lg font-bold" : "mt-3 text-base font-semibold"}>{inlineMarkdown(heading[2])}</Tag>);
      return;
    }
    const item = /^(?:[-*]|\d+\.)\s+(.+)$/.exec(trimmed);
    if (item) {
      listItems.push(item[1]);
      return;
    }
    flushList();
    nodes.push(<p key={`paragraph-${index}`} className="my-2 leading-7">{inlineMarkdown(trimmed)}</p>);
  });
  flushList();
  if (inCode && codeLines.length > 0) nodes.push(<pre key="code-final" className="my-3 overflow-x-auto rounded-lg bg-slate-950 p-3 font-mono text-xs leading-5 text-slate-100"><code>{codeLines.join("\n")}</code></pre>);
  return <div className={className}>{nodes.map((node, index) => <Fragment key={index}>{node}</Fragment>)}</div>;
}

function inlineMarkdown(value: string): ReactNode {
  const pattern = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\(https?:\/\/[^\s)]+\))/g;
  const parts = value.split(pattern);
  return parts.map((part, index) => {
    if (part.startsWith("**") && part.endsWith("**")) return <strong key={index}>{part.slice(2, -2)}</strong>;
    if (part.startsWith("`") && part.endsWith("`")) return <code key={index} className="rounded bg-slate-200/70 px-1 py-0.5 font-mono text-[0.9em] dark:bg-slate-800">{part.slice(1, -1)}</code>;
    const link = /^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)$/.exec(part);
    if (link) return <a key={index} href={link[2]} target="_blank" rel="noreferrer" className="text-indigo-600 underline dark:text-indigo-300">{link[1]}</a>;
    return part;
  });
}
