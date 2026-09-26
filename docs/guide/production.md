---
title: Production traces
order: 4
description: Watching a deployed agent through the traces it already sends.
---

# Production traces

A deployed agent does not have a hook. It has traces: OpenTelemetry records of
every model call and tool run, which most agent stacks can already send.
trackline reads those with the same engine that watches coding agents.

In production it **alerts, it cannot stop anything**. A trace is a record of
what already happened.

## Check exported traces

```bash
trackline traces exports/           # a directory of OTLP export files
trackline traces --json exports/    # one result per conversation
```

Files are OTLP export requests: `.json` in the OTLP/JSON format, anything else
protobuf.

## Receive them live

```bash
trackline serve
```

Then point your OTLP/HTTP exporter at `http://127.0.0.1:4318/v1/traces`. Both
protobuf, the default for every SDK, and JSON are accepted. Each conversation is
checked when it finishes.

```bash
trackline serve --sample 0.1                    # check 10% of conversations
trackline serve --alert https://hooks.slack.com/services/...
trackline serve --out results.jsonl              # keep every result
trackline serve --addr 0.0.0.0:4318              # listen beyond this machine
```

- **Sampling** keeps or drops whole conversations, never parts of one.
- **Alerts** post `{"text": ...}`, which Slack and most chat tools accept as is.
- **It listens on localhost by default**, because traces carry customer
  messages. Opening it to the network is your decision.

## See it on the dashboard

```bash
trackline connect                 # once, on the machine running trackline serve
trackline serve --connect
```

Each checked conversation then appears under **Production** on
[trackline.dev](https://trackline.dev/app/production), beside your coding
agents, and a broken policy or an incident alerts your phone: at most once per
service, per kind of problem, per hour.

The conversations are your customers', so what is sent is what trackline
concluded, never what was said:

| Sent | Never sent |
|---|---|
| service name, conversation id | customer messages, model replies |
| tool names, how many calls failed | tool arguments and results |
| policy findings, incidents | system prompts |
| model calls, tokens, latency | raw spans |

The contract is [`docs/contract/production-v1.schema.json`](../contract/production-v1.schema.json);
anything outside it is refused by the CLI and by the server. A recorded stream
can be sent the same way: `trackline traces --connect DIR`.

## Turn on content capture

This is the part that matters most. By default, OpenTelemetry's GenAI
instrumentation records that a tool was called, **not which one**. Without the
message content, trackline cannot check a tool policy and cannot know what the
user asked, and it reports that rather than passing the calls.

For the official Python instrumentation:

```bash
OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=SPAN_AND_EVENT
OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental
```

If your privacy review will not allow raw content, redact identifiers before
export. Alignment survives redaction, **as long as the redaction keeps the JSON
valid**: replace values inside strings, never across the serialised message.

## What it watches for

- **Your [tool policy](tool-policy.md)**: tools that must never be called, or
  need an approval step first.
- **Outages**: a tool failing at least 3 of its last 10 calls, and at least half
  of them. One alert per outage, not one per conversation.
- **Loops**: the same tool with the same arguments 3 or more times in one run.
- **Latency and cost drift**: a run more than 5 times the usual model latency
  (and over one second) or token use, once 10 runs have set the baseline.
- **Off-task tool use**, with `--judge`: see [the judge](judge.md).

The outage, loop and drift thresholds are defaults, not tuned to any one
deployment.
