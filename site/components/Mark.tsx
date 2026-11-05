import { LOGO } from "@/lib/logo";

// A line that stays on its track: the product in one shape.
export default function Mark({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox={LOGO.bare} fill="none" aria-hidden="true">
      <path d={LOGO.curve} stroke="currentColor" strokeWidth={LOGO.stroke} strokeLinecap="round" />
      <circle cx={LOGO.ball.cx} cy={LOGO.ball.cy} r={LOGO.ball.r} fill={LOGO.orange} />
    </svg>
  );
}
