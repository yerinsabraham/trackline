import type { Metadata } from "next";
import Remote from "./Remote";

export const metadata: Metadata = { title: "New task", robots: { index: false } };

export default function Page() {
  return <Remote />;
}
