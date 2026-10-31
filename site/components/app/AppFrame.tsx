"use client";

import { Fragment, type ReactNode } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import AppShell, { type Section } from "./AppShell";

// The frame stays while pages change beneath it, so moving between tabs keeps
// its data and shows no skeletons. Each page comes from the address alone.

const SECTIONS: [string, Section][] = [
  ["/app/ontrack", "ontrack"], ["/app/production", "production"], ["/app/sessions", "sessions"], ["/app/session", "sessions"],
  ["/app/machines", "machines"], ["/app/alerts", "alerts"], ["/app/remote", "new"],
];

export default function AppFrame({ children }: { children: ReactNode }) {
  const path = usePathname().replace(/\/$/, "");
  const q = useSearchParams();
  const section = SECTIONS.find(([p]) => path === p)?.[1] ?? "overview";
  const chat = path === "/app/session" || (path === "/app/remote" && !!q.get("job"));
  // A new page starts fresh, as does another session on the same page.
  return <AppShell section={section} chat={chat}><Fragment key={`${path}?${q.get("id") ?? ""}`}>{children}</Fragment></AppShell>;
}
