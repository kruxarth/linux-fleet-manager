import type { ReactNode } from "react";

const WORDS: Record<string, string> = {
  online: "Online",
  offline: "Offline",
  pending: "Pending",
  degraded: "Degraded",
  revoked: "Revoked",
  queued: "Queued",
  running: "Running",
  succeeded: "Succeeded",
  failed: "Failed",
  timeout: "Timed out",
  rejected: "Rejected",
};

/** Fully rounded status pill. Colour is never the only signal: the word is always there. */
export default function StatePill({ state, children }: { state: string; children?: ReactNode }) {
  return (
    <span className="pill" data-state={state}>
      <i className="dot" aria-hidden="true" />
      {children ?? WORDS[state] ?? state}
    </span>
  );
}
