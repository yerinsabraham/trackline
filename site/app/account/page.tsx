import type { Metadata } from "next";
import AppShell from "@/components/app/AppShell";
import Account from "./Account";

export const metadata: Metadata = { title: "Account", robots: { index: false } };

export default function Page() {
  return <AppShell section="account"><Account /></AppShell>;
}
