import type { Metadata } from "next";
import Alerts from "./Alerts";

export const metadata: Metadata = { title: "Alerts", robots: { index: false } };

export default function Page() {
  return <Alerts />;
}
