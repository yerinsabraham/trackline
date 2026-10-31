import type { Metadata } from "next";
import Machines from "./Machines";

export const metadata: Metadata = { title: "Machines", robots: { index: false } };

export default function Page() {
  return <Machines />;
}
