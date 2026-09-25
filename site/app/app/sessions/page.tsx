import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Sessions from "./Sessions";

export const metadata: Metadata = { title: "Sessions", robots: { index: false } };

export default function Page() {
  return <AppShell section="sessions"><Sessions /></AppShell>;
}
