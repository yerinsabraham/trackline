// A line that stays on its track: the product in one shape.
export default function Mark({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M3 17c4 0 5-10 9-10s5 10 9 10" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" />
      <circle cx="12" cy="7" r="2.3" fill="var(--signal)" />
    </svg>
  );
}
