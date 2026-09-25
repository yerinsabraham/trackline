import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Session from "./Session";

export const metadata: Metadata = { title: "Session", robots: { index: false } };

export default function Page() {
  return <AppShell section="sessions" chat><Session /></AppShell>;
}
