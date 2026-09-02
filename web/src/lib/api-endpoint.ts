import { PublicAPIEndpoint } from "@/types";

export interface APIEndpointURLs {
  root: string;
  openai: string;
  anthropic: string;
}

function normalizeGatewayRoot(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  try {
    const parsed = new URL(trimmed);
    let path = parsed.pathname.replace(/\/+$/, "");
    if (/\/v1$/i.test(path)) path = path.slice(0, -3);
    parsed.pathname = path || "/";
    parsed.search = "";
    parsed.hash = "";
    return parsed.toString().replace(/\/+$/, "");
  } catch {
    return trimmed.replace(/\/+$/, "").replace(/\/v1$/i, "");
  }
}

export function resolveAPIEndpointURLs(endpoint: Pick<PublicAPIEndpoint, "base_url" | "openai_base_url" | "anthropic_base_url">): APIEndpointURLs {
  const root = normalizeGatewayRoot(endpoint.base_url);
  return {
    root,
    openai: endpoint.openai_base_url?.trim() || (root ? `${root}/v1` : ""),
    anthropic: endpoint.anthropic_base_url?.trim() || root,
  };
}
