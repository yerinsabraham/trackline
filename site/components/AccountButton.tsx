"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { session } from "@/lib/account";

// "Sign in" for a visitor, "Dashboard" once signed in. The page is static, so
// it renders as "Sign in" and switches after loading if a session is kept.
export default function AccountButton() {
  const [signedIn, setSignedIn] = useState(false);
  useEffect(() => setSignedIn(!!session()), []);
  return signedIn ? (
    <Link className="btn btn-quiet" href="/app"><span className="plus">+</span>Dashboard</Link>
  ) : (
    <Link className="btn btn-quiet" href="/signin"><span className="plus">+</span>Sign in</Link>
  );
}
