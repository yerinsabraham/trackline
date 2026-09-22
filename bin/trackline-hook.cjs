#!/usr/bin/env node
/**
 * Runs the hook binary by hand.
 *
 * Rarely what you want. An agent invokes the hook before every tool call, and
 * going through node adds roughly 80ms to a binary that takes 14ms — which is
 * the whole reason the hook is compiled. So `trackline init` writes the path of
 * the real binary into the agent's configuration, never this shim.
 */
"use strict";
require("./trackline.cjs").run("trackline-hook", process.argv.slice(2));
