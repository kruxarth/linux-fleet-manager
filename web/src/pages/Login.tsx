import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, demoInfo, setToken } from "../api";
import NodeFace from "../components/NodeFace";

export default function Login() {
  const [token, setTok] = useState("");
  const [err, setErr] = useState("");
  const [pending, setPending] = useState(false);
  const [demoToken, setDemoToken] = useState("");
  const nav = useNavigate();

  useEffect(() => {
    void demoInfo().then((i) => {
      if (i.enabled && i.admin_token) setDemoToken(i.admin_token);
    });
  }, []);

  async function signIn(value: string) {
    setErr("");
    setPending(true);
    setToken(value.trim());
    try {
      await api.session();
      nav("/");
    } catch {
      setErr("That token was rejected. Check FLEET_ADMIN_TOKEN on the manager and try again.");
    } finally {
      setPending(false);
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    void signIn(token);
  }

  return (
    <main className="gate">
      <div className="gate-card hero">
        <div className="gate-face">
          <NodeFace size={150} wave bob mood="happy" />
        </div>
        <p className="kicker">Fleet Manager</p>
        <h1>Sign in to your fleet</h1>
        <p className="lede">
          This console talks to the Go control plane. Your Linux machines call in to it, so you never have to SSH out to them.
        </p>
        {demoToken ? (
          <div className="demo-signin" role="note">
            <p>
              <b>This is a demo install.</b> The admin token is <code>{demoToken}</code>.
            </p>
            <button type="button" className="btn" disabled={pending} onClick={() => void signIn(demoToken)}>
              {pending ? "Signing in…" : "Sign in to the demo"}
            </button>
          </div>
        ) : null}
        <form onSubmit={onSubmit}>
          <label htmlFor="token">Admin token</label>
          <input
            id="token"
            name="token"
            type="password"
            autoComplete="off"
            value={token}
            onChange={(e) => setTok(e.target.value)}
            aria-describedby="token-help"
            required
          />
          <p id="token-help" className="hint">
            The value of <code>FLEET_ADMIN_TOKEN</code> the manager was started with.
          </p>
          {err ? (
            <p className="error" role="alert">
              {err}
            </p>
          ) : null}
          <button type="submit" className="btn" disabled={pending}>
            {pending ? "Checking…" : "Sign in"}
          </button>
        </form>
      </div>
    </main>
  );
}
