// A score as a ring: the share that matched, drawn round a circle, with the
// number in the middle. Nothing checked draws an empty ring and a dash, never
// a number.

export function band(p: number | null): "good" | "fair" | "poor" | "none" {
  if (p === null) return "none";
  return p >= 90 ? "good" : p >= 70 ? "fair" : "poor";
}

export function verdict(p: number | null): string {
  if (p === null) return "Nothing checked yet";
  return p >= 95 ? "On track" : p >= 85 ? "Mostly on track" : p >= 70 ? "Drifting" : "Off track";
}

export default function Ring({ percent, size = 168, stroke = 14, label = true }: { percent: number | null; size?: number; stroke?: number; label?: boolean }) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const shown = percent ?? 0;
  return (
    <span className={`ring ring-${band(percent)}`} style={{ width: size, height: size }} role="img" aria-label={percent === null ? "Nothing checked yet" : `${percent}% on track`}>
      <svg viewBox={`0 0 ${size} ${size}`} width={size} height={size} aria-hidden="true">
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke} className="ring-track" />
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke} className="ring-value"
          strokeLinecap="round" strokeDasharray={c} strokeDashoffset={c * (1 - shown / 100)}
          style={{ ["--ring-c" as string]: c }} transform={`rotate(-90 ${size / 2} ${size / 2})`} />
      </svg>
      {label && <span className="ring-num" style={{ fontSize: size * 0.24 }}>{percent === null ? "—" : `${percent}%`}</span>}
    </span>
  );
}
