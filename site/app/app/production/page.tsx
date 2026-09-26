import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Production from "./Production";

export const metadata: Metadata = { title: "Production", robots: { index: false } };

export default function Page() {
  return <AppShell section="production"><Production /></AppShell>;
}
