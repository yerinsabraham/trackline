// The app's icons: plain strokes in the text colour, one family everywhere.
//
// Each carries its own size. An svg with none fills its box in Chrome and is
// 0×0 in WebKit, which left the back and send buttons blank on every iPhone
// while looking right in every desktop check (found in real use, 2026-10-31).

type Size = { size?: number };
const base = (size = 20) => ({
  width: size, height: size, viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 1.8,
  strokeLinecap: "round" as const, strokeLinejoin: "round" as const, "aria-hidden": true, style: { flex: "none" },
});

export const IconTeam = ({ size }: Size) => <svg {...base(size)}><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20a6.5 6.5 0 0 1 13 0M16 4.8a3.5 3.5 0 0 1 0 6.4M18 14.5a6.5 6.5 0 0 1 3.5 5.5" /></svg>;
export const IconGear = ({ size }: Size) => <svg {...base(size)}><circle cx="12" cy="12" r="3" /><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M18.7 5.3l-2.1 2.1M7.4 16.6l-2.1 2.1" /></svg>;
export const IconHome = ({ size }: Size) => <svg {...base(size)}><path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" /></svg>;
export const IconChat = ({ size }: Size) => <svg {...base(size)}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>;
export const IconLaptop = ({ size }: Size) => <svg {...base(size)}><rect x="3" y="4" width="18" height="12" rx="2" /><path d="M2 20h20" /></svg>;
export const IconBell = ({ size }: Size) => <svg {...base(size)}><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" /><path d="M13.7 21a2 2 0 0 1-3.4 0" /></svg>;
export const IconUser = ({ size }: Size) => <svg {...base(size)}><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></svg>;
export const IconPlus = ({ size }: Size) => <svg {...base(size)} strokeWidth={2.3}><path d="M12 5v14M5 12h14" /></svg>;
export const IconSignOut = ({ size }: Size) => <svg {...base(size)}><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" /></svg>;
export const IconTerminal = ({ size }: Size) => <svg {...base(size)} strokeWidth={2}><path d="m4 17 6-6-6-6M12 19h8" /></svg>;
export const IconShield = ({ size }: Size) => <svg {...base(size)}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>;
export const IconAlert = ({ size }: Size) => <svg {...base(size)}><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" /><path d="M12 9v4M12 17h.01" /></svg>;
export const IconBack = ({ size }: Size) => <svg {...base(size)} strokeWidth={2}><path d="m15 18-6-6 6-6" /></svg>;
export const IconChevron = ({ size }: Size) => <svg {...base(size)} strokeWidth={2}><path d="m9 18 6-6-6-6" /></svg>;
export const IconClose = ({ size }: Size) => <svg {...base(size)} strokeWidth={2}><path d="M18 6 6 18M6 6l12 12" /></svg>;
export const IconUp = ({ size }: Size) => <svg {...base(size)} strokeWidth={2.2}><path d="M12 19V5M5 12l7-7 7 7" /></svg>;
export const IconMore = ({ size = 20 }: Size) => <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" style={{ flex: "none" }} fill="currentColor"><circle cx="5" cy="12" r="1.8" /><circle cx="12" cy="12" r="1.8" /><circle cx="19" cy="12" r="1.8" /></svg>;
export const IconDown = ({ size }: Size) => <svg {...base(size)} strokeWidth={2.2}><path d="M12 5v14M5 12l7 7 7-7" /></svg>;
export const IconStop = ({ size = 20 }: Size) => <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" style={{ flex: "none" }}><rect x="5" y="5" width="14" height="14" rx="2" fill="currentColor" /></svg>;
export const IconFace = ({ size }: Size) => <svg {...base(size)}><path d="M3 7V5a2 2 0 0 1 2-2h2M17 3h2a2 2 0 0 1 2 2v2M21 17v2a2 2 0 0 1-2 2h-2M7 21H5a2 2 0 0 1-2-2v-2" /><path d="M8 14s1.5 2 4 2 4-2 4-2M9 9h.01M15 9h.01" /></svg>;
export const IconKey = ({ size }: Size) => <svg {...base(size)}><circle cx="7.5" cy="15.5" r="4.5" /><path d="m10.7 12.3 9.3-9.3M17 6l3 3" /></svg>;
export const IconClock = ({ size }: Size) => <svg {...base(size)}><path d="M21 12a9 9 0 1 1-9-9" /><path d="M12 7v5l3 2" /></svg>;
export const IconPhone = ({ size }: Size) => <svg {...base(size)}><rect x="7" y="2" width="10" height="20" rx="2" /><path d="M11 18h2" /></svg>;
export const IconTarget = ({ size }: Size) => <svg {...base(size)}><circle cx="12" cy="12" r="9" /><circle cx="12" cy="12" r="5" /><circle cx="12" cy="12" r="1.2" fill="currentColor" /></svg>;
export const IconClip = ({ size }: Size) => <svg {...base(size)}><path d="m21 11-8.6 8.6a5.5 5.5 0 0 1-7.8-7.8l8.6-8.6a3.7 3.7 0 0 1 5.2 5.2l-8.6 8.6a1.8 1.8 0 0 1-2.6-2.6L15 6.6" /></svg>;
export const IconServer = ({ size }: Size) => <svg {...base(size)}><rect x="3" y="4" width="18" height="7" rx="2" /><rect x="3" y="13" width="18" height="7" rx="2" /><path d="M7 7.5h.01M7 16.5h.01" /></svg>;
