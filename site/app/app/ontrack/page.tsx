import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import OnTrack from "./OnTrack";

export const metadata: Metadata = { title: "On track", robots: { index: false } };

export default function Page() {
  return <AppShell section="ontrack"><OnTrack /></AppShell>;
}
