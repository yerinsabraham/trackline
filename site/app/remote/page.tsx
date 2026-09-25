import type { Metadata } from "next";
// The address the laptop prints when pairing: short enough to type on a phone.
import Remote from "../app/remote/Remote";

export const metadata: Metadata = { title: "Remote", robots: { index: false } };

export default function Page() {
  return <Remote />;
}
