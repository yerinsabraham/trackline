import type { Metadata } from "next";
import Settings from "./Settings";

export const metadata: Metadata = { title: "Settings", robots: { index: false } };

export default function Page() {
  return <Settings />;
}
