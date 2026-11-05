// The explainer's yellow node character, ported from the node() helper in
// explainer/story.html. Mood follows the node's state: happy = online,
// sleep = offline, sick = error. Use it as an avatar or in empty states only.
import type { FleetNode } from "../api";

export type Mood = "happy" | "sick" | "sleep";

export function moodFor(node: Pick<FleetNode, "state" | "last_error">): Mood {
  if (node.state === "offline" || node.state === "revoked") return "sleep";
  if (node.state === "degraded" || node.last_error) return "sick";
  return "happy";
}

type Props = {
  mood?: Mood;
  /** Rendered width in px. */
  size?: number;
  wave?: boolean;
  /** Show the little green agent with the antenna. */
  agent?: boolean;
  /** Gentle bobbing; only for big, decorative uses (login, empty states). */
  bob?: boolean;
  /** Accessible name. Omit when the face sits next to text that says the same thing. */
  label?: string;
};

const INK = "var(--ink)";

export default function NodeFace({ mood = "happy", size = 40, wave, agent, bob, label }: Props) {
  // Thin strokes vanish at avatar size, so scale them up a little when small.
  const k = size < 64 ? 1.45 : 1;
  const line = { stroke: INK, strokeWidth: 3 * k, fill: "none", strokeLinecap: "round", strokeLinejoin: "round" } as const;
  const top = agent ? -76 : -42;
  const right = wave ? 90 : 52;
  const vbW = right + 52;
  const vbH = 50 - top;
  const body = mood === "sleep" ? "#E7D9AE" : "var(--sun)";
  return (
    <svg
      className={`face${bob ? " face-bob" : ""}`}
      width={size}
      height={(size * vbH) / vbW}
      viewBox={`-52 ${top} ${vbW} ${vbH}`}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      focusable="false"
    >
      <g>
        <rect x="-28" y="34" width="14" height="10" rx="3" fill={INK} />
        <rect x="14" y="34" width="14" height="10" rx="3" fill={INK} />
        <rect x="-46" y="-36" width="92" height="74" rx="16" fill={body} stroke={INK} strokeWidth={3.5 * k} />
        <rect x="-34" y="20" width="68" height="6" rx="3" fill={INK} opacity=".18" />
        {mood === "happy" ? (
          <g>
            <circle cx="-15" cy="-8" r="5.5" fill={INK} />
            <circle cx="15" cy="-8" r="5.5" fill={INK} />
            <path d="M-11 6 Q0 15 11 6" {...line} />
          </g>
        ) : null}
        {mood === "sick" ? (
          <g>
            <path d="M-20 -13 l10 10 M-10 -13 l-10 10 M10 -13 l10 10 M20 -13 l-10 10" {...line} />
            <path d="M-11 10 q4 -5 8 0 t8 0" {...line} />
          </g>
        ) : null}
        {mood === "sleep" ? (
          <g>
            <path d="M-21 -7 h12 M9 -7 h12" {...line} />
            <path d="M-6 9 h12" {...line} />
            <text className="face-zz" x="40" y="-30" fontFamily="Fredoka, sans-serif" fontWeight="600" fontSize="20" fill={INK}>
              z
            </text>
          </g>
        ) : null}
        {wave ? (
          <g transform="translate(46,-4)">
            <g className="face-wave">
              <path d="M0 0 L26 -22" {...line} strokeWidth={5} />
              <circle cx="28" cy="-24" r="7" fill={body} stroke={INK} strokeWidth="3" />
            </g>
          </g>
        ) : null}
        {agent ? (
          <g transform="translate(34,-40)">
            <line x1="0" y1="-12" x2="0" y2="-24" {...line} />
            <circle cx="0" cy="-27" r="4" fill="var(--leaf)" stroke={INK} strokeWidth="2.5" />
            <circle r="13" fill="var(--leaf)" stroke={INK} strokeWidth="3" />
            <circle cx="-4" cy="-2" r="2.2" fill="#fff" />
            <circle cx="4" cy="-2" r="2.2" fill="#fff" />
            <path d="M-4 5 Q0 8 4 5" stroke="#fff" strokeWidth="2" fill="none" strokeLinecap="round" />
          </g>
        ) : null}
      </g>
    </svg>
  );
}
