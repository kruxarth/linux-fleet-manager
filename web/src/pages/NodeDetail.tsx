import { useEffect, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, type CatalogItem, type CommandJob, type FleetNode } from "../api";
import GrafanaPanels from "../components/GrafanaPanels";
import NodeFace, { moodFor } from "../components/NodeFace";
import StatePill from "../components/StatePill";
import { ago, clock, durationMs, useNow } from "../lib";
import { Labels } from "./Fleet";

const CAN_RUN = new Set(["online", "degraded"]);

function outputText(job: CommandJob): string {
  const head = `$ ${job.argv.join(" ")}`;
  const body = [job.stdout, job.stderr].filter(Boolean).join("\n");
  if (job.status === "queued") return `${head}\n# waiting for the node to pick this up…`;
  if (job.status === "running") return `${head}\n# running…`;
  return `${head}\n${body || "# (no output)"}`;
}

export default function NodeDetail() {
  const { id } = useParams();
  const nav = useNavigate();
  const now = useNow(1000);
  const [node, setNode] = useState<FleetNode | null>(null);
  const [jobs, setJobs] = useState<CommandJob[]>([]);
  const [catalog, setCatalog] = useState<CatalogItem[]>([]);
  const [picked, setPicked] = useState<string | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState<string | null>(null);

  async function load() {
    if (!id) return;
    const [detail, cat] = await Promise.all([api.node(id), api.catalog()]);
    setNode(detail.node);
    setJobs(detail.commands ?? []);
    setCatalog(cat);
  }

  useEffect(() => {
    void load().catch((e) => setErr(e instanceof Error ? e.message : "failed"));
    const t = setInterval(() => void load().catch(() => undefined), 2000);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function run(commandId: string) {
    if (!id) return;
    setBusy(commandId);
    setErr("");
    try {
      const job = await api.run(id, commandId);
      setPicked(job.id);
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : "failed");
    } finally {
      setBusy(null);
    }
  }

  async function revoke() {
    if (!id) return;
    if (!confirm("Revoke this node? It will not be able to heartbeat again.")) return;
    await api.revoke(id);
    nav("/");
  }

  if (!node) {
    return err ? (
      <p className="error" role="alert">
        {err}
      </p>
    ) : (
      <p className="soft">Loading node…</p>
    );
  }

  const shown = jobs.find((j) => j.id === picked) ?? jobs[0];
  const canRun = CAN_RUN.has(node.state);

  return (
    <>
      <nav className="crumb" aria-label="Breadcrumb">
        <Link to="/">Your fleet</Link> <span aria-hidden="true">/</span> <span aria-current="page">{node.hostname}</span>
      </nav>

      <header className="node-head card">
        <NodeFace mood={moodFor(node)} size={88} label={`${node.hostname} is ${node.state}`} />
        <div className="node-head-main">
          <div className="title-row">
            <h1>{node.hostname}</h1>
            <StatePill state={node.state} />
          </div>
          <p className="lede">
            Last heartbeat {ago(node.last_heartbeat_at, now)}
            {node.last_error ? <> · last command {node.last_error}</> : null}
          </p>
        </div>
        <button type="button" className="btn danger" onClick={() => void revoke()}>
          Revoke this node
        </button>
      </header>

      {err ? (
        <p className="error" role="alert">
          {err}
        </p>
      ) : null}

      <section className="card" aria-labelledby="inv-h">
        <div className="card-head">
          <h2 id="inv-h">Inventory</h2>
        </div>
        <dl className="facts">
          <Fact term="Node ID">
            <span className="mono">{node.id}</span>
          </Fact>
          <Fact term="Operating system">
            {node.os || "—"} / {node.arch || "—"}
          </Fact>
          <Fact term="Kernel">
            <span className="mono">{node.kernel || "—"}</span>
          </Fact>
          <Fact term="Agent version">
            <span className="mono">{node.agent_version || "—"}</span>
          </Fact>
          <Fact term="Labels">
            <Labels labels={node.labels} />
          </Fact>
          <Fact term="Metrics address (node_exporter)">
            <span className="mono">{node.node_exporter_url || "not reported"}</span>
          </Fact>
          <Fact term="Joined">{clock(node.created_at)}</Fact>
          <Fact term="Last error">{node.last_error || "none"}</Fact>
        </dl>
      </section>

      <div className="split">
        <section className="card shadowed menu-card" aria-labelledby="menu-h">
          <div className="card-head">
            <div>
              <h2 id="menu-h">The menu</h2>
              <p className="soft">Runs only commands on the menu — no free-form shell.</p>
            </div>
          </div>
          <ul className="menu">
            {catalog.map((c) => (
              <li key={c.id}>
                <div className="menu-item">
                  <code className="mono cmd">{c.argv.join(" ")}</code>
                  <span className="soft">
                    {c.description || c.id} · stops after {c.timeout_seconds}s
                  </span>
                </div>
                <button
                  type="button"
                  className="btn small"
                  onClick={() => void run(c.id)}
                  disabled={!canRun || busy !== null}
                  aria-label={`Run ${c.argv.join(" ")} on ${node.hostname}`}
                >
                  {busy === c.id ? "Queuing…" : "Run"}
                </button>
              </li>
            ))}
            {catalog.length === 0 ? <li className="soft">The menu is empty.</li> : null}
          </ul>
          <p className="menu-note">
            {canRun
              ? "Anything not on the menu is refused — checked by the manager, then again by the agent."
              : `This node is ${node.state}, so it can't take commands right now.`}
          </p>
        </section>

        <section className="card out-card" aria-labelledby="out-h">
          <div className="card-head">
            <div>
              <h2 id="out-h">Command output</h2>
              <p className="soft">{shown ? "Pick a row in the logbook to see its result here." : "Run something from the menu."}</p>
            </div>
            {shown ? <StatePill state={shown.status} /> : null}
          </div>
          <pre className="terminal" aria-live="polite" tabIndex={0}>
            {shown ? outputText(shown) : "# no commands yet — pick one from the menu"}
          </pre>
          {shown ? (
            <p className="out-meta soft">
              {shown.exit_code != null ? `Exit code ${shown.exit_code} · ` : ""}took {durationMs(shown)} · limit {shown.timeout_seconds}s
            </p>
          ) : null}
        </section>
      </div>

      <GrafanaPanels nodeExporterUrl={node.node_exporter_url} />

      <section className="card" aria-labelledby="log-h">
        <div className="card-head">
          <div>
            <h2 id="log-h">Logbook (audit trail)</h2>
            <p className="soft">Who asked this node to run what, and how it ended.</p>
          </div>
        </div>
        {jobs.length === 0 ? (
          <p className="soft pad">Nothing has been run on this node yet.</p>
        ) : (
          <div className="table-wrap">
            <table className="table compact">
              <thead>
                <tr>
                  <th>When</th>
                  <th>Who</th>
                  <th>Command</th>
                  <th>Result</th>
                  <th>Exit</th>
                  <th className="col-arch">Took</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((j) => (
                  <tr key={j.id} data-selected={j.id === shown?.id || undefined}>
                    <td className="nowrap" title={clock(j.created_at)}>
                      {ago(j.created_at, now)}
                    </td>
                    <td>admin</td>
                    <td>
                      <button type="button" className="linklike mono" onClick={() => setPicked(j.id)}>
                        {j.argv.join(" ")}
                      </button>
                    </td>
                    <td>
                      <StatePill state={j.status} />
                    </td>
                    <td className="mono">{j.exit_code ?? "—"}</td>
                    <td className="mono col-arch">{durationMs(j)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}

function Fact({ term, children }: { term: string; children: ReactNode }) {
  return (
    <div className="fact">
      <dt>{term}</dt>
      <dd>{children}</dd>
    </div>
  );
}
