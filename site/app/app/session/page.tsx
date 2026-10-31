import type { Metadata } from "next";
import Session from "./Session";

export const metadata: Metadata = { title: "Session", robots: { index: false } };

export default function Page() {
  return <Session />;
}
