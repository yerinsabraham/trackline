// The app's icons: plain strokes in the text colour, one family everywhere.

const base = { viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", strokeWidth: 1.8, strokeLinecap: "round" as const, strokeLinejoin: "round" as const, "aria-hidden": true };

export const IconHome = () => <svg {...base}><path d="M3 10.5 12 3l9 7.5V20a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z" /></svg>;
export const IconChat = () => <svg {...base}><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" /></svg>;
export const IconLaptop = () => <svg {...base}><rect x="3" y="4" width="18" height="12" rx="2" /><path d="M2 20h20" /></svg>;
export const IconBell = () => <svg {...base}><path d="M18 8a6 6 0 0 0-12 0c0 7-3 9-3 9h18s-3-2-3-9" /><path d="M13.7 21a2 2 0 0 1-3.4 0" /></svg>;
export const IconUser = () => <svg {...base}><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></svg>;
export const IconPlus = () => <svg {...base} strokeWidth={2.3}><path d="M12 5v14M5 12h14" /></svg>;
export const IconSignOut = () => <svg {...base}><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4M16 17l5-5-5-5M21 12H9" /></svg>;
export const IconTerminal = () => <svg {...base} strokeWidth={2}><path d="m4 17 6-6-6-6M12 19h8" /></svg>;
export const IconShield = () => <svg {...base}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></svg>;
export const IconAlert = () => <svg {...base}><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" /><path d="M12 9v4M12 17h.01" /></svg>;
export const IconBack = () => <svg {...base} strokeWidth={2}><path d="m15 18-6-6 6-6" /></svg>;
export const IconChevron = () => <svg {...base} strokeWidth={2}><path d="m9 18 6-6-6-6" /></svg>;
export const IconClose = () => <svg {...base} strokeWidth={2}><path d="M18 6 6 18M6 6l12 12" /></svg>;
export const IconUp = () => <svg {...base} strokeWidth={2.2}><path d="M12 19V5M5 12l7-7 7 7" /></svg>;
export const IconStop = () => <svg viewBox="0 0 24 24" aria-hidden="true"><rect x="5" y="5" width="14" height="14" rx="2" fill="currentColor" /></svg>;
export const IconFace = () => <svg {...base}><path d="M3 7V5a2 2 0 0 1 2-2h2M17 3h2a2 2 0 0 1 2 2v2M21 17v2a2 2 0 0 1-2 2h-2M7 21H5a2 2 0 0 1-2-2v-2" /><path d="M8 14s1.5 2 4 2 4-2 4-2M9 9h.01M15 9h.01" /></svg>;
export const IconKey = () => <svg {...base}><circle cx="7.5" cy="15.5" r="4.5" /><path d="m10.7 12.3 9.3-9.3M17 6l3 3" /></svg>;
export const IconClock = () => <svg {...base}><path d="M21 12a9 9 0 1 1-9-9" /><path d="M12 7v5l3 2" /></svg>;
export const IconPhone = () => <svg {...base}><rect x="7" y="2" width="10" height="20" rx="2" /><path d="M11 18h2" /></svg>;
export const IconTarget = () => <svg {...base}><circle cx="12" cy="12" r="9" /><circle cx="12" cy="12" r="5" /><circle cx="12" cy="12" r="1.2" fill="currentColor" /></svg>;
