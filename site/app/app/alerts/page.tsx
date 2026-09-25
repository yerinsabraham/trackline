import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Alerts from "./Alerts";

export const metadata: Metadata = { title: "Alerts", robots: { index: false } };

export default function Page() {
  return <AppShell section="alerts"><Alerts /></AppShell>;
}
