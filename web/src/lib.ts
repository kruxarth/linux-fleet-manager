import { useEffect, useState } from "react";

/** Re-renders the caller every `ms`, so "12s ago" labels keep ticking between polls. */
export function useNow(ms = 1000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), ms);
    return () => clearInterval(t);
  }, [ms]);
  return now;
}

export function ago(iso: string | undefined, now: number): string {
  if (!iso) return "never";
  const s = Math.max(0, (now - new Date(iso).getTime()) / 1000);
  if (s < 5) return "just now";
  if (s < 60) return `${Math.floor(s)}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

export function until(iso: string, now: number): string {
  const s = Math.max(0, Math.floor((new Date(iso).getTime() - now) / 1000));
  if (s === 0) return "expired";
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
  return `${Math.floor(s / 3600)}h ${String(Math.floor((s % 3600) / 60)).padStart(2, "0")}m`;
}

export function clock(iso: string | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "medium" });
}

export function durationMs(job: { started_at?: string; finished_at?: string }): string {
  if (!job.started_at || !job.finished_at) return "—";
  const ms = new Date(job.finished_at).getTime() - new Date(job.started_at).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "—";
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
}

/** Base URL of Grafana as the operator's browser sees it. Override at build time with VITE_GRAFANA_URL. */
export function grafanaBase(): string {
  const fromEnv = import.meta.env.VITE_GRAFANA_URL as string | undefined;
  if (fromEnv) return fromEnv.replace(/\/$/, "");
  return `${location.protocol}//${location.hostname}:3000`;
}

/** The Prometheus instance label for a node: its reported node_exporter address, if it names a host. */
export function scrapeInstance(addr: string | undefined): string | null {
  const a = (addr ?? "").trim();
  if (!a || a.startsWith(":")) return null;
  return a;
}
