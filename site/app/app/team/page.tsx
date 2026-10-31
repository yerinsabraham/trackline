import type { Metadata } from "next";
import Team from "./Team";

export const metadata: Metadata = { title: "Team", robots: { index: false } };

export default function Page() {
  return <Team />;
}
