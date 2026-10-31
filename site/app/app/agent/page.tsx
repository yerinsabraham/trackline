import type { Metadata } from "next";
import Agent from "./Agent";

export const metadata: Metadata = { title: "Agent", robots: { index: false } };

export default function Page() {
  return <Agent />;
}
