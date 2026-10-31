import type { Metadata } from "next";
import Production from "./Production";

export const metadata: Metadata = { title: "Production", robots: { index: false } };

export default function Page() {
  return <Production />;
}
