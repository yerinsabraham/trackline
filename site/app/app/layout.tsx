import { Suspense, type ReactNode } from "react";
import AppFrame from "@/components/app/AppFrame";

export default function Layout({ children }: { children: ReactNode }) {
  return <Suspense><AppFrame>{children}</AppFrame></Suspense>;
}
