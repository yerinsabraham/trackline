import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Overview from "./Overview";

export const metadata: Metadata = { title: "Dashboard", robots: { index: false } };

export default function Page() {
  return <AppShell section="overview"><Overview /></AppShell>;
}
