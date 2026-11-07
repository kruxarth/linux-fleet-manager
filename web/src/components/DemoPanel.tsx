import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, demoInfo, type DemoNodes, type FleetNode } from "../api";
import NodeFace from "./NodeFace";
import StatePill from "./StatePill";

/**
 * Demo machines: pretend Linux nodes running as Docker containers on this computer.
 * Only shown when the manager runs with FLEET_DEMO_MODE. Each one joins the fleet
 * with its own single-use invite code, exactly like a real machine would.
 */
export default function DemoPanel({ fleet, onChange }: { fleet: FleetNode[] | null; onChange: () => void }) {
  const [st, setSt] = useState<DemoNodes | null>(null);
  const [busy, setBusy] = useState<string>("");
  const [note, setNote] = useState<string>("");
  const [err, setErr] = useState<string>("");
  const [offlineAfter, setOfflineAfter] = useState<number>(90);

  async function load() {
    try {
      setSt(await api.demoNodes());
    } catch {
      setSt(null);
    }
  }

  useEffect(() => {
    void demoInfo().then((i) => i.offline_after && setOfflineAfter(Math.round(i.offline_after)));
    void load();
    const t = setInterval(() => void load(), 3000);
    return () => clearInterval(t);
  }, []);

  if (!st || !st.enabled) return null;

  async function act(key: string, fn: () => Promise<unknown>, done: string) {
    setBusy(key);
    setErr("");
    setNote("");
    try {
      await fn();
      setNote(done);
      await load();
      onChange();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "That didn't work");
    } finally {
      setBusy("");
    }
  }

  const byHost = new Map((fleet ?? []).map((n) => [n.hostname, n]));

  return (
    <section className="card demo" aria-labelledby="demo-h">
      <div className="card-head">
        <div>
          <h2 id="demo-h">Demo machines</h2>
          <p className="soft">
            Pretend Linux machines running as Docker containers on this computer. Each one joins with its own invite code, like a
            real server would.
          </p>
        </div>
        <button
          type="button"
          className="btn"
          disabled={!!busy || st.docker_ok === false}
          onClick={() =>
            act("add", api.demoAdd, "New machine started. It joins the fleet in a few seconds, as soon as its first heartbeat arrives.")
          }
        >
          {busy === "add" ? "Starting…" : "Add a machine"}
        </button>
      </div>

      {st.docker_ok === false ? (
        <div className="notice warn" role="status">
          <b>Docker isn't reachable from the manager.</b> {st.error} The rest of the console still works.
        </div>
      ) : null}
      {err ? (
        <div className="notice bad" role="alert">
          {err}
        </div>
      ) : null}
      {note ? (
        <div className="notice ok" role="status">
          {note}
        </div>
      ) : null}

      {st.nodes.length === 0 && st.docker_ok !== false ? (
        <p className="soft pad">No demo machines right now. Add one to watch it join the fleet.</p>
      ) : (
        <ul className="demo-list">
          {st.nodes.map((d) => {
            const fn = byHost.get(d.name);
            const running = d.state === "running";
            return (
              <li key={d.name} className="demo-item" data-running={running}>
                <NodeFace size={46} mood={running ? "happy" : "sleep"} />
                <div className="demo-who">
                  {fn ? (
                    <Link to={`/nodes/${fn.id}`} className="host">
                      {d.name}
                    </Link>
                  ) : (
                    <span className="host">{d.name}</span>
                  )}
                  <div className="sub">{running ? "Plugged in" : "Unplugged"}</div>
                </div>
                <div className="demo-fleet">{fn ? <StatePill state={fn.state} /> : <StatePill state="pending">Joining…</StatePill>}</div>
                <div className="actions">
                  {running ? (
                    <button
                      type="button"
                      className="btn secondary small"
                      disabled={!!busy}
                      onClick={() =>
                        act(
                          d.name,
                          () => api.demoStop(d.name),
                          `${d.name} is unplugged and its heartbeats have stopped. The manager marks it offline after ${offlineAfter}s of silence. Keep watching the fleet table.`,
                        )
                      }
                    >
                      Unplug
                    </button>
                  ) : (
                    <button
                      type="button"
                      className="btn secondary small"
                      disabled={!!busy}
                      onClick={() =>
                        act(d.name, () => api.demoStart(d.name), `${d.name} is plugged back in. It turns online again with its next heartbeat.`)
                      }
                    >
                      Plug back in
                    </button>
                  )}
                  <button
                    type="button"
                    className="btn danger small"
                    disabled={!!busy}
                    onClick={() => act(d.name + ":rm", () => api.demoRemove(d.name), `${d.name} was removed and its node revoked.`)}
                  >
                    Remove
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
