import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type CommandJob, type FleetNode } from "../api";
import NodeFace from "../components/NodeFace";
import StatePill from "../components/StatePill";
import { ago, clock, durationMs, useNow } from "../lib";

export default function Commands() {
  const [jobs, setJobs] = useState<CommandJob[] | null>(null);
  const [names, setNames] = useState<Record<string, string>>({});
  const [err, setErr] = useState("");
  const now = useNow(1000);

  useEffect(() => {
    const tick = () => {
      api
        .commands()
        .then((j) => {
          setJobs(j ?? []);
          setErr("");
        })
        .catch((e) => setErr(e instanceof Error ? e.message : "failed"));
      api
        .nodes()
        .then((ns: FleetNode[]) => setNames(Object.fromEntries(ns.map((n) => [n.id, n.hostname]))))
        .catch(() => undefined);
    };
    tick();
    const t = setInterval(tick, 3000);
    return () => clearInterval(t);
  }, []);

  return (
    <>
      <header className="page-head">
        <div>
          <h1>Commands</h1>
          <p className="lede">Everything agents have been asked to run, newest first. Only menu commands, only on online nodes.</p>
        </div>
      </header>
      {err ? (
        <p className="error" role="alert">
          {err}
        </p>
      ) : null}
      {jobs && jobs.length === 0 ? (
        <section className="empty hero">
          <NodeFace size={150} wave bob />
          <h2>Nothing has run yet</h2>
          <p>Open an online node and pick a command from its menu. The result lands here and in that node's logbook.</p>
          <Link to="/" className="btn">
            Go to your fleet
          </Link>
        </section>
      ) : (
        <section className="card shadowed" aria-label="Commands">
          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Command</th>
                  <th>Node</th>
                  <th>Result</th>
                  <th>Exit</th>
                  <th className="col-arch">Took</th>
                  <th>Queued</th>
                </tr>
              </thead>
              <tbody>
                {(jobs ?? []).map((j) => (
                  <tr key={j.id}>
                    <td>
                      <Link to={`/nodes/${j.node_id}`} className="host mono">
                        {j.argv.join(" ")}
                      </Link>
                    </td>
                    <td>{names[j.node_id] ?? <span className="mono">{j.node_id.slice(0, 8)}</span>}</td>
                    <td>
                      <StatePill state={j.status} />
                    </td>
                    <td className="mono">{j.exit_code ?? "—"}</td>
                    <td className="mono col-arch">{durationMs(j)}</td>
                    <td className="nowrap" title={clock(j.created_at)}>
                      {ago(j.created_at, now)}
                    </td>
                  </tr>
                ))}
                {!jobs ? (
                  <tr>
                    <td colSpan={6} className="soft pad">
                      Loading…
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}
