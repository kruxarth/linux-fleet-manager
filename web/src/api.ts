export type NodeState = "pending" | "online" | "degraded" | "offline" | "revoked";

export type FleetNode = {
  id: string;
  hostname: string;
  os: string;
  kernel: string;
  arch: string;
  agent_version: string;
  labels: Record<string, string>;
  state: NodeState;
  node_exporter_url?: string;
  last_heartbeat_at?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type CatalogItem = {
  id: string;
  argv: string[];
  timeout_seconds: number;
  description: string;
};

export type CommandJob = {
  id: string;
  node_id: string;
  command_id: string;
  argv: string[];
  status: string;
  stdout?: string;
  stderr?: string;
  exit_code?: number;
  timeout_seconds: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
};

export type EnrollToken = {
  id: string;
  token: string;
  expires_at: string;
  max_uses: number;
  install_url: string;
  install_cmd: string;
};

export type ScrapeTarget = {
  job: string;
  instance: string;
  hostname?: string;
  health: string;
  last_scrape?: string;
  last_error?: string;
};

export type MetricsStatus = {
  enabled: boolean;
  reachable: boolean;
  targets: ScrapeTarget[];
};

export type DemoInfo = {
  enabled: boolean;
  admin_token?: string;
  offline_after?: number;
};

/** A pretend Linux machine running as a Docker container (demo mode only). */
export type DemoNode = {
  name: string;
  state: string; // docker state: running, exited, created…
  status: string; // e.g. "Up 3 minutes"
};

export type DemoNodes = {
  enabled: boolean;
  docker_ok?: boolean;
  error?: string;
  nodes: DemoNode[];
};

/** Public: tells the login screen whether this is a demo install. Never throws. */
export async function demoInfo(): Promise<DemoInfo> {
  try {
    const res = await fetch("/api/v1/demo-info");
    if (!res.ok) return { enabled: false };
    return (await res.json()) as DemoInfo;
  } catch {
    return { enabled: false };
  }
}

const TOKEN_KEY = "fleet_admin_token";

export function getToken(): string | null {
  return sessionStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  sessionStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  sessionStorage.removeItem(TOKEN_KEY);
}

async function req<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers });
  if (res.status === 401) {
    clearToken();
    if (location.pathname !== "/login") location.assign("/login");
    throw new Error("unauthorized");
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    throw new Error(data?.error || res.statusText);
  }
  return data as T;
}

export const api = {
  session: () => req<{ ok: boolean }>("/api/v1/session"),
  nodes: () => req<FleetNode[]>("/api/v1/nodes"),
  node: (id: string) => req<{ node: FleetNode; commands: CommandJob[] }>(`/api/v1/nodes/${id}`),
  revoke: (id: string) => req<void>(`/api/v1/nodes/${id}`, { method: "DELETE" }),
  catalog: () => req<CatalogItem[]>("/api/v1/catalog"),
  enroll: (body: { ttl_seconds: number; max_uses: number; labels?: Record<string, string> }) =>
    req<EnrollToken>("/api/v1/enroll-tokens", { method: "POST", body: JSON.stringify(body) }),
  run: (nodeId: string, command_id: string) =>
    req<CommandJob>(`/api/v1/nodes/${nodeId}/commands`, {
      method: "POST",
      body: JSON.stringify({ command_id }),
    }),
  command: (id: string) => req<CommandJob>(`/api/v1/commands/${id}`),
  commands: () => req<CommandJob[]>("/api/v1/commands"),
  metricsStatus: () => req<MetricsStatus>("/api/v1/metrics-status"),
  demoNodes: () => req<DemoNodes>("/api/v1/demo/nodes"),
  demoAdd: () => req<DemoNode>("/api/v1/demo/nodes", { method: "POST" }),
  demoStop: (name: string) => req<void>(`/api/v1/demo/nodes/${encodeURIComponent(name)}/stop`, { method: "POST" }),
  demoStart: (name: string) => req<void>(`/api/v1/demo/nodes/${encodeURIComponent(name)}/start`, { method: "POST" }),
  demoRemove: (name: string) => req<void>(`/api/v1/demo/nodes/${encodeURIComponent(name)}`, { method: "DELETE" }),
};
