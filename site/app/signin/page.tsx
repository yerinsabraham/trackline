import type { Metadata } from "next";
import SignIn from "./SignIn";

export const metadata: Metadata = {
  title: "Sign in",
  robots: { index: false },
  alternates: { canonical: "/signin" },
};

export default function Page() {
  return <SignIn />;
}
