import type { Metadata } from "next";
import OnTrack from "./OnTrack";

export const metadata: Metadata = { title: "On track", robots: { index: false } };

export default function Page() {
  return <OnTrack />;
}
