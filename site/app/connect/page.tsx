import type { Metadata } from "next";
import Connect from "./Connect";

export const metadata: Metadata = { title: "Connect a machine", robots: { index: false } };

export default function Page() {
  return <Connect />;
}
