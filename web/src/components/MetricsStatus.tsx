import { useEffect, useState } from "react";
import { api, type MetricsStatus as Status } from "../api";
import { ago, useNow } from "../lib";
import StatePill from "./StatePill";

/** "Metrics collection": which Prometheus scrape targets are up. Hidden when the feature is off. */
export default function MetricsStatus() {
  const [st, setSt] = useState<Status | null>(null);
  const now = useNow(1000);

  useEffect(() => {
    const load = () =>
      api
        .metricsStatus()
        .then(setSt)
        .catch(() => setSt(null));
    void load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, []);

  if (!st || !st.enabled) return null;

  const up = st.targets.filter((t) => t.health === "up").length;

  return (
    <section className="card" aria-labelledby="mc-h">
      <div className="card-head">
        <div>
          <h2 id="mc-h">Metrics collection</h2>
          <p className="soft">
            Prometheus visits each target every few seconds to read its numbers (a “scrape”).
          </p>
        </div>
        {st.reachable ? (
          <span className="summary-chip">
            {up} of {st.targets.length} up
          </span>
        ) : null}
      </div>
      {!st.reachable ? (
        <div className="notice warn" role="status">
          <b>Prometheus unreachable.</b> The fleet, commands and logbook keep working; charts may be stale until it's back.
        </div>
      ) : st.targets.length === 0 ? (
        <p className="soft pad">Prometheus is running but has no scrape targets yet.</p>
      ) : (
        <div className="table-wrap">
          <table className="table compact">
            <thead>
              <tr>
                <th>Target</th>
                <th>Job</th>
                <th>Status</th>
                <th>Last scrape</th>
              </tr>
            </thead>
            <tbody>
              {st.targets.map((t) => (
                <tr key={`${t.job}/${t.instance}`}>
                  <td>
                    <span className="mono">{t.instance || "—"}</span>
                    {t.hostname ? <div className="sub">{t.hostname}</div> : null}
                  </td>
                  <td>{t.job}</td>
                  <td>
                    <StatePill state={t.health === "up" ? "online" : t.health === "down" ? "offline" : "pending"}>
                      {t.health === "up" ? "Up" : t.health === "down" ? "Down" : "Unknown"}
                    </StatePill>
                    {t.last_error ? <div className="err-line mono">{t.last_error}</div> : null}
                  </td>
                  <td className="soft">{ago(t.last_scrape, now)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
