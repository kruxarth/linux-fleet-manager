import { useState } from "react";

export default function CopyButton({ text, label = "Copy" }: { text: string; label?: string }) {
  const [state, setState] = useState<"idle" | "copied" | "failed">("idle");

  async function copy() {
    try {
      await navigator.clipboard.writeText(text);
      setState("copied");
    } catch {
      setState("failed");
    }
    setTimeout(() => setState("idle"), 1800);
  }

  return (
    <button type="button" className="btn secondary" onClick={() => void copy()} aria-live="polite">
      {state === "copied" ? "Copied ✓" : state === "failed" ? "Copy failed — select the text" : label}
    </button>
  );
}
