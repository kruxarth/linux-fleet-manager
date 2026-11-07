import { FormEvent, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { api, type EnrollToken, type FleetNode } from "../api";
import CopyButton from "../components/CopyButton";
import NodeFace from "../components/NodeFace";
import { until, useNow } from "../lib";

export default function Enroll() {
  const [minutes, setMinutes] = useState(60);
  const [uses, setUses] = useState(1);
  const [labels, setLabels] = useState("role=lab");
  const [out, setOut] = useState<EnrollToken | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [joined, setJoined] = useState<FleetNode[]>([]);
  const known = useRef<Set<string>>(new Set());
  const now = useNow(1000);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setErr("");
    setBusy(true);
    const lab: Record<string, string> = {};
    for (const part of labels.split(",")) {
      const [k, v] = part.split("=").map((s) => s.trim());
      if (k && v) lab[k] = v;
    }
    try {
      // Remember who is already in the fleet so we can spot the newcomer.
      known.current = new Set((await api.nodes().catch(() => [])).map((n) => n.id));
      setJoined([]);
      setOut(await api.enroll({ ttl_seconds: minutes * 60, max_uses: uses, labels: lab }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : "failed");
    } finally {
      setBusy(false);
    }
  }

  // Once a code exists, watch for nodes that were not there before.
  useEffect(() => {
    if (!out) return;
    const t = setInterval(() => {
      api
        .nodes()
        .then((all) => setJoined(all.filter((n) => !known.current.has(n.id))))
        .catch(() => undefined);
    }, 3000);
    return () => clearInterval(t);
  }, [out]);

  const expired = out ? new Date(out.expires_at).getTime() <= now : false;
  const left = out ? Math.max(0, out.max_uses - joined.length) : 0;

  return (
    <>
      <header className="page-head">
        <div>
          <h1>Onboard a node</h1>
          <p className="lede">Three steps to bring a new Linux machine into the fleet.</p>
        </div>
      </header>

      <ol className="steps">
        <li className="step card">
          <span className="n" aria-hidden="true">1</span>
          <div className="step-body">
            <h2>Make an invite code (enrollment token)</h2>
            <p className="soft">Codes expire quickly and work a limited number of times, so a leaked one isn't a disaster.</p>
            <form className="enroll" onSubmit={onSubmit}>
              <label>
                Valid for (minutes)
                <input type="number" min={1} value={minutes} onChange={(e) => setMinutes(Math.max(1, Number(e.target.value)))} />
              </label>
              <label>
                Number of uses
                <input type="number" min={1} value={uses} onChange={(e) => setUses(Math.max(1, Number(e.target.value)))} />
              </label>
              <label className="wide">
                Labels (optional)
                <input value={labels} onChange={(e) => setLabels(e.target.value)} placeholder="role=web,env=lab" />
              </label>
              <button type="submit" className="btn" disabled={busy}>
                {busy ? "Creating…" : out ? "Create another invite code" : "Create invite code"}
              </button>
            </form>
            {err ? (
              <p className="error" role="alert">
                {err}
              </p>
            ) : null}
            {out ? (
              <dl className="code-facts">
                <div>
                  <dt>Expires in</dt>
                  <dd className={expired ? "bad" : ""}>{until(out.expires_at, now)}</dd>
                </div>
                <div>
                  <dt>Uses left</dt>
                  <dd>
                    {left} of {out.max_uses}
                  </dd>
                </div>
                <div>
                  <dt>Invite code</dt>
                  <dd className="mono code">{out.token}</dd>
                </div>
              </dl>
            ) : null}
          </div>
        </li>

        <li className={`step card${out ? " shadowed" : " dim"}`}>
          <span className="n" aria-hidden="true">2</span>
          <div className="step-body">
            <h2>Run the install command on the new computer</h2>
            <p className="soft">
              Needs root on a systemd Linux machine. It installs the agent, which then shows the code to the manager. The code is
              shown once, so copy it now.
            </p>
            {out ? (
              <>
                <pre className="terminal wrap" tabIndex={0}>
                  {out.install_cmd}
                </pre>
                <div className="actions">
                  <CopyButton text={out.install_cmd} label="Copy install command" />
                </div>
                <p className="hint">
                  Trying it on this machine instead? Run{" "}
                  <code>FLEET_MANAGER_URL={location.origin} FLEET_ENROLL_TOKEN={out.token} ./bin/agent</code>
                </p>
              </>
            ) : (
              <p className="placeholder">Create an invite code in step 1 and your install command will appear here.</p>
            )}
          </div>
        </li>

        <li className="step card">
          <span className="n" aria-hidden="true">3</span>
          <div className="step-body">
            <h2>The node joins the fleet</h2>
            <p className="soft">
              The agent shows the code to the manager and gets its own long-lived ID card (a node credential). Its first heartbeat
              moves it from pending to online.
            </p>
            {joined.length > 0 ? (
              <ul className="joined" aria-live="polite">
                {joined.map((n) => (
                  <li key={n.id}>
                    <NodeFace size={64} wave mood="happy" />
                    <div>
                      <b>{n.hostname || n.id.slice(0, 8)}</b> joined the fleet
                      <div>
                        <Link to={`/nodes/${n.id}`}>Open its page →</Link>
                      </div>
                    </div>
                  </li>
                ))}
              </ul>
            ) : out ? (
              <p className="waiting" role="status">
                <i className="dot" aria-hidden="true" />
                {expired ? "This code has expired. Create another one to continue." : "Waiting for a node to use the code…"}
              </p>
            ) : null}
          </div>
        </li>
      </ol>
    </>
  );
}
