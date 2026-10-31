import type { Metadata } from "next";
import Overview from "./Overview";

export const metadata: Metadata = { title: "Dashboard", robots: { index: false } };

export default function Page() {
  return <Overview />;
}
