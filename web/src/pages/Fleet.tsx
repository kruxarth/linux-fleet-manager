import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api, type FleetNode } from "../api";
import DemoPanel from "../components/DemoPanel";
import MetricsStatus from "../components/MetricsStatus";
import NodeFace, { moodFor, type Mood } from "../components/NodeFace";
import StatePill from "../components/StatePill";
import { ago, useNow } from "../lib";

type Tile = { key: string; label: string; hint: string; mood: Mood; count: number };

export default function Fleet() {
  const [nodes, setNodes] = useState<FleetNode[] | null>(null);
  const [err, setErr] = useState("");
  const now = useNow(1000);

  async function load() {
    try {
      setNodes(await api.nodes());
      setErr("");
    } catch (e) {
      setErr(e instanceof Error ? e.message : "failed");
    }
  }

  useEffect(() => {
    void load();
    const t = setInterval(() => void load(), 3000);
    return () => clearInterval(t);
  }, []);

  const counts = useMemo(() => {
    const c: Record<string, number> = { online: 0, degraded: 0, offline: 0, pending: 0 };
    for (const n of nodes ?? []) c[n.state] = (c[n.state] ?? 0) + 1;
    return c;
  }, [nodes]);

  const tiles: Tile[] = [
    { key: "online", label: "Online", hint: "Sending heartbeats", mood: "happy", count: counts.online },
    { key: "offline", label: "Offline", hint: "No heartbeat for a while", mood: "sleep", count: counts.offline },
    { key: "pending", label: "Pending", hint: "Joined, waiting for a first heartbeat", mood: "happy", count: counts.pending },
  ];
  if (counts.degraded > 0) {
    tiles.push({ key: "degraded", label: "Degraded", hint: "Online, but something is wrong", mood: "sick", count: counts.degraded });
  }

  return (
    <>
      <header className="page-head">
        <div>
          <h1>Your fleet</h1>
          <p className="lede">Every Linux machine that has joined. Heartbeats decide who can take work.</p>
        </div>
        <Link to="/enroll" className="btn">
          Onboard a node
        </Link>
      </header>

      {err ? (
        <p className="error" role="alert">
          {err}
        </p>
      ) : null}

      <ul className="tiles" aria-label="Fleet summary">
        {tiles.map((t) => (
          <li key={t.key} className="tile" data-state={t.key}>
            <NodeFace mood={t.mood} size={64} />
            <div>
              <div className="tile-count">{nodes ? t.count : "–"}</div>
              <div className="tile-label">{t.label}</div>
              <div className="tile-hint">{t.hint}</div>
            </div>
          </li>
        ))}
      </ul>

      <DemoPanel fleet={nodes} onChange={() => void load()} />

      {nodes && nodes.length === 0 ? (
        <section className="empty hero">
          <NodeFace size={150} wave bob />
          <h2>No nodes yet</h2>
          <p>Create an invite code, run one install command on a Linux machine, and it will show up here within about 15 seconds.</p>
          <Link to="/enroll" className="btn">
            Onboard your first node
          </Link>
        </section>
      ) : (
        <section className="card shadowed" aria-label="Nodes">
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th className="col-face">
                    <span className="sr-only">Avatar</span>
                  </th>
                  <th>Hostname</th>
                  <th>State</th>
                  <th>Last heartbeat</th>
                  <th>Kernel</th>
                  <th className="col-arch">Arch</th>
                  <th>Labels</th>
                </tr>
              </thead>
              <tbody>
                {(nodes ?? []).map((n) => (
                  <tr key={n.id} data-state={n.state}>
                    <td className="col-face">
                      <NodeFace mood={moodFor(n)} size={46} />
                    </td>
                    <td>
                      <Link to={`/nodes/${n.id}`} className="host rowlink">
                        {n.hostname || n.id.slice(0, 8)}
                      </Link>
                      <div className="sub mono">{n.id.slice(0, 8)}</div>
                    </td>
                    <td>
                      <StatePill state={n.state} />
                      {n.last_error ? <div className="err-line">Last command {n.last_error}</div> : null}
                    </td>
                    <td className="nowrap">{ago(n.last_heartbeat_at, now)}</td>
                    <td className="mono">{n.kernel || "—"}</td>
                    <td className="mono col-arch">{n.arch || "—"}</td>
                    <td>
                      <Labels labels={n.labels} />
                    </td>
                  </tr>
                ))}
                {!nodes ? (
                  <tr>
                    <td colSpan={7} className="soft pad">
                      Loading nodes…
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </section>
      )}

      <MetricsStatus />
    </>
  );
}

export function Labels({ labels }: { labels?: Record<string, string> }) {
  const entries = Object.entries(labels ?? {});
  if (entries.length === 0) return <span className="soft">—</span>;
  return (
    <span className="chips">
      {entries.map(([k, v]) => (
        <span key={k} className="chip mono">
          {k}={v}
        </span>
      ))}
    </span>
  );
}
