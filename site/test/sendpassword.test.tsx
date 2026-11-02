import assert from "node:assert/strict";
import { test } from "node:test";
import { passwordKey, passwordMAC } from "../lib/sendpassword";

// The laptop computes the same value (engine/internal/relay/password_test.go,
// TestPasswordMACVector). If either side changes alone, every message sent
// with a send password is refused.
test("the send password makes the same MAC the laptop checks", async () => {
  const key = await passwordKey("dev_example", "c2FsdHNhbHRzYWx0c2FsdA", 600_000, "correct horse battery");
  const mac = await passwordMAC(key, new TextEncoder().encode('{"v":1}'));
  assert.equal(Buffer.from(mac).toString("hex"), "ad9aefb78a51d0f5028569e2a9a02aaf63c64d9ae89666a2a2d2aa9caeda690d");
});

test("another machine or another password makes another key", async () => {
  const job = new TextEncoder().encode('{"v":1}');
  const hex = async (m: string, p: string) => Buffer.from(await passwordMAC(await passwordKey(m, "c2FsdA", 1000, p), job)).toString("hex");
  assert.notEqual(await hex("dev_a", "correct horse battery"), await hex("dev_b", "correct horse battery"));
  assert.notEqual(await hex("dev_a", "correct horse battery"), await hex("dev_a", "correct horse battern"));
});
