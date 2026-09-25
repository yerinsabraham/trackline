import type { Metadata } from "next";
import NotMe from "./NotMe";

export const metadata: Metadata = { title: "Security check", robots: { index: false } };

export default function Page() {
  return <NotMe />;
}
