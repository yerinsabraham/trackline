import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Machines from "./Machines";

export const metadata: Metadata = { title: "Machines", robots: { index: false } };

export default function Page() {
  return <AppShell section="machines"><Machines /></AppShell>;
}
