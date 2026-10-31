import type { Metadata } from "next";
import Sessions from "./Sessions";

export const metadata: Metadata = { title: "Chats", robots: { index: false } };

export default function Page() {
  return <Sessions />;
}
