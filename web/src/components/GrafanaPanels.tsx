import { useEffect, useState, type ReactNode } from "react";
import { grafanaBase, scrapeInstance } from "../lib";
import NodeFace from "./NodeFace";

// Panel ids come from dashboards/linux-node.json and are kept stable on purpose.
const STATS = [
  { id: 5, title: "CPU busy" },
  { id: 6, title: "Memory used" },
  { id: 7, title: "Disk used (/)" },
  { id: 8, title: "Uptime" },
];
const CHARTS = [
  { id: 1, title: "CPU busy over time" },
  { id: 2, title: "Memory over time" },
  { id: 3, title: "Load average" },
  { id: 9, title: "Network traffic" },
];

function panelUrl(base: string, panelId: number, instance: string): string {
  const q = new URLSearchParams({
    orgId: "1",
    panelId: String(panelId),
    "var-instance": instance,
    from: "now-1h",
    to: "now",
    refresh: "15s",
    theme: "light",
  });
  return `${base}/d-solo/linux-node/linux-node?${q.toString()}&kiosk`;
}

type Reach = "checking" | "up" | "down";

/** Probe Grafana without CORS: a network failure rejects, any HTTP answer resolves. */
function useGrafanaReachable(base: string): Reach {
  const [state, setState] = useState<Reach>("checking");
  useEffect(() => {
    const ctl = new AbortController();
    const timer = setTimeout(() => ctl.abort(), 3000);
    setState("checking");
    fetch(`${base}/api/health`, { mode: "no-cors", signal: ctl.signal, cache: "no-store" })
      .then(() => setState("up"))
      .catch(() => setState("down"))
      .finally(() => clearTimeout(timer));
    return () => {
      clearTimeout(timer);
      ctl.abort();
    };
  }, [base]);
  return state;
}

function Empty({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="empty small">
      <NodeFace mood="sleep" size={64} />
      <div>
        <h3>{title}</h3>
        <p>{children}</p>
      </div>
    </div>
  );
}

export default function GrafanaPanels({ nodeExporterUrl }: { nodeExporterUrl?: string }) {
  const base = grafanaBase();
  const reach = useGrafanaReachable(base);
  const instance = scrapeInstance(nodeExporterUrl);
  const dash = instance
    ? `${base}/d/linux-node/linux-node?orgId=1&var-instance=${encodeURIComponent(instance)}`
    : base;

  return (
    <section className="card" aria-labelledby="metrics-h">
      <div className="card-head">
        <div>
          <h2 id="metrics-h">Live metrics</h2>
          <p className="soft">CPU, memory, disk and network, charted by Grafana from node_exporter.</p>
        </div>
        {reach === "up" && instance ? (
          <a className="btn secondary" href={dash} target="_blank" rel="noopener noreferrer">
            Open in Grafana ↗
          </a>
        ) : null}
      </div>
      {!instance ? (
        <Empty title="No metrics address yet">
          This node hasn't told the manager where its node_exporter listens, so there's nothing to chart. Everything else on
          this page still works.
        </Empty>
      ) : reach === "down" ? (
        <Empty title="Grafana isn't reachable">
          Looking for it at <code>{base}</code>. The charts come back by themselves once Grafana is running. The fleet,
          commands and logbook don't depend on it.
        </Empty>
      ) : reach === "checking" ? (
        <p className="soft pad">Checking for Grafana…</p>
      ) : (
        <>
          <div className="panels stats">
            {STATS.map((p) => (
              <iframe
                key={p.id}
                title={`${p.title} (Grafana)`}
                src={panelUrl(base, p.id, instance)}
                loading="lazy"
                className="panel stat"
              />
            ))}
          </div>
          <div className="panels charts">
            {CHARTS.map((p) => (
              <iframe
                key={p.id}
                title={`${p.title} (Grafana)`}
                src={panelUrl(base, p.id, instance)}
                loading="lazy"
                className="panel chart"
              />
            ))}
          </div>
        </>
      )}
    </section>
  );
}
