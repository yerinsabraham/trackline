import type { Metadata } from "next";
import Home from "./Home";

export const metadata: Metadata = { title: "Home", robots: { index: false } };

export default function Page() {
  return <Home />;
}
