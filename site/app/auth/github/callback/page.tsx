import type { Metadata } from "next";
import Callback from "./Callback";

export const metadata: Metadata = { title: "Signing in", robots: { index: false } };

export default function Page() {
  return <Callback />;
}
