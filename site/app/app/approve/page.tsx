import type { Metadata } from "next";
import Approve from "./Approve";

export const metadata: Metadata = { title: "Needs your OK", robots: { index: false } };

export default function Page() {
  return <Approve />;
}
